package validation

import (
	"fmt"
	"net/http"
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

func TestValidateCookie(t *testing.T) {
	tests := []struct {
		name        string
		cookie      *http.Cookie
		wantErr     bool
		errContains string
	}{
		{
			name: "valid cookie",
			cookie: &http.Cookie{
				Name:  "session",
				Value: "abc123",
			},
			wantErr: false,
		},
		{
			name: "valid cookie with domain and path",
			cookie: &http.Cookie{
				Name:   "session",
				Value:  "abc123",
				Domain: "example.com",
				Path:   "/api",
			},
			wantErr: false,
		},
		{
			name: "invalid cookie name",
			cookie: &http.Cookie{
				Name:  "session;id",
				Value: "abc123",
			},
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name: "invalid cookie value",
			cookie: &http.Cookie{
				Name:  "session",
				Value: "abc\x01",
			},
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name: "domain too long",
			cookie: &http.Cookie{
				Name:   "session",
				Value:  "abc123",
				Domain: strings.Repeat("a", 256),
			},
			wantErr:     true,
			errContains: "domain too long",
		},
		{
			name: "path too long",
			cookie: &http.Cookie{
				Name:  "session",
				Value: "abc123",
				Path:  strings.Repeat("a", 1025),
			},
			wantErr:     true,
			errContains: "path too long",
		},
		{
			name: "domain with control character",
			cookie: &http.Cookie{
				Name:   "session",
				Value:  "abc123",
				Domain: "example\x01.com",
			},
			wantErr:     true,
			errContains: "invalid characters",
		},
		{
			name: "path with control character",
			cookie: &http.Cookie{
				Name:  "session",
				Value: "abc123",
				Path:  "/api\x01/path",
			},
			wantErr:     true,
			errContains: "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCookie(tt.cookie)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateCookie() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateCookie() error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateCookie() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestIsValidHeaderChar(t *testing.T) {
	tests := []struct {
		char  rune
		valid bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'-', true},
		{' ', false},
		{':', false},
		{'_', false},
		{'.', false},
		{'\n', false},
		{'\r', false},
		{'\t', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			result := IsValidHeaderChar(tt.char)
			if result != tt.valid {
				t.Errorf("IsValidHeaderChar(%q) = %v, want %v", tt.char, result, tt.valid)
			}
		})
	}
}

func TestIsValidHeaderString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		isValid bool
	}{
		{"valid string", "Content-Type: application/json", true},
		{"string with tab", "value\twith\ttab", true},
		{"string with CR", "value\rwithCR", false},
		{"string with LF", "value\nwithLF", false},
		{"string with CRLF", "value\r\nwithCRLF", false},
		{"string with null", "value\x00withnull", false},
		{"string with DEL", "value\x7FwithDEL", false},
		{"empty string", "", true},
		{"string with control char", "value\x01control", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidHeaderString(tt.input)
			if result != tt.isValid {
				t.Errorf("IsValidHeaderString(%q) = %v, want %v", tt.input, result, tt.isValid)
			}
		})
	}
}

func TestValidateFieldName(t *testing.T) {
	tests := []struct {
		name        string
		fieldName   string
		fieldType   string
		wantErr     bool
		errContains string
	}{
		{"valid field", "file1", "form field", false, ""},
		{"field with quotes", "file\"name", "form field", true, "dangerous characters"},
		{"field with angle brackets", "file<name>", "form field", true, "dangerous characters"},
		{"field with ampersand", "file&name", "form field", true, "dangerous characters"},
		{"field with single quote", "file'name", "form field", true, "dangerous characters"},
		{"empty field", "", "form field", true, "cannot be empty"},
		{"too long field", strings.Repeat("a", 257), "form field", true, "too long"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFieldName(tt.fieldName, tt.fieldType)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateFieldName() expected error, got nil")
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateFieldName() error = %v, want to contain %v", err, tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateFieldName() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestValidateHeaderKeyValue_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		key         string
		value       string
		wantErr     bool
		errContains string
	}{
		{"empty key", "", "value", true, "cannot be empty"},
		{"pseudo header :path", ":path", "/api", true, "invalid character"},
		{"pseudo header :method", ":method", "GET", true, "invalid character"},
		{"key with space", "Content Type", "application/json", true, "invalid character"},
		{"key with underscore", "Content_Type", "application/json", true, "invalid character"},
		{"key with dot", "Content.Type", "application/json", true, "invalid character"},
		{"empty value", "Content-Type", "", false, ""},
		{"value at max length", "X-Custom", strings.Repeat("a", MaxHeaderValueLen), false, ""},
		{"value over max length", "X-Custom", strings.Repeat("a", MaxHeaderValueLen+1), true, "too long"},
		{"key at max length", strings.Repeat("a", MaxHeaderKeyLen), "value", false, ""},
		{"key over max length", strings.Repeat("a", MaxHeaderKeyLen+1), "value", true, "too long"},
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
