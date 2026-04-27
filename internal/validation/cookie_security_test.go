package validation

import (
	"net/http"
	"strings"
	"testing"
)

func TestCookieSecurityConfig(t *testing.T) {
	t.Run("DefaultCookieSecurityConfig", func(t *testing.T) {
		cfg := DefaultCookieSecurityConfig()
		if cfg == nil {
			t.Fatal("expected non-nil config")
		}
		if cfg.RequireSecure {
			t.Error("expected RequireSecure to be false by default")
		}
		if cfg.RequireHttpOnly {
			t.Error("expected RequireHttpOnly to be false by default")
		}
		if cfg.RequireSameSite != "" {
			t.Error("expected RequireSameSite to be empty by default")
		}
		if !cfg.AllowSameSiteNone {
			t.Error("expected AllowSameSiteNone to be true by default")
		}
	})

	t.Run("StrictCookieSecurityConfig", func(t *testing.T) {
		cfg := StrictCookieSecurityConfig()
		if cfg == nil {
			t.Fatal("expected non-nil config")
		}
		if !cfg.RequireSecure {
			t.Error("expected RequireSecure to be true in strict mode")
		}
		if !cfg.RequireHttpOnly {
			t.Error("expected RequireHttpOnly to be true in strict mode")
		}
		if cfg.RequireSameSite != "Strict" {
			t.Errorf("expected RequireSameSite to be 'Strict', got %q", cfg.RequireSameSite)
		}
		if cfg.AllowSameSiteNone {
			t.Error("expected AllowSameSiteNone to be false in strict mode")
		}
	})
}

func TestValidateCookieSecurity(t *testing.T) {
	tests := []struct {
		name     string
		cookie   *http.Cookie
		config   *CookieSecurityConfig
		wantErr  bool
		errMatch string
	}{
		{
			name:     "nil cookie",
			cookie:   nil,
			config:   DefaultCookieSecurityConfig(),
			wantErr:  true,
			errMatch: "cookie is nil",
		},
		{
			name:    "nil config",
			cookie:  &http.Cookie{Name: "test", Value: "value"},
			config:  nil,
			wantErr: false,
		},
		{
			name:     "missing Secure attribute",
			cookie:   &http.Cookie{Name: "test", Value: "value", Secure: false},
			config:   &CookieSecurityConfig{RequireSecure: true},
			wantErr:  true,
			errMatch: "missing Secure attribute",
		},
		{
			name:     "missing HttpOnly attribute",
			cookie:   &http.Cookie{Name: "test", Value: "value", HttpOnly: false},
			config:   &CookieSecurityConfig{RequireHttpOnly: true},
			wantErr:  true,
			errMatch: "missing HttpOnly attribute",
		},
		{
			name:    "valid cookie with Secure and HttpOnly",
			cookie:  &http.Cookie{Name: "test", Value: "value", Secure: true, HttpOnly: true},
			config:  &CookieSecurityConfig{RequireSecure: true, RequireHttpOnly: true},
			wantErr: false,
		},
		{
			name:     "SameSite None not allowed",
			cookie:   &http.Cookie{Name: "test", Value: "value", SameSite: http.SameSiteNoneMode},
			config:   &CookieSecurityConfig{AllowSameSiteNone: false},
			wantErr:  true,
			errMatch: "SameSite=None is not allowed",
		},
		{
			name:    "SameSite None allowed",
			cookie:  &http.Cookie{Name: "test", Value: "value", SameSite: http.SameSiteNoneMode},
			config:  &CookieSecurityConfig{AllowSameSiteNone: true},
			wantErr: false,
		},
		{
			name:    "valid all attributes",
			cookie:  &http.Cookie{Name: "test", Value: "value", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Path: "/"},
			config:  StrictCookieSecurityConfig(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCookieSecurity(tt.cookie, tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errMatch != "" && !strings.Contains(err.Error(), tt.errMatch) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMatch)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

// TestSameSiteConversions tests the sameSiteToString and stringToSameSite conversion
// functions across all SameSite modes and edge cases.
func TestSameSiteConversions(t *testing.T) {
	t.Run("sameSiteToString", func(t *testing.T) {
		tests := []struct {
			name     string
			sameSite http.SameSite
			want     string
		}{
			{"DefaultMode", http.SameSiteDefaultMode, "Default"},
			{"LaxMode", http.SameSiteLaxMode, "Lax"},
			{"StrictMode", http.SameSiteStrictMode, "Strict"},
			{"NoneMode", http.SameSiteNoneMode, "None"},
			{"Unknown value", http.SameSite(99), ""},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := sameSiteToString(tt.sameSite)
				if got != tt.want {
					t.Errorf("sameSiteToString(%v) = %q, want %q", tt.sameSite, got, tt.want)
				}
			})
		}
	})
}

// TestValidateSameSite tests the validateSameSite function with various SameSite
// requirements and cookie configurations.
func TestValidateSameSite(t *testing.T) {
	tests := []struct {
		name      string
		cookie    *http.Cookie
		required  string
		allowNone bool
		wantErr   bool
	}{
		{
			name:      "Strict cookie with Strict requirement",
			cookie:    &http.Cookie{Name: "test", SameSite: http.SameSiteStrictMode},
			required:  "Strict",
			allowNone: false,
			wantErr:   false,
		},
		{
			name:      "Lax cookie with Lax requirement",
			cookie:    &http.Cookie{Name: "test", SameSite: http.SameSiteLaxMode},
			required:  "Lax",
			allowNone: false,
			wantErr:   false,
		},
		{
			name:      "None cookie with None requirement but allowNone=false",
			cookie:    &http.Cookie{Name: "test", SameSite: http.SameSiteNoneMode},
			required:  "None",
			allowNone: false,
			wantErr:   true,
		},
		{
			name:      "None cookie with None requirement and allowNone=true",
			cookie:    &http.Cookie{Name: "test", SameSite: http.SameSiteNoneMode},
			required:  "None",
			allowNone: true,
			wantErr:   false,
		},
		{
			name:      "Default mode cookie with Lax requirement accepted",
			cookie:    &http.Cookie{Name: "test", SameSite: http.SameSiteDefaultMode},
			required:  "Lax",
			allowNone: false,
			wantErr:   false,
		},
		{
			name:      "Default mode cookie with Strict requirement rejected",
			cookie:    &http.Cookie{Name: "test", SameSite: http.SameSiteDefaultMode},
			required:  "Strict",
			allowNone: false,
			wantErr:   true,
		},
		{
			name:      "Mismatch Strict cookie with Lax requirement",
			cookie:    &http.Cookie{Name: "test", SameSite: http.SameSiteStrictMode},
			required:  "Lax",
			allowNone: false,
			wantErr:   true,
		},
		{
			name:      "Zero SameSite with Lax requirement accepted as default",
			cookie:    &http.Cookie{Name: "test", SameSite: 0},
			required:  "lax",
			allowNone: false,
			wantErr:   false,
		},
		{
			name:      "Zero SameSite with Strict requirement rejected",
			cookie:    &http.Cookie{Name: "test", SameSite: 0},
			required:  "strict",
			allowNone: false,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSameSite(tt.cookie, tt.required, tt.allowNone)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
