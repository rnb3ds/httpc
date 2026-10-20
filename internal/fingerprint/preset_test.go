package fingerprint

import "testing"

func TestResolvePresetName(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantOK   bool
	}{
		// Short aliases and canonical desktop names.
		{"chrome", "chrome", true},
		{"firefox", "firefox", true},
		{"chrome-latest", "chrome-latest", true},
		{"firefox-latest", "firefox-latest", true},
		// Version-accurate names (they name exactly what the backend ships).
		{"chrome-152", "chrome-152", true},
		{"firefox-148", "firefox-148", true},
		// OS-suffixed desktop variants, case/whitespace tolerant.
		{"chrome-windows", "chrome-windows", true},
		{"chrome-macos", "chrome-macos", true},
		{"chrome-linux", "chrome-linux", true},
		{"firefox-windows", "firefox-windows", true},
		{"  Chrome-Windows ", "chrome-windows", true},
		// Mobile variants and legacy compound aliases.
		{"chrome-android", "chrome-android", true},
		{"chrome-ios", "chrome-ios", true},
		{"firefox-android", "firefox-android", true},
		{"firefox-ios", "firefox-ios", true},
		{"android-chrome-latest", "android-chrome-latest", true},
		{"ios-chrome-latest", "ios-chrome-latest", true},
		// Unknown names are rejected: the backend has no Safari profile, and
		// version pins other than the shipped ones (chrome-152/firefox-148)
		// would silently misreport the version actually impersonated.
		{"", "", false},
		{"opera", "", false},
		{"edge", "", false},
		{"safari", "", false},
		{"safari-latest", "", false},
		{"ios", "", false},
		{"ios-safari-latest", "", false},
		{"safari-18", "", false},
		{"chrome-155", "", false},
		{"chrome-999", "", false},
		{"netscape", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			name, ok := ResolvePresetName(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("ResolvePresetName(%q) ok = %v, want %v", tc.input, ok, tc.wantOK)
			}
			if ok && name != tc.wantName {
				t.Errorf("ResolvePresetName(%q) = %q, want %q", tc.input, name, tc.wantName)
			}
		})
	}
}
