package httpc

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// REQUEST OPTIONS COMPREHENSIVE TESTS
// ============================================================================

func TestOptions_WithHeaderMap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Header-1") != "value1" {
			t.Error("Expected X-Header-1 to be value1")
		}
		if r.Header.Get("X-Header-2") != "value2" {
			t.Error("Expected X-Header-2 to be value2")
		}
		if r.Header.Get("X-Header-3") != "value3" {
			t.Error("Expected X-Header-3 to be value3")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	headers := map[string]string{
		"X-Header-1": "value1",
		"X-Header-2": "value2",
		"X-Header-3": "value3",
	}

	resp, err := client.Get(server.URL, WithHeaderMap(headers))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOptions_WithContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/custom" {
			t.Errorf("Expected Content-Type application/custom, got %s", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Post(server.URL,
		WithContentType("application/custom"),
		WithBody("test data"),
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOptions_WithAccept(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/custom" {
			t.Errorf("Expected Accept application/custom, got %s", r.Header.Get("Accept"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL, WithAccept("application/custom"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOptions_WithJSONAccept(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Expected Accept application/json, got %s", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL, WithJSONAccept())
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]string
	if err := resp.JSON(&result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("Expected status=ok, got %v", result["status"])
	}
}

func TestOptions_WithXMLAccept(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/xml" {
			t.Errorf("Expected Accept application/xml, got %s", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<response><status>ok</status></response>`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL, WithAccept("application/xml"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOptions_WithHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret-key-123" {
			t.Errorf("Expected X-API-Key secret-key-123, got %s", r.Header.Get("X-API-Key"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL, WithHeader("X-API-Key", "secret-key-123"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOptions_WithQueryMap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("param1") != "value1" {
			t.Error("Expected param1=value1")
		}
		if query.Get("param2") != "123" {
			t.Error("Expected param2=123")
		}
		if query.Get("param3") != "true" {
			t.Error("Expected param3=true")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	params := map[string]interface{}{
		"param1": "value1",
		"param2": 123,
		"param3": true,
	}

	resp, err := client.Get(server.URL, WithQueryMap(params))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOptions_WithText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "text/plain" {
			t.Errorf("Expected Content-Type text/plain, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		if string(body) != "Hello, World!" {
			t.Errorf("Expected body 'Hello, World!', got %s", string(body))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Post(server.URL, WithText("Hello, World!"))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOptions_WithFormData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if !strings.Contains(contentType, "multipart/form-data") {
			t.Errorf("Expected multipart/form-data, got %s", contentType)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	formData := &FormData{
		Fields: map[string]string{
			"field1": "value1",
			"field2": "value2",
		},
		Files: map[string]*FileData{
			"file1": {
				Filename: "test.txt",
				Content:  []byte("test content"),
			},
		},
	}

	resp, err := client.Post(server.URL, WithFormData(formData))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOptions_WithFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if !strings.Contains(contentType, "multipart/form-data") {
			t.Errorf("Expected multipart/form-data, got %s", contentType)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	fileContent := []byte("This is a test file content")
	resp, err := client.Post(server.URL, WithFile("document", "test.txt", fileContent))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOptions_WithContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Get(server.URL, WithContext(ctx))
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
}

func TestOptions_CombinedOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "custom-value" {
			t.Error("Custom header not set")
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("Bearer token not set")
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("Accept header not set")
		}
		if r.URL.Query().Get("page") != "1" {
			t.Error("Query parameter not set")
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	config := DefaultConfig()
	config.AllowPrivateIPs = true
	config.Timeout = 10 * time.Second
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(server.URL,
		WithHeader("X-Custom", "custom-value"),
		WithBearerToken("test-token"),
		WithJSONAccept(),
		WithQuery("page", 1),
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestOptions_BodyTypes(t *testing.T) {
	tests := []struct {
		name       string
		option     RequestOption
		expectedCT string
		verifyBody func(*testing.T, []byte)
	}{
		{
			name:       "String Body",
			option:     WithBody("test string"),
			expectedCT: "text/plain",
			verifyBody: func(t *testing.T, body []byte) {
				if string(body) != "test string" {
					t.Errorf("Expected 'test string', got %s", string(body))
				}
			},
		},
		{
			name:       "Byte Array Body",
			option:     WithBody([]byte{0x01, 0x02, 0x03}),
			expectedCT: "application/octet-stream",
			verifyBody: func(t *testing.T, body []byte) {
				if len(body) != 3 || body[0] != 0x01 {
					t.Error("Byte array body not correct")
				}
			},
		},
		{
			name:       "Reader Body",
			option:     WithBody(bytes.NewReader([]byte("reader content"))),
			expectedCT: "",
			verifyBody: func(t *testing.T, body []byte) {
				if string(body) != "reader content" {
					t.Errorf("Expected 'reader content', got %s", string(body))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				tt.verifyBody(t, body)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client, _ := newTestClient()
			defer client.Close()

			resp, err := client.Post(server.URL, tt.option)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}
