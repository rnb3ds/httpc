package httpc

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ============================================================================
// REQUEST OPTIONS TESTS - All WithXxx() functions
// ============================================================================

func TestOptions_Headers(t *testing.T) {
	t.Run("WithHeader", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Custom") != "value" {
				t.Error("Expected X-Custom header")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithHeader("X-Custom", "value"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithHeaderMap", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Header-1") != "value1" || r.Header.Get("X-Header-2") != "value2" {
				t.Error("Expected multiple headers")
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
			if r.Header.Get("User-Agent") != "test-agent" {
				t.Error("Expected User-Agent header")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithUserAgent("test-agent"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithContentType", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/custom" {
				t.Error("Expected Content-Type header")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithContentType("application/custom"), WithBody("test"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithAccept", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "application/json" {
				t.Error("Expected Accept header")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithAccept("application/json"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

func TestOptions_Authentication(t *testing.T) {
	t.Run("WithBasicAuth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !ok || username != "user" || password != "pass" {
				t.Error("Expected basic auth user:pass")
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

	t.Run("WithBearerToken", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer token123" {
				t.Error("Expected Authorization header")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithBearerToken("token123"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

func TestOptions_QueryParameters(t *testing.T) {
	t.Run("WithQuery", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key1") != "value1" {
				t.Error("Expected query param key1=value1")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithQuery("key1", "value1"))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithQueryMap", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key1") != "value1" || r.URL.Query().Get("key2") != "123" {
				t.Error("Expected multiple query params")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		params := map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		}
		_, err := client.Get(server.URL, WithQueryMap(params))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

func TestOptions_Body(t *testing.T) {
	type TestData struct {
		Message string `json:"message" xml:"message"`
		Code    int    `json:"code" xml:"code"`
	}

	t.Run("WithJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("Expected Content-Type: application/json")
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

	t.Run("WithXML", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/xml" {
				t.Error("Expected Content-Type: application/xml")
			}
			body, _ := io.ReadAll(r.Body)
			var data TestData
			if err := xml.Unmarshal(body, &data); err != nil {
				t.Errorf("Failed to unmarshal XML: %v", err)
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
		_, err := client.Post(server.URL, WithXML(data))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithBinary", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/octet-stream" {
				t.Error("Expected Content-Type: application/octet-stream")
			}
			body, _ := io.ReadAll(r.Body)
			if len(body) != 1024 {
				t.Errorf("Expected 1024 bytes, got %d", len(body))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		data := make([]byte, 1024)
		_, err := client.Post(server.URL, WithBinary(data))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("WithText", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "text/plain" {
				t.Error("Expected Content-Type: text/plain")
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != "test text" {
				t.Errorf("Expected 'test text', got %s", string(body))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Post(server.URL, WithText("test text"))
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
}

func TestOptions_Timeout(t *testing.T) {
	t.Run("WithTimeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		_, err := client.Get(server.URL, WithTimeout(50*time.Millisecond))
		if err == nil {
			t.Error("Expected timeout error")
		}
	})

	t.Run("WithContext", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err := client.Get(server.URL, WithContext(ctx))
		if err == nil {
			t.Error("Expected timeout error")
		}
	})
}

func TestOptions_Combined(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify all options applied
		if r.Header.Get("X-Custom") != "value" {
			t.Error("Expected X-Custom header")
		}
		if r.Header.Get("Authorization") != "Bearer token123" {
			t.Error("Expected Authorization header")
		}
		if r.URL.Query().Get("param") != "value" {
			t.Error("Expected query param")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	_, err := client.Get(server.URL,
		WithHeader("X-Custom", "value"),
		WithBearerToken("token123"),
		WithQuery("param", "value"),
		WithTimeout(5*time.Second),
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
}
