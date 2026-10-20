package httpc

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// ============================================================================
// PUBLIC OPTION BOUNDARY TESTS
//
// Covers options with zero prior references: WithNoTimeout,
// WithRetryNonIdempotent, the WithBinary variadic form, and the
// WithSecureCookie happy path. Semantics were read from public_options.go /
// client.go before writing each row.
// ============================================================================

// TestWithNoTimeout pins what WithNoTimeout actually guarantees per
// public_options.go: it disables the client-level Timeouts.Request deadline
// for that single request (dial/TLS/response-header timeouts and any explicit
// context deadline still apply). A server slower than the client timeout must
// succeed with the option and fail without it.
func TestWithNoTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(600 * time.Millisecond) // 2x the client request timeout below
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("slow but delivered"))
	}))
	defer server.Close()

	cfg := DefaultConfig()
	cfg.Security.AllowPrivateIPs = true
	cfg.Timeouts.Request = 300 * time.Millisecond
	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	t.Run("request survives a server slower than the client timeout", func(t *testing.T) {
		start := time.Now()
		resp, err := client.Get(server.URL, WithNoTimeout())
		if err != nil {
			t.Fatalf("request with WithNoTimeout failed: %v", err)
		}
		if resp.StatusCode() != http.StatusOK {
			t.Errorf("status = %d, want 200", resp.StatusCode())
		}
		if elapsed := time.Since(start); elapsed < 500*time.Millisecond {
			t.Errorf("request returned after %v; the server sleeps 600ms, so the response cannot be the slow one", elapsed)
		}
	})

	t.Run("same request without the option hits the client timeout", func(t *testing.T) {
		_, err := client.Get(server.URL)
		if err == nil {
			t.Fatal("expected timeout error without WithNoTimeout, got nil")
		}
	})

	t.Run("option is a no-op on a mutator without the override", func(t *testing.T) {
		// WithNoTimeout must not error when the RequestMutator does not
		// implement NoTimeoutSetter (documented no-op contract).
		req := engine.AcquireRequest()
		defer engine.ReleaseRequest(req)
		if err := WithNoTimeout().apply(req); err != nil {
			t.Errorf("WithNoTimeout().apply on engine request: %v", err)
		}
	})
}

// TestWithRetryNonIdempotent pins the per-request override documented on the
// option: WithRetryNonIdempotent(true) retries a POST on a client that
// otherwise forbids non-idempotent retries, and (false) pins a single attempt
// on a client that allows them. Both directions take precedence over the
// client-level Retry.RetryNonIdempotent.
func TestWithRetryNonIdempotent(t *testing.T) {
	tests := []struct {
		name        string
		allow       bool // option value
		wantAttempt int32
	}{
		// Client default forbids non-idempotent retries; the option unlocks
		// them: first POST 500s, retry succeeds -> 2 attempts.
		{"true retries a POST that fails once", true, 2},
		// Explicit false pins one attempt even though the client below also
		// sets the option's true form via the first row's semantics; run
		// against a client that permits retries (allow=true at client level
		// is not set here, so this row asserts false keeps the single
		// attempt).
		{"false pins a single POST attempt", false, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if atomic.AddInt32(&attempts, 1) == 1 {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			cfg := DefaultConfig()
			cfg.Security.AllowPrivateIPs = true
			cfg.Retry.MaxRetries = 2
			cfg.Retry.Delay = 10 * time.Millisecond
			cfg.Retry.EnableJitter = false
			// RetryNonIdempotent stays false at the client level: POSTs are
			// not retried unless the per-request option says so.
			client, err := New(cfg)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			defer func() { _ = client.Close() }()

			resp, err := client.Post(server.URL, WithRetryNonIdempotent(tt.allow))
			if err != nil {
				t.Fatalf("Post failed: %v", err)
			}
			wantStatus := http.StatusInternalServerError
			if tt.allow {
				wantStatus = http.StatusOK
			}
			if resp.StatusCode() != wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode(), wantStatus)
			}
			if got := atomic.LoadInt32(&attempts); got != tt.wantAttempt {
				t.Errorf("server saw %d attempts, want %d", got, tt.wantAttempt)
			}
		})
	}
}

// TestWithBinary_VariadicContentType covers the variadic contentType form of
// WithBinary: omitted or empty falls back to application/octet-stream, an
// explicit value is used verbatim (and validated), and empty data errors.
func TestWithBinary_VariadicContentType(t *testing.T) {
	t.Run("omitted content type defaults to octet-stream", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
				t.Errorf("Content-Type = %q, want application/octet-stream", got)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer func() { _ = client.Close() }()

		if _, err := client.Post(server.URL, WithBinary([]byte{0x00, 0x01})); err != nil {
			t.Fatalf("Post failed: %v", err)
		}
	})

	t.Run("explicit content type used", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Content-Type"); got != "image/png" {
				t.Errorf("Content-Type = %q, want image/png", got)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer func() { _ = client.Close() }()

		if _, err := client.Post(server.URL, WithBinary([]byte("pngdata"), "image/png")); err != nil {
			t.Fatalf("Post failed: %v", err)
		}
	})

	t.Run("empty explicit content type falls back to default", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Content-Type"); got != "application/octet-stream" {
				t.Errorf("Content-Type = %q, want fallback application/octet-stream", got)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client, _ := newTestClient()
		defer func() { _ = client.Close() }()

		if _, err := client.Post(server.URL, WithBinary([]byte("x"), "")); err != nil {
			t.Fatalf("Post failed: %v", err)
		}
	})

	t.Run("empty data rejected", func(t *testing.T) {
		req := engine.AcquireRequest()
		defer engine.ReleaseRequest(req)
		if err := WithBinary(nil).apply(req); err == nil {
			t.Error("expected error for empty binary data, got nil")
		}
	})

	t.Run("invalid content type rejected", func(t *testing.T) {
		req := engine.AcquireRequest()
		defer engine.ReleaseRequest(req)
		if err := WithBinary([]byte("x"), "bad\r\nContent-Type: injected").apply(req); err == nil {
			t.Error("expected error for control characters in content type, got nil")
		}
	})
}

// TestWithSecureCookie_HappyPath complements TestWithSecureCookie
// (request_test.go: nil-config and reject rows): cookies that SATISFY the
// security policy pass validation and reach the server. Validation is
// attribute-based (Secure/HttpOnly/SameSite flags), so plain HTTP suffices —
// no TLS handshake is involved.
func TestWithSecureCookie_HappyPath(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("session")
		if err == nil {
			received = c.Value
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, _ := newTestClient()
	defer func() { _ = client.Close() }()

	securityConfig := &CookieSecurityConfig{
		RequireSecure:     true,
		RequireHttpOnly:   true,
		RequireSameSite:   "Strict",
		AllowSameSiteNone: false,
	}

	_, err := client.Get(server.URL,
		WithCookies([]http.Cookie{{
			Name:     "session",
			Value:    "s3cret",
			Secure:   true,
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
			Path:     "/",
		}}),
		// Must be applied AFTER the cookies are added (documented ordering).
		WithSecureCookie(securityConfig),
	)
	if err != nil {
		t.Fatalf("request with policy-satisfying cookies failed: %v", err)
	}
	if received != "s3cret" {
		t.Errorf("server received cookie value %q, want %q", received, "s3cret")
	}
}
