// Proxy-aware raw dialing for the fingerprint transport.
//
// Proxying rides on DialContext replacement (transport.Proxy stays nil): the
// uTLS handshake must wrap the already-tunneled connection, so the tunnel
// has to exist at the raw-dial layer. HTTP targets therefore traverse an
// HTTP proxy as a CONNECT tunnel rather than absolute-form forwarded
// requests — the same behavior the previous surf-based backend had.
//
// Simplified port of github.com/enetx/surf (pkg/connectproxy), Copyright
// enetx/surf authors, MIT License: http/https CONNECT and socks5/socks5h
// are supported. Deliberate deltas from upstream, all documented in the
// README's known-edges section:
//   - HTTPS proxies are dialed with ALPN restricted to http/1.1 (upstream
//     could tunnel CONNECT over h2), and the proxy leg's certificate is not
//     verified (unchanged from upstream; only the target's handshake is
//     fingerprinted and verified here).
//   - The proxy leg uses a regular 10s-timeout dialer (upstream embedded a
//     zero-value net.Dialer, leaving proxy dials without a timeout).
//   - socks4/4a and custom DNS plumbing are not carried over.

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

	"golang.org/x/net/proxy"
)

// newProxyDialer returns a raw dial function that tunnels through the given
// proxy: CONNECT for http/https proxies, SOCKS5 for socks5/socks5h (via
// golang.org/x/net/proxy, fresh per dial — no shared mutable state).
func newProxyDialer(proxyURL *url.URL) (func(context.Context, string, string) (net.Conn, error), error) {
	var defaultPort string
	switch proxyURL.Scheme {
	case "http":
		defaultPort = "80"
	case "https":
		defaultPort = "443"
	case "socks5", "socks5h":
		defaultPort = "1080"
	default:
		return nil, fmt.Errorf("fingerprint: unsupported proxy scheme %q", proxyURL.Scheme)
	}
	if proxyURL.Host == "" {
		return nil, fmt.Errorf("fingerprint: proxy URL has no host")
	}

	host := proxyURL.Hostname()
	port := proxyURL.Port()
	if port == "" {
		port = defaultPort
	}
	proxyAddr := net.JoinHostPort(host, port)

	// Basic proxy authentication from the proxy URL's userinfo.
	proxyAuth := ""
	if user := proxyURL.User; user != nil && user.Username() != "" {
		password, _ := user.Password()
		creds := user.Username() + ":" + password
		proxyAuth = "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
	}

	dialer := &net.Dialer{Timeout: fpDialTimeout, KeepAlive: fpTCPKeepAlive}
	isSocks := strings.HasPrefix(proxyURL.Scheme, "socks")
	scheme := proxyURL.Scheme

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if isSocks {
			socksURL := &url.URL{Scheme: scheme, Host: proxyAddr, User: proxyURL.User}
			d, err := proxy.FromURL(socksURL, &ctxDialer{dialer: dialer})
			if err != nil {
				return nil, fmt.Errorf("fingerprint: socks proxy dialer: %w", err)
			}
			return d.(proxy.ContextDialer).DialContext(ctx, network, addr)
		}

		var conn net.Conn
		if scheme == "https" {
			tcpConn, err := dialer.DialContext(ctx, network, proxyAddr)
			if err != nil {
				return nil, err
			}
			tlsConn := tls.Client(tcpConn, &tls.Config{
				NextProtos:         []string{"http/1.1"},
				ServerName:         host,
				InsecureSkipVerify: true, // proxy leg; the target leg is verified in tlsHandshake
			})
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = tcpConn.Close()
				return nil, fmt.Errorf("fingerprint: TLS to proxy: %w", err)
			}
			conn = tlsConn
		} else {
			var err error
			conn, err = dialer.DialContext(ctx, network, proxyAddr)
			if err != nil {
				return nil, err
			}
		}

		req := (&http.Request{
			Method: http.MethodConnect,
			URL:    &url.URL{Host: addr},
			Host:   addr,
			Header: make(http.Header),
		}).WithContext(ctx)
		if proxyAuth != "" {
			req.Header.Set("Proxy-Authorization", proxyAuth)
		}

		if err := proxyConnect(req, conn); err != nil {
			return nil, err
		}
		return conn, nil
	}, nil
}

// proxyConnect writes the CONNECT request and requires a 200 response. The
// buffered reader used for the proxy's reply may buffer past it, but a proxy
// sends nothing after the 200 until the client speaks first, so discarding
// it is safe.
func proxyConnect(req *http.Request, conn net.Conn) error {
	req.Proto = "HTTP/1.1"
	req.ProtoMajor = 1
	req.ProtoMinor = 1

	ctx := req.Context()

	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			// A cancel racing completion must not poison the handed-back
			// conn: if the exchange already finished, the caller (TLS
			// handshake) owns the conn now — leave the deadline alone.
			select {
			case <-done:
				return
			default:
			}
			// Interrupt a stuck CONNECT exchange. The negative deadline
			// persists, but every path that observes it returns an error and
			// closes the conn.
			_ = conn.SetDeadline(time.Now().Add(-time.Second))
		case <-done:
			// Success: touch nothing. The conn now belongs to the caller
			// (TLS handshake next); clearing the deadline from this goroutine
			// could erase a deadline the handshake set at the instant ctx
			// expires — the watcher may be scheduled only after proxyConnect
			// has already returned.
		}
	}()

	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		_ = conn.Close()
		return err
	}

	if resp.StatusCode != http.StatusOK {
		_ = conn.Close()
		return fmt.Errorf("fingerprint: proxy response status: %s", resp.Status)
	}

	return nil
}

// ctxDialer adapts *net.Dialer to proxy.Dialer for the SOCKS5 forward leg.
// It implements proxy.ContextDialer: x/net/proxy's socks5 dialer prefers a
// ContextDialer, so a plain Dial that forwarded context.Background (as here
// previously) left request cancellation unable to interrupt the forward
// dial — DialContext returned ctx.Err() immediately while the TCP dial ran
// on in the background until its timeout.
type ctxDialer struct {
	dialer *net.Dialer
}

var _ proxy.ContextDialer = (*ctxDialer)(nil)

func (d *ctxDialer) Dial(network, addr string) (net.Conn, error) {
	return d.dialer.DialContext(context.Background(), network, addr)
}

func (d *ctxDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.dialer.DialContext(ctx, network, addr)
}
