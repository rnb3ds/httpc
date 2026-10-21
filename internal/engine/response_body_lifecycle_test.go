package engine

// Response body lifecycle: truncation detection, streaming (raw reader)
// passthrough and its size cap, release/cleanup of response resources, and
// the Response accessors/mutators that manage body state. Merges the former
// response_truncation_test.go and resource_leak_test.go plus the response-
// lifecycle tests dissolved out of coverage_test.go (same subject area).

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/types"
)

// ============================================================================
// READ-BODY TRUNCATION (former response_truncation_test.go)
// ============================================================================

// TestReadBody_TruncatedBodyIsError is a regression test for the fast-path
// readBody bug: a server that declares Content-Length but closes the
// connection after sending fewer bytes used to produce a "successful"
// response with the partial body when StrictContentLength was disabled
// (e.g. PerformanceConfig). The slow path already surfaced this as an error;
// the fast path must behave the same.
func TestReadBody_TruncatedBodyIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("short")) // 5 of 100 declared bytes, then close
	}))
	defer srv.Close()

	cfg := &Config{
		Timeout:     5 * 1e9,
		ValidateURL: true,

		StrictContentLength: false, Connection: // must not matter — truncation is an error per se
		connection.Config{AllowPrivateIPs: true},
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Small declared length exercises the pre-sized fast path in readBody.
	if _, err := client.Request(backgroundCtx, "GET", srv.URL); err == nil {
		t.Fatal("expected error for truncated response body, got nil")
	}

	// A declared length above maxBufferSize (512KB) exercises the buffered
	// slow path and must keep failing as before.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1048576")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("short"))
	}))
	defer srv2.Close()
	if _, err := client.Request(backgroundCtx, "GET", srv2.URL); err == nil {
		t.Fatal("expected error for truncated large response body, got nil")
	}
}

// TestReadBody_HeadRequestKeepsEmptyBody guards the one legitimate case the
// truncation fix must preserve: HEAD responses carry a declared
// Content-Length but no body, which reads as (0, io.EOF).
func TestReadBody_HeadRequestKeepsEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &Config{Timeout: 5 * 1e9, ValidateURL: true, Connection: connection.Config{AllowPrivateIPs: true}}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Request(backgroundCtx, "HEAD", srv.URL)
	if err != nil {
		t.Fatalf("HEAD request failed: %v", err)
	}
	if got := len(resp.RawBody()); got != 0 {
		t.Fatalf("HEAD body should be empty, got %d bytes", got)
	}
}

// ============================================================================
// RELEASE / CLEANUP (former resource_leak_test.go)
// ============================================================================

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

	// Normal release zeroes the response in place (folded in from the former
	// standalone TestReleaseResponse in coverage_test.go).
	resp := &Response{}
	resp.SetStatusCode(200)
	resp.SetBody("test")
	ReleaseResponse(resp)
	if resp.StatusCode() != 0 {
		t.Error("Expected zeroed response after release")
	}

	resp2 := getResponse()
	ReleaseResponse(resp2)
	ReleaseResponse(resp2) // Double release should not panic
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

// TestDecompressorPoolCleanup verifies that decompressor fallback paths
// properly handle pooled readers instead of silently discarding them.
func TestDecompressorPoolCleanup(t *testing.T) {
	config := &Config{
		Timeout: 10 * time.Second, Connection: connection.Config{AllowPrivateIPs: true},
	}

	// Verify gzip decompression works correctly and the reader returns to the pool.
	mock := &mockTransport{
		Response: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Encoding": []string{"gzip"},
			},
			Body: createGzipBody(t, "compressed data"),
		},
	}

	client, err := NewClient(config, withMockTransport(mock))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Request(context.Background(), "GET", "https://example.com")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if got := resp.Body(); got != "compressed data" {
		t.Errorf("gzip body = %q, want %q (decompression broken)", got, "compressed data")
	}
	ReleaseResponse(resp)

	// Verify deflate decompression works correctly and the reader returns to the pool.
	mock2 := &mockTransport{
		Response: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Encoding": []string{"deflate"},
			},
			Body: createDeflateBody(t, "deflated data"),
		},
	}

	client2, err := NewClient(config, withMockTransport(mock2))
	if err != nil {
		t.Fatalf("Failed to create client2: %v", err)
	}
	defer func() { _ = client2.Close() }()

	resp2, err := client2.Request(context.Background(), "GET", "https://example.com")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if got := resp2.Body(); got != "deflated data" {
		t.Errorf("deflate body = %q, want %q (decompression broken)", got, "deflated data")
	}
	ReleaseResponse(resp2)
}

// createGzipBody creates a gzip-compressed HTTP body for testing.
func createGzipBody(t *testing.T, data string) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(data)); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return io.NopCloser(&buf)
}

// createDeflateBody creates a deflate-compressed HTTP body for testing.
func createDeflateBody(t *testing.T, data string) io.ReadCloser {
	t.Helper()
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	if _, err := w.Write([]byte(data)); err != nil {
		t.Fatalf("flate write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("flate close failed: %v", err)
	}
	return io.NopCloser(&buf)
}

// ============================================================================
// RESPONSE ACCESSORS / MUTATORS (moved from coverage_test.go)
// ============================================================================

// TestResponse_Accessors_TableDriven validates that each Response getter returns the set value.
func TestResponse_Accessors_TableDriven(t *testing.T) {
	cookies := []*http.Cookie{{Name: "session", Value: "abc"}}
	redirectChain := []string{"https://a.com", "https://b.com"}
	reqHeaders := http.Header{"X-Test": {"value"}}

	tests := []struct {
		name    string
		setFunc func(*Response)
		getFunc func(*Response) any
		want    any
	}{
		{
			name:    "StatusCode",
			setFunc: func(r *Response) { r.SetStatusCode(201) },
			getFunc: func(r *Response) any { return r.StatusCode() },
			want:    201,
		},
		{
			name:    "Status",
			setFunc: func(r *Response) { r.SetStatus("201 Created") },
			getFunc: func(r *Response) any { return r.Status() },
			want:    "201 Created",
		},
		{
			name:    "Headers",
			setFunc: func(r *Response) { r.SetHeaders(http.Header{"X-Custom": {"val"}}) },
			getFunc: func(r *Response) any { return r.Headers().Get("X-Custom") },
			want:    "val",
		},
		{
			name:    "Body",
			setFunc: func(r *Response) { r.SetBody("response body") },
			getFunc: func(r *Response) any { return r.Body() },
			want:    "response body",
		},
		{
			name:    "RawBody",
			setFunc: func(r *Response) { r.SetRawBody([]byte("raw")) },
			getFunc: func(r *Response) any { return string(r.RawBody()) },
			want:    "raw",
		},
		{
			name:    "ContentLength",
			setFunc: func(r *Response) { r.SetContentLength(1234) },
			getFunc: func(r *Response) any { return r.ContentLength() },
			want:    int64(1234),
		},
		{
			name:    "Proto",
			setFunc: func(r *Response) { r.SetProto("HTTP/2.0") },
			getFunc: func(r *Response) any { return r.Proto() },
			want:    "HTTP/2.0",
		},
		{
			name:    "Duration",
			setFunc: func(r *Response) { r.SetDuration(5 * time.Second) },
			getFunc: func(r *Response) any { return r.Duration() },
			want:    5 * time.Second,
		},
		{
			name:    "Attempts",
			setFunc: func(r *Response) { r.SetAttempts(3) },
			getFunc: func(r *Response) any { return r.Attempts() },
			want:    3,
		},
		{
			name:    "Cookies count",
			setFunc: func(r *Response) { r.SetCookies(cookies) },
			getFunc: func(r *Response) any { return len(r.Cookies()) },
			want:    1,
		},
		{
			name:    "RedirectChain length",
			setFunc: func(r *Response) { r.SetRedirectChain(redirectChain) },
			getFunc: func(r *Response) any { return len(r.RedirectChain()) },
			want:    2,
		},
		{
			name:    "RedirectCount",
			setFunc: func(r *Response) { r.SetRedirectCount(2) },
			getFunc: func(r *Response) any { return r.RedirectCount() },
			want:    2,
		},
		{
			name:    "ProxyURL",
			setFunc: func(r *Response) { r.SetProxyURL("http://proxy.example.com:8080") },
			getFunc: func(r *Response) any { return r.ProxyURL() },
			want:    "http://proxy.example.com:8080",
		},
		{
			name:    "RequestHeaders",
			setFunc: func(r *Response) { r.SetRequestHeaders(reqHeaders) },
			getFunc: func(r *Response) any { return r.RequestHeaders().Get("X-Test") },
			want:    "value",
		},
		{
			name:    "RequestURL",
			setFunc: func(r *Response) { r.SetRequestURL("https://example.com/api") },
			getFunc: func(r *Response) any { return r.RequestURL() },
			want:    "https://example.com/api",
		},
		{
			name:    "RequestMethod",
			setFunc: func(r *Response) { r.SetRequestMethod("PUT") },
			getFunc: func(r *Response) any { return r.RequestMethod() },
			want:    "PUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{}
			tt.setFunc(resp)
			got := tt.getFunc(resp)
			if got != tt.want {
				t.Errorf("Accessor mismatch: got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestResponse_SetHeader validates SetHeader behavior with nil map and multiple values.
func TestResponse_SetHeader(t *testing.T) {
	t.Run("Nil headers auto-init", func(t *testing.T) {
		resp := &Response{}
		if resp.Headers() != nil {
			t.Error("Expected nil headers initially")
		}
		resp.SetHeader("X-Test", "value1")
		if resp.Headers() == nil {
			t.Error("Expected headers to be auto-initialized")
		}
		if resp.Headers().Get("X-Test") != "value1" {
			t.Errorf("Expected X-Test=value1, got %s", resp.Headers().Get("X-Test"))
		}
	})

	t.Run("Multiple values for same key", func(t *testing.T) {
		resp := &Response{}
		resp.SetHeader("Accept", "text/html", "application/json")
		vals := resp.Headers()["Accept"]
		if len(vals) != 2 {
			t.Errorf("Expected 2 values, got %d", len(vals))
		}
		if vals[0] != "text/html" || vals[1] != "application/json" {
			t.Errorf("Expected [text/html, application/json], got %v", vals)
		}
	})
}

// ============================================================================
// INTERMEDIATE-RESPONSE RELEASE (moved from coverage_test.go)
// ============================================================================

// TestReleaseLastResp validates releasing intermediate response objects.
func TestReleaseLastResp(t *testing.T) {
	t.Run("Non-nil response", func(t *testing.T) {
		resp := getResponse()
		resp.SetStatusCode(200)
		var lastResp = resp
		releaseLastResp(&lastResp)
		if lastResp != nil {
			t.Error("Expected pointer to be nil after release")
		}
	})

	t.Run("Nil response", func(t *testing.T) {
		var lastResp *Response
		releaseLastResp(&lastResp)
		if lastResp != nil {
			t.Error("Expected pointer to remain nil")
		}
	})
}

// ============================================================================
// STREAMING BODY LIFECYCLE (moved from coverage_test.go)
// ============================================================================

func TestStreamingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("streaming body content"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries: 0,
		UserAgent:  "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	t.Run("SetStreamBody true returns raw body reader", func(t *testing.T) {
		streamOption := func(req types.RequestMutator) error {
			req.SetStreamBody(true)
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := client.Request(ctx, "GET", server.URL, streamOption)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer ReleaseResponse(resp)

		if resp.StatusCode() != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode())
		}

		reader := resp.RawBodyReader()
		if reader == nil {
			t.Fatal("Expected non-nil RawBodyReader for streaming request")
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("Failed to read from RawBodyReader: %v", err)
		}

		if string(data) != "streaming body content" {
			t.Errorf("Expected 'streaming body content', got %q", string(data))
		}

		_ = reader.Close()
	})

	t.Run("Non-streaming request returns nil RawBodyReader", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		resp, err := client.Request(ctx, "GET", server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		defer ReleaseResponse(resp)

		if resp.RawBodyReader() != nil {
			t.Error("Expected nil RawBodyReader for non-streaming request")
		}

		if resp.Body() != "streaming body content" {
			t.Errorf("Expected 'streaming body content', got %q", resp.Body())
		}
	})

	t.Run("StreamBody accessor round-trip", func(t *testing.T) {
		req := &Request{}
		if req.StreamBody() {
			t.Error("Expected StreamBody=false by default")
		}
		req.SetStreamBody(true)
		if !req.StreamBody() {
			t.Error("Expected StreamBody=true after SetStreamBody(true)")
		}
	})
}

// TestStreamingBody_ExceedsLimit verifies that a streamed body larger than
// MaxResponseBodySize surfaces as a Read error instead of a synthetic EOF.
// Before this regression guard, the pooled limit reader returned io.EOF when
// its budget ran out, so io.Copy-based consumers (Download) silently truncated
// the body to the limit and reported success.
func TestStreamingBody_ExceedsLimit(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 2000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries:          0,
		MaxResponseBodySize: 1000, Connection: connection.Config{AllowPrivateIPs: true},
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Request(ctx, "GET", server.URL, func(req types.RequestMutator) error {
		req.SetStreamBody(true)
		return nil
	})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer ReleaseResponse(resp)

	reader := resp.RawBodyReader()
	if reader == nil {
		t.Fatal("Expected non-nil RawBodyReader for streaming request")
	}

	read, readErr := io.ReadAll(reader)
	if readErr == nil {
		t.Fatalf("Expected error reading oversize streamed body, got success (%d bytes)", len(read))
	}
	if !strings.Contains(readErr.Error(), "exceeds size limit") {
		t.Errorf("Expected size-limit error, got: %v", readErr)
	}
	if len(read) > 1001 { // limit + the one over-read byte
		t.Errorf("Read %d bytes, want at most limit+1", len(read))
	}
}
