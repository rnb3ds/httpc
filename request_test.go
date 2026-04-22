package httpc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/validation"
)

// ============================================================================
// REQUEST TESTS - Headers, cookies, query params, body, options
// Consolidates: options_test.go, cookie_*.go, request_headers_test.go
// ============================================================================

// ----------------------------------------------------------------------------
// Headers
// ----------------------------------------------------------------------------

func TestRequest_Headers(t *testing.T) {
	t.Run("SingleHeaders", func(t *testing.T) {
		tests := []struct {
			name       string
			key        string
			value      string
			optionFunc func(string, string) RequestOption
		}{
			{"WithHeader", "X-Custom-Header", "custom-value", WithHeader},
			{"AcceptJSON", "Accept", "application/json", WithHeader},
			{"AcceptXML", "Accept", "application/xml", WithHeader},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var gotKey, gotVal string
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					gotKey = tt.key
					gotVal = r.Header.Get(tt.key)
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				client, _ := newTestClient()
				defer client.Close()

				_, err := client.Get(server.URL, tt.optionFunc(tt.key, tt.value))
				if err != nil {
					t.Fatalf("Request failed: %v", err)
				}
				if gotVal != tt.value {
					t.Errorf("Expected %s: %s, got %s", gotKey, tt.value, gotVal)
				}
			})
		}
	})

	t.Run("WithHeaderMap", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Header-1") != "value1" {
				t.Error("Expected X-Header-1: value1")
			}
			if r.Header.Get("X-Header-2") != "value2" {
				t.Error("Expected X-Header-2: value2")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithHeaderMap(map[string]string{
			"X-Header-1": "value1",
			"X-Header-2": "value2",
		}))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithUserAgent", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("User-Agent") != "custom-agent/1.0" {
				t.Errorf("Expected User-Agent: custom-agent/1.0, got %s", r.Header.Get("User-Agent"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithUserAgent("custom-agent/1.0"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("RequestHeadersInspection", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL,
			WithHeader("X-Custom", "test-value"),
			WithHeader("X-Another", "another-value"),
		)
		if err != nil {
			t.Fatal(err)
		}

		if resp.Request == nil || resp.Request.Headers == nil {
			t.Fatal("Request headers not captured")
		}
		if resp.Request.Headers.Get("X-Custom") != "test-value" {
			t.Error("X-Custom header not captured correctly")
		}
		if resp.Request.Headers.Get("X-Another") != "another-value" {
			t.Error("X-Another header not captured correctly")
		}
	})
}

// ----------------------------------------------------------------------------
// Authentication
// ----------------------------------------------------------------------------

func TestRequest_Authentication(t *testing.T) {
	t.Run("WithBasicAuth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok || username != "user" || password != "pass" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL, WithBasicAuth("user", "pass"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		if resp.StatusCode() != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode())
		}
	})

	t.Run("WithBasicAuth_EmptyUsername", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithBasicAuth("", "pass"))
		if err == nil {
			t.Error("Expected error for empty username")
		}
	})

	t.Run("WithBearerToken", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token-123" {
				t.Errorf("Expected Bearer token, got %s", auth)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithBearerToken("test-token-123"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithBearerToken_Empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithBearerToken(""))
		if err == nil {
			t.Error("Expected error for empty token")
		}
	})
}

// ----------------------------------------------------------------------------
// Query Parameters
// ----------------------------------------------------------------------------

func TestRequest_QueryParameters(t *testing.T) {
	t.Run("WithQueryMap", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key1") != "value1" {
				t.Error("Expected key1=value1")
			}
			if r.URL.Query().Get("key2") != "value2" {
				t.Error("Expected key2=value2")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		params := map[string]any{
			"key1": "value1",
			"key2": "value2",
		}
		_, err := client.Get(server.URL, WithQueryMap(params))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithQuery", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("search") != "test query" {
				t.Error("Expected search=test query")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithQuery("search", "test query"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithQueryMap nil", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithQueryMap(nil))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithQueryMap empty", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithQueryMap(map[string]any{}))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// WithBody - Auto-detection and explicit body kinds
// ----------------------------------------------------------------------------

func TestRequest_WithBody(t *testing.T) {
	type TestData struct {
		Message string `json:"message" xml:"message"`
		Code    int    `json:"code" xml:"code"`
	}

	t.Run("AutoDetect_JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		data := TestData{Message: "test", Code: 200}
		_, err := client.Post(server.URL, WithBody(data))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("AutoDetect_String", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
				t.Errorf("Expected Content-Type: text/plain; charset=utf-8, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithBody("plain text body"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("AutoDetect_ByteArray", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/octet-stream" {
				t.Errorf("Expected Content-Type: application/octet-stream, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithBody([]byte("binary data")))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("AutoDetect_FormMap", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Errorf("Expected Content-Type: application/x-www-form-urlencoded, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithBody(map[string]string{"key": "value"}))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("AutoDetect_FormData", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Errorf("Expected Content-Type: multipart/form-data, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		formData := &FormData{
			Fields: map[string]string{"field1": "value1"},
		}
		_, err := client.Post(server.URL, WithBody(formData))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("AutoDetect_Reader", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// io.Reader should NOT set Content-Type automatically
			if r.Header.Get("Content-Type") != "" {
				t.Errorf("Expected no Content-Type, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithBody(strings.NewReader("reader content")))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("Explicit_JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		// Even with string input, explicit JSON should set application/json
		_, err := client.Post(server.URL, WithBody("string as json", BodyJSON))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("Explicit_XML", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/xml" {
				t.Errorf("Expected Content-Type: application/xml, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		data := TestData{Message: "test", Code: 200}
		_, err := client.Post(server.URL, WithBody(data, BodyXML))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("Explicit_Form", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Errorf("Expected Content-Type: application/x-www-form-urlencoded, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithBody(map[string]string{"key": "value"}, BodyForm))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("Explicit_Binary", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/octet-stream" {
				t.Errorf("Expected Content-Type: application/octet-stream, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithBody([]byte("binary"), BodyBinary))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("Explicit_Multipart", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Errorf("Expected Content-Type: multipart/form-data, got %s", r.Header.Get("Content-Type"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		formData := &FormData{
			Fields: map[string]string{"field1": "value1"},
		}
		_, err := client.Post(server.URL, WithBody(formData, BodyMultipart))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("Error_NilBody", func(t *testing.T) {
		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post("http://example.com", WithBody(nil))
		if err == nil {
			t.Error("Expected error for nil body")
		}
	})

	t.Run("Error_FormWrongType", func(t *testing.T) {
		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post("http://example.com", WithBody("not a map", BodyForm))
		if err == nil {
			t.Error("Expected error for wrong type with BodyForm")
		}
	})

	t.Run("Error_BinaryWrongType", func(t *testing.T) {
		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post("http://example.com", WithBody(123, BodyBinary))
		if err == nil {
			t.Error("Expected error for wrong type with BodyBinary")
		}
	})

	t.Run("Error_MultipartWrongType", func(t *testing.T) {
		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post("http://example.com", WithBody(map[string]string{"key": "value"}, BodyMultipart))
		if err == nil {
			t.Error("Expected error for wrong type with BodyMultipart")
		}
	})

	t.Run("AutoDetect_NilByteArray", func(t *testing.T) {
		client, _ := newTestClient()
		defer client.Close()

		var data []byte = nil
		_, err := client.Post("http://example.com", WithBody(data))
		if err == nil {
			t.Error("Expected error for nil byte array")
		}
	})

	t.Run("AutoDetect_NilFormData", func(t *testing.T) {
		client, _ := newTestClient()
		defer client.Close()

		var formData *FormData = nil
		_, err := client.Post("http://example.com", WithBody(formData))
		if err == nil {
			t.Error("Expected error for nil FormData")
		}
	})

	t.Run("AutoDetect_NilFormMap", func(t *testing.T) {
		client, _ := newTestClient()
		defer client.Close()

		var formMap map[string]string = nil
		_, err := client.Post("http://example.com", WithBody(formMap))
		if err == nil {
			t.Error("Expected error for nil form map")
		}
	})
}

// ----------------------------------------------------------------------------
// Note: Cookie tests have been moved to cookie_test.go for better organization
// ----------------------------------------------------------------------------

// ----------------------------------------------------------------------------
// Timeout & Retry Options
// ----------------------------------------------------------------------------

func TestRequest_TimeoutAndRetry(t *testing.T) {
	t.Run("WithMaxRetries", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithMaxRetries(3))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// Combined Options
// ----------------------------------------------------------------------------

func TestRequest_CombinedOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify all options applied
		if r.Header.Get("X-Custom") != "value" {
			t.Error("Header not set")
		}
		if r.URL.Query().Get("param") != "test" {
			t.Error("Query param not set")
		}
		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value != "abc123" {
			t.Error("Cookie not set")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	_, err := client.Get(server.URL,
		WithHeader("X-Custom", "value"),
		WithQuery("param", "test"),
		WithCookie(http.Cookie{Name: "session", Value: "abc123"}),
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
}

// ----------------------------------------------------------------------------
// WithFile
// ----------------------------------------------------------------------------

func TestWithFile(t *testing.T) {
	t.Parallel()

	t.Run("empty field name", func(t *testing.T) {
		opt := WithFile("", "test.txt", []byte("data"))
		err := opt(nil)
		if err == nil {
			t.Error("expected error for empty field name")
		}
	})

	t.Run("empty filename", func(t *testing.T) {
		opt := WithFile("file", "", []byte("data"))
		err := opt(nil)
		if err == nil {
			t.Error("expected error for empty filename")
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		// Filename with path traversal should be rejected by validation
		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post("http://example.com", WithFile("file", "../etc/passwd", []byte("data")))
		if err == nil {
			t.Error("expected error for path traversal filename")
		}
	})

	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Error("expected multipart content type")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithFile("upload", "test.txt", []byte("file content")))
		if err != nil {
			t.Fatalf("WithFile failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// WithContext
// ----------------------------------------------------------------------------

func TestWithContext(t *testing.T) {
	t.Parallel()

	t.Run("nil context error", func(t *testing.T) {
		opt := WithContext(nil)
		err := opt(nil)
		if err == nil {
			t.Error("expected error for nil context")
		}
	})

	t.Run("valid context", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		ctx := context.Background()
		_, err := client.Get(server.URL, WithContext(ctx))
		if err != nil {
			t.Fatalf("Request with context failed: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// WithSecureCookie
// ----------------------------------------------------------------------------

func TestWithSecureCookie(t *testing.T) {
	t.Parallel()

	t.Run("nil config", func(t *testing.T) {
		opt := WithSecureCookie(nil)
		err := opt(nil)
		if err == nil {
			t.Error("expected error for nil config")
		}
	})

	t.Run("insecure cookie rejected", func(t *testing.T) {
		client, _ := newTestClient()
		defer client.Close()

		securityConfig := &validation.CookieSecurityConfig{
			RequireSecure: true,
		}

		_, err := client.Get("http://example.com",
			WithCookie(http.Cookie{Name: "test", Value: "val"}),
			WithSecureCookie(securityConfig),
		)
		if err == nil {
			t.Error("expected error for insecure cookie with strict config")
		}
	})
}

// ----------------------------------------------------------------------------
// WithTimeout Boundaries
// ----------------------------------------------------------------------------

func TestWithTimeout_Boundaries(t *testing.T) {
	t.Parallel()

	t.Run("negative timeout", func(t *testing.T) {
		opt := WithTimeout(-1 * time.Second)
		err := opt(nil)
		if err == nil {
			t.Error("expected error for negative timeout")
		}
	})

	t.Run("exceeds max timeout", func(t *testing.T) {
		opt := WithTimeout(31 * time.Minute)
		err := opt(nil)
		if err == nil {
			t.Error("expected error for exceeding max timeout")
		}
	})

	t.Run("valid timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithTimeout(5*time.Second))
		if err != nil {
			t.Fatalf("valid timeout should work: %v", err)
		}
	})
}

// ----------------------------------------------------------------------------
// queryValueLength type coverage
// ----------------------------------------------------------------------------

func TestQueryValueLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		value  any
		minLen int
	}{
		{"string", "hello", 5},
		{"int", 42, 2},
		{"int64", int64(42), 2},
		{"int32", int32(42), 2},
		{"uint", uint(42), 2},
		{"uint64", uint64(42), 2},
		{"uint32", uint32(42), 2},
		{"float64", 3.14, 1},
		{"float32", float32(3.14), 1},
		{"bool true", true, 4},
		{"bool false", false, 5},
		{"negative int64", int64(-42), 3},
		{"default type", struct{}{}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryValueLength(tt.value)
			if got < tt.minLen {
				t.Errorf("queryValueLength(%v) = %d, expected >= %d", tt.value, got, tt.minLen)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// convertToBinary edge cases
// ----------------------------------------------------------------------------

func TestConvertToBinary(t *testing.T) {
	t.Parallel()

	t.Run("nil byte slice", func(t *testing.T) {
		_, err := convertToBinary([]byte(nil))
		if err == nil {
			t.Error("expected error for nil byte slice")
		}
	})

	t.Run("empty string", func(t *testing.T) {
		_, err := convertToBinary("")
		if err == nil {
			t.Error("expected error for empty string")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		_, err := convertToBinary(42)
		if err == nil {
			t.Error("expected error for wrong type")
		}
	})

	t.Run("valid bytes", func(t *testing.T) {
		got, err := convertToBinary([]byte("data"))
		if err != nil || string(got) != "data" {
			t.Errorf("expected 'data', got %q, err=%v", got, err)
		}
	})

	t.Run("valid string", func(t *testing.T) {
		got, err := convertToBinary("hello")
		if err != nil || string(got) != "hello" {
			t.Errorf("expected 'hello', got %q, err=%v", got, err)
		}
	})
}

// ----------------------------------------------------------------------------
// convertToForm edge cases
// ----------------------------------------------------------------------------

func TestConvertToForm(t *testing.T) {
	t.Parallel()

	t.Run("nil url.Values", func(t *testing.T) {
		_, err := convertToForm(url.Values(nil))
		if err == nil {
			t.Error("expected error for nil url.Values")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		_, err := convertToForm(42)
		if err == nil {
			t.Error("expected error for wrong type")
		}
	})

	t.Run("valid url.Values", func(t *testing.T) {
		v := url.Values{"k": {"v"}}
		got, err := convertToForm(v)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "k=v" {
			t.Errorf("expected 'k=v', got %q", got)
		}
	})
}
