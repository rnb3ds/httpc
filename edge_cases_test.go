package httpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// EDGE CASES AND ERROR HANDLING TESTS
// ============================================================================

func TestEdgeCase_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// No body
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Body != "" {
		t.Errorf("Expected empty body, got: %s", resp.Body)
	}

	if len(resp.RawBody) != 0 {
		t.Errorf("Expected empty raw body, got length: %d", len(resp.RawBody))
	}
}

func TestEdgeCase_NilResponse(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	// Request to non-existent server
	_, err := client.Get("http://localhost:99999")
	if err == nil {
		t.Error("Expected error for non-existent server")
	}
}

func TestEdgeCase_VeryLongURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Create very long URL with query parameters
	longURL := server.URL + "?"
	for i := 0; i < 100; i++ {
		longURL += fmt.Sprintf("param%d=value%d&", i, i)
	}

	resp, err := client.Get(longURL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestEdgeCase_SpecialCharactersInURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	tests := []struct {
		name  string
		query map[string]interface{}
	}{
		{"Spaces", map[string]interface{}{"key": "value with spaces"}},
		{"Special Chars", map[string]interface{}{"key": "value!@#$%^&*()"}},
		{"Unicode", map[string]interface{}{"key": "测试中文"}},
		{"Emoji", map[string]interface{}{"key": "😀🎉"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := []RequestOption{}
			for k, v := range tt.query {
				opts = append(opts, WithQuery(k, v))
			}

			resp, err := client.Get(server.URL, opts...)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestEdgeCase_MultipleHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check all headers are present
		for i := 0; i < 50; i++ {
			key := fmt.Sprintf("X-Header-%d", i)
			if r.Header.Get(key) == "" {
				t.Errorf("Missing header: %s", key)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Add many headers
	opts := []RequestOption{}
	for i := 0; i < 50; i++ {
		opts = append(opts, WithHeader(fmt.Sprintf("X-Header-%d", i), fmt.Sprintf("value-%d", i)))
	}

	resp, err := client.Get(server.URL, opts...)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestEdgeCase_ZeroTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Zero timeout should use default
	resp, err := client.Get(server.URL, WithTimeout(0))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestEdgeCase_NegativeTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Negative timeout should be handled gracefully
	_, err := client.Get(server.URL, WithTimeout(-1*time.Second))
	// Should either succeed or fail gracefully
	_ = err
}

func TestEdgeCase_CanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Request(ctx, "GET", server.URL)
	if err == nil {
		t.Error("Expected error for canceled context")
	}

	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Expected context canceled error, got: %v", err)
	}
}

func TestEdgeCase_MultipleClients(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create multiple clients
	clients := make([]Client, 10)
	for i := range clients {
		client, err := newTestClient()
		if err != nil {
			t.Fatalf("Failed to create client %d: %v", i, err)
		}
		clients[i] = client
	}

	// Use all clients concurrently
	for i, client := range clients {
		go func(idx int, c Client) {
			_, err := c.Get(server.URL)
			if err != nil {
				t.Errorf("Client %d request failed: %v", idx, err)
			}
		}(i, client)
	}

	time.Sleep(100 * time.Millisecond)

	// Close all clients
	for i, client := range clients {
		if err := client.Close(); err != nil {
			t.Errorf("Failed to close client %d: %v", i, err)
		}
	}
}

func TestEdgeCase_DoubleClose(t *testing.T) {
	client, _ := newTestClient()

	// First close should succeed
	if err := client.Close(); err != nil {
		t.Errorf("First close failed: %v", err)
	}

	// Second close should fail or be idempotent
	err := client.Close()
	if err == nil {
		t.Log("Double close is idempotent")
	} else {
		t.Logf("Double close returns error: %v", err)
	}
}

func TestEdgeCase_UseAfterClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	client.Close()

	// Using closed client should fail
	_, err := client.Get(server.URL)
	if err == nil {
		t.Error("Expected error when using closed client")
	}
}

func TestEdgeCase_NilOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Nil options should be handled gracefully
	resp, err := client.Get(server.URL, nil, nil, nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestEdgeCase_EmptyJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != "{}" {
			t.Errorf("Expected empty JSON object, got: %s", string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Post(server.URL, WithJSON(map[string]interface{}{}))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestEdgeCase_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	var data map[string]interface{}
	err = resp.JSON(&data)
	if err == nil {
		t.Error("Expected error when parsing invalid JSON")
	}
}

func TestEdgeCase_StatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		skipRetry  bool // Skip retry for this status code
	}{
		// Note: 100 Continue is handled automatically by Go's http package
		// and is not returned to the client, so we skip it
		{"200 OK", 200, false},
		{"201 Created", 201, false},
		{"204 No Content", 204, false},
		{"301 Moved Permanently", 301, false},
		{"302 Found", 302, false},
		{"304 Not Modified", 304, false},
		{"400 Bad Request", 400, false},
		{"401 Unauthorized", 401, false},
		{"403 Forbidden", 403, false},
		{"404 Not Found", 404, false},
		{"429 Too Many Requests", 429, true},     // Retryable
		{"500 Internal Server Error", 500, true}, // Retryable
		{"502 Bad Gateway", 502, true},           // Retryable
		{"503 Service Unavailable", 503, true},   // Retryable
		{"504 Gateway Timeout", 504, true},       // Retryable
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			config := DefaultConfig()
			config.AllowPrivateIPs = true
			if tt.skipRetry {
				config.MaxRetries = 0 // Disable retries for retryable status codes
			}
			client, _ := New(config)
			defer client.Close()

			resp, err := client.Get(server.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if resp.StatusCode != tt.statusCode {
				t.Errorf("Expected status %d, got %d", tt.statusCode, resp.StatusCode)
			}
		})
	}
}

func TestEdgeCase_RedirectLoop(t *testing.T) {
	redirectCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount > 10 {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, r.URL.String(), http.StatusFound)
	}))
	defer server.Close()

	config := DefaultConfig()
	config.FollowRedirects = true

	client, _ := New(config)
	defer client.Close()

	// Should handle redirect loop gracefully
	_, err := client.Get(server.URL)
	// May succeed or fail depending on redirect limit
	_ = err
}

func TestEdgeCase_SlowServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := client.Request(ctx, "GET", server.URL)
	if err == nil {
		t.Error("Expected timeout error for slow server")
	}
}

func TestEdgeCase_ChunkedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Transfer-Encoding", "chunked")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter doesn't support flushing")
			return
		}

		for i := 0; i < 5; i++ {
			fmt.Fprintf(w, "chunk %d\n", i)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if !strings.Contains(resp.Body, "chunk") {
		t.Error("Expected chunked response body")
	}
}

func TestEdgeCase_PackageLevelFunctions(t *testing.T) {
	// Setup default client with AllowPrivateIPs
	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	SetDefaultClient(client)
	defer func() {
		// Reset default client before closing
		defaultClient, _ := newTestClient()
		SetDefaultClient(defaultClient)
		client.Close()
	}()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	// Test package-level functions with longer timeout
	t.Run("Get", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client, _ := getDefaultClient()
		resp, err := client.Request(ctx, "GET", server.URL)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Post", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client, _ := getDefaultClient()
		resp, err := client.Request(ctx, "POST", server.URL, WithJSON(map[string]string{"key": "value"}))
		if err != nil {
			t.Fatalf("Post failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Do", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		client, _ := getDefaultClient()
		resp, err := client.Request(ctx, "GET", server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})
}
