package httpc

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================================================
// REQUEST TESTS - Headers, cookies, query params, body, options
// Consolidates: options_test.go, cookie_*.go, request_headers_test.go
// ============================================================================

// ----------------------------------------------------------------------------
// Headers
// ----------------------------------------------------------------------------

func TestRequest_Headers(t *testing.T) {
	t.Run("WithHeader", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Custom-Header") != "custom-value" {
				t.Error("Expected X-Custom-Header: custom-value")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithHeader("X-Custom-Header", "custom-value"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
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

		headers := map[string]string{
			"X-Header-1": "value1",
			"X-Header-2": "value2",
		}
		_, err := client.Get(server.URL, WithHeaderMap(headers))
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

	t.Run("WithJSONAccept", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "application/json" {
				t.Error("Expected Accept: application/json")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithHeader("Accept", "application/json"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithXMLAccept", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "application/xml" {
				t.Error("Expected Accept: application/xml")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithHeader("Accept", "application/xml"))
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

		// Verify request headers are captured
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
// Body Content
// ----------------------------------------------------------------------------

func TestRequest_Body(t *testing.T) {
	type TestData struct {
		Message string `json:"message" xml:"message"`
		Code    int    `json:"code" xml:"code"`
	}

	t.Run("WithJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("Expected Content-Type: application/json")
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

	t.Run("WithXML", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/xml" {
				t.Error("Expected Content-Type: application/xml")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		data := TestData{Message: "test", Code: 200}
		_, err := client.Post(server.URL, WithXML(data))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithBody", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if string(body) != "raw body content" {
				t.Errorf("Expected 'raw body content', got %s", string(body))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithBody([]byte("raw body content")))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithForm", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
				t.Error("Expected Content-Type: application/x-www-form-urlencoded")
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("Failed to parse form: %v", err)
			}
			if r.FormValue("field1") != "value1" {
				t.Error("Expected field1=value1")
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

	t.Run("WithFile", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		content := []byte("test content")
		_, err := client.Post(server.URL, WithFile("file", "test.txt", content))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithFormData", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Error("Expected Content-Type: multipart/form-data")
			}
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			if r.FormValue("field1") != "value1" {
				t.Error("Expected field1=value1")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		formData := &FormData{
			Fields: map[string]string{"field1": "value1"},
			Files: map[string]*FileData{
				"file": {Filename: "test.txt", Content: []byte("content")},
			},
		}
		_, err := client.Post(server.URL, WithFormData(formData))
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
