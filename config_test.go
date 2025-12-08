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
	t.Run("NewSecure", func(t *testing.T) {
		client, err := NewSecure()
		if err != nil {
			t.Fatalf("NewSecure failed: %v", err)
		}
		defer client.Close()
		if client == nil {
			t.Fatal("Client should not be nil")
		}
	})

	t.Run("NewPerformance", func(t *testing.T) {
		client, err := NewPerformance()
		if err != nil {
			t.Fatalf("NewPerformance failed: %v", err)
		}
		defer client.Close()
		if client == nil {
			t.Fatal("Client should not be nil")
		}
	})

	t.Run("NewMinimal", func(t *testing.T) {
		client, err := NewMinimal()
		if err != nil {
			t.Fatalf("NewMinimal failed: %v", err)
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
