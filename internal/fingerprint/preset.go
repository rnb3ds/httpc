package fingerprint

import "strings"

// profileSpec describes one surf impersonation target: a browser family and
// an optional operating-system flavour. An empty OS means the desktop
// default; "android" and "ios" select the mobile form factor.
type profileSpec struct {
	browser string // "chrome" (v155), "chrome152" / "firefox148" (retired pins), "firefox" (v158)
	os      string // "", "windows", "macos", "linux", "android", "ios"
}

// presets is the local registry of canonical preset names. The backend ships
// two browser families — Chrome 155 and Firefox 158 (the current labels) —
// plus the retired Chrome 152 / Firefox 148 as explicit version pins (Chrome
// 152–156 share one ClientHello; only UA and brand strings differ); names
// are kept browser-first, matching the vocabulary the previous httpcloak
// backend used. The Safari family is iOS-only ("safari-ios" /
// "ios-safari-latest", built on the same WebKit data as the iOS
// Chrome/Firefox profiles); there is deliberately no desktop-safari preset —
// see profile_safari.go.
var presets = map[string]profileSpec{
	// Chrome 155 desktop (current label).
	"chrome":         {browser: "chrome"},
	"chrome-latest":  {browser: "chrome"},
	"chrome-155":     {browser: "chrome"},
	"chrome-windows": {browser: "chrome", os: "windows"},
	"chrome-macos":   {browser: "chrome", os: "macos"},
	"chrome-linux":   {browser: "chrome", os: "linux"},

	// Chrome 155 mobile (legacy compound aliases kept from the httpcloak
	// vocabulary, where they named per-OS mobile presets).
	"chrome-android":        {browser: "chrome", os: "android"},
	"chrome-ios":            {browser: "chrome", os: "ios"},
	"android-chrome-latest": {browser: "chrome", os: "android"},
	"ios-chrome-latest":     {browser: "chrome", os: "ios"},

	// Chrome 152 desktop (the retired major, pinned; served only by its
	// exact name).
	"chrome-152": {browser: "chrome152"},

	// Firefox 158 desktop (current label).
	"firefox":         {browser: "firefox"},
	"firefox-latest":  {browser: "firefox"},
	"firefox-158":     {browser: "firefox"},
	"firefox-windows": {browser: "firefox", os: "windows"},
	"firefox-macos":   {browser: "firefox", os: "macos"},
	"firefox-linux":   {browser: "firefox", os: "linux"},

	// Firefox 158 mobile (iOS resolves to the WebKit profile).
	"firefox-android": {browser: "firefox", os: "android"},
	"firefox-ios":     {browser: "firefox", os: "ios"},

	// Firefox 148 desktop (the retired major, pinned; served only by its
	// exact name).
	"firefox-148": {browser: "firefox148"},

	// Safari (iOS only): the third family, on the same verified WebKit data
	// as the iOS Chrome/Firefox profiles. There is deliberately no
	// desktop-safari preset — see profile_safari.go.
	"safari-ios":        {browser: "safari", os: "ios"},
	"ios-safari-latest": {browser: "safari", os: "ios"},
}

// ResolvePresetName normalizes a configured fingerprint value (short alias,
// version-accurate name, or OS-suffixed name like "chrome", "chrome-152",
// "chrome-windows") and reports whether the registry knows it. Matching is
// case-insensitive with surrounding whitespace trimmed, mirroring the
// tolerance the previous backend's resolver had for "Chrome" / " chrome ".
func ResolvePresetName(v string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(v))
	if name == "" {
		return "", false
	}
	if _, ok := presets[name]; ok {
		return name, true
	}
	return "", false
}
