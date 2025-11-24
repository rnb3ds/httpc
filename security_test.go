package httpc

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============================================================================
// SECURITY TESTS - SSRF, CRLF injection, TLS configuration, input validation
// ============================================================================

func TestSecurity_URLValidation(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"Empty URL", "", true},
		{"Invalid scheme", "ftp://example.com", true},
		{"CRLF injection", "http://example.com\r\nX-Injected: header", true},
		{"Localhost without flag", "http://localhost", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Get(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestSecurity_HeaderValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check dangerous headers are filtered
		if r.Header.Get("X-Injected") != "" {
			t.Error("CRLF injection succeeded")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer client.Close()

	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{"Valid header", "X-Custom", "value", false},
		{"CRLF in key", "X-Custom\r\n", "value", true},
		{"CRLF in value", "X-Custom", "value\r\nX-Injected: bad", true},
		{"Newline in value", "X-Custom", "value\nX-Injected: bad", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Get(server.URL, WithHeader(tt.key, tt.value))
			if (err != nil) != tt.wantErr {
				t.Errorf("WithHeader(%q, %q) error = %v, wantErr %v", tt.key, tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestSecurity_TLSConfiguration(t *testing.T) {
	t.Run("DefaultTLSVersion", func(t *testing.T) {
		config := DefaultConfig()

		if config.MinTLSVersion < tls.VersionTLS12 {
			t.Error("Default config should enforce TLS 1.2 minimum")
		}

		if config.InsecureSkipVerify {
			t.Error("Default config should not skip TLS verification")
		}
	})

	t.Run("SecureConfig", func(t *testing.T) {
		config := SecureConfig()

		if config.MinTLSVersion < tls.VersionTLS12 {
			t.Error("Secure config should enforce TLS 1.2+")
		}
		if config.InsecureSkipVerify {
			t.Error("Secure config should not skip TLS verification")
		}
		if config.AllowPrivateIPs {
			t.Error("Secure config should not allow private IPs")
		}
	})
}

func TestSecurity_PrivateIPBlocking(t *testing.T) {
	client, _ := newTestClient()
	defer client.Close()

	// Test only localhost which is fast to fail
	privateIPs := []string{
		"http://127.0.0.1",
		"http://localhost",
	}

	for _, url := range privateIPs {
		t.Run(url, func(t *testing.T) {
			_, err := client.Get(url)
			if err == nil {
				t.Errorf("Expected error for private IP: %s", url)
			}
		})
	}
}

func TestSecurity_AllowPrivateIPs(t *testing.T) {
	config := DefaultConfig()
	config.AllowPrivateIPs = true
	client, _ := New(config)
	defer client.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := client.Get(server.URL)
	if err != nil {
		t.Errorf("Request to localhost should succeed with AllowPrivateIPs: %v", err)
	}
}

func TestSecurity_SSRFProtection(t *testing.T) {
	config := DefaultConfig()
	config.AllowPrivateIPs = false
	client, _ := New(config)
	defer client.Close()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"Localhost", "http://localhost:8080", true},
		{"127.0.0.1", "http://127.0.0.1", true},
		{"Private 10.x", "http://10.0.0.1", true},
		{"Private 192.168.x", "http://192.168.1.1", true},
		{"Private 172.16.x", "http://172.16.0.1", true},
		{"Link-local", "http://169.254.169.254", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Get(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestSecurity_UserAgentValidation(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		wantErr   bool
	}{
		{"Valid", "MyApp/1.0", false},
		{"WithCRLF", "MyApp\r\n/1.0", true},
		{"WithNewline", "MyApp\n/1.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.UserAgent = tt.userAgent
			client, err := New(config)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
			if client != nil {
				client.Close()
			}
		})
	}
}
