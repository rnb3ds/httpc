// Browser profile model: one browserProfile value describes everything the
// transport needs to reproduce one browser version's wire behavior — the TLS
// ClientHello spec, per-OS User-Agent and sec-ch-ua-platform data, the
// per-method ordered header block with its exact wire casing, the static
// header set with its empty placeholders, and the exact HTTP/2 SETTINGS
// sequence.
//
// Profile data ported from github.com/enetx/surf (profiles/), Copyright
// enetx/surf authors, MIT License. Mobile variants are today verbatim copies
// of the desktop data (as they are upstream); each keeps its own variables so
// a future divergence is a single-point substitution.
//
// The profile owns its TLS shape: httpc builds the ClientHelloSpec itself and
// hands it to the transport through a per-handshake SpecFactory, so a profile
// can target a browser build ahead of any upstream library's release.

package fingerprint

import (
	"strings"

	"github.com/bogdanfinn/fhttp/http2"
	"github.com/bogdanfinn/tls-client/profiles"
	utls "github.com/bogdanfinn/utls"
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

// headerField is one entry of a profile's header set. Name is the exact wire
// casing the impersonated browser uses — lower case for the Chromium
// sec-ch-ua family, Title-Case for everything else — because HTTP/1.1 header
// casing is a fingerprint axis the transport writes verbatim.
type headerField struct {
	Name  string
	Value string
}

// h2Config is a profile's HTTP/2 fingerprint data. SettingsOrder is the exact
// SETTINGS-frame emission order (frame order is wire-visible and part of the
// Akamai fingerprint); pseudoOrder is the pseudo-header wire order (Chrome
// m,a,s,p; Firefox and iOS WebKit differ); the remaining fields map onto the
// transport's exported knobs. Zero-valued StreamID/ConnectionFlow mean
// "leave default"; hasPriority gates PriorityParam (its zero value is a
// meaningful setting); an empty pseudoOrder means the Chrome order.
type h2Config struct {
	settings       []h2Setting
	streamID       uint32
	connectionFlow uint32
	priority       priorityParam
	hasPriority    bool
	pseudoOrder    []string
}

// h2Setting is one SETTINGS entry: a setting ID and its value.
type h2Setting struct {
	id  uint16
	val uint32
}

// HTTP/2 SETTINGS identifiers (RFC 7540 §6.5.2; RFC 9218 §4.2), used in
// profile data.
const (
	settingHeaderTableSize      uint16 = 0x1
	settingEnablePush           uint16 = 0x2
	settingMaxConcurrentStreams uint16 = 0x3
	settingInitialWindowSize    uint16 = 0x4
	settingMaxFrameSize         uint16 = 0x5
	settingMaxHeaderListSize    uint16 = 0x6
	settingNoRFC7540Priorities  uint16 = 0x9
)

// priorityParam mirrors http2.PriorityParam (declared here to keep profile
// data free of transport imports; converted at profile construction).
type priorityParam struct {
	streamDep uint32
	exclusive bool
	weight    uint8
}

// browserProfile describes one browser version and form factor.
type browserProfile struct {
	// name is the human-readable profile identity ("chrome-155", "firefox-148").
	name string

	// hello builds a fresh ClientHelloSpec for one handshake. It must return
	// a value that shares no mutable memory with the profile: specForDial
	// mutates what it receives (extension shuffle, GREASE signature-algorithm
	// substitution), and every connection gets its own call.
	hello func() (utls.ClientHelloSpec, error)

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
	// block, pseudo-headers first. Names are lower case (the order lookup is
	// case-insensitive); the wire casing comes from headerField.Name.
	// Methods other than GET/POST fall back to orderGet (the upstream
	// profiles only differentiate GET and POST).
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

// mergedHeaders builds the profile's full header set for one request: the
// static base set plus the method-specific inserts, later entries winning
// name collisions (case-insensitively, so a base placeholder and an insert
// entry naming the same field collapse into one).
func (p *browserProfile) mergedHeaders(os osKey, method string) []headerField {
	merged := make([]headerField, 0, len(p.baseHeaders(p, os))+len(p.inserts(method)))
	index := make(map[string]int, cap(merged))

	put := func(f headerField) {
		key := strings.ToLower(f.Name)
		if i, ok := index[key]; ok {
			merged[i] = f
			return
		}
		index[key] = len(merged)
		merged = append(merged, f)
	}

	for _, f := range p.baseHeaders(p, os) {
		put(f)
	}
	for _, f := range p.inserts(method) {
		put(f)
	}
	return merged
}

// clientProfile assembles the transport-facing profile for one backend: the
// per-handshake ClientHello factory plus the HTTP/2 frame fingerprint. A fresh
// value is built per backend so the SETTINGS map is never shared between
// independent connection pools.
func (p *browserProfile) clientProfile() profiles.ClientProfile {
	settings := make(map[http2.SettingID]uint32, len(p.h2.settings))
	order := make([]http2.SettingID, 0, len(p.h2.settings))
	for _, s := range p.h2.settings {
		id := http2.SettingID(s.id)
		settings[id] = s.val
		order = append(order, id)
	}

	var headerPriority *http2.PriorityParam
	if p.h2.hasPriority {
		prio := p.h2.priority
		headerPriority = &http2.PriorityParam{
			StreamDep: prio.streamDep,
			Exclusive: prio.exclusive,
			Weight:    prio.weight,
		}
	}

	pseudoOrder := p.h2.pseudoOrder
	if len(pseudoOrder) == 0 {
		pseudoOrder = []string{":method", ":authority", ":scheme", ":path"}
	}
	// Note: for requests that run the profile header pipeline (every adapter
	// request), the per-request PHeaderOrderKey takes precedence on the wire —
	// this value is the fallback for pipeline-less requests handed straight to
	// the backend, and documents the browser's own pseudo order.

	return profiles.NewClientProfile(
		utls.ClientHelloID{
			Client:  p.name,
			Version: p.name,
			// The factory is specForDial, not p.hello: it is where the
			// per-connection mutations live, and the transport calls it once
			// per handshake. RandomExtensionOrder stays false because the
			// shuffle here follows Chromium's rules (GREASE/padding/PSK
			// positionally fixed); the library's own shuffle does not.
			SpecFactory: func() (utls.ClientHelloSpec, error) { return specForDial(p) },
		},
		settings,
		order,
		pseudoOrder,
		p.h2.connectionFlow,
		nil, // priorities: PRIORITY frames are deprecated (RFC 9113 §5.3)
		headerPriority,
		p.h2.streamID,
		false,                   // allowHTTP
		nil, nil, 0, nil, false, // HTTP/3 is never attempted; see adapter.go
	)
}

// fetchStyleInsertPost builds the fetch/XHR (POST) header block shared by the
// current browser captures; only the Priority value differs between profiles
// (the drift dimension that previously lived in four byte-identical tables).
// content-type and content-length stay empty placeholders filled by the
// actual request.
func fetchStyleInsertPost(priority string) []headerField {
	return []headerField{
		{Name: "Accept", Value: "*/*"},
		{Name: "Cache-Control", Value: "no-cache"},
		{Name: "Content-Type", Value: ""},
		{Name: "Content-Length", Value: ""},
		{Name: "Pragma", Value: "no-cache"},
		{Name: "Priority", Value: priority},
		{Name: "Sec-Fetch-Dest", Value: "empty"},
		{Name: "Sec-Fetch-Mode", Value: "cors"},
		{Name: "Sec-Fetch-Site", Value: "same-origin"},
	}
}

// profileFor resolves a preset registry entry to its concrete profile
// variant and OS key. iOS resolves to the WebKit-based profiles: every iOS
// browser (CriOS, FxiOS) must use Apple's network stack, so their TLS and
// HTTP/2 shapes are Safari's, not their desktop siblings'.
func profileFor(spec profileSpec) (*browserProfile, osKey) {
	os := osFor(spec.os)

	var p *browserProfile
	switch spec.browser {
	case "firefox148":
		// No mobile presets exist for the retired pins (the registry maps
		// them with os ""), so the mobile branch is unreachable today; kept
		// for symmetry with the current majors.
		if os.isMobile() {
			p = firefox148Mobile
		} else {
			p = firefox148Desktop
		}
	case "firefox":
		switch os {
		case osIOS:
			p = firefox158IOS
		case osAndroid:
			p = firefox158Android
		default:
			p = firefox158Desktop
		}
	case "chrome152":
		// As above: no mobile presets exist for the retired pin.
		if os.isMobile() {
			p = chrome152Mobile
		} else {
			p = chrome152Desktop
		}
	case "safari":
		p = safariIOS
	default: // "chrome" — the current shipped major (155)
		switch os {
		case osIOS:
			p = chrome155IOS
		case osAndroid:
			p = chrome155Android
		default:
			p = chrome155Desktop
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
