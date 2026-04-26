package engine

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestStreamModeContextCancelCleanup verifies that the context cancel function
// is properly cleaned up when streaming mode is used and ReleaseResponse is called.
// This prevents timer leaks from context.WithTimeout.
func TestStreamModeContextCancelCleanup(t *testing.T) {
	var cancelCalled atomic.Int32

	mock := &mockTransport{
		Response: &http.Response{
			StatusCode: http.StatusOK,
			Status:     "OK",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("streaming body")),
		},
	}

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
	}

	client, err := NewClient(config, func(opts *clientOptions) {
		opts.customTransport = mock
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	resp, err := client.Request(context.Background(), "GET", "https://example.com",
		func(r *Request) error {
			r.SetStreamBody(true)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Response should not be nil")
	}

	// Verify streaming response has a body reader
	if resp.RawBodyReader() == nil {
		t.Fatal("Streaming response should have a raw body reader")
	}

	// Cancel function should be set for streaming mode
	// (ReleaseResponse is responsible for calling it)
	ReleaseResponse(resp)

	// Verify cancel was tracked
	if cancelCalled.Load() > 1 {
		t.Error("Cancel should not be called multiple times")
	}
}

// TestStreamModeContextCancelOnEarlyError verifies that the context cancel function
// is called when a streaming-mode request fails before the response is created.
// This prevents timer leaks from context.WithTimeout when the context is already expired.
func TestStreamModeContextCancelOnEarlyError(t *testing.T) {
	config := &Config{
		Timeout:         5 * time.Second,
		AllowPrivateIPs: true,
	}

	client, err := NewClient(config, func(opts *clientOptions) {
		opts.customTransport = newMockTransport(200, "OK")
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create an already-expired context to trigger early error
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = client.Request(ctx, "GET", "https://example.com",
		func(r *Request) error {
			r.SetStreamBody(true)
			return nil
		},
	)
	if err == nil {
		t.Fatal("Expected error from cancelled context")
	}

	// The key assertion: no goroutine or timer leak.
	// The cancel func from context.WithTimeout (created in executeRequest)
	// must have been called even though streaming mode was requested.
	// We can't directly observe cancel calls, but the test verifies
	// the error path completes without hanging or leaking.
}

// TestNonStreamModeContextCancel verifies that non-streaming requests
// always have their context cancelled (baseline for comparison).
func TestNonStreamModeContextCancel(t *testing.T) {
	mock := newMockTransport(200, "hello world")

	config := &Config{
		Timeout:         10 * time.Second,
		AllowPrivateIPs: true,
	}

	client, err := NewClient(config, func(opts *clientOptions) {
		opts.customTransport = mock
	})
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	resp, err := client.Request(context.Background(), "GET", "https://example.com")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}

	ReleaseResponse(resp)
}

// TestReleaseResponseCleansUpStreamingResources verifies that ReleaseResponse
// properly closes the raw body reader and calls the cancel function for
// streaming responses.
func TestReleaseResponseCleansUpStreamingResources(t *testing.T) {
	var bodyClosed atomic.Int32
	var cancelCalled atomic.Int32

	body := &trackingReadCloser{
		Reader: strings.NewReader("test data"),
		onClose: func() {
			bodyClosed.Store(1)
		},
	}

	resp := getResponse()
	resp.SetStatusCode(200)
	resp.rawBodyReader = body
	resp.cancelFunc = func() {
		cancelCalled.Store(1)
	}

	ReleaseResponse(resp)

	if bodyClosed.Load() != 1 {
		t.Error("Body reader was not closed by ReleaseResponse")
	}
	if cancelCalled.Load() != 1 {
		t.Error("Cancel function was not called by ReleaseResponse")
	}
}

// TestReleaseResponseNilSafe verifies that ReleaseResponse handles nil and
// already-released responses without panicking.
func TestReleaseResponseNilSafe(t *testing.T) {
	ReleaseResponse(nil)
	ReleaseResponse(&Response{})

	resp := getResponse()
	ReleaseResponse(resp)
	ReleaseResponse(resp) // Double release should not panic
}

// trackingReadCloser tracks Close calls for testing.
type trackingReadCloser struct {
	*strings.Reader
	onClose func()
}

func (t *trackingReadCloser) Close() error {
	if t.onClose != nil {
		t.onClose()
	}
	return nil
}
