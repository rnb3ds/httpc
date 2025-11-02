package httpc

import (
	"crypto/tls"
	"time"
)

func newTestClient() (Client, error) {
	config := &Config{
		Timeout:             60 * time.Second,
		MaxIdleConns:        200,
		MaxConnsPerHost:     200,
		InsecureSkipVerify:  true,
		MaxResponseBodySize: 10 * 1024 * 1024,
		AllowPrivateIPs:     true,
		MaxRetries:          0,
		RetryDelay:          100 * time.Millisecond,
		BackoffFactor:       2.0,
		UserAgent:           "httpc-test/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     true,
		EnableHTTP2:         false,
		EnableCookies:       true,
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	return New(config)
}
