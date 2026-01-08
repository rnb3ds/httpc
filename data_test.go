package httpc

import (
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================================================
// DATA HANDLING TESTS - JSON, XML, multipart, form data
// Consolidates: data_handling_test.go
// ============================================================================

type TestData struct {
	Message string `json:"message" xml:"message"`
	Code    int    `json:"code" xml:"code"`
}

// ----------------------------------------------------------------------------
// JSON Handling
// ----------------------------------------------------------------------------

func TestData_JSON(t *testing.T) {
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

// ----------------------------------------------------------------------------
// XML Handling
// ----------------------------------------------------------------------------

func TestData_XML(t *testing.T) {
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

// ----------------------------------------------------------------------------
// Multipart Form Data
// ----------------------------------------------------------------------------

func TestData_Multipart(t *testing.T) {
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
			defer func() { _ = file.Close() }()
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

// ----------------------------------------------------------------------------
// Note: Cookie tests have been moved to cookie_test.go for better organization
// ----------------------------------------------------------------------------

// ----------------------------------------------------------------------------
// Form URL Encoded
// ----------------------------------------------------------------------------

func TestData_FormURLEncoded(t *testing.T) {
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

// ----------------------------------------------------------------------------
// Compression Handling
// ----------------------------------------------------------------------------

func TestData_Compression(t *testing.T) {
	t.Run("GzipResponse", func(t *testing.T) {
		content := "This is compressed content"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)

			// Write gzip compressed content
			gw := gzip.NewWriter(w)
			_, _ = gw.Write([]byte(content))
			_ = gw.Close()
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		// Response should be automatically decompressed
		if resp.Body() != content {
			t.Errorf("Expected decompressed content %q, got %q", content, resp.Body())
		}
	})

	t.Run("DeflateResponse", func(t *testing.T) {
		content := "This is deflate compressed content"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Encoding", "deflate")
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)

			// Write deflate compressed content
			fw, _ := flate.NewWriter(w, flate.DefaultCompression)
			_, _ = fw.Write([]byte(content))
			_ = fw.Close()
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		// Response should be automatically decompressed
		if resp.Body() != content {
			t.Errorf("Expected decompressed content %q, got %q", content, resp.Body())
		}
	})

	t.Run("NoCompression", func(t *testing.T) {
		content := "This is uncompressed content"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(content))
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.Body() != content {
			t.Errorf("Expected content %q, got %q", content, resp.Body())
		}
	})
}
