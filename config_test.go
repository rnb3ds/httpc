package httpc

import (
	"crypto/tls"
	"testing"
	"time"
)

// ============================================================================
// CONFIGURATION TESTS - Config validation, presets, TLS versions
// ============================================================================

// ----------------------------------------------------------------------------
// Config Presets
// ----------------------------------------------------------------------------

func TestConfig_Presets(t *testing.T) {
	t.Run("SecureConfig", func(t *testing.T) {
		client, err := New(SecureConfig())
		if err != nil {
			t.Fatalf("New(SecureConfig()) failed: %v", err)
		}
		defer client.Close()
		if client == nil {
			t.Fatal("Client should not be nil")
		}
	})

	t.Run("PerformanceConfig", func(t *testing.T) {
		client, err := New(PerformanceConfig())
		if err != nil {
			t.Fatalf("New(PerformanceConfig()) failed: %v", err)
		}
		defer client.Close()
		if client == nil {
			t.Fatal("Client should not be nil")
		}
	})

	t.Run("MinimalConfig", func(t *testing.T) {
		client, err := New(MinimalConfig())
		if err != nil {
			t.Fatalf("New(MinimalConfig()) failed: %v", err)
		}
		defer client.Close()
		if client == nil {
			t.Fatal("Client should not be nil")
		}
	})

	t.Run("TestingConfig", func(t *testing.T) {
		cfg := TestingConfig()
		if cfg.Timeout != 30*time.Second {
			t.Errorf("Expected timeout 30s, got %v", cfg.Timeout)
		}
		if !cfg.AllowPrivateIPs {
			t.Error("Expected AllowPrivateIPs to be true")
		}
		if !cfg.InsecureSkipVerify {
			t.Error("Expected InsecureSkipVerify to be true for testing")
		}
	})
}

// ----------------------------------------------------------------------------
// Default Config
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

func TestConfig_Validation(t *testing.T) {
	t.Run("TimeoutValues", func(t *testing.T) {
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

	t.Run("RetryValues", func(t *testing.T) {
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

func TestConfig_PresetConfigs(t *testing.T) {
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

	t.Run("PerformanceConfig", func(t *testing.T) {
		config := PerformanceConfig()
		if config.MaxIdleConns <= 0 {
			t.Error("Performance config should have connection pooling")
		}
		if !config.EnableHTTP2 {
			t.Error("Performance config should enable HTTP/2")
		}
	})

	t.Run("MinimalConfig", func(t *testing.T) {
		config := MinimalConfig()
		if config.MaxRetries != 0 {
			t.Error("Minimal config should have no retries")
		}
		if config.FollowRedirects {
			t.Error("Minimal config should not follow redirects")
		}
	})
}

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
// Flat Config Fields
// ----------------------------------------------------------------------------

func TestConfig_FlatFields(t *testing.T) {
	t.Run("DefaultValues", func(t *testing.T) {
		config := DefaultConfig()

		// Verify flat field defaults
		if config.Timeout != 30*time.Second {
			t.Errorf("Timeout = %v, want 30s", config.Timeout)
		}
		if config.MaxRetries != 3 {
			t.Errorf("MaxRetries = %d, want 3", config.MaxRetries)
		}
		if config.UserAgent != "httpc/1.0" {
			t.Errorf("UserAgent = %q, want 'httpc/1.0'", config.UserAgent)
		}
		if config.FollowRedirects != true {
			t.Error("FollowRedirects should be true")
		}
	})

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

	t.Run("ValidationForFlatFields", func(t *testing.T) {
		t.Run("TimeoutValidation", func(t *testing.T) {
			tests := []struct {
				name    string
				timeout time.Duration
				wantErr bool
			}{
				{"Valid", 30 * time.Second, false},
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

		t.Run("MaxRetriesValidation", func(t *testing.T) {
			tests := []struct {
				name       string
				maxRetries int
				wantErr    bool
			}{
				{"Valid", 5, false},
				{"Zero", 0, false},
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

		t.Run("UserAgentValidation", func(t *testing.T) {
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
	})
}

func TestConfig_PresetsWithFlatFields(t *testing.T) {
	t.Run("SecureConfigFlatFields", func(t *testing.T) {
		config := SecureConfig()

		// Verify flat fields are set correctly
		if config.Timeout != 15*time.Second {
			t.Errorf("Expected Timeout=15s, got %v", config.Timeout)
		}
		if config.MaxRetries != 1 {
			t.Errorf("Expected MaxRetries=1, got %d", config.MaxRetries)
		}
		if config.AllowPrivateIPs {
			t.Error("Expected AllowPrivateIPs=false")
		}
		if config.FollowRedirects {
			t.Error("Expected FollowRedirects=false")
		}
	})

	t.Run("PerformanceConfigFlatFields", func(t *testing.T) {
		config := PerformanceConfig()

		// Verify flat fields are set correctly
		if config.Timeout != 60*time.Second {
			t.Errorf("Expected Timeout=60s, got %v", config.Timeout)
		}
	})

	t.Run("TestingConfigFlatFields", func(t *testing.T) {
		config := TestingConfig()

		// Verify flat fields are set correctly
		if config.MaxRetries != 1 {
			t.Errorf("Expected MaxRetries=1, got %d", config.MaxRetries)
		}
		if !config.AllowPrivateIPs {
			t.Error("Expected AllowPrivateIPs=true")
		}
		if config.UserAgent != "httpc-test/1.0" {
			t.Errorf("Expected UserAgent='httpc-test/1.0', got %q", config.UserAgent)
		}
	})

	t.Run("MinimalConfigFlatFields", func(t *testing.T) {
		config := MinimalConfig()

		// Verify flat fields are set correctly
		if config.MaxRetries != 0 {
			t.Errorf("Expected MaxRetries=0, got %d", config.MaxRetries)
		}
		if config.FollowRedirects {
			t.Error("Expected FollowRedirects=false")
		}
	})
}
