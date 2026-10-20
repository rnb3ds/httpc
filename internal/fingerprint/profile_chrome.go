// Chrome 152 profile data, ported verbatim from
// github.com/enetx/surf (profiles/chrome), Copyright enetx/surf authors, MIT
// License. The TLS spec is surf's own contribution — utls upstream has no
// Chrome 152 hello — so it lives here and must track real Chrome releases.
//
// Mobile data is a verbatim copy of desktop (as upstream); each Mobile
// variable is the single point of substitution when real Chrome Android 152
// bytes are observed.

package fingerprint

import (
	utls "github.com/refraction-networking/utls"
)

// ML-DSA (post-quantum) TLS 1.3 signature schemes that Chrome 152 advertises
// at the head of its signature_algorithms list. utls does not yet name them,
// so they are spelled as raw SignatureScheme code points (per the TLS ML-DSA
// draft). Without them a Chrome-152 UA ships a pre-152 JA4, which some Akamai
// deployments reject.
const (
	MLDSA44 utls.SignatureScheme = 0x0904
	MLDSA65 utls.SignatureScheme = 0x0905
	MLDSA87 utls.SignatureScheme = 0x0906
)

// extensionTrustAnchors is the TLS Trust Anchor Identifiers extension
// (draft-ietf-tls-trust-anchor-ids, code point 0xca34). Chrome 152 sends it
// on every ClientHello, so its presence is part of the Chrome 152 JA4
// (t13d1517h2_…); a hello without it is a pre-152 fingerprint, which some
// Akamai deployments reject when the UA claims Chrome 152.
const extensionTrustAnchors uint16 = 0xca34

// chrome152TrustAnchorIDs is the extension body Chrome 152 sends: a
// length-prefixed TrustAnchorIdentifierList of the Chrome Root Store trust
// anchor IDs it advertises, captured verbatim from a Google Chrome
// 152.0.7977 desktop ClientHello.
//
// The order matters and is not randomised. Chromium walks the compiled-in
// kChromeRootCertList in array order (net/cert/internal/trust_store_chrome.cc),
// passes the bytes to SSL_set1_requested_trust_anchors untouched, and
// BoringSSL writes them into the extension verbatim — nothing shuffles the
// list the way the extension order is shuffled. So the order is a
// compile-time constant of one Chrome build: stable across connections and
// hosts, but two builds carrying different root store snapshots emit
// different orders for the same set of IDs. These bytes must therefore come
// from a Google Chrome release, matching the branding the profile claims in
// sec-ch-ua; a Chromium or ungoogled-chromium build of the same major version
// is not interchangeable here.
var chrome152TrustAnchorIDs = []byte{
	0x00, 0xcc,
	0x04, 0xd6, 0x79, 0x09, 0x06,
	0x08, 0x83, 0x9a, 0x64, 0x8c, 0x9b, 0x2d, 0x01, 0x07,
	0x04, 0xd6, 0x79, 0x09, 0x0c,
	0x05, 0x82, 0xdf, 0x13, 0x02, 0x06,
	0x05, 0x82, 0xdf, 0x13, 0x02, 0x13,
	0x08, 0x83, 0x9a, 0x64, 0x8c, 0x9b, 0x2d, 0x01, 0x0d,
	0x04, 0xd6, 0x79, 0x09, 0x01,
	0x05, 0x82, 0xdf, 0x13, 0x02, 0x0d,
	0x04, 0xd6, 0x79, 0x09, 0x0d,
	0x05, 0x82, 0xdf, 0x13, 0x02, 0x0f,
	0x08, 0x83, 0x9a, 0x64, 0x8c, 0x9b, 0x2d, 0x01, 0x08,
	0x05, 0x82, 0xdf, 0x13, 0x02, 0x12,
	0x08, 0x83, 0x9a, 0x64, 0x8c, 0x9b, 0x2d, 0x01, 0x09,
	0x04, 0xd6, 0x79, 0x09, 0x02,
	0x05, 0x82, 0xdf, 0x13, 0x02, 0x01,
	0x04, 0xd6, 0x79, 0x09, 0x0e,
	0x04, 0xd6, 0x79, 0x09, 0x09,
	0x08, 0x83, 0x9a, 0x64, 0x8c, 0x9b, 0x2d, 0x01, 0x0a,
	0x04, 0xd6, 0x79, 0x09, 0x03,
	0x04, 0xd6, 0x79, 0x09, 0x0f,
	0x08, 0x83, 0x9a, 0x64, 0x8c, 0x9b, 0x2d, 0x01, 0x0b,
	0x04, 0xd6, 0x79, 0x09, 0x04,
	0x05, 0x82, 0xdf, 0x13, 0x02, 0x14,
	0x04, 0xd6, 0x79, 0x09, 0x0a,
	0x08, 0x83, 0x9a, 0x64, 0x8c, 0x9b, 0x2d, 0x01, 0x13,
	0x08, 0x83, 0x9a, 0x64, 0x8c, 0x9b, 0x2d, 0x01, 0x12,
	0x04, 0xd6, 0x79, 0x09, 0x07,
	0x04, 0xd6, 0x79, 0x09, 0x08,
	0x08, 0x83, 0x9a, 0x64, 0x8c, 0x9b, 0x2d, 0x01, 0x0c,
	0x04, 0xd6, 0x79, 0x09, 0x05,
	0x05, 0x82, 0xdf, 0x13, 0x02, 0x0e,
	0x04, 0xd6, 0x79, 0x09, 0x0b,
}

// chrome152Spec mirrors HelloChrome_150 (ML-DSA signature schemes at the
// head of signature_algorithms) and adds what Chrome 152 sends on top: a
// GREASE signature scheme and the trust_anchors extension. Together with the
// per-connection extension shuffle this reproduces a Chrome 152 desktop
// ClientHello structurally, not just its JA4.
//
// The extension order below is Chrome's own order. It is never shuffled in
// place: the shuffle runs per connection on a private clone at dial time, so
// this value stays a stable reference point across runs.
var chrome152Spec = utls.ClientHelloSpec{
	CipherSuites: []uint16{
		utls.GREASE_PLACEHOLDER,
		utls.TLS_AES_128_GCM_SHA256,
		utls.TLS_AES_256_GCM_SHA384,
		utls.TLS_CHACHA20_POLY1305_SHA256,
		utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		utls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
		utls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
		utls.TLS_RSA_WITH_AES_128_GCM_SHA256,
		utls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		utls.TLS_RSA_WITH_AES_128_CBC_SHA,
		utls.TLS_RSA_WITH_AES_256_CBC_SHA,
	},
	CompressionMethods: []byte{0x00},
	Extensions: []utls.TLSExtension{
		&utls.UtlsGREASEExtension{},
		&utls.SNIExtension{},
		&utls.ExtendedMasterSecretExtension{},
		&utls.RenegotiationInfoExtension{
			Renegotiation: utls.RenegotiateOnceAsClient,
		},
		&utls.SupportedCurvesExtension{
			Curves: []utls.CurveID{
				utls.GREASE_PLACEHOLDER,
				utls.X25519MLKEM768,
				utls.X25519,
				utls.CurveP256,
				utls.CurveP384,
			},
		},
		&utls.SupportedPointsExtension{
			SupportedPoints: []byte{0x00},
		},
		&utls.SessionTicketExtension{},
		&utls.ALPNExtension{
			AlpnProtocols: []string{"h2", "http/1.1"},
		},
		&utls.StatusRequestExtension{},
		&utls.SignatureAlgorithmsExtension{
			SupportedSignatureAlgorithms: []utls.SignatureScheme{
				// Chrome 152 GREASEs signature_algorithms; the dial path
				// substitutes the placeholder with a random GREASE value per
				// connection (see greaseSignatureAlgorithms).
				utls.GREASE_PLACEHOLDER,
				MLDSA44,
				MLDSA65,
				MLDSA87,
				utls.ECDSAWithP256AndSHA256,
				utls.PSSWithSHA256,
				utls.PKCS1WithSHA256,
				utls.ECDSAWithP384AndSHA384,
				utls.PSSWithSHA384,
				utls.PKCS1WithSHA384,
				utls.PSSWithSHA512,
				utls.PKCS1WithSHA512,
			},
		},
		&utls.SCTExtension{},
		&utls.KeyShareExtension{
			KeyShares: []utls.KeyShare{
				{Group: utls.GREASE_PLACEHOLDER, Data: []byte{0}},
				{Group: utls.X25519MLKEM768},
				{Group: utls.X25519},
			},
		},
		&utls.PSKKeyExchangeModesExtension{
			Modes: []uint8{
				utls.PskModeDHE,
			},
		},
		&utls.SupportedVersionsExtension{
			Versions: []uint16{
				utls.GREASE_PLACEHOLDER,
				utls.VersionTLS13,
				utls.VersionTLS12,
			},
		},
		&utls.UtlsCompressCertExtension{
			Algorithms: []utls.CertCompressionAlgo{
				utls.CertCompressionBrotli,
			},
		},
		&utls.ApplicationSettingsExtensionNew{
			SupportedProtocols: []string{"h2"},
		},
		&utls.GenericExtension{Id: extensionTrustAnchors, Data: chrome152TrustAnchorIDs},
		utls.BoringGREASEECH(),
		&utls.UtlsGREASEExtension{},
		&utls.UtlsPreSharedKeyExtension{},
	},
}

// chrome152SpecMobile is a placeholder mobile variant. On the day real
// Chrome Android 152 ClientHello bytes are observed, replace this value — it
// is the single point of substitution for the mobile TLS fingerprint.
var chrome152SpecMobile = chrome152Spec

// chromeSecCHUA is the static sec-ch-ua brand list for Chrome 152.
const chromeSecCHUA = `"Chromium";v="152", "Not?A_Brand";v="24", "Google Chrome";v="152"`

// chromeUA maps every supported impersonated OS to its Chrome 152 User-Agent
// string. UA strings are an OS property, not a form-factor property (Chrome
// on Android always identifies as mobile, Chrome on Windows always as
// desktop).
var chromeUA = map[osKey]string{
	osWindows: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36",
	osMacOS:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36",
	osLinux:   "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36",
	osAndroid: "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Mobile Safari/537.36",
	osIOS:     "Mozilla/5.0 (iPad; CPU OS 26_3_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/152.0.7977.53 Mobile/15E148 Safari/604.1",
}

// chromePlatform maps every supported impersonated OS to its
// sec-ch-ua-platform header value.
var chromePlatform = map[osKey]string{
	osWindows: `"Windows"`,
	osMacOS:   `"macOS"`,
	osLinux:   `"Linux"`,
	osAndroid: `"Android"`,
	osIOS:     `"iOS"`,
}

// chromeOrderGet is the Chrome 152 header-block wire order for GET (and
// every non-POST method): pseudo-headers first, then the exact browser
// order. Header names are lowercase.
var chromeOrderGet = []string{
	":method",
	":authority",
	":scheme",
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

// chromeOrderPost is the Chrome 152 header-block wire order for POST.
var chromeOrderPost = []string{
	":method",
	":authority",
	":scheme",
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

// chromeBaseHeaders constructs the static Chrome 152 request header set for
// one OS: browser-matched Accept-Encoding/Accept-Language, OS-resolved UA
// and Client Hints entries, and the empty placeholders (authorization,
// cookie, origin, referer) that occupy order slots but are dropped while
// empty at request time.
func chromeBaseHeaders(p *browserProfile, os osKey) []headerField {
	return []headerField{
		{Name: ":authority", Value: ""},
		{Name: ":method", Value: ""},
		{Name: ":path", Value: ""},
		{Name: ":scheme", Value: ""},
		{Name: "accept-encoding", Value: "gzip, deflate, br, zstd"},
		{Name: "accept-language", Value: "en-US,en;q=0.9"},
		{Name: "authorization", Value: ""},
		{Name: "cookie", Value: ""},
		{Name: "origin", Value: ""},
		{Name: "referer", Value: ""},
		{Name: "sec-ch-ua", Value: p.secCHUA},
		{Name: "sec-ch-ua-mobile", Value: os.chUAMobile()},
		{Name: "sec-ch-ua-platform", Value: p.platform[os]},
		{Name: "user-agent", Value: p.ua[os]},
	}
}

// chromeInsertGet adds the navigation (GET) header values.
var chromeInsertGet = []headerField{
	{Name: "accept", Value: "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
	{Name: "priority", Value: "u=0, i"},
	{Name: "sec-fetch-dest", Value: "document"},
	{Name: "sec-fetch-mode", Value: "navigate"},
	{Name: "sec-fetch-site", Value: "none"},
	{Name: "sec-fetch-user", Value: "?1"},
	{Name: "upgrade-insecure-requests", Value: "1"},
}

// chromeInsertPost adds the XHR/fetch (POST) header values; content-type and
// content-length stay empty placeholders filled by the actual request.
var chromeInsertPost = []headerField{
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

// chrome152H2 is the desktop Chrome 152 HTTP/2 SETTINGS chain, in exact
// emission order (SETTINGS_ENABLE_PUSH:0 is emitted — Chrome disables push
// explicitly). No initial stream id (default odd start), connection flow
// 15663105, root stream priority {exclusive, weight 255}.
var chrome152H2 = h2Config{
	settings: []h2Setting{
		{id: settingHeaderTableSize, val: 65536},
		{id: settingEnablePush, val: 0},
		{id: settingInitialWindowSize, val: 6291456},
		{id: settingMaxHeaderListSize, val: 262144},
	},
	connectionFlow: 15663105,
	priority:       priorityParam{exclusive: true, weight: 255},
	hasPriority:    true,
}

// chrome152H2Mobile is a placeholder mobile SETTINGS chain (desktop copy);
// replace on observation of real Chrome Android 152 H/2 settings.
var chrome152H2Mobile = chrome152H2

// chrome152Desktop is the Chrome 152 desktop profile.
var chrome152Desktop = &browserProfile{
	name:              "chrome-152",
	helloSpec:         &chrome152Spec,
	shuffleExtensions: true,
	ua:                chromeUA,
	platform:          chromePlatform,
	secCHUA:           chromeSecCHUA,
	orderGet:          chromeOrderGet,
	orderPost:         chromeOrderPost,
	baseHeaders:       chromeBaseHeaders,
	insertGet:         chromeInsertGet,
	insertPost:        chromeInsertPost,
	h2:                chrome152H2,
}

// chrome152Mobile is the placeholder Chrome 152 mobile profile (desktop
// data; each Mobile variable above is its single point of substitution).
var chrome152Mobile = &browserProfile{
	name:              "chrome-152-mobile",
	helloSpec:         &chrome152SpecMobile,
	shuffleExtensions: true,
	ua:                chromeUA,
	platform:          chromePlatform,
	secCHUA:           chromeSecCHUA,
	orderGet:          chromeOrderGet,
	orderPost:         chromeOrderPost,
	baseHeaders:       chromeBaseHeaders,
	insertGet:         chromeInsertGet,
	insertPost:        chromeInsertPost,
	h2:                chrome152H2Mobile,
}
