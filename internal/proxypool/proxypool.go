// Package proxypool provides a rotating proxy pool with passive health checking
// and circuit breaking for the httpc library.
//
// A Pool holds a list of proxy URLs and selects one per request according to a
// strategy (round-robin or random). Connection-level failures are tracked
// passively: after a configurable number of consecutive failures a proxy's
// circuit opens and it is temporarily skipped, then automatically retried
// (half-open probe) after a cooldown.
//
// HTTP status codes (e.g. 403) are NOT treated as proxy failures here — they
// are target-specific. Status-based rotation is handled at the retry layer
// via deterministic SelectIndex(base+attempt), which advances one pool entry
// per retry attempt regardless of strategy.
package proxypool

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/cybergodev/httpc/internal/validation"
)

// Strategy selects the algorithm for choosing a proxy from the pool.
type Strategy int

const (
	// StrategyRoundRobin cycles through proxies in order. Each selection
	// advances the cursor, so a retry that re-invokes Select naturally lands
	// on a different proxy without any extra wiring.
	StrategyRoundRobin Strategy = iota
	// StrategyRandom picks a healthy proxy uniformly at random.
	StrategyRandom
)

// Defaults applied when the corresponding Config field is zero.
const (
	defaultFailureThreshold = 3
	defaultCooldown         = 30 * time.Second
)

// ErrNoProxies is returned when the pool is created with no proxy URLs.
var ErrNoProxies = errors.New("proxy pool is empty")

// Config configures a proxy pool. Zero-value fields receive defaults.
type Config struct {
	// Proxies is the list of proxy URLs (http, https, socks5, socks5h).
	// Entries sharing the same host:port are collapsed to the first occurrence.
	Proxies []string

	// Strategy selects how proxies are chosen. The zero value defaults to
	// StrategyRoundRobin.
	Strategy Strategy

	// FailureThreshold is the number of consecutive connection failures to a
	// proxy before its circuit opens. Zero defaults to 3.
	FailureThreshold int

	// Cooldown is how long a circuit stays open before the proxy is eligible
	// again (half-open probe). Zero defaults to 30s.
	Cooldown time.Duration
}

// entry tracks the live state of a single proxy.
type entry struct {
	url  *url.URL // parsed proxy URL returned by Select
	host string   // canonical dial address (host:port, default port appended); the key for ReportFailure/ReportSuccess

	failures  atomic.Int64 // consecutive failure count; reset to 0 on success
	openUntil atomic.Int64 // unix-nano timestamp until which the circuit is open; 0 = closed
}

// Pool is a concurrency-safe, rotating proxy pool with passive circuit breaking.
// It is safe for concurrent use by multiple goroutines. Create one with New.
type Pool struct {
	entries []*entry
	byHost  map[string]*entry // host:port -> entry; built once, read-only after

	counter atomic.Uint64 // round-robin cursor

	failureThreshold int
	cooldown         time.Duration
	strategy         Strategy
}

// New creates a proxy pool from the given configuration. All proxy URLs are
// validated with the same validator the public Config layer uses, so scheme
// and host rules cannot drift. Entries sharing a host:port are collapsed to
// the first occurrence.
func New(cfg Config) (*Pool, error) {
	if len(cfg.Proxies) == 0 {
		return nil, ErrNoProxies
	}

	threshold := cfg.FailureThreshold
	if threshold <= 0 {
		threshold = defaultFailureThreshold
	}
	cooldown := cfg.Cooldown
	if cooldown <= 0 {
		cooldown = defaultCooldown
	}

	entries := make([]*entry, 0, len(cfg.Proxies))
	byHost := make(map[string]*entry, len(cfg.Proxies))

	for _, raw := range cfg.Proxies {
		u, err := validation.ValidateProxyURL(raw)
		if err != nil {
			// SECURITY: sanitize before echoing — a misconfigured proxy URL
			// containing userinfo would otherwise print its password into
			// application logs.
			return nil, fmt.Errorf("proxy pool entry %q: %w", validation.SanitizeURL(raw), err)
		}
		// Key every entry by the canonical address net/http actually dials
		// (default port appended when the URL omits one). The dialer reports
		// failures/successes with that same address, so the keys must match or
		// circuit breaking silently never fires (e.g. "socks5://p" dials
		// "p:1080" but was previously keyed as "p").
		host := validation.CanonicalProxyAddr(u)
		// Collapse duplicates by host:port — the same proxy listed twice would
		// skew round-robin distribution and double-count failures.
		if _, exists := byHost[host]; exists {
			continue
		}
		e := &entry{url: u, host: host}
		entries = append(entries, e)
		byHost[host] = e
	}

	if len(entries) == 0 {
		return nil, ErrNoProxies
	}

	return &Pool{
		entries:          entries,
		byHost:           byHost,
		failureThreshold: threshold,
		cooldown:         cooldown,
		strategy:         cfg.Strategy,
	}, nil
}

// Select returns a proxy URL for the given request, skipping any proxy whose
// circuit is currently open. If every circuit is open it returns the proxy
// closest to recovery (earliest open-until timestamp) as a best-effort fallback
// rather than failing outright.
//
// The request parameter is accepted to satisfy the http.Transport.Proxy
// signature; it is not used in the current implementation.
func (p *Pool) Select(_ *http.Request) (*url.URL, error) {
	now := time.Now().UnixNano()

	if p.strategy == StrategyRandom {
		return p.selectRandom(now), nil
	}
	return p.selectRoundRobin(now), nil
}

// selectFrom scans entries starting at (start+i)%n, returning the first one
// whose circuit is closed; when every circuit is open it returns the entry
// closest to recovery (earliest open-until timestamp) as a best-effort
// fallback rather than failing outright. start must already be normalized
// to [0, n). Single implementation for the scan+fallback logic so
// selectRoundRobin, selectRandom and SelectIndex cannot drift.
func (p *Pool) selectFrom(start, n int, now int64) *url.URL {
	var fallback *entry
	var fallbackOpenUntil int64

	for i := 0; i < n; i++ {
		e := p.entries[(start+i)%n]
		ou := e.openUntil.Load()
		if ou <= now {
			return e.url
		}
		if fallback == nil || ou < fallbackOpenUntil {
			fallback = e
			fallbackOpenUntil = ou
		}
	}
	return fallback.url
}

// selectRoundRobin advances the global cursor and scans forward, returning the
// first healthy entry. The cursor advances on every call, so a retry that
// re-invokes Select naturally lands on a different proxy.
//
// The counter is reduced modulo n BEFORE the int conversion: on 32-bit
// platforms int(uint64) truncates and can sign-extend negative, and a single
// "if start < 0 { start += n }" cannot recover from values far below -n —
// index-out-of-range after 2^31 selections on a long-lived process.
func (p *Pool) selectRoundRobin(now int64) *url.URL {
	n := len(p.entries)
	// Add(1)-1 = the pre-increment value: the first call returns index 0,
	// preserving the historical sequence. The modulo happens BEFORE the int
	// conversion (32-bit truncation safety — see the comment above).
	start := int((p.counter.Add(1) - 1) % uint64(n))
	return p.selectFrom(start, n, now)
}

// selectRandom probes from a random entry point, returning the first healthy
// one. If every circuit is open it falls back to the entry closest to
// recovery.
func (p *Pool) selectRandom(now int64) *url.URL {
	n := len(p.entries)

	// Random start + sequential scan. Bounded random probing (n draws with
	// replacement) misses the only healthy entry with probability
	// ((n-m)/n)^n — ~35% for n=10, m=1 — and then the fallback inside
	// selectFrom returns a known-dead proxy. The rotation scan keeps the
	// random entry point but guarantees a healthy proxy is returned whenever
	// one exists (the scan covers all n entries; the fallback tracks the
	// global minimum open-until across the same sweep).
	return p.selectFrom(rand.IntN(n), n, now)
}

// NextIndex atomically advances the round-robin cursor once and returns the
// resulting entry index (modulo pool size). Unlike Select, it does not check
// circuit-open proxies or return a URL — it simply reserves a starting
// position. Used by the retry layer to reserve a unique base proxy per
// request, ensuring inter-request rotation; each retry attempt then uses
// base + attempt as the SelectIndex argument for intra-request rotation.
//
// Under StrategyRandom the base is drawn uniformly at random instead of
// advancing the cursor: without this, any rotation wiring (ProxyRotatePerRequest
// or ProxyRotateOnStatus, both of which pin every attempt to SelectIndex)
// would silently turn the random strategy into sequential round-robin. The
// random draw is per REQUEST (per base), so the base+attempt walk stays
// deterministic within one attempt — redirect hops re-selecting the same
// index keep landing on the same proxy.
func (p *Pool) NextIndex() int {
	n := len(p.entries)
	if p.strategy == StrategyRandom {
		return rand.IntN(n)
	}
	// Pre-increment value, modulo before the int conversion — see
	// selectRoundRobin for both rationales.
	return int((p.counter.Add(1) - 1) % uint64(n))
}

// SelectIndex returns a proxy URL deterministically indexed by attempt,
// skipping any proxy whose circuit is currently open. Unlike Select it does
// NOT advance the round-robin counter, so the same attempt always lands on the
// same proxy regardless of how many times the transport calls Proxy within a
// single logical request (e.g. redirect-following).
//
// This is used by the retry layer to guarantee that retry attempt N selects a
// DIFFERENT proxy than attempt N-1, even when redirect chains consume extra
// Select calls and would otherwise desynchronize the round-robin cursor.
func (p *Pool) SelectIndex(attempt int) *url.URL {
	now := time.Now().UnixNano()

	n := len(p.entries)
	start := attempt % n
	if start < 0 {
		start += n
	}

	return p.selectFrom(start, n, now)
}

// ReportFailure records a connection-level failure for the proxy at the given
// canonical host:port (as net/http dials it — see CanonicalProxyAddr). After
// FailureThreshold consecutive failures the proxy's circuit opens and it is
// temporarily skipped by Select for Cooldown.
//
// SCOPE: this is called from the connection pool's DialContext callback, so it
// observes first-hop TCP dial outcomes ONLY. Failures that occur after the
// dial returns — TLS handshake errors to an https:// proxy, CONNECT refusals
// (407/502), socks5 greeting failures — are structurally invisible here and
// do NOT circuit-break the proxy; a successful TCP dial reports success even
// if the proxy then rejects every CONNECT. Use Connection.ProxyRotateOnStatus
// for status-based rotation instead.
//
// Only connection-level failures should be reported here. HTTP status codes
// (e.g. 403) are target-specific and must NOT circuit-break a proxy; they are
// handled by retrying with a fresh Select (see Connection.ProxyRotateOnStatus).
func (p *Pool) ReportFailure(host string) {
	e, ok := p.byHost[host]
	if !ok {
		return
	}
	if e.failures.Add(1) >= int64(p.failureThreshold) {
		e.openUntil.Store(time.Now().Add(p.cooldown).UnixNano())
	}
}

// ReportSuccess resets a proxy's failure count and closes its circuit, marking
// it healthy for immediate reuse.
func (p *Pool) ReportSuccess(host string) {
	e, ok := p.byHost[host]
	if !ok {
		return
	}
	e.failures.Store(0)
	e.openUntil.Store(0)
}

// Hosts returns the canonical host:port dial address of every proxy in the
// pool, including those whose circuit is currently open. Used to seed SSRF
// exemptions so that proxy connections are never blocked by private-IP
// validation. The addresses match what net/http dials (default port appended
// for portless entries), which is what the dial callback compares against.
func (p *Pool) Hosts() []string {
	hosts := make([]string, len(p.entries))
	for i, e := range p.entries {
		hosts[i] = e.host
	}
	return hosts
}

// Len returns the number of proxies in the pool.
func (p *Pool) Len() int {
	return len(p.entries)
}
