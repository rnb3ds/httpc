package engine

import (
	"net/http"
	"testing"
	"time"
)

func TestResponse_Creation(t *testing.T) {
	cookies := []*http.Cookie{
		{Name: "session", Value: "abc123"},
	}

	resp := &Response{
		StatusCode:    200,
		Status:        "200 OK",
		Headers:       make(http.Header),
		Body:          "test response",
		RawBody:       []byte("test response"),
		ContentLength: 13,
		Duration:      100 * time.Millisecond,
		Attempts:      1,
		Cookies:       cookies,
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	if resp.Status != "200 OK" {
		t.Errorf("Expected status '200 OK', got %s", resp.Status)
	}

	if resp.Body != "test response" {
		t.Errorf("Expected body 'test response', got %s", resp.Body)
	}

	if string(resp.RawBody) != "test response" {
		t.Errorf("Expected raw body 'test response', got %s", resp.RawBody)
	}

	if resp.ContentLength != 13 {
		t.Errorf("Expected content length 13, got %d", resp.ContentLength)
	}

	if resp.Duration != 100*time.Millisecond {
		t.Errorf("Expected duration 100ms, got %v", resp.Duration)
	}

	if resp.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", resp.Attempts)
	}

	if len(resp.Cookies) != 1 {
		t.Errorf("Expected 1 cookie, got %d", len(resp.Cookies))
	}

	if resp.Cookies[0].Name != "session" {
		t.Errorf("Expected cookie name 'session', got %s", resp.Cookies[0].Name)
	}
}

func TestResponse_Headers(t *testing.T) {
	resp := &Response{
		Headers: make(http.Header),
	}

	resp.Headers["Content-Type"] = []string{"application/json"}
	resp.Headers["Cache-Control"] = []string{"no-cache"}

	if resp.Headers["Content-Type"][0] != "application/json" {
		t.Error("Content-Type header not set correctly")
	}

	if resp.Headers["Cache-Control"][0] != "no-cache" {
		t.Error("Cache-Control header not set correctly")
	}
}

func TestResponse_MultipleAttempts(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Attempts:   3,
		Duration:   300 * time.Millisecond,
	}

	if resp.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", resp.Attempts)
	}

	// Average duration per attempt
	avgDuration := resp.Duration / time.Duration(resp.Attempts)
	expectedAvg := 100 * time.Millisecond

	if avgDuration != expectedAvg {
		t.Errorf("Expected average duration %v, got %v", expectedAvg, avgDuration)
	}
}

func TestResponse_EmptyBody(t *testing.T) {
	resp := &Response{
		StatusCode:    204,
		Status:        "204 No Content",
		Body:          "",
		RawBody:       []byte{},
		ContentLength: 0,
	}

	if resp.Body != "" {
		t.Errorf("Expected empty body, got %s", resp.Body)
	}

	if len(resp.RawBody) != 0 {
		t.Errorf("Expected empty raw body, got %d bytes", len(resp.RawBody))
	}

	if resp.ContentLength != 0 {
		t.Errorf("Expected content length 0, got %d", resp.ContentLength)
	}
}

func TestResponse_LargeBody(t *testing.T) {
	largeContent := make([]byte, 1024*1024) // 1MB
	for i := range largeContent {
		largeContent[i] = 'x'
	}

	resp := &Response{
		StatusCode:    200,
		Body:          string(largeContent),
		RawBody:       largeContent,
		ContentLength: int64(len(largeContent)),
	}

	if len(resp.RawBody) != 1024*1024 {
		t.Errorf("Expected 1MB raw body, got %d bytes", len(resp.RawBody))
	}

	if resp.ContentLength != 1024*1024 {
		t.Errorf("Expected content length 1MB, got %d", resp.ContentLength)
	}
}

func TestResponse_NilCookies(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Cookies:    nil,
	}

	if resp.Cookies != nil {
		t.Error("Expected nil cookies")
	}
}

func TestResponse_MultipleCookies(t *testing.T) {
	cookies := []*http.Cookie{
		{Name: "session", Value: "abc123", Path: "/"},
		{Name: "theme", Value: "dark", Path: "/"},
		{Name: "lang", Value: "en", Path: "/"},
	}

	resp := &Response{
		StatusCode: 200,
		Cookies:    cookies,
	}

	if len(resp.Cookies) != 3 {
		t.Errorf("Expected 3 cookies, got %d", len(resp.Cookies))
	}

	// Check each cookie
	expectedCookies := map[string]string{
		"session": "abc123",
		"theme":   "dark",
		"lang":    "en",
	}

	for _, cookie := range resp.Cookies {
		expectedValue, exists := expectedCookies[cookie.Name]
		if !exists {
			t.Errorf("Unexpected cookie: %s", cookie.Name)
		}
		if cookie.Value != expectedValue {
			t.Errorf("Expected cookie %s value %s, got %s", cookie.Name, expectedValue, cookie.Value)
		}
	}
}

func TestResponse_ZeroDuration(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Duration:   0,
		Attempts:   1,
	}

	if resp.Duration != 0 {
		t.Errorf("Expected zero duration, got %v", resp.Duration)
	}
}

func TestResponse_StatusCodes(t *testing.T) {
	testCases := []struct {
		code   int
		status string
	}{
		{200, "200 OK"},
		{201, "201 Created"},
		{400, "400 Bad Request"},
		{404, "404 Not Found"},
		{500, "500 Internal Server Error"},
	}

	for _, tc := range testCases {
		resp := &Response{
			StatusCode: tc.code,
			Status:     tc.status,
		}

		if resp.StatusCode != tc.code {
			t.Errorf("Expected status code %d, got %d", tc.code, resp.StatusCode)
		}

		if resp.Status != tc.status {
			t.Errorf("Expected status %s, got %s", tc.status, resp.Status)
		}
	}
}
