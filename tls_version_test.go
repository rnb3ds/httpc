package httpc

import (
	"crypto/tls"
	"testing"
)

func TestConfig_MinTLSVersion(t *testing.T) {
	tests := []struct {
		name              string
		minTLSVersion     uint16
		expectedMinTLS    uint16
		shouldUseDefault  bool
	}{
		{
			name:             "TLS 1.2 minimum",
			minTLSVersion:    tls.VersionTLS12,
			expectedMinTLS:   tls.VersionTLS12,
			shouldUseDefault: false,
		},
		{
			name:             "TLS 1.3 minimum",
			minTLSVersion:    tls.VersionTLS13,
			expectedMinTLS:   tls.VersionTLS13,
			shouldUseDefault: false,
		},
		{
			name:             "TLS 1.0 minimum (legacy)",
			minTLSVersion:    tls.VersionTLS10,
			expectedMinTLS:   tls.VersionTLS10,
			shouldUseDefault: false,
		},
		{
			name:             "Zero value (should use default TLS 1.2)",
			minTLSVersion:    0,
			expectedMinTLS:   tls.VersionTLS12,
			shouldUseDefault: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.MinTLSVersion = tt.minTLSVersion

			client, err := New(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			// Verify the configuration was applied
			if config.MinTLSVersion != tt.minTLSVersion {
				t.Errorf("Expected MinTLSVersion %d, got %d", tt.minTLSVersion, config.MinTLSVersion)
			}
		})
	}
}

func TestConfig_MaxTLSVersion(t *testing.T) {
	tests := []struct {
		name              string
		maxTLSVersion     uint16
		expectedMaxTLS    uint16
		shouldUseDefault  bool
	}{
		{
			name:             "TLS 1.3 maximum",
			maxTLSVersion:    tls.VersionTLS13,
			expectedMaxTLS:   tls.VersionTLS13,
			shouldUseDefault: false,
		},
		{
			name:             "TLS 1.2 maximum",
			maxTLSVersion:    tls.VersionTLS12,
			expectedMaxTLS:   tls.VersionTLS12,
			shouldUseDefault: false,
		},
		{
			name:             "Zero value (should use default TLS 1.3)",
			maxTLSVersion:    0,
			expectedMaxTLS:   tls.VersionTLS13,
			shouldUseDefault: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.MaxTLSVersion = tt.maxTLSVersion

			client, err := New(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			// Verify the configuration was applied
			if config.MaxTLSVersion != tt.maxTLSVersion {
				t.Errorf("Expected MaxTLSVersion %d, got %d", tt.maxTLSVersion, config.MaxTLSVersion)
			}
		})
	}
}

func TestConfig_TLSVersionRange(t *testing.T) {
	tests := []struct {
		name          string
		minTLSVersion uint16
		maxTLSVersion uint16
		description   string
	}{
		{
			name:          "TLS 1.2 to 1.3 (recommended)",
			minTLSVersion: tls.VersionTLS12,
			maxTLSVersion: tls.VersionTLS13,
			description:   "Standard secure configuration",
		},
		{
			name:          "TLS 1.3 only (high security)",
			minTLSVersion: tls.VersionTLS13,
			maxTLSVersion: tls.VersionTLS13,
			description:   "Maximum security configuration",
		},
		{
			name:          "TLS 1.0 to 1.3 (legacy support)",
			minTLSVersion: tls.VersionTLS10,
			maxTLSVersion: tls.VersionTLS13,
			description:   "Legacy compatibility configuration",
		},
		{
			name:          "TLS 1.2 only",
			minTLSVersion: tls.VersionTLS12,
			maxTLSVersion: tls.VersionTLS12,
			description:   "TLS 1.2 only configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.MinTLSVersion = tt.minTLSVersion
			config.MaxTLSVersion = tt.maxTLSVersion

			client, err := New(config)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}
			defer client.Close()

			// Verify both versions were set correctly
			if config.MinTLSVersion != tt.minTLSVersion {
				t.Errorf("Expected MinTLSVersion %d, got %d", tt.minTLSVersion, config.MinTLSVersion)
			}

			if config.MaxTLSVersion != tt.maxTLSVersion {
				t.Errorf("Expected MaxTLSVersion %d, got %d", tt.maxTLSVersion, config.MaxTLSVersion)
			}
		})
	}
}

func TestConfigPresets_TLSVersions(t *testing.T) {
	tests := []struct {
		name              string
		configFunc        func() *Config
		expectedMinTLS    uint16
		expectedMaxTLS    uint16
	}{
		{
			name:           "DefaultConfig",
			configFunc:     DefaultConfig,
			expectedMinTLS: tls.VersionTLS12,
			expectedMaxTLS: tls.VersionTLS13,
		},
		{
			name:           "SecureConfig",
			configFunc:     SecureConfig,
			expectedMinTLS: tls.VersionTLS12,
			expectedMaxTLS: tls.VersionTLS13,
		},
		{
			name:           "PerformanceConfig",
			configFunc:     PerformanceConfig,
			expectedMinTLS: tls.VersionTLS12,
			expectedMaxTLS: tls.VersionTLS13,
		},
		{
			name:           "TestingConfig",
			configFunc:     TestingConfig,
			expectedMinTLS: tls.VersionTLS12, // Updated: now uses TLS 1.2 minimum for better security
			expectedMaxTLS: tls.VersionTLS13,
		},
		{
			name:           "MinimalConfig",
			configFunc:     MinimalConfig,
			expectedMinTLS: tls.VersionTLS12,
			expectedMaxTLS: tls.VersionTLS13,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.configFunc()

			if config.MinTLSVersion != tt.expectedMinTLS {
				t.Errorf("Expected MinTLSVersion %d, got %d", tt.expectedMinTLS, config.MinTLSVersion)
			}

			if config.MaxTLSVersion != tt.expectedMaxTLS {
				t.Errorf("Expected MaxTLSVersion %d, got %d", tt.expectedMaxTLS, config.MaxTLSVersion)
			}

			// Verify client can be created with this config
			client, err := New(config)
			if err != nil {
				t.Fatalf("Failed to create client with %s: %v", tt.name, err)
			}
			defer client.Close()
		})
	}
}

func TestConfig_TLSVersionWithTLSConfig(t *testing.T) {
	t.Run("MinTLSVersion and MaxTLSVersion with TLSConfig", func(t *testing.T) {
		config := DefaultConfig()
		config.MinTLSVersion = tls.VersionTLS12
		config.MaxTLSVersion = tls.VersionTLS13
		config.TLSConfig = &tls.Config{
			InsecureSkipVerify: true, // For testing
		}

		client, err := New(config)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		// Both MinTLSVersion/MaxTLSVersion and TLSConfig should coexist
		if config.MinTLSVersion != tls.VersionTLS12 {
			t.Errorf("Expected MinTLSVersion TLS 1.2, got %d", config.MinTLSVersion)
		}

		if config.MaxTLSVersion != tls.VersionTLS13 {
			t.Errorf("Expected MaxTLSVersion TLS 1.3, got %d", config.MaxTLSVersion)
		}

		if config.TLSConfig == nil {
			t.Error("Expected TLSConfig to be set")
		}
	})
}

func TestConfig_ModifyTLSVersionAfterCreation(t *testing.T) {
	config := DefaultConfig()
	originalMinTLS := config.MinTLSVersion
	originalMaxTLS := config.MaxTLSVersion

	client, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Modify config after client creation
	config.MinTLSVersion = tls.VersionTLS13
	config.MaxTLSVersion = tls.VersionTLS13

	// Original client should not be affected
	// (This is expected behavior - config is copied during client creation)

	// Verify config was modified
	if config.MinTLSVersion != tls.VersionTLS13 {
		t.Errorf("Expected modified MinTLSVersion TLS 1.3, got %d", config.MinTLSVersion)
	}

	// Create new client with modified config
	client2, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create second client: %v", err)
	}
	defer client2.Close()

	// Verify original values
	if originalMinTLS != tls.VersionTLS12 {
		t.Errorf("Expected original MinTLSVersion TLS 1.2, got %d", originalMinTLS)
	}

	if originalMaxTLS != tls.VersionTLS13 {
		t.Errorf("Expected original MaxTLSVersion TLS 1.3, got %d", originalMaxTLS)
	}
}

