// Proxy dialing for the fingerprint transport.
//
// The transport accepts a custom dialer factory, which is what this package
// uses instead of its built-in proxy support. Two reasons:
//
//   - The built-in CONNECT path derives its socket deadline from the client
//     timeout, so a client created with WithTimeoutSeconds(0) (which is what
//     this package asks for, because the engine owns the deadline budget) sets
//     a deadline of "now" and every proxied request dies with an i/o timeout.
//   - Deadlines here come from the request context, matching the rest of
//     httpc: cancellation interrupts a stuck CONNECT, and a successful tunnel
//     hands the connection over with no deadline left on it.
//
// Supported schemes: http and https (CONNECT; https negotiates http/1.1 for
// the tunnel leg, and that leg's certificate is not verified — only the
// target's handshake is fingerprinted and verified on this path), plus
// socks4/4a and socks5/5h through golang.org/x/net/proxy.

package fingerprint

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	ehttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"golang.org/x/net/proxy"
)

// proxyDialTimeout bounds the TCP connect to the proxy when the context
// carries no deadline of its own.
const proxyDialTimeout = 30 * time.Second

// newProxyDialerFactory returns a factory that dials every target through
// proxyURL. The URL is fixed per backend: the httpc proxy pool rotates by
// selecting a different backend, never by re-pointing a live one.
func newProxyDialerFactory(proxyURL *url.URL) (tls_client.ProxyDialerFactory, error) {
	if err := validateProxyScheme(proxyURL.Scheme); err != nil {
		return nil, err
	}
	target := *proxyURL

	return func(_ string, _ time.Duration, localAddr *net.TCPAddr, _ ehttp.Header, _ tls_client.Logger) (proxy.ContextDialer, error) {
		// The proxy URL and the timeout are fixed per backend: the transport's
		// arguments restate them, and the deadline actually honoured comes
		// from the request context.
		return newConnectDialer(target, localAddr), nil
	}, nil
}

// validateProxyScheme rejects schemes the fingerprint path cannot tunnel.
// socks4/socks4a are deliberately absent: the SOCKS branch dials through
// x/net/proxy.FromURL, which implements socks5/socks5h only — accepting them
// here would construct cleanly and then fail on every dial with
// "proxy: unknown scheme".
func validateProxyScheme(scheme string) error {
	switch scheme {
	case "http", "https", "socks5", "socks5h":
		return nil
	default:
		return fmt.Errorf("fingerprint: unsupported proxy scheme %q", scheme)
	}
}

// connectDialer dials every target through one fixed proxy.
type connectDialer struct {
	proxy     url.URL
	localAddr *net.TCPAddr
	tcp       *net.Dialer
	socks     func() (proxy.Dialer, error) // lazily built; nil for CONNECT proxies
}

// newConnectDialer builds the dialer for one proxy URL.
func newConnectDialer(proxyURL url.URL, localAddr *net.TCPAddr) *connectDialer {
	d := &connectDialer{
		proxy:     proxyURL,
		localAddr: localAddr,
		tcp:       &net.Dialer{Timeout: proxyDialTimeout, LocalAddr: localAddr},
	}
	if strings.HasPrefix(proxyURL.Scheme, "socks") {
		d.socks = func() (proxy.Dialer, error) { return proxy.FromURL(&proxyURL, d.tcp) }
	}
	return d
}

// Dial implements the non-context half of the dialer contract.
func (d *connectDialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

// DialContext establishes the tunnel to addr: SOCKS handshake for socks
// schemes, CONNECT for http/https.
func (d *connectDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if d.socks != nil {
		return d.dialSOCKS(ctx, network, addr)
	}
	return d.dialConnect(ctx, network, addr)
}

// dialSOCKS tunnels through a SOCKS proxy. The dialer is built per dial (no
// shared mutable state) and prefers the context-aware path so cancellation
// interrupts a stalled forward dial.
func (d *connectDialer) dialSOCKS(ctx context.Context, network, addr string) (net.Conn, error) {
	base, err := d.socks()
	if err != nil {
		return nil, fmt.Errorf("fingerprint: socks proxy dialer: %w", err)
	}
	cd, ok := base.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("fingerprint: socks proxy dialer does not support contexts")
	}
	return cd.DialContext(ctx, network, addr)
}

// dialConnect opens a CONNECT tunnel: TCP (plus TLS for an https proxy) to the
// proxy, the CONNECT exchange, then the raw tunnel with no deadline attached.
func (d *connectDialer) dialConnect(ctx context.Context, network, addr string) (net.Conn, error) {
	host := d.proxy.Hostname()
	port := d.proxy.Port()
	if port == "" {
		if d.proxy.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	conn, err := d.tcp.DialContext(ctx, network, net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if d.proxy.Scheme == "https" {
		tlsConn := tls.Client(conn, &tls.Config{
			NextProtos: []string{"http/1.1"},
			ServerName: host,
			// The proxy leg is not the fingerprinting target: only the
			// target's handshake is verified on this path. Documented in the
			// README's proxy notes.
			InsecureSkipVerify: true, //nolint:gosec // see comment above
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("fingerprint: TLS to proxy: %w", err)
		}
		conn = tlsConn
	}

	if err := writeConnect(ctx, conn, &d.proxy, addr); err != nil {
		_ = conn.Close()
		return nil, err
	}

	// The tunnel is up; the engine's request context governs everything above
	// this layer, so no deadline is left on the socket.
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

// writeConnect performs the CONNECT exchange and requires a 200 response. The
// buffered reader used for the proxy's reply may buffer past it, but a proxy
// sends nothing after the 200 until the client speaks first, so discarding it
// is safe.
func writeConnect(ctx context.Context, conn net.Conn, proxyURL *url.URL, addr string) error {
	req := (&http.Request{
		Method:     http.MethodConnect,
		URL:        &url.URL{Host: addr},
		Host:       addr,
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
	}).WithContext(ctx)

	if user := proxyURL.User; user != nil && user.Username() != "" {
		password, _ := user.Password()
		creds := user.Username() + ":" + password
		req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
	}

	// A canceled context must interrupt a stuck CONNECT by expiring the
	// socket deadline. RACE (accepted): if the exchange finishes between the
	// watchdog's non-blocking done check and its SetDeadline, the expired
	// deadline can land on a connection dialConnect has already cleared and
	// returned; that window only opens when the caller's context is already
	// canceled, so the request is being abandoned anyway.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			select {
			case <-done:
				return // the exchange finished; the caller owns the conn now
			default:
			}
			_ = conn.SetDeadline(time.Now().Add(-time.Second))
		case <-done:
		}
	}()

	if err := req.Write(conn); err != nil {
		return err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fingerprint: proxy response status: %s", resp.Status)
	}
	return nil
}
