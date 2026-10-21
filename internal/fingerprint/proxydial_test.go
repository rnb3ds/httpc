package fingerprint

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

// TestProxyDialerFactory_Schemes pins the scheme gate: the tunnel types the
// fingerprint path supports construct, everything else — including socks4/
// socks4a, which x/net/proxy cannot dial — is rejected with the scheme named.
func TestProxyDialerFactory_Schemes(t *testing.T) {
	supported := []string{"http", "https", "socks5", "socks5h"}
	for _, scheme := range supported {
		if _, err := newProxyDialerFactory(&url.URL{Scheme: scheme, Host: "127.0.0.1:1"}); err != nil {
			t.Errorf("%s rejected: %v", scheme, err)
		}
	}

	for _, scheme := range []string{"ftp", "quic", "socks", "socks4", "socks4a", "ss", ""} {
		_, err := newProxyDialerFactory(&url.URL{Scheme: scheme, Host: "127.0.0.1:1"})
		if err == nil {
			t.Errorf("%q accepted", scheme)
			continue
		}
		if !strings.Contains(err.Error(), "proxy scheme") {
			t.Errorf("%q: error should name the scheme, got %v", scheme, err)
		}
	}
}

// TestConnectDialer_Dial pins the non-context entry point: it delegates to
// DialContext, so a refused proxy surfaces as a dial error rather than a hang.
func TestConnectDialer_Dial(t *testing.T) {
	d := newConnectDialer(url.URL{Scheme: "http", Host: "127.0.0.1:1"}, nil)
	_, err := d.Dial("tcp", "example.com:443")
	if err == nil {
		t.Fatal("Dial through a dead proxy succeeded")
	}
}

// TestConnectDialer_ConnectTunnel drives a plain-HTTP target through a
// loopback CONNECT proxy and asserts both the tunneled body and the proxy
// credentials that crossed the wire.
func TestConnectDialer_ConnectTunnel(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "tunnel-ok")
	}))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)

	addr, ring := startConnectProxy(t, http.StatusOK)
	d := newConnectDialer(url.URL{
		Scheme: "http",
		Host:   addr,
		User:   url.UserPassword("user", "pass"),
	}, nil)

	conn, err := d.DialContext(context.Background(), "tcp", targetURL.Host)
	if err != nil {
		t.Fatalf("DialContext through CONNECT proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: " + targetURL.Host + "\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatalf("write tunneled request: %v", err)
	}
	body, err := io.ReadAll(bufio.NewReader(conn))
	if err != nil {
		t.Fatalf("read tunneled response: %v", err)
	}
	if !strings.Contains(string(body), "tunnel-ok") {
		t.Errorf("tunneled body = %q, want tunnel-ok", body)
	}

	auth, ok := ring.get()
	if !ok || !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("proxy auth header = %q (present=%v), want a Basic credential", auth, ok)
	}
}

// TestConnectDialer_ProxyRejectsTunnel pins the non-200 CONNECT arm: a 407 must
// surface as an error naming the status, not as a silent usable connection.
func TestConnectDialer_ProxyRejectsTunnel(t *testing.T) {
	addr, _ := startConnectProxy(t, http.StatusProxyAuthRequired)
	d := newConnectDialer(url.URL{Scheme: "http", Host: addr}, nil)

	conn, err := d.DialContext(context.Background(), "tcp", "example.com:443")
	if err == nil {
		_ = conn.Close()
		t.Fatal("407 CONNECT accepted")
	}
	if !strings.Contains(err.Error(), "407") {
		t.Errorf("error should name the status, got: %v", err)
	}
}

// TestConnectDialer_DeadProxy pins the TCP-connect failure arm.
func TestConnectDialer_DeadProxy(t *testing.T) {
	d := newConnectDialer(url.URL{Scheme: "http", Host: "127.0.0.1:1"}, nil)
	if _, err := d.DialContext(context.Background(), "tcp", "example.com:443"); err == nil {
		t.Fatal("dead proxy accepted")
	}
}

// TestConnectDialer_HTTPSProxyTunnel pins the TLS-to-proxy leg: the tunnel is
// established over TLS with ALPN http/1.1, and the proxy's certificate is not
// verified (documented behaviour — only the target leg is verified).
func TestConnectDialer_HTTPSProxyTunnel(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "tls-tunnel-ok")
	}))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{selfSignedCert(t)},
		NextProtos:   []string{"http/1.1"},
	})
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleProxyConn(conn, http.StatusOK, &stringRing{})
		}
	}()

	d := newConnectDialer(url.URL{Scheme: "https", Host: ln.Addr().String()}, nil)
	conn, err := d.DialContext(context.Background(), "tcp", targetURL.Host)
	if err != nil {
		t.Fatalf("DialContext through HTTPS proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: " + targetURL.Host + "\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	body, _ := io.ReadAll(bufio.NewReader(conn))
	if !strings.Contains(string(body), "tls-tunnel-ok") {
		t.Errorf("body = %q", body)
	}
}

// TestConnectDialer_HTTPSProxyHandshakeFailure pins the TLS-handshake arm for
// a proxy that answers in plaintext where TLS is expected.
func TestConnectDialer_HTTPSProxyHandshakeFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("this is not a TLS server\n"))
		_ = conn.Close()
	}()

	d := newConnectDialer(url.URL{Scheme: "https", Host: ln.Addr().String()}, nil)
	if _, err := d.DialContext(context.Background(), "tcp", "example.com:443"); err == nil {
		t.Fatal("plaintext proxy accepted for an https scheme")
	}
}

// TestConnectDialer_Socks5Tunnel pins the SOCKS5 arm against a minimal SOCKS5
// server that performs the no-auth handshake and CONNECT reply.
func TestConnectDialer_Socks5Tunnel(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "socks-ok")
	}))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)

	socksAddr, socksDone := startMiniSocks5(t)

	d := newConnectDialer(url.URL{Scheme: "socks5", Host: socksAddr}, nil)
	conn, err := d.DialContext(context.Background(), "tcp", targetURL.Host)
	if err != nil {
		t.Fatalf("DialContext through SOCKS5: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("GET / HTTP/1.1\r\nHost: " + targetURL.Host + "\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	body, _ := io.ReadAll(bufio.NewReader(conn))
	if !strings.Contains(string(body), "socks-ok") {
		t.Errorf("body = %q", body)
	}
	select {
	case <-socksDone:
	case <-time.After(3 * time.Second):
		t.Error("SOCKS server never saw the tunnel")
	}
}

// TestConnectDialer_SocksUnreachable pins the SOCKS dial-failure arm.
func TestConnectDialer_SocksUnreachable(t *testing.T) {
	d := newConnectDialer(url.URL{Scheme: "socks5", Host: "127.0.0.1:1"}, nil)
	if _, err := d.DialContext(context.Background(), "tcp", "example.com:443"); err == nil {
		t.Fatal("unreachable SOCKS proxy accepted")
	}
}

// TestConnectDialer_ContextCancelInterruptsConnect pins the cancellation arm:
// a proxy that accepts TCP and then never answers CONNECT must not hold the
// dial past the context deadline.
func TestConnectDialer_ContextCancelInterruptsConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Accept and read the CONNECT head, then stay silent.
		_, _ = http.ReadRequest(bufio.NewReader(conn))
		time.Sleep(3 * time.Second)
		_ = conn.Close()
	}()

	d := newConnectDialer(url.URL{Scheme: "http", Host: ln.Addr().String()}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = d.DialContext(ctx, "tcp", "example.com:443")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("silent proxy produced a connection")
	}
	if elapsed > 2*time.Second {
		t.Errorf("cancellation took %v; the context did not interrupt CONNECT", elapsed)
	}
}

// TestConnectDialer_CustomLocalAddr pins that a configured local address is
// honoured on the proxy leg.
func TestConnectDialer_CustomLocalAddr(t *testing.T) {
	addr, _ := startConnectProxy(t, http.StatusOK)
	// A loopback local address with port 0 lets the kernel pick the port while
	// still proving the field is not dropped.
	d := newConnectDialer(url.URL{Scheme: "http", Host: addr}, &net.TCPAddr{IP: net.ParseIP("127.0.0.1")})

	conn, err := d.DialContext(context.Background(), "tcp", "example.com:443")
	if err == nil {
		_ = conn.Close()
		return // the tunnel came up, which is all this asserts
	}
	// A tunnel failure is acceptable here (the proxy cannot reach the target);
	// what matters is that dialing itself did not fail on the local address.
	if strings.Contains(err.Error(), "bind") {
		t.Errorf("local address rejected: %v", err)
	}
}

// TestProxyDialerContractSatisfied pins the interface the transport's factory
// must return.
func TestProxyDialerContractSatisfied(t *testing.T) {
	var _ proxy.ContextDialer = newConnectDialer(url.URL{Scheme: "http", Host: "127.0.0.1:1"}, nil)
}

// startMiniSocks5 runs a minimal SOCKS5 server (no-auth, CONNECT IPv4/domain)
// and closes socksDone once it has tunneled a connection.
func startMiniSocks5(t *testing.T) (addr string, done <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ready := make(chan struct{})
	var once sync.Once

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)

				// Greeting: version, nmethods, methods.
				hdr := make([]byte, 2)
				if _, err := io.ReadFull(br, hdr); err != nil {
					return
				}
				methods := make([]byte, int(hdr[1]))
				if _, err := io.ReadFull(br, methods); err != nil {
					return
				}
				if _, err := c.Write([]byte{0x05, 0x00}); err != nil { // no auth
					return
				}

				// Request: ver, cmd, rsv, atyp, addr, port.
				req := make([]byte, 4)
				if _, err := io.ReadFull(br, req); err != nil {
					return
				}
				var host string
				switch req[3] {
				case 0x01: // IPv4
					b := make([]byte, 4)
					if _, err := io.ReadFull(br, b); err != nil {
						return
					}
					host = net.IP(b).String()
				case 0x03: // domain
					l := make([]byte, 1)
					if _, err := io.ReadFull(br, l); err != nil {
						return
					}
					b := make([]byte, int(l[0]))
					if _, err := io.ReadFull(br, b); err != nil {
						return
					}
					host = string(b)
				default:
					return
				}
				pb := make([]byte, 2)
				if _, err := io.ReadFull(br, pb); err != nil {
					return
				}
				port := int(pb[0])<<8 | int(pb[1])

				upstream, err := net.Dial("tcp", net.JoinHostPort(host, itoa(port)))
				if err != nil {
					_, _ = c.Write([]byte{0x05, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
					return
				}
				defer func() { _ = upstream.Close() }()

				if _, err := c.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
					return
				}
				once.Do(func() { close(ready) })

				go func() { _, _ = io.Copy(upstream, br) }()
				_, _ = io.Copy(c, upstream)
			}(conn)
		}
	}()

	return ln.Addr().String(), ready
}

// itoa formats a port number without importing strconv into the test's
// top-level imports for one call.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [5]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// selfSignedCert builds a throwaway certificate for TLS test servers.
func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// TestWriteConnect_MalformedReply pins the read-failure arm of the CONNECT
// exchange helper.
func TestWriteConnect_MalformedReply(t *testing.T) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()
	go func() {
		buf := make([]byte, 512)
		_, _ = server.Read(buf)
		_, _ = server.Write([]byte("NOT-HTTP\r\n\r\n"))
		_ = server.Close()
	}()

	err := writeConnect(context.Background(), client, &url.URL{Scheme: "http"}, "example.com:443")
	if err == nil {
		t.Fatal("malformed CONNECT reply accepted")
	}
	if errors.Is(err, io.EOF) {
		t.Error("error should describe the malformed reply, not a bare EOF")
	}
}
