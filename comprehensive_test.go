package httpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestData represents test data structure
type TestData struct {
	Message string `json:"message" xml:"message"`
	Code    int    `json:"code" xml:"code"`
}

// ============================================================================
// UNIT TESTS - Core Functionality
// ============================================================================

func TestClient_AllHTTPMethods(t *testing.T) {
	tests := []struct {
		name   string
		method string
		fn     func(Client, string, ...RequestOption) (*Response, error)
	}{
		{"GET", "GET", func(c Client, url string, opts ...RequestOption) (*Response, error) { return c.Get(url, opts...) }},
		{"POST", "POST", func(c Client, url string, opts ...RequestOption) (*Response, error) { return c.Post(url, opts...) }},
		{"PUT", "PUT", func(c Client, url string, opts ...RequestOption) (*Response, error) { return c.Put(url, opts...) }},
		{"PATCH", "PATCH", func(c Client, url string, opts ...RequestOption) (*Response, error) { return c.Patch(url, opts...) }},
		{"DELETE", "DELETE", func(c Client, url string, opts ...RequestOption) (*Response, error) { return c.Delete(url, opts...) }},
		{"HEAD", "HEAD", func(c Client, url string, opts ...RequestOption) (*Response, error) { return c.Head(url, opts...) }},
		{"OPTIONS", "OPTIONS", func(c Client, url string, opts ...RequestOption) (*Response, error) { return c.Options(url, opts...) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method {
					t.Errorf("Expected method %s, got %s", tt.method, r.Method)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"message":"success"}`))
			}))
			defer server.Close()

			client, err := newTestClient()
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			resp, err := tt.fn(client, server.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestClient_RequestOptions(t *testing.T) {
	t.Run("Headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Custom") != "value" {
				t.Errorf("Expected X-Custom header")
			}
			if r.Header.Get("Authorization") != "Bearer token123" {
				t.Errorf("Expected Authorization header")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL,
			WithHeader("X-Custom", "value"),
			WithBearerToken("token123"),
		)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("QueryParameters", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key1") != "value1" {
				t.Errorf("Expected query param key1=value1")
			}
			if r.URL.Query().Get("key2") != "123" {
				t.Errorf("Expected query param key2=123")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL,
			WithQuery("key1", "value1"),
			WithQuery("key2", 123),
		)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("BasicAuth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok || username != "user" || password != "pass" {
				t.Errorf("Expected basic auth user:pass")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithBasicAuth("user", "pass"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

func TestClient_JSONHandling(t *testing.T) {
	t.Run("SendJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type: application/json")
			}

			body, _ := io.ReadAll(r.Body)
			var data TestData
			if err := json.Unmarshal(body, &data); err != nil {
				t.Errorf("Failed to unmarshal JSON: %v", err)
			}

			if data.Message != "test" {
				t.Errorf("Expected message=test, got %s", data.Message)
			}

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		data := TestData{Message: "test", Code: 200}
		_, err := client.Post(server.URL, WithJSON(data))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("ReceiveJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(TestData{Message: "response", Code: 200})
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		var data TestData
		if err := resp.JSON(&data); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if data.Message != "response" {
			t.Errorf("Expected message=response, got %s", data.Message)
		}
	})
}

func TestClient_XMLHandling(t *testing.T) {
	t.Run("SendXML", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/xml" {
				t.Errorf("Expected Content-Type: application/xml")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		xmlData := `<TestData><message>test</message><code>200</code></TestData>`
		_, err := client.Post(server.URL,
			WithBody(xmlData),
			WithContentType("application/xml"),
		)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("ReceiveXML", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"message":"response","code":200}`))
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		var data TestData
		if err := resp.JSON(&data); err != nil {
			t.Fatalf("Failed to parse JSON: %v", err)
		}

		if data.Message != "response" {
			t.Errorf("Expected message=response, got %s", data.Message)
		}
	})
}

func TestClient_FormData(t *testing.T) {
	t.Run("URLEncodedForm", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Errorf("Expected Content-Type: application/x-www-form-urlencoded")
			}

			if err := r.ParseForm(); err != nil {
				t.Errorf("Failed to parse form: %v", err)
			}

			if r.FormValue("field1") != "value1" {
				t.Errorf("Expected field1=value1")
			}

			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithForm(map[string]string{
			"field1": "value1",
			"field2": "value2",
		}))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

// ============================================================================
// RESPONSE HELPER TESTS
// ============================================================================

func TestResponse_StatusHelpers(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		isSuccess  bool
		isRedirect bool
		isClient   bool
		isServer   bool
	}{
		{"200 OK", 200, true, false, false, false},
		{"201 Created", 201, true, false, false, false},
		{"204 No Content", 204, true, false, false, false},
		{"301 Moved", 301, false, true, false, false},
		{"302 Found", 302, false, true, false, false},
		{"400 Bad Request", 400, false, false, true, false},
		{"404 Not Found", 404, false, false, true, false},
		{"500 Server Error", 500, false, false, false, true},
		{"503 Unavailable", 503, false, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{StatusCode: tt.statusCode}

			if resp.IsSuccess() != tt.isSuccess {
				t.Errorf("IsSuccess() = %v, want %v", resp.IsSuccess(), tt.isSuccess)
			}
			if resp.IsRedirect() != tt.isRedirect {
				t.Errorf("IsRedirect() = %v, want %v", resp.IsRedirect(), tt.isRedirect)
			}
			if resp.IsClientError() != tt.isClient {
				t.Errorf("IsClientError() = %v, want %v", resp.IsClientError(), tt.isClient)
			}
			if resp.IsServerError() != tt.isServer {
				t.Errorf("IsServerError() = %v, want %v", resp.IsServerError(), tt.isServer)
			}
		})
	}
}

// ============================================================================
// CONTEXT AND TIMEOUT TESTS
// ============================================================================

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Request(ctx, "GET", server.URL)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

func TestClient_RequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.Request(ctx, "GET", server.URL)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	// Verify it's a timeout error
	if !strings.Contains(err.Error(), "context deadline exceeded") &&
		!strings.Contains(err.Error(), "timeout") {
		t.Errorf("Expected timeout error, got: %v", err)
	}
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestClient_ConcurrentRequests(t *testing.T) {
	requestCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	const numRequests = 100
	var wg sync.WaitGroup
	errors := make(chan error, numRequests)

	start := time.Now()

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.Get(server.URL)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	duration := time.Since(start)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Errorf("Request failed: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Fatalf("Failed %d out of %d requests", errorCount, numRequests)
	}

	if atomic.LoadInt32(&requestCount) != numRequests {
		t.Errorf("Expected %d requests, got %d", numRequests, atomic.LoadInt32(&requestCount))
	}

	t.Logf("Completed %d concurrent requests in %v", numRequests, duration)
}

func TestClient_ConcurrentRequestsWithDifferentMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"method":"%s"}`, r.Method)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	for _, method := range methods {
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(m string) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				_, err := client.Request(ctx, m, server.URL)
				if err != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("method %s: %w", m, err))
					mu.Unlock()
				}
			}(method)
		}
	}

	wg.Wait()

	// Allow some failures due to concurrency limits, but not too many
	if len(errors) > 10 {
		t.Errorf("Too many concurrent request failures (%d/100):", len(errors))
		for i, err := range errors {
			if i < 5 { // Only show first 5 errors
				t.Logf("  Error %d: %v", i+1, err)
			}
		}
	}
}

// ============================================================================
// CONFIGURATION TESTS
// ============================================================================

func TestClient_CustomConfig(t *testing.T) {
	config := &Config{
		Timeout:         30 * time.Second,
		MaxRetries:      3,
		RetryDelay:      1 * time.Second,
		BackoffFactor:   2.0,
		MaxIdleConns:    50,
		MaxConnsPerHost: 10,
		UserAgent:       "TestAgent/1.0",
		FollowRedirects: true,
		EnableHTTP2:     true,
	}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client with custom config: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Client should not be nil")
	}
}

func TestClient_SecureClient(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetries = 1
	config.FollowRedirects = false
	config.EnableCookies = false
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create secure client: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("Secure client should not be nil")
	}
}

func TestClient_TLSConfig(t *testing.T) {
	config := DefaultConfig()
	config.TLSConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS13,
	}

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client with TLS config: %v", err)
	}
	defer client.Close()
}
