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
	"fmt"
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
)

// startConnectProxy runs a minimal CONNECT proxy: it answers CONNECT with
// status (200 or 407), then pipes bytes bidirectionally. It records the raw
// CONNECT request head for assertions.
func startConnectProxy(t *testing.T, status int) (addr string, lastRequest *stringRing) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	ring := &stringRing{}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleProxyConn(conn, status, ring)
		}
	}()

	return ln.Addr().String(), ring
}

func handleProxyConn(conn net.Conn, status int, ring *stringRing) {
	defer func() { _ = conn.Close() }()

	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil || req.Method != http.MethodConnect {
		return
	}
	ring.put(req.Header.Get("Proxy-Authorization"))

	if status != http.StatusOK {
		_, _ = conn.Write([]byte("HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n"))
		return
	}

	target, err := net.Dial("tcp", req.URL.Host)
	if err != nil {
		_, _ = conn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n"))
		return
	}
	defer func() { _ = target.Close() }()

	_, _ = conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	go func() {
		_, _ = io.Copy(target, br) // client → target (br holds buffered bytes)
		_ = target.Close()
	}()
	_, _ = io.Copy(conn, target) // target → client
}

// stringRing is a tiny single-slot synchronized string holder.
type stringRing struct {
	mu   sync.Mutex
	val  string
	full bool
}

func (r *stringRing) put(v string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.val, r.full = v, true
}

func (r *stringRing) get() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.val, r.full
}

// TestProxyDialer_Connect pins the http-proxy path: CONNECT reaches the
// proxy with the userinfo-derived Proxy-Authorization, and the returned
// connection carries a full HTTP exchange with the target.
func TestProxyDialer_Connect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "via-proxy")
	}))
	defer target.Close()

	proxyAddr, ring := startConnectProxy(t, http.StatusOK)

	u := &url.URL{
		Scheme: "http",
		User:   url.UserPassword("user", "pass"),
		Host:   proxyAddr,
	}
	dial, err := newProxyDialer(u)
	if err != nil {
		t.Fatalf("newProxyDialer: %v", err)
	}

	targetURL, _ := url.Parse(target.URL)
	conn, err := dial(context.Background(), "tcp", targetURL.Host)
	if err != nil {
		t.Fatalf("dial through proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(5 * time.Second)
	_ = conn.SetDeadline(deadline)
	req, _ := http.NewRequest(http.MethodGet, "http://target/", nil)
	req.Host = targetURL.Host
	if err := req.Write(conn); err != nil {
		t.Fatalf("write request through tunnel: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		t.Fatalf("read response through tunnel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "via-proxy" {
		t.Fatalf("body through tunnel = %q", body)
	}

	if auth, ok := ring.get(); !ok || !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("Proxy-Authorization = %q (seen=%v), want Basic credentials", auth, ok)
	}
}

// TestProxyDialer_AuthFailure pins that a proxy's non-200 CONNECT response
// fails the dial with the proxy status.
func TestProxyDialer_AuthFailure(t *testing.T) {
	proxyAddr, _ := startConnectProxy(t, http.StatusProxyAuthRequired)

	dial, err := newProxyDialer(&url.URL{Scheme: "http", Host: proxyAddr})
	if err != nil {
		t.Fatalf("newProxyDialer: %v", err)
	}

	_, err = dial(context.Background(), "tcp", "example.com:443")
	if err == nil {
		t.Fatal("dial succeeded through a 407 proxy")
	}
	if !strings.Contains(err.Error(), "proxy response status") {
		t.Errorf("error lacks proxy status context: %v", err)
	}
}

// TestProxyDialer_UnsupportedScheme pins the validation errors.
func TestProxyDialer_UnsupportedScheme(t *testing.T) {
	for _, scheme := range []string{"ftp", "socks4", ""} {
		u := &url.URL{Scheme: scheme, Host: "127.0.0.1:1"}
		if _, err := newProxyDialer(u); err == nil {
			t.Errorf("scheme %q accepted", scheme)
		}
	}
	if _, err := newProxyDialer(&url.URL{Scheme: "http"}); err == nil {
		t.Error("empty host accepted")
	}
}

// TestProxyDialer_PortlessURLsAccepted pins the default-port arms: portless
// proxy URLs of every supported scheme construct a dialer (the default port
// is filled in at dial time).
func TestProxyDialer_PortlessURLsAccepted(t *testing.T) {
	for _, scheme := range []string{"http", "https", "socks5", "socks5h"} {
		u := &url.URL{Scheme: scheme, Host: "proxy.example.com"}
		if _, err := newProxyDialer(u); err != nil {
			t.Errorf("portless %q rejected: %v", scheme, err)
		}
	}
}

// TestProxyDialer_HTTPSProxyHandshakeFailure pins the https-proxy TLS branch:
// an https proxy URL pointed at a plaintext listener fails the TLS handshake
// with the "TLS to proxy" context (the target leg is never attempted).
func TestProxyDialer_HTTPSProxyHandshakeFailure(t *testing.T) {
	// A plain TCP listener that never speaks TLS: accept, swallow the
	// ClientHello, close — the client handshake fails on the early EOF.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _ = io.Copy(io.Discard, conn)
			}()
		}
	}()

	dial, err := newProxyDialer(&url.URL{Scheme: "https", Host: ln.Addr().String()})
	if err != nil {
		t.Fatalf("newProxyDialer: %v", err)
	}

	_, err = dial(context.Background(), "tcp", "example.com:443")
	if err == nil {
		t.Fatal("https dial through a plaintext proxy should fail")
	}
	if !strings.Contains(err.Error(), "TLS to proxy") {
		t.Errorf("error lacks TLS-to-proxy context: %v", err)
	}
}

// TestProxyDialer_HTTPSProxyUnreachable pins the https dial arm: an https
// proxy at an unreachable address fails at the TCP dial (before any TLS).
func TestProxyDialer_HTTPSProxyUnreachable(t *testing.T) {
	dead := closedLoopbackAddr(t)

	dial, err := newProxyDialer(&url.URL{Scheme: "https", Host: dead})
	if err != nil {
		t.Fatalf("newProxyDialer: %v", err)
	}
	if _, err := dial(context.Background(), "tcp", "example.com:443"); err == nil {
		t.Fatal("dial to a dead https proxy should fail")
	}
}

// TestProxyDialer_HTTPSProxyConnect pins the https-proxy success path: the
// proxy leg negotiates TLS (its certificate is deliberately not verified —
// only the target leg is), then CONNECT tunnels a full HTTP exchange.
func TestProxyDialer_HTTPSProxyConnect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "https-proxy-ok")
	}))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)

	cert := selfSignedCert(t)
	tlsLn, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
	})
	if err != nil {
		t.Fatalf("tls listen: %v", err)
	}
	t.Cleanup(func() { _ = tlsLn.Close() })
	go func() {
		for {
			conn, err := tlsLn.Accept()
			if err != nil {
				return
			}
			go handleProxyConn(conn, http.StatusOK, &stringRing{})
		}
	}()

	dial, err := newProxyDialer(&url.URL{Scheme: "https", Host: tlsLn.Addr().String()})
	if err != nil {
		t.Fatalf("newProxyDialer: %v", err)
	}

	conn, err := dial(context.Background(), "tcp", targetURL.Host)
	if err != nil {
		t.Fatalf("dial through https proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	req, _ := http.NewRequest(http.MethodGet, "http://target/", nil)
	req.Host = targetURL.Host
	if err := req.Write(conn); err != nil {
		t.Fatalf("write request through tunnel: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		t.Fatalf("read response through tunnel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "https-proxy-ok" {
		t.Fatalf("body through https proxy = %q", body)
	}
}

// selfSignedCert generates a throwaway self-signed certificate for the TLS
// proxy listener (the proxy leg runs with verification disabled, so the
// certificate's contents do not matter).
func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<60))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "fingerprint-test-proxy"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// TestProxyDialer_Socks5Tunnel pins the socks5 branch end to end offline: a
// minimal loopback SOCKS5 server accepts the handshake, connects to the
// loopback target, and a full HTTP exchange rides the tunnel. This also
// exercises ctxDialer.DialContext as the socks dialer's forward leg.
func TestProxyDialer_Socks5Tunnel(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "socks-ok")
	}))
	defer target.Close()
	targetURL, _ := url.Parse(target.URL)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go serveSOCKS5(t, ln)

	dial, err := newProxyDialer(&url.URL{Scheme: "socks5", Host: ln.Addr().String()})
	if err != nil {
		t.Fatalf("newProxyDialer(socks5): %v", err)
	}

	conn, err := dial(context.Background(), "tcp", targetURL.Host)
	if err != nil {
		t.Fatalf("dial through socks5 proxy: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	req, _ := http.NewRequest(http.MethodGet, "http://target/", nil)
	req.Host = targetURL.Host
	if err := req.Write(conn); err != nil {
		t.Fatalf("write request through socks tunnel: %v", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		t.Fatalf("read response through socks tunnel: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "socks-ok" {
		t.Fatalf("body through socks tunnel = %q", body)
	}
}

// serveSOCKS5 implements the minimal no-auth SOCKS5 CONNECT server used by
// TestProxyDialer_Socks5Tunnel: greeting → method reply → CONNECT request →
// success reply → bidirectional pipe.
func serveSOCKS5(t *testing.T, ln net.Listener) {
	t.Helper()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleSOCKS5Conn(conn)
	}
}

func handleSOCKS5Conn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	// Client greeting: VER NMETHODS METHODS...
	head := make([]byte, 2)
	if _, err := io.ReadFull(conn, head); err != nil {
		return
	}
	methods := make([]byte, int(head[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	// Reply: no auth required.
	if _, err := conn.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Request: VER CMD RSV ATYP ADDR PORT.
	req := make([]byte, 4)
	if _, err := io.ReadFull(conn, req); err != nil {
		return
	}
	if req[1] != 0x01 { // CONNECT
		return
	}
	var host string
	switch req[3] {
	case 0x01: // IPv4
		b := make([]byte, 4)
		if _, err := io.ReadFull(conn, b); err != nil {
			return
		}
		host = net.IP(b).String()
	case 0x03: // domain
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return
		}
		b := make([]byte, int(l[0]))
		if _, err := io.ReadFull(conn, b); err != nil {
			return
		}
		host = string(b)
	case 0x04: // IPv6
		b := make([]byte, 16)
		if _, err := io.ReadFull(conn, b); err != nil {
			return
		}
		host = net.IP(b).String()
	default:
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(conn, portBytes); err != nil {
		return
	}
	port := int(portBytes[0])<<8 | int(portBytes[1])

	target, err := net.Dial("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)))
	if err != nil {
		return // closing the conn fails the client dial
	}
	defer func() { _ = target.Close() }()

	// Success reply: VER REP RSV ATYP BND.ADDR BND.PORT.
	if _, err := conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(target, conn)
		_ = target.Close()
		close(done)
	}()
	_, _ = io.Copy(conn, target)
	<-done
}

// TestProxyDialer_Socks5_UnreachableProxy pins the socks forward-dial error
// propagation: an unreachable socks proxy surfaces the dial error.
func TestProxyDialer_Socks5_UnreachableProxy(t *testing.T) {
	dead := closedLoopbackAddr(t)

	dial, err := newProxyDialer(&url.URL{Scheme: "socks5h", Host: dead})
	if err != nil {
		t.Fatalf("newProxyDialer(socks5h): %v", err)
	}
	if _, err := dial(context.Background(), "tcp", "example.com:443"); err == nil {
		t.Fatal("dial through an unreachable socks proxy should fail")
	}
}

// TestCtxDialer pins the SOCKS forward-leg adapter directly: plain Dial dials
// loopback with a background context, DialContext honors cancellation.
func TestCtxDialer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			func() { defer func() { _ = conn.Close() }(); _, _ = io.Copy(io.Discard, conn) }()
		}
	}()

	d := &ctxDialer{dialer: &net.Dialer{Timeout: 2 * time.Second}}

	t.Run("Dial reaches loopback", func(t *testing.T) {
		conn, err := d.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("Dial: %v", err)
		}
		_ = conn.Close()
	})

	t.Run("DialContext reaches loopback", func(t *testing.T) {
		conn, err := d.DialContext(context.Background(), "tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("DialContext: %v", err)
		}
		_ = conn.Close()
	})

	t.Run("DialContext honors cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := d.DialContext(ctx, "tcp", ln.Addr().String()); !errors.Is(err, context.Canceled) {
			t.Fatalf("DialContext with canceled ctx = %v, want context.Canceled", err)
		}
	})
}

// errConn is a net.Conn whose Write always fails, to drive proxyConnect's
// request-write error path deterministically.
type errConn struct {
	net.Conn // embedded no-op base for the unused methods
	writeErr error
}

func (c *errConn) Write([]byte) (int, error) { return 0, c.writeErr }

// TestProxyConnect_WriteFailure pins the write-error path: a conn that
// cannot accept the CONNECT request fails proxyConnect with that error.
func TestProxyConnect_WriteFailure(t *testing.T) {
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	defer func() { _ = c2.Close() }()

	req, _ := http.NewRequest(http.MethodConnect, "http://example.com:443", nil)
	req = req.WithContext(context.Background())

	writeErr := errors.New("pipe burst")
	if err := proxyConnect(req, &errConn{Conn: c1, writeErr: writeErr}); !errors.Is(err, writeErr) {
		t.Fatalf("proxyConnect write failure = %v, want %v", err, writeErr)
	}
}

// TestProxyConnect_MalformedReply pins the read path: a proxy reply that is
// not an HTTP response fails proxyConnect.
func TestProxyConnect_MalformedReply(t *testing.T) {
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
		// Read just the CONNECT request head (never to EOF — the client
		// keeps the conn open awaiting the reply), then answer with garbage.
		br := bufio.NewReader(conn)
		for {
			line, err := br.ReadString('\n')
			if err != nil || strings.TrimSpace(line) == "" {
				break // request head complete (or read failed)
			}
		}
		_, _ = conn.Write([]byte("this is not http\r\n\r\n"))
		_ = conn.Close()
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	req, _ := http.NewRequest(http.MethodConnect, "http://example.com:443", nil)
	req = req.WithContext(context.Background())
	if err := proxyConnect(req, conn); err == nil {
		t.Fatal("malformed proxy reply accepted")
	}
}

// TestProxyConnect_CanceledContextInterrupts pins the context watcher: with
// an already-canceled context and a proxy that accepts but never replies,
// the watcher's negative deadline interrupts the blocked read instead of
// hanging until the dial timeout.
func TestProxyConnect_CanceledContextInterrupts(t *testing.T) {
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
		_, _ = io.Copy(io.Discard, conn) // swallow the CONNECT, never reply
		_ = conn.Close()
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req, _ := http.NewRequest(http.MethodConnect, "http://example.com:443", nil)
	req = req.WithContext(ctx)

	start := time.Now()
	if err := proxyConnect(req, conn); err == nil {
		t.Fatal("canceled context should fail the CONNECT exchange")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("canceled CONNECT took %v; the watcher should interrupt promptly", elapsed)
	}
}
