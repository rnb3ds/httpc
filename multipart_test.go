package httpc

import (
	"bytes"
	"crypto/rand"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ============================================================================
// MULTIPART FORM DATA AND FILE UPLOAD TESTS
// ============================================================================

func TestMultipart_SingleFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify content type
		contentType := r.Header.Get("Content-Type")
		if !strings.Contains(contentType, "multipart/form-data") {
			t.Errorf("Expected multipart/form-data, got %s", contentType)
		}

		// Parse multipart form
		err := r.ParseMultipartForm(10 << 20) // 10 MB
		if err != nil {
			t.Fatalf("Failed to parse multipart form: %v", err)
		}

		// Check file
		file, header, err := r.FormFile("document")
		if err != nil {
			t.Fatalf("Failed to get file: %v", err)
		}
		defer file.Close()

		if header.Filename != "test.txt" {
			t.Errorf("Expected filename test.txt, got %s", header.Filename)
		}

		content, _ := io.ReadAll(file)
		if string(content) != "Test file content" {
			t.Errorf("Expected 'Test file content', got %s", string(content))
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"uploaded"}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	fileContent := []byte("Test file content")
	resp, err := client.Post(server.URL, WithFile("document", "test.txt", fileContent))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestMultipart_MultipleFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("Failed to parse multipart form: %v", err)
		}

		// Check multiple files
		if r.MultipartForm == nil || len(r.MultipartForm.File) == 0 {
			t.Error("No files found in multipart form")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	formData := &FormData{
		Fields: map[string]string{},
		Files: map[string]*FileData{
			"file1": {
				Filename:    "file1.txt",
				Content:     []byte("Content of file 1"),
				ContentType: "text/plain",
			},
			"file2": {
				Filename:    "file2.txt",
				Content:     []byte("Content of file 2"),
				ContentType: "text/plain",
			},
			"file3": {
				Filename:    "file3.txt",
				Content:     []byte("Content of file 3"),
				ContentType: "text/plain",
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

func TestMultipart_MixedFieldsAndFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("Failed to parse multipart form: %v", err)
		}

		// Check form fields
		if r.FormValue("username") != "testuser" {
			t.Errorf("Expected username=testuser, got %s", r.FormValue("username"))
		}
		if r.FormValue("email") != "test@example.com" {
			t.Errorf("Expected email=test@example.com, got %s", r.FormValue("email"))
		}

		// Check file
		file, header, err := r.FormFile("avatar")
		if err != nil {
			t.Fatalf("Failed to get file: %v", err)
		}
		defer file.Close()

		if header.Filename != "avatar.png" {
			t.Errorf("Expected filename avatar.png, got %s", header.Filename)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	formData := &FormData{
		Fields: map[string]string{
			"username": "testuser",
			"email":    "test@example.com",
		},
		Files: map[string]*FileData{
			"avatar": {
				Filename:    "avatar.png",
				Content:     []byte{0x89, 0x50, 0x4E, 0x47}, // PNG header
				ContentType: "image/png",
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

func TestMultipart_LargeFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(50 << 20) // 50 MB
		if err != nil {
			t.Fatalf("Failed to parse multipart form: %v", err)
		}

		file, header, err := r.FormFile("largefile")
		if err != nil {
			t.Fatalf("Failed to get file: %v", err)
		}
		defer file.Close()

		content, _ := io.ReadAll(file)
		expectedSize := 1024 * 1024 // 1 MB
		if len(content) != expectedSize {
			t.Errorf("Expected file size %d, got %d", expectedSize, len(content))
		}

		if header.Filename != "large.bin" {
			t.Errorf("Expected filename large.bin, got %s", header.Filename)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Create 1MB file
	largeContent := make([]byte, 1024*1024)
	rand.Read(largeContent)

	resp, err := client.Post(server.URL, WithFile("largefile", "large.bin", largeContent))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestMultipart_BinaryFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("Failed to parse multipart form: %v", err)
		}

		file, header, err := r.FormFile("binary")
		if err != nil {
			t.Fatalf("Failed to get file: %v", err)
		}
		defer file.Close()

		content, _ := io.ReadAll(file)
		
		// Verify binary content
		if len(content) != 256 {
			t.Errorf("Expected 256 bytes, got %d", len(content))
		}

		// Verify it's binary data (all byte values 0-255)
		for i := 0; i < 256; i++ {
			if content[i] != byte(i) {
				t.Errorf("Expected byte %d at position %d, got %d", i, i, content[i])
			}
		}

		if header.Filename != "binary.dat" {
			t.Errorf("Expected filename binary.dat, got %s", header.Filename)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	// Create binary content with all byte values
	binaryContent := make([]byte, 256)
	for i := 0; i < 256; i++ {
		binaryContent[i] = byte(i)
	}

	formData := &FormData{
		Fields: map[string]string{},
		Files: map[string]*FileData{
			"binary": {
				Filename:    "binary.dat",
				Content:     binaryContent,
				ContentType: "application/octet-stream",
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

func TestMultipart_EmptyFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("Failed to parse multipart form: %v", err)
		}

		file, header, err := r.FormFile("emptyfile")
		if err != nil {
			t.Fatalf("Failed to get file: %v", err)
		}
		defer file.Close()

		content, _ := io.ReadAll(file)
		if len(content) != 0 {
			t.Errorf("Expected empty file, got %d bytes", len(content))
		}

		if header.Filename != "empty.txt" {
			t.Errorf("Expected filename empty.txt, got %s", header.Filename)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Post(server.URL, WithFile("emptyfile", "empty.txt", []byte{}))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestMultipart_DifferentContentTypes(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		contentType string
		content     []byte
	}{
		{"Text File", "document.txt", "text/plain", []byte("Text content")},
		{"JSON File", "data.json", "application/json", []byte(`{"key":"value"}`)},
		{"XML File", "data.xml", "application/xml", []byte(`<root><key>value</key></root>`)},
		{"PNG Image", "image.png", "image/png", []byte{0x89, 0x50, 0x4E, 0x47}},
		{"JPEG Image", "image.jpg", "image/jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}},
		{"PDF Document", "document.pdf", "application/pdf", []byte{0x25, 0x50, 0x44, 0x46}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				err := r.ParseMultipartForm(10 << 20)
				if err != nil {
					t.Fatalf("Failed to parse multipart form: %v", err)
				}

				file, header, err := r.FormFile("file")
				if err != nil {
					t.Fatalf("Failed to get file: %v", err)
				}
				defer file.Close()

				if header.Filename != tt.filename {
					t.Errorf("Expected filename %s, got %s", tt.filename, header.Filename)
				}

				content, _ := io.ReadAll(file)
				if !bytes.Equal(content, tt.content) {
					t.Error("File content mismatch")
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client, _ := newTestClient()
			defer client.Close()

			formData := &FormData{
				Fields: map[string]string{},
				Files: map[string]*FileData{
					"file": {
						Filename:    tt.filename,
						Content:     tt.content,
						ContentType: tt.contentType,
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
		})
	}
}

func TestMultipart_BoundaryHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		
		// Parse boundary from content type
		_, params, err := mime.ParseMediaType(contentType)
		if err != nil {
			t.Fatalf("Failed to parse media type: %v", err)
		}

		boundary := params["boundary"]
		if boundary == "" {
			t.Error("Boundary not found in Content-Type")
		}

		// Parse multipart form with boundary
		reader := multipart.NewReader(r.Body, boundary)
		
		partCount := 0
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Failed to read part: %v", err)
			}
			partCount++
			part.Close()
		}

		if partCount == 0 {
			t.Error("No parts found in multipart form")
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	formData := &FormData{
		Fields: map[string]string{
			"field1": "value1",
		},
		Files: map[string]*FileData{
			"file1": {
				Filename:    "test.txt",
				Content:     []byte("test"),
				ContentType: "text/plain",
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

