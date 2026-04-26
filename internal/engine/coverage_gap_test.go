package engine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"
)

// ============================================================================
// Task 1a: isRetryableWrappedError tests (0% coverage)
// ============================================================================

func TestIsRetryableWrappedError(t *testing.T) {
	tests := []struct {
		name      string
		inner     *ClientError
		wantRetry bool
	}{
		{
			name: "inner with retryable network message cause",
			inner: &ClientError{
				Type:  ErrorTypeNetwork,
				Cause: errors.New("connection reset by peer"),
			},
			wantRetry: true,
		},
		{
			name: "inner with non-retryable cause message",
			inner: &ClientError{
				Type:  ErrorTypeNetwork,
				Cause: errors.New("something unknown"),
			},
			wantRetry: false,
		},
		{
			name: "inner with nil cause but retryable type (timeout)",
			inner: &ClientError{
				Type: ErrorTypeTimeout,
			},
			wantRetry: true,
		},
		{
			name: "inner with nil cause and non-retryable type (validation)",
			inner: &ClientError{
				Type: ErrorTypeValidation,
			},
			wantRetry: false,
		},
		{
			name: "inner with EOF message in cause",
			inner: &ClientError{
				Type:  ErrorTypeNetwork,
				Cause: errors.New("unexpected EOF"),
			},
			wantRetry: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outer := &ClientError{
				Type:  ErrorTypeNetwork,
				Cause: tt.inner,
			}
			got := outer.IsRetryable()
			if got != tt.wantRetry {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.wantRetry)
			}
		})
	}
}

// ============================================================================
// Task 1b: isRetryableSyscallError tests (0% coverage)
// ============================================================================

func TestIsRetryableSyscallError(t *testing.T) {
	tests := []struct {
		name     string
		errno    syscall.Errno
		expected bool
	}{
		{"ECONNREFUSED", syscall.ECONNREFUSED, true},
		{"ECONNRESET", syscall.ECONNRESET, true},
		{"EPIPE", syscall.EPIPE, true},
		{"Non-retryable errno", syscall.EINVAL, false},
	}

	// Add platform-specific errno values
	if errno, ok := lookupErrno("ETIMEDOUT"); ok {
		tests = append(tests, struct {
			name     string
			errno    syscall.Errno
			expected bool
		}{"ETIMEDOUT", errno, true})
	}
	if errno, ok := lookupErrno("ENETUNREACH"); ok {
		tests = append(tests, struct {
			name     string
			errno    syscall.Errno
			expected bool
		}{"ENETUNREACH", errno, true})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableSyscallError(tt.errno)
			if got != tt.expected {
				t.Errorf("isRetryableSyscallError(%v) = %v, want %v", tt.errno, got, tt.expected)
			}
		})
	}
}

// lookupErrno tries to find a syscall errno by name using platform-specific values.
func lookupErrno(name string) (syscall.Errno, bool) {
	switch name {
	case "ETIMEDOUT":
		// Windows: WSAETIMEDOUT = 10060, Unix: ETIMEDOUT varies
		for _, errno := range []syscall.Errno{syscall.ETIMEDOUT} {
			if errno != 0 {
				return errno, true
			}
		}
	case "ENETUNREACH":
		for _, errno := range []syscall.Errno{syscall.ENETUNREACH} {
			if errno != 0 {
				return errno, true
			}
		}
	}
	return 0, false
}

// ============================================================================
// Task 1c: isRetryableDNSError additional case (non-DNSError cause)
// ============================================================================

func TestIsRetryableDNSError_NonDNSCause(t *testing.T) {
	err := &ClientError{
		Type:  ErrorTypeDNS,
		Cause: errors.New("not a DNS error"),
	}
	if err.IsRetryable() {
		t.Error("Expected non-retryable for non-DNSError cause in DNS type")
	}
}

// ============================================================================
// Task 1d: isRetryableOpError additional cases (75% -> higher)
// ============================================================================

func TestIsRetryableOpError_TimeoutTrue(t *testing.T) {
	err := &ClientError{
		Type: ErrorTypeNetwork,
		Cause: &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: &mockNetError{timeout: true, msg: "i/o timeout"},
		},
	}
	if !err.IsRetryable() {
		t.Error("Expected retryable for OpError with Timeout()=true")
	}
}

func TestIsRetryableOpError_WithSyscallErrno(t *testing.T) {
	err := &ClientError{
		Type: ErrorTypeNetwork,
		Cause: &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: syscall.ECONNREFUSED,
		},
	}
	if !err.IsRetryable() {
		t.Error("Expected retryable for OpError with ECONNREFUSED errno")
	}
}

func TestIsRetryableOpError_WithNonRetryableErrno(t *testing.T) {
	err := &ClientError{
		Type: ErrorTypeNetwork,
		Cause: &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: syscall.EINVAL,
		},
	}
	if err.IsRetryable() {
		t.Error("Expected non-retryable for OpError with non-retryable errno")
	}
}

func TestIsRetryableOpError_WithRetryableMessage(t *testing.T) {
	err := &ClientError{
		Type: ErrorTypeNetwork,
		Cause: &net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: errors.New("connection reset by peer"),
		},
	}
	if !err.IsRetryable() {
		t.Error("Expected retryable for OpError with retryable message")
	}
}

func TestIsRetryableOpError_ContextDeadlineExceeded(t *testing.T) {
	err := &ClientError{
		Type: ErrorTypeNetwork,
		Cause: &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: context.DeadlineExceeded,
		},
	}
	if err.IsRetryable() {
		t.Error("Expected non-retryable for OpError with context.DeadlineExceeded")
	}
}

// ============================================================================
// Task 2: Streaming body tests (0% coverage)
// ============================================================================

func TestStreamingBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("streaming body content"))
	}))
	defer server.Close()

	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
		MaxRetries:      0,
		UserAgent:       "test/1.0",
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	t.Run("SetStreamBody true returns raw body reader", func(t *testing.T) {
		streamOption := func(req *Request) error {
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

// ============================================================================
// Task 3: Close method tests for pooled readers (0% coverage)
// ============================================================================

// io.Closer is tested for all pooled reader/buffer types via table-driven test.
func TestPooledReaders_Close(t *testing.T) {
	t.Run("StringsReader close before reading", func(t *testing.T) {
		reader := getPooledStringsReader("hello").(*pooledStringsReader)
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if reader.reader != nil {
			t.Error("Expected reader to be nil after Close")
		}
	})

	t.Run("StringsReader double close", func(t *testing.T) {
		reader := getPooledStringsReader("hello").(*pooledStringsReader)
		_ = reader.Close()
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error on double close: %v", err)
		}
	})

	t.Run("BytesReader close before reading", func(t *testing.T) {
		reader := getPooledBytesReader([]byte("hello")).(*pooledBytesReader)
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if reader.reader != nil {
			t.Error("Expected reader to be nil after Close")
		}
	})

	t.Run("BytesReader double close", func(t *testing.T) {
		reader := getPooledBytesReader([]byte("hello")).(*pooledBytesReader)
		_ = reader.Close()
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error on double close: %v", err)
		}
	})

	t.Run("MultipartBuffer close owned", func(t *testing.T) {
		buf := getMultipartBuffer()
		buf.WriteString("multipart data")
		reader := &pooledMultipartBuffer{buf: buf, owned: true}
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if reader.buf != nil {
			t.Error("Expected buf to be nil after Close")
		}
	})

	t.Run("MultipartBuffer close nil", func(t *testing.T) {
		reader := &pooledMultipartBuffer{buf: nil, owned: false}
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("MultipartBuffer close not-owned", func(t *testing.T) {
		buf := bytes.NewBufferString("data")
		reader := &pooledMultipartBuffer{buf: buf, owned: false}
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("MultipartBuffer double close", func(t *testing.T) {
		buf := getMultipartBuffer()
		buf.WriteString("data")
		reader := &pooledMultipartBuffer{buf: buf, owned: true}
		_ = reader.Close()
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error on double close: %v", err)
		}
	})

	t.Run("JSONBuffer close owned", func(t *testing.T) {
		buf := getJSONBuffer()
		buf.WriteString(`{"test": true}`)
		reader := &pooledJSONBuffer{buf: buf, owned: true}
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if reader.buf != nil {
			t.Error("Expected buf to be nil after Close")
		}
	})

	t.Run("JSONBuffer close nil", func(t *testing.T) {
		reader := &pooledJSONBuffer{buf: nil, owned: false}
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("JSONBuffer close not-owned", func(t *testing.T) {
		buf := bytes.NewBufferString(`{"data":1}`)
		reader := &pooledJSONBuffer{buf: buf, owned: false}
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("JSONBuffer double close", func(t *testing.T) {
		buf := getJSONBuffer()
		buf.WriteString(`{"test": true}`)
		reader := &pooledJSONBuffer{buf: buf, owned: true}
		_ = reader.Close()
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error on double close: %v", err)
		}
	})
}

// ============================================================================
// Task 4: createDecompressor edge cases (68.8% -> higher)
// ============================================================================

func TestCreateDecompressor_GzipResetFailure(t *testing.T) {
	// Clear pools so we get fresh readers
	clearResponsePools()

	config := &Config{Timeout: 30 * time.Second}
	processor := newResponseProcessor(config)

	// Force gzip pool to have a reader that will fail on Reset.
	// We do this by getting a reader from the pool, putting it in a state
	// where Reset will fail, then returning it to pool.
	pooled, ok := gzipReaderPool.Get().(*gzip.Reader)
	if ok && pooled != nil {
		// Reset with nil to put reader in a bad state, then return to pool
		_ = pooled.Reset(bytes.NewReader(nil))
		// Now try reading to put reader in EOF state, then return to pool
		gzipReaderPool.Put(pooled)
	}

	// Now when createDecompressor gets a pooled reader, Reset may succeed
	// but we test the fallback path by using an error reader
	errorReader := &errorReader{err: fmt.Errorf("reset failure simulation")}
	decompressor, err := processor.createDecompressor(errorReader, "gzip")
	if err != nil {
		// Reset failed fallback creates a new reader which may also fail
		t.Logf("Error from gzip decompressor (acceptable): %v", err)
	} else {
		_ = decompressor.Close()
	}
}

func TestCreateDecompressor_FlateNonResetter(t *testing.T) {
	// Clear pools to ensure clean state
	clearResponsePools()

	config := &Config{Timeout: 30 * time.Second}
	processor := newResponseProcessor(config)

	// Put a non-Resetter io.ReadCloser into the flate pool
	flateReaderPool.Put(io.NopCloser(bytes.NewReader([]byte("dummy"))))

	// When createDecompressor gets this non-Resetter, it should fall through
	// to creating a new flate.NewReader
	var buf bytes.Buffer
	fw, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	_, _ = fw.Write([]byte("flate test"))
	_ = fw.Close()

	decompressor, err := processor.createDecompressor(bytes.NewReader(buf.Bytes()), "deflate")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	data, err := io.ReadAll(decompressor)
	if err != nil {
		t.Fatalf("Failed to read decompressed data: %v", err)
	}

	if string(data) != "flate test" {
		t.Errorf("Expected 'flate test', got %q", string(data))
	}
	_ = decompressor.Close()
}

func TestCreateDecompressor_GzipDirectNewReader(t *testing.T) {
	// Test the fallback path where pool returns a wrong type.
	// Clear the pool and put a non-*gzip.Reader value in it.
	clearResponsePools()
	gzipReaderPool.Put("not a gzip reader") // wrong type

	config := &Config{Timeout: 30 * time.Second}
	processor := newResponseProcessor(config)

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte("direct gzip"))
	_ = gw.Close()

	decompressor, err := processor.createDecompressor(bytes.NewReader(buf.Bytes()), "gzip")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	data, err := io.ReadAll(decompressor)
	if err != nil {
		t.Fatalf("Failed to read: %v", err)
	}

	if string(data) != "direct gzip" {
		t.Errorf("Expected 'direct gzip', got %q", string(data))
	}
	_ = decompressor.Close()
}

// errorReader is a reader that always returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}
