package fingerprint

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestTransport_ConcurrentH2Dials pins the property the enetx/http2 fork did
// not have: many concurrent h2 connections through one adapter must be
// race-free. The old transport wrote a shared Transport field from every
// newClientConn, which -race reported immediately at this shape.
func TestTransport_ConcurrentH2Dials(t *testing.T) {
	const servers = 8
	urls := make([]string, servers)
	srvs := make([]*httptest.Server, servers)
	for i := 0; i < servers; i++ {
		s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))
		s.EnableHTTP2 = true
		s.StartTLS()
		srvs[i] = s
		urls[i] = s.URL
	}
	t.Cleanup(func() {
		for _, s := range srvs {
			s.Close()
		}
	})

	adapter := newTestAdapter(t, "chrome", true, nil)
	defer func() { _ = adapter.Close() }()

	for round := 0; round < 3; round++ {
		var wg sync.WaitGroup
		for i := 0; i < servers; i++ {
			wg.Add(1)
			go func(u string) {
				defer wg.Done()
				resp, err := adapter.RoundTrip(httptestReq(t, u+"/x"))
				if err != nil {
					t.Errorf("RoundTrip(%s): %v", u, err)
					return
				}
				_ = resp.Body.Close()
			}(urls[i])
		}
		wg.Wait()
	}
}

// TestTransport_PerProxyIsolation pins that each proxy selection gets its own
// backend and that a second request reuses it: one connection pool never mixes
// hops, and construction is not repeated per request.
func TestTransport_PerProxyIsolation(t *testing.T) {
	a := NewAdapter(Config{PresetName: "chrome", AllowPrivateIPs: true})
	if a.baseErr != nil {
		t.Fatalf("base backend build: %v", a.baseErr)
	}

	b1, err := a.backendFor(mustParseURL(t, "http://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("backendFor: %v", err)
	}
	b2, err := a.backendFor(mustParseURL(t, "http://127.0.0.1:2"))
	if err != nil {
		t.Fatalf("backendFor: %v", err)
	}
	b1again, err := a.backendFor(mustParseURL(t, "http://127.0.0.1:1"))
	if err != nil {
		t.Fatalf("backendFor again: %v", err)
	}

	if b1 == b2 {
		t.Error("distinct proxies share one backend")
	}
	if b1 != b1again {
		t.Error("the same proxy built a second backend instead of reusing the cached one")
	}
	if a.base == b1 {
		t.Error("a proxied backend aliases the direct-connection backend")
	}
}

// TestTransport_RequestDeadlineGovernsDial pins that the engine's context
// deadline reaches the socket: a target that never answers must fail near the
// deadline, not hang.
func TestTransport_RequestDeadlineGovernsDial(t *testing.T) {
	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
	}))
	defer blocked.Close()

	a := newTestAdapter(t, "chrome", true, nil)
	defer func() { _ = a.Close() }()

	req := httptestReq(t, blocked.URL+"/").WithContext(mustTimeoutCtx(t, 300*time.Millisecond))
	start := time.Now()
	resp, err := a.RoundTrip(req)
	elapsed := time.Since(start)

	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("request completed despite the deadline")
	}
	if elapsed > 2*time.Second {
		t.Errorf("took %v; the request deadline did not reach the transport", elapsed)
	}
}
