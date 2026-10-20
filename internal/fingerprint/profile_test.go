package fingerprint

import (
	"strings"
	"testing"
)

// TestProfileFor_PresetResolution pins the preset-registry → profile/OS
// resolution table, including the desktop default for bare presets and the
// mobile variant selection for android/ios.
func TestProfileFor_PresetResolution(t *testing.T) {
	tests := []struct {
		preset  string
		profile string
		os      osKey
	}{
		{"chrome", "chrome-152", osWindows},
		{"chrome-latest", "chrome-152", osWindows},
		{"chrome-windows", "chrome-152", osWindows},
		{"chrome-macos", "chrome-152", osMacOS},
		{"chrome-android", "chrome-152-mobile", osAndroid},
		{"chrome-ios", "chrome-152-mobile", osIOS},
		{"firefox", "firefox-148", osWindows},
		{"firefox-latest", "firefox-148", osWindows},
		{"firefox-linux", "firefox-148", osLinux},
		{"firefox-android", "firefox-148-mobile", osAndroid},
	}

	for _, tt := range tests {
		t.Run(tt.preset, func(t *testing.T) {
			spec, ok := presets[tt.preset]
			if !ok {
				t.Fatalf("preset %q missing from registry", tt.preset)
			}
			p, os := profileFor(spec)
			if p.name != tt.profile {
				t.Errorf("profile = %q, want %q", p.name, tt.profile)
			}
			if os != tt.os {
				t.Errorf("os = %v, want %v", os, tt.os)
			}
		})
	}
}

// TestProfile_OrderCoversHeaderSet pins the invariant the wire order depends
// on: every header the profile actually EMITS (non-empty value under the
// method's merge) appears in that method's order table, so it lands at its
// browser position. Empty placeholders (origin on GET, content-length on
// GET) may legitimately be absent — they are never sent, and a caller-set
// value for such a name rides among the unlisted headers, matching upstream
// behavior.
func TestProfile_OrderCoversHeaderSet(t *testing.T) {
	for _, p := range []*browserProfile{chrome152Desktop, chrome152Mobile, firefox148Desktop, firefox148Mobile} {
		for _, os := range []osKey{osWindows, osMacOS, osLinux, osAndroid, osIOS} {
			for _, method := range []string{"GET", "POST", "PUT"} {
				merged := p.mergedHeaders(os, method)

				order := p.order(method)
				inOrder := make(map[string]bool, len(order))
				for _, name := range order {
					inOrder[name] = true
				}

				for name, value := range merged {
					if strings.HasPrefix(name, ":") || value == "" {
						continue
					}
					if !inOrder[name] {
						t.Errorf("%s/%v/%s: emitted header %q missing from order table", p.name, os, method, name)
					}
				}
			}
		}
	}
}

// TestProfile_H2SettingsSequence pins the exact SETTINGS emission order —
// frame order is wire-visible and part of the Akamai fingerprint, so the
// sequences are asserted as literal snapshots.
func TestProfile_H2SettingsSequence(t *testing.T) {
	chromeWant := []h2Setting{
		{settingHeaderTableSize, 65536},
		{settingEnablePush, 0},
		{settingInitialWindowSize, 6291456},
		{settingMaxHeaderListSize, 262144},
	}
	if len(chrome152Desktop.h2.settings) != len(chromeWant) {
		t.Fatalf("chrome settings length = %d, want %d", len(chrome152Desktop.h2.settings), len(chromeWant))
	}
	for i, s := range chrome152Desktop.h2.settings {
		if s != chromeWant[i] {
			t.Errorf("chrome setting[%d] = %+v, want %+v", i, s, chromeWant[i])
		}
	}
	if chrome152Desktop.h2.connectionFlow != 15663105 || chrome152Desktop.h2.streamID != 0 ||
		!chrome152Desktop.h2.hasPriority || !chrome152Desktop.h2.priority.exclusive || chrome152Desktop.h2.priority.weight != 255 {
		t.Errorf("chrome h2 knobs: %+v", chrome152Desktop.h2)
	}

	firefoxWant := []h2Setting{
		{settingHeaderTableSize, 65536},
		{settingEnablePush, 0},
		{settingInitialWindowSize, 131072},
		{settingMaxFrameSize, 16384},
	}
	if len(firefox148Desktop.h2.settings) != len(firefoxWant) {
		t.Fatalf("firefox settings length = %d, want %d", len(firefox148Desktop.h2.settings), len(firefoxWant))
	}
	for i, s := range firefox148Desktop.h2.settings {
		if s != firefoxWant[i] {
			t.Errorf("firefox setting[%d] = %+v, want %+v", i, s, firefoxWant[i])
		}
	}
	if firefox148Desktop.h2.streamID != 3 || firefox148Desktop.h2.connectionFlow != 12517377 ||
		firefox148Desktop.h2.priority.exclusive || firefox148Desktop.h2.priority.weight != 41 {
		t.Errorf("firefox h2 knobs: %+v", firefox148Desktop.h2)
	}
}

// TestProfile_UACoverage pins that every profile resolves a UA for every
// supported OS, and the shape of the OS-specific entries (CriOS/FxiOS for
// iOS, Mobile for Android).
func TestProfile_UACoverage(t *testing.T) {
	allOS := []osKey{osWindows, osMacOS, osLinux, osAndroid, osIOS}
	for _, p := range []*browserProfile{chrome152Desktop, firefox148Desktop} {
		for _, os := range allOS {
			if p.ua[os] == "" {
				t.Errorf("%s: no UA for OS %v", p.name, os)
			}
		}
	}
	if !strings.Contains(chrome152Desktop.ua[osIOS], "CriOS/") {
		t.Error("chrome iOS UA missing CriOS/")
	}
	if !strings.Contains(firefox148Desktop.ua[osIOS], "FxiOS/") {
		t.Error("firefox iOS UA missing FxiOS/")
	}
	if !strings.Contains(chrome152Desktop.ua[osAndroid], " Mobile Safari/") {
		t.Error("chrome Android UA missing Mobile marker")
	}
	// Firefox emits no Client Hints.
	if firefox148Desktop.platform != nil || firefox148Desktop.secCHUA != "" {
		t.Error("firefox profile must not carry Client Hints data")
	}
}
