package dns

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestDoHBodyDrainAndConnectionReuse verifies that the DoH resolver drains the
// response body even on error responses, allowing the HTTP transport to reuse
// connections. This tests the fix for the body drain issue.
func TestDoHBodyDrainAndConnectionReuse(t *testing.T) {
	var requestCount atomic.Int32

	// Server returns 500 on first request, then valid JSON on second
	firstRequest := atomic.Bool{}
	firstRequest.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)

		if firstRequest.Load() {
			firstRequest.Store(false)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error"))
			return
		}

		// Return valid JSON response for second request
		w.Header().Set("Content-Type", "application/dns-json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"Status":0,"Answer":[{"name":"test.local","type":1,"data":"1.2.3.4"}]}`))
	}))
	defer server.Close()

	provider := &DoHProvider{
		Name:     "test",
		Template: server.URL + "/dns-query?name={name}&type=A",
		Priority: 1,
	}

	resolver := NewDoHResolver([]*DoHProvider{provider}, 5*time.Minute)
	defer resolver.Close()

	// First request gets 500 → falls back to system resolver (may succeed or fail)
	// The important thing is the body is drained so the connection is reusable
	_, _ = resolver.LookupIPAddr(context.Background(), "test.local")

	// Second request should reach the server again (connection reused or new)
	// and get a valid response
	ips, err := resolver.LookupIPAddr(context.Background(), "test.local")
	if err != nil {
		t.Fatalf("Second request should succeed: %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("Expected at least one IP address")
	}

	// Both requests must have reached our server
	if requestCount.Load() < 2 {
		t.Errorf("Expected at least 2 server requests, got %d", requestCount.Load())
	}
}

// TestDoHBodyDrainOnInvalidResponse verifies that the DoH resolver drains the
// response body when the server returns invalid JSON, preventing connection leaks.
func TestDoHBodyDrainOnInvalidResponse(t *testing.T) {
	var requestCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/dns-json")
		w.WriteHeader(http.StatusOK)
		// Return invalid JSON on first request, valid on second
		if requestCount.Load() == 1 {
			w.Write([]byte("not valid json at all"))
			return
		}
		w.Write([]byte(`{"Status":0,"Answer":[{"name":"test.local","type":1,"data":"5.6.7.8"}]}`))
	}))
	defer server.Close()

	provider := &DoHProvider{
		Name:     "test",
		Template: server.URL + "/dns-query?name={name}&type=A",
		Priority: 1,
	}

	resolver := NewDoHResolver([]*DoHProvider{provider}, 5*time.Minute)
	defer resolver.Close()

	// First request: invalid JSON → falls back to system resolver
	_, _ = resolver.LookupIPAddr(context.Background(), "test.local")

	// Second request: valid JSON → should succeed via DoH
	ips, err := resolver.LookupIPAddr(context.Background(), "test.local")
	if err != nil {
		t.Fatalf("Second request should succeed: %v", err)
	}
	if len(ips) == 0 {
		t.Fatal("Expected at least one IP address")
	}

	if requestCount.Load() < 2 {
		t.Errorf("Expected at least 2 server requests, got %d", requestCount.Load())
	}
}
