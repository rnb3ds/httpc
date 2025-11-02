package httpc

import (
	"crypto/tls"
	"time"
)

func SecureConfig() *Config {
	return &Config{
		Timeout:             15 * time.Second,
		MaxIdleConns:        20,
		MaxConnsPerHost:     5,
		MinTLSVersion:       tls.VersionTLS12,
		MaxTLSVersion:       tls.VersionTLS13,
		InsecureSkipVerify:  false,
		MaxResponseBodySize: 5 * 1024 * 1024,
		AllowPrivateIPs:     false,
		MaxRetries:          1,
		RetryDelay:          2 * time.Second,
		BackoffFactor:       2.0,
		UserAgent:           "httpc/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     false,
		EnableHTTP2:         true,
		EnableCookies:       false,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			MaxVersion: tls.VersionTLS13,
		},
	}
}

func PerformanceConfig() *Config {
	return &Config{
		Timeout:             60 * time.Second,
		MaxIdleConns:        100,
		MaxConnsPerHost:     20,
		MinTLSVersion:       tls.VersionTLS12,
		MaxTLSVersion:       tls.VersionTLS13,
		InsecureSkipVerify:  false,
		MaxResponseBodySize: 50 * 1024 * 1024,
		AllowPrivateIPs:     false,
		MaxRetries:          3,
		RetryDelay:          500 * time.Millisecond,
		BackoffFactor:       1.5,
		UserAgent:           "httpc/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     true,
		EnableHTTP2:         true,
		EnableCookies:       true,
	}
}

func TestingConfig() *Config {
	return &Config{
		Timeout:             30 * time.Second,
		MaxIdleConns:        10,
		MaxConnsPerHost:     5,
		MinTLSVersion:       tls.VersionTLS10,
		MaxTLSVersion:       tls.VersionTLS13,
		InsecureSkipVerify:  true,
		MaxResponseBodySize: 10 * 1024 * 1024,
		AllowPrivateIPs:     true,
		MaxRetries:          1,
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
}

func MinimalConfig() *Config {
	return &Config{
		Timeout:             30 * time.Second,
		MaxIdleConns:        10,
		MaxConnsPerHost:     2,
		MinTLSVersion:       tls.VersionTLS12,
		MaxTLSVersion:       tls.VersionTLS13,
		InsecureSkipVerify:  false,
		MaxResponseBodySize: 1 * 1024 * 1024,
		AllowPrivateIPs:     false,
		MaxRetries:          0,
		RetryDelay:          0,
		BackoffFactor:       1.0,
		UserAgent:           "httpc/1.0",
		Headers:             make(map[string]string),
		FollowRedirects:     false,
		EnableHTTP2:         false,
		EnableCookies:       false,
	}
}
