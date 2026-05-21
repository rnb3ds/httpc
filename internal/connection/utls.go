// Package connection provides utls integration for TLS fingerprint spoofing.
package connection

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"
)

// resolveHelloID maps a browser fingerprint string to a utls ClientHelloID.
func resolveHelloID(fingerprint string) (utls.ClientHelloID, bool) {
	switch strings.ToLower(strings.TrimSpace(fingerprint)) {
	case "chrome":
		return utls.HelloChrome_Auto, true
	case "firefox":
		return utls.HelloFirefox_Auto, true
	case "safari":
		return utls.HelloSafari_Auto, true
	case "ios":
		return utls.HelloIOS_Auto, true
	default:
		return utls.ClientHelloID{}, false
	}
}

// utlsConnWrapper wraps a utls.UConn to satisfy http.Transport's
// HTTP/2 detection via ConnectionState() tls.ConnectionState.
type utlsConnWrapper struct {
	net.Conn
	uconn *utls.UConn
}

// ConnectionState converts utls.ConnectionState to crypto/tls.ConnectionState
// so that http.Transport can detect HTTP/2 via ALPN negotiation.
func (w *utlsConnWrapper) ConnectionState() tls.ConnectionState {
	cs := w.uconn.ConnectionState()
	return tls.ConnectionState{
		Version:            cs.Version,
		HandshakeComplete:  cs.HandshakeComplete,
		CipherSuite:        cs.CipherSuite,
		NegotiatedProtocol: cs.NegotiatedProtocol,
		ServerName:         cs.ServerName,
		PeerCertificates:   cs.PeerCertificates,
		VerifiedChains:     cs.VerifiedChains,
	}
}

// tunnelConn wraps a net.Conn that has buffered data in a bufio.Reader
// after CONNECT response parsing. Read drains the bufio buffer first,
// then delegates to the underlying connection.
type tunnelConn struct {
	net.Conn
	reader *bufio.Reader
}

// Read drains buffered data from the bufio.Reader first, then reads from
// the underlying connection. bufio.Reader.Read() handles this transparently.
func (tc *tunnelConn) Read(b []byte) (int, error) {
	return tc.reader.Read(b)
}

// createDialTLSContext returns a DialTLSContext function that uses utls
// to establish TLS connections with a browser-like ClientHello fingerprint.
func (pm *PoolManager) createDialTLSContext() func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := pm.createDialer()
	helloID, _ := resolveHelloID(pm.config.BrowserFingerprint)

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		var tcpConn net.Conn
		var err error

		if pm.proxyURL != nil {
			// Proxy + fingerprint path: dial proxy, send CONNECT, get tunnel.
			// The proxy address is in pm.proxyAddrs so SSRF is bypassed by the dialer.
			tcpConn, err = pm.dialAndConnectProxy(ctx, dialer, addr)
		} else {
			// Direct path: dial TCP using existing dialer (includes SSRF protection + connection tracking).
			tcpConn, err = dialer(ctx, network, addr)
		}
		if err != nil {
			return nil, err
		}

		// Extract hostname for SNI
		host, _, _ := net.SplitHostPort(addr)

		// Create utls connection with browser fingerprint.
		// Include MinVersion/MaxVersion so that post-handshake verification
		// stays consistent with the standard TLS path.
		//
		// Restrict ALPN to HTTP/1.1 only. Go's http.Transport detects HTTP/2
		// via the ConnectionState() method on the returned connection. If the
		// server negotiates "h2" via ALPN, it sends HTTP/2 frames that the
		// HTTP/1.1 transport cannot parse.
		//
		// NOTE: utls.Config.NextProtos is overwritten by the fingerprint's
		// built-in ALPNExtension during BuildHandshakeState. We must manually
		// replace the ALPN extension after building the handshake state.
		tlsConfig := &utls.Config{
			ServerName:         host,
			InsecureSkipVerify: pm.config.InsecureSkipVerify,
			MinVersion:         pm.config.MinTLSVersion,
			MaxVersion:         pm.config.MaxTLSVersion,
		}
		uconn := utls.UClient(tcpConn, tlsConfig, helloID)

		// Force ALPN to HTTP/1.1 only. The fingerprint's ALPN includes "h2"
		// which would cause HTTP/2 negotiation and break the HTTP/1.1 transport.
		if err := forceHTTP11ALPN(uconn); err != nil {
			uconn.Close()
			return nil, fmt.Errorf("utls ALPN config failed: %w", err)
		}

		// Enforce TLSHandshakeTimeout independently — when DialTLSContext
		// is set, http.Transport does NOT apply its TLSHandshakeTimeout.
		// Use the tighter of the existing context deadline and the configured timeout.
		hsCancel := enforceHandshakeTimeout(&ctx, pm.config.TLSHandshakeTimeout)
		if hsCancel != nil {
			defer hsCancel()
		}

		// Perform TLS handshake with context support
		if err := uconn.HandshakeContext(ctx); err != nil {
			uconn.Close()
			return nil, fmt.Errorf("utls handshake failed: %w", err)
		}

		return &utlsConnWrapper{Conn: uconn, uconn: uconn}, nil
	}
}

// dialAndConnectProxy dials the configured HTTP proxy, sends a CONNECT request
// for the target address, and returns the tunnel connection ready for TLS.
func (pm *PoolManager) dialAndConnectProxy(
	ctx context.Context,
	dialer func(context.Context, string, string) (net.Conn, error),
	targetAddr string,
) (net.Conn, error) {
	// Dial the proxy using the existing dialer (bypasses SSRF via proxyAddrs).
	proxyConn, err := dialer(ctx, "tcp", pm.proxyURL.Host)
	if err != nil {
		return nil, fmt.Errorf("proxy dial failed: %w", err)
	}

	// Build CONNECT request line and headers.
	var sb strings.Builder
	sb.WriteString("CONNECT ")
	sb.WriteString(targetAddr)
	sb.WriteString(" HTTP/1.1\r\nHost: ")
	sb.WriteString(targetAddr)
	sb.WriteString("\r\n")

	// Add proxy authentication if credentials are present in the proxy URL.
	if pm.proxyURL.User != nil {
		username := pm.proxyURL.User.Username()
		password, _ := pm.proxyURL.User.Password()
		if username != "" {
			sb.WriteString("Proxy-Authorization: ")
			sb.WriteString(basicAuthHeader(username, password))
			sb.WriteString("\r\n")
		}
	}
	sb.WriteString("\r\n")

	// Set deadline for the CONNECT phase using the configured dial timeout.
	if pm.config.DialTimeout > 0 {
		deadline := time.Now().Add(pm.config.DialTimeout)
		if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
		if err := proxyConn.SetDeadline(deadline); err != nil {
			proxyConn.Close()
			return nil, fmt.Errorf("proxy set deadline failed: %w", err)
		}
	}

	// Send CONNECT request.
	if _, err := proxyConn.Write([]byte(sb.String())); err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("CONNECT write failed: %w", err)
	}

	// Read CONNECT response manually to avoid http.ReadResponse body handling.
	// When ReadResponse receives a nil request, it cannot determine the method
	// and creates a response body that reads until EOF. Close() may then start
	// a background drain goroutine that races with the TLS handshake for data
	// from the shared bufio.Reader, causing the uTLS handshake to time out.
	br := bufio.NewReader(proxyConn)
	statusCode, statusText, err := readConnectResponse(br)
	if err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("CONNECT response read failed: %w", err)
	}

	if statusCode != http.StatusOK {
		proxyConn.Close()
		return nil, fmt.Errorf("proxy CONNECT failed: %d %s", statusCode, statusText)
	}

	// Clear deadline — the TLS handshake manages its own timeout via context.
	proxyConn.SetDeadline(time.Time{})

	// Wrap the connection to drain any data buffered in the bufio.Reader.
	// After parsing the CONNECT response headers, the bufio.Reader may have
	// already consumed bytes from the socket (early TLS ServerHello data).
	// tunnelConn.Read() delegates to bufio.Reader.Read(), which returns
	// buffered data first, then reads from the underlying socket.
	return &tunnelConn{Conn: proxyConn, reader: br}, nil
}

// basicAuthHeader returns a Basic authentication header value for proxy auth.
func basicAuthHeader(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}

// forceHTTP11ALPN overrides the utls fingerprint's ALPN extension to negotiate
// HTTP/1.1 only. Browser fingerprints include "h2" in their ALPN list, and
// ALPNExtension.writeToUConn() overwrites Config.NextProtos during
// BuildHandshakeState. This function replaces the extension and rebuilds the
// handshake state so the ClientHello advertises only "http/1.1".
func forceHTTP11ALPN(uconn *utls.UConn) error {
	if err := uconn.BuildHandshakeState(); err != nil {
		return fmt.Errorf("build handshake state: %w", err)
	}
	for i, ext := range uconn.Extensions {
		if _, ok := ext.(*utls.ALPNExtension); ok {
			uconn.Extensions[i] = &utls.ALPNExtension{
				AlpnProtocols: []string{"http/1.1"},
			}
			break
		}
	}
	// Rebuild so writeToUConn fires with the replacement ALPN values.
	if err := uconn.BuildHandshakeState(); err != nil {
		return fmt.Errorf("rebuild handshake state: %w", err)
	}
	return nil
}

// enforceHandshakeTimeout applies a TLS handshake timeout to the context if
// the configured timeout is shorter than any existing deadline.
// Returns a cancel function that must be deferred by the caller, or nil if
// no timeout was applied.
func enforceHandshakeTimeout(ctx *context.Context, timeout time.Duration) context.CancelFunc {
	if timeout <= 0 {
		return nil
	}
	if deadline, ok := (*ctx).Deadline(); ok && time.Until(deadline) <= timeout {
		return nil
	}
	var cancel context.CancelFunc
	*ctx, cancel = context.WithTimeout(*ctx, timeout)
	return cancel
}

// readConnectResponse reads the HTTP response to a CONNECT request directly,
// without using http.ReadResponse. This avoids creating a response body reader
// that could race with subsequent TLS handshake reads on the shared bufio.Reader.
func readConnectResponse(br *bufio.Reader) (statusCode int, statusText string, err error) {
	// Read status line: "HTTP/1.x 200 OK\r\n"
	line, err := br.ReadString('\n')
	if err != nil {
		return 0, "", fmt.Errorf("reading CONNECT status line: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")

	// Parse "HTTP/1.x CODE REASON"
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 {
		return 0, "", fmt.Errorf("malformed CONNECT response: %q", line)
	}
	statusCode, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, "", fmt.Errorf("parsing CONNECT status code from %q: %w", line, err)
	}
	if len(parts) >= 3 {
		statusText = parts[2]
	}

	// Read headers until empty line (end of headers).
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return 0, "", fmt.Errorf("reading CONNECT headers: %w", err)
		}
		if strings.TrimRight(line, "\r\n") == "" {
			break
		}
	}

	return statusCode, statusText, nil
}
