package fingerprint

import (
	"slices"
	"testing"
)

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
		{"chrome-155", "chrome-155", true},
		{"chrome-152", "chrome-152", true},
		{"firefox-158", "firefox-158", true},
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
		// The third family: iOS Safari on the verified WebKit data.
		{"safari-ios", "safari-ios", true},
		{"ios-safari-latest", "ios-safari-latest", true},
		// Unknown names are rejected: there is no desktop-Safari profile (the
		// Safari data is iOS WebKit, served under safari-ios), and version
		// pins other than the shipped ones (chrome-155/chrome-152/
		// firefox-158/firefox-148) would silently misreport the version
		// actually impersonated.
		{"", "", false},
		{"opera", "", false},
		{"edge", "", false},
		{"safari", "", false},
		{"safari-latest", "", false},
		{"ios", "", false},
		{"safari-18", "", false},
		{"chrome-153", "", false},
		{"firefox-160", "", false},
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

// TestPresetNames pins the registry size and the PresetNames/ResolvePresetName
// round-trip. The count is deliberate over-pinning: the preset vocabulary is
// small and every entry is a conscious decision, so a size change must update
// this pin — and the failure message points at the baseline table in
// dev_test/tls_fingerprint_verify_all.go, which must gain/lose the same rows.
func TestPresetNames(t *testing.T) {
	names := PresetNames()
	if !slices.IsSorted(names) {
		t.Errorf("PresetNames not sorted: %v", names)
	}
	if len(names) != 22 {
		t.Errorf("registry has %d presets, want 22 — if a preset was added/removed, update this pin and dev_test/tls_fingerprint_verify_all.go presetCases in the same change: %v",
			len(names), names)
	}
	for _, n := range names {
		if resolved, ok := ResolvePresetName(n); !ok || resolved != n {
			t.Errorf("ResolvePresetName(%q) = (%q, %v), want (%q, true)", n, resolved, ok, n)
		}
	}
}
