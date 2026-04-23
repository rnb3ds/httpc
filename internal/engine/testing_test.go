package engine

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// ============================================================================
// MOCK TRANSPORT TESTS
// ============================================================================

func TestMockTransport_New(t *testing.T) {
	mock := newMockTransport(http.StatusOK, "test body")

	if mock == nil {
		t.Fatal("expected non-nil mockTransport")
	}
	if mock.Response == nil {
		t.Fatal("Expected non-nil Response")
	}
	if mock.Response.StatusCode != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, mock.Response.StatusCode)
	}
}

func TestMockTransport_RoundTrip(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		mock := newMockTransport(http.StatusOK, "success")
		req, _ := http.NewRequest("GET", "http://example.com", nil)

		resp, err := mock.RoundTrip(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
		}
		if !mock.Called {
			t.Error("Expected Called to be true")
		}
		if mock.CallCount != 1 {
			t.Errorf("Expected CallCount=1, got %d", mock.CallCount)
		}
	})

	t.Run("Error", func(t *testing.T) {
		mock := newMockTransport(http.StatusOK, "")
		expectedErr := errors.New("network error")
		mock.SetError(expectedErr)

		req, _ := http.NewRequest("GET", "http://example.com", nil)
		_, err := mock.RoundTrip(req)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
		if !errors.Is(err, expectedErr) {
			t.Errorf("Expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("MultipleCalls", func(t *testing.T) {
		mock := newMockTransport(http.StatusOK, "")

		for i := 0; i < 5; i++ {
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			_, _ = mock.RoundTrip(req)
		}

		if mock.CallCount != 5 {
			t.Errorf("Expected CallCount=5, got %d", mock.CallCount)
		}
		if len(mock.Requests) != 5 {
			t.Errorf("Expected 5 requests, got %d", len(mock.Requests))
		}
	})
}

func TestMockTransport_SetRedirectPolicy(t *testing.T) {
	mock := newMockTransport(http.StatusOK, "")
	ctx := context.Background()

	// Should return context unchanged
	result, cleanup := mock.SetRedirectPolicy(ctx, true, 5)
	if result != ctx {
		t.Error("Expected context to be returned unchanged")
	}
	cleanup()
}

func TestMockTransport_GetRedirectChain(t *testing.T) {
	t.Run("Empty", func(t *testing.T) {
		mock := newMockTransport(http.StatusOK, "")
		chain := mock.GetRedirectChain(context.Background())

		if len(chain) != 0 {
			t.Errorf("Expected empty chain, got %d items", len(chain))
		}
	})

	t.Run("WithRedirects", func(t *testing.T) {
		mock := newMockTransport(http.StatusOK, "")
		mock.RedirectChain = []string{"http://a.com", "http://b.com", "http://c.com"}

		chain := mock.GetRedirectChain(context.Background())
		if len(chain) != 3 {
			t.Errorf("Expected 3 redirects, got %d", len(chain))
		}
	})
}

func TestMockTransport_Close(t *testing.T) {
	mock := newMockTransport(http.StatusOK, "")

	err := mock.Close()
	if err != nil {
		t.Errorf("Unexpected error on Close: %v", err)
	}
}

func TestMockTransport_SetResponse(t *testing.T) {
	mock := newMockTransport(http.StatusOK, "initial")

	// Change response
	mock.SetResponse(http.StatusNotFound, "not found")

	if mock.Response.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, mock.Response.StatusCode)
	}
}

func TestMockTransport_SetError(t *testing.T) {
	mock := newMockTransport(http.StatusOK, "")

	expectedErr := errors.New("test error")
	mock.SetError(expectedErr)

	if mock.Error == nil {
		t.Fatal("Expected error to be set")
	}
	if !errors.Is(mock.Error, expectedErr) {
		t.Errorf("Expected error %v, got %v", expectedErr, mock.Error)
	}
}

func TestMockTransport_GetCallCount(t *testing.T) {
	mock := newMockTransport(http.StatusOK, "")

	// Initial count
	if mock.GetCallCount() != 0 {
		t.Errorf("Expected initial count 0, got %d", mock.GetCallCount())
	}

	// Make some calls
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest("GET", "http://example.com", nil)
		_, _ = mock.RoundTrip(req)
	}

	if mock.GetCallCount() != 3 {
		t.Errorf("Expected count 3, got %d", mock.GetCallCount())
	}
}

func TestMockTransport_GetLastRequest(t *testing.T) {
	t.Run("NoRequests", func(t *testing.T) {
		mock := newMockTransport(http.StatusOK, "")
		req := mock.GetLastRequest()

		if req != nil {
			t.Error("Expected nil for no requests")
		}
	})

	t.Run("WithRequests", func(t *testing.T) {
		mock := newMockTransport(http.StatusOK, "")

		urls := []string{"http://a.com", "http://b.com", "http://c.com"}
		for _, url := range urls {
			req, _ := http.NewRequest("GET", url, nil)
			_, _ = mock.RoundTrip(req)
		}

		lastReq := mock.GetLastRequest()
		if lastReq == nil {
			t.Fatal("Expected non-nil request")
		}
		if !strings.HasSuffix(lastReq.URL.String(), "c.com") {
			t.Errorf("Expected last URL to end with c.com, got %s", lastReq.URL)
		}
	})
}

func TestMockTransport_Reset(t *testing.T) {
	mock := newMockTransport(http.StatusOK, "")

	// Make some calls and set error
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	_, _ = mock.RoundTrip(req)
	mock.SetError(errors.New("test error"))

	// Reset
	mock.Reset()

	if mock.Called {
		t.Error("Expected Called to be false after reset")
	}
	if mock.CallCount != 0 {
		t.Errorf("Expected CallCount=0, got %d", mock.CallCount)
	}
	if len(mock.Requests) != 0 {
		t.Errorf("Expected no requests, got %d", len(mock.Requests))
	}
	if mock.Error != nil {
		t.Error("Expected error to be nil after reset")
	}
}

func TestMockTransport_ConcurrentAccess(t *testing.T) {
	mock := newMockTransport(http.StatusOK, "")

	// Concurrent reads and writes
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			req, _ := http.NewRequest("GET", "http://example.com", nil)
			_, _ = mock.RoundTrip(req)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = mock.GetCallCount()
			_ = mock.GetLastRequest()
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done

	if mock.GetCallCount() != 100 {
		t.Errorf("Expected CallCount=100, got %d", mock.GetCallCount())
	}
}
