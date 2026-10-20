// Browser profile model: one browserProfile value describes everything the
// transport needs to reproduce one browser version's wire behavior — TLS
// ClientHello spec (or utls built-in hello ID), per-OS User-Agent and
// sec-ch-ua-platform data, the per-method ordered header block, the static
// header set with its empty placeholders, and the exact HTTP/2 SETTINGS
// sequence.
//
// Profile data ported from github.com/enetx/surf (profiles/), Copyright
// enetx/surf authors, MIT License. Mobile variants are today verbatim copies
// of the desktop data (as they are upstream); each keeps its own variables so
// a future divergence is a single-point substitution.

package fingerprint

import (
	utls "github.com/refraction-networking/utls"
)

// osKey identifies the impersonated operating system.
type osKey int

const (
	osWindows osKey = iota
	osMacOS
	osLinux
	osAndroid
	osIOS
)

// isMobile reports whether the OS is a mobile form factor (Android or iOS),
// which selects the mobile profile variant.
func (k osKey) isMobile() bool { return k == osAndroid || k == osIOS }

// chUAMobile returns the sec-ch-ua-mobile header value for this OS: "?1" for
// mobile form factors, "?0" otherwise.
func (k osKey) chUAMobile() string {
	if k.isMobile() {
		return "?1"
	}
	return "?0"
}

// headerField is one entry of a profile's ordered header set. Names are
// lowercase, matching the header-order tables and the wire-order magic keys.
type headerField struct {
	Name  string
	Value string
}

// h2Config is a profile's HTTP/2 fingerprint data. Settings is the exact
// SETTINGS-frame emission order (frame order is wire-visible and part of the
// Akamai fingerprint); the remaining fields map onto the fork transport's
// exported knobs. Zero-valued StreamID/ConnectionFlow mean "leave default";
// hasPriority gates PriorityParam (its zero value is a meaningful setting).
type h2Config struct {
	settings       []h2Setting
	streamID       uint32
	connectionFlow uint32
	priority       priorityParam
	hasPriority    bool
}

// h2Setting is one SETTINGS entry: a setting ID and its value.
type h2Setting struct {
	id  uint16
	val uint32
}

// HTTP/2 SETTINGS identifiers (RFC 7540 §6.5.2), used in profile data.
// (0x3 SETTINGS_MAX_CONCURRENT_STREAMS has no constant here: no shipped
// profile emits it.)
const (
	settingHeaderTableSize   uint16 = 0x1
	settingEnablePush        uint16 = 0x2
	settingInitialWindowSize uint16 = 0x4
	settingMaxFrameSize      uint16 = 0x5
	settingMaxHeaderListSize uint16 = 0x6
)

// priorityParam mirrors http2.PriorityParam (declared here to keep profile
// data free of transport imports; converted at transport construction).
type priorityParam struct {
	streamDep uint32
	exclusive bool
	weight    uint8
}

// browserProfile describes one browser version and form factor.
type browserProfile struct {
	// name is the human-readable profile identity ("chrome-152", "firefox-148").
	name string

	// helloSpec takes precedence over helloID when non-nil.
	helloSpec *utls.ClientHelloSpec
	helloID   utls.ClientHelloID

	// shuffleExtensions re-shuffles the spec's extension order on every
	// connection, as Chromium does since v110. Firefox keeps a fixed order.
	shuffleExtensions bool

	// ua maps every supported OS to this browser's User-Agent string.
	ua map[osKey]string

	// platform maps every OS to its sec-ch-ua-platform value. nil means the
	// browser does not emit Client Hints headers at all (Firefox).
	platform map[osKey]string

	// secCHUA is the static sec-ch-ua brand-list value; "" disables it.
	secCHUA string

	// orderGet/orderPost are the per-method wire order of the full header
	// block, pseudo-headers first. Methods other than GET/POST fall back to
	// orderGet (the upstream profiles only differentiate GET and POST).
	orderGet  []string
	orderPost []string

	// baseHeaders builds the static per-request header set for one OS:
	// Accept-Encoding/Accept-Language values, OS-resolved UA / sec-ch-ua
	// entries, and empty placeholders (authorization/cookie/origin/referer,
	// content-type/content-length on POST) that occupy order slots but are
	// dropped when still empty at request time. It receives the profile so
	// UA/platform/sec-ch-ua values are read from the profile's own ua/
	// platform/secCHUA fields — a single source of truth, not a second copy
	// of the same tables.
	baseHeaders func(p *browserProfile, os osKey) []headerField

	// insertGet/insertPost are the method-specific header additions applied
	// with Set semantics before ordering (Accept, Priority, sec-fetch-*, …).
	insertGet  []headerField
	insertPost []headerField

	// h2 is the HTTP/2 SETTINGS fingerprint.
	h2 h2Config
}

// order returns the wire-order list for a request method: POST uses its own
// table, every other method falls back to the GET table.
func (p *browserProfile) order(method string) []string {
	if method == "POST" {
		return p.orderPost
	}
	return p.orderGet
}

// inserts returns the method-specific header additions: POST uses its own
// set, every other method gets the GET set.
func (p *browserProfile) inserts(method string) []headerField {
	if method == "POST" {
		return p.insertPost
	}
	return p.insertGet
}

// mergedHeaders builds the profile's full header map for one request: the
// static base set plus the method-specific inserts, later entries winning
// name collisions (Set semantics).
func (p *browserProfile) mergedHeaders(os osKey, method string) map[string]string {
	merged := make(map[string]string, len(p.orderGet))
	for _, f := range p.baseHeaders(p, os) {
		merged[f.Name] = f.Value
	}
	for _, f := range p.inserts(method) {
		merged[f.Name] = f.Value
	}
	return merged
}

// profileFor resolves a preset registry entry to its concrete profile
// variant (desktop or mobile by OS) and OS key.
func profileFor(spec profileSpec) (*browserProfile, osKey) {
	os := osFor(spec.os)

	var p *browserProfile
	switch spec.browser {
	case "firefox":
		if os.isMobile() {
			p = firefox148Mobile
		} else {
			p = firefox148Desktop
		}
	default: // "chrome"
		if os.isMobile() {
			p = chrome152Mobile
		} else {
			p = chrome152Desktop
		}
	}
	return p, os
}

// osFor maps a preset registry OS string to its key; the empty string (bare
// presets like "chrome") means the Windows desktop default.
func osFor(v string) osKey {
	switch v {
	case "macos":
		return osMacOS
	case "linux":
		return osLinux
	case "android":
		return osAndroid
	case "ios":
		return osIOS
	default: // "", "windows"
		return osWindows
	}
}
