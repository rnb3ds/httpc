package engine

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestRequest_Validation was removed: it never invoked any validation —
// it compared builder-set values back against the table that supplied
// them (wantErr was decorative). Real validation paths are exercised in
// request_processor_test.go and the client option tests.

// TestRequest_WithOptionsAccessors consolidates the six former sequential
// TestRequest_With{Timeout,Headers,QueryParams,Cookies,Body,MaxRetries} tests
// into one table: each row drives the production Set* path (via the shared
// testRequestBuilder helper, which forwards to the setters) and asserts the
// matching getter round-trips the value.
func TestRequest_WithOptionsAccessors(t *testing.T) {
	tests := []struct {
		name  string
		apply func(*testRequest) *testRequest
		check func(*testing.T, *Request)
	}{
		{
			name:  "Timeout",
			apply: func(b *testRequest) *testRequest { return b.Timeout(5 * time.Second) },
			check: func(t *testing.T, req *Request) {
				if req.Timeout() != 5*time.Second {
					t.Errorf("Expected timeout 5s, got %v", req.Timeout())
				}
			},
		},
		{
			name: "Headers",
			apply: func(b *testRequest) *testRequest {
				return b.Headers(map[string]string{
					"Authorization": "Bearer token",
					"Content-Type":  "application/json",
				})
			},
			check: func(t *testing.T, req *Request) {
				if req.Headers()["Authorization"] != "Bearer token" {
					t.Error("Authorization header not set correctly")
				}

				if req.Headers()["Content-Type"] != "application/json" {
					t.Error("Content-Type header not set correctly")
				}
			},
		},
		{
			name: "QueryParams",
			apply: func(b *testRequest) *testRequest {
				return b.QueryParams(map[string]any{
					"page":  1,
					"limit": 10,
					"sort":  "name",
				})
			},
			check: func(t *testing.T, req *Request) {
				if req.QueryParams()["page"] != 1 {
					t.Error("Page query param not set correctly")
				}

				if req.QueryParams()["limit"] != 10 {
					t.Error("Limit query param not set correctly")
				}

				if req.QueryParams()["sort"] != "name" {
					t.Error("Sort query param not set correctly")
				}
			},
		},
		{
			name: "Cookies",
			apply: func(b *testRequest) *testRequest {
				return b.Cookies([]http.Cookie{
					{Name: "session", Value: "abc123"},
					{Name: "theme", Value: "dark"},
				})
			},
			check: func(t *testing.T, req *Request) {
				if len(req.Cookies()) != 2 {
					t.Errorf("Expected 2 cookies, got %d", len(req.Cookies()))
				}

				reqCookies := req.Cookies()
				if reqCookies[0].Name != "session" || reqCookies[0].Value != "abc123" {
					t.Error("Session cookie not set correctly")
				}
			},
		},
		{
			name:  "Body",
			apply: func(b *testRequest) *testRequest { return b.Body(map[string]string{"key": "value"}) },
			check: func(t *testing.T, req *Request) {
				if req.Body() == nil {
					t.Error("Request body should not be nil")
				}

				bodyMap, ok := req.Body().(map[string]string)
				if !ok {
					t.Error("Request body should be map[string]string")
				}

				if bodyMap["key"] != "value" {
					t.Error("Request body not set correctly")
				}
			},
		},
		{
			name:  "MaxRetries",
			apply: func(b *testRequest) *testRequest { return b.MaxRetries(3) },
			check: func(t *testing.T, req *Request) {
				if req.MaxRetries() != 3 {
					t.Errorf("Expected MaxRetries 3, got %d", req.MaxRetries())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := testRequestBuilder().
				Method("GET").
				URL("https://example.com").
				Context(context.Background())
			req := tt.apply(builder).Build()
			tt.check(t, req)
		})
	}
}

// TestRequest_Clone was removed: no Clone logic exists on *Request (grep of
// request.go confirms — only bytes.Clone in the replay buffer). The test
// mutated the map returned by Headers() and read it back, guarding nothing.

// TestBodyLimitReader_ExactBoundary pins the request-side size-limit reader
// at unit level: reading exactly the limit succeeds, one byte over fails with
// the same wording validateRequestBodySize uses, so the failure reads the
// same regardless of which layer caught it.
func TestBodyLimitReader_ExactBoundary(t *testing.T) {
	t.Run("exactly at limit reads cleanly", func(t *testing.T) {
		lr := newBodyLimitReader(strings.NewReader("12345678"), 8)
		data, err := io.ReadAll(lr)
		if err != nil {
			t.Fatalf("read at exact limit: unexpected error %v", err)
		}
		if string(data) != "12345678" {
			t.Errorf("read at exact limit: got %q, want %q", data, "12345678")
		}
	})

	t.Run("one byte over fails with the validator's wording", func(t *testing.T) {
		lr := newBodyLimitReader(strings.NewReader("123456789"), 8)
		_, err := io.ReadAll(lr)
		if err == nil {
			t.Fatal("read over limit: expected error, got success (silent truncation)")
		}
		if err.Error() != "request body exceeds limit of 8 bytes" {
			t.Errorf("error = %q, want exact message %q", err.Error(), "request body exceeds limit of 8 bytes")
		}
	})
}

// TestURLCache tests were consolidated into url_cache_test.go.

// TestRequest_SetHeader_NilMap validates that SetHeader auto-creates the headers map.
// (Moved from coverage_test.go during its dissolution.)
func TestRequest_SetHeader_NilMap(t *testing.T) {
	req := &Request{}
	if req.Headers() != nil {
		t.Error("Expected nil headers initially")
	}

	req.SetHeader("Authorization", "Bearer token")

	if req.Headers() == nil {
		t.Error("Expected headers map to be auto-created")
	}
	if req.Headers()["Authorization"] != "Bearer token" {
		t.Errorf("Expected Authorization=Bearer token, got %s", req.Headers()["Authorization"])
	}
}

// TestRequest_AccessorRoundTrip locks the round-trip contract of the small
// Request accessors/mutators that are not exercised elsewhere
// (SanitizedURL/SetSanitizedURL, EnsureQueryParams lazy-init + identity,
// SetAllowPrivateIPs, OnRequest/OnResponse callbacks). Consolidated into one
// test to avoid accessor sprawl.
func TestRequest_AccessorRoundTrip(t *testing.T) {
	r := &Request{maxRetries: maxRetriesUnset}

	r.SetSanitizedURL("https://sanitized.example.com/x")
	if r.SanitizedURL() != "https://sanitized.example.com/x" {
		t.Errorf("SanitizedURL round-trip failed: %q", r.SanitizedURL())
	}

	// EnsureQueryParams lazily allocates a pooled map and returns the SAME
	// map on subsequent calls.
	qp := r.EnsureQueryParams()
	if qp == nil {
		t.Fatal("EnsureQueryParams returned nil")
	}
	qp["page"] = 1
	if r.EnsureQueryParams()["page"] != 1 {
		t.Error("EnsureQueryParams should return the same map instance")
	}

	// AllowPrivateIPs override round-trips through the *bool pointer.
	allow := true
	r.SetAllowPrivateIPs(&allow)
	if r.AllowPrivateIPs() == nil || *r.AllowPrivateIPs() != true {
		t.Error("SetAllowPrivateIPs round-trip failed")
	}

	// (Folded in from the former standalone TestRequest_OnRequestOnResponse
	// in coverage_test.go.)
	t.Run("OnRequestOnResponse", func(t *testing.T) {
		req := &Request{}

		if req.OnRequest() != nil {
			t.Error("Expected nil OnRequest initially")
		}
		if req.OnResponse() != nil {
			t.Error("Expected nil OnResponse initially")
		}

		req.SetOnRequest(func(r *Request) error {
			return nil
		})
		if req.OnRequest() == nil {
			t.Error("Expected OnRequest to be set")
		}

		req.SetOnResponse(func(r *Response) error {
			return nil
		})
		if req.OnResponse() == nil {
			t.Error("Expected OnResponse to be set")
		}
	})
}

// ---------------------------------------------------------------------------
// Cookie header sanitization (request.go)
// ---------------------------------------------------------------------------

// TestSanitizeCookieNameValue pins the Cookie header-injection defenses and
// net/http parity: CR/LF in names are REPLACED with "-" (net/http replaces,
// it does not delete), invalid value bytes are DROPPED, and a value
// containing a space or comma is double-quoted \u2014 RFC 6265's quoted-string
// form. (The former table pinned octal \ooo escaping, pre-RFC-6265 stdlib
// behavior that modern net/http no longer performs.)
func TestSanitizeCookieNameValue(t *testing.T) {
	t.Run("sanitizeCookieName", func(t *testing.T) {
		tests := []struct {
			name, in, want string
		}{
			{"clean name passthrough", "session", "session"},
			{"LF replaced", "bad\nname", "bad-name"},
			{"CR replaced", "bad\rname", "bad-name"},
			{"both replaced", "a\r\nb", "a--b"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := sanitizeCookieName(tt.in); got != tt.want {
					t.Errorf("sanitizeCookieName(%q) = %q, want %q", tt.in, got, tt.want)
				}
			})
		}
	})

	t.Run("sanitizeCookieValue", func(t *testing.T) {
		tests := []struct {
			name, in, want string
		}{
			{"clean value passthrough (fast path)", "abc123", "abc123"},
			{"printable specials stay clean", "a:b/c?d#e", "a:b/c?d#e"},
			{"quote dropped", "a\"b", "ab"},
			{"semicolon dropped", "a;b", "ab"},
			{"backslash dropped", "a\\b", "ab"},
			{"control byte dropped", "a\x01b", "ab"},
			{"non-ASCII dropped as UTF-8 byte pair (slow path)", "a\u00e4b", "ab"},
			{"invalid at start", "\x00x", "x"},
			{"space value quoted", "a b", `"a b"`},
			{"comma value quoted", "a,b", `"a,b"`},
			{"drop then quote", "a b;c", `"a bc"`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if got := sanitizeCookieValue(tt.in); got != tt.want {
					t.Errorf("sanitizeCookieValue(%q) = %q, want %q", tt.in, got, tt.want)
				}
			})
		}
	})
}

// TestPooledStringsReader_NoReleaseOnEOF moved to pooled_readers_test.go with
// the other pooled reader/wrapper release-discipline coverage.

// TestBuild_MultipartCRLFInjectionRejected verifies that control characters
// in multipart field names, filenames, and per-file Content-Types are
// rejected at the encoding sink. *FormData is a public struct constructible
// without going through WithFile's validation, and mime/multipart.Writer
// writes Content-Disposition values verbatim — CRLF in a token would inject
// arbitrary MIME headers/parts into the outgoing body.
func TestBuild_MultipartCRLFInjectionRejected(t *testing.T) {
	tests := []struct {
		name    string
		fields  map[string]string
		files   map[string]*fileDataHelper
		wantErr bool
	}{
		{
			name:    "CRLF in field name",
			fields:  map[string]string{"field\r\nX-Injected: 1": "v"},
			wantErr: true,
		},
		{
			name:   "CRLF in field value is legal (part body, not a header)",
			fields: map[string]string{"note": "line1\r\nline2"},
			files:  map[string]*fileDataHelper{},
		},
		{
			name:    "CRLF in filename",
			files:   map[string]*fileDataHelper{"f": {Filename: "a\"\r\nX-Injected: 1\r\n.txt", Content: []byte("x")}},
			wantErr: true,
		},
		{
			name:    "CRLF in file field name",
			files:   map[string]*fileDataHelper{"f\r\nX-Injected: 1": {Filename: "a.txt", Content: []byte("x")}},
			wantErr: true,
		},
		{
			name:    "CRLF in file Content-Type",
			files:   map[string]*fileDataHelper{"f": {Filename: "a.txt", Content: []byte("x"), ContentType: "text/plain\r\nX-Injected: 1"}},
			wantErr: true,
		},
		{
			name:    "NUL in filename",
			files:   map[string]*fileDataHelper{"f": {Filename: "a\x00.txt", Content: []byte("x")}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fields map[string]string
			if tt.fields != nil {
				fields = tt.fields
			}
			formData := formDataHelper(fields, tt.files)
			req := testRequestBuilder().
				Method("POST").
				URL("https://api.example.com/upload").
				Context(context.Background()).
				Body(formData).
				Build()

			httpReq, err := newRequestProcessor(&Config{Timeout: 30 * time.Second}).Build(req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Build() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				bodyBytes, readErr := io.ReadAll(httpReq.Body)
				if readErr != nil {
					t.Fatalf("read body: %v", readErr)
				}
				if strings.Contains(string(bodyBytes), "X-Injected") {
					t.Fatalf("injection leaked into multipart body:\n%s", bodyBytes)
				}
			}
		})
	}
}

// TestRequest_SetHeadersAndQueryParams_Nil pins the nil-input contract of the
// pooled-map setters: a nil map must clear the field (not allocate a pooled
// map), and a non-nil map must be COPIED so the caller's map can never leak
// into a recycled request (pool-poisoning guard).
func TestRequest_SetHeadersAndQueryParams_Nil(t *testing.T) {
	t.Run("SetHeaders nil clears", func(t *testing.T) {
		r := AcquireRequest()
		defer ReleaseRequest(r)
		r.SetHeader("X-A", "1")
		r.SetHeaders(nil)
		if r.headers != nil {
			t.Errorf("SetHeaders(nil) should clear headers, got %v", r.headers)
		}
	})

	t.Run("SetHeaders copies caller map", func(t *testing.T) {
		r := AcquireRequest()
		defer ReleaseRequest(r)
		src := map[string]string{"X-A": "1"}
		r.SetHeaders(src)
		src["X-A"] = "mutated" // caller mutates its map afterwards
		if got := r.headers["X-A"]; got != "1" {
			t.Errorf("SetHeaders stored caller map by reference: got %q, want %q", got, "1")
		}
	})

	t.Run("SetQueryParams nil clears", func(t *testing.T) {
		r := AcquireRequest()
		defer ReleaseRequest(r)
		r.EnsureQueryParams()["q"] = "1"
		r.SetQueryParams(nil)
		if r.queryParams != nil {
			t.Errorf("SetQueryParams(nil) should clear queryParams, got %v", r.queryParams)
		}
	})

	t.Run("SetQueryParams copies caller map", func(t *testing.T) {
		r := AcquireRequest()
		defer ReleaseRequest(r)
		src := map[string]any{"page": 1}
		r.SetQueryParams(src)
		src["page"] = 99
		if got := r.queryParams["page"]; got != 1 {
			t.Errorf("SetQueryParams stored caller map by reference: got %v, want 1", got)
		}
	})
}

// TestRequest_HeaderKeysCanonical pins the canonical-key storage contract:
// SetHeader and SetHeaders store keys in http.CanonicalHeaderKey form, so one
// header set under multiple casings collapses onto a single entry with the
// last write winning deterministically. Previously both raw keys coexisted
// and the build-time canonicalizer let Go's random map iteration order pick
// the surviving value.
func TestRequest_HeaderKeysCanonical(t *testing.T) {
	t.Run("SetHeader canonicalizes the stored key", func(t *testing.T) {
		r := AcquireRequest()
		defer ReleaseRequest(r)
		r.SetHeader("cache-control", "max-age=0")
		if got, ok := r.headers["Cache-Control"]; !ok || got != "max-age=0" {
			t.Errorf("headers[Cache-Control] = %q, %v; want %q, true", got, ok, "max-age=0")
		}
		if len(r.headers) != 1 {
			t.Errorf("expected 1 stored entry, got %d (%v)", len(r.headers), r.headers)
		}
	})

	t.Run("same header two casings collapses, last write wins", func(t *testing.T) {
		r := AcquireRequest()
		defer ReleaseRequest(r)
		r.SetHeader("Cache-Control", "no-store")
		r.SetHeader("cache-control", "max-age=0")
		if len(r.headers) != 1 {
			t.Fatalf("expected 1 stored entry, got %d (%v)", len(r.headers), r.headers)
		}
		if got := r.headers["Cache-Control"]; got != "max-age=0" {
			t.Errorf("collapsed value = %q, want last-written %q", got, "max-age=0")
		}
	})

	t.Run("SetHeaders canonicalizes and collapses deterministically", func(t *testing.T) {
		r := AcquireRequest()
		defer ReleaseRequest(r)
		r.SetHeaders(map[string]string{
			"Accept-Language": "en-US",
			"accept-language": "zh-CN", // 'a'(0x61) sorts after 'A'(0x41), must win
		})
		if len(r.headers) != 1 {
			t.Fatalf("expected 1 stored entry, got %d (%v)", len(r.headers), r.headers)
		}
		if got := r.headers["Accept-Language"]; got != "zh-CN" {
			t.Errorf("collapsed value = %q, want later-sorted %q", got, "zh-CN")
		}
	})

	t.Run("uncanonicalizable key stored unchanged", func(t *testing.T) {
		// Middleware calls SetHeader directly, bypassing option validation,
		// so keys CanonicalHeaderKey leaves untouched must round-trip as-is.
		r := AcquireRequest()
		defer ReleaseRequest(r)
		r.SetHeader("weird key", "v")
		if got, ok := r.headers["weird key"]; !ok || got != "v" {
			t.Errorf("headers[weird key] = %q, %v; want %q, true", got, ok, "v")
		}
	})
}
