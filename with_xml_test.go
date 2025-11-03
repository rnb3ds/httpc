package httpc

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithXML(t *testing.T) {
	tests := []struct {
		name     string
		data     interface{}
		expected string
	}{
		{
			name: "Simple struct",
			data: struct {
				XMLName struct{} `xml:"user"`
				Name    string   `xml:"name"`
				Email   string   `xml:"email"`
			}{
				Name:  "John Doe",
				Email: "john@example.com",
			},
			expected: "<user><name>John Doe</name><email>john@example.com</email></user>",
		},
		{
			name: "Struct with numbers",
			data: struct {
				XMLName struct{} `xml:"product"`
				ID      int      `xml:"id"`
				Price   float64  `xml:"price"`
			}{
				ID:    123,
				Price: 99.99,
			},
			expected: "<product><id>123</id><price>99.99</price></product>",
		},
		{
			name: "Nested struct",
			data: struct {
				XMLName struct{} `xml:"order"`
				ID      int      `xml:"id"`
				User    struct {
					Name  string `xml:"name"`
					Email string `xml:"email"`
				} `xml:"user"`
			}{
				ID: 456,
				User: struct {
					Name  string `xml:"name"`
					Email string `xml:"email"`
				}{
					Name:  "Jane Smith",
					Email: "jane@example.com",
				},
			},
			expected: "<order><id>456</id><user><name>Jane Smith</name><email>jane@example.com</email></user></order>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check Content-Type header
				if r.Header.Get("Content-Type") != "application/xml" {
					t.Errorf("Expected Content-Type: application/xml, got %s", r.Header.Get("Content-Type"))
				}

				// Read and verify body
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("Failed to read body: %v", err)
				}

				if string(body) != tt.expected {
					t.Errorf("Expected body:\n%s\nGot:\n%s", tt.expected, string(body))
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client, _ := newTestClient()
			defer client.Close()

			resp, err := client.Post(server.URL, WithXML(tt.data))
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
			}
		})
	}
}

func TestWithXML_ContentTypeOverride(t *testing.T) {
	// Test that WithXML sets Content-Type correctly
	req := &Request{}
	
	data := struct {
		XMLName struct{} `xml:"test"`
		Value   string   `xml:"value"`
	}{
		Value: "test",
	}
	
	opt := WithXML(data)
	opt(req)

	if req.Headers == nil {
		t.Fatal("Headers map is nil")
	}

	ct, exists := req.Headers["Content-Type"]
	if !exists {
		t.Error("Content-Type header not set")
	}

	if ct != "application/xml" {
		t.Errorf("Expected Content-Type: application/xml, got %s", ct)
	}

	if req.Body == nil {
		t.Error("Body is nil")
	}
}

func TestWithXML_EmptyStruct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/xml" {
			t.Errorf("Expected Content-Type: application/xml")
		}

		body, _ := io.ReadAll(r.Body)
		expected := "<empty></empty>"
		if string(body) != expected {
			t.Errorf("Expected body: %s, got: %s", expected, string(body))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	data := struct {
		XMLName struct{} `xml:"empty"`
	}{}

	resp, err := client.Post(server.URL, WithXML(data))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestWithXML_WithAttributes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		
		// Parse the XML to verify structure
		type TestData struct {
			XMLName xml.Name `xml:"book"`
			ID      string   `xml:"id,attr"`
			Title   string   `xml:"title"`
		}
		
		var data TestData
		if err := xml.Unmarshal(body, &data); err != nil {
			t.Errorf("Failed to unmarshal XML: %v", err)
		}
		
		if data.ID != "123" {
			t.Errorf("Expected ID=123, got %s", data.ID)
		}
		
		if data.Title != "Go Programming" {
			t.Errorf("Expected Title='Go Programming', got %s", data.Title)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	type Book struct {
		XMLName xml.Name `xml:"book"`
		ID      string   `xml:"id,attr"`
		Title   string   `xml:"title"`
	}

	book := Book{
		ID:    "123",
		Title: "Go Programming",
	}

	resp, err := client.Post(server.URL, WithXML(book))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestWithXML_PackageLevel(t *testing.T) {
	setupPackageLevelTests()
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/xml" {
			t.Errorf("Expected Content-Type: application/xml, got %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		expected := "<user><name>Alice</name><age>25</age></user>"
		if string(body) != expected {
			t.Errorf("Expected body: %s, got: %s", expected, string(body))
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	type User struct {
		XMLName struct{} `xml:"user"`
		Name    string   `xml:"name"`
		Age     int      `xml:"age"`
	}

	user := User{
		Name: "Alice",
		Age:  25,
	}

	resp, err := Post(server.URL, WithXML(user))
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}
}

