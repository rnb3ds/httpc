// Firefox profile data, ported verbatim from
// github.com/enetx/surf (profiles/firefox), Copyright enetx/surf authors,
// MIT License. The TLS fingerprint uses utls's built-in HelloFirefox_148
// (constructed fresh per dial), so no local spec data is needed.
//
// The shipped label is Firefox 158 (stable since 2026-10-13); the ClientHello
// of the 149—58 line is, per every available signal, unchanged from 148's —// no newer parrot exists in the uTLS/tls-client ecosystem, and the 148
// fingerprint (t13d1715h2_5b57614c22b0_a54fffd0eb61) is still attributed to
// Firefox in live October-2026 traffic. Both profiles therefore ride
// HelloFirefox_148; the network gates pin the exact JA4 and will fire if
// that ever stops being true. Firefox 148 is kept as an explicit version pin.
//
// Android data is a verbatim copy of desktop (Firefox's Gecko TLS stack is
// shared across form factors); iOS uses the WebKit profile every iOS browser
// must use.

package fingerprint

import (
	utls "github.com/bogdanfinn/utls"
)

// FirefoxMajor is the shipped Firefox major —the label the UA map claims.
// The drift gates compare it against the current stable major.
const FirefoxMajor = 158

// firefox148UA maps every supported impersonated OS to its Firefox 148
// User-Agent string. iOS Firefox uses the FxiOS variant (under the hood it
// is WebKit).
var firefox148UA = map[osKey]string{
	osWindows: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:148.0) Gecko/20100101 Firefox/148.0",
	osMacOS:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0",
	osLinux:   "Mozilla/5.0 (X11; Linux x86_64; rv:148.0) Gecko/20100101 Firefox/148.0",
	osAndroid: "Mozilla/5.0 (Android 16; Mobile; rv:148.0) Gecko/148.0 Firefox/148.0",
	osIOS:     "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/148.0 Mobile/15E148 Safari/605.1.15",
}

// firefox158UA maps every supported impersonated OS to its Firefox 158
// User-Agent string. Desktop and Android entries are real 158 captures; iOS
// composes the observed FxiOS-on-iOS-26 pattern (third-party iOS browsers
// report the real OS version, unlike Safari's frozen UA) with the FxiOS/158.1
// build, on the iOS 26.3 whose WebKit the hello claims.
var firefox158UA = map[osKey]string{
	osWindows: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:158.0) Gecko/20100101 Firefox/158.0",
	osMacOS:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:158.0) Gecko/20100101 Firefox/158.0",
	osLinux:   "Mozilla/5.0 (X11; Linux x86_64; rv:158.0) Gecko/20100101 Firefox/158.0",
	osAndroid: "Mozilla/5.0 (Android 17; Mobile; rv:158.0) Gecko/158.0 Firefox/158.0",
	osIOS:     "Mozilla/5.0 (iPhone; CPU iPhone OS 26_3 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/158.1 Mobile/15E148 Safari/604.1",
}

// firefoxOrderGet is the Firefox 148 header-block wire order for GET (and
// every non-POST method). Note the Firefox pseudo-header order differs from
// Chrome's (:path before :authority).
var firefoxOrderGet = []string{
	":method",
	"host",
	":path",
	":authority",
	":scheme",
	"user-agent",
	"accept",
	"accept-language",
	"accept-encoding",
	"referer",
	"authorization",
	"cookie",
	"upgrade-insecure-requests",
	"sec-fetch-dest",
	"sec-fetch-mode",
	"sec-fetch-site",
	"sec-fetch-user",
	"priority",
}

// firefoxOrderPost is the Firefox 148 header-block wire order for POST.
var firefoxOrderPost = []string{
	":method",
	"host",
	":path",
	":authority",
	":scheme",
	"user-agent",
	"accept",
	"accept-language",
	"accept-encoding",
	"referer",
	"content-type",
	"authorization",
	"content-length",
	"origin",
	"cookie",
	"sec-fetch-dest",
	"sec-fetch-mode",
	"sec-fetch-site",
	"priority",
	"pragma",
	"cache-control",
}

// firefoxBaseHeaders constructs the static Firefox request header set for
// one OS, shared by the 148 and 158 profiles. Firefox does not emit Client
// Hints UA-CH headers (sec-ch-ua / sec-ch-ua-mobile / sec-ch-ua-platform).
//
// Accept-Language carries q=0.9: Firefox 147 (January 2026) changed the
// second language's quality value from the decades-old 0.5 to 0.9 to match
// other browsers, and both shipped majors postdate that change (the surf
// capture's 0.5 was pre-147 behavior mislabeled as 148).
func firefoxBaseHeaders(p *browserProfile, os osKey) []headerField {
	return []headerField{
		{Name: ":authority", Value: ""},
		{Name: ":method", Value: ""},
		// Host is written by the transport, not read from this map; the
		// empty placeholder exists so the name reaches the order list and
		// the browser's first-header position is preserved on HTTP/1.1.
		{Name: "Host", Value: ""},
		{Name: ":path", Value: ""},
		{Name: ":scheme", Value: ""},
		{Name: "Accept-Encoding", Value: "gzip, deflate, br, zstd"},
		{Name: "Accept-Language", Value: "en-US,en;q=0.9"},
		{Name: "Authorization", Value: ""},
		{Name: "Cookie", Value: ""},
		{Name: "Origin", Value: ""},
		{Name: "Referer", Value: ""},
		{Name: "User-Agent", Value: p.ua[os]},
	}
}

// firefoxInsertGet adds the navigation (GET) header values.
var firefoxInsertGet = []headerField{
	{Name: "Accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
	{Name: "Priority", Value: "u=0, i"},
	{Name: "Sec-Fetch-Dest", Value: "document"},
	{Name: "Sec-Fetch-Mode", Value: "navigate"},
	{Name: "Sec-Fetch-Site", Value: "none"},
	{Name: "Sec-Fetch-User", Value: "?1"},
	{Name: "Upgrade-Insecure-Requests", Value: "1"},
}

// firefox148InsertPost adds the XHR/fetch (POST) header values for the
// retired 148 pin, frozen as captured. content-type and content-length stay
// empty placeholders filled by the actual request.
var firefox148InsertPost = fetchStyleInsertPost("u=1, i")

// firefox158InsertPost adds the fetch/XHR (POST) header values for Firefox
// 158. Real 158 captures send Priority "u=4" on fetch()-style requests (and
// none on XHR); the old "u=1, i" shape was Chrome's, not Firefox's.
var firefox158InsertPost = fetchStyleInsertPost("u=4")

// firefox148H2 is the desktop Firefox HTTP/2 SETTINGS chain, in exact
// emission order (SETTINGS_ENABLE_PUSH:0 emitted; no MAX_HEADER_LIST_SIZE
// entry). Firefox starts streams at id 3 and uses connection flow 12517377
// with a non-exclusive weight-41 root priority. Shared by the 148 and 158
// profiles: Firefox's H2 framing has not moved between them.
var firefox148H2 = h2Config{
	settings: []h2Setting{
		{id: settingHeaderTableSize, val: 65536},
		{id: settingEnablePush, val: 0},
		{id: settingInitialWindowSize, val: 131072},
		{id: settingMaxFrameSize, val: 16384},
	},
	streamID:       3,
	connectionFlow: 12517377,
	priority:       priorityParam{exclusive: false, weight: 41},
	hasPriority:    true,
}

// firefoxH2Mobile is a placeholder mobile SETTINGS chain (desktop copy);
// replace on observation of real Firefox Android H/2 settings.
var firefoxH2Mobile = firefox148H2

// firefox148Desktop is the Firefox 148 desktop profile —the retired major,
// served under the explicit "firefox-148" version pin.
var firefox148Desktop = &browserProfile{
	name:        "firefox-148",
	hello:       helloFromID(utls.HelloFirefox_148),
	ua:          firefox148UA,
	platform:    nil, // Firefox emits no Client Hints headers
	secCHUA:     "",
	orderGet:    firefoxOrderGet,
	orderPost:   firefoxOrderPost,
	baseHeaders: firefoxBaseHeaders,
	insertGet:   firefoxInsertGet,
	insertPost:  firefox148InsertPost,
	h2:          firefox148H2,
}

// firefox148Mobile is the placeholder Firefox 148 mobile profile (desktop
// data; each Mobile variable above is its single point of substitution).
var firefox148Mobile = &browserProfile{
	name:        "firefox-148-mobile",
	hello:       helloFromID(utls.HelloFirefox_148),
	ua:          firefox148UA,
	platform:    nil,
	secCHUA:     "",
	orderGet:    firefoxOrderGet,
	orderPost:   firefoxOrderPost,
	baseHeaders: firefoxBaseHeaders,
	insertGet:   firefoxInsertGet,
	insertPost:  firefox148InsertPost,
	h2:          firefoxH2Mobile,
}

// firefox158Desktop is the Firefox 158 desktop profile —the current
// "firefox" / "firefox-latest" label. Rides the HelloFirefox_148 parrot; see
// the file comment for why that is believed byte-identical for 158.
var firefox158Desktop = &browserProfile{
	name:        "firefox-158",
	hello:       helloFromID(utls.HelloFirefox_148),
	ua:          firefox158UA,
	platform:    nil,
	secCHUA:     "",
	orderGet:    firefoxOrderGet,
	orderPost:   firefoxOrderPost,
	baseHeaders: firefoxBaseHeaders,
	insertGet:   firefoxInsertGet,
	insertPost:  firefox158InsertPost,
	h2:          firefox148H2,
}

// firefox158Android is the Firefox 158 Android profile: desktop TLS and H2
// bytes with the mobile UA. Firefox's Gecko TLS stack is shared across form
// factors, so unlike Chrome the desktop copy is believed byte-accurate; the
// placeholder marker remains until a device capture confirms it.
var firefox158Android = &browserProfile{
	name:        "firefox-158-android",
	hello:       helloFromID(utls.HelloFirefox_148),
	ua:          firefox158UA,
	platform:    nil,
	secCHUA:     "",
	orderGet:    firefoxOrderGet,
	orderPost:   firefoxOrderPost,
	baseHeaders: firefoxBaseHeaders,
	insertGet:   firefoxInsertGet,
	insertPost:  firefox158InsertPost,
	h2:          firefoxH2Mobile,
}

// firefox158IOS is the Firefox 158 iOS profile: FxiOS is WebKit underneath
// (Apple mandate), so the TLS and HTTP/2 layers are Safari's —the same
// HelloSafari_26_3 hello and safariIOSH2 chain as chrome-155-ios. The
// regular-header order remains the desktop approximation (see PROFILES.md).
var firefox158IOS = &browserProfile{
	name:              "firefox-158-ios",
	hello:             helloFromID(utls.HelloSafari_26_3),
	ua:                firefox158UA,
	platform:          nil,
	secCHUA:           "",
	shuffleExtensions: false, // iOS WebKit keeps a fixed extension order
	orderGet:          safariOrderGet,
	orderPost:         safariOrderPost,
	baseHeaders:       firefoxBaseHeaders,
	insertGet:         firefoxInsertGet,
	insertPost:        firefox158InsertPost,
	h2:                safariIOSH2,
}
