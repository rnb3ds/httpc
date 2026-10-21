package httpc

import (
	"crypto/tls"
	"time"
)

// testConfig returns a configuration suitable for testing with localhost servers.
// SECURITY: This configuration allows private IPs and skips TLS verification.
// DO NOT use in production.
func testConfig() Config {
	return Config{
		Timeouts: TimeoutConfig{
			Request:        60 * time.Second,
			Dial:           10 * time.Second,
			TLSHandshake:   10 * time.Second,
			ResponseHeader: 30 * time.Second,
			IdleConn:       90 * time.Second,
		},
		Connection: ConnectionConfig{
			MaxIdleConns:    200,
			MaxConnsPerHost: 200,
			EnableHTTP2:     false,
			EnableCookies:   true,
		},
		Security: SecurityConfig{
			InsecureSkipVerify:  true,
			MaxResponseBodySize: 10 * 1024 * 1024,
			AllowPrivateIPs:     true,
			FollowRedirects:     true,
			MaxRedirects:        10,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Retry: RetryConfig{
			MaxRetries:    0,
			Delay:         100 * time.Millisecond,
			BackoffFactor: 2.0,
		},
		Middleware: MiddlewareConfig{},
		Defaults: RequestDefaults{
			UserAgent: "httpc-test/1.0",
			Headers:   make(map[string]string),
		},
	}
}

func newTestClient() (Client, error) {
	return New(testConfig())
}

// newBenchmarkClient builds the client used by the benchmarks. It is kept
// separate from newTestClient because the semantics differ where it matters:
// benchmarks measure the DEFAULT configuration's behavior (ValidateURL and
// ValidateHeaders on, InsecureSkipVerify off, HTTP/2 on, 180s request
// timeout) with only two mutations — loopback access for httptest servers
// and retries disabled so a benchmark iteration never silently repeats a
// request. testConfig() disables validation and alters timeouts, which would
// measure a different code path than production defaults.
func newBenchmarkClient() (Client, error) {
	config := DefaultConfig()
	config.Security.AllowPrivateIPs = true
	config.Retry.MaxRetries = 0
	return New(config)
}
