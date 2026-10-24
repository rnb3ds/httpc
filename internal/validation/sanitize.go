package validation

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// sensitiveQueryParamNames contains query parameter names whose values should be
// redacted when sanitizing URLs for logging and error messages.
// Shared across packages to avoid duplicate definitions.
// Read-only after initialization — must not be mutated (concurrent reads).
var sensitiveQueryParamNames = map[string]bool{
	// OAuth and authentication tokens
	"token": true, "access_token": true, "refresh_token": true,
	"id_token": true, "idtoken": true, "bearer": true,
	// API keys and secrets
	"api_key": true, "apikey": true, "api-key": true,
	"secret": true, "secret_key": true, "client_secret": true,
	"private_key": true, "privatekey": true, "private-key": true,
	// Passwords and credentials
	"password": true, "passwd": true, "pass": true, "pwd": true,
	"credential": true, "credentials": true,
	// Session identifiers
	"session_id": true, "sessionid": true,
	// JWT and signatures
	"jwt": true, "signature": true, "sign": true, "sig": true,
	// OAuth/OIDC authorization-code flow and common token spellings that the
	// concise forms above miss (accessToken lowercased, OIDC code, client id).
	"accesstoken": true, "auth_token": true, "authtoken": true,
	"client_id": true, "clientid": true, "code": true,
}

// ASCIILower converts ASCII uppercase letters to lowercase in-place using a
// stack-allocated buffer, avoiding the heap allocation strings.ToLower incurs
// for typical short parameter names. Exported so the security package's domain
// whitelist can reuse it instead of carrying its own identical copy.
func ASCIILower(s string) string {
	// Fast path: check if any uppercase exists
	hasUpper := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return s
	}
	var buf [128]byte
	if len(s) > len(buf) {
		return strings.ToLower(s)
	}
	b := buf[:len(s)]
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

// isSensitiveQueryParamCI performs case-insensitive lookup for sensitive query param names.
func isSensitiveQueryParamCI(name string) bool {
	return sensitiveQueryParamNames[ASCIILower(name)]
}

// isSensitiveQueryParamDecoded checks a parameter name first literally (and
// case-insensitively), then percent-decoded. Without the decode, a key
// obfuscated as "%74oken" (which decodes to "token") would bypass redaction
// entirely — the lookup above matches literal bytes only. Only names
// containing '%' incur the QueryUnescape cost.
func isSensitiveQueryParamDecoded(name string) bool {
	if isSensitiveQueryParamCI(name) {
		return true
	}
	if !strings.Contains(name, "%") {
		return false
	}
	decoded, err := url.QueryUnescape(name)
	if err != nil || decoded == name {
		return false
	}
	return isSensitiveQueryParamCI(decoded)
}

// IsSensitiveQueryParam reports whether the given query parameter name is
// considered sensitive and should be redacted from logs and cache keys.
//
// Currently exercised only by tests; the redaction paths use the
// case-insensitive and percent-decoding variants directly. Retained as the
// package's simple exported probe.
func isSensitiveQueryParam(name string) bool {
	return isSensitiveQueryParamCI(name)
}

// redactSensitiveParams replaces values of sensitive query parameters with
// [REDACTED]. Operates directly on the raw query string to avoid url.Values
// allocation. ';' is treated as a parameter separator alongside '&': legacy
// PHP/servlet configurations parse it as one, and a redaction filter that
// only splits on '&' lets sensitive values ride in ;-separated parameters
// past it ("?a=1;password=hunter2" stayed verbatim). Separators are preserved
// as written so non-sensitive values are byte-identical after redaction.
func redactSensitiveParams(rawQuery string) string {
	b := getSanitizeBuilder()
	b.Grow(len(rawQuery) + 64)

	// ';' counts as a parameter separator for DETECTION (legacy PHP/servlet
	// configurations parse it as one, so "?a=1;password=hunter2" must not
	// pass through verbatim) — but once a sensitive key is found inside an
	// &-delimited segment, the redaction extends to the END of that segment:
	// a ';' inside a sensitive VALUE ("token=a;b") is indistinguishable from
	// a separator, and stopping the redaction at it would leak the value's
	// tail ("token=[REDACTED];b" kept ";b" readable). When the two readings
	// are ambiguous the redaction over-redacts — a possible non-sensitive
	// neighbor behind the sensitive key is dropped, which is the safe
	// direction for a sanitized representation. Segments without any
	// sensitive key are emitted byte-identical.
	ampStart := 0
	for ampStart <= len(rawQuery) {
		ampEnd := len(rawQuery)
		ampSep := byte(0)
		if i := strings.IndexByte(rawQuery[ampStart:], '&'); i >= 0 {
			ampEnd = ampStart + i
			ampSep = '&'
		}
		segment := rawQuery[ampStart:ampEnd]

		// Scan ;-delimited sub-segments for the first sensitive key.
		semiStart, redacted := 0, false
		for semiStart <= len(segment) {
			semiEnd := len(segment)
			semiSep := byte(0)
			if i := strings.IndexByte(segment[semiStart:], ';'); i >= 0 {
				semiEnd = semiStart + i
				semiSep = ';'
			}
			sub := segment[semiStart:semiEnd]
			key := sub
			if k, _, found := strings.Cut(sub, "="); found {
				key = k
			}
			if isSensitiveQueryParamDecoded(key) {
				// Prior sub-segments verbatim (separators included), then the
				// sensitive key redacted to the end of the &-segment.
				b.WriteString(rawQuery[ampStart : ampStart+semiStart])
				b.WriteString(key)
				b.WriteString("=[REDACTED]")
				redacted = true
				break
			}
			if semiSep == 0 {
				break
			}
			semiStart = semiEnd + 1
		}
		if !redacted {
			b.WriteString(segment)
		}

		if ampSep != 0 {
			b.WriteByte('&')
			ampStart = ampEnd + 1
		} else {
			break
		}
	}

	result := strings.Clone(b.String()) // copy: the buffer is pooled and reused
	putSanitizeBuilder(b)
	return result
}

// SanitizeURL removes credentials and redacts sensitive query parameters from a URL
// for safe logging. URLs with credentials are transformed from user:pass@host to
// ***:***@host. Sensitive query parameters (token, api_key, password, etc.) have
// their values replaced with [REDACTED].
//
// SECURITY: this function must FAIL CLOSED. It exists to keep credentials out
// of logs and error messages, so an input that url.Parse rejects — or parses
// into an opaque form that hides userinfo — is best-effort redacted by
// redactUnparseableURL instead of being returned verbatim (returning the raw
// string would leak user:pass@... precisely on the malformed inputs most
// likely to come from untrusted sources).
//
// This function is used to prevent credential leakage in:
//   - Log messages
//   - Error messages
//   - Audit events
//   - Debug output
//
// Example:
//
//	SanitizeURL("https://user:pass@example.com/path?token=secret")
//	// Returns "https://***:***@example.com/path?token=[REDACTED]"
func SanitizeURL(urlStr string) string {
	if urlStr == "" {
		return ""
	}

	// Fast path: skip parsing if URL has no credentials, query params, fragments,
	// or characters that would be escaped by url.Parse (spaces).
	// Most real URLs pass this check and avoid the expensive url.Parse call.
	// Single scan replaces 4 separate strings.Contains calls.
	// Also skip if URL contains query params but none are sensitive — avoids
	// url.Parse + Query() + Encode() round-trip for the common case.
	if !strings.ContainsAny(urlStr, "@?# ") {
		return urlStr
	}

	// If there are query params, check if any are sensitive before parsing.
	// This avoids the expensive url.Parse + query encode cycle for non-sensitive URLs.
	if _, query, found := strings.Cut(urlStr, "?"); found {
		if !HasSensitiveQueryParams(query) && !strings.ContainsAny(urlStr, "@# ") {
			return urlStr
		}
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return redactUnparseableURL(urlStr)
	}

	// Redact sensitive query parameters directly on raw query string
	// to avoid url.Values map allocation from Query()/Encode() round-trip.
	if parsedURL.RawQuery != "" && HasSensitiveQueryParams(parsedURL.RawQuery) {
		parsedURL.RawQuery = redactSensitiveParams(parsedURL.RawQuery)
	}

	// Clear fragment to prevent credential leakage (e.g., OAuth implicit grants)
	parsedURL.Fragment = ""
	parsedURL.RawFragment = ""
	if parsedURL.User == nil {
		// Opaque URLs ("user:pass@host/path" parses as Scheme="user",
		// Opaque="pass@host/path", User=nil) carry the password verbatim in
		// Opaque. Redact it instead of returning parsedURL.String() as-is.
		if parsedURL.Opaque != "" && strings.Contains(parsedURL.Opaque, "@") {
			return redactUnparseableURL(urlStr)
		}
		return parsedURL.String()
	}

	_, hasPassword := parsedURL.User.Password()
	parsedURL.User = nil

	// Estimate size: scheme (8) + ://***:***@ (10) + host + path + query
	estimatedLen := 18 + len(parsedURL.Scheme) + len(parsedURL.Host) + len(parsedURL.Path) + len(parsedURL.RawQuery)
	b := getSanitizeBuilder()
	b.Grow(estimatedLen)
	if parsedURL.Scheme != "" {
		b.WriteString(parsedURL.Scheme)
		b.WriteString("://")
	} else {
		// Scheme-less URLs (e.g. "//u:p@h/p"): keep the protocol-relative
		// "//" form instead of emitting a malformed "://..." prefix.
		b.WriteString("//")
	}
	if hasPassword {
		b.WriteString("***:***")
	} else {
		b.WriteString("***")
	}
	b.WriteByte('@')
	b.WriteString(parsedURL.Host)
	b.WriteString(parsedURL.Path)
	if parsedURL.RawQuery != "" {
		b.WriteByte('?')
		b.WriteString(parsedURL.RawQuery)
	}
	result := strings.Clone(b.String()) // copy: the buffer is pooled and reused
	putSanitizeBuilder(b)
	return result
}

// redactUnparseableURL best-effort redacts credentials in a URL string that
// url.Parse rejected (or that parsed into an opaque form). Without a parsed
// structure the only reliable signal is the '@' separating userinfo from the
// host. The delimiter is the FIRST '@' inside the pre-path region — after any
// scheme separator, before the first '/' or '?' — because userinfo cannot
// contain '/', '?' or '@'. Everything from the scheme separator up to that '@'
// is replaced with "***@". Sensitive query parameters are redacted when a query
// string is present. The result is safe for logging even though it may not be
// a parseable URL — diagnostics beat a verbatim credential leak.
//
// SEC-001 M-2: an earlier version scanned for the LAST '@' in the string.
// When an '@' sat after the first '/' — a path byte (".../p%zz@th") or a
// query value ("...?email=a@b.com") — a path guard concluded "no userinfo"
// and returned the string verbatim, leaking real credentials
// ("http://u:p@h/p%zz@th?token=x" kept both "u:p" and "token=x") and
// defeating the fail-closed contract this function exists to enforce.
func redactUnparseableURL(s string) string {
	// The pre-path region starts after the scheme separator: "scheme://" for
	// hierarchical URLs ("//" itself contains the '/' that would otherwise
	// end the region at once), "//" for protocol-relative URLs, and
	// "scheme:" for opaque forms. A separator candidate is rejected when the
	// text before it already contains '/', '?', or '@' — that text is
	// userinfo or path material, not a scheme.
	start := 0
	if strings.HasPrefix(s, "//") {
		start = 2
	} else if i := strings.Index(s, "://"); i >= 0 && !strings.ContainsAny(s[:i], "/?@") {
		start = i + 3
	} else if i := strings.IndexByte(s, ':'); i >= 0 && !strings.ContainsAny(s[:i], "/?@") {
		start = i + 1
	}

	// Delimiter candidate: first '@' before the first path or query byte.
	region := s[start:]
	regionEnd := len(region)
	if cut := strings.IndexAny(region, "/?"); cut >= 0 {
		regionEnd = cut
	}
	relAt := strings.IndexByte(region[:regionEnd], '@')
	if relAt < 0 {
		// No userinfo-shaped credential, but a query string may still carry
		// sensitive parameters. SEC-001 M-1: url.Parse errors quote the raw
		// URL verbatim (`parse "…?token=…": …`), so this is the last redaction
		// chance for a URL that never parsed — returning it intact would leak
		// the values the parseable path redacts. Same treatment the userinfo
		// branch below applies to its query string.
		if idx := strings.IndexByte(s, '?'); idx >= 0 {
			if q := s[idx+1:]; HasSensitiveQueryParams(q) {
				return s[:idx+1] + redactSensitiveParams(q)
			}
		}
		// Nothing that looks like a credential.
		return s
	}

	prefix, at := s[:start], start+relAt
	redacted := prefix + "***@" + s[at+1:]
	if idx := strings.IndexByte(redacted, '?'); idx >= 0 {
		if q := redacted[idx+1:]; HasSensitiveQueryParams(q) {
			redacted = redacted[:idx+1] + redactSensitiveParams(q)
		}
	}
	return redacted
}

// RedactURLInMessage replaces every occurrence of rawURL in msg with its
// sanitized form, matching BOTH the raw spelling and the strconv.Quote-escaped
// one. The quote form matters because net/url renders *url.Error (and parse
// errors) with %q — a URL containing a character %q escapes (", \, control
// characters) reaches the message in ESCAPED form, so a plain substring match
// misses it and the credential or sensitive query value the redaction exists
// to scrub leaks through verbatim (escaped, but readable). Reports whether
// anything was redacted; msg is returned unchanged when the URL has nothing
// to sanitize or does not appear in either form.
//
// Single implementation shared by validation.redactErrorURL and the engine's
// redactURLInError so the two cannot drift apart again.
func RedactURLInMessage(msg, rawURL string) (string, bool) {
	sanitized := SanitizeURL(rawURL)
	if sanitized == rawURL {
		return msg, false
	}
	changed := false
	if strings.Contains(msg, rawURL) {
		msg = strings.ReplaceAll(msg, rawURL, sanitized)
		changed = true
	}
	if quoted := strconv.Quote(rawURL); strings.Contains(msg, quoted) {
		msg = strings.ReplaceAll(msg, quoted, strconv.Quote(sanitized))
		changed = true
	}
	return msg, changed
}

// sanitizeBuilderPool reduces buffer allocations in SanitizeURL and
// redactSensitiveParams. It pools *bytes.Buffer — NOT strings.Builder (whose
// Reset discards the backing array, retaining only the ~32-byte header);
// bytes.Buffer.Reset keeps the backing slice. The result is COPIED out
// (strings.Clone) before the Put — bytes.Buffer's String aliases the live
// buffer, which the next pool user overwrites.
var sanitizeBuilderPool = sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

// HasSensitiveQueryParams checks if any key-value pair in the query string
// has a sensitive parameter name. Uses byte-level ASCII lowercase to avoid
// allocations from strings.ToLower.
// HasSensitiveQueryParams reports whether any parameter key in the raw query
// string looks sensitive. Parsing is segment-based (&-split, then strip the
// value after '=') so it matches redactSensitiveParams exactly: a bare key
// without '=' is still treated as a key. ';' splits segments alongside '&'
// (see redactSensitiveParams for why legacy separators must count).
func HasSensitiveQueryParams(query string) bool {
	for len(query) > 0 {
		segment := query
		if idx := strings.IndexAny(query, "&;"); idx >= 0 {
			segment = query[:idx]
			query = query[idx+1:]
		} else {
			query = ""
		}
		key := segment
		if eqIdx := strings.IndexByte(segment, '='); eqIdx >= 0 {
			key = segment[:eqIdx]
		}
		if isSensitiveQueryParamDecoded(key) {
			return true
		}
	}
	return false
}

func getSanitizeBuilder() *bytes.Buffer {
	b, ok := sanitizeBuilderPool.Get().(*bytes.Buffer)
	if !ok || b == nil {
		return &bytes.Buffer{}
	}
	b.Reset()
	return b
}

func putSanitizeBuilder(b *bytes.Buffer) {
	if b == nil || b.Cap() > 2048 {
		return
	}
	b.Reset()
	sanitizeBuilderPool.Put(b)
}
