package engine

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/types"
)

// TestClient_ReaderBodyContentLength pins net/http (http.NewRequest) parity
// for io.Reader bodies: the three concrete replayable reader types carry a
// knowable length, so the request goes out with Content-Length instead of
// Transfer-Encoding: chunked — servers, proxies, and request-signing schemes
// that reject (or sign over) chunked uploads expect the stdlib wire shape.
// The MaxRequestBodySize limit-reader wrap hides the concrete type and with
// it the authoritative length, so those requests stay chunked.
func TestClient_ReaderBodyContentLength(t *testing.T) {
	const payload = "reader-body-payload"
	tests := []struct {
		name             string
		body             func() io.Reader
		maxRequestBodySz int64
		wantLen          int64
		wantChunked      bool
	}{
		{
			name:        "strings.Reader sends Content-Length",
			body:        func() io.Reader { return strings.NewReader(payload) },
			wantLen:     int64(len(payload)),
			wantChunked: false,
		},
		{
			name:        "bytes.Reader sends Content-Length",
			body:        func() io.Reader { return bytes.NewReader([]byte(payload)) },
			wantLen:     int64(len(payload)),
			wantChunked: false,
		},
		{
			name:        "bytes.Buffer sends Content-Length",
			body:        func() io.Reader { return bytes.NewBufferString(payload) },
			wantLen:     int64(len(payload)),
			wantChunked: false,
		},
		{
			name: "limit-reader wrap stays chunked",
			body: func() io.Reader { return strings.NewReader(payload) },
			// Above the payload size: the limit reader wraps but never trips;
			// its presence alone must switch the wire to chunked.
			maxRequestBodySz: int64(len(payload)) + 8,
			wantLen:          -1,
			wantChunked:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotChunked := len(r.TransferEncoding) > 0 && r.TransferEncoding[0] == "chunked"
				if r.ContentLength != tt.wantLen {
					t.Errorf("server saw Content-Length = %d, want %d", r.ContentLength, tt.wantLen)
				}
				if gotChunked != tt.wantChunked {
					t.Errorf("server saw Transfer-Encoding = %v, wantChunked = %t", r.TransferEncoding, tt.wantChunked)
				}
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			config := &Config{
				Timeout:            30 * time.Second,
				MaxRetries:         0, // fast path: the raw reader reaches Build unwrapped
				UserAgent:          "test-client/1.0",
				MaxRequestBodySize: tt.maxRequestBodySz,
				Connection:         connection.Config{AllowPrivateIPs: true},
			}
			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("NewClient failed: %v", err)
			}
			defer func() { _ = client.Close() }()

			body := tt.body()
			resp, err := client.Request(context.Background(), http.MethodPost, server.URL,
				func(r types.RequestMutator) error {
					r.SetBody(body)
					return nil
				})
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}
			if resp.StatusCode() != http.StatusOK {
				t.Errorf("status = %d, want 200", resp.StatusCode())
			}
		})
	}
}

// TestClient_RetryBufferedReaderKeepsContentType: enabling retries buffers a
// raw-reader body to []byte, which used to flip the wire Content-Type from
// none to application/octet-stream (Build's []byte auto-detection). The
// buffered body keeps the raw-reader contract — no automatic Content-Type —
// so the wire is identical with retries on or off.
func TestClient_RetryBufferedReaderKeepsContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "" {
			t.Errorf("server saw Content-Type = %q, want none (raw-reader contract)", ct)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &Config{
		Timeout:    30 * time.Second,
		MaxRetries: 1, // slow path: the reader is buffered to []byte before the loop
		UserAgent:  "test-client/1.0",
		Connection: connection.Config{AllowPrivateIPs: true},
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Request(context.Background(), http.MethodPost, server.URL,
		func(r types.RequestMutator) error {
			r.SetBody(strings.NewReader("payload"))
			return nil
		})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode())
	}
}

// TestClient_NilReaderBodyRejected: a typed-nil io.Reader body
// ((*bytes.Reader)(nil)) must surface as a request error on BOTH paths —
// the fast path previously dereferenced it inside net/http's writeLoop
// goroutine (process crash), and the retry path panicked in io.ReadAll on
// the caller's stack.
func TestClient_NilReaderBodyRejected(t *testing.T) {
	for _, retries := range []int{0, 1} {
		config := &Config{
			Timeout:    30 * time.Second,
			MaxRetries: retries,
			UserAgent:  "test-client/1.0",
			Connection: connection.Config{AllowPrivateIPs: true},
		}
		client, err := NewClient(config)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		_, err = client.Request(context.Background(), http.MethodPost, "http://127.0.0.1:1/x",
			func(r types.RequestMutator) error {
				r.SetBody((*bytes.Reader)(nil))
				return nil
			})
		if err == nil {
			t.Errorf("MaxRetries=%d: expected nil-reader rejection, got nil", retries)
		} else if !strings.Contains(err.Error(), "nil io.Reader") {
			t.Errorf("MaxRetries=%d: error should name the nil reader, got: %v", retries, err)
		}
		_ = client.Close()
	}
}
