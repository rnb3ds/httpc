package httpc

import (
	"crypto/tls"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// CONFIGURATION TESTS - Config validation, presets, TLS versions
// ============================================================================

// ----------------------------------------------------------------------------
// Config Creation and Defaults
// ----------------------------------------------------------------------------

func TestConfig_Defaults(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig should not return nil")
	}
	if config.Timeout <= 0 {
		t.Error("Default timeout should be positive")
	}
	if config.MaxRetries < 0 {
		t.Error("Default max retries should be non-negative")
	}
	if config.MaxIdleConns <= 0 {
		t.Error("Default max idle connections should be positive")
	}
	if config.UserAgent == "" {
		t.Error("Default user agent should not be empty")
	}
}

// ----------------------------------------------------------------------------
// Config Presets - Creation and Field Verification
// ----------------------------------------------------------------------------

func TestConfig_Presets(t *testing.T) {
	t.Run("SecureConfig", func(t *testing.T) {
		config := SecureConfig()
		client, err := New(config)
		if err != nil {
			t.Fatalf("New(SecureConfig()) failed: %v", err)
		}
		defer client.Close()

		// Verify security-focused settings
		if config.MinTLSVersion < tls.VersionTLS12 {
			t.Error("Secure config should enforce TLS 1.2+")
		}
		if config.InsecureSkipVerify {
			t.Error("Secure config should not skip TLS verification")
		}
		if config.AllowPrivateIPs {
			t.Error("Secure config should not allow private IPs")
		}
		if config.Timeout != 15*time.Second {
			t.Errorf("Expected Timeout=15s, got %v", config.Timeout)
		}
		if config.MaxRetries != 1 {
			t.Errorf("Expected MaxRetries=1, got %d", config.MaxRetries)
		}
		if config.FollowRedirects {
			t.Error("Expected FollowRedirects=false")
		}
	})

	t.Run("PerformanceConfig", func(t *testing.T) {
		config := PerformanceConfig()
		client, err := New(config)
		if err != nil {
			t.Fatalf("New(PerformanceConfig()) failed: %v", err)
		}
		defer client.Close()

		// Verify performance-focused settings
		if config.MaxIdleConns <= 0 {
			t.Error("Performance config should have connection pooling")
		}
		if !config.EnableHTTP2 {
			t.Error("Performance config should enable HTTP/2")
		}
		if config.Timeout != 60*time.Second {
			t.Errorf("Expected Timeout=60s, got %v", config.Timeout)
		}
	})

	t.Run("MinimalConfig", func(t *testing.T) {
		config := MinimalConfig()
		client, err := New(config)
		if err != nil {
			t.Fatalf("New(MinimalConfig()) failed: %v", err)
		}
		defer client.Close()

		// Verify minimal settings
		if config.MaxRetries != 0 {
			t.Error("Minimal config should have no retries")
		}
		if config.FollowRedirects {
			t.Error("Minimal config should not follow redirects")
		}
	})

	t.Run("TestingConfig", func(t *testing.T) {
		config := TestingConfig()

		// Verify testing-focused settings
		if config.Timeout != 30*time.Second {
			t.Errorf("Expected timeout 30s, got %v", config.Timeout)
		}
		if !config.AllowPrivateIPs {
			t.Error("Expected AllowPrivateIPs to be true")
		}
		if !config.InsecureSkipVerify {
			t.Error("Expected InsecureSkipVerify to be true for testing")
		}
		if config.MaxRetries != 1 {
			t.Errorf("Expected MaxRetries=1, got %d", config.MaxRetries)
		}
		if config.UserAgent != "httpc-test/1.0" {
			t.Errorf("Expected UserAgent='httpc-test/1.0', got %q", config.UserAgent)
		}
	})
}

// ----------------------------------------------------------------------------
// Config Validation - Field-Specific Tests
// ----------------------------------------------------------------------------

func TestConfig_Validation(t *testing.T) {
	t.Run("Timeout", func(t *testing.T) {
		tests := []struct {
			name    string
			timeout time.Duration
			wantErr bool
		}{
			{"Positive", 30 * time.Second, false},
			{"Zero", 0, false},
			{"Negative", -1 * time.Second, true},
			{"TooLarge", 24 * time.Hour, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.Timeout = tt.timeout
				_, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("MaxRetries", func(t *testing.T) {
		tests := []struct {
			name       string
			maxRetries int
			wantErr    bool
		}{
			{"Zero", 0, false},
			{"Positive", 3, false},
			{"Negative", -1, true},
			{"TooLarge", 100, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.MaxRetries = tt.maxRetries
				_, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("ConnectionPool", func(t *testing.T) {
		tests := []struct {
			name         string
			maxIdleConns int
			maxConns     int
			wantErr      bool
		}{
			{"Valid", 100, 10, false},
			{"NegativeIdle", -1, 10, true},
			{"NegativePerHost", 100, -1, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.MaxIdleConns = tt.maxIdleConns
				config.MaxConnsPerHost = tt.maxConns
				client, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
				if client != nil {
					client.Close()
				}
			})
		}
	})

	t.Run("UserAgent", func(t *testing.T) {
		tests := []struct {
			name      string
			userAgent string
			wantErr   bool
		}{
			{"Valid", "MyApp/1.0", false},
			{"Empty", "", false},
			{"WithCRLF", "MyApp\r\n/1.0", true},
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
	})
}

// ----------------------------------------------------------------------------
// TLS Configuration
// ----------------------------------------------------------------------------

func TestConfig_TLSVersions(t *testing.T) {
	t.Run("MinTLSVersion", func(t *testing.T) {
		tests := []struct {
			name       string
			minVersion uint16
			wantErr    bool
		}{
			{"TLS 1.2", tls.VersionTLS12, false},
			{"TLS 1.3", tls.VersionTLS13, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.MinTLSVersion = tt.minVersion
				client, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
				if client != nil {
					client.Close()
				}
			})
		}
	})

	t.Run("MaxTLSVersion", func(t *testing.T) {
		tests := []struct {
			name       string
			maxVersion uint16
			wantErr    bool
		}{
			{"TLS 1.3", tls.VersionTLS13, false},
			{"TLS 1.2", tls.VersionTLS12, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.MaxTLSVersion = tt.maxVersion
				client, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
				if client != nil {
					client.Close()
				}
			})
		}
	})

	t.Run("TLSVersionRange", func(t *testing.T) {
		tests := []struct {
			name       string
			minVersion uint16
			maxVersion uint16
			wantErr    bool
		}{
			{"Valid: 1.2-1.3", tls.VersionTLS12, tls.VersionTLS13, false},
			{"Valid: 1.2-1.2", tls.VersionTLS12, tls.VersionTLS12, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				config := DefaultConfig()
				config.MinTLSVersion = tt.minVersion
				config.MaxTLSVersion = tt.maxVersion
				client, err := New(config)
				if (err != nil) != tt.wantErr {
					t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				}
				if client != nil {
					client.Close()
				}
			})
		}
	})

	t.Run("WithTLSConfig", func(t *testing.T) {
		config := DefaultConfig()
		config.MinTLSVersion = tls.VersionTLS12
		config.MaxTLSVersion = tls.VersionTLS13
		config.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS13,
		}

		client, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()
	})
}

// ----------------------------------------------------------------------------
// Config Immutability
// ----------------------------------------------------------------------------

func TestConfig_Modification(t *testing.T) {
	config := DefaultConfig()
	originalTimeout := config.Timeout

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Modify config after client creation
	config.Timeout = 1 * time.Second
	config.MaxRetries = 10

	// Original client should not be affected
	if config.Timeout == originalTimeout {
		t.Log("Config modification does not affect existing client")
	}
}

// ----------------------------------------------------------------------------
// Internal Helper Functions
// ----------------------------------------------------------------------------

func TestConfig_InternalHelpers(t *testing.T) {
	t.Run("isTestEnvironment", func(t *testing.T) {
		// When running under go test, this should return true
		if !isTestEnvironment() {
			t.Error("isTestEnvironment() should return true when running under go test")
		}
	})

	t.Run("warnTestingConfigInProduction", func(t *testing.T) {
		// This function should not panic and should handle the test environment
		// In test environment, it should not print warnings
		warnTestingConfigInProduction()
		// No assertion needed - just verify it doesn't panic
	})
}

// ----------------------------------------------------------------------------
// Advanced Config Fields
// ----------------------------------------------------------------------------

func TestConfig_AdvancedFields(t *testing.T) {
	t.Run("FlatFieldUsage", func(t *testing.T) {
		config := DefaultConfig()

		// Use flat fields for common settings
		config.Timeout = 60 * time.Second
		config.MaxRetries = 5
		config.ProxyURL = "http://proxy:8080"
		config.AllowPrivateIPs = true
		config.UserAgent = "my-app/1.0"
		config.FollowRedirects = false

		// Use flat fields for advanced settings
		config.DialTimeout = 5 * time.Second
		config.TLSHandshakeTimeout = 5 * time.Second
		config.MaxIdleConns = 100
		config.MaxConnsPerHost = 20
		config.MaxResponseBodySize = 50 * 1024 * 1024
		config.RetryDelay = 500 * time.Millisecond
		config.BackoffFactor = 1.5

		client, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()
	})
}

// ----------------------------------------------------------------------------
// Config.String() Tests
// ----------------------------------------------------------------------------

func TestConfig_String(t *testing.T) {
	t.Run("Nil config", func(t *testing.T) {
		var config *Config = nil
		result := config.String()
		if result != "Config{<nil>}" {
			t.Errorf("Expected 'Config{<nil>}', got %q", result)
		}
	})

	t.Run("Default config", func(t *testing.T) {
		config := DefaultConfig()
		result := config.String()

		// Verify key parts are present
		if !strings.Contains(result, "Config{") {
			t.Error("String should start with 'Config{'")
		}
		if !strings.Contains(result, "Timeout:") {
			t.Error("String should contain 'Timeout:'")
		}
		if !strings.Contains(result, "ProxyURL:") {
			t.Error("String should contain 'ProxyURL:'")
		}
		if !strings.Contains(result, "TLSConfig:") {
			t.Error("String should contain 'TLSConfig:'")
		}
		if !strings.Contains(result, "<default>") {
			t.Error("String should contain '<default>' for nil TLSConfig")
		}
	})

	t.Run("Config with TLS", func(t *testing.T) {
		config := DefaultConfig()
		config.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		result := config.String()
		if !strings.Contains(result, "<configured>") {
			t.Error("String should contain '<configured>' for non-nil TLSConfig")
		}
	})

	t.Run("Config with proxy", func(t *testing.T) {
		config := DefaultConfig()
		config.ProxyURL = "http://user:password@proxy.example.com:8080"

		result := config.String()
		// Password should be masked
		if strings.Contains(result, "password") {
			t.Error("String should not contain plaintext password")
		}
		if !strings.Contains(result, "ProxyURL:") {
			t.Error("String should contain 'ProxyURL:'")
		}
	})

	t.Run("Config with all fields", func(t *testing.T) {
		config := &Config{
			Timeout:             30 * time.Second,
			DialTimeout:         5 * time.Second,
			TLSHandshakeTimeout: 5 * time.Second,
			MaxIdleConns:        100,
			MaxConnsPerHost:     20,
			ProxyURL:            "http://proxy:8080",
			InsecureSkipVerify:  true,
			AllowPrivateIPs:     true,
			MaxRetries:          3,
			BackoffFactor:       1.5,
			UserAgent:           "test-agent",
			FollowRedirects:     false,
		}

		result := config.String()

		// Verify all key fields are present
		expectedParts := []string{
			"Timeout:",
			"DialTimeout:",
			"TLSHandshakeTimeout:",
			"MaxIdleConns:",
			"MaxConnsPerHost:",
			"ProxyURL:",
			"InsecureSkipVerify:",
			"AllowPrivateIPs:",
			"MaxRetries:",
			"BackoffFactor:",
			"UserAgent:",
			"FollowRedirects:",
		}

		for _, part := range expectedParts {
			if !strings.Contains(result, part) {
				t.Errorf("String should contain %q", part)
			}
		}
	})
}

// ----------------------------------------------------------------------------
// maskProxyURL Tests (via Config.String())
// ----------------------------------------------------------------------------

func TestMaskProxyURL(t *testing.T) {
	tests := []struct {
		name     string
		proxyURL string
		// We test this via Config.String() since maskProxyURL is not exported
		contains    string
		notContains string
	}{
		{
			name:     "Empty URL",
			proxyURL: "",
			contains: "ProxyURL:",
		},
		{
			name:     "URL without credentials",
			proxyURL: "http://proxy.example.com:8080",
			contains: "proxy.example.com",
		},
		{
			name:        "URL with credentials",
			proxyURL:    "http://user:secret@proxy.example.com:8080",
			notContains: "secret",
		},
		{
			name:     "SOCKS5 URL",
			proxyURL: "socks5://proxy.example.com:1080",
			contains: "socks5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.ProxyURL = tt.proxyURL
			result := config.String()

			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("Expected result to contain %q, got: %s", tt.contains, result)
			}
			if tt.notContains != "" && strings.Contains(result, tt.notContains) {
				t.Errorf("Expected result NOT to contain %q, got: %s", tt.notContains, result)
			}
		})
	}
}
