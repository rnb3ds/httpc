// Firefox 148 profile data, ported verbatim from
// github.com/enetx/surf (profiles/firefox), Copyright enetx/surf authors,
// MIT License. The TLS fingerprint uses utls's built-in HelloFirefox_148
// (constructed fresh per dial), so no local spec data is needed.
//
// Mobile data is a verbatim copy of desktop (as upstream); each Mobile
// variable is the single point of substitution when real Firefox Android 148
// bytes are observed.

package fingerprint

import (
	utls "github.com/refraction-networking/utls"
)

// firefoxUA maps every supported impersonated OS to its Firefox 148
// User-Agent string. iOS Firefox uses the FxiOS variant (under the hood it
// is WebKit).
var firefoxUA = map[osKey]string{
	osWindows: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:148.0) Gecko/20100101 Firefox/148.0",
	osMacOS:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:148.0) Gecko/20100101 Firefox/148.0",
	osLinux:   "Mozilla/5.0 (X11; Linux x86_64; rv:148.0) Gecko/20100101 Firefox/148.0",
	osAndroid: "Mozilla/5.0 (Android 16; Mobile; rv:148.0) Gecko/148.0 Firefox/148.0",
	osIOS:     "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/148.0 Mobile/15E148 Safari/605.1.15",
}

// firefoxOrderGet is the Firefox 148 header-block wire order for GET (and
// every non-POST method). Note the Firefox pseudo-header order differs from
// Chrome's (:path before :authority).
var firefoxOrderGet = []string{
	":method",
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

// firefoxBaseHeaders constructs the static Firefox 148 request header set
// for one OS. Firefox does not emit Client Hints UA-CH headers (sec-ch-ua /
// sec-ch-ua-mobile / sec-ch-ua-platform).
func firefoxBaseHeaders(p *browserProfile, os osKey) []headerField {
	return []headerField{
		{Name: ":authority", Value: ""},
		{Name: ":method", Value: ""},
		{Name: ":path", Value: ""},
		{Name: ":scheme", Value: ""},
		{Name: "accept-encoding", Value: "gzip, deflate, br, zstd"},
		{Name: "accept-language", Value: "en-US,en;q=0.5"},
		{Name: "authorization", Value: ""},
		{Name: "cookie", Value: ""},
		{Name: "origin", Value: ""},
		{Name: "referer", Value: ""},
		{Name: "user-agent", Value: p.ua[os]},
	}
}

// firefoxInsertGet adds the navigation (GET) header values.
var firefoxInsertGet = []headerField{
	{Name: "accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
	{Name: "priority", Value: "u=0, i"},
	{Name: "sec-fetch-dest", Value: "document"},
	{Name: "sec-fetch-mode", Value: "navigate"},
	{Name: "sec-fetch-site", Value: "none"},
	{Name: "sec-fetch-user", Value: "?1"},
	{Name: "upgrade-insecure-requests", Value: "1"},
}

// firefoxInsertPost adds the XHR/fetch (POST) header values; content-type
// and content-length stay empty placeholders filled by the actual request.
var firefoxInsertPost = []headerField{
	{Name: "accept", Value: "*/*"},
	{Name: "cache-control", Value: "no-cache"},
	{Name: "content-type", Value: ""},
	{Name: "content-length", Value: ""},
	{Name: "pragma", Value: "no-cache"},
	{Name: "priority", Value: "u=1, i"},
	{Name: "sec-fetch-dest", Value: "empty"},
	{Name: "sec-fetch-mode", Value: "cors"},
	{Name: "sec-fetch-site", Value: "same-origin"},
}

// firefox148H2 is the desktop Firefox 148 HTTP/2 SETTINGS chain, in exact
// emission order (SETTINGS_ENABLE_PUSH:0 emitted; no MAX_HEADER_LIST_SIZE
// entry). Firefox starts streams at id 3 and uses connection flow 12517377
// with a non-exclusive weight-41 root priority.
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

// firefox148H2Mobile is a placeholder mobile SETTINGS chain (desktop copy);
// replace on observation of real Firefox Android 148 H/2 settings.
var firefox148H2Mobile = firefox148H2

// firefox148Desktop is the Firefox 148 desktop profile.
var firefox148Desktop = &browserProfile{
	name:        "firefox-148",
	helloID:     utls.HelloFirefox_148,
	ua:          firefoxUA,
	platform:    nil, // Firefox emits no Client Hints headers
	secCHUA:     "",
	orderGet:    firefoxOrderGet,
	orderPost:   firefoxOrderPost,
	baseHeaders: firefoxBaseHeaders,
	insertGet:   firefoxInsertGet,
	insertPost:  firefoxInsertPost,
	h2:          firefox148H2,
}

// firefox148Mobile is the placeholder Firefox 148 mobile profile (desktop
// data; each Mobile variable above is its single point of substitution).
var firefox148Mobile = &browserProfile{
	name:        "firefox-148-mobile",
	helloID:     utls.HelloFirefox_148,
	ua:          firefoxUA,
	platform:    nil,
	secCHUA:     "",
	orderGet:    firefoxOrderGet,
	orderPost:   firefoxOrderPost,
	baseHeaders: firefoxBaseHeaders,
	insertGet:   firefoxInsertGet,
	insertPost:  firefoxInsertPost,
	h2:          firefox148H2Mobile,
}
