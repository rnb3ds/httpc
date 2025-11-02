package httpc

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFileData_ContentType(t *testing.T) {
	tests := []struct {
		name                string
		fileData            *FileData
		expectedContentType string
	}{
		{
			name: "PDF file with ContentType",
			fileData: &FileData{
				Filename:    "document.pdf",
				Content:     []byte("PDF content"),
				ContentType: "application/pdf",
			},
			expectedContentType: "application/pdf",
		},
		{
			name: "JPEG image with ContentType",
			fileData: &FileData{
				Filename:    "photo.jpg",
				Content:     []byte("JPEG content"),
				ContentType: "image/jpeg",
			},
			expectedContentType: "image/jpeg",
		},
		{
			name: "PNG image with ContentType",
			fileData: &FileData{
				Filename:    "icon.png",
				Content:     []byte("PNG content"),
				ContentType: "image/png",
			},
			expectedContentType: "image/png",
		},
		{
			name: "Text file with ContentType",
			fileData: &FileData{
				Filename:    "readme.txt",
				Content:     []byte("Text content"),
				ContentType: "text/plain",
			},
			expectedContentType: "text/plain",
		},
		{
			name: "File without ContentType (should use default)",
			fileData: &FileData{
				Filename: "file.bin",
				Content:  []byte("Binary content"),
			},
			expectedContentType: "application/octet-stream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Parse multipart form
				err := r.ParseMultipartForm(10 << 20) // 10 MB
				if err != nil {
					t.Fatalf("Failed to parse multipart form: %v", err)
				}

				// Get the file
				file, header, err := r.FormFile("file")
				if err != nil {
					t.Fatalf("Failed to get form file: %v", err)
				}
				defer file.Close()

				// Check filename
				if header.Filename != tt.fileData.Filename {
					t.Errorf("Expected filename %s, got %s", tt.fileData.Filename, header.Filename)
				}

				// Check Content-Type
				contentType := header.Header.Get("Content-Type")
				if contentType != tt.expectedContentType {
					t.Errorf("Expected Content-Type %s, got %s", tt.expectedContentType, contentType)
				}

				// Check content
				content, err := io.ReadAll(file)
				if err != nil {
					t.Fatalf("Failed to read file content: %v", err)
				}

				if string(content) != string(tt.fileData.Content) {
					t.Errorf("Expected content %s, got %s", string(tt.fileData.Content), string(content))
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client, _ := newTestClient()
			defer client.Close()

			// Use WithFormData to test ContentType field
			formData := &FormData{
				Files: map[string]*FileData{
					"file": tt.fileData,
				},
			}

			resp, err := client.Post(server.URL,
				WithFormData(formData),
			)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestFormData_MultipleFilesWithContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse multipart form
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("Failed to parse multipart form: %v", err)
		}

		// Check form fields
		if r.FormValue("title") != "My Documents" {
			t.Errorf("Expected title 'My Documents', got %s", r.FormValue("title"))
		}

		if r.FormValue("description") != "Important files" {
			t.Errorf("Expected description 'Important files', got %s", r.FormValue("description"))
		}

		// Check first file
		file1, header1, err := r.FormFile("document1")
		if err != nil {
			t.Fatalf("Failed to get document1: %v", err)
		}
		defer file1.Close()

		if header1.Filename != "doc1.pdf" {
			t.Errorf("Expected filename doc1.pdf, got %s", header1.Filename)
		}

		if header1.Header.Get("Content-Type") != "application/pdf" {
			t.Errorf("Expected Content-Type application/pdf, got %s", header1.Header.Get("Content-Type"))
		}

		// Check second file
		file2, header2, err := r.FormFile("document2")
		if err != nil {
			t.Fatalf("Failed to get document2: %v", err)
		}
		defer file2.Close()

		if header2.Filename != "doc2.pdf" {
			t.Errorf("Expected filename doc2.pdf, got %s", header2.Filename)
		}

		if header2.Header.Get("Content-Type") != "application/pdf" {
			t.Errorf("Expected Content-Type application/pdf, got %s", header2.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	formData := &FormData{
		Fields: map[string]string{
			"title":       "My Documents",
			"description": "Important files",
		},
		Files: map[string]*FileData{
			"document1": {
				Filename:    "doc1.pdf",
				Content:     []byte("PDF content 1"),
				ContentType: "application/pdf",
			},
			"document2": {
				Filename:    "doc2.pdf",
				Content:     []byte("PDF content 2"),
				ContentType: "application/pdf",
			},
		},
	}

	resp, err := client.Post(server.URL,
		WithFormData(formData),
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestFormData_MixedContentTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := r.ParseMultipartForm(10 << 20)
		if err != nil {
			t.Fatalf("Failed to parse multipart form: %v", err)
		}

		// Check PDF file
		pdfFile, pdfHeader, err := r.FormFile("document")
		if err != nil {
			t.Fatalf("Failed to get document: %v", err)
		}
		defer pdfFile.Close()

		if pdfHeader.Header.Get("Content-Type") != "application/pdf" {
			t.Errorf("Expected PDF Content-Type application/pdf, got %s", pdfHeader.Header.Get("Content-Type"))
		}

		// Check image file
		imgFile, imgHeader, err := r.FormFile("thumbnail")
		if err != nil {
			t.Fatalf("Failed to get thumbnail: %v", err)
		}
		defer imgFile.Close()

		if imgHeader.Header.Get("Content-Type") != "image/jpeg" {
			t.Errorf("Expected image Content-Type image/jpeg, got %s", imgHeader.Header.Get("Content-Type"))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	formData := &FormData{
		Fields: map[string]string{
			"title": "Mixed Upload",
		},
		Files: map[string]*FileData{
			"document": {
				Filename:    "report.pdf",
				Content:     []byte("PDF content"),
				ContentType: "application/pdf",
			},
			"thumbnail": {
				Filename:    "preview.jpg",
				Content:     []byte("JPEG content"),
				ContentType: "image/jpeg",
			},
		},
	}

	resp, err := client.Post(server.URL,
		WithFormData(formData),
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestFormData_EmptyContentType(t *testing.T) {
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

		// When ContentType is empty, it should use default (application/octet-stream)
		contentType := header.Header.Get("Content-Type")
		if contentType != "application/octet-stream" {
			t.Errorf("Expected default Content-Type application/octet-stream, got %s", contentType)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	formData := &FormData{
		Files: map[string]*FileData{
			"file": {
				Filename: "data.bin",
				Content:  []byte("binary data"),
				// ContentType is empty
			},
		},
	}

	resp, err := client.Post(server.URL,
		WithFormData(formData),
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestFormData_CustomContentType(t *testing.T) {
	customContentType := "application/vnd.custom+json"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("Failed to create multipart reader: %v", err)
		}

		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Failed to read part: %v", err)
			}

			if part.FormName() == "customfile" {
				contentType := part.Header.Get("Content-Type")
				if contentType != customContentType {
					t.Errorf("Expected Content-Type %s, got %s", customContentType, contentType)
				}
			}

			part.Close()
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	formData := &FormData{
		Files: map[string]*FileData{
			"customfile": {
				Filename:    "data.custom",
				Content:     []byte(`{"custom":"data"}`),
				ContentType: customContentType,
			},
		},
	}

	resp, err := client.Post(server.URL,
		WithFormData(formData),
	)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

