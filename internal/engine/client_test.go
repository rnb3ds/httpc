package engine

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/types"

	"github.com/cybergodev/httpc/internal/connection"
)

func TestNewClient(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		MaxResponseBodySize: 50 * 1024 * 1024,
		ValidateURL:         true,
		ValidateHeaders:     true,

		MaxRetries:    3,
		RetryDelay:    100 * time.Millisecond,
		BackoffFactor: 2.0,
		UserAgent:     "test-client/1.0",
		EnableCookies: true, Connection: connection.Config{MaxIdleConns: 100,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     20,

			AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	if client == nil {
		t.Fatal("Client should not be nil")
	}

	// Test that client is properly initialized
	// Note: Config is deep-copied for thread safety, so pointer comparison won't work
	if client.config == nil {
		t.Error("Config not properly set")
	}

	// Verify config values are copied correctly
	if client.config.Timeout != config.Timeout {
		t.Error("Timeout not properly copied")
	}
	if client.config.MaxRetries != config.MaxRetries {
		t.Error("MaxRetries not properly copied")
	}
}

func TestBuildConnectionConfig_PlumbsCertPinner(t *testing.T) {
	pinner := &mockEngineCertPinner{}
	config := &Config{
		CertificatePinner: pinner,
		Connection:        *connection.DefaultConfig(),
	}

	connCfg := buildConnectionConfig(config)
	if connCfg.CertPinner != pinner {
		t.Error("buildConnectionConfig did not assign Connection.CertPinner from CertificatePinner")
	}
}

// mockEngineCertPinner implements security.CertificatePinner for the plumbing test.
type mockEngineCertPinner struct{}

func (m *mockEngineCertPinner) Pin() string { return "mock" }
func (m *mockEngineCertPinner) VerifyPeerCertificate([][]byte, [][]*x509.Certificate) error {
	return nil
}

func TestNewClient_InvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{name: "Nil config", config: nil, wantErr: true},
		// The former "Zero timeout falls back to default" row moved to
		// TestClient_TimeoutHandling below (consolidated with the other
		// zero-timeout scaffolds).
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.config)
			if client != nil {
				defer func() { _ = client.Close() }()
			}
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected success, got: %v", err)
			}
			if tt.wantErr && client != nil {
				t.Error("expected nil client on error")
			}
		})
	}
}

// TestClient_HTTPMethods removed - duplicate of TestClient_ConvenienceMethods below

func TestClient_Request(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries: 1,
		UserAgent:  "test-client/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Use a context with timeout to ensure the request doesn't hang
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.Request(ctx, "GET", server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}
}

func TestClient_RequestWithOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check custom header
		if r.Header.Get("X-Test") != "test-value" {
			t.Errorf("Expected X-Test header, got: %s", r.Header.Get("X-Test"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries: 1,
		UserAgent:  "test-client/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Create a request option that adds a header
	headerOption := func(req types.RequestMutator) error {
		req.SetHeader("X-Test", "test-value")
		return nil
	}

	resp, err := client.Request(backgroundCtx, "GET", server.URL, headerOption)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}
}

func TestClient_Close(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries: 1,
		UserAgent:  "test-client/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	// Close should not error
	err = client.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Multiple closes should be safe
	err = client.Close()
	if err != nil {
		t.Errorf("Second close failed: %v", err)
	}
}

// TestClient_Statistics was removed: it asserted totalRequests >= 0 on a
// monotonic counter (could never fail). Request-count tracking is asserted
// with a real request by TestClient_IsHealthy.

func TestClient_ConcurrentRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add small delay to test concurrency
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries: 1,

		UserAgent: "test-client/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Make concurrent requests
	const numRequests = 5
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			_, err := client.Request(backgroundCtx, "GET", server.URL)
			results <- err
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < numRequests; i++ {
		if err := <-results; err != nil {
			t.Errorf("Concurrent request failed: %v", err)
		}
	}
}

func TestClient_TLSConfig(t *testing.T) {
	// Create HTTPS test server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries: 1,
		UserAgent:  "test-client/1.0", Connection: connection.Config{AllowPrivateIPs: true,

			TLSConfig: &tls.Config{
				InsecureSkipVerify: true, // For testing only
			},
			InsecureSkipVerify: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Request(backgroundCtx, "GET", server.URL)
	if err != nil {
		t.Fatalf("HTTPS request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}
}

// TestClient_ContextCancellation removed (early version) - the surviving
// test of the same name lives further down in this file.

// TestClient_InvalidURL removed - duplicate of TestClient_ErrorHandling in client_test.go

// TestClient_LargeResponse was removed: the "Large response" row of
// TestClient_ResponseProcessing drives the same 1MB round trip through the
// same pipeline (and TestResponseProcessor_LargeChunkedBody pins exact-byte
// behavior at the processor level).

func TestClient_ConvenienceMethods(t *testing.T) {
	methods := []struct {
		name   string
		method string
	}{
		{"Post", "POST"},
		{"Put", "PUT"},
		{"Patch", "PATCH"},
		{"Delete", "DELETE"},
		{"Head", "HEAD"},
		{"Options", "OPTIONS"},
	}

	for _, tt := range methods {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			config := &Config{
				Timeout: 30 * time.Second,

				MaxRetries: 0,
				UserAgent:  "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
			}
			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("NewClient failed: %v", err)
			}
			defer func() { _ = client.Close() }()

			resp, err := client.Request(backgroundCtx, tt.method, server.URL)
			if err != nil {
				t.Fatalf("%s failed: %v", tt.name, err)
			}
			if resp.StatusCode() != http.StatusOK {
				t.Errorf("Status = %d, want 200", resp.StatusCode())
			}
			if gotMethod != tt.method {
				t.Errorf("Method = %q, want %q", gotMethod, tt.method)
			}
		})
	}
}

// TestClient_RequestSucceedsWithoutMetrics was removed: TestClient_Request
// drives the identical 200-OK round trip (the metrics-counters rationale died
// with the counters).

func TestClient_OnRequestOnResponse(t *testing.T) {
	var onRequestVal, onResponseVal bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries: 0,
		UserAgent:  "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	onReqOption := func(req types.RequestMutator) error {
		req.(*Request).SetOnRequest(func(r *Request) error {
			onRequestVal = true
			return nil
		})
		return nil
	}
	onRespOption := func(req types.RequestMutator) error {
		req.(*Request).SetOnResponse(func(r *Response) error {
			onResponseVal = true
			return nil
		})
		return nil
	}

	_, err = client.Request(ctx, "GET", server.URL, onReqOption, onRespOption)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if !onRequestVal {
		t.Error("OnRequest callback was not called")
	}
	if !onResponseVal {
		t.Error("OnResponse callback was not called")
	}
}

// TestClient_ResponseProcessing validates response handling for various server responses.
func TestClient_ResponseProcessing(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		validate       func(*testing.T, *Response)
	}{
		{
			name: "JSON response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"message":"success","code":200}`))
			},
			validate: func(t *testing.T, resp *Response) {
				if resp.StatusCode() != 200 {
					t.Errorf("Expected status 200, got %d", resp.StatusCode())
				}
				if !strings.Contains(resp.Body(), "success") {
					t.Error("Response body doesn't contain expected content")
				}
				if len(resp.RawBody()) == 0 {
					t.Error("RawBody should not be empty")
				}
			},
		},
		{
			name: "Error response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid request"}`))
			},
			validate: func(t *testing.T, resp *Response) {
				if resp.StatusCode() != 400 {
					t.Errorf("Expected status 400, got %d", resp.StatusCode())
				}
				if !strings.Contains(resp.Body(), "error") {
					t.Error("Response body doesn't contain error message")
				}
			},
		},
		{
			name: "Large response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				// Write large amount of data
				data := strings.Repeat("A", 1024*1024) // 1MB
				_, _ = w.Write([]byte(data))
			},
			validate: func(t *testing.T, resp *Response) {
				if resp.StatusCode() != 200 {
					t.Errorf("Expected status 200, got %d", resp.StatusCode())
				}
				if len(resp.RawBody()) < 1024*1024 {
					t.Error("Large response not handled correctly")
				}
			},
		},
		{
			name: "Response with cookies",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				http.SetCookie(w, &http.Cookie{
					Name:  "session_id",
					Value: "abc123",
					Path:  "/",
				})
				http.SetCookie(w, &http.Cookie{
					Name:  "user_pref",
					Value: "dark_mode",
					Path:  "/",
				})
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			},
			validate: func(t *testing.T, resp *Response) {
				cookies := resp.Cookies()
				if len(cookies) != 2 {
					t.Errorf("Expected 2 cookies, got %d", len(cookies))
				}

				foundSession := false
				foundPref := false
				for _, cookie := range cookies {
					if cookie.Name == "session_id" && cookie.Value == "abc123" {
						foundSession = true
					}
					if cookie.Name == "user_pref" && cookie.Value == "dark_mode" {
						foundPref = true
					}
				}

				if !foundSession {
					t.Error("Session cookie not found")
				}
				if !foundPref {
					t.Error("Preference cookie not found")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			config := &Config{
				Timeout: 60 * time.Second,

				ValidateURL:     true,
				ValidateHeaders: true, Connection: connection.Config{AllowPrivateIPs: true}, // Allow test server access
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer func() { _ = client.Close() }()

			resp, err := client.Request(backgroundCtx, "GET", server.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			tt.validate(t, resp)
		})
	}
}

// TestClient_ErrorHandling validates error handling for various failure scenarios.
func TestClient_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		setupServer func() *httptest.Server
		expectError bool
		errorCheck  func(*testing.T, error)
	}{
		{
			name: "Connection refused",
			setupServer: func() *httptest.Server {
				// Return a closed server
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				server.Close() // Close immediately
				return server
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected connection error, got nil")
				}
			},
		},
		{
			name: "Server timeout",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(2 * time.Second) // Exceed client timeout
					w.WriteHeader(http.StatusOK)
				}))
			},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected timeout error, got nil")
				}
			},
		},
		{
			name: "Invalid response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Send invalid HTTP response
					w.Header().Set("Content-Length", "100")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("short")) // Content length mismatch
				}))
			},
			expectError: true, // Our enhanced security now detects this
			errorCheck: func(t *testing.T, err error) {
				if err == nil {
					t.Error("Expected content-length mismatch error, got nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := tt.setupServer()
			if server != nil {
				defer server.Close()
			}

			config := &Config{
				Timeout: 1 * time.Second, // Short timeout for testing

				ValidateURL:     true,
				ValidateHeaders: true,
				// Allow test server access
				StrictContentLength: true, Connection: // Enable strict content-length validation
				connection.Config{AllowPrivateIPs: true},
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer func() { _ = client.Close() }()

			_, err = client.Request(backgroundCtx, "GET", server.URL)

			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			if tt.errorCheck != nil {
				tt.errorCheck(t, err)
			}
		})
	}
}

// TestClient_ContextCancellation validates that context cancellation aborts requests promptly.
func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Long processing time
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true, Connection: connection.Config{AllowPrivateIPs: true}, // Allow test server access
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = client.Request(ctx, "GET", server.URL)
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}

	if duration > 1*time.Second {
		t.Errorf("Request took too long to cancel: %v", duration)
	}

	t.Logf("Context cancellation worked correctly in %v", duration)
}

func TestClient_OnResponseErrorReleasesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test body")) // best-effort test response
	}))
	defer server.Close()

	client, err := NewClient(&Config{
		FollowRedirects: true,
	}, withMockTransport(newMockTransport(200, "test response body")))
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	// onResponse callback that returns an error
	onRespOption := func(req types.RequestMutator) error {
		req.(*Request).SetOnResponse(func(r *Response) error {
			return fmt.Errorf("simulated callback error")
		})
		return nil
	}

	// This should return error but NOT leak the pooled Response
	_, err = client.Request(backgroundCtx, "GET", server.URL, onRespOption)
	if err == nil {
		t.Fatal("expected error from onResponse callback")
	}
	if !strings.Contains(err.Error(), "onResponse callback failed") {
		t.Errorf("unexpected error: %v", err)
	}

	// Verify the response pool is still functional by making another request
	resp, err := client.Request(backgroundCtx, "GET", server.URL)
	if err != nil {
		t.Fatalf("subsequent request failed: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode())
	}
}

func TestClient_SetRawBodyReader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "body=%s,content-type=%s", string(body), r.Header.Get("Content-Type")) // best-effort test response
	}))
	defer server.Close()

	cfg := &Config{
		Timeout: 30 * time.Second,

		MaxRetries: 1,
		UserAgent:  "test-client/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Request(backgroundCtx, "GET", server.URL)
	if err != nil {
		t.Fatalf("Request error: %v", err)
	}

	// Test SetRawBodyReader on the response
	originalReader := resp.RawBodyReader()

	// Create a new io.ReadCloser and set it
	newReader := io.NopCloser(strings.NewReader("replacement body"))
	resp.SetRawBodyReader(newReader)

	// Verify it was set
	gotReader := resp.RawBodyReader()
	if gotReader != newReader {
		t.Error("expected RawBodyReader to be the new reader after SetRawBodyReader")
	}

	// Test setting to nil (ownership transfer scenario)
	resp.SetRawBodyReader(nil)
	if resp.RawBodyReader() != nil {
		t.Error("expected RawBodyReader to be nil after SetRawBodyReader(nil)")
	}

	// Restore original to avoid leak if it was non-nil
	_ = originalReader
}

// TestRequestApply verifies Apply forwards every per-request mutable field
// and transfers ownership of the pooled maps (src's references cleared).
func TestRequestApply(t *testing.T) {
	src := &Request{}
	src.SetMethod("POST")
	src.SetURL("https://api.example.com/apply")
	src.SetHeader("X-Test", "value")
	src.EnsureQueryParams()["q"] = "1"
	src.SetBody(map[string]string{"k": "v"})
	src.SetTimeout(7 * time.Second)
	src.SetMaxRetries(3)
	src.SetCookies([]http.Cookie{{Name: "c", Value: "v"}})
	fr, mr, ap := true, 4, true
	src.SetFollowRedirects(&fr)
	src.SetMaxRedirects(&mr)
	src.SetAllowPrivateIPs(&ap)
	src.SetStreamBody(true)

	dst := &Request{}
	dst.Apply(src)

	if dst.Method() != "POST" || dst.URL() != "https://api.example.com/apply" {
		t.Errorf("method/url not forwarded: %s %s", dst.Method(), dst.URL())
	}
	if dst.Headers()["X-Test"] != "value" {
		t.Errorf("headers not forwarded: %v", dst.Headers())
	}
	if dst.QueryParams()["q"] != "1" {
		t.Errorf("query params not forwarded: %v", dst.QueryParams())
	}
	if dst.Body() == nil || dst.Timeout() != 7*time.Second || dst.MaxRetries() != 3 {
		t.Errorf("body/timeout/retries not forwarded: %v %v %v", dst.Body(), dst.Timeout(), dst.MaxRetries())
	}
	if len(dst.Cookies()) != 1 || dst.Cookies()[0].Name != "c" {
		t.Errorf("cookies not forwarded: %v", dst.Cookies())
	}
	if dst.FollowRedirects() == nil || !*dst.FollowRedirects() {
		t.Error("followRedirects not forwarded")
	}
	if dst.MaxRedirects() == nil || *dst.MaxRedirects() != 4 {
		t.Error("maxRedirects not forwarded")
	}
	if dst.AllowPrivateIPs() == nil || !*dst.AllowPrivateIPs() {
		t.Error("allowPrivateIPs not forwarded")
	}
	if !dst.StreamBody() {
		t.Error("streamBody not forwarded")
	}

	// Ownership transfer: pooled maps must be detached from src so a later
	// release of src cannot double-pool them.
	if src.Headers() != nil || src.QueryParams() != nil {
		t.Errorf("src pooled maps not cleared: headers=%v queryParams=%v", src.Headers(), src.QueryParams())
	}
	// dst still sees the transferred maps.
	if dst.Headers()["X-Test"] != "value" {
		t.Error("headers lost after ownership transfer")
	}
}

// TestExecuteWithRetry_ProxyRotationPaths covers the proxy-rotation branches
// of executeWithRetry that plain mock-transport retry tests cannot reach:
// recorder attachment + NextProxyIndex reservation in both the no-retry fast
// path and the retry loop, and per-attempt WithProxyAttempt context wiring.
// The mock transport bypasses the real Proxy callback, so ProxyURL stays
// empty — the branches, not the value, are the point here.
func TestExecuteWithRetry_ProxyRotationPaths(t *testing.T) {
	newPoolCfg := func(maxRetries int) *Config {
		return &Config{
			Timeout: 5 * time.Second,

			MaxRetries: maxRetries,
			RetryDelay: time.Millisecond,

			ProxyRotatePerRequest: true, Connection: connection.Config{AllowPrivateIPs: true,

				ProxyPool: []string{"http://p1.example.com:8080", "http://p2.example.com:8080"}},
		}
	}

	t.Run("fast path attaches recorder and reserves index", func(t *testing.T) {
		mt := newMockTransport(200, "ok")
		client, err := NewClient(newPoolCfg(0), withMockTransport(mt))
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer func() { _ = client.Close() }()

		resp, err := client.Request(context.Background(), "GET", "https://example.com")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.Attempts() != 1 {
			t.Errorf("Attempts = %d, want 1 (no-retry fast path)", resp.Attempts())
		}
		ReleaseResponse(resp)
	})

	t.Run("retry loop rotates proxy index across attempts", func(t *testing.T) {
		mt := newMockTransport(200, "ok")
		mt.failFirst = 2 // two retryable failures, success on attempt 3
		client, err := NewClient(newPoolCfg(2), withMockTransport(mt))
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer func() { _ = client.Close() }()

		resp, err := client.Request(context.Background(), "GET", "https://example.com")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.Attempts() != 3 {
			t.Errorf("Attempts = %d, want 3", resp.Attempts())
		}
		if mt.GetCallCount() != 3 {
			t.Errorf("transport calls = %d, want 3", mt.GetCallCount())
		}
		ReleaseResponse(resp)
	})
}

// TestExecuteWithRetry_CustomPolicyVetoAndSleepError covers the two
// custom-policy exits of the retry loop: a policy that vetoes a retryable
// error (ShouldRetry=false → immediate return), and a sleep aborted by the
// request deadline during the inter-attempt delay.
func TestExecuteWithRetry_CustomPolicyVetoAndSleepError(t *testing.T) {
	t.Run("policy vetoes retryable error", func(t *testing.T) {
		mt := newMockTransport(200, "ok")
		mt.failFirst = 1 // retryable connection-reset, but policy says stop
		client, err := NewClient(&Config{
			Timeout: 5 * time.Second,

			MaxRetries:        2,
			CustomRetryPolicy: &testRetryPolicy{maxRetries: 0, delay: time.Millisecond}, Connection: connection.Config{AllowPrivateIPs: true},
		}, withMockTransport(mt))
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer func() { _ = client.Close() }()

		_, err = client.Request(context.Background(), "GET", "https://example.com")
		if err == nil {
			t.Fatal("expected error: policy must stop after first failure")
		}
		if mt.GetCallCount() != 1 {
			t.Errorf("transport calls = %d, want 1 (veto honored)", mt.GetCallCount())
		}
	})

	t.Run("sleep aborted by request deadline", func(t *testing.T) {
		mt := newMockTransport(200, "ok")
		mt.failFirst = 1
		client, err := NewClient(&Config{
			Timeout: 30 * time.Second, // > ctx deadline: ctx deadline wins

			MaxRetries: 1,
			CustomRetryPolicy: &testRetryPolicy{
				maxRetries: 1,
				delay:      500 * time.Millisecond, // longer than the ctx deadline
			}, Connection: connection.Config{AllowPrivateIPs: true},
		}, withMockTransport(mt))
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer func() { _ = client.Close() }()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err = client.Request(ctx, "GET", "https://example.com")
		if err == nil {
			t.Fatal("expected error: sleep must be aborted by the deadline")
		}
	})
}

// TestExecuteWithRetry_CustomPolicyOverridesNonRetryableError pins the M4
// regression: a custom RetryPolicy decides the error path. Previously the
// built-in IsRetryable() veto ran first, so the policy was never consulted
// for errors the classifier marked non-retryable — despite Config docs
// promising "If set, it overrides the built-in retry logic". The built-in
// engine policy delegates to the same classifyError call, so default
// behavior is unchanged (first subtest).
func TestExecuteWithRetry_CustomPolicyOverridesNonRetryableError(t *testing.T) {
	permanentErr := errors.New("permanent failure") // matches no retryable pattern

	t.Run("built-in classifier still stops", func(t *testing.T) {
		mock := newMockTransport(200, "ok")
		mock.SetError(permanentErr)
		client, err := NewClient(&Config{
			Timeout:    5 * time.Second,
			MaxRetries: 2,
			Connection: connection.Config{AllowPrivateIPs: true},
		}, withMockTransport(mock))
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer func() { _ = client.Close() }()

		_, err = client.Request(backgroundCtx, "GET", "https://example.com")
		if err == nil {
			t.Fatal("expected error: non-retryable classification must stop the built-in loop")
		}
		if got := mock.GetCallCount(); got != 1 {
			t.Errorf("transport calls = %d, want 1", got)
		}
	})

	t.Run("custom policy retries what the classifier rejects", func(t *testing.T) {
		mock := newMockTransport(200, "recovered")
		mock.failFirst = 1
		mock.failFirstErr = permanentErr
		client, err := NewClient(&Config{
			Timeout:           5 * time.Second,
			MaxRetries:        2,
			CustomRetryPolicy: &testRetryPolicy{maxRetries: 2, delay: time.Millisecond},
			Connection:        connection.Config{AllowPrivateIPs: true},
		}, withMockTransport(mock))
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer func() { _ = client.Close() }()

		resp, err := client.Request(backgroundCtx, "GET", "https://example.com")
		if err != nil {
			t.Fatalf("custom policy must be allowed to retry: %v", err)
		}
		defer ReleaseResponse(resp)
		if got := mock.GetCallCount(); got != 2 {
			t.Errorf("transport calls = %d, want 2 (policy overrode the classification)", got)
		}
	})
}

// TestSecurityRequestPool_FallbackAndReset covers the defensive branches of
// getSecurityRequest/putSecurityRequest: a wrong-typed pooled value must fall
// back to a fresh security.Request, and put must clear every field so a
// recycled request never leaks prior-request state. Mirrors the poisoned-pool
// tests the other engine pools already have.
func TestSecurityRequestPool_FallbackAndReset(t *testing.T) {
	client, err := NewClient(&Config{Timeout: 30 * time.Second, Connection: connection.Config{AllowPrivateIPs: true}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	t.Run("wrong-typed pooled value falls back to fresh", func(t *testing.T) {
		client.securityRequestPool.Put(42) //nolint:staticcheck // intentional wrong-type poisoning

		req := client.getSecurityRequest()
		if req == nil {
			t.Fatal("getSecurityRequest returned nil from poisoned pool")
		}
		// Must be a usable zero value.
		if req.URL != "" || req.Headers != nil {
			t.Errorf("fallback request not zero-valued: %+v", req)
		}
	})

	t.Run("put clears all fields", func(t *testing.T) {
		req := client.getSecurityRequest()
		req.URL = "https://example.com"
		req.Headers = map[string]string{"X-A": "1"}

		client.putSecurityRequest(req)

		if req.URL != "" || req.Headers != nil {
			t.Errorf("putSecurityRequest did not reset: %+v", req)
		}
	})

	t.Run("put nil is a no-op", func(t *testing.T) {
		client.putSecurityRequest(nil) // must not panic
	})
}

// ============================================================================
// CLIENT POOL OPERATIONS (moved from coverage_test.go)
// ============================================================================

// TestClient_PoolOperations validates get/put operations for internal request pools.
func TestClient_PoolOperations(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		UserAgent: "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	t.Run("Request pool get/put", func(t *testing.T) {
		req := client.getRequest()
		if req == nil {
			t.Error("getRequest returned nil")
		}
		req.SetMethod("GET")
		req.SetURL("https://example.com")

		client.putRequest(req)
		// Should not panic
	})

	t.Run("Security request pool get/put", func(t *testing.T) {
		secReq := client.getSecurityRequest()
		if secReq == nil {
			t.Fatal("getSecurityRequest returned nil")
		}
		secReq.URL = "https://example.com"

		client.putSecurityRequest(secReq)
	})

	t.Run("Exec request pool get/put", func(t *testing.T) {
		execReq := client.getExecRequest()
		if execReq == nil {
			t.Error("getExecRequest returned nil")
		}
		execReq.SetMethod("POST")

		client.putExecRequest(execReq)
	})

	t.Run("Put nil security request", func(t *testing.T) {
		client.putSecurityRequest(nil) // should not panic
	})
}

// ============================================================================
// CLOSED-CLIENT BEHAVIOR
// (merges the former TestClient_IsClosed, TestClient_ClosedRequest, and
// TestClient_CloseWithConnectionPool — one concern, three scaffolds)
// ============================================================================

func TestClient_ClosedBehavior(t *testing.T) {
	t.Run("IsClosed reports open then closed", func(t *testing.T) {
		config := &Config{
			Timeout: 30 * time.Second,

			UserAgent: "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
		}

		client, err := NewClient(config)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		if client.IsClosed() {
			t.Error("New client should not be closed")
		}

		if err := client.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}

		if !client.IsClosed() {
			t.Error("Client should be closed after Close()")
		}
	})

	t.Run("Request after close fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := &Config{
			Timeout: 30 * time.Second,

			UserAgent: "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
		}

		client, err := NewClient(config)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}

		// Close before making request
		_ = client.Close()

		_, err = client.Request(backgroundCtx, "GET", server.URL)
		if err == nil {
			t.Error("Expected error when using closed client")
		}
	})

	t.Run("Close with active connection pool", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
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

		// Make a request first to establish connections
		_, _ = client.Request(backgroundCtx, "GET", server.URL)

		// Close should clean up transport and connection pool
		err = client.Close()
		if err != nil {
			t.Errorf("Close failed: %v", err)
		}
	})
}

// ============================================================================
// REDIRECTS (moved from coverage_test.go)
// ============================================================================

// TestClient_Redirects is the consolidated redirect behavior table (formerly
// six standalone tests: RedirectFollowing, NoRedirectFollowing,
// MultipleRedirects, MaxRedirectLimit, CircularRedirect, SSRSRedirectBlocked —
// identical scaffolds differing only in handler, config, and expectation).
func TestClient_Redirects(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		follow        bool
		maxRedirects  int
		blockPrivate  bool // SSRF protection on: redirect targets are validated
		path          string
		wantStatus    int    // asserted when wantErr is false
		wantBody      string // asserted when non-empty
		wantRedirects int    // asserted when wantErr is false
		wantErr       bool
		errContains   string // asserted when wantErr is true
	}{
		{
			name: "single redirect is followed",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/redirect":
					http.Redirect(w, r, "/final", http.StatusFound)
				case "/final":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("final destination"))
				}
			},
			follow: true, path: "/redirect",
			wantStatus: 200, wantBody: "final destination", wantRedirects: 1,
		},
		{
			name: "following disabled returns 302 as-is",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/final", http.StatusFound)
			},
			follow: false, path: "/redirect",
			wantStatus: http.StatusFound,
		},
		{
			name: "multi-hop chain with mixed statuses",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/start":
					http.Redirect(w, r, "/r1", http.StatusMovedPermanently)
				case "/r1":
					http.Redirect(w, r, "/r2", http.StatusFound)
				case "/r2":
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("final"))
				}
			},
			follow: true, path: "/start",
			wantStatus: 200, wantBody: "final", wantRedirects: 2,
		},
		{
			name: "max redirect limit exceeded errors",
			handler: func() http.HandlerFunc {
				count := 0
				return func(w http.ResponseWriter, r *http.Request) {
					count++
					if count <= 15 {
						http.Redirect(w, r, fmt.Sprintf("/redirect/%d", count), http.StatusFound)
						return
					}
					w.WriteHeader(http.StatusOK)
				}
			}(),
			follow: true, maxRedirects: 3, path: "/start",
			wantErr: true, errContains: "redirect",
		},
		{
			name: "circular redirect is detected",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/a":
					http.Redirect(w, r, "/b", http.StatusFound)
				case "/b":
					http.Redirect(w, r, "/a", http.StatusFound)
				default:
					w.WriteHeader(http.StatusOK)
				}
			},
			follow: true, path: "/a", // maxRedirects 0 = default limit
			wantErr: true, errContains: "circular",
		},
		{
			name: "redirect to private IP blocked under SSRF protection",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirect-to-localhost" {
					http.Redirect(w, r, "http://127.0.0.1:1/blocked", http.StatusFound)
					return
				}
				w.WriteHeader(http.StatusOK)
			},
			follow: true, blockPrivate: true, path: "/redirect-to-localhost",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			config := &Config{
				Timeout: 30 * time.Second,

				FollowRedirects: tt.follow,
				MaxRedirects:    tt.maxRedirects,
				MaxRetries:      0,
				UserAgent:       "test/1.0", Connection: connection.Config{AllowPrivateIPs: !tt.blockPrivate},
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer func() { _ = client.Close() }()

			resp, err := client.Request(backgroundCtx, "GET", server.URL+tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			defer ReleaseResponse(resp)
			if resp.StatusCode() != tt.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), tt.wantStatus)
			}
			if tt.wantBody != "" && resp.Body() != tt.wantBody {
				t.Errorf("body = %q, want %q", resp.Body(), tt.wantBody)
			}
			if resp.RedirectCount() != tt.wantRedirects {
				t.Errorf("redirect count = %d, want %d", resp.RedirectCount(), tt.wantRedirects)
			}
		})
	}
}

// ============================================================================
// OPTION / CALLBACK ERROR PROPAGATION (moved from coverage_test.go)
// ============================================================================

// TestClient_OnRequestError validates that OnRequest callback errors propagate.
func TestClient_OnRequestError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
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

	errOption := func(req types.RequestMutator) error {
		req.(*Request).SetOnRequest(func(r *Request) error {
			return fmt.Errorf("callback error")
		})
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.Request(ctx, "GET", server.URL, errOption)
	if err == nil {
		t.Error("Expected error from OnRequest callback")
	}
}

// TestClient_NilOption validates that nil options are skipped gracefully.
func TestClient_NilOption(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var nilOption RequestOption = nil
	resp, err := client.Request(ctx, "GET", server.URL, nilOption)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}
}

// TestClient_OptionError validates that option errors propagate.
func TestClient_OptionError(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	failOption := func(req types.RequestMutator) error {
		return fmt.Errorf("option failed")
	}

	_, err = client.Request(ctx, "GET", "https://example.com", failOption)
	if err == nil {
		t.Error("Expected error from failed option")
	}
}

// TestClient_PutWithBody was removed: the "Put" row of
// TestClient_ConvenienceMethods covers the PUT method round trip; body
// delivery through options is pinned by TestFastPath_ReaderBodyClosedUnderSizeLimit
// and the Build body-serialization table.

// ============================================================================
// COOKIE JAR INTEGRATION (moved from coverage_test.go)
// ============================================================================

// TestClient_WithCookieJar validates cookie jar integration.
func TestClient_WithCookieJar(t *testing.T) {
	jar := newTestCookieJar()

	var mu sync.Mutex            // protects receivedCookies (written in handler goroutine)
	var receivedCookies []string // cookie names seen by the server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		for _, c := range r.Cookies() {
			receivedCookies = append(receivedCookies, c.Name)
		}
		mu.Unlock()
		// Set a cookie on every response
		http.SetCookie(w, &http.Cookie{
			Name:  "server-cookie",
			Value: "server-value",
			Path:  "/",
		})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries:    0,
		UserAgent:     "test/1.0",
		EnableCookies: true, Connection: connection.Config{AllowPrivateIPs: true,

			CookieJar: jar},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	// First request: send a manual cookie, server sets server-cookie via jar.
	cookieOption := func(req types.RequestMutator) error {
		req.SetCookies([]http.Cookie{
			{Name: "manual-cookie", Value: "manual-value"},
		})
		return nil
	}

	resp, err := client.Request(backgroundCtx, "GET", server.URL, cookieOption)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}

	// Second request: jar should replay server-cookie from the first response.
	resp2, err := client.Request(backgroundCtx, "GET", server.URL)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	if resp2.StatusCode() != 200 {
		t.Errorf("Expected status 200 on second request, got %d", resp2.StatusCode())
	}

	mu.Lock()
	defer mu.Unlock()

	// The manual cookie from the first request must have been received.
	foundManual := false
	for _, name := range receivedCookies {
		if name == "manual-cookie" {
			foundManual = true
		}
	}
	if !foundManual {
		t.Error("server never received manual-cookie from first request")
	}

	// The server-cookie set on the first response must be replayed by the jar
	// on the second request (proves cookie jar persistence).
	serverCookieCount := 0
	for _, name := range receivedCookies {
		if name == "server-cookie" {
			serverCookieCount++
		}
	}
	// First request: manual-cookie sent. Second request: server-cookie replayed.
	// So server-cookie should appear at least once (on the second request).
	if serverCookieCount == 0 {
		t.Error("cookie jar did not replay server-cookie on second request")
	}
}

// ============================================================================
// TIMEOUT HANDLING
// (consolidates the former TestClient_ZeroTimeout,
// TestClient_RequestTimeoutOverride, and the "Zero timeout" row of
// TestNewClient_InvalidConfig — three scaffolds, one table)
// ============================================================================

func TestClient_TimeoutHandling(t *testing.T) {
	defaultHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}

	tests := []struct {
		name          string
		configTimeout time.Duration
		request       bool             // run a request and assert 200
		handler       http.HandlerFunc // nil = the default instant 200 handler
		option        RequestOption    // optional per-request option
	}{
		{name: "zero timeout config is accepted by the constructor", configTimeout: 0},
		{name: "zero timeout request completes without a deadline", configTimeout: 0, request: true},
		{
			name:          "per-request timeout override via option",
			configTimeout: 30 * time.Second,
			request:       true,
			option: func(req types.RequestMutator) error {
				req.SetTimeout(5 * time.Second)
				return nil
			},
		},
		{
			// NoTimeout must opt out of the client-level budget end to end:
			// without it this row is the timeout scenario of
			// TestErrorHandling_TimeoutScenarios and fails.
			name:          "NoTimeout opts out of the client deadline",
			configTimeout: 100 * time.Millisecond,
			request:       true,
			handler: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(300 * time.Millisecond) // outlasts the configured budget
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			},
			option: func(req types.RequestMutator) error {
				req.(*Request).SetNoTimeout(true)
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.handler
			if handler == nil {
				handler = defaultHandler
			}
			srv := httptest.NewServer(handler)
			defer srv.Close()

			config := &Config{
				Timeout: tt.configTimeout, // Zero timeout - no deadline

				MaxRetries: 0,
				UserAgent:  "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
			}

			client, err := NewClient(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer func() { _ = client.Close() }()

			if !tt.request {
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var options []RequestOption
			if tt.option != nil {
				options = append(options, tt.option)
			}

			resp, err := client.Request(ctx, "GET", srv.URL, options...)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if resp.StatusCode() != 200 {
				t.Errorf("Expected status 200, got %d", resp.StatusCode())
			}
		})
	}
}

// TestEffectiveRequestTimeout_NoTimeout pins the per-request timeout
// resolution order, including the previously untested NoTimeout branch:
// NoTimeout wins over everything, then the per-request timeout, then the
// client-level config timeout.
func TestEffectiveRequestTimeout_NoTimeout(t *testing.T) {
	c := &Client{config: &Config{Timeout: 30 * time.Second}}
	req := &Request{}

	if got := effectiveRequestTimeout(c, req); got != 30*time.Second {
		t.Errorf("default: effectiveRequestTimeout = %v, want client 30s", got)
	}

	req.SetTimeout(5 * time.Second)
	if got := effectiveRequestTimeout(c, req); got != 5*time.Second {
		t.Errorf("per-request override: effectiveRequestTimeout = %v, want 5s", got)
	}

	req.SetNoTimeout(true)
	if got := effectiveRequestTimeout(c, req); got != 0 {
		t.Errorf("NoTimeout: effectiveRequestTimeout = %v, want 0 (no budget)", got)
	}

	// NoTimeout wins even when an explicit per-request timeout is set.
	req.SetTimeout(7 * time.Second)
	if got := effectiveRequestTimeout(c, req); got != 0 {
		t.Errorf("NoTimeout + Timeout: effectiveRequestTimeout = %v, want 0", got)
	}

	req.SetNoTimeout(false)
	if got := effectiveRequestTimeout(c, req); got != 7*time.Second {
		t.Errorf("NoTimeout cleared: effectiveRequestTimeout = %v, want 7s", got)
	}
}

// ============================================================================
// EXECUTE-WITH-RETRY BOUNDARY CONDITIONS
// ============================================================================

// infiniteReader is an io.Reader that never runs out of bytes, for driving
// the retry-body size cap without allocating the source data.
type infiniteReader struct{}

func (infiniteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

// TestExecuteWithRetry_StreamingBodyOverRetryBufferLimit pins the
// maxRetryBodySize cap (client.go): a retryable request whose io.Reader body
// exceeds 100MB must fail loudly with "retry not supported for streaming
// bodies" instead of buffering the whole stream into memory for replay.
// The body itself costs no source allocation (infiniteReader); only the
// capped 100MB+1 read buffer is transient.
func TestExecuteWithRetry_StreamingBodyOverRetryBufferLimit(t *testing.T) {
	mock := newMockTransport(200, "never reached")

	client, err := NewClient(&Config{
		Timeout:       30 * time.Second,
		MaxRetries:    1,
		RetryDelay:    time.Millisecond,
		BackoffFactor: 1.0,
		UserAgent:     "test/1.0",
		Connection:    connection.Config{AllowPrivateIPs: true},
	}, withMockTransport(mock))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	bodyOption := func(r types.RequestMutator) error {
		r.SetBody(infiniteReader{})
		return nil
	}

	_, err = client.Request(backgroundCtx, "GET", "https://example.com", bodyOption)
	if err == nil {
		t.Fatal("expected error for over-limit retry body, got nil")
	}
	if !strings.Contains(err.Error(), "retry not supported for streaming bodies exceeding") {
		t.Errorf("error should carry the streaming-body cap message, got: %v", err)
	}
	if got := mock.GetCallCount(); got != 0 {
		t.Errorf("transport calls = %d, want 0 (fail before the first attempt)", got)
	}
}

// readSpyReader records whether the retry loop touched the body reader.
type readSpyReader struct {
	reads int
}

func (r *readSpyReader) Read(p []byte) (int, error) {
	r.reads++
	copy(p, "payload")
	return len("payload"), nil
}

// TestExecuteWithRetry_PreCancelledContextSkipsBodyBuffering pins the
// fail-fast check before the up-to-100MB retry-body buffering (client.go):
// an already-cancelled context must abort with the context error BEFORE any
// body byte is read.
func TestExecuteWithRetry_PreCancelledContextSkipsBodyBuffering(t *testing.T) {
	mock := newMockTransport(200, "never reached")

	client, err := NewClient(&Config{
		Timeout:       30 * time.Second,
		MaxRetries:    1,
		RetryDelay:    time.Millisecond,
		BackoffFactor: 1.0,
		UserAgent:     "test/1.0",
		Connection:    connection.Config{AllowPrivateIPs: true},
	}, withMockTransport(mock))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	body := &readSpyReader{}
	bodyOption := func(r types.RequestMutator) error {
		r.SetBody(body)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled before the request

	_, err = client.Request(ctx, "GET", "https://example.com", bodyOption)
	if err == nil {
		t.Fatal("expected error for pre-cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error should unwrap to context.Canceled, got: %v", err)
	}
	if body.reads != 0 {
		t.Errorf("body was read %d times before the cancellation check; want 0", body.reads)
	}
	if got := mock.GetCallCount(); got != 0 {
		t.Errorf("transport calls = %d, want 0", got)
	}
}

// TestExecuteWithRetry_NilResponseFromTransport drives a (nil, nil) RoundTrip
// result — a contract violation only a caller-injected transport can produce —
// and pins that it surfaces as an error instead of panicking. The buffered-path
// defer once dereferenced httpResp.Body before the defensive "no response from
// transport" guard in executeWithRetry could run; executeRequest now fails
// immediately on a nil httpResp.
func TestExecuteWithRetry_NilResponseFromTransport(t *testing.T) {
	mock := &mockTransport{} // Response nil, Error nil → RoundTrip returns (nil, nil)

	client, err := NewClient(&Config{
		Timeout:    30 * time.Second,
		MaxRetries: 0,
		UserAgent:  "test/1.0",
		Connection: connection.Config{AllowPrivateIPs: true},
	}, withMockTransport(mock))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	_, err = client.Request(backgroundCtx, "GET", "https://example.com")
	if err == nil {
		t.Fatal("expected error for (nil, nil) transport result, got nil")
	}
	if !strings.Contains(err.Error(), "no response from transport") {
		t.Errorf("error should name the missing response, got: %v", err)
	}
}

// TestStreamingResponse_ContentEncodingRejected pins the streaming guard
// (client.go): streaming requests advertise Accept-Encoding: identity, so a
// server that compresses anyway must fail loudly — handing raw compressed
// bytes to the caller would silently persist a gzip archive under the
// original filename.
func TestStreamingResponse_ContentEncodingRejected(t *testing.T) {
	mock := newMockTransport(200, "compressed bytes")
	mock.Response.Header.Set("Content-Encoding", "gzip")

	client, err := NewClient(&Config{
		Timeout:    30 * time.Second,
		MaxRetries: 0,
		UserAgent:  "test/1.0",
		Connection: connection.Config{AllowPrivateIPs: true},
	}, withMockTransport(mock))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = client.Close() }()

	streamOption := func(r types.RequestMutator) error {
		r.SetStreamBody(true)
		return nil
	}

	_, err = client.Request(backgroundCtx, "GET", "https://example.com", streamOption)
	if err == nil {
		t.Fatal("expected error for compressed streaming response, got nil")
	}
	if !strings.Contains(err.Error(), "server ignored Accept-Encoding: identity") {
		t.Errorf("error should explain the ignored identity hint, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Content-Encoding") {
		t.Errorf("error should name the offending Content-Encoding, got: %v", err)
	}
}

// TestClient_PerRequestMaxRetriesZeroDisablesRetry pins the zero-vs-unset
// sentinel contract through the public Request path: SetMaxRetries(0) is an
// EXPLICIT disable (one attempt total even with client MaxRetries=3), while
// an unset value inherits the client's budget (see TestClient_ExecuteRetry_MaxReached).
func TestClient_PerRequestMaxRetriesZeroDisablesRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable) // always retryable
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries:    3,
		RetryDelay:    time.Millisecond,
		BackoffFactor: 1.0,
		UserAgent:     "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	zeroRetriesOption := func(req types.RequestMutator) error {
		req.SetMaxRetries(0) // explicit disable — NOT the unset sentinel
		return nil
	}

	resp, err := client.Request(backgroundCtx, "GET", server.URL, zeroRetriesOption)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer ReleaseResponse(resp)
	if resp.StatusCode() != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 returned as-is", resp.StatusCode())
	}
	if resp.Attempts() != 1 {
		t.Errorf("attempts = %d, want 1 (retries explicitly disabled)", resp.Attempts())
	}
	if attempts != 1 {
		t.Errorf("server hits = %d, want 1", attempts)
	}
}

// ============================================================================
// CLIENT-LEVEL RETRY INTEGRATION
// (moved from retry_test.go — same target: executeWithRetry via Client)
// ============================================================================

// TestClient_RetryOnServerErrors validates retry behavior on server errors.
func TestClient_RetryOnServerErrors(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("success"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries:    3,
		RetryDelay:    50 * time.Millisecond,
		BackoffFactor: 2.0,
		UserAgent:     "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Request(backgroundCtx, "GET", server.URL)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}
	if resp.Attempts() < 3 {
		t.Errorf("Expected at least 3 attempts, got %d", resp.Attempts())
	}
}

// TestClient_OverrideMaxRetries validates per-request retry override.
func TestClient_OverrideMaxRetries(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusOK)
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

	retryOption := func(req types.RequestMutator) error {
		req.SetMaxRetries(2)
		return nil
	}

	resp, err := client.Request(backgroundCtx, "GET", server.URL, retryOption)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}
}

// TestClient_ExecuteRetry_MaxReached validates the full retry exhaustion path.
func TestClient_ExecuteRetry_MaxReached(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	config := &Config{
		Timeout: 5 * time.Second,

		MaxRetries:    2,
		RetryDelay:    10 * time.Millisecond,
		BackoffFactor: 1.5,
		Jitter:        false,
		UserAgent:     "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Request(backgroundCtx, "GET", server.URL)
	// After retries are exhausted, the server must have been hit exactly
	// 1 + MaxRetries times and the last (503) response is returned.
	_ = err
	if wantHits := 3; attempts != wantHits { // 1 initial + 2 retries
		t.Errorf("Expected server to be hit %d times, got %d", wantHits, attempts)
	}
	if resp == nil {
		t.Fatal("expected non-nil response after exhausted retries")
	}
	if resp.StatusCode() != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d after exhausted retries, got %d", http.StatusServiceUnavailable, resp.StatusCode())
	}
}

// TestClient_CustomRetryPolicy validates custom retry policy integration.
func TestClient_CustomRetryPolicy(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		MaxRetries:        0, // Let custom policy decide
		RetryDelay:        10 * time.Millisecond,
		BackoffFactor:     1.0,
		UserAgent:         "test/1.0",
		CustomRetryPolicy: &testRetryPolicy{maxRetries: 3, delay: 10 * time.Millisecond}, Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Request(backgroundCtx, "GET", server.URL)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}
}

// TestClient_CustomRetryPolicy_NegativeMaxRetries verifies that a custom
// policy violating the RetryPolicy contract (MaxRetries < 0) still results in
// the request being executed exactly once. Before the clamp in
// executeWithRetry, a negative value skipped the no-retry fast path and fell
// into a never-entered retry loop, returning "request failed after -4
// attempts" without the request ever being sent.
func TestClient_CustomRetryPolicy_NegativeMaxRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	config := &Config{
		Timeout: 30 * time.Second,

		UserAgent:         "test/1.0",
		CustomRetryPolicy: &testRetryPolicy{maxRetries: -5, delay: time.Millisecond}, Connection: connection.Config{AllowPrivateIPs: true},
	}

	client, err := NewClient(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer func() { _ = client.Close() }()

	resp, err := client.Request(backgroundCtx, "GET", server.URL)
	if err != nil {
		t.Fatalf("Request must still execute with a contract-violating policy, got: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}
	if resp.Attempts() != 1 {
		t.Errorf("Expected exactly 1 attempt, got %d", resp.Attempts())
	}
	if attempts != 1 {
		t.Errorf("Server saw %d requests, want 1", attempts)
	}
}

// TestClient_MockTransportRetry validates retry behavior with mock transport.
// (The former "Non-retryable error" subtest was removed: the "built-in
// classifier still stops" subtest of TestExecuteWithRetry_CustomPolicyOverridesNonRetryableError
// asserts the same contract and additionally pins the transport call count.)
func TestClient_MockTransportRetry(t *testing.T) {
	t.Run("Error then success", func(t *testing.T) {
		mock := newMockTransport(200, "OK")
		mock.failFirst = 1 // first attempt fails, second succeeds
		config := &Config{
			Timeout: 30 * time.Second,

			MaxRetries:    2,
			RetryDelay:    10 * time.Millisecond,
			BackoffFactor: 1.0,
			UserAgent:     "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
		}

		client, err := NewClient(config, withMockTransport(mock))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer func() { _ = client.Close() }()

		resp, err := client.Request(context.Background(), "GET", "https://example.com")
		if err != nil {
			t.Fatalf("expected success after retry, got: %v", err)
		}
		if resp.StatusCode() != 200 {
			t.Errorf("status = %d, want 200", resp.StatusCode())
		}
		if resp.Attempts() != 2 {
			t.Errorf("attempts = %d, want 2 (1 failure + 1 success)", resp.Attempts())
		}
	})

	t.Run("Success without retries", func(t *testing.T) {
		mock := newMockTransport(200, "success")
		config := &Config{
			Timeout: 30 * time.Second,

			MaxRetries:    3,
			RetryDelay:    10 * time.Millisecond,
			BackoffFactor: 1.0,
			UserAgent:     "test/1.0", Connection: connection.Config{AllowPrivateIPs: true},
		}

		client, err := NewClient(config, withMockTransport(mock))
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer func() { _ = client.Close() }()

		ctx := context.Background()
		resp, err := client.Request(ctx, "GET", "https://example.com")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if resp.StatusCode() != 200 {
			t.Errorf("Expected status 200, got %d", resp.StatusCode())
		}
		if resp.Attempts() != 1 {
			t.Errorf("Expected 1 attempt, got %d", resp.Attempts())
		}
	})
}
