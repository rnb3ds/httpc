package validation

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateInputString(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		maxLen         int
		fieldName      string
		additionalFunc func(rune) error
		wantErr        bool
		errContains    string
	}{
		{
			name:      "valid input",
			input:     "valid-input",
			maxLen:    20,
			fieldName: "test field",
			wantErr:   false,
		},
		{
			name:        "empty input",
			input:       "",
			maxLen:      20,
			fieldName:   "test field",
			wantErr:     true,
			errContains: "cannot be empty",
		},
		{
			name:        "too long",
			input:       "this-is-too-long",
			maxLen:      10,
			fieldName:   "test field",
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:        "control character",
			input:       "test\x01",
			maxLen:      20,
			fieldName:   "test field",
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "CRLF injection",
			input:       "test\r\ninjection",
			maxLen:      20,
			fieldName:   "test field",
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:      "additional check passes",
			input:     "valid",
			maxLen:    20,
			fieldName: "test field",
			additionalFunc: func(r rune) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:      "additional check fails",
			input:     "invalid:",
			maxLen:    20,
			fieldName: "test field",
			additionalFunc: func(r rune) error {
				if r == ':' {
					return fmt.Errorf("colon not allowed")
				}
				return nil
			},
			wantErr:     true,
			errContains: "validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateInputString(tt.input, tt.maxLen, tt.fieldName, tt.additionalFunc)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateInputString() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateInputString() error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateInputString() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateCredential(t *testing.T) {
	tests := []struct {
		name        string
		cred        string
		maxLen      int
		checkColon  bool
		credType    string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid username",
			cred:       "validuser",
			maxLen:     20,
			checkColon: true,
			credType:   "username",
			wantErr:    false,
		},
		{
			name:       "valid password",
			cred:       "valid:password",
			maxLen:     20,
			checkColon: false,
			credType:   "password",
			wantErr:    false,
		},
		{
			name:        "username with colon",
			cred:        "user:name",
			maxLen:      20,
			checkColon:  true,
			credType:    "username",
			wantErr:     true,
			errContains: "colon",
		},
		{
			name:        "too long credential",
			cred:        strings.Repeat("a", 300),
			maxLen:      255,
			checkColon:  false,
			credType:    "credential",
			wantErr:     true,
			errContains: "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCredential(tt.cred, tt.maxLen, tt.checkColon, tt.credType)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateCredential() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateCredential() error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateCredential() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateToken(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid token",
			token:   "valid-token-123",
			wantErr: false,
		},
		{
			name:        "token with space",
			token:       "invalid token",
			wantErr:     true,
			errContains: "spaces",
		},
		{
			name:        "too long token",
			token:       strings.Repeat("a", 2049),
			wantErr:     true,
			errContains: "too long",
		},
		{
			name:        "token with control character",
			token:       "token\x01",
			wantErr:     true,
			errContains: "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateToken(tt.token)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateToken() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateToken() error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateToken() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateQueryKey(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid key",
			key:     "valid-key",
			wantErr: false,
		},
		{
			name:        "key with ampersand",
			key:         "key&invalid",
			wantErr:     true,
			errContains: "reserved characters",
		},
		{
			name:        "key with equals",
			key:         "key=invalid",
			wantErr:     true,
			errContains: "reserved characters",
		},
		{
			name:        "too long key",
			key:         strings.Repeat("a", 257),
			wantErr:     true,
			errContains: "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQueryKey(tt.key)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateQueryKey() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateQueryKey() error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateQueryKey() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateHeaderKeyValue(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid header",
			key:     "Content-Type",
			value:   "application/json",
			wantErr: false,
		},
		{
			name:        "pseudo header",
			key:         ":authority",
			value:       "example.com",
			wantErr:     true,
			errContains: "invalid character",
		},
		{
			name:        "invalid key character",
			key:         "Content Type",
			value:       "application/json",
			wantErr:     true,
			errContains: "invalid character",
		},
		{
			name:    "value with tab",
			key:     "Content-Type",
			value:   "application/json\t",
			wantErr: false,
		},
		{
			name:        "value with control character",
			key:         "Content-Type",
			value:       "application/json\x01",
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "too long value",
			key:         "Content-Type",
			value:       strings.Repeat("a", 8193),
			wantErr:     true,
			errContains: "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHeaderKeyValue(tt.key, tt.value)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateHeaderKeyValue() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateHeaderKeyValue() error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateHeaderKeyValue() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateCookieName(t *testing.T) {
	tests := []struct {
		name        string
		cookieName  string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid cookie name",
			cookieName: "session-id",
			wantErr:    false,
		},
		{
			name:        "cookie name with semicolon",
			cookieName:  "session;id",
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "cookie name with comma",
			cookieName:  "session,id",
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "too long cookie name",
			cookieName:  strings.Repeat("a", 257),
			wantErr:     true,
			errContains: "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCookieName(tt.cookieName)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateCookieName() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateCookieName() error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateCookieName() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateCookieValue(t *testing.T) {
	tests := []struct {
		name        string
		cookieValue string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid cookie value",
			cookieValue: "abc123",
			wantErr:     false,
		},
		{
			name:        "cookie value with control character",
			cookieValue: "abc\x01",
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name:        "too long cookie value",
			cookieValue: strings.Repeat("a", 4097),
			wantErr:     true,
			errContains: "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCookieValue(tt.cookieValue)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateCookieValue() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateCookieValue() error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateCookieValue() unexpected error = %v", err)
				}
			}
		})
	}
}
