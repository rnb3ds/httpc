package dns

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestDoHResolver_ClearCacheCounterInvariant pins the counter-pairing
// invariant after the recount removal: every cacheSize mutation is paired
// with exactly one successful map mutation, so after all mutators quiesce the
// counter must equal the true map size. The removed "reconcile" Store could
// overwrite the accurate value with a count taken while concurrent
// evictions were still decrementing — permanently over-counting the cache
// (at maxDoHCacheSize every admission then took the evict path, churning
// fresh entries).
func TestDoHResolver_ClearCacheCounterInvariant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Status":0,"Answer":[{"name":"x.test","type":1,"data":"9.9.9.9"}]}`))
	}))
	defer server.Close()

	// Tiny TTL keeps the read-path expiry deletions firing throughout, so
	// ClearCache's scan genuinely interleaves with decrements.
	resolver := NewDoHResolver([]*dohProvider{
		{Name: "local", Template: server.URL + "/dns-query?name={name}&type=A", Priority: 1},
	}, 2*time.Millisecond)
	defer func() { _ = resolver.Close() }()
	ctx := context.Background()

	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				_, _ = resolver.LookupIPAddr(ctx, fmt.Sprintf("host%d.test", (w+i)%8))
			}
		}(w)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			resolver.ClearCache()
			time.Sleep(time.Millisecond)
		}
	}()
	wg.Wait()

	// Quiesced: counter must equal the true map size.
	count := int64(0)
	resolver.cache.Range(func(_, _ any) bool { count++; return true })
	if got := resolver.cacheSize.Load(); got != count {
		t.Errorf("cacheSize = %d, want %d (true map size) after quiesce", got, count)
	}
}
