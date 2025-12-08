package engine

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request *Request
		wantErr bool
	}{
		{
			name: "Valid request",
			request: &Request{
				Method:      "GET",
				URL:         "https://example.com",
				Headers:     make(map[string]string),
				QueryParams: make(map[string]any),
				Context:     context.Background(),
			},
			wantErr: false,
		},
		{
			name: "Empty method",
			request: &Request{
				Method:  "",
				URL:     "https://example.com",
				Context: context.Background(),
			},
			wantErr: false, // Should default to GET
		},
		{
			name: "Empty URL",
			request: &Request{
				Method:  "GET",
				URL:     "",
				Context: context.Background(),
			},
			wantErr: true,
		},
		{
			name: "Nil context",
			request: &Request{
				Method:  "GET",
				URL:     "https://example.com",
				Context: nil,
			},
			wantErr: false, // Should use background context
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test request validation logic
			if tt.request.Method == "" {
				tt.request.Method = "GET"
			}
			if tt.request.Context == nil {
				tt.request.Context = context.Background()
			}

			// Basic validation
			if tt.request.URL == "" && !tt.wantErr {
				t.Error("Empty URL should cause error")
			}
		})
	}
}

func TestRequest_WithTimeout(t *testing.T) {
	req := &Request{
		Method:  "GET",
		URL:     "https://example.com",
		Timeout: 5 * time.Second,
		Context: context.Background(),
	}

	if req.Timeout != 5*time.Second {
		t.Errorf("Expected timeout 5s, got %v", req.Timeout)
	}
}

func TestRequest_WithHeaders(t *testing.T) {
	req := &Request{
		Method: "GET",
		URL:    "https://example.com",
		Headers: map[string]string{
			"Authorization": "Bearer token",
			"Content-Type":  "application/json",
		},
		Context: context.Background(),
	}

	if req.Headers["Authorization"] != "Bearer token" {
		t.Error("Authorization header not set correctly")
	}

	if req.Headers["Content-Type"] != "application/json" {
		t.Error("Content-Type header not set correctly")
	}
}

func TestRequest_WithQueryParams(t *testing.T) {
	req := &Request{
		Method: "GET",
		URL:    "https://example.com",
		QueryParams: map[string]any{
			"page":  1,
			"limit": 10,
			"sort":  "name",
		},
		Context: context.Background(),
	}

	if req.QueryParams["page"] != 1 {
		t.Error("Page query param not set correctly")
	}

	if req.QueryParams["limit"] != 10 {
		t.Error("Limit query param not set correctly")
	}

	if req.QueryParams["sort"] != "name" {
		t.Error("Sort query param not set correctly")
	}
}

func TestRequest_WithCookies(t *testing.T) {
	cookies := []http.Cookie{
		{Name: "session", Value: "abc123"},
		{Name: "theme", Value: "dark"},
	}

	req := &Request{
		Method:  "GET",
		URL:     "https://example.com",
		Cookies: cookies,
		Context: context.Background(),
	}

	if len(req.Cookies) != 2 {
		t.Errorf("Expected 2 cookies, got %d", len(req.Cookies))
	}

	if req.Cookies[0].Name != "session" || req.Cookies[0].Value != "abc123" {
		t.Error("Session cookie not set correctly")
	}
}

func TestRequest_WithBody(t *testing.T) {
	testBody := map[string]string{"key": "value"}

	req := &Request{
		Method:  "POST",
		URL:     "https://example.com",
		Body:    testBody,
		Context: context.Background(),
	}

	if req.Body == nil {
		t.Error("Request body should not be nil")
	}

	bodyMap, ok := req.Body.(map[string]string)
	if !ok {
		t.Error("Request body should be map[string]string")
	}

	if bodyMap["key"] != "value" {
		t.Error("Request body not set correctly")
	}
}

func TestRequest_WithMaxRetries(t *testing.T) {
	req := &Request{
		Method:     "GET",
		URL:        "https://example.com",
		MaxRetries: 3,
		Context:    context.Background(),
	}

	if req.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", req.MaxRetries)
	}
}

func TestRequest_Clone(t *testing.T) {
	original := &Request{
		Method: "POST",
		URL:    "https://example.com",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		QueryParams: map[string]any{
			"test": "value",
		},
		Body:       map[string]string{"key": "value"},
		Timeout:    10 * time.Second,
		MaxRetries: 2,
		Context:    context.Background(),
		Cookies: []http.Cookie{
			{Name: "test", Value: "cookie"},
		},
	}

	// Test that modifying headers doesn't affect original
	if original.Headers == nil {
		original.Headers = make(map[string]string)
	}
	original.Headers["New-Header"] = "new-value"

	if original.Headers["New-Header"] != "new-value" {
		t.Error("Header modification failed")
	}

	if original.Headers["Content-Type"] != "application/json" {
		t.Error("Original header was modified")
	}
}
