package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/types"
)

// ============================================================================
// REQUEST PROCESSOR TESTS
// ============================================================================

// TestRequestProcessor_Build_ClientConfigContentType pins the M3 regression:
// client-level Config.Headers participate in the body marshal decision.
// Auto-detection previously consulted only the per-request Content-Type, and
// its detected value took the header slot before config headers were merged
// (fill-empty), so a configured "application/xml" was silently overridden by
// "application/json" — both in the marshaling and on the wire.
func TestRequestProcessor_Build_ClientConfigContentType(t *testing.T) {
	type xmlPayload struct {
		XMLName xml.Name `xml:"payload"`
		K       string   `xml:"k"`
	}

	build := func(headers, requestHeaders map[string]string) (*http.Request, error) {
		config := &Config{Timeout: 30 * time.Second, Headers: headers}
		processor := newRequestProcessor(config)
		req := testRequestBuilder().
			Method("POST").
			URL("https://example.com").
			Headers(requestHeaders).
			Body(xmlPayload{K: "v"}).
			Build()
		return processor.Build(req)
	}

	t.Run("config XML wins over JSON auto-detection", func(t *testing.T) {
		httpReq, err := build(map[string]string{"Content-Type": "application/xml"}, nil)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if got := httpReq.Header.Get("Content-Type"); got != "application/xml" {
			t.Errorf("Content-Type = %q, want application/xml", got)
		}
		body, _ := io.ReadAll(httpReq.Body)
		if !strings.Contains(string(body), "<k>v</k>") {
			t.Errorf("body = %q, want XML-marshaled payload", body)
		}
	})

	t.Run("no config header keeps JSON detection", func(t *testing.T) {
		httpReq, err := build(nil, nil)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if got := httpReq.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
	})

	t.Run("request-level header still wins", func(t *testing.T) {
		httpReq, err := build(map[string]string{"Content-Type": "application/xml"}, map[string]string{"Content-Type": "text/xml"})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if got := httpReq.Header.Get("Content-Type"); got != "text/xml" {
			t.Errorf("Content-Type = %q, want text/xml", got)
		}
	})
}

// TestRequestProcessor_Build_ReaderBodySizeLimit pins the M8 regression:
// Security.MaxRequestBodySize no longer fails open for raw io.Reader bodies —
// the pre-send validator can only size known types, so Build wraps readers
// in a counting reader that fails the transfer once the limit is crossed.
func TestRequestProcessor_Build_ReaderBodySizeLimit(t *testing.T) {
	processor := newRequestProcessor(&Config{Timeout: 30 * time.Second, MaxRequestBodySize: 16})

	build := func(payload string) *http.Request {
		t.Helper()
		req := testRequestBuilder().
			Method("POST").
			URL("https://example.com").
			Body(strings.NewReader(payload)).
			Build()
		httpReq, err := processor.Build(req)
		if err != nil {
			t.Fatalf("Build must not fail before send: %v", err)
		}
		return httpReq
	}

	t.Run("under limit passes", func(t *testing.T) {
		body, err := io.ReadAll(build("short body").Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(body) != "short body" {
			t.Errorf("body = %q", body)
		}
	})

	t.Run("exactly at limit passes", func(t *testing.T) {
		const exact = "1234567890123456" // 16 bytes
		body, err := io.ReadAll(build(exact).Body)
		if err != nil {
			t.Fatalf("exact-limit body must pass, got %v", err)
		}
		if string(body) != exact {
			t.Errorf("body = %q", body)
		}
	})

	t.Run("over limit fails at send time", func(t *testing.T) {
		_, err := io.ReadAll(build("this payload is well over sixteen bytes").Body)
		if err == nil || !strings.Contains(err.Error(), "exceeds limit") {
			t.Fatalf("want limit error mid-read, got %v", err)
		}
	})
}

// TestRequestProcessor_Build_DoesNotLeakURLMutationIntoCache pins the shared-
// URL defensive copy: a query-less Build hands each request its own shallow
// copy of the process-wide cached *url.URL, so a downstream mutation of
// req.URL (custom RoundTripper, redirect rewriting) cannot poison the cache
// for every other request to the same URL.
func TestRequestProcessor_Build_DoesNotLeakURLMutationIntoCache(t *testing.T) {
	processor := newRequestProcessor(&Config{Timeout: 30 * time.Second})

	build := func() *http.Request {
		t.Helper()
		req := testRequestBuilder().Method("GET").URL("https://example.com/cache-isolation-probe").Build()
		httpReq, err := processor.Build(req)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		return httpReq
	}

	h1 := build()
	h1.URL.Path = "/mutated-by-downstream" // simulate a mutating RoundTripper

	h2 := build()
	if h2.URL.Path != "/cache-isolation-probe" {
		t.Errorf("URL cache poisoned by downstream mutation: Path = %q, want %q",
			h2.URL.Path, "/cache-isolation-probe")
	}
}

func TestRequestProcessor_Build(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
		UserAgent:       "TestClient/1.0",
	}

	processor := newRequestProcessor(config)

	tests := []struct {
		name     string
		request  *Request
		validate func(*testing.T, *http.Request)
	}{
		{
			name: "Simple GET request",
			request: testRequestBuilder().
				Method("GET").
				URL("https://api.example.com/users").
				Context(context.Background()).
				Headers(map[string]string{
					"Accept": "application/json",
				}).
				Build(),
			validate: func(t *testing.T, req *http.Request) {
				if req.Method != "GET" {
					t.Errorf("Expected method GET, got %s", req.Method)
				}
				if req.URL.String() != "https://api.example.com/users" {
					t.Errorf("Expected URL https://api.example.com/users, got %s", req.URL.String())
				}
				if req.Header.Get("Accept") != "application/json" {
					t.Errorf("Expected Accept header application/json, got %s", req.Header.Get("Accept"))
				}
			},
		},
		{
			name: "POST with JSON body",
			request: testRequestBuilder().
				Method("POST").
				URL("https://api.example.com/users").
				Context(context.Background()).
				Headers(map[string]string{
					"Content-Type": "application/json",
				}).
				Body(map[string]interface{}{
					"name":  "John Doe",
					"email": "john@example.com",
				}).
				Build(),
			validate: func(t *testing.T, req *http.Request) {
				if req.Method != "POST" {
					t.Errorf("Expected method POST, got %s", req.Method)
				}
				if req.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type application/json, got %s", req.Header.Get("Content-Type"))
				}
				if req.Body == nil {
					t.Error("Expected request body, got nil")
				}

				// Read and validate body
				bodyBytes, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("Failed to read body: %v", err)
				}

				var data map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &data); err != nil {
					t.Errorf("Failed to unmarshal JSON body: %v", err)
				}

				if data["name"] != "John Doe" {
					t.Errorf("Expected name 'John Doe', got %v", data["name"])
				}
			},
		},
		{
			name: "Request with query parameters",
			request: testRequestBuilder().
				Method("GET").
				URL("https://api.example.com/users").
				Context(context.Background()).
				QueryParams(map[string]any{
					"page":   1,
					"limit":  10,
					"filter": "active",
				}).
				Build(),
			validate: func(t *testing.T, req *http.Request) {
				query := req.URL.Query()
				if query.Get("page") != "1" {
					t.Errorf("Expected page=1, got %s", query.Get("page"))
				}
				if query.Get("limit") != "10" {
					t.Errorf("Expected limit=10, got %s", query.Get("limit"))
				}
				if query.Get("filter") != "active" {
					t.Errorf("Expected filter=active, got %s", query.Get("filter"))
				}
			},
		},
		{
			name: "Request with cookies",
			request: testRequestBuilder().
				Method("GET").
				URL("https://api.example.com/users").
				Context(context.Background()).
				Cookies([]http.Cookie{
					{Name: "session_id", Value: "abc123"},
					{Name: "user_pref", Value: "dark_mode"},
				}).
				Build(),
			validate: func(t *testing.T, req *http.Request) {
				cookies := req.Cookies()
				if len(cookies) != 2 {
					t.Errorf("Expected 2 cookies, got %d", len(cookies))
				}

				foundSession := false
				foundPref := false
				for _, cookie := range cookies {
					if cookie.Name == "session_id" && cookie.Value == "abc123" {
						foundSession = true
					}
					if cookie.Name == "user_pref" && cookie.Value == "dark_mode" {
						foundPref = true
					}
				}

				if !foundSession {
					t.Error("Session cookie not found")
				}
				if !foundPref {
					t.Error("Preference cookie not found")
				}
			},
		},
		{
			name: "Request with timeout",
			request: testRequestBuilder().
				Method("GET").
				URL("https://api.example.com/users").
				Context(context.Background()).
				Timeout(15 * time.Second).
				Build(),
			validate: func(t *testing.T, req *http.Request) {
				if req.Context() == nil {
					t.Error("Expected context to be set")
				}
			},
		},
		{
			name: "Request with string body",
			request: testRequestBuilder().
				Method("POST").
				URL("https://api.example.com/data").
				Context(context.Background()).
				Headers(map[string]string{
					"Content-Type": "text/plain",
				}).
				Body("Hello, World!").
				Build(),
			validate: func(t *testing.T, req *http.Request) {
				if req.Body == nil {
					t.Error("Expected request body, got nil")
				}

				bodyBytes, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("Failed to read body: %v", err)
				}

				if string(bodyBytes) != "Hello, World!" {
					t.Errorf("Expected body 'Hello, World!', got '%s'", string(bodyBytes))
				}
			},
		},
		{
			name: "Request with byte array body",
			request: testRequestBuilder().
				Method("POST").
				URL("https://api.example.com/data").
				Context(context.Background()).
				Headers(map[string]string{
					"Content-Type": "application/octet-stream",
				}).
				Body([]byte{0x48, 0x65, 0x6c, 0x6c, 0x6f}). // "Hello"
				Build(),
			validate: func(t *testing.T, req *http.Request) {
				if req.Body == nil {
					t.Error("Expected request body, got nil")
				}

				bodyBytes, err := io.ReadAll(req.Body)
				if err != nil {
					t.Errorf("Failed to read body: %v", err)
				}

				expected := []byte{0x48, 0x65, 0x6c, 0x6c, 0x6f}
				if !bytes.Equal(bodyBytes, expected) {
					t.Errorf("Expected body %v, got %v", expected, bodyBytes)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReq, err := processor.Build(tt.request)
			if err != nil {
				t.Fatalf("Failed to build request: %v", err)
			}

			tt.validate(t, httpReq)
		})
	}
}

func TestRequestProcessor_BuildErrors(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
	}

	processor := newRequestProcessor(config)

	tests := []struct {
		name        string
		request     *Request
		expectError bool
		wantMethod  string // if set, assert the built request's method (post-normalization)
		wantCtxSet  bool   // if true, assert the built request has a non-nil context
	}{
		{
			name: "Invalid URL",
			request: testRequestBuilder().
				Method("GET").
				URL("://invalid-url").
				Context(context.Background()).
				Build(),
			expectError: true,
		},
		{
			// Build normalizes an empty method to "GET" (request.go:633-635) —
			// this is intentional defaulting, not a validation gap.
			name: "Empty method defaults to GET",
			request: testRequestBuilder().
				Method("").
				URL("https://api.example.com/users").
				Context(context.Background()).
				Build(),
			expectError: false,
			wantMethod:  "GET",
		},
		{
			// Build defaults a nil context to context.Background (request.go:637-639).
			name: "Nil context defaults to background",
			request: testRequestBuilder().
				Method("GET").
				URL("https://api.example.com/users").
				Build(),
			expectError: false,
			wantCtxSet:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpReq, err := processor.Build(tt.request)

			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// Verify the documented defaulting behavior when Build succeeds.
			if err == nil && httpReq != nil {
				if tt.wantMethod != "" && httpReq.Method != tt.wantMethod {
					t.Errorf("Method = %q, want %q", httpReq.Method, tt.wantMethod)
				}
				if tt.wantCtxSet && httpReq.Context() == nil {
					t.Error("Expected non-nil context after defaulting")
				}
			}
		})
	}
}

// TestRequestProcessor_BodySerialization covers body-type dispatch in Build.
// Formerly spread across this table plus standalone TestBuild_NilBody,
// TestBuild_IOReaderBody, and TestBuild_XMLBody (coverage_test.go) — the nil,
// io.Reader, and XML cases are rows here now.
func TestRequestProcessor_BodySerialization(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
	}

	processor := newRequestProcessor(config)

	type xmlRequest struct {
		XMLName struct{} `xml:"request"`
		Key     string   `xml:"key"`
	}

	tests := []struct {
		name            string
		method          string // defaults to POST when empty
		headers         map[string]string
		body            any
		wantContentType string // asserted on the built request when non-empty
		validate        func(*testing.T, io.Reader)
	}{
		{
			name: "JSON object",
			body: map[string]interface{}{"key": "value"},
			validate: func(t *testing.T, reader io.Reader) {
				bodyBytes, err := io.ReadAll(reader)
				if err != nil {
					t.Errorf("Failed to read body: %v", err)
				}

				var data map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &data); err != nil {
					t.Errorf("Failed to unmarshal JSON: %v", err)
				}

				if data["key"] != "value" {
					t.Errorf("Expected key=value, got %v", data["key"])
				}
			},
		},
		{
			name: "String body",
			body: "test string",
			validate: func(t *testing.T, reader io.Reader) {
				bodyBytes, err := io.ReadAll(reader)
				if err != nil {
					t.Errorf("Failed to read body: %v", err)
				}

				if string(bodyBytes) != "test string" {
					t.Errorf("Expected 'test string', got '%s'", string(bodyBytes))
				}
			},
		},
		{
			name: "Byte array body",
			body: []byte("test bytes"),
			validate: func(t *testing.T, reader io.Reader) {
				bodyBytes, err := io.ReadAll(reader)
				if err != nil {
					t.Errorf("Failed to read body: %v", err)
				}

				if string(bodyBytes) != "test bytes" {
					t.Errorf("Expected 'test bytes', got '%s'", string(bodyBytes))
				}
			},
		},
		{
			name: "URL values (form)",
			body: url.Values{"key": []string{"value1", "value2"}},
			validate: func(t *testing.T, reader io.Reader) {
				bodyBytes, err := io.ReadAll(reader)
				if err != nil {
					t.Errorf("Failed to read body: %v", err)
				}

				bodyStr := string(bodyBytes)
				// URL values will be serialized as JSON, not form format
				if !strings.Contains(bodyStr, "key") {
					t.Errorf("Expected body to contain 'key', got '%s'", bodyStr)
				}
			},
		},
		{
			// Formerly TestBuild_NilBody: nil body must build cleanly.
			name:   "Nil body builds without error",
			method: "GET",
			body:   nil,
		},
		{
			// Formerly TestBuild_IOReaderBody: a raw io.Reader body passes
			// through to the wire unchanged.
			name:    "io.Reader body passes through",
			headers: map[string]string{"Content-Type": "application/octet-stream"},
			body:    bytes.NewBufferString("raw reader data"),
			validate: func(t *testing.T, reader io.Reader) {
				bodyBytes, err := io.ReadAll(reader)
				if err != nil {
					t.Errorf("Failed to read body: %v", err)
				}
				if string(bodyBytes) != "raw reader data" {
					t.Errorf("Expected 'raw reader data', got '%s'", string(bodyBytes))
				}
			},
		},
		{
			// Formerly TestBuild_XMLBody: an XML payload with an explicit
			// Content-Type is XML-marshaled and keeps the header.
			name:            "XML body keeps explicit Content-Type",
			headers:         map[string]string{"Content-Type": "application/xml"},
			body:            &xmlRequest{Key: "value"},
			wantContentType: "application/xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = "POST"
			}
			builder := testRequestBuilder().
				Method(method).
				URL("https://api.example.com/data").
				Context(context.Background()).
				Body(tt.body)
			if tt.headers != nil {
				builder = builder.Headers(tt.headers)
			}

			httpReq, err := processor.Build(builder.Build())
			if err != nil {
				t.Fatalf("Failed to build request: %v", err)
			}

			if httpReq == nil {
				t.Fatal("Expected HTTP request, got nil")
			}

			if tt.wantContentType != "" && httpReq.Header.Get("Content-Type") != tt.wantContentType {
				t.Errorf("Expected Content-Type %s, got %s", tt.wantContentType, httpReq.Header.Get("Content-Type"))
			}

			if httpReq.Body != nil && tt.validate != nil {
				tt.validate(t, httpReq.Body)
			}
		})
	}
}

// TestBuild_MultipartFormData validates multipart form data with fields and
// files. Moved from coverage_test.go; the former TestBuild_MultipartSpecialChars
// is folded in as the "filename with quotes" subtest (assertions preserved —
// the quoted filename must build successfully with a multipart Content-Type;
// byte-level quote-escaping parity is additionally pinned by
// TestMultipartWireParity in pools_test.go).
func TestBuild_MultipartFormData(t *testing.T) {
	config := &Config{Timeout: 30 * time.Second}
	processor := newRequestProcessor(config)

	build := func(t *testing.T, formData *types.FormData) *http.Request {
		t.Helper()
		req := testRequestBuilder().
			Method("POST").
			URL("https://api.example.com/upload").
			Context(context.Background()).
			Body(formData).
			Build()

		httpReq, err := processor.Build(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		return httpReq
	}

	t.Run("Fields only", func(t *testing.T) {
		formData := formDataHelper(map[string]string{"username": "john"}, nil)
		ct := build(t, formData).Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("Expected multipart content-type, got %s", ct)
		}
	})

	t.Run("Fields and files without content type", func(t *testing.T) {
		files := map[string]*fileDataHelper{
			"file1": {Filename: "test.txt", Content: []byte("file content")},
		}
		formData := formDataHelper(map[string]string{"field1": "value1"}, files)
		ct := build(t, formData).Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("Expected multipart content-type, got %s", ct)
		}
	})

	t.Run("Files with content type", func(t *testing.T) {
		files := map[string]*fileDataHelper{
			"file1": {Filename: "test.png", Content: []byte("png data"), ContentType: "image/png"},
		}
		formData := formDataHelper(nil, files)
		ct := build(t, formData).Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("Expected multipart content-type, got %s", ct)
		}
	})

	t.Run("filename with quotes", func(t *testing.T) {
		files := map[string]*fileDataHelper{
			"file": {Filename: `test "file".txt`, Content: []byte("data"), ContentType: "text/plain"},
		}
		formData := formDataHelper(nil, files)
		ct := build(t, formData).Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("Expected multipart content-type, got %s", ct)
		}
	})

	t.Run("Nil file entry errors", func(t *testing.T) {
		// A nil *FileData map entry means the caller's upload is missing a
		// file. Silently dropping it (the old behavior) loses data without
		// notice — Build must fail loudly instead.
		files := map[string]*fileDataHelper{
			"nil_file": nil,
		}
		formData := formDataHelper(nil, files)
		req := testRequestBuilder().
			Method("POST").
			URL("https://api.example.com/upload").
			Context(context.Background()).
			Body(formData).
			Build()

		httpReq, err := processor.Build(req)
		if err == nil {
			t.Fatal("Expected error for nil FileData entry, got nil")
		}
		if httpReq != nil {
			t.Errorf("Expected nil request on error, got %v", httpReq)
		}
	})
}

func TestRequestProcessor_HeaderHandling(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
		UserAgent:       "TestClient/1.0",
		Headers: map[string]string{
			"X-Default-Header": "default-value",
		},
	}

	processor := newRequestProcessor(config)

	request := testRequestBuilder().
		Method("GET").
		URL("https://api.example.com/users").
		Context(context.Background()).
		Headers(map[string]string{
			"Accept":          "application/json",
			"X-Custom-Header": "custom-value",
		}).
		Build()

	httpReq, err := processor.Build(request)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	// Check default headers
	if httpReq.Header.Get("X-Default-Header") != "default-value" {
		t.Errorf("Expected X-Default-Header 'default-value', got '%s'", httpReq.Header.Get("X-Default-Header"))
	}

	// Check custom headers
	if httpReq.Header.Get("Accept") != "application/json" {
		t.Errorf("Expected Accept 'application/json', got '%s'", httpReq.Header.Get("Accept"))
	}

	if httpReq.Header.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("Expected X-Custom-Header 'custom-value', got '%s'", httpReq.Header.Get("X-Custom-Header"))
	}

	// Check User-Agent
	if httpReq.Header.Get("User-Agent") != "TestClient/1.0" {
		t.Errorf("Expected User-Agent 'TestClient/1.0', got '%s'", httpReq.Header.Get("User-Agent"))
	}
}

func TestRequestProcessor_QueryParameterHandling(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		ValidateURL:     true,
		ValidateHeaders: true,
	}

	processor := newRequestProcessor(config)

	tests := []struct {
		name        string
		baseURL     string
		queryParams map[string]any
		expected    string
	}{
		{
			name:    "URL without existing query",
			baseURL: "https://api.example.com/users",
			queryParams: map[string]any{
				"page":  1,
				"limit": 10,
			},
			expected: "https://api.example.com/users?limit=10&page=1",
		},
		{
			name:    "URL with existing query",
			baseURL: "https://api.example.com/users?sort=name",
			queryParams: map[string]any{
				"page":  1,
				"limit": 10,
			},
			expected: "https://api.example.com/users?sort=name&limit=10&page=1",
		},
		{
			name:    "Mixed parameter types",
			baseURL: "https://api.example.com/search",
			queryParams: map[string]any{
				"q":      "golang",
				"count":  100,
				"active": true,
			},
			expected: "https://api.example.com/search?active=true&count=100&q=golang",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := testRequestBuilder().
				Method("GET").
				URL(tt.baseURL).
				Context(context.Background()).
				QueryParams(tt.queryParams).
				Build()

			httpReq, err := processor.Build(request)
			if err != nil {
				t.Fatalf("Failed to build request: %v", err)
			}

			// Parse expected URL and actual URL for comparison
			expectedURL, err := url.Parse(tt.expected)
			if err != nil {
				t.Fatalf("Failed to parse expected URL: %v", err)
			}

			expectedQuery := expectedURL.Query()
			actualQuery := httpReq.URL.Query()

			for key, expectedValues := range expectedQuery {
				actualValues := actualQuery[key]
				if len(actualValues) != len(expectedValues) {
					t.Errorf("Query param %s: expected %v, got %v", key, expectedValues, actualValues)
					continue
				}

				for i, expectedValue := range expectedValues {
					if actualValues[i] != expectedValue {
						t.Errorf("Query param %s[%d]: expected %s, got %s", key, i, expectedValue, actualValues[i])
					}
				}
			}
		})
	}
}

// ============================================================================
// REQUEST PROCESSOR EDGE CASE TESTS
// ============================================================================

func TestRequestProcessor_EdgeCases(t *testing.T) {
	config := &Config{
		Timeout: 30 * time.Second,

		MaxResponseBodySize: 50 * 1024 * 1024,
	}

	processor := newRequestProcessor(config)

	t.Run("Very long URL", func(t *testing.T) {
		longPath := strings.Repeat("a", 2000)
		request := testRequestBuilder().
			Method("GET").
			URL("https://api.example.com/" + longPath).
			Context(context.Background()).
			Build()

		_, err := processor.Build(request)
		// Should be able to handle long URLs (within reasonable limits)
		if err != nil {
			t.Errorf("Unexpected error for long URL: %v", err)
		}
	})

	t.Run("Many query parameters", func(t *testing.T) {
		params := make(map[string]any)
		for i := 0; i < 100; i++ {
			params[fmt.Sprintf("param%d", i)] = fmt.Sprintf("value%d", i)
		}

		request := testRequestBuilder().
			Method("GET").
			URL("https://api.example.com/test").
			Context(context.Background()).
			QueryParams(params).
			Build()

		httpReq, err := processor.Build(request)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		query := httpReq.URL.Query()
		if len(query) != 100 {
			t.Errorf("Expected 100 query parameters, got %d", len(query))
		}
	})

	t.Run("Many headers", func(t *testing.T) {
		headers := make(map[string]string)
		for i := 0; i < 50; i++ {
			headers[fmt.Sprintf("X-Header-%d", i)] = fmt.Sprintf("value-%d", i)
		}

		request := testRequestBuilder().
			Method("GET").
			URL("https://api.example.com/test").
			Context(context.Background()).
			Headers(headers).
			Build()

		httpReq, err := processor.Build(request)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Check that all headers are set
		for key, expectedValue := range headers {
			actualValue := httpReq.Header.Get(key)
			if actualValue != expectedValue {
				t.Errorf("Expected header %s=%s, got %s", key, expectedValue, actualValue)
			}
		}
	})
}

// TestBuild_AcceptEncodingStreaming verifies the Accept-Encoding contract:
// buffered responses are decompressed by the response processor (advertise
// gzip/deflate), while streaming responses bypass that pipeline and must
// request identity so callers never receive compressed bytes. A user-supplied
// Accept-Encoding always wins in both modes.
func TestBuild_AcceptEncodingStreaming(t *testing.T) {
	config := &Config{Timeout: 30 * time.Second}
	processor := newRequestProcessor(config)

	build := func(stream bool, headers map[string]string) *http.Request {
		t.Helper()
		req := testRequestBuilder().
			Method("GET").
			URL("https://api.example.com/data").
			Context(context.Background()).
			Build()
		req.SetStreamBody(stream)
		if headers != nil {
			req.SetHeaders(headers)
		}
		httpReq, err := processor.Build(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		return httpReq
	}

	if got := build(false, nil).Header.Get("Accept-Encoding"); got != "gzip, deflate" {
		t.Errorf("non-streaming Accept-Encoding = %q, want %q", got, "gzip, deflate")
	}
	if got := build(true, nil).Header.Get("Accept-Encoding"); got != "identity" {
		t.Errorf("streaming Accept-Encoding = %q, want %q", got, "identity")
	}
	if got := build(true, map[string]string{"Accept-Encoding": "br"}).Header.Get("Accept-Encoding"); got != "br" {
		t.Errorf("user override Accept-Encoding = %q, want %q", got, "br")
	}
}

// TestBuild_HeaderKeyCasingDeterministic guards the canonical-key storage fix
// at the wire-format boundary: one header set under two casings with different
// values must build to a single canonical header carrying the last-written
// value on EVERY build. Before keys were canonicalized at store time, both raw
// entries survived until this loop and the surviving value followed Go's
// random map iteration order.
func TestBuild_HeaderKeyCasingDeterministic(t *testing.T) {
	config := &Config{Timeout: 30 * time.Second}
	processor := newRequestProcessor(config)

	for i := 0; i < 20; i++ {
		req := testRequestBuilder().
			Method("GET").
			URL("https://api.example.com/data").
			Context(context.Background()).
			Build()
		req.SetHeader("cache-control", "max-age=0")
		req.SetHeader("Cache-Control", "no-store")

		httpReq, err := processor.Build(req)
		if err != nil {
			t.Fatalf("build %d: unexpected error: %v", i, err)
		}
		if vals := httpReq.Header["Cache-Control"]; len(vals) != 1 {
			t.Fatalf("build %d: Cache-Control values = %v, want exactly one", i, vals)
		}
		if got := httpReq.Header.Get("Cache-Control"); got != "no-store" {
			t.Fatalf("build %d: Cache-Control = %q, want last-written %q", i, got, "no-store")
		}
	}
}

// TestBuild_URLUserinfoBasicAuth verifies URL credentials are converted to a
// Basic Authorization header (net/http parity) instead of being silently
// stripped, and that an explicit Authorization header wins.
func TestBuild_URLUserinfoBasicAuth(t *testing.T) {
	config := &Config{Timeout: 30 * time.Second}
	processor := newRequestProcessor(config)

	build := func(rawURL string, headers map[string]string) *http.Request {
		t.Helper()
		req := testRequestBuilder().
			Method("GET").
			URL(rawURL).
			Context(context.Background()).
			Build()
		if headers != nil {
			req.SetHeaders(headers)
		}
		httpReq, err := processor.Build(req)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		return httpReq
	}

	// base64("user:pass") == "dXNlcjpwYXNz"
	httpReq := build("https://user:pass@api.example.com/data", nil)
	if got := httpReq.Header.Get("Authorization"); got != "Basic dXNlcjpwYXNz" {
		t.Errorf("userinfo not converted to Basic auth: got %q", got)
	}
	if httpReq.URL.User != nil {
		t.Errorf("credentials not stripped from request URL: %v", httpReq.URL.User)
	}

	// Explicit Authorization header wins over URL userinfo.
	httpReq = build("https://user:pass@api.example.com/data", map[string]string{"Authorization": "Bearer token123"})
	if got := httpReq.Header.Get("Authorization"); got != "Bearer token123" {
		t.Errorf("explicit Authorization should win, got %q", got)
	}

	// URLs without userinfo are unaffected (the "@" pre-filter must not
	// misfire on "@" in query strings).
	httpReq = build("https://api.example.com/data?email=a@b.com", nil)
	if got := httpReq.Header.Get("Authorization"); got != "" {
		t.Errorf("no userinfo: Authorization should be absent, got %q", got)
	}
}

// TestIsXMLContentType covers parameter-tolerant, case-insensitive XML
// Content-Type detection used by requestProcessor.Build.
// (Moved from query_param_parity_test.go, which was dissolved: its wire-
// encoding contract test is subsumed by
// TestWriteQueryParamValue_MatchesQueryEscapeFormatQueryParam in pools_test.go.)
func TestIsXMLContentType(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"application/xml", true},
		{"application/xml; charset=utf-8", true},
		{"APPLICATION/XML", true},
		{"text/xml", true},
		{" text/xml ; charset=iso-8859-1 ", true},
		{"application/json", false},
		{"", false},
		{"application/xmlx", false},
		{"text/xml-ish", false},
	}
	for _, tc := range cases {
		if got := isXMLContentType(tc.in); got != tc.want {
			t.Errorf("isXMLContentType(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
