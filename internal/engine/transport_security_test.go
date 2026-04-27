package engine

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
)

func TestCheckRedirect_CrossOriginHeaderStripping(t *testing.T) {
	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
	}

	connConfig := testConnectionConfig()
	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer func() { _ = poolManager.Close() }()

	trans, err := newTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = trans.Close() }()

	tests := []struct {
		name                 string
		originalHost         string
		redirectHost         string
		expectAuthStripped   bool
		expectCookieStripped bool
	}{
		{
			name:                 "different hosts strips sensitive headers",
			originalHost:         "api.example.com",
			redirectHost:         "evil.com",
			expectAuthStripped:   true,
			expectCookieStripped: true,
		},
		{
			name:                 "same host preserves headers",
			originalHost:         "example.com",
			redirectHost:         "example.com",
			expectAuthStripped:   false,
			expectCookieStripped: false,
		},
		{
			name:                 "different subdomain strips headers",
			originalHost:         "api.example.com",
			redirectHost:         "www.example.com",
			expectAuthStripped:   true,
			expectCookieStripped: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redirectURL, _ := url.Parse("https://" + tt.redirectHost + "/redirected")
			redirectReq, _ := http.NewRequest("GET", redirectURL.String(), nil)
			redirectReq.Header.Set("Authorization", "Bearer secret-token")
			redirectReq.Header.Set("Proxy-Authorization", "Basic creds")
			redirectReq.Header.Set("Cookie", "session=abc")
			redirectReq.Header.Set("X-Custom", "visible")

			originalURL, _ := url.Parse("https://" + tt.originalHost + "/original")
			originalReq, _ := http.NewRequest("GET", originalURL.String(), nil)

			settings := getRedirectSettings()
			settings.followRedirects = true
			settings.maxRedirects = 5
			ctx := context.WithValue(redirectReq.Context(), redirectContextKey{}, settings)
			redirectReq = redirectReq.WithContext(ctx)

			via := []*http.Request{originalReq}
			err := trans.checkRedirect(redirectReq, via)
			if err != nil {
				t.Fatalf("checkRedirect returned error: %v", err)
			}

			authStripped := redirectReq.Header.Get("Authorization") == ""
			cookieStripped := redirectReq.Header.Get("Cookie") == ""

			if authStripped != tt.expectAuthStripped {
				t.Errorf("Authorization stripped=%v, want=%v", authStripped, tt.expectAuthStripped)
			}
			if cookieStripped != tt.expectCookieStripped {
				t.Errorf("Cookie stripped=%v, want=%v", cookieStripped, tt.expectCookieStripped)
			}

			if redirectReq.Header.Get("X-Custom") != "visible" {
				t.Error("X-Custom header should never be stripped")
			}

			putRedirectSettings(settings)
		})
	}
}

func TestCheckRedirect_SameOriginHeadersPreserved(t *testing.T) {
	config := &Config{
		Timeout:         30 * time.Second,
		AllowPrivateIPs: true,
	}

	connConfig := testConnectionConfig()
	poolManager, err := connection.NewPoolManager(connConfig)
	if err != nil {
		t.Fatalf("Failed to create pool manager: %v", err)
	}
	defer func() { _ = poolManager.Close() }()

	trans, err := newTransport(config, poolManager)
	if err != nil {
		t.Fatalf("Failed to create transport: %v", err)
	}
	defer func() { _ = trans.Close() }()

	redirectURL, _ := url.Parse("https://example.com/new-path")
	redirectReq, _ := http.NewRequest("GET", redirectURL.String(), nil)
	redirectReq.Header.Set("Authorization", "Bearer secret-token")

	originalURL, _ := url.Parse("https://example.com/old-path")
	originalReq, _ := http.NewRequest("GET", originalURL.String(), nil)

	settings := getRedirectSettings()
	settings.followRedirects = true
	settings.maxRedirects = 5
	ctx := context.WithValue(redirectReq.Context(), redirectContextKey{}, settings)
	redirectReq = redirectReq.WithContext(ctx)

	err = trans.checkRedirect(redirectReq, []*http.Request{originalReq})
	if err != nil {
		t.Fatalf("checkRedirect returned error: %v", err)
	}

	if redirectReq.Header.Get("Authorization") != "Bearer secret-token" {
		t.Error("Authorization should be preserved on same-origin redirect")
	}
	putRedirectSettings(settings)
}

func TestClearPools(t *testing.T) {
	clearPools()

	settings := getRedirectSettings()
	if settings == nil {
		t.Fatal("getRedirectSettings returned nil after clearPools")
	}
	settings.followRedirects = true
	settings.maxRedirects = 5
	settings.addRedirect("https://example.com")
	putRedirectSettings(settings)

	clearPools()
}

func TestCrossOriginRedirectHostComparison(t *testing.T) {
	tests := []struct {
		name         string
		originalHost string
		redirectHost string
		shouldStrip  bool
	}{
		{"same host", "example.com", "example.com", false},
		{"different host", "example.com", "evil.com", true},
		{"same host different port", "example.com:8080", "example.com:9090", false},
		{"different subdomain", "api.example.com", "www.example.com", true},
		{"ip vs hostname", "example.com", "127.0.0.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, _ := url.Parse("http://" + tt.originalHost + "/path")
			redirect, _ := url.Parse("http://" + tt.redirectHost + "/path")

			stripNeeded := original.Hostname() != redirect.Hostname()
			if stripNeeded != tt.shouldStrip {
				t.Errorf("hostname comparison: %q vs %q, stripNeeded=%v, want=%v",
					original.Hostname(), redirect.Hostname(), stripNeeded, tt.shouldStrip)
			}
		})
	}
}
