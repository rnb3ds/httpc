package httpc

import (
	"net/http"
	"testing"
)

func TestWithCookieString(t *testing.T) {
	tests := []struct {
		name            string
		cookieString    string
		expectedCount   int
		expectedCookies map[string]string
		expectError     bool
		errorContains   string
	}{
		{
			name:            "empty string",
			cookieString:    "",
			expectedCount:   0,
			expectedCookies: map[string]string{},
			expectError:     false,
		},
		{
			name:          "single cookie",
			cookieString:  "BSID=4418ECBB1281B550",
			expectedCount: 1,
			expectedCookies: map[string]string{
				"BSID": "4418ECBB1281B550",
			},
			expectError: false,
		},
		{
			name:          "multiple cookies",
			cookieString:  "BSID=4418ECBB1281B550; PSTM=1733760779; BS=kUwNTVFcEUBUItoc",
			expectedCount: 3,
			expectedCookies: map[string]string{
				"BSID": "4418ECBB1281B550",
				"PSTM":     "1733760779",
				"BS":    "kUwNTVFcEUBUItoc",
			},
			expectError: false,
		},
		{
			name:          "complex cookie string with special characters",
			cookieString:  "BID=01E8D701159F774:FG=1; MCITY=-257%3A; BUPN=12314753",
			expectedCount: 3,
			expectedCookies: map[string]string{
				"BID": "01E8D701159F774:FG=1",
				"MCITY":   "-257%3A",
				"BUPN":  "12314753",
			},
			expectError: false,
		},
		{
			name:          "cookies with spaces around values",
			cookieString:  "session = abc123 ; token = xyz789 ",
			expectedCount: 2,
			expectedCookies: map[string]string{
				"session": "abc123",
				"token":   "xyz789",
			},
			expectError: false,
		},
		{
			name:          "cookie with empty value",
			cookieString:  "empty_cookie=; normal_cookie=value",
			expectedCount: 2,
			expectedCookies: map[string]string{
				"empty_cookie":  "",
				"normal_cookie": "value",
			},
			expectError: false,
		},
		{
			name:          "malformed cookie without equals",
			cookieString:  "invalid_cookie_without_equals",
			expectedCount: 0,
			expectError:   true,
			errorContains: "malformed cookie pair",
		},
		{
			name:          "cookie with empty name",
			cookieString:  "=value_without_name",
			expectedCount: 0,
			expectError:   true,
			errorContains: "empty cookie name",
		},
		{
			name:          "trailing semicolon",
			cookieString:  "cookie1=value1; cookie2=value2;",
			expectedCount: 2,
			expectedCookies: map[string]string{
				"cookie1": "value1",
				"cookie2": "value2",
			},
			expectError: false,
		},
		{
			name:          "multiple semicolons",
			cookieString:  "cookie1=value1;; cookie2=value2",
			expectedCount: 2,
			expectedCookies: map[string]string{
				"cookie1": "value1",
				"cookie2": "value2",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &Request{}
			option := WithCookieString(tt.cookieString)
			err := option(req)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorContains != "" && !containsString(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain %q, got %q", tt.errorContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(req.Cookies) != tt.expectedCount {
				t.Errorf("expected %d cookies, got %d", tt.expectedCount, len(req.Cookies))
				return
			}

			// Verify each expected cookie
			for expectedName, expectedValue := range tt.expectedCookies {
				found := false
				for _, cookie := range req.Cookies {
					if cookie.Name == expectedName {
						found = true
						if cookie.Value != expectedValue {
							t.Errorf("cookie %s: expected value %q, got %q", expectedName, expectedValue, cookie.Value)
						}
						// Verify secure defaults
						if !cookie.HttpOnly {
							t.Errorf("cookie %s: expected HttpOnly=true, got false", expectedName)
						}
						if cookie.Secure {
							t.Errorf("cookie %s: expected Secure=false, got true", expectedName)
						}
						if cookie.SameSite != http.SameSiteLaxMode {
							t.Errorf("cookie %s: expected SameSite=Lax, got %v", expectedName, cookie.SameSite)
						}
						break
					}
				}
				if !found {
					t.Errorf("expected cookie %s not found", expectedName)
				}
			}
		})
	}
}

func TestWithCookieString_Integration(t *testing.T) {
	t.Parallel()

	// Test with actual cookie string from the example
	cookieString := "BSID=4418ECBB1281B550; PSTM=1733760779; BS=kUwNTVFcEUBUItoc; BID=01E8D701159F774:FG=1; MCITY=-257%3A; BUPN=12314753; H_WISE_SIDS_BFESS=60276_666772; H_PS_PSSID=60276_666819; H_WISE_SIDS=60276_69; BDSVRTM=274"

	req := &Request{}
	option := WithCookieString(cookieString)
	err := option(req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedCookies := map[string]string{
		"BSID":          "4418ECBB1281B550",
		"PSTM":              "1733760779",
		"BS":             "kUwNTVFcEUBUItoc",
		"BID":           "01E8D701159F774:FG=1",
		"MCITY":             "-257%3A",
		"BUPN":            "12314753",
		"H_WISE_SIDS_BFESS": "60276_666772",
		"H_PS_PSSID":        "60276_666819",
		"H_WISE_SIDS":       "60276_69",
		"BDSVRTM":           "274",
	}

	if len(req.Cookies) != len(expectedCookies) {
		t.Errorf("expected %d cookies, got %d", len(expectedCookies), len(req.Cookies))
	}

	for expectedName, expectedValue := range expectedCookies {
		found := false
		for _, cookie := range req.Cookies {
			if cookie.Name == expectedName {
				found = true
				if cookie.Value != expectedValue {
					t.Errorf("cookie %s: expected value %q, got %q", expectedName, expectedValue, cookie.Value)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected cookie %s not found", expectedName)
		}
	}
}

func TestWithCookieString_ValidationLimits(t *testing.T) {
	t.Parallel()

	// Test cookie name too long
	longName := make([]byte, maxCookieNameLen+1)
	for i := range longName {
		longName[i] = 'a'
	}
	cookieString := string(longName) + "=value"

	req := &Request{}
	option := WithCookieString(cookieString)
	err := option(req)

	if err == nil {
		t.Error("expected error for cookie name too long")
	}

	// Test cookie value too long
	longValue := make([]byte, maxCookieValueLen+1)
	for i := range longValue {
		longValue[i] = 'a'
	}
	cookieString = "name=" + string(longValue)

	req = &Request{}
	option = WithCookieString(cookieString)
	err = option(req)

	if err == nil {
		t.Error("expected error for cookie value too long")
	}
}

func TestWithCookieString_CombineWithOtherCookieMethods(t *testing.T) {
	t.Parallel()

	req := &Request{}

	// Add cookie using WithCookieValue
	err := WithCookieValue("manual", "cookie")(req)
	if err != nil {
		t.Fatalf("WithCookieValue failed: %v", err)
	}

	// Add cookies using WithCookieString
	err = WithCookieString("parsed1=value1; parsed2=value2")(req)
	if err != nil {
		t.Fatalf("WithCookieString failed: %v", err)
	}

	// Add cookie using WithCookie
	cookie := &http.Cookie{
		Name:  "direct",
		Value: "cookie",
	}
	err = WithCookie(cookie)(req)
	if err != nil {
		t.Fatalf("WithCookie failed: %v", err)
	}

	// Should have 4 cookies total
	if len(req.Cookies) != 4 {
		t.Errorf("expected 4 cookies, got %d", len(req.Cookies))
	}

	// Verify all cookies are present
	expectedNames := []string{"manual", "parsed1", "parsed2", "direct"}
	for _, expectedName := range expectedNames {
		found := false
		for _, cookie := range req.Cookies {
			if cookie.Name == expectedName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected cookie %s not found", expectedName)
		}
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (len(substr) == 0 || indexString(s, substr) >= 0)
}

// Helper function to find index of substring
func indexString(s, substr string) int {
	n := len(substr)
	if n == 0 {
		return 0
	}
	if n > len(s) {
		return -1
	}
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == substr {
			return i
		}
	}
	return -1
}
