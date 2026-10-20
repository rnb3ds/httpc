package fingerprint

import "strings"

// profileSpec describes one surf impersonation target: a browser family and
// an optional operating-system flavour. An empty OS means the desktop
// default; "android" and "ios" select the mobile form factor.
type profileSpec struct {
	browser string // "chrome" (v152) or "firefox" (v148)
	os      string // "", "windows", "macos", "linux", "android", "ios"
}

// presets is the local registry of canonical preset names. The surf backend
// ships two browser families at fixed versions (Chrome 152, Firefox 148)
// with per-OS variants; names are kept browser-first, matching the vocabulary
// the previous httpcloak backend used. Safari has no surf profile and is
// deliberately absent — validation rejects it with the supported list.
var presets = map[string]profileSpec{
	// Chrome 152 desktop.
	"chrome":         {browser: "chrome"},
	"chrome-latest":  {browser: "chrome"},
	"chrome-152":     {browser: "chrome"},
	"chrome-windows": {browser: "chrome", os: "windows"},
	"chrome-macos":   {browser: "chrome", os: "macos"},
	"chrome-linux":   {browser: "chrome", os: "linux"},

	// Chrome 152 mobile (legacy compound aliases kept from the httpcloak
	// vocabulary, where they named per-OS mobile presets).
	"chrome-android":        {browser: "chrome", os: "android"},
	"chrome-ios":            {browser: "chrome", os: "ios"},
	"android-chrome-latest": {browser: "chrome", os: "android"},
	"ios-chrome-latest":     {browser: "chrome", os: "ios"},

	// Firefox 148 desktop.
	"firefox":         {browser: "firefox"},
	"firefox-latest":  {browser: "firefox"},
	"firefox-148":     {browser: "firefox"},
	"firefox-windows": {browser: "firefox", os: "windows"},
	"firefox-macos":   {browser: "firefox", os: "macos"},
	"firefox-linux":   {browser: "firefox", os: "linux"},

	// Firefox 148 mobile.
	"firefox-android": {browser: "firefox", os: "android"},
	"firefox-ios":     {browser: "firefox", os: "ios"},
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
