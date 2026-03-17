package engine

import (
	"net/http"
	"testing"
	"time"
)

func TestResponse_Accessors(t *testing.T) {
	cookies := []*http.Cookie{
		{Name: "session", Value: "abc123"},
	}
	resp := &Response{}
	resp.SetStatusCode(200)
	resp.SetStatus("200 OK")
	resp.SetHeaders(make(http.Header))
	resp.SetBody("test response")
	resp.SetRawBody([]byte("test response"))
	resp.SetContentLength(13)
	resp.SetDuration(100 * time.Millisecond)
	resp.SetAttempts(1)
	resp.SetCookies(cookies)

	if resp.StatusCode() != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode())
	}

	if resp.Status() != "200 OK" {
		t.Errorf("Expected status '200 OK', got %s", resp.Status())
	}

	if resp.Body() != "test response" {
		t.Errorf("Expected body 'test response', got %s", resp.Body())
	}

	if string(resp.RawBody()) != "test response" {
		t.Errorf("Expected raw body 'test response', got %s", resp.RawBody())
	}

	if resp.ContentLength() != 13 {
		t.Errorf("Expected content length 13, got %d", resp.ContentLength())
	}

	if resp.Duration() != 100*time.Millisecond {
		t.Errorf("Expected duration 100ms, got %v", resp.Duration())
	}

	if resp.Attempts() != 1 {
		t.Errorf("Expected 1 attempt, got %d", resp.Attempts())
	}

	if len(resp.Cookies()) != 1 {
		t.Errorf("Expected 1 cookie, got %d", len(resp.Cookies()))
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
		t.Run(tc.status, func(t *testing.T) {
			resp := &Response{}
			resp.SetStatusCode(tc.code)
			resp.SetStatus(tc.status)

			if resp.StatusCode() != tc.code {
				t.Errorf("Expected status code %d, got %d", tc.code, resp.StatusCode())
			}

			if resp.Status() != tc.status {
				t.Errorf("Expected status %s, got %s", tc.status, resp.Status())
			}
		})
	}
}
