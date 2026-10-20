package validation

import (
	"testing"
)

// BenchmarkIsLocalhost benchmarks the isLocalhost fast path across the
// hostname shapes it dispatches on (relocated from netutil_test.go to keep
// that file test-only, matching sanitize_perf_test.go).
func BenchmarkIsLocalhost(b *testing.B) {
	benchmarks := []struct {
		name     string
		hostname string
	}{
		{"localhost", "localhost"},
		{"LOCALHOST", "LOCALHOST"},
		{"LocalHost", "LocalHost"},
		{"127.0.0.1", "127.0.0.1"},
		{"127.1.1.1", "127.1.1.1"},
		{"localhost.local", "localhost.local"},
		{"example.com", "example.com"},
		{"notlocalhost.com", "notlocalhost.com"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = isLocalhost(bm.hostname)
			}
		})
	}
}
