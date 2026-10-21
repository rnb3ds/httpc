package fingerprint

// Test-only CONNECT proxy: a minimal TCP proxy that answers CONNECT with a
// fixed status and then pipes bytes bidirectionally, recording the CONNECT
// request head so proxy-path tests can assert what actually crossed the wire.
//
// Kept in its own file because two suites use it: the adapter's proxied round
// trip (proxy_test.go) and the proxy-scheme validation tests.

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
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
