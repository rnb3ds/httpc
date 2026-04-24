package engine

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
)

// mockTransport is a mock implementation of transportManager for testing.
// It allows controlling the response and error returned by RoundTrip.
type mockTransport struct {
	mu        sync.Mutex
	Response  *http.Response
	Error     error
	Called    bool
	CallCount int
	Requests  []*http.Request

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

	if m.Error != nil {
		return nil, m.Error
	}

	return m.Response, nil
}

// SetRedirectPolicy implements transportManager.
func (m *mockTransport) SetRedirectPolicy(ctx context.Context, followRedirects bool, maxRedirects int) (context.Context, *redirectSettings) {
	return ctx, nil
}

// GetRedirectChain implements transportManager.
func (m *mockTransport) GetRedirectChain(ctx context.Context) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	chain := make([]string, len(m.RedirectChain))
	copy(chain, m.RedirectChain)
	return chain
}

// Close implements transportManager.
func (m *mockTransport) Close() error {
	return nil
}

// SetResponse sets the response to return for subsequent requests.
func (m *mockTransport) SetResponse(statusCode int, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Response = &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
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

// GetLastRequest returns the last request made, or nil if none.
func (m *mockTransport) GetLastRequest() *http.Request {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Requests) == 0 {
		return nil
	}
	return m.Requests[len(m.Requests)-1]
}

// Reset clears all recorded state.
func (m *mockTransport) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Called = false
	m.CallCount = 0
	m.Requests = nil
	m.Error = nil
}
