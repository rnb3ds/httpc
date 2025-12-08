package httpc

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// testConfig returns a config suitable for testing with localhost
func testRedirectConfig() *Config {
	config := DefaultConfig()
	config.AllowPrivateIPs = true
	return config
}

func TestRedirect_AutoFollow(t *testing.T) {
	t.Parallel()

	redirectCount := 0
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Final destination"))
	}))
	defer finalServer.Close()

	var redirectServer *httptest.Server
	redirectServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount < 3 {
			http.Redirect(w, r, redirectServer.URL, http.StatusFound)
		} else {
			http.Redirect(w, r, finalServer.URL, http.StatusFound)
		}
	}))
	defer redirectServer.Close()

	config := testRedirectConfig()
	config.FollowRedirects = true
	config.MaxRedirects = 10
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(redirectServer.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode())
	}

	if resp.Body() != "Final destination" {
		t.Errorf("Expected 'Final destination', got '%s'", resp.Body())
	}

	if resp.Meta.RedirectCount != 3 {
		t.Errorf("Expected 3 redirects, got %d", resp.Meta.RedirectCount)
	}

	if len(resp.Meta.RedirectChain) != 3 {
		t.Errorf("Expected redirect chain length 3, got %d", len(resp.Meta.RedirectChain))
	}
}

func TestRedirect_NoFollow(t *testing.T) {
	t.Parallel()

	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Final destination"))
	}))
	defer finalServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	config := testRedirectConfig()
	config.FollowRedirects = false
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(redirectServer.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusFound {
		t.Errorf("Expected status 302, got %d", resp.StatusCode())
	}

	location := resp.Response.Headers.Get("Location")
	if location != finalServer.URL {
		t.Errorf("Expected Location header '%s', got '%s'", finalServer.URL, location)
	}

	if resp.Meta.RedirectCount != 0 {
		t.Errorf("Expected 0 redirects, got %d", resp.Meta.RedirectCount)
	}

	if len(resp.Meta.RedirectChain) != 0 {
		t.Errorf("Expected empty redirect chain, got %d entries", len(resp.Meta.RedirectChain))
	}
}

func TestRedirect_MaxRedirectsLimit(t *testing.T) {
	t.Parallel()

	var redirectServer *httptest.Server
	redirectServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	config := testRedirectConfig()
	config.FollowRedirects = true
	config.MaxRedirects = 3
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	_, err = client.Get(redirectServer.URL)
	if err == nil {
		t.Error("Expected error for too many redirects, got nil")
	}
}

func TestRedirect_PerRequestOverride(t *testing.T) {
	t.Parallel()

	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Final destination"))
	}))
	defer finalServer.Close()

	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	// Client configured to follow redirects
	config := testRedirectConfig()
	config.FollowRedirects = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Override to not follow redirects for this request
	resp, err := client.Get(redirectServer.URL, WithFollowRedirects(false))
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.StatusCode() != http.StatusFound {
		t.Errorf("Expected status 302, got %d", resp.StatusCode())
	}

	if resp.Meta.RedirectCount != 0 {
		t.Errorf("Expected 0 redirects, got %d", resp.Meta.RedirectCount)
	}
}

func TestRedirect_MaxRedirectsPerRequest(t *testing.T) {
	t.Parallel()

	redirectCount := 0
	var redirectServer *httptest.Server
	redirectServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		http.Redirect(w, r, redirectServer.URL, http.StatusFound)
	}))
	defer redirectServer.Close()

	config := testRedirectConfig()
	config.FollowRedirects = true
	config.MaxRedirects = 10
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Override max redirects to 2 for this request
	_, err = client.Get(redirectServer.URL, WithMaxRedirects(2))
	if err == nil {
		t.Error("Expected error for too many redirects, got nil")
	}

	if redirectCount > 3 {
		t.Errorf("Expected at most 3 redirect attempts, got %d", redirectCount)
	}
}

func TestRedirect_DifferentStatusCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		shouldWork bool
	}{
		{"301 Moved Permanently", http.StatusMovedPermanently, true},
		{"302 Found", http.StatusFound, true},
		{"303 See Other", http.StatusSeeOther, true},
		{"307 Temporary Redirect", http.StatusTemporaryRedirect, true},
		{"308 Permanent Redirect", http.StatusPermanentRedirect, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("Success"))
			}))
			defer finalServer.Close()

			redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", finalServer.URL)
				w.WriteHeader(tt.statusCode)
			}))
			defer redirectServer.Close()

			config := testRedirectConfig()
			config.FollowRedirects = true
			client, err := New(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			resp, err := client.Get(redirectServer.URL)
			if err != nil {
				t.Fatalf("Request failed: %v", err)
			}

			if tt.shouldWork && resp.StatusCode() != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode())
			}

			if tt.shouldWork && resp.Meta.RedirectCount != 1 {
				t.Errorf("Expected 1 redirect, got %d", resp.Meta.RedirectCount)
			}
		})
	}
}

func TestRedirect_ChainTracking(t *testing.T) {
	t.Parallel()

	server3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Final"))
	}))
	defer server3.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server3.URL, http.StatusFound)
	}))
	defer server2.Close()

	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server2.URL, http.StatusFound)
	}))
	defer server1.Close()

	config := testRedirectConfig()
	config.FollowRedirects = true
	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	resp, err := client.Get(server1.URL)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	if resp.Meta.RedirectCount != 2 {
		t.Errorf("Expected 2 redirects, got %d", resp.Meta.RedirectCount)
	}

	if len(resp.Meta.RedirectChain) != 2 {
		t.Fatalf("Expected redirect chain length 2, got %d", len(resp.Meta.RedirectChain))
	}

	// Verify the chain contains the intermediate URLs
	if resp.Meta.RedirectChain[0] != server1.URL {
		t.Errorf("Expected first redirect to be %s, got %s", server1.URL, resp.Meta.RedirectChain[0])
	}

	if resp.Meta.RedirectChain[1] != server2.URL {
		t.Errorf("Expected second redirect to be %s, got %s", server2.URL, resp.Meta.RedirectChain[1])
	}
}

func TestRedirect_IsRedirectMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		statusCode int
		expected   bool
	}{
		{200, false},
		{299, false},
		{300, true},
		{301, true},
		{302, true},
		{303, true},
		{304, true},
		{307, true},
		{308, true},
		{399, true},
		{400, false},
		{500, false},
	}

	for _, tt := range tests {
		resp := &Response{StatusCode: tt.statusCode}
		if got := resp.IsRedirect(); got != tt.expected {
			t.Errorf("StatusCode %d: IsRedirect() = %v, want %v", tt.statusCode, got, tt.expected)
		}
	}
}

func TestRedirect_ConfigValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		maxRedirect int
		wantErr     bool
	}{
		{"Valid: 0", 0, false},
		{"Valid: 10", 10, false},
		{"Valid: 50", 50, false},
		{"Invalid: negative", -1, true},
		{"Invalid: too large", 51, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.MaxRedirects = tt.maxRedirect
			err := ValidateConfig(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRedirect_OptionValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		maxRedirect int
		wantErr     bool
	}{
		{"Valid: 0", 0, false},
		{"Valid: 10", 10, false},
		{"Valid: 50", 50, false},
		{"Invalid: negative", -1, true},
		{"Invalid: too large", 51, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{}
			opt := WithMaxRedirects(tt.maxRedirect)
			err := opt(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("WithMaxRedirects() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
