package httpc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============================================================================
// RESPONSE TESTS - Result object, response parsing, formats, cookies
// Consolidates: result_test.go, response_format_test.go
// ============================================================================

// ----------------------------------------------------------------------------
// Basic Result Usage
// ----------------------------------------------------------------------------

func TestResult_BasicUsage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom-Header", "custom-value")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"success","code":200}`))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	result, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Test status code
	if result.StatusCode() != http.StatusOK {
		t.Errorf("Expected status 200, got %d", result.StatusCode())
	}

	// Test body
	body := result.Body()
	if body == "" {
		t.Error("Body should not be empty")
	}

	// Test response info
	if result.Response == nil {
		t.Fatal("Response info should not be nil")
	}
	if result.Response.StatusCode != http.StatusOK {
		t.Error("Response status code mismatch")
	}
	if result.Response.Headers.Get("Content-Type") != "application/json" {
		t.Error("Content-Type header not found")
	}
	if result.Response.Headers.Get("X-Custom-Header") != "custom-value" {
		t.Error("Custom header not found")
	}
}

// ----------------------------------------------------------------------------
// Convenience Methods
// ----------------------------------------------------------------------------

func TestResult_ConvenienceMethods(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("X-Header-1", "value1")
		w.Header().Set("X-Header-2", "value2")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response body"))
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	result, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Test Body()
	if result.Body() != "response body" {
		t.Errorf("Expected 'response body', got %s", result.Body())
	}

	// Test RawBody()
	bytes := result.RawBody()
	if string(bytes) != "response body" {
		t.Error("RawBody() mismatch")
	}

	// Test StatusCode()
	if result.StatusCode() != http.StatusOK {
		t.Errorf("Expected 200, got %d", result.StatusCode())
	}

	// Test Response headers
	if result.Response.Headers.Get("Content-Type") != "text/plain" {
		t.Error("Header failed for Content-Type")
	}
	if result.Response.Headers.Get("X-Header-1") != "value1" {
		t.Error("Header failed for X-Header-1")
	}
	if result.Response.Headers.Get("X-Header-2") != "value2" {
		t.Error("Header failed for X-Header-2")
	}
}

// ----------------------------------------------------------------------------
// Status Checks
// ----------------------------------------------------------------------------

func TestResult_StatusChecks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		statusCode    int
		isSuccess     bool
		isClientError bool
		isServerError bool
	}{
		{"200 OK", http.StatusOK, true, false, false},
		{"201 Created", http.StatusCreated, true, false, false},
		{"204 No Content", http.StatusNoContent, true, false, false},
		{"400 Bad Request", http.StatusBadRequest, false, true, false},
		{"404 Not Found", http.StatusNotFound, false, true, false},
		{"500 Internal Error", http.StatusInternalServerError, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client, _ := newTestClient()
			defer client.Close()

			result, err := client.Get(server.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if result.IsSuccess() != tt.isSuccess {
				t.Errorf("IsSuccess() = %v, want %v", result.IsSuccess(), tt.isSuccess)
			}
			if result.IsClientError() != tt.isClientError {
				t.Errorf("IsClientError() = %v, want %v", result.IsClientError(), tt.isClientError)
			}
			if result.IsServerError() != tt.isServerError {
				t.Errorf("IsServerError() = %v, want %v", result.IsServerError(), tt.isServerError)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// JSON Parsing
// ----------------------------------------------------------------------------

func TestResult_JSON(t *testing.T) {
	t.Parallel()

	t.Run("Valid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"message": "success",
				"code":    200,
			})
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		result, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		var data map[string]interface{}
		if err := result.JSON(&data); err != nil {
			t.Fatalf("JSON parsing failed: %v", err)
		}

		if data["message"] != "success" {
			t.Error("JSON data mismatch")
		}
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		result, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		var data map[string]interface{}
		err = result.JSON(&data)
		if err == nil {
			t.Error("Expected JSON parsing error")
		}
	})
}

// ----------------------------------------------------------------------------
// Response Cookies
// ----------------------------------------------------------------------------

func TestResult_Cookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123"})
		http.SetCookie(w, &http.Cookie{Name: "token", Value: "xyz789"})
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	t.Run("GetCookie", func(t *testing.T) {
		cookie := resp.GetCookie("session")
		if cookie == nil {
			t.Fatal("Expected cookie to be found")
		}
		if cookie.Value != "abc123" {
			t.Errorf("Expected cookie value abc123, got %s", cookie.Value)
		}
	})

	t.Run("HasCookie", func(t *testing.T) {
		if !resp.HasCookie("session") {
			t.Error("Expected HasCookie to return true")
		}
		if resp.HasCookie("nonexistent") {
			t.Error("Expected HasCookie to return false for nonexistent cookie")
		}
	})

	t.Run("ResponseCookies", func(t *testing.T) {
		cookies := resp.ResponseCookies()
		if len(cookies) != 2 {
			t.Errorf("Expected 2 cookies, got %d", len(cookies))
		}
	})
}

// ----------------------------------------------------------------------------
// String Formatting
// ----------------------------------------------------------------------------

func TestResult_String(t *testing.T) {
	t.Run("valid response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"message":"test"}`))
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		result, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		str := result.String()
		if str == "" {
			t.Error("String() should not be empty")
		}
	})
}

// ----------------------------------------------------------------------------
// HTML Formatting
// ----------------------------------------------------------------------------

func TestResult_Html(t *testing.T) {
	t.Run("valid response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html><body>test</body></html>"))
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer client.Close()

		result, err := client.Get(server.URL)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		html := result.Html()
		if html == "" {
			t.Error("Html() should not be empty")
		}
	})
}

// ----------------------------------------------------------------------------
// Nil Safety
// ----------------------------------------------------------------------------

func TestResult_NilSafety(t *testing.T) {
	t.Parallel()

	var result *Result

	// All methods should handle nil gracefully
	if result.StatusCode() != 0 {
		t.Error("Nil result StatusCode should be 0")
	}
	if result.Body() != "" {
		t.Error("Nil result Body should be empty")
	}
	if result.RawBody() != nil {
		t.Error("Nil result RawBody should be nil")
	}
	if result.IsSuccess() {
		t.Error("Nil result IsSuccess should be false")
	}
	if result.IsClientError() {
		t.Error("Nil result IsClientError should be false")
	}
	if result.IsServerError() {
		t.Error("Nil result IsServerError should be false")
	}

	var data map[string]interface{}
	if err := result.JSON(&data); err == nil {
		t.Error("Nil result JSON should return error")
	}
}
