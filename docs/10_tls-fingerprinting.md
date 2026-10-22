# Browser Transport Fingerprinting

How `Security.TLSFingerprint` emulates a browser's TLS, HTTP/2, and header
fingerprint — what stays guaranteed, and what it cannot do. For a quick
introduction see the README section "Browser Transport Fingerprinting".

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Preset Names](#preset-names)
- [Scope](#scope)
- [Behavior Notes](#behavior-notes)
- [Session Resumption](#session-resumption)
- [Limitations](#limitations)
- [Verification](#verification)
- [Implementation and Dependencies](#implementation-and-dependencies)

## Overview

Sites that fingerprint browsers observe more than the User-Agent: the TLS
ClientHello (JA3/JA4), the HTTP/2 frames (SETTINGS values and order, WINDOW_UPDATE,
pseudo-header order, priorities, HPACK behavior), and the header block itself — including
a User-Agent coherent with the ClientHello version. Setting `Security.TLSFingerprint`
replaces httpc's transport layer with a browser emulation covering all of them at once,
over fully fingerprinted HTTP/2 (preferred) and HTTP/1.1 (fallback).

Everything above the transport stays in charge: redirects, retries, middleware, streaming
downloads, and cookie handling keep their httpc semantics.

## Quick Start

```go
cfg := httpc.DefaultConfig()
cfg.Security.TLSFingerprint = "chrome-latest" // or "chrome-windows", "firefox-latest", ...
client, err := httpc.New(cfg)
```

Leave `TLSFingerprint` empty to keep the standard stack with no extra dependencies.

## Preset Names

Preset names are browser profiles with optional OS suffixes: `chrome` / `chrome-latest`
/ `chrome-155` (= Chrome 155) and `firefox` / `firefox-latest` / `firefox-158`
(= Firefox 158), each as `*-windows` / `*-macos` / `*-linux` desktop variants and
`*-android` / `*-ios` mobile variants (e.g. `chrome-windows`, `firefox-ios`), plus the
retired-major pins `chrome-152` and `firefox-148`, the legacy compound aliases
`android-chrome-latest` and `ios-chrome-latest`, and `safari-ios` (=
`ios-safari-latest`) — Safari on the same verified WebKit data as the other iOS
presets. There is no desktop-Safari profile: macOS Safari's ClientHello differs and
no verified data for it exists yet.

## Scope

Fingerprinting makes the *transport layers* — TLS, HTTP/2 framing, ordered header
block — indistinguishable from the spoofed browser on passive observations (JA3/JA4, the
HTTP/2 fingerprint, header order). It does not reproduce browser-side JavaScript or
behavioral signals — no pure HTTP client can — so it is not a substitute for a real browser
or an authorized challenge-solving step where a site requires one. Profiles are snapshots of
one browser build and age as browsers ship; a weekly CI job watches for drift
(`.github/workflows/fingerprint-drift.yml`).

## Behavior Notes

- **Header set is authoritative**: the preset replaces same-named request headers
  (User-Agent, Accept, Accept-Language, ...) with the browser profile's values so the wire
  stays coherent with the spoofed ClientHello; custom headers pass through untouched. An
  explicitly set User-Agent does **not** override the preset on this path. Names are written
  with the browser's own casing — lower-case `sec-ch-ua*`, Title-Case everything else — on
  HTTP/1.1 as well as HTTP/2. `Result.Request.Headers` reports exactly these wire headers
  (the browser's UA and casing), not the engine-side defaults such as `httpc/1.0`.
- **Protocols**: ALPN decides. A server that speaks HTTP/2 gets HTTP/2, one that only speaks
  HTTP/1.1 gets HTTP/1.1, and no request is ever replayed: there is no manual fallback between
  the two. HTTP/3 is never attempted — a QUIC connection uses a separate TLS stack that
  bypasses `CertificatePinner`, and losing the pin silently is worse than not racing protocols.
  `Connection.EnableHTTP2=false` pins the path to HTTP/1.1 and rewrites the ClientHello's ALPN
  list to match.
- **Proxies**: proxy pools, rotation, and circuit breaking keep working (HTTP, HTTPS and
  SOCKS5/5h). Plain-HTTP targets through an HTTP proxy traverse a CONNECT tunnel; HTTPS proxies
  speak HTTP/1.1 CONNECT only and their certificate is verified against the system roots and the
  proxy hostname (set `Security.InsecureSkipVerify` to opt out, exactly as on the standard
  transport). The proxy is fixed per connection pool — rotation
  selects a different pool rather than re-pointing a live one.
- **Redirect coherence**: on every redirect hop the profile's first-hop values are corrected
  the way a browser does: `sec-fetch-site` is recomputed against the previous hop's URL
  (`same-origin` / `same-site` / `cross-site`), `sec-fetch-user` is dropped (first hop
  only), and a 301/302/303 that downgrades a fetch-style request to GET keeps fetch-style
  headers instead of switching to navigation headers.
- **Certificate pinning works**: `CertificatePinner` runs on every fingerprinted handshake
  (see the README "Certificate Pinning" section). `Security.TLSConfig` remains mutually
  exclusive with fingerprinting (a custom `tls.Config` cannot shape a spoofed ClientHello).
- **Decompression**: the profile advertises `gzip, deflate, br, zstd` and the fingerprint
  stack decodes all of them (streaming preserved), handing the engine identity bodies —
  the engine cannot see the original `Content-Encoding`, so the decoded stream is capped
  by `MaxResponseBodySize` (`MaxDecompressedBodySize` does not apply on this path). An
  unknown `Content-Encoding` fails the request loudly instead of returning compressed
  bytes.
- **SSRF protection**: direct connections dial through a validating resolver — resolve
  once, filter to SSRF-allowed IPs, dial the validated IP literal (never the hostname) —
  the same dial-layer guarantee as the standard transport: no TOCTOU window, no
  mixed-record pass. A per-request pre-flight (repeated per redirect hop) fails blocked
  targets before any dial. Through a proxy the target cannot be pinned (the proxy resolves
  it), so the pre-flight is the enforcement — the same rule and the same trust-the-proxy
  residual as the standard path's proxied-target validation.

## Session Resumption

The Chrome profiles cache TLS session tickets per connection
pool and resume repeat connections with a PSK, exactly like a real browser revisiting
a host (verified live in `session_resumption_test.go`). Profiles whose ClientHello
carries no pre_shared_key extension (Firefox, the iOS WebKit profiles) always
full-handshake. Set `Security.TLSFingerprintDisableSessionTickets` to force a full
handshake on every connection.

## Limitations

Fingerprinted connections are not counted in pool metrics and skip
`Connection.EnableDoH`. Android presets (`*-android`) currently emit the desktop
fingerprint bytes — placeholders pending real device captures. iOS presets
(`*-ios`, including `safari-ios`) use real WebKit data (Safari 26.3 TLS + HTTP/2,
byte-verified live), with the regular-header order still a desktop approximation.

## Verification

Verified end-to-end against real endpoints in `internal/fingerprint/e2e_network_test.go`
(run with `go test -tags fingerprint_e2e ./internal/fingerprint/ -run PeetWSE2E`):

- `chrome-latest` — negotiates h2 with the Chrome HTTP/2 fingerprint
  `1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p`
  (hash `52d84b11737d980aef856699f885ca86`) and a Chrome 155 UA.
- `firefox-latest` — `1:65536;2:0;4:131072;5:16384|12517377|0|m,p,a,s`
  (hash `6ea73faa8fc5aac76bded7bd238f6433`) with a Firefox 158 UA.
- `chrome-ios` / `safari-ios` — WebKit JA4 `t13d2013h2_a09f3c656075_7f0f34a4126d`
  with the HTTP/2 fingerprint `2:0;3:100;4:2097152;9:1|10420225|0|m,s,a,p`
  (hash `c52879e43202aeb92740be6e8c86ea96`).

## Implementation and Dependencies

The fingerprint data lives in this repository (`internal/fingerprint`): httpc builds the
ClientHello spec, the HTTP/2 SETTINGS sequence and the header tables, and hands them to
[`bogdanfinn/tls-client`](https://github.com/bogdanfinn/tls-client) through its profile API —
so a profile can target a browser build ahead of any upstream release. The transport itself
(uTLS handshake, HTTP/2 framing, header order and casing) comes from that library, and
[`andybalholm/brotli`](https://github.com/andybalholm/brotli) +
[`klauspost/compress`](https://github.com/klauspost/compress) (zstd) handle decoding.
Profile data was originally ported from [`enetx/surf`](https://github.com/enetx/surf) (MIT);
httpc does not depend on that client library, nor on `enetx/http`, `enetx/http2` or
`refraction-networking/utls`.
