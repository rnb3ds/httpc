package proxypool

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The standard 3-proxy set used by most tests (the default for newTestPool).
const (
	testProxy1 = "http://proxy1.example.com:8080"
	testProxy2 = "http://proxy2.example.com:8080"
	testProxy3 = "http://proxy3.example.com:8080"
)

// testPoolOpts describes a pool under test; the zero value builds the
// standard 3-proxy round-robin pool. Fields map 1:1 onto proxypool.Config.
type testPoolOpts struct {
	proxies          []string
	strategy         Strategy
	failureThreshold int
	cooldown         time.Duration
}

// newTestPool constructs a pool, failing the test on any construction error,
// so test bodies start from a valid pool without repeating New() scaffolding.
func newTestPool(t *testing.T, opts testPoolOpts) *Pool {
	t.Helper()
	if opts.proxies == nil {
		opts.proxies = []string{testProxy1, testProxy2, testProxy3}
	}
	pool, err := New(Config{
		Proxies:          opts.proxies,
		Strategy:         opts.strategy,
		FailureThreshold: opts.failureThreshold,
		Cooldown:         opts.cooldown,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return pool
}

// ============================================================================
// Construction & validation (table)
// ============================================================================

// TestNew covers construction validation: empty input, invalid URLs (one bad
// entry poisons the pool), the accepted scheme set, and host:port
// deduplication. Formerly five separate TestNew_* functions.
func TestNew(t *testing.T) {
	tests := []struct {
		name         string
		proxies      []string
		wantErr      bool
		wantSentinel error // when set, the error must be exactly this value
		wantLen      int   // expected Len() when construction succeeds
	}{
		{
			name:         "empty pool",
			proxies:      nil,
			wantErr:      true,
			wantSentinel: ErrNoProxies,
		},
		{
			name:    "missing scheme",
			proxies: []string{"proxy.example.com:8080", "http://valid.example.com:8080"},
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			proxies: []string{"ftp://proxy.example.com:8080", "http://valid.example.com:8080"},
			wantErr: true,
		},
		{
			name:    "missing host",
			proxies: []string{"http://", "http://valid.example.com:8080"},
			wantErr: true,
		},
		{
			name: "valid schemes",
			proxies: []string{
				"http://proxy.example.com:8080",
				"https://proxy.example.com:8443",
				"socks5://proxy.example.com:1080",
				"socks5h://proxy.example.com:1081",
			},
			wantLen: 4,
		},
		{
			name: "deduplicates by identity (host:port + credentials)",
			proxies: []string{
				"http://127.0.0.1:8080",
				"http://user:pass@127.0.0.1:8080", // same host:port, different creds: DISTINCT (per-user egress credentials)
				"http://127.0.0.1:8081",           // different port
				"http://127.0.0.1:8080",           // verbatim duplicate of the first: collapses
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := New(Config{Proxies: tt.proxies})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("New(%v): expected error, got nil", tt.proxies)
				}
				if tt.wantSentinel != nil && err != tt.wantSentinel {
					t.Errorf("New() with no proxies: got err=%v, want %v", err, tt.wantSentinel)
				}
				return
			}
			if err != nil {
				t.Fatalf("New(%v) failed: %v", tt.proxies, err)
			}
			if pool.Len() != tt.wantLen {
				t.Errorf("Len() = %d, want %d", pool.Len(), tt.wantLen)
			}
		})
	}
}

// TestNew_AppliesDefaults verifies zero-value Config fields receive the
// documented defaults (threshold 3, cooldown 30s, round-robin strategy).
func TestNew_AppliesDefaults(t *testing.T) {
	pool := newTestPool(t, testPoolOpts{proxies: []string{"http://proxy.example.com:8080"}})

	if pool.failureThreshold != defaultFailureThreshold {
		t.Errorf("failureThreshold = %d, want %d", pool.failureThreshold, defaultFailureThreshold)
	}
	if pool.cooldown != defaultCooldown {
		t.Errorf("cooldown = %v, want %v", pool.cooldown, defaultCooldown)
	}
	if pool.strategy != StrategyRoundRobin {
		t.Errorf("strategy = %d, want %d (zero value default)", pool.strategy, StrategyRoundRobin)
	}
}

// ============================================================================
// Select: rotation and circuit-aware fallback (table)
// ============================================================================

// TestSelect covers selection behavior across strategies and circuit states.
// Each row was formerly a standalone TestSelect_* function; assertions are
// preserved verbatim inside the check closures.
func TestSelect(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) *Pool
		check func(t *testing.T, pool *Pool)
	}{
		{
			name:  "round robin",
			setup: func(t *testing.T) *Pool { return newTestPool(t, testPoolOpts{}) },
			check: func(t *testing.T, pool *Pool) {
				expected := []string{
					testProxy1,
					testProxy2,
					testProxy3,
					testProxy1, // wraps around
				}

				for i, want := range expected {
					got, err := pool.Select(nil)
					if err != nil {
						t.Fatalf("Select() error: %v", err)
					}
					if got.String() != want {
						t.Errorf("Select() call %d: got %s, want %s", i, got.String(), want)
					}
				}
			},
		},
		{
			name: "skips circuit open",
			setup: func(t *testing.T) *Pool {
				pool := newTestPool(t, testPoolOpts{
					proxies:          []string{testProxy1, testProxy2},
					failureThreshold: 1, // open on first failure for easy testing
					cooldown:         10 * time.Minute,
				})
				// Force proxy1's circuit open
				pool.ReportFailure("proxy1.example.com:8080")
				return pool
			},
			check: func(t *testing.T, pool *Pool) {
				// All selects should skip proxy1 and return proxy2
				for i := 0; i < 5; i++ {
					got, err := pool.Select(nil)
					if err != nil {
						t.Fatalf("Select() error: %v", err)
					}
					if got.Host == "proxy1.example.com:8080" {
						t.Errorf("Select() returned circuit-open proxy1 on call %d", i)
					}
					if got.Host != "proxy2.example.com:8080" {
						t.Errorf("Select() call %d: got host %s, want proxy2.example.com:8080", i, got.Host)
					}
				}
			},
		},
		{
			name: "all open returns closest to recovery",
			setup: func(t *testing.T) *Pool {
				pool := newTestPool(t, testPoolOpts{
					proxies:          []string{testProxy1, testProxy2},
					failureThreshold: 1,
					cooldown:         10 * time.Minute,
				})
				// Open both circuits; proxy1 opens first → closest to recovery.
				pool.ReportFailure("proxy1.example.com:8080")
				time.Sleep(5 * time.Millisecond)
				pool.ReportFailure("proxy2.example.com:8080")
				return pool
			},
			check: func(t *testing.T, pool *Pool) {
				got, err := pool.Select(nil)
				if err != nil {
					t.Fatalf("Select() error: %v", err)
				}
				if got.Host != "proxy1.example.com:8080" {
					t.Errorf("Select() with all open: got %s, want proxy1 (earliest open)", got.Host)
				}
			},
		},
		{
			name: "random",
			setup: func(t *testing.T) *Pool {
				return newTestPool(t, testPoolOpts{strategy: StrategyRandom})
			},
			check: func(t *testing.T, pool *Pool) {
				// Over many selections, every proxy should appear at least once
				seen := make(map[string]bool)
				for i := 0; i < 100; i++ {
					got, err := pool.Select(nil)
					if err != nil {
						t.Fatalf("Select() error: %v", err)
					}
					seen[got.Host] = true
				}
				if len(seen) != 3 {
					t.Errorf("random selection visited %d distinct proxies, want 3 (got: %v)", len(seen), seen)
				}
			},
		},
		{
			name: "random all open returns closest to recovery",
			setup: func(t *testing.T) *Pool {
				pool := newTestPool(t, testPoolOpts{
					proxies:          []string{testProxy1, testProxy2},
					strategy:         StrategyRandom,
					failureThreshold: 1,
					cooldown:         10 * time.Minute,
				})
				// Open both circuits — proxy1 first (earlier open = closest to recovery).
				pool.ReportFailure("proxy1.example.com:8080")
				time.Sleep(5 * time.Millisecond)
				pool.ReportFailure("proxy2.example.com:8080")
				return pool
			},
			check: func(t *testing.T, pool *Pool) {
				// With all circuits open, selectRandom falls back to the entry
				// closest to recovery (proxy1, whose openUntil is smaller).
				got, err := pool.Select(nil)
				if err != nil {
					t.Fatalf("Select() error: %v", err)
				}
				if got.Host != "proxy1.example.com:8080" {
					t.Errorf("Select() with all open: got %s, want proxy1 (earliest open)", got.Host)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, tt.setup(t))
		})
	}
}

// TestSelectRandom_AlwaysFindsHealthyEntry verifies the random strategy
// returns the only healthy proxy whenever one exists. The previous bounded
// random probing missed it ~35% of the time (n=10, m=1) and fell back to a
// circuit-open entry.
func TestSelectRandom_AlwaysFindsHealthyEntry(t *testing.T) {
	const n = 10
	proxies := make([]string, 0, n)
	hosts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		proxies = append(proxies, fmt.Sprintf("http://proxy%d.example.com:8080", i))
		hosts = append(hosts, fmt.Sprintf("proxy%d.example.com:8080", i))
	}

	pool := newTestPool(t, testPoolOpts{
		proxies:          proxies,
		strategy:         StrategyRandom,
		failureThreshold: 1, // open on first failure for easy testing
		cooldown:         10 * time.Minute,
	})

	// Open circuits for every proxy except index 7.
	for i, host := range hosts {
		if i != 7 {
			pool.ReportFailure(host)
		}
	}

	for i := 0; i < 100; i++ {
		got, err := pool.Select(nil)
		if err != nil {
			t.Fatalf("Select() error: %v", err)
		}
		if got.Host != hosts[7] {
			t.Fatalf("iteration %d: Select() = %s, want the only healthy proxy %s", i, got.Host, hosts[7])
		}
	}
}

// ============================================================================
// Circuit breaker reporting (table)
// ============================================================================

// TestCircuitBreaker covers ReportFailure/ReportSuccess state transitions:
// threshold counting, reset-on-success, half-open recovery, and unknown-host
// tolerance. Each row was formerly a standalone function.
func TestCircuitBreaker(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) *Pool
		check func(t *testing.T, pool *Pool)
	}{
		{
			name: "threshold not reached",
			setup: func(t *testing.T) *Pool {
				return newTestPool(t, testPoolOpts{
					proxies:          []string{testProxy1, testProxy2},
					failureThreshold: 3,
				})
			},
			check: func(t *testing.T, pool *Pool) {
				// Two failures below threshold — circuit should stay closed
				pool.ReportFailure("proxy1.example.com:8080")
				pool.ReportFailure("proxy1.example.com:8080")

				e := pool.byHost["proxy1.example.com:8080"][0]
				if ou := e.openUntil.Load(); ou != nil {
					t.Errorf("below threshold: openUntil = %v, want nil (closed)", ou)
				}
				if f := e.failures.Load(); f != 2 {
					t.Errorf("failures = %d, want 2", f)
				}
			},
		},
		{
			name: "success resets circuit",
			setup: func(t *testing.T) *Pool {
				return newTestPool(t, testPoolOpts{
					proxies:          []string{testProxy1},
					failureThreshold: 1,
					cooldown:         10 * time.Minute,
				})
			},
			check: func(t *testing.T, pool *Pool) {
				// Trip the breaker, then report success and verify it resets.
				pool.ReportFailure("proxy1.example.com:8080")
				pool.ReportSuccess("proxy1.example.com:8080")

				e := pool.byHost["proxy1.example.com:8080"][0]
				if ou := e.openUntil.Load(); ou != nil {
					t.Errorf("after ReportSuccess, openUntil = %v, want nil", ou)
				}
				if f := e.failures.Load(); f != 0 {
					t.Errorf("after ReportSuccess, failures = %d, want 0", f)
				}
			},
		},
		{
			name: "success between failures resets streak",
			setup: func(t *testing.T) *Pool {
				return newTestPool(t, testPoolOpts{
					proxies:          []string{testProxy1},
					failureThreshold: 3,
				})
			},
			check: func(t *testing.T, pool *Pool) {
				// Two failures, then a success resets the count
				pool.ReportFailure("proxy1.example.com:8080")
				pool.ReportFailure("proxy1.example.com:8080")
				pool.ReportSuccess("proxy1.example.com:8080")
				pool.ReportFailure("proxy1.example.com:8080")

				e := pool.byHost["proxy1.example.com:8080"][0]
				if f := e.failures.Load(); f != 1 {
					t.Errorf("failures = %d, want 1 (success reset count)", f)
				}
				if ou := e.openUntil.Load(); ou != nil {
					t.Errorf("openUntil = %v, want nil (circuit closed)", ou)
				}
			},
		},
		{
			name: "half-open recovery after cooldown",
			setup: func(t *testing.T) *Pool {
				return newTestPool(t, testPoolOpts{
					proxies:          []string{testProxy1, testProxy2},
					failureThreshold: 1,
					cooldown:         50 * time.Millisecond, // short for testing
				})
			},
			check: func(t *testing.T, pool *Pool) {
				// Open proxy1's circuit
				pool.ReportFailure("proxy1.example.com:8080")

				// Immediately: proxy1 is skipped
				got, _ := pool.Select(nil)
				if got.Host == "proxy1.example.com:8080" {
					t.Error("proxy1 should be skipped while circuit open")
				}

				// Wait for cooldown to expire
				time.Sleep(70 * time.Millisecond)

				// Now proxy1 is eligible again (half-open)
				// Under round-robin, eventually proxy1 will be selected
				found := false
				for i := 0; i < 10; i++ {
					got, _ := pool.Select(nil)
					if got.Host == "proxy1.example.com:8080" {
						found = true
						break
					}
				}
				if !found {
					t.Error("proxy1 should be selectable again after cooldown expired")
				}
			},
		},
		{
			name: "unknown host is a no-op",
			setup: func(t *testing.T) *Pool {
				return newTestPool(t, testPoolOpts{proxies: []string{testProxy1}})
			},
			check: func(t *testing.T, pool *Pool) {
				// Should be a no-op, not panic
				pool.ReportFailure("unknown.example.com:9999")
				pool.ReportSuccess("unknown.example.com:9999")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, tt.setup(t))
		})
	}
}

// ============================================================================
// Deterministic selection (table)
// ============================================================================

// TestSelectDeterminism covers NextIndex/SelectIndex: cursor sequencing,
// attempt-indexed determinism, non-interference with the round-robin cursor,
// circuit skipping, and the negative-modulo correction. Each row was formerly
// a standalone function.
func TestSelectDeterminism(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) *Pool
		check func(t *testing.T, pool *Pool)
	}{
		{
			name:  "next index round robin",
			setup: func(t *testing.T) *Pool { return newTestPool(t, testPoolOpts{}) },
			check: func(t *testing.T, pool *Pool) {
				// NextIndex should return sequential indices: 0, 1, 2, 0, 1, ...
				for i := 0; i < 6; i++ {
					expected := i % 3
					got := pool.NextIndex()
					if got != expected {
						t.Errorf("NextIndex() call %d: got %d, want %d", i, got, expected)
					}
				}
			},
		},
		{
			name: "select index deterministic",
			setup: func(t *testing.T) *Pool {
				return newTestPool(t, testPoolOpts{proxies: []string{testProxy1, testProxy2}})
			},
			check: func(t *testing.T, pool *Pool) {
				// SelectIndex with the same argument always returns the same proxy
				for attempt := 0; attempt < 4; attempt++ {
					expectedIdx := attempt % 2
					got := pool.SelectIndex(attempt)
					want := pool.entries[expectedIdx].url
					if got != want {
						t.Errorf("SelectIndex(%d): got %v, want %v", attempt, got, want)
					}
				}
			},
		},
		{
			name: "select index does not advance counter",
			setup: func(t *testing.T) *Pool {
				return newTestPool(t, testPoolOpts{proxies: []string{testProxy1, testProxy2}})
			},
			check: func(t *testing.T, pool *Pool) {
				// Calling SelectIndex should NOT advance the round-robin counter
				_ = pool.SelectIndex(0)
				_ = pool.SelectIndex(1)
				_ = pool.SelectIndex(0)

				// Next Select should still return entries[0] (counter at 0)
				got, _ := pool.Select(nil)
				if got.Host != "proxy1.example.com:8080" {
					t.Errorf("Select() after SelectIndex calls: got %s, want proxy1", got.Host)
				}
			},
		},
		{
			name: "select index skips circuit open",
			setup: func(t *testing.T) *Pool {
				pool := newTestPool(t, testPoolOpts{
					proxies:          []string{testProxy1, testProxy2},
					failureThreshold: 1,
					cooldown:         time.Minute,
				})
				// Open circuit on proxy1
				pool.ReportFailure("proxy1.example.com:8080")
				return pool
			},
			check: func(t *testing.T, pool *Pool) {
				// SelectIndex(0) should skip proxy1 (circuit open) and return proxy2
				got := pool.SelectIndex(0)
				if got.Host != "proxy2.example.com:8080" {
					t.Errorf("SelectIndex(0) with proxy1 circuit open: got %s, want proxy2", got.Host)
				}
			},
		},
		{
			name:  "select index negative attempt",
			setup: func(t *testing.T) *Pool { return newTestPool(t, testPoolOpts{}) },
			check: func(t *testing.T, pool *Pool) {
				// A negative attempt (caller bug upstream of the API contract)
				// must still map into the valid index range — never panic or
				// index out of range.
				byHost := map[string]bool{testProxy1: true, testProxy2: true, testProxy3: true}

				for _, attempt := range []int{-1, -2, -3, -4, -100} {
					got := pool.SelectIndex(attempt)
					if got == nil {
						t.Fatalf("SelectIndex(%d) = nil, want a configured proxy", attempt)
					}
					if !byHost[got.String()] {
						t.Errorf("SelectIndex(%d) = %s, not a configured proxy", attempt, got)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, tt.setup(t))
		})
	}
}

// ============================================================================
// Introspection
// ============================================================================

// TestHosts verifies Hosts returns the canonical dial address of every entry.
func TestHosts(t *testing.T) {
	pool := newTestPool(t, testPoolOpts{proxies: []string{testProxy1, testProxy2}})

	hosts := pool.Hosts()
	if len(hosts) != 2 {
		t.Fatalf("Hosts() returned %d entries, want 2", len(hosts))
	}

	hostSet := make(map[string]bool, len(hosts))
	for _, h := range hosts {
		hostSet[h] = true
	}
	for _, want := range []string{"proxy1.example.com:8080", "proxy2.example.com:8080"} {
		if !hostSet[want] {
			t.Errorf("Hosts() missing %s", want)
		}
	}
}

// TestPortlessProxyCanonicalKeys verifies that pool entries are keyed by the
// canonical dial address (default port appended) rather than the raw url.Host.
// net/http dials canonicalAddr (e.g. "socks5://p" → "p:1080"); before this
// normalization, ReportFailure with the dialed address never matched the
// portless key, so circuit breaking silently never fired for portless proxies.
func TestPortlessProxyCanonicalKeys(t *testing.T) {
	pool := newTestPool(t, testPoolOpts{
		proxies: []string{
			"http://http-proxy.example.com",
			"https://tls-proxy.example.com",
			"socks5://socks-proxy.example.com",
			"http://explicit.example.com:8080",
		},
		failureThreshold: 1,
	})

	want := map[string]bool{
		"http-proxy.example.com:80":    true,
		"tls-proxy.example.com:443":    true,
		"socks-proxy.example.com:1080": true,
		"explicit.example.com:8080":    true,
	}
	for _, h := range pool.Hosts() {
		if !want[h] {
			t.Errorf("unexpected host key %q (want one of the canonical forms)", h)
		}
		delete(want, h)
	}
	if len(want) > 0 {
		t.Errorf("missing canonical host keys: %v", want)
	}

	// Reporting with the address net/http actually dials must circuit-break
	// the portless socks proxy (threshold 1). Assert on the entry's internal
	// circuit state directly — selecting an index would trivially return the
	// earlier healthy http proxy regardless.
	pool.ReportFailure("socks-proxy.example.com:1080")
	entryList := pool.byHost["socks-proxy.example.com:1080"]
	if len(entryList) == 0 {
		t.Fatal("byHost missing canonical key socks-proxy.example.com:1080")
	}
	if ou := entryList[0].openUntil.Load(); ou == nil {
		t.Error("portless socks proxy was not circuit-broken after canonical ReportFailure (openUntil = 0)")
	}
}

// ============================================================================
// Concurrency
// ============================================================================

// TestSelect_Concurrent hammers Select concurrently with circuit reporting.
// Beyond not panicking (run with -race for full verification), the Select
// contract holds under contention: every call returns a non-nil error-free
// URL drawn from the configured entries — Select never fails, and when all
// circuits are open it returns the closest-to-recovery entry rather than nil.
// The post-state of the reported hosts is deterministic: all 100 failures
// land on proxy1 (opening its circuit at threshold 2) and all 100 successes
// keep proxy2 healthy.
func TestSelect_Concurrent(t *testing.T) {
	pool := newTestPool(t, testPoolOpts{failureThreshold: 2})

	const goroutines = 100
	valid := map[string]bool{testProxy1: true, testProxy2: true, testProxy3: true}

	var (
		wg         sync.WaitGroup
		selections atomic.Int64
		badReturns atomic.Int64
	)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u, err := pool.Select(&http.Request{})
			if err != nil || u == nil || !valid[u.String()] {
				badReturns.Add(1)
				return
			}
			selections.Add(1)
			pool.ReportFailure("proxy1.example.com:8080")
			pool.ReportSuccess("proxy2.example.com:8080")
		}()
	}
	wg.Wait()

	if bad := badReturns.Load(); bad != 0 {
		t.Errorf("%d Select calls returned an error, nil, or a non-configured proxy; want none", bad)
	}
	if got := selections.Load(); got != goroutines {
		t.Errorf("successful selections = %d, want %d", got, goroutines)
	}

	// Deterministic post-state of the two reported hosts.
	e1 := pool.byHost["proxy1.example.com:8080"][0]
	if e1 == nil {
		t.Fatal("byHost missing proxy1.example.com:8080")
	}
	if f := e1.failures.Load(); f != goroutines {
		t.Errorf("proxy1 failures = %d, want %d (one report per goroutine)", f, goroutines)
	}
	if ou := e1.openUntil.Load(); ou == nil {
		t.Error("proxy1 circuit should have opened (threshold=2, 100 failures reported)")
	}

	e2 := pool.byHost["proxy2.example.com:8080"][0]
	if e2 == nil {
		t.Fatal("byHost missing proxy2.example.com:8080")
	}
	if f := e2.failures.Load(); f != 0 {
		t.Errorf("proxy2 failures = %d, want 0 (100 successes reset the streak)", f)
	}
	if ou := e2.openUntil.Load(); ou != nil {
		t.Errorf("proxy2 openUntil = %v, want nil (never opened)", ou)
	}
}

// TestNextIndex_RandomStrategyHonored pins that NextIndex (the rotation
// wiring's per-request base draw) honors StrategyRandom. Before the fix it
// always advanced the round-robin cursor, so ProxyRotatePerRequest and
// ProxyRotateOnStatus silently downgraded the random strategy to sequential
// round-robin. Deterministic assertions only: values stay in range, and a
// 50-draw sample over a 2-proxy pool contains both indexes (failure odds
// 2*2^-50 — impossible in practice, so no flake risk).
func TestNextIndex_RandomStrategyHonored(t *testing.T) {
	t.Run("round robin stays sequential", func(t *testing.T) {
		pool := newTestPool(t, testPoolOpts{})
		for i := 0; i < 6; i++ {
			if got := pool.NextIndex(); got != i%3 {
				t.Fatalf("NextIndex() = %d, want %d", got, i%3)
			}
		}
	})

	t.Run("random draws in range and varies", func(t *testing.T) {
		pool := newTestPool(t, testPoolOpts{
			proxies:  []string{testProxy1, testProxy2},
			strategy: StrategyRandom,
		})
		seen0, seen1 := false, false
		for i := 0; i < 50; i++ {
			idx := pool.NextIndex()
			if idx < 0 || idx > 1 {
				t.Fatalf("NextIndex() = %d, want 0 or 1", idx)
			}
			if idx == 0 {
				seen0 = true
			} else {
				seen1 = true
			}
		}
		if !seen0 || !seen1 {
			t.Fatalf("50 random draws produced indexes (0=%v, 1=%v); want both", seen0, seen1)
		}
	})
}

// TestReportFailureWhileOpenDoesNotExtend pins the fixed-cooldown contract:
// a failure that arrives while the circuit is already open (an in-flight dial
// selected before the circuit opened, or the all-open fallback probing) must
// not re-arm openUntil from its own timestamp — under sustained fallback
// traffic, re-arming deferred recovery indefinitely.
func TestReportFailureWhileOpenDoesNotExtend(t *testing.T) {
	pool := newTestPool(t, testPoolOpts{
		proxies:          []string{testProxy1},
		failureThreshold: 1,
		cooldown:         time.Hour,
	})
	const host = "proxy1.example.com:8080"

	pool.ReportFailure(host) // opens the circuit
	e := pool.byHost[host][0]
	opened := e.openUntil.Load()
	if opened == nil {
		t.Fatal("first failure at threshold 1 should open the circuit")
	}

	// Late failures from in-flight dials while the circuit is open.
	for range 3 {
		pool.ReportFailure(host)
	}
	if got := e.openUntil.Load(); got != opened {
		t.Errorf("failure while open re-armed the cooldown: openUntil %v → %v, want unchanged", opened, got)
	}

	// After a success the circuit closes; the NEXT failure (a fresh streak)
	// opens it again. Only non-zero is asserted: the fresh window is
	// now+cooldown, which on a coarse clock can equal the previous window's
	// nanosecond value — a semantically identical fresh Store, not a re-arm.
	pool.ReportSuccess(host)
	if got := e.openUntil.Load(); got != nil {
		t.Fatalf("ReportSuccess should close the circuit, openUntil = %v", got)
	}
	pool.ReportFailure(host)
	if reopened := e.openUntil.Load(); reopened == nil {
		t.Error("fresh failure after a success should re-open the circuit, openUntil = nil")
	}
}

// TestReportConcurrentNoTornState hammers ReportFailure/ReportSuccess from
// concurrent goroutines and then asserts the completed-success boundary: a
// ReportSuccess that has returned must leave the circuit closed (failures=0,
// openUntil=0). Before the reports were serialized, a concurrent
// ReportFailure could interleave its two field writes around a success and
// leave failures=0 with the circuit open — an unobservable-from-dials state
// that excluded a healthy proxy for a full cooldown. Run under -race in CI.
func TestReportConcurrentNoTornState(t *testing.T) {
	pool := newTestPool(t, testPoolOpts{
		proxies:          []string{testProxy1},
		failureThreshold: 3,
		cooldown:         time.Minute,
	})
	const host = "proxy1.example.com:8080"

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				if (w+i)%2 == 0 {
					pool.ReportFailure(host)
				} else {
					pool.ReportSuccess(host)
				}
			}
		}(w)
	}
	wg.Wait()

	// Method-boundary postcondition: the last completed report is a success.
	pool.ReportSuccess(host)
	e := pool.byHost[host][0]
	if f := e.failures.Load(); f != 0 {
		t.Errorf("after completed ReportSuccess, failures = %d, want 0", f)
	}
	if ou := e.openUntil.Load(); ou != nil {
		t.Errorf("after completed ReportSuccess, openUntil = %v, want nil (torn report pair survived)", ou)
	}
}

// TestNewPoolCredentialVariantsDistinct pins the dedup rule: entries are
// collapsed only when host:port AND credentials match. Entries differing
// only in user:pass are distinct configured proxies (per-user egress
// credentials through one gateway are a real rotation pattern) and were
// previously silently collapsed; the circuit-breaker key stays host:port, so
// the variants share one byHost bucket.
func TestNewPoolCredentialVariantsDistinct(t *testing.T) {
	pool, err := New(Config{
		Proxies: []string{
			"http://u:pass1@gw.example.com:8080",
			"http://u:pass2@gw.example.com:8080", // same gateway, other creds: distinct
			"http://gw.example.com:8080",         // no creds: distinct again
			"http://u:pass1@gw.example.com:8080", // verbatim duplicate: collapses
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := pool.Len(); got != 3 {
		t.Errorf("pool.Len() = %d, want 3 (credential variants distinct, verbatim dup collapsed)", got)
	}
	if buckets := len(pool.byHost); buckets != 1 {
		t.Errorf("len(byHost) = %d, want 1 shared host bucket for all variants", buckets)
	}
	if variants := len(pool.byHost["gw.example.com:8080"]); variants != 3 {
		t.Errorf("byHost bucket holds %d entries, want 3", variants)
	}
}
