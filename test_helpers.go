package httpc

import (
	"crypto/tls"
	"time"
)

func newTestClient() (Client, error) {
	config := &Config{
		Timeouts: TimeoutConfig{
			Request: 60 * time.Second,
		},
		Connections: ConnectionConfig{
			MaxIdleConns:    200,
			MaxConnsPerHost: 200,
			EnableHTTP2:     false,
			EnableCookies:   true,
		},
		Security: SecurityConfig{
			InsecureSkipVerify:  true,
			MaxResponseBodySize: 10 * 1024 * 1024,
			AllowPrivateIPs:     true,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Retry: RetryConfig{
			MaxRetries:    0,
			Delay:         100 * time.Millisecond,
			BackoffFactor: 2.0,
		},
		Middleware: MiddlewareConfig{
			UserAgent:       "httpc-test/1.0",
			Headers:         make(map[string]string),
			FollowRedirects: true,
		},
	}

	return New(config)
}
