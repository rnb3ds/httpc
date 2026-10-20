package engine

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
	"github.com/cybergodev/httpc/internal/types"
)

// errForcedFailure is returned by mockTransport for the first failFirst calls,
// enabling deterministic retry tests without goroutine timing. The message uses
// "connection reset by peer" so the error classifier marks it as retryable.
var errForcedFailure = errors.New("connection reset by peer")

// mockTransport is a mock implementation of transportManager for testing.
// It allows controlling the response and error returned by RoundTrip.
type mockTransport struct {
	mu        sync.Mutex
	Response  *http.Response
	Error     error
	Called    bool
	CallCount int
	Requests  []*http.Request

	// failFirst forces the first N RoundTrip calls to return failFirstErr
	// (or errForcedFailure when nil), then falls through to the normal
	// Error/Response path. Useful for deterministic retry tests
	// (fail-then-succeed scenarios).
	failFirst int

	// failFirstErr overrides the error used by the failFirst counter, for
	// tests that need a specific (e.g. non-retryable) failure.
	failFirstErr error

	// Redirect behavior
	RedirectChain []string
}

// newMockTransport creates a new mockTransport with a predefined response.
func newMockTransport(statusCode int, body string) *mockTransport {
	return &mockTransport{
		Response: &http.Response{
			StatusCode: statusCode,
			Status:     http.StatusText(statusCode),
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}
}

// RoundTrip implements transportManager.
func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Called = true
	m.CallCount++
	m.Requests = append(m.Requests, req)

	if m.failFirst > 0 {
		m.failFirst--
		if m.failFirstErr != nil {
			return nil, m.failFirstErr
		}
		return nil, errForcedFailure
	}

	if m.Error != nil {
		return nil, m.Error
	}

	return m.Response, nil
}

// SetRedirectPolicy implements transportManager. The mock carries no redirect
// state (GetRedirectChain was removed from the interface with it); returning
// nil settings matches how executeRequest treats transports without redirect
// bookkeeping.
func (m *mockTransport) SetRedirectPolicy(ctx context.Context, followRedirects bool, maxRedirects int) (context.Context, *redirectSettings) {
	return ctx, nil
}

// Close implements transportManager.
func (m *mockTransport) Close() error {
	return nil
}

// SetError sets the error to return for subsequent requests.
func (m *mockTransport) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Error = err
}

// GetCallCount returns the number of times RoundTrip was called.
func (m *mockTransport) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.CallCount
}

// withMockTransport returns a clientOption that injects a mock transport.
func withMockTransport(mt *mockTransport) clientOption {
	return func(opts *clientOptions) {
		opts.customTransport = mt
	}
}

// clearResponsePools resets all sync.Pool instances used for response processing.
// For use in tests only — ensures a clean state between test cases.
func clearResponsePools() {
	gzipReaderPool = sync.Pool{
		New: func() any {
			// Must mirror the production New: gzip.NewReader on an empty
			// stream returns (nil, io.EOF), and discarding that error
			// silently yields a nil New that disables the pool entirely.
			return new(gzip.Reader)
		},
	}
	flateReaderPool = sync.Pool{
		New: func() any {
			return flate.NewReader(bytes.NewReader(nil))
		},
	}
	bufferPool = sync.Pool{
		New: func() any {
			return bytes.NewBuffer(make([]byte, 0, defaultBufferSize))
		},
	}
	responsePool = sync.Pool{
		New: func() any {
			return &Response{}
		},
	}
	limitReaderPool = sync.Pool{
		New: func() any {
			return &pooledLimitReader{}
		},
	}
}

// clearTransportPools resets all sync.Pool instances used by the transport package.
// For use in tests only.
func clearTransportPools() {
	redirectSettingsPool = sync.Pool{
		New: func() any {
			return &redirectSettings{}
		},
	}
}

// clearURLCache clears the global URL cache to release memory.
// For use in tests only.
func clearURLCache() {
	globalURLCache.clear()
}

// getURLCacheSize returns the current number of entries in the URL cache.
// For use in tests only.
func getURLCacheSize() int {
	return globalURLCache.size()
}

// testConnectionConfig returns a connection config suitable for testing.
func testConnectionConfig() *connection.Config {
	config := connection.DefaultConfig()
	config.AllowPrivateIPs = true
	return config
}

// ============================================================================
// SHARED TEST FIXTURES
// (consolidated here from request_processor_test.go, coverage_test.go,
// response_processor_test.go, retry_test.go, and errors_test.go so every
// test file in the package shares one definition)
// ============================================================================

// testRequestBuilder creates a Request with the given parameters using setters.
// This helper function is used because Request fields are now private.
func testRequestBuilder() *testRequest {
	return &testRequest{}
}

type testRequest struct {
	method          string
	url             string
	headers         map[string]string
	queryParams     map[string]any
	body            any
	timeout         time.Duration
	maxRetries      int
	ctx             context.Context
	cookies         []http.Cookie
	followRedirects *bool
	maxRedirects    *int
}

func (tr *testRequest) Method(v string) *testRequest              { tr.method = v; return tr }
func (tr *testRequest) URL(v string) *testRequest                 { tr.url = v; return tr }
func (tr *testRequest) Headers(v map[string]string) *testRequest  { tr.headers = v; return tr }
func (tr *testRequest) QueryParams(v map[string]any) *testRequest { tr.queryParams = v; return tr }
func (tr *testRequest) Body(v any) *testRequest                   { tr.body = v; return tr }
func (tr *testRequest) Timeout(v time.Duration) *testRequest      { tr.timeout = v; return tr }
func (tr *testRequest) MaxRetries(v int) *testRequest             { tr.maxRetries = v; return tr }
func (tr *testRequest) Context(v context.Context) *testRequest    { tr.ctx = v; return tr }
func (tr *testRequest) Cookies(v []http.Cookie) *testRequest      { tr.cookies = v; return tr }

func (tr *testRequest) Build() *Request {
	req := &Request{}
	req.SetMethod(tr.method)
	req.SetURL(tr.url)
	if tr.headers != nil {
		req.SetHeaders(tr.headers)
	}
	if tr.queryParams != nil {
		req.SetQueryParams(tr.queryParams)
	}
	req.SetBody(tr.body)
	req.SetTimeout(tr.timeout)
	req.SetMaxRetries(tr.maxRetries)
	req.SetContext(tr.ctx)
	req.SetCookies(tr.cookies)
	req.SetFollowRedirects(tr.followRedirects)
	req.SetMaxRedirects(tr.maxRedirects)
	return req
}

// fileDataHelper is a test helper for creating file data.
type fileDataHelper struct {
	Filename    string
	Content     []byte
	ContentType string
}

// formDataHelper creates a *types.FormData for testing.
func formDataHelper(fields map[string]string, files map[string]*fileDataHelper) *types.FormData {
	fd := &types.FormData{
		Fields: fields,
		Files:  make(map[string]*types.FileData),
	}
	for key, fh := range files {
		if fh == nil {
			fd.Files[key] = nil
			continue
		}
		fd.Files[key] = &types.FileData{
			Filename:    fh.Filename,
			Content:     fh.Content,
			ContentType: fh.ContentType,
		}
	}
	return fd
}

// countingReadCloser tracks how many times Close is invoked.
type countingReadCloser struct {
	inner  io.Reader
	closes int
}

func (c *countingReadCloser) Read(p []byte) (int, error) { return c.inner.Read(p) }
func (c *countingReadCloser) Close() error {
	c.closes++
	return nil
}

// testRetryPolicy is a simple retry policy for testing.
type testRetryPolicy struct {
	maxRetries int
	delay      time.Duration
}

func (p *testRetryPolicy) MaxRetries() int                    { return p.maxRetries }
func (p *testRetryPolicy) GetDelay(attempt int) time.Duration { return p.delay }
func (p *testRetryPolicy) ShouldRetry(resp types.ResponseReader, err error, attempt int) bool {
	if err != nil {
		return attempt < p.maxRetries
	}
	if resp != nil && resp.StatusCode() >= 500 {
		return attempt < p.maxRetries
	}
	return false
}

// mockNetError implements net.Error for testing.
type mockNetError struct {
	timeout   bool
	temporary bool
	msg       string
}

func (e *mockNetError) Error() string   { return e.msg }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return e.temporary }

// testCookieJar is a minimal in-memory cookie jar for testing.
// (Moved from coverage_test.go during its dissolution.)
type testCookieJar struct {
	cookies map[string][]*http.Cookie
}

func newTestCookieJar() *testCookieJar {
	return &testCookieJar{
		cookies: make(map[string][]*http.Cookie),
	}
}

func (j *testCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.cookies[u.Host] = append(j.cookies[u.Host], cookies...)
}

func (j *testCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return j.cookies[u.Host]
}
