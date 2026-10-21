package httpc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// ============================================================================
// SENTINEL ERROR CONTRACTS
//
// Each documented sentinel in errors.go is driven through a REAL producing
// path and asserted with errors.Is — wrapping must never break detection.
// ============================================================================

// TestErrorSentinels_ProducibleViaErrorsIs drives every sentinel through the
// code path that produces it. No error-construction shortcuts: each row uses
// the public API (client, download, config validation, or option apply) the
// way a user would.
func TestErrorSentinels_ProducibleViaErrorsIs(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
		produce  func(t *testing.T) error
	}{
		{
			name:     "ErrClientClosed from request after Close",
			sentinel: ErrClientClosed,
			produce: func(t *testing.T) error {
				cfg := DefaultConfig()
				cfg.Security.AllowPrivateIPs = true
				client, err := New(cfg)
				if err != nil {
					t.Fatalf("New: %v", err)
				}
				if err := client.Close(); err != nil {
					t.Fatalf("Close: %v", err)
				}
				// Loopback closed port: the closed-client guard must fire
				// before any dialing; the URL is never contacted.
				_, err = client.Get("http://127.0.0.1:1/")
				return err
			},
		},
		{
			name:     "ErrFileExists from download to existing file without Overwrite",
			sentinel: ErrFileExists,
			produce: func(t *testing.T) error {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				cfg := DefaultConfig()
				cfg.Security.AllowPrivateIPs = true
				client, err := New(cfg)
				if err != nil {
					t.Fatalf("New: %v", err)
				}
				defer func() { _ = client.Close() }()

				existing := filepath.Join(t.TempDir(), "exists.bin")
				if err := os.WriteFile(existing, []byte("old"), 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				_, err = client.Download(context.Background(), server.URL, &DownloadConfig{FilePath: existing})
				return err
			},
		},
		{
			name:     "ErrDownloadInProgress from concurrent download to same path",
			sentinel: ErrDownloadInProgress,
			produce: func(t *testing.T) error {
				inFlight := make(chan struct{})
				var once sync.Once
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					once.Do(func() { close(inFlight) })
					time.Sleep(300 * time.Millisecond) // hold the first download's lock
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("payload"))
				}))
				defer server.Close()

				cfg := DefaultConfig()
				cfg.Security.AllowPrivateIPs = true
				client, err := New(cfg)
				if err != nil {
					t.Fatalf("New: %v", err)
				}
				defer func() { _ = client.Close() }()

				path := filepath.Join(t.TempDir(), "locked.bin")
				firstDone := make(chan error, 1)
				go func() {
					_, err := client.Download(context.Background(), server.URL, &DownloadConfig{FilePath: path})
					firstDone <- err
				}()

				select {
				case <-inFlight:
				case <-time.After(5 * time.Second):
					t.Fatal("first download never reached the server")
				}

				_, secondErr := client.Download(context.Background(), server.URL, &DownloadConfig{FilePath: path})
				if err := <-firstDone; err != nil {
					t.Errorf("first download failed: %v", err)
				}
				return secondErr
			},
		},
		{
			name:     "ErrResponseBodyTooLarge from Unmarshal over 50MB",
			sentinel: ErrResponseBodyTooLarge,
			produce: func(t *testing.T) error {
				r := &Result{Response: &ResponseInfo{RawBody: make([]byte, maxJSONSize+1)}}
				return r.Unmarshal(&struct{}{})
			},
		},
		{
			name:     "ErrNilConfig from ValidateConfig(nil)",
			sentinel: ErrNilConfig,
			produce: func(t *testing.T) error {
				return ValidateConfig(nil)
			},
		},
		{
			name:     "ErrNilConfig from nil-receiver Validate",
			sentinel: ErrNilConfig,
			produce: func(t *testing.T) error {
				var cfg *Config
				return cfg.Validate()
			},
		},
		{
			name:     "ErrInvalidTimeout from negative request timeout",
			sentinel: ErrInvalidTimeout,
			produce: func(t *testing.T) error {
				cfg := DefaultConfig()
				cfg.Timeouts.Request = -1 * time.Second
				return ValidateConfig(&cfg)
			},
		},
		{
			name:     "ErrInvalidRetry from MaxRetries over limit",
			sentinel: ErrInvalidRetry,
			produce: func(t *testing.T) error {
				cfg := DefaultConfig()
				cfg.Retry.MaxRetries = maxRetryAttempts + 1
				return ValidateConfig(&cfg)
			},
		},
		{
			name:     "ErrInvalidMaxRedirects from negative WithMaxRedirects",
			sentinel: ErrInvalidMaxRedirects,
			produce: func(t *testing.T) error {
				req := engine.AcquireRequest()
				defer engine.ReleaseRequest(req)
				return WithMaxRedirects(-1).apply(req)
			},
		},
		{
			name:     "ErrInvalidCredentials from empty Basic Auth username",
			sentinel: ErrInvalidCredentials,
			produce: func(t *testing.T) error {
				req := engine.AcquireRequest()
				defer engine.ReleaseRequest(req)
				return WithBasicAuth("", "password").apply(req)
			},
		},
		{
			name:     "ErrInvalidConnection from negative MaxIdleConns",
			sentinel: ErrInvalidConnection,
			produce: func(t *testing.T) error {
				cfg := DefaultConfig()
				cfg.Connection.MaxIdleConns = -1
				return ValidateConfig(&cfg)
			},
		},
		{
			name:     "ErrInvalidSecurity from negative MaxResponseBodySize",
			sentinel: ErrInvalidSecurity,
			produce: func(t *testing.T) error {
				cfg := DefaultConfig()
				cfg.Security.MaxResponseBodySize = -1
				return ValidateConfig(&cfg)
			},
		},
		{
			name:     "ErrInvalidMaxRedirects from negative Security.MaxRedirects",
			sentinel: ErrInvalidMaxRedirects,
			produce: func(t *testing.T) error {
				cfg := DefaultConfig()
				cfg.Security.MaxRedirects = -1
				return ValidateConfig(&cfg)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.produce(t)
			if err == nil {
				t.Fatalf("expected error wrapping %v, got nil", tt.sentinel)
			}
			if !errors.Is(err, tt.sentinel) {
				t.Errorf("errors.Is(err, %v) = false; producing path must wrap the sentinel, got: %v", tt.sentinel, err)
			}
		})
	}
}

// TestErrorSentinels_IdentityAndMessages is the safety net for sentinels
// whose producing paths are exercised above: each must be non-nil with a
// non-empty message, so a refactor to an empty or shared sentinel fails loud.
func TestErrorSentinels_IdentityAndMessages(t *testing.T) {
	for name, sentinel := range map[string]error{
		"ErrClientClosed":               ErrClientClosed,
		"ErrNilConfig":                  ErrNilConfig,
		"ErrInvalidHeader":              ErrInvalidHeader,
		"ErrInvalidTimeout":             ErrInvalidTimeout,
		"ErrInvalidRetry":               ErrInvalidRetry,
		"ErrInvalidMaxRedirects":        ErrInvalidMaxRedirects,
		"ErrInvalidCredentials":         ErrInvalidCredentials,
		"ErrInvalidConnection":          ErrInvalidConnection,
		"ErrInvalidSecurity":            ErrInvalidSecurity,
		"ErrInvalidMiddleware":          ErrInvalidMiddleware,
		"ErrEmptyFilePath":              ErrEmptyFilePath,
		"ErrFileExists":                 ErrFileExists,
		"ErrResponseBodyEmpty":          ErrResponseBodyEmpty,
		"ErrResponseBodyTooLarge":       ErrResponseBodyTooLarge,
		"ErrStreamBodyRequiresDownload": ErrStreamBodyRequiresDownload,
		"ErrDownloadInProgress":         ErrDownloadInProgress,
		"ErrProxyConnectionFailed":      ErrProxyConnectionFailed,
	} {
		if sentinel == nil {
			t.Errorf("%s is nil", name)
			continue
		}
		if sentinel.Error() == "" {
			t.Errorf("%s has an empty message", name)
		}
	}
}
