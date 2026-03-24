package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// NewTestServer creates an httptest.Server with common configuration.
// The server is returned for convenience.
func NewTestServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(handler)
}

// NewJSONTestServer creates a server that responds with JSON.
func NewJSONTestServer(t *testing.T, response interface{}, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Logf("Failed to encode JSON response: %v", err)
		}
	}))
}

// NewSlowTestServer creates a server that delays its response.
func NewSlowTestServer(delay time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow response"))
	}))
}

// AssertError asserts that an error occurred.
func AssertError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		t.Fatalf("Expected error but got nil. %v", msgAndArgs)
	}
}

// AssertNoError asserts that no error occurred.
func AssertNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		t.Fatalf("Unexpected error: %v. %v", err, msgAndArgs)
	}
}

// AssertStatusCode asserts the HTTP status code matches expected.
func AssertStatusCode(t *testing.T, got, expected int) {
	t.Helper()
	if got != expected {
		t.Errorf("Status code = %d, want %d", got, expected)
	}
}

// AssertCookieValid validates a cookie meets basic requirements.
func AssertCookieValid(t *testing.T, cookie *http.Cookie, name, value string) {
	t.Helper()
	if cookie == nil {
		t.Fatal("Cookie is nil")
	}
	if cookie.Name != name {
		t.Errorf("Cookie name = %q, want %q", cookie.Name, name)
	}
	if cookie.Value != value {
		t.Errorf("Cookie value = %q, want %q", cookie.Value, value)
	}
}

// RetryableStatusCodes returns status codes that should trigger retries.
func RetryableStatusCodes() []int {
	return []int{
		http.StatusRequestTimeout,      // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}
}

// NonRetryableStatusCodes returns status codes that should NOT trigger retries.
func NonRetryableStatusCodes() []int {
	return []int{
		http.StatusOK,                  // 200
		http.StatusBadRequest,          // 400
		http.StatusUnauthorized,        // 401
		http.StatusForbidden,           // 403
		http.StatusNotFound,            // 404
		http.StatusMethodNotAllowed,    // 405
		http.StatusUnprocessableEntity, // 422
	}
}

// StatusCodeTest represents a test case for status code handling.
type StatusCodeTest struct {
	Name       string
	StatusCode int
	Retryable  bool
}

// AllStatusCodeTests returns all status code test cases.
func AllStatusCodeTests() []StatusCodeTest {
	tests := make([]StatusCodeTest, 0, len(RetryableStatusCodes())+len(NonRetryableStatusCodes()))

	for _, code := range RetryableStatusCodes() {
		tests = append(tests, StatusCodeTest{
			Name:       http.StatusText(code),
			StatusCode: code,
			Retryable:  true,
		})
	}

	for _, code := range NonRetryableStatusCodes() {
		tests = append(tests, StatusCodeTest{
			Name:       http.StatusText(code),
			StatusCode: code,
			Retryable:  false,
		})
	}

	return tests
}
