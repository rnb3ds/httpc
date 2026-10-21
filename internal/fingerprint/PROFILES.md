# Browser Profile Data — Update Runbook

Profiles are snapshots of **one browser build's** wire behavior. Browsers ship every
~4 weeks; a profile that does not move with them becomes a contradiction on the wire
(a User-Agent claiming one version, a ClientHello claiming another), and cross-layer
consistency checks are exactly what modern WAFs score. This runbook defines how a
profile is cut, verified, and refreshed.

## What one profile is

| Layer | Where it lives | Notes |
|---|---|---|
| TLS ClientHello (Chrome) | `profile_chrome.go` — `chrome152Spec` + `chrome152TrustAnchorIDs` | Hand-maintained and owned by httpc, handed to the transport through a per-handshake `SpecFactory`; no upstream library ships a hello this new. Includes ML-DSA sigalgs, X25519MLKEM768, trust-anchor-ids ext `0xca34`. Serves every shipped Chrome profile: majors 152–156 share one ClientHello (shipped label: 155; 152 kept as a pinned alias; only the UA map and sec-ch-ua brand list are per-major) |
| TLS ClientHello (Firefox) | `profile_firefox.go` → utls `HelloFirefox_148` | Delegated to utls. The 158 label rides the 148 parrot (no observed ClientHello change in the 149–158 line; the JA4 network gate is the tripwire). Bump the hello ID when utls ships a newer one |
| Per-connection mutations | `ja.go` | Chromium extension shuffle + GREASE signature-algorithm substitution — shared, not per-profile data |
| HTTP/2 frames | `profile_*.go` — `chrome152H2` / `firefox148H2` | SETTINGS order+values, connection flow, stream id, root priority |
| Header block | `profile_*.go` — order tables, base headers, per-method inserts, `secCHUA`, UA/platform maps | GET = navigation shape, POST = fetch/XHR shape |
| Preset names | `preset.go` | Registry; `*-latest` aliases follow the shipped version. Third family: `safari-ios` on the same verified WebKit data (`profile_safari.go`) |
| Verification gates | `e2e_network_test.go` + `ja4.go`/`ja4_test.go` | Byte-exact Akamai H2 hashes **and byte-exact JA4 strings** against tls.peet.ws; JA4s are also derived **offline** from the specs (`ja4OfSpec`), so a spec edit that changes the wire shape fails a unit test before any network run |

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
   as an explicit alias only if you intend to keep serving it (the 152→155
   bump did exactly that: `chrome-152` still serves its own UA/brand data over
   the shared spec).
   If only the version label moved and the echo endpoint still reports the same
   JA4, the ClientHello did not change: bump the UA / `sec-ch-ua` strings and
   the preset aliases only. Chrome 152 through 156 all share
   `t13d1517h2_8daaf6152771_cb7bf5808d99`, so that is the common case now that
   Chrome ships every two weeks. The sec-ch-ua GREASE brand, its version and
   its position rotate with the major — never copy the old value forward;
   `TestProfile_SecCHUABrandLists` pins them.
5. **Local gates**: `go test -race ./internal/fingerprint/` must pass.
6. **Re-baseline the network gates**: run
   `go test -tags fingerprint_e2e ./internal/fingerprint/ -run PeetWSE2E -v`,
   transcribe the fresh **JA4 strings**, Akamai hashes and UA into
   `e2e_network_test.go`, and commit data + gates together (never update one
   without the other). The JA4 assertion is exact on purpose: a prefix check
   would pass for any TLS 1.3 + h2 client and could never notice a lost ML-DSA
   scheme or trust_anchors extension.
7. **Docs**: update the "Verified end-to-end" fingerprints in `README.md` and
   `README_zh-CN.md`.

## Mobile variants

**iOS is real data** (since 2026-10-21): every iOS browser is WebKit underneath,
so `chrome-ios` / `firefox-ios` use utls's `HelloSafari_26_3` (offline JA4
`t13d2013h2_a09f3c656075_7f0f34a4126d`, matching the dominant live iOS
observation in public JA4 databases) plus the Safari iOS 26 H2 chain
(`safariIOSH2`, cross-checked against tls-client's Safari_IOS_26_0 profile).
The regular-header order tables (`safariOrderGet`/`safariOrderPost`) carry
WebKit's `m,s,a,p` pseudo order but keep the desktop regular-header
approximation — replace them when a real CriOS/FxiOS capture exists. The
network gates pin the WebKit JA4 and Akamai hash.

**Android remains a placeholder**: `chrome155Android` emits desktop TLS bytes,
which are KNOWN wrong — live traffic shows Android Chrome as a 16-extension
hello (`t13d1516h2_8daaf6152771_02713d6af862`, 109 public observations,
October 2026), and the desktop spec minus any single extension (ALPS,
trust_anchors, GREASE-ECH each tested) does not reproduce that hash, so the
difference is more than one extension. To cut it: capture a real Chrome
Android 155 ClientHello, diff against `chrome152Spec`, and verify with
`ja4OfSpec` (ja4.go) before touching the network gates. `firefox158Android`
rides the desktop Gecko hello, which is believed identical across form
factors (Firefox shares the TLS stack), but is marked placeholder until a
device capture confirms it.

## Drift monitoring

Next label bump watch (observed 2026-10-21, not yet shipped): Chrome 156
(rolling out since 2026-10-20) adds `image/jxl` to the navigation Accept
header — the first header-value change in the 152–156 line. A 155→156 bump
is therefore NOT label-only: update `chromeInsertGet`'s Accept as well.

`.github/workflows/fingerprint-drift.yml` runs the peet.ws gates weekly, plus the
stable-version lag gate (`drift_version_test.go`): it compares `ChromeMajor` /
`FirefoxMajor` against the vendors' stable channels and fails when a profile lags
by four majors or more — the fingerprint bytes may still be byte-exact while the
claimed version has aged out of the plausible population. A red run
means one of:

1. **Profile drift** — the shipped profile no longer matches the browser version
   it claims (the common case; cut a new profile per above), or
2. **Version lag** — the UA major fell too far behind stable; usually a label
   bump (`ChromeMajor`/`FirefoxMajor` + UA tables + aliases), or
3. **Echo-service change** — tls.peet.ws itself changed; verify by hand against a
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
