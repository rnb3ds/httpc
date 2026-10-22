package fingerprint

import (
	"strconv"
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
		{"chrome", "chrome-155", osWindows},
		{"chrome-latest", "chrome-155", osWindows},
		{"chrome-155", "chrome-155", osWindows},
		{"chrome-152", "chrome-152", osWindows},
		{"chrome-windows", "chrome-155", osWindows},
		{"chrome-macos", "chrome-155", osMacOS},
		{"chrome-android", "chrome-155-android", osAndroid},
		{"chrome-ios", "chrome-155-ios", osIOS},
		{"firefox", "firefox-158", osWindows},
		{"firefox-latest", "firefox-158", osWindows},
		{"firefox-158", "firefox-158", osWindows},
		{"firefox-148", "firefox-148", osWindows},
		{"firefox-linux", "firefox-158", osLinux},
		{"firefox-android", "firefox-158-android", osAndroid},
		{"firefox-ios", "firefox-158-ios", osIOS},
		{"safari-ios", "safari-ios", osIOS},
		{"ios-safari-latest", "safari-ios", osIOS},
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
	all := []*browserProfile{
		chrome152Desktop, chrome152Mobile,
		chrome155Desktop, chrome155Android, chrome155IOS,
		firefox148Desktop, firefox148Mobile,
		firefox158Desktop, firefox158Android, firefox158IOS,
		safariIOS,
	}
	for _, p := range all {
		for _, os := range []osKey{osWindows, osMacOS, osLinux, osAndroid, osIOS} {
			for _, method := range []string{"GET", "POST", "PUT"} {
				merged := p.mergedHeaders(os, method)

				order := p.order(method)
				inOrder := make(map[string]bool, len(order))
				for _, name := range order {
					inOrder[name] = true
				}

				for _, f := range merged {
					name := strings.ToLower(f.Name)
					if strings.HasPrefix(name, ":") || f.Value == "" {
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
// frame order is wire-visible and part of the HTTP/2 fingerprint, so the
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
	for _, p := range []*browserProfile{
		chrome152Desktop, chrome155Desktop, firefox148Desktop, firefox158Desktop,
	} {
		for _, os := range allOS {
			if p.ua[os] == "" {
				t.Errorf("%s: no UA for OS %v", p.name, os)
			}
		}
	}
	if !strings.Contains(chrome155Desktop.ua[osIOS], "CriOS/") {
		t.Error("chrome 155 iOS UA missing CriOS/")
	}
	if !strings.Contains(firefox158Desktop.ua[osIOS], "FxiOS/") {
		t.Error("firefox 158 iOS UA missing FxiOS/")
	}
	if !strings.Contains(chrome155Desktop.ua[osAndroid], " Mobile Safari/") {
		t.Error("chrome Android UA missing Mobile marker")
	}
	// Firefox 147 (Jan 2026) moved the second language's q-value to 0.9;
	// both shipped majors postdate it.
	if !strings.Contains(firefox158Desktop.ua[osWindows], "Firefox/158.0") {
		t.Error("firefox 158 desktop UA not version 158")
	}
	for _, p := range []*browserProfile{firefox148Desktop, firefox158Desktop} {
		acceptLang := ""
		for _, f := range p.baseHeaders(p, osWindows) {
			if f.Name == "Accept-Language" {
				acceptLang = f.Value
			}
		}
		if acceptLang != "en-US,en;q=0.9" {
			t.Errorf("%s: Accept-Language = %q, want en-US,en;q=0.9", p.name, acceptLang)
		}
	}
	// Firefox emits no Client Hints.
	for _, p := range []*browserProfile{firefox148Desktop, firefox158Desktop, firefox158IOS} {
		if p.platform != nil || p.secCHUA != "" {
			t.Errorf("%s must not carry Client Hints data", p.name)
		}
	}
}

// TestProfile_IOSUsesWebKit pins the iOS invariant: every iOS browser is
// WebKit underneath, so the iOS profiles must share Safari's hello and the
// iOS H2 chain — not their desktop siblings'.
func TestProfile_IOSUsesWebKit(t *testing.T) {
	if chrome155IOS.hello == nil || firefox158IOS.hello == nil {
		t.Fatal("iOS profiles must define a hello factory")
	}
	if chrome155IOS.h2.connectionFlow != safariIOSH2.connectionFlow ||
		len(firefox158IOS.h2.settings) != len(safariIOSH2.settings) {
		t.Error("iOS profiles must use the WebKit H2 chain")
	}
	if chrome155IOS.h2.pseudoOrder[1] != ":scheme" {
		t.Errorf("iOS pseudo order = %v, want :scheme second", chrome155IOS.h2.pseudoOrder)
	}
	if chrome155IOS.shuffleExtensions {
		t.Error("iOS WebKit must not shuffle extensions")
	}
}

// TestProfile_SecCHUABrandLists pins the exact sec-ch-ua brand lists: the
// GREASE brand, its version and its position all rotate with the Chrome
// major, so a copied-forward value would silently contradict the spoofed
// version.
func TestProfile_SecCHUABrandLists(t *testing.T) {
	if got := chrome152Desktop.secCHUA; got != `"Chromium";v="152", "Not?A_Brand";v="24", "Google Chrome";v="152"` {
		t.Errorf("chrome-152 sec-ch-ua = %q", got)
	}
	if got := chrome155Desktop.secCHUA; got != `"Google Chrome";v="155", "Chromium";v="155", "Not(A:Brand";v="24"` {
		t.Errorf("chrome-155 sec-ch-ua = %q", got)
	}
}

// TestProfile_UAMajorCoherence pins the shipped-major consts against the UA
// tables: the consts drive the drift gates, so a mismatch here would make
// the gates lie about what the profiles actually claim.
func TestProfile_UAMajorCoherence(t *testing.T) {
	for os, ua := range chrome155UA {
		want := "Chrome/" + strconv.Itoa(ChromeMajor) + ".0.0.0"
		if os == osIOS {
			want = "CriOS/" + strconv.Itoa(ChromeMajor) + "."
		}
		if !strings.Contains(ua, want) {
			t.Errorf("chrome %v UA %q does not carry the shipped major %d", os, ua, ChromeMajor)
		}
	}
	for os, ua := range firefox158UA {
		want := "Firefox/" + strconv.Itoa(FirefoxMajor) + ".0"
		if os == osIOS {
			want = "FxiOS/" + strconv.Itoa(FirefoxMajor) + "."
		}
		if !strings.Contains(ua, want) {
			t.Errorf("firefox %v UA %q does not carry the shipped major %d", os, ua, FirefoxMajor)
		}
	}
	if !strings.Contains(safariIOS.ua[osIOS], "Version/26.0") {
		t.Errorf("safari iOS UA = %q, want the Version/26.0 token", safariIOS.ua[osIOS])
	}
}
