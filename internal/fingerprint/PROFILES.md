# Browser Profile Data — Update Runbook

Profiles are snapshots of **one browser build's** wire behavior. Browsers ship every
~4 weeks; a profile that does not move with them becomes a contradiction on the wire
(a User-Agent claiming one version, a ClientHello claiming another), and cross-layer
consistency checks are exactly what modern WAFs score. This runbook defines how a
profile is cut, verified, and refreshed.

## What one profile is

| Layer | Where it lives | Notes |
|---|---|---|
| TLS ClientHello (Chrome) | `profile_chrome.go` — `chrome152Spec` + `chrome152TrustAnchorIDs` | Hand-maintained; utls has no built-in hello this new. Includes ML-DSA sigalgs, X25519MLKEM768, trust-anchor-ids ext `0xca34` |
| TLS ClientHello (Firefox) | `profile_firefox.go` → utls `HelloFirefox_148` | Delegated to utls; bump the hello ID when utls ships a newer one |
| Per-connection mutations | `ja.go` | Chromium extension shuffle + GREASE signature-algorithm substitution — shared, not per-profile data |
| HTTP/2 frames | `profile_*.go` — `chrome152H2` / `firefox148H2` | SETTINGS order+values, connection flow, stream id, root priority |
| Header block | `profile_*.go` — order tables, base headers, per-method inserts, `secCHUA`, UA/platform maps | GET = navigation shape, POST = fetch/XHR shape |
| Preset names | `preset.go` | Registry; `*-latest` aliases follow the shipped version |
| Verification gates | `e2e_network_test.go` | Byte-exact Akamai H2 hashes + JA4 shape against tls.peet.ws |

Each browser has explicit **single points of substitution** for its mobile variant
(`chrome152SpecMobile`, `chrome152H2Mobile`, Firefox equivalents) — today verbatim
desktop copies, to be replaced when real device bytes are captured.

## Cutting a new profile (e.g. Chrome 152 → 153)

Capture from a **release build of the real browser** on a clean profile. For Chrome
the trust-anchor bytes must come from a Google Chrome release specifically — the list
order is a compile-time constant of the shipped root store, so Chromium or
ungoogled-chromium builds of the same version are not interchangeable
(`profile_chrome.go` documents why).

1. **TLS**: capture the ClientHello bytes (Wireshark on a clean-profile session, or
   any echo endpoint that reports ja3/ja4, e.g. `https://tls.peet.ws/api/all`).
   Diff the fields against `chrome152Spec`: cipher list, extensions (presence,
   order — noting the shuffle makes order per-connection), supported groups,
   key shares, signature algorithms (incl. GREASE position), ALPN, new/removed
   extensions. Watch for post-quantum and trust-anchor changes first — those are
   the fields recent Chrome majors moved.
2. **HTTP/2**: read the Akamai fingerprint string from the echo endpoint, e.g.
   `1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p`. Map it back onto
   SETTINGS id:value pairs in emission order, the connection-level
   WINDOW_UPDATE increment, PRIORITY frames (usually none for Chrome), and the
   pseudo-header order. Transcribe into the profile's `h2Config`.
3. **Headers**: from DevTools (or a proxy capture), take the raw request headers
   of a plain address-bar GET (navigation shape) and an XHR/fetch POST (fetch
   shape). Update the order tables, insert values, `sec-ch-ua` brand list
   (must match the new major), and per-OS UA strings.
4. **Point the data**: update the variables in the profile file. If the major
   version changed, rename the profile (`name`, preset entries in `preset.go`,
   and the versioned aliases) and let `*-latest` follow. Add the retired major
   as an explicit alias only if you intend to keep serving it.
5. **Local gates**: `go test -race ./internal/fingerprint/` must pass.
6. **Re-baseline the network gates**: run
   `go test -tags fingerprint_e2e ./internal/fingerprint/ -run PeetWSE2E -v`,
   transcribe the fresh Akamai hashes and UA into `e2e_network_test.go`, and
   commit data + gates together (never update one without the other).
7. **Docs**: update the "Verified end-to-end" fingerprints in `README.md` and
   `README_zh-CN.md`.

## Mobile variants

`*-android` / `*-ios` presets currently emit desktop bytes — an internally
incoherent pair (mobile UA, desktop JA4/Akamai hash) that a coherence check can
flag. Cutting real ones requires captures from real devices:

- Chrome on Android sends its own SETTINGS chain and header set; replace
  `chrome152SpecMobile` / `chrome152H2Mobile` plus the order tables.
- iOS Chrome is WebKit underneath (CriOS UA); it resembles Safari's shapes more
  than desktop Chrome's.

## Drift monitoring

`.github/workflows/fingerprint-drift.yml` runs the peet.ws gates weekly. A red run
means one of:

1. **Profile drift** — the shipped profile no longer matches the browser version
   it claims (the common case; cut a new profile per above), or
2. **Echo-service change** — tls.peet.ws itself changed; verify by hand against a
   second source (e.g. a self-hosted `fingerproxy`) before acting.

peet.ws is a third-party service; for self-hosted verification, run any endpoint
that echoes JA3/JA4 + the Akamai H2 fingerprint and point a local copy of the
gates at it.

## Coherence checklist (what a WAF cross-checks)

Every layer must point at **one** browser build:

- JA4 (TLS) ↔ Akamai H2 hash (framing) ↔ header order and values (JA4H) ↔
  `sec-ch-ua` brand list ↔ User-Agent.
- Updating the UA string alone manufactures a mismatch. Never mix data captured
  from different browser versions, and land data + verification hashes in the
  same change.
