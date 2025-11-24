package httpc

import (
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================================================
// DATA HANDLING TESTS - JSON, XML, multipart, cookies
// ============================================================================

type TestData struct {
	Message string `json:"message" xml:"message"`
	Code    int    `json:"code" xml:"code"`
}

func TestJSON_Handling(t *testing.T) {
	t.Run("SendJSON", func(t *testing.T) {
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

	t.Run("ReceiveJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(TestData{Message: "response", Code: 200})
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

	t.Run("InvalidJSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{invalid json}`))
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
	})

	t.Run("EmptyJSON", func(t *testing.T) {
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

		_, err := client.Post(server.URL, WithJSON(map[string]interface{}{}))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

func TestXML_Handling(t *testing.T) {
	t.Run("SendXML", func(t *testing.T) {
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

	t.Run("EmptyXML", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Content-Type") != "application/xml" {
				t.Error("Expected Content-Type: application/xml")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		data := TestData{}
		_, err := client.Post(server.URL, WithXML(data))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

func TestMultipart_Handling(t *testing.T) {
	t.Run("SingleFile", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
				t.Error("Expected Content-Type: multipart/form-data")
			}
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Errorf("Failed to get file: %v", err)
			}
			defer file.Close()
			if header.Filename != "test.txt" {
				t.Errorf("Expected filename test.txt, got %s", header.Filename)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		formData := &FormData{
			Files: map[string]*FileData{
				"file": {
					Filename: "test.txt",
					Content:  []byte("test content"),
				},
			},
		}
		_, err := client.Post(server.URL, WithFormData(formData))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("MultipleFiles", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			if len(r.MultipartForm.File) != 2 {
				t.Errorf("Expected 2 files, got %d", len(r.MultipartForm.File))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		formData := &FormData{
			Files: map[string]*FileData{
				"file1": {Filename: "test1.txt", Content: []byte("content1")},
				"file2": {Filename: "test2.txt", Content: []byte("content2")},
			},
		}
		_, err := client.Post(server.URL, WithFormData(formData))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})

	t.Run("MixedFieldsAndFiles", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			if r.FormValue("field1") != "value1" {
				t.Error("Expected field1=value1")
			}
			if len(r.MultipartForm.File) != 1 {
				t.Errorf("Expected 1 file, got %d", len(r.MultipartForm.File))
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

	t.Run("ContentTypes", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseMultipartForm(10 << 20); err != nil {
				t.Errorf("Failed to parse multipart form: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		formData := &FormData{
			Files: map[string]*FileData{
				"image": {
					Filename:    "test.png",
					Content:     []byte("fake image data"),
					ContentType: "image/png",
				},
				"doc": {
					Filename:    "test.pdf",
					Content:     []byte("fake pdf data"),
					ContentType: "application/pdf",
				},
			},
		}
		_, err := client.Post(server.URL, WithFormData(formData))
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
	})
}

func TestCookies_Handling(t *testing.T) {
	t.Run("BasicCookies", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{
				Name:  "session",
				Value: "abc123",
			})
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := DefaultConfig()
		config.EnableCookies = true
		config.AllowPrivateIPs = true
		client, _ := New(config)
		defer client.Close()

		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		cookies := resp.Cookies
		if len(cookies) != 1 {
			t.Errorf("Expected 1 cookie, got %d", len(cookies))
		}
		if cookies[0].Name != "session" || cookies[0].Value != "abc123" {
			t.Error("Cookie values don't match")
		}
	})

	t.Run("CookiePersistence", func(t *testing.T) {
		requestCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			if requestCount == 1 {
				http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
			} else {
				cookie, err := r.Cookie("session")
				if err != nil || cookie.Value != "abc123" {
					t.Error("Cookie not persisted across requests")
				}
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := DefaultConfig()
		config.EnableCookies = true
		config.AllowPrivateIPs = true
		client, _ := New(config)
		defer client.Close()

		// First request sets cookie
		_, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("First request failed: %v", err)
		}

		// Second request should send cookie
		_, err = client.Get(server.URL)
		if err != nil {
			t.Fatalf("Second request failed: %v", err)
		}
	})
}

func TestFormData_URLEncoded(t *testing.T) {
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
}
