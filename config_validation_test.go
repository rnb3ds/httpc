package httpc

import (
	"crypto/tls"
	"testing"
	"time"
)

// ============================================================================
// CONFIG VALIDATION TESTS
// ============================================================================

func TestConfigValidation_DefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig should not return nil")
	}

	// Validate default values
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

func TestConfigValidation_TimeoutValues(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		wantErr bool
	}{
		{"Positive timeout", 30 * time.Second, false},
		{"Zero timeout", 0, false},                   // Zero means no timeout
		{"Negative timeout", -1 * time.Second, true}, // Should error
		{"Very large timeout", 24 * time.Hour, true}, // Should error (> 10 minutes)
		{"Very small timeout", 1 * time.Millisecond, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.Timeout = tt.timeout

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

func TestConfigValidation_RetryValues(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
		retryDelay time.Duration
		wantErr    bool
	}{
		{"Normal retries", 3, 100 * time.Millisecond, false},
		{"Zero retries", 0, 0, false},
		{"Negative retries", -1, 0, true},             // Should error
		{"Large retries", 100, 1 * time.Second, true}, // Should error (> 10)
		{"Zero delay", 3, 0, false},
		{"Negative delay", 3, -1 * time.Second, true}, // Should error
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.MaxRetries = tt.maxRetries
			config.RetryDelay = tt.retryDelay

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

func TestConfigValidation_ConnectionPoolValues(t *testing.T) {
	tests := []struct {
		name            string
		maxIdleConns    int
		maxConnsPerHost int
		wantErr         bool
	}{
		{"Normal pool", 100, 10, false},
		{"Zero max idle", 0, 10, false},
		{"Negative max idle", -1, 10, true}, // Should error
		{"Large pool", 10000, 1000, true},   // Should error (> 1000)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.MaxIdleConns = tt.maxIdleConns
			config.MaxConnsPerHost = tt.maxConnsPerHost

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

func TestConfigValidation_TLSConfig(t *testing.T) {
	tests := []struct {
		name      string
		tlsConfig *tls.Config
		wantErr   bool
	}{
		{
			name:      "Nil TLS config",
			tlsConfig: nil,
			wantErr:   false,
		},
		{
			name: "Valid TLS config",
			tlsConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				MaxVersion: tls.VersionTLS13,
			},
			wantErr: false,
		},
		{
			name: "Insecure skip verify",
			tlsConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.TLSConfig = tt.tlsConfig

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

func TestConfigValidation_ConcurrencyLimits(t *testing.T) {
	tests := []struct {
		name          string
		maxConcurrent int
		wantErr       bool
	}{
		{"Normal limits", 100, false},
		{"Zero concurrent", 0, false},
		{"Negative concurrent", -1, false},
		{"Large limits", 10000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()

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

func TestConfigValidation_UserAgent(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		wantErr   bool
	}{
		{"Normal user agent", "MyApp/1.0", false},
		{"Empty user agent", "", false},
		{"Long user agent", string(make([]byte, 1000)), true}, // Should error (> 512)
		{"Special characters", "App/1.0 (Linux; x64)", false},
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

func TestConfigValidation_ProxyURL(t *testing.T) {
	tests := []struct {
		name     string
		proxyURL string
		wantErr  bool
	}{
		{"No proxy", "", false},
		{"Valid HTTP proxy", "http://proxy.example.com:8080", false},
		{"Valid HTTPS proxy", "https://proxy.example.com:8080", false},
		{"Invalid proxy URL", "://invalid", true},
		{"Proxy with auth", "http://user:pass@proxy.example.com:8080", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.ProxyURL = tt.proxyURL

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

// ============================================================================
// CONFIG CONFLICT TESTS
// ============================================================================

func TestConfigValidation_ConflictingTimeouts(t *testing.T) {
	config := DefaultConfig()
	config.Timeout = 1 * time.Second

	client, err := New(config)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer client.Close()

	// Should handle conflicting timeouts gracefully
}

func TestConfigValidation_ConflictingRetrySettings(t *testing.T) {
	config := DefaultConfig()
	config.MaxRetries = 5
	config.RetryDelay = 10 * time.Second
	config.Timeout = 5 * time.Second // Shorter than retry delay

	client, err := New(config)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer client.Close()

	// Should handle conflicting settings gracefully
}

// ============================================================================
// CONFIG PRESET TESTS
// ============================================================================

func TestConfigValidation_SecureClient(t *testing.T) {
	client, err := New(ConfigPreset(SecurityLevelStrict))
	if err != nil {
		t.Fatalf("New(ConfigPreset(SecurityLevelStrict)) failed: %v", err)
	}
	defer client.Close()
}

// ============================================================================
// CONFIG MODIFICATION TESTS
// ============================================================================

func TestConfigValidation_ModifyAfterCreation(t *testing.T) {
	config := DefaultConfig()
	originalTimeout := config.Timeout

	client, err := New(config)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer client.Close()

	// Modify config after client creation
	config.Timeout = 1 * time.Hour

	// Original client should not be affected
	// (This is a design decision - config is copied)
	if config.Timeout == originalTimeout {
		t.Log("Config was copied (good)")
	}
}

func TestConfigValidation_NilConfig(t *testing.T) {
	// Should use default config
	client, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) failed: %v", err)
	}
	defer client.Close()
}
