// Safari iOS profile. iOS Safari is the third browser family: its TLS and
// HTTP/2 layers are the same WebKit shapes the chrome-ios/firefox-ios
// profiles already use (utls HelloSafari_26_3 — offline JA4
// t13d2013h2_a09f3c656075_7f0f34a4126d, live-matched — plus the Safari iOS
// 26 H2 chain), so this profile is data-complete at the transport layers.
//
// Header values come from real Safari 26 captures (navigation and fetch/XHR
// shapes); the wire ORDER of the regular headers remains the shared
// approximation. Safari emits no Client Hints and no
// GREASE: a fixed, quieter header block than the Chromium family.
//
// There is deliberately no desktop-safari preset: macOS Safari's ClientHello
// carries one more extension than iOS's (observed live as t13d2014h2_* vs
// iOS t13d2013h2_*), no parrot for it exists, and a silently-wrong desktop
// shape would be worse than the preset's absence.

package fingerprint

import (
	utls "github.com/bogdanfinn/utls"
)

// safariUA maps the impersonated OS to Safari's User-Agent. Apple freezes
// the OS token in Safari's UA (unlike third-party iOS browsers): Safari 26
// on iOS reports iPhone OS 18_6 regardless of the installed iOS version,
// with the real Safari major in the Version/ token.
var safariUA = map[osKey]string{
	osIOS: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Mobile/15E148 Safari/604.1",
}

// safariBaseHeaders constructs the static Safari request header set: no
// Client Hints family at all, and Safari's shorter Accept-Encoding (no
// brotli/zstd advertisement).
func safariBaseHeaders(p *browserProfile, os osKey) []headerField {
	return []headerField{
		{Name: ":authority", Value: ""},
		{Name: ":method", Value: ""},
		{Name: "Host", Value: ""},
		{Name: ":path", Value: ""},
		{Name: ":scheme", Value: ""},
		{Name: "Accept-Encoding", Value: "gzip, deflate"},
		{Name: "Accept-Language", Value: "en-US,en;q=0.9"},
		{Name: "Authorization", Value: ""},
		{Name: "Cookie", Value: ""},
		{Name: "Origin", Value: ""},
		{Name: "Referer", Value: ""},
		{Name: "User-Agent", Value: p.ua[os]},
	}
}

// safariInsertGet adds the navigation (GET) header values (Safari 26
// captures: the plain Accept list, priority u=0, i).
var safariInsertGet = []headerField{
	{Name: "Accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
	{Name: "Priority", Value: "u=0, i"},
	{Name: "Sec-Fetch-Dest", Value: "document"},
	{Name: "Sec-Fetch-Mode", Value: "navigate"},
	{Name: "Sec-Fetch-Site", Value: "none"},
	{Name: "Sec-Fetch-User", Value: "?1"},
	{Name: "Upgrade-Insecure-Requests", Value: "1"},
}

// safariInsertPost adds the fetch/XHR (POST) header values (Safari 26
// captures send priority u=3, i on both fetch and XHR requests).
var safariInsertPost = fetchStyleInsertPost("u=3, i")

// safariOrderGet is the iOS WebKit header-block wire order for GET (and
// every non-POST method): the pseudo-header section follows WebKit's
// :method,:scheme,:authority,:path order (the per-request order table drives
// the wire pseudo order —the h2 pseudoOrder field alone does not), while
// the regular-header section remains the desktop approximation pending real
// CriOS/FxiOS captures.
var safariOrderGet = []string{
	":method",
	":scheme",
	":authority",
	"host",
	":path",
	"sec-ch-ua",
	"sec-ch-ua-mobile",
	"sec-ch-ua-platform",
	"authorization",
	"upgrade-insecure-requests",
	"user-agent",
	"accept",
	"sec-fetch-site",
	"sec-fetch-mode",
	"sec-fetch-user",
	"sec-fetch-dest",
	"referer",
	"accept-encoding",
	"accept-language",
	"cookie",
	"priority",
}

// safariOrderPost is the iOS WebKit POST order (pseudo section as above).
var safariOrderPost = []string{
	":method",
	":scheme",
	":authority",
	"host",
	":path",
	"content-length",
	"pragma",
	"cache-control",
	"sec-ch-ua-platform",
	"authorization",
	"user-agent",
	"sec-ch-ua",
	"content-type",
	"sec-ch-ua-mobile",
	"accept",
	"origin",
	"sec-fetch-site",
	"sec-fetch-mode",
	"sec-fetch-dest",
	"referer",
	"accept-encoding",
	"accept-language",
	"cookie",
	"priority",
}

// safariIOSH2 is the iOS WebKit HTTP/2 fingerprint (Safari 26 / iOS 26
// captures, cross-checked against tls-client's Safari_IOS_26_0 profile,
// BSD-3): no HEADER_TABLE_SIZE or MAX_FRAME_SIZE entries (defaults), streams
// capped at 100, a 2 MiB initial window, the RFC 9218 no-priorities setting
// and a 10420225 connection flow. No PRIORITY frames and no header priority:
// the no-RFC7540-priorities setting replaces the priority tree. The m,s,a,p
// pseudo order here documents the browser's shape; the wire order comes from
// the per-request safariOrder* tables (PHeaderOrderKey takes precedence).
var safariIOSH2 = h2Config{
	settings: []h2Setting{
		{id: settingEnablePush, val: 0},
		{id: settingMaxConcurrentStreams, val: 100},
		{id: settingInitialWindowSize, val: 2097152},
		{id: settingNoRFC7540Priorities, val: 1},
	},
	connectionFlow: 10420225,
	pseudoOrder:    []string{":method", ":scheme", ":authority", ":path"},
}

// safariIOS is the Safari iOS profile: verified WebKit TLS + H2 data, real
// Safari 26 header values, no Client Hints, no extension shuffle.
var safariIOS = &browserProfile{
	name:              "safari-ios",
	hello:             helloFromID(utls.HelloSafari_26_3),
	ua:                safariUA,
	platform:          nil, // Safari emits no Client Hints headers
	secCHUA:           "",
	shuffleExtensions: false, // WebKit keeps a fixed extension order
	orderGet:          safariOrderGet,
	orderPost:         safariOrderPost,
	baseHeaders:       safariBaseHeaders,
	insertGet:         safariInsertGet,
	insertPost:        safariInsertPost,
	h2:                safariIOSH2,
}
