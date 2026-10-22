package httpc

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// benchModes are the two pool transport modes: the default shared HTTP/1
// transport (forced h1 for multi-proxy pools) and the opt-in per-proxy
// dispatcher with isolated HTTP/2 pools (EnableProxyPoolHTTP2).
var benchModes = []struct {
	name   string
	h2pool bool
}{
	{"h1-shared", false},
	{"h2-per-proxy", true},
}

// runBurst issues n concurrent GETs through client and appends each request's
// latency to latencies.
func runBurst(client Client, url string, n int, latencies *[]time.Duration) error {
	var wg sync.WaitGroup
	errs := make(chan error, n)
	mu := sync.Mutex{} // guards *latencies appends
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			res, err := client.Get(url)
			dur := time.Since(start)
			mu.Lock()
			*latencies = append(*latencies, dur)
			mu.Unlock()
			if err != nil {
				errs <- err
				return
			}
			_ = res.Body()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		return err
	}
	return nil
}

// BenchmarkProxyPoolTransportModes compares the two pool transport modes at
// several concurrency levels against a local h2-capable TLS backend through 4
// CONNECT proxies. Loopback cannot reproduce the WAN RTT each avoided
// handshake would save; tunnels/request is the structural metric (handshakes
// that would each cost ~1 RTT through a real proxy), and p50/p99 show the
// local cost profile.
func BenchmarkProxyPoolTransportModes(b *testing.B) {
	for _, mode := range benchModes {
		for _, conc := range []int{1, 10, 50} {
			b.Run(fmt.Sprintf("%s/conc-%d", mode.name, conc), func(b *testing.B) {
				target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte("ok")) // best-effort bench response
				}))
				target.EnableHTTP2 = true
				target.StartTLS()
				defer target.Close()
				targetAddr := strings.TrimPrefix(target.URL, "https://")

				proxyURLs, tunnelCounts := newCONNECTProxBenches(b, 4, targetAddr)

				cfg := DefaultConfig()
				cfg.Connection.ProxyPool = proxyURLs
				cfg.Connection.EnableProxyPoolHTTP2 = mode.h2pool
				cfg.Security.AllowPrivateIPs = true
				cfg.Security.InsecureSkipVerify = true
				cfg.Timeouts.Request = 30 * time.Second
				cfg.Retry.MaxRetries = 0

				client, err := New(cfg)
				if err != nil {
					b.Fatalf("New() failed: %v", err)
				}
				defer func() { _ = client.Close() }()

				// Warm the pools so the benchmark measures steady state, not
				// first-connection setup.
				if err := runBurst(client, target.URL, conc, &[]time.Duration{}); err != nil {
					b.Fatalf("warmup failed: %v", err)
				}
				baseline := tunnelCounts()

				var latencies []time.Duration
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := runBurst(client, target.URL, conc, &latencies); err != nil {
						b.Fatalf("burst failed: %v", err)
					}
				}
				b.StopTimer()

				reportLatenciesAndTunnels(b, latencies, tunnelCounts(), baseline)
			})
		}
	}
}

// BenchmarkProxyPoolTransportIdleChurn models spiky traffic through proxies
// with short idle timeouts: every burst re-handshakes because the previous
// burst's connections have gone idle and been dropped. The shared h1
// transport re-opens one connection per concurrent miss; the per-proxy h2
// dispatcher re-opens at most one per proxy. This is where the WAN win lives
// (each avoided handshake 鈮?one proxy + TLS RTT), compressed to CPU cost on
// loopback.
func BenchmarkProxyPoolTransportIdleChurn(b *testing.B) {
	const conc = 50
	for _, mode := range benchModes {
		b.Run(mode.name, func(b *testing.B) {
			target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte("ok")) // best-effort bench response
			}))
			target.EnableHTTP2 = true
			target.StartTLS()
			defer target.Close()
			targetAddr := strings.TrimPrefix(target.URL, "https://")

			proxyURLs, tunnelCounts := newCONNECTProxBenches(b, 4, targetAddr)

			cfg := DefaultConfig()
			cfg.Connection.ProxyPool = proxyURLs
			cfg.Connection.EnableProxyPoolHTTP2 = mode.h2pool
			cfg.Timeouts.IdleConn = 20 * time.Millisecond // drop conns between bursts
			cfg.Security.AllowPrivateIPs = true
			cfg.Security.InsecureSkipVerify = true
			cfg.Timeouts.Request = 30 * time.Second
			cfg.Retry.MaxRetries = 0

			client, err := New(cfg)
			if err != nil {
				b.Fatalf("New() failed: %v", err)
			}
			defer func() { _ = client.Close() }()

			baseline := tunnelCounts()
			var latencies []time.Duration
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := runBurst(client, target.URL, conc, &latencies); err != nil {
					b.Fatalf("burst failed: %v", err)
				}
				b.StopTimer()
				time.Sleep(60 * time.Millisecond) // let idle conns expire
				b.StartTimer()
			}
			b.StopTimer()

			reportLatenciesAndTunnels(b, latencies, tunnelCounts(), baseline)
		})
	}
}

// newCONNECTProxBenches is the benchmark twin of newCONNECTProxies (which is
// *testing.T-bound in proxy_pool_integration_test.go).
func newCONNECTProxBenches(b *testing.B, n int, targetAddr string) (proxyURLs []string, counts func() []int64) {
	b.Helper()
	raw := make([]int64, n)
	servers := make([]*httptest.Server, n)
	proxyURLs = make([]string, n)

	for i := range proxyURLs {
		idx := i
		proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodConnect {
				http.Error(w, "only CONNECT supported", http.StatusBadRequest)
				return
			}
			raw[idx]++

			dst, err := net.Dial("tcp", targetAddr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			hj, ok := w.(http.Hijacker)
			if !ok {
				_ = dst.Close()
				http.Error(w, "hijack not supported", http.StatusInternalServerError)
				return
			}
			clientConn, bufrw, err := hj.Hijack()
			if err != nil {
				_ = dst.Close()
				return
			}
			defer func() { _ = clientConn.Close() }()
			defer func() { _ = dst.Close() }()

			_, _ = bufrw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n") // best-effort handshake
			_ = bufrw.Flush()                                                       // best-effort handshake completion

			go func() {
				_, _ = io.Copy(dst, bufrw) // tunnel relay
				_ = dst.Close()
			}()
			_, _ = io.Copy(clientConn, dst) // tunnel relay
		}))
		servers[i] = proxy
		b.Cleanup(proxy.Close)
		proxyURLs[i] = proxy.URL
	}

	return proxyURLs, func() []int64 {
		out := make([]int64, len(raw))
		copy(out, raw)
		return out
	}
}

// reportLatenciesAndTunnels derives the benchmark's custom metrics from the
// collected per-request latencies and per-proxy tunnel counters.
func reportLatenciesAndTunnels(b *testing.B, latencies []time.Duration, after, baseline []int64) {
	if len(latencies) == 0 {
		b.Fatal("no latency samples collected")
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	pick := func(p float64) time.Duration {
		idx := int(float64(len(latencies)-1) * p)
		return latencies[idx]
	}
	b.ReportMetric(float64(pick(0.50))/float64(time.Millisecond), "p50-ms/req")
	b.ReportMetric(float64(pick(0.99))/float64(time.Millisecond), "p99-ms/req")

	var tunnels int64
	for i, c := range after {
		tunnels += c - baseline[i]
	}
	b.ReportMetric(float64(tunnels)/float64(len(latencies)), "tunnels/req")
}
