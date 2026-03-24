// Package testutil provides shared test utilities for the httpc library.
package testutil

import "time"

// Time durations for tests
const (
	TestTimeoutShort      = 100 * time.Millisecond
	TestTimeoutMedium     = 500 * time.Millisecond
	TestTimeoutLong       = 2 * time.Second
	TestRetryDelay        = 10 * time.Millisecond
	TestBackoffFactor     = 2.0
	TestCacheTTL          = 5 * time.Minute
	TestIdleConnTimeout   = 30 * time.Second
	TestSlowServerDelay   = 200 * time.Millisecond
	TestMiddlewareTimeout = 10 * time.Millisecond
)

// Concurrency test parameters
const (
	DefaultNumGoroutines      = 50
	DefaultOpsPerGoroutine    = 20
	HighConcurrencyGoroutines = 100
)

// Response sizes for tests
const (
	SmallResponseBody  = 1024        // 1KB
	MediumResponseBody = 1024 * 100  // 100KB
	LargeResponseBody  = 1024 * 1024 // 1MB
)

// File size constants
const (
	SmallFileSize  = 10240       // 10KB
	MediumFileSize = 1024 * 100  // 100KB
	LargeFileSize  = 1024 * 1024 // 1MB
)
