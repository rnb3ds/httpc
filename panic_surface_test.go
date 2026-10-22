package httpc

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// SEC-003 contract pins (single file, merged from the former panic_safety_test.go):
// the exported surface must not panic on nil receivers, zero-value structs, or
// hostile constructor arguments. These tests are hermetic (no network, no
// servers) — the recover-net behavior for panics that escape INTO the request
// path is pinned separately by security_audit_test.go and middleware_test.go.
// What this file pins is everything
// reachable OUTSIDE those nets, where a panic would reach the caller directly.

// TestNilResultAccessorsDoNotPanic sweeps every Result method on a nil
// receiver. Each accessor is documented nil-safe; the error-returning methods
// must yield their documented sentinel, not just "some error".
func TestNilResultAccessorsDoNotPanic(t *testing.T) {
	var r *Result

	if got := r.Body(); got != "" {
		t.Errorf("nil Body() = %q, want empty", got)
	}
	if got := r.RawBody(); got != nil {
		t.Errorf("nil RawBody() = %v, want nil", got)
	}
	if got := r.StatusCode(); got != 0 {
		t.Errorf("nil StatusCode() = %d, want 0", got)
	}
	if got := r.Proto(); got != "" {
		t.Errorf("nil Proto() = %q, want empty", got)
	}
	if r.IsSuccess() || r.IsRedirect() || r.IsClientError() || r.IsServerError() {
		t.Error("nil status classifiers must all report false")
	}
	if c := r.GetCookie("session"); c != nil {
		t.Errorf("nil GetCookie = %v, want nil", c)
	}
	if r.HasCookie("session") {
		t.Error("nil HasCookie must be false")
	}
	if c := r.GetRequestCookie("session"); c != nil {
		t.Errorf("nil GetRequestCookie = %v, want nil", c)
	}
	if r.HasRequestCookie("session") {
		t.Error("nil HasRequestCookie must be false")
	}
	if err := r.Unmarshal(&struct{}{}); !errors.Is(err, ErrResponseBodyEmpty) {
		t.Errorf("nil Unmarshal err = %v, want ErrResponseBodyEmpty", err)
	}
	if err := r.SaveToFile(t.TempDir() + "/out.bin"); !errors.Is(err, ErrResponseBodyEmpty) {
		t.Errorf("nil SaveToFile err = %v, want ErrResponseBodyEmpty", err)
	}
	if got := r.String(); got != "Result{}" {
		t.Errorf("nil String() = %q, want Result{}", got)
	}
}

// TestZeroValueResultAccessorsDoNotPanic pins the documented "read them on a
// client-returned Result requires a nil-check" stance from the other side: a
// caller who forgets the check and reads a zero-value Result gets safe
// defaults, never a panic.
func TestZeroValueResultAccessorsDoNotPanic(t *testing.T) {
	r := &Result{} // Request/Response/Meta all nil

	if r.IsSuccess() || r.IsRedirect() || r.IsClientError() || r.IsServerError() {
		t.Error("zero-value status classifiers must all report false")
	}
	if err := r.Unmarshal(nil); !errors.Is(err, ErrResponseBodyEmpty) {
		t.Errorf("zero-value Unmarshal(nil) err = %v, want ErrResponseBodyEmpty", err)
	}
	if err := r.SaveToFile(""); !errors.Is(err, ErrResponseBodyEmpty) {
		t.Errorf("zero-value SaveToFile err = %v, want ErrResponseBodyEmpty (nil check precedes path validation)", err)
	}
	if got := r.String(); got != "Result{}" {
		t.Errorf("zero-value String() = %q, want Result{}", got)
	}
	if c := r.GetCookie(""); c != nil {
		t.Errorf("zero-value GetCookie = %v, want nil", c)
	}
}

// TestNilSessionManagerDoesNotPanic pins that every SessionManager method
// tolerates a nil receiver (documented: mis-construction degrades to an empty
// session, and write methods report "session manager is nil").
func TestNilSessionManagerDoesNotPanic(t *testing.T) {
	var s *SessionManager

	s.SetCookieSecurity(nil) // no return; must be silent
	if err := s.SetHeader("k", "v"); err == nil {
		t.Error("nil SetHeader should report an error")
	}
	if err := s.SetHeaders(map[string]string{"k": "v"}); err == nil {
		t.Error("nil SetHeaders should report an error")
	}
	s.DeleteHeader("k")
	s.ClearHeaders()
	if got := s.GetHeaders(); got != nil {
		t.Errorf("nil GetHeaders = %v, want nil", got)
	}
	if err := s.SetCookie(nil); err == nil {
		t.Error("nil SetCookie(nil) should report an error")
	}
	if err := s.SetCookies([]*http.Cookie{nil}); err == nil {
		t.Error("nil SetCookies should report an error")
	}
	s.DeleteCookie("k")
	s.ClearCookies()
	if got := s.GetCookies(); got != nil {
		t.Errorf("nil GetCookies = %v, want nil", got)
	}
	if c := s.GetCookie("k"); c != nil {
		t.Errorf("nil GetCookie = %v, want nil", c)
	}
	s.UpdateFromResult(nil)  // nil result: documented no-op
	s.UpdateFromCookies(nil) // nil slice: documented no-op
}

// TestZeroValueSessionManagerUsable pins the documented zero-value usability:
// "a zero-value SessionManager is also usable — its internal maps initialize
// lazily on the first write" (session.go type doc). Without the lazy
// ensureMaps this would be a nil-map-assignment panic.
func TestZeroValueSessionManagerUsable(t *testing.T) {
	s := &SessionManager{}

	if err := s.SetHeader("X-Test", "v"); err != nil {
		t.Fatalf("zero-value SetHeader failed: %v", err)
	}
	if err := s.SetCookie(&http.Cookie{Name: "a", Value: "b"}); err != nil {
		t.Fatalf("zero-value SetCookie failed: %v", err)
	}
	if len(s.GetHeaders()) != 1 {
		t.Errorf("GetHeaders after one SetHeader = %v, want 1 entry", s.GetHeaders())
	}
	if len(s.GetCookies()) != 1 {
		t.Errorf("GetCookies after one SetCookie = %v, want 1 entry", s.GetCookies())
	}
}

// TestNilDomainClientDoesNotPanic pins the nil-receiver guards on every
// DomainClient entry point: request methods route through checkInit's error,
// accessors return zero values, Close is a no-op.
func TestNilDomainClientDoesNotPanic(t *testing.T) {
	var dc *DomainClient

	if got := dc.URL(); got != "" {
		t.Errorf("nil URL() = %q, want empty", got)
	}
	if got := dc.Domain(); got != "" {
		t.Errorf("nil Domain() = %q, want empty", got)
	}
	if got := dc.Session(); got != nil {
		t.Errorf("nil Session() = %v, want nil", got)
	}
	if err := dc.Close(); err != nil {
		t.Errorf("nil Close() = %v, want nil", err)
	}
	if _, err := dc.Get("/x"); err == nil {
		t.Error("nil Get should report an error, not panic or succeed")
	}
	if _, err := dc.Post("/x", WithJSON(map[string]int{"a": 1})); err == nil {
		t.Error("nil Post should report an error")
	}
	if _, err := dc.Request(context.Background(), "GET", "/x"); err == nil {
		t.Error("nil Request should report an error")
	}
	if _, err := dc.Download(context.Background(), "/x", nil); err == nil {
		t.Error("nil Download should report an error")
	}
}

// TestConstructorHostileInputsDoNotPanic pins the error-returning constructor
// contract on hostile inputs: every fallible constructor reports an error
// instead of panicking (the SEC-003 "After" shape).
func TestConstructorHostileInputsDoNotPanic(t *testing.T) {
	if err := ValidateConfig(nil); !errors.Is(err, ErrNilConfig) {
		t.Errorf("ValidateConfig(nil) = %v, want ErrNilConfig", err)
	}
	if err := SetDefaultClient(nil); err == nil {
		t.Error("SetDefaultClient(nil) should report an error")
	}

	for _, baseURL := range []string{"", "notaurl", "://missing-scheme", "http://"} {
		if _, err := NewDomain(baseURL, DefaultConfig()); err == nil {
			t.Errorf("NewDomain(%q) should report an error", baseURL)
		}
	}

	// Pinner constructors reject empty/invalid input with errors.
	if _, err := NewSPKIHashPinner(); err == nil {
		t.Error("NewSPKIHashPinner() with no hashes should report an error")
	}
	if _, err := NewSPKIHashPinner("not-base64!!", "short"); err == nil {
		t.Error("NewSPKIHashPinner with invalid hashes should report an error")
	}
	if _, err := NewPublicKeyPinner(nil, []byte{}); err == nil {
		t.Error("NewPublicKeyPinner with no valid keys should report an error")
	}

	// Preset constructors must be infallible.
	_ = DefaultConfig()
	_ = SecureConfig()
	_ = PerformanceConfig()
	_ = TestingConfig() // test binary: no production warning fires
	_ = MinimalConfig()
	if DefaultDownloadConfig() == nil {
		t.Fatal("DefaultDownloadConfig() = nil")
	}

	// Nil-tolerant setters.
	SetSecurityWarnOutput(nil) // normalizes to io.Discard
	defer SetSecurityWarnOutput(os.Stderr)

	// Package-level Download rejects a nil config before touching the
	// singleton or the network.
	if _, err := Download(context.Background(), "http://example.invalid", nil); err == nil {
		t.Error("Download with nil config should report an error")
	}
}

// hostileOptions are constructor-time edge cases applied to a real pooled
// engine request: constructors must not panic when built, and their apply
// functions must surface errors (or succeed) — never panic. This exercises
// the apply path without a server: option application happens before any
// transport activity in engine.Request.
func hostileOptions() []RequestOption {
	return []RequestOption{
		nil, // skipped by design (panic_safety_test.go pins the wire path)
		WithHeader("", ""),
		WithHeaderMap(nil),
		WithUserAgent(""),
		WithBasicAuth("", ""),
		WithBearerToken(""),
		WithQuery("", nil),
		WithQueryMap(nil),
		WithJSON(nil),
		WithXML(nil),
		WithBody(nil),
		WithBody(nil, BodyMultipart), // kind/body mismatch: error, not panic
		WithBody("x", BodyForm),      // form wants map/url.Values: error, not panic
		WithForm(nil),
		WithFormData(nil),
		WithFile("", "", nil),
		WithBinary(nil),
		WithBinary(nil, ""),
		WithCookies(nil),
		WithCookieMap(nil),
		WithCookieString(""),
		WithCookieString("="),   // malformed: error deferred to apply
		WithCookieString(";;;"), // only separators: no-op
		WithTimeout(-time.Hour),
		WithMaxRetries(-5),
		WithMaxRedirects(-1),
		WithFollowRedirects(false),
		WithAllowPrivateIPs(true),
		WithStreamBody(true),
		WithNoTimeout(),
		WithSecureCookie(nil),
		WithOnRequest(nil),
		WithOnResponse(nil),
		WithContext(context.Background()),
		WithProxy(""),          // empty routing option
		WithTLSFingerprint(""), // empty preset
		WithSystemProxy(),
	}
}

// TestHostileOptionApplyDoesNotPanic runs every hostile option's apply against
// a real engine request. The Request recover net (clientImpl.Request) would
// convert a panic to an error anyway, but option application also happens in
// the facade's middleware path and DomainClient capture BEFORE the net covers
// user code — an apply-time panic outside Request would be a direct public-API
// panic. This pins the lower guarantee: apply itself never panics.
func TestHostileOptionApplyDoesNotPanic(t *testing.T) {
	for _, opt := range hostileOptions() {
		if opt == nil {
			continue
		}
		req := engine.AcquireRequest()
		// Errors are fine (validation working); panics are not.
		_ = opt.apply(req)
		engine.ReleaseRequest(req)
	}
}

// TestFormatHelpersHostileInputs pins the pure formatting helpers: negative,
// zero, and extreme values must format without panicking (float division in
// calculateSpeed-adjacent code never panics, but these are exported surface).
func TestFormatHelpersHostileInputs(t *testing.T) {
	for _, v := range []int64{-1, 0, 1, 512, 1023, 1024, 1 << 40, math.MaxInt64} {
		if FormatBytes(v) == "" {
			t.Errorf("FormatBytes(%d) returned empty", v)
		}
		if FormatSpeed(float64(v)) == "" {
			t.Errorf("FormatSpeed(%d) returned empty", v)
		}
	}
	if FormatBytes(-1) == "" || FormatSpeed(math.SmallestNonzeroFloat64) == "" || FormatSpeed(math.MaxFloat64) == "" {
		t.Error("extreme formatting inputs must still produce output")
	}
}

// TestAuditEventMarshalNilError pins AuditEvent.MarshalJSON with a nil Error
// and empty chains: the method is called by user serializers and must not
// panic on the zero-value event.
func TestAuditEventMarshalNilError(t *testing.T) {
	b, err := (AuditEvent{}).MarshalJSON()
	if err != nil {
		t.Fatalf("AuditEvent{}.MarshalJSON() error: %v", err)
	}
	if len(b) == 0 {
		t.Error("AuditEvent{}.MarshalJSON() returned empty bytes")
	}
}

// TestNewWithNilMiddlewareEntry pins that a nil entry in
// Config.Middleware.Middlewares does not panic New. The chain is built inside
// New (buildMiddlewareChain -> Chain); before the nil-skip in Chain, a nil
// MiddlewareFunc crashed the constructor with a nil-function-call panic — a
// public API panicking directly, outside every recover safety net.
func TestNewWithNilMiddlewareEntry(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Middleware.Middlewares = []MiddlewareFunc{nil, HeaderMiddleware(nil), nil}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New() with nil middleware entries failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // empty 200 is enough; body is irrelevant here
	}))
	defer srv.Close()

	result, err := client.Get(srv.URL, WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatalf("Get through the nil-tolerant chain failed: %v", err)
	}
	if result.StatusCode() != http.StatusOK {
		t.Errorf("status = %d, want 200", result.StatusCode())
	}
}

// TestChainSkipsNilMiddleware covers direct Chain use: nil entries must be
// skipped, live entries must still run in order.
func TestChainSkipsNilMiddleware(t *testing.T) {
	order := []string{}

	tag := func(name string) MiddlewareFunc {
		return func(next Handler) Handler {
			return func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
				order = append(order, name)
				return next(ctx, req)
			}
		}
	}

	final := Handler(func(ctx context.Context, req RequestMutator) (ResponseMutator, error) {
		order = append(order, "final")
		return nil, nil
	})

	chained := Chain(nil, tag("a"), nil, tag("b"), nil)(final)
	if _, err := chained(context.Background(), nil); err != nil {
		t.Fatalf("chained handler returned error: %v", err)
	}

	want := []string{"a", "b", "final"}
	if len(order) != len(want) {
		t.Fatalf("execution order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("execution order = %v, want %v", order, want)
		}
	}
}

// TestRequestNilOptionIsSkipped pins that a nil RequestOption interface value
// is skipped without a panic: toEngineOptions drops nil entries by design, so
// the request proceeds with the remaining options.
func TestRequestNilOptionIsSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() failed: %v", err)
	}
	defer func() { _ = client.Close() }()

	var nilOption RequestOption // typed nil interface
	result, err := client.Get(srv.URL, nilOption, WithAllowPrivateIPs(true))
	if err != nil {
		t.Fatalf("Get with a nil option failed: %v", err)
	}
	if result.StatusCode() != http.StatusOK {
		t.Errorf("status = %d, want 200", result.StatusCode())
	}
}
