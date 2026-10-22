package httpc

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
)

// TestRoutedClientsRaceNeverReturnsSelfClosedClient stresses the routed-client
// cache creation path against an external closer. Invariant: a client returned
// by getOrCreateRoutedClient may only be dead at the caller if the test's own
// closer closed it. A dead client the closer never touched can only have been
// closed by the resolution path itself — the getOrCreateRoutedClient loser
// branch previously closed its own client BEFORE checking whether the raced-in
// winner was usable, then returned that same (now closed) client when the
// winner turned out dead.
func TestRoutedClientsRaceNeverReturnsSelfClosedClient(t *testing.T) {
	defer func() { _ = CloseRoutedClients() }() // best-effort cleanup

	const goroutines = 8
	const iterations = 300
	key := routingKey{proxy: "http://race-proxy:8080"}

	// externallyClosed records every client instance the closer has closed.
	// Guarded by mu; the closer records BEFORE closing so a concurrent
	// dead-at-return observation always finds the instance registered.
	var mu sync.Mutex
	externallyClosed := make(map[*clientImpl]bool)

	stop := make(chan struct{})
	var closerDone sync.WaitGroup
	closerDone.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Close cached entries WITHOUT deleting them: a closed entry left
			// in the map is exactly the state that drives creators into the
			// race-resolution branch.
			routedClients.Range(func(_, v any) bool {
				e := v.(*routingClientEntry)
				mu.Lock()
				externallyClosed[e.client] = true
				mu.Unlock()
				_ = e.client.Close()
				return true
			})
			runtime.Gosched()
		}
	})

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for range goroutines {
		wg.Go(func() {
			for j := range iterations {
				c, err := getOrCreateRoutedClient(key)
				if err != nil {
					errs <- fmt.Errorf("getOrCreateRoutedClient: %w", err)
					return
				}
				impl := c.(*clientImpl)
				if impl.engine.IsClosed() {
					mu.Lock()
					known := externallyClosed[impl]
					mu.Unlock()
					if !known {
						errs <- fmt.Errorf("iteration %d: returned a closed client the test never closed (self-closed by the race-resolution path)", j)
						return
					}
				}
			}
		})
	}
	wg.Wait()
	close(stop)
	closerDone.Wait()

	select {
	case err := <-errs:
		t.Error(err)
	default:
	}
}
