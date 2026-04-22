package engine

import (
	"context"
	"net/http"
	"sync"
)

// MockTransport is a mock implementation of TransportManager for testing.
// It allows controlling the response and error returned by RoundTrip.
type MockTransport struct {
	mu        sync.Mutex
	Response  *http.Response
	Error     error
	Called    bool
	CallCount int
	Requests  []*http.Request

	// Redirect behavior
	RedirectChain []string
}

// NewMockTransport creates a new MockTransport with a predefined response.
func NewMockTransport(statusCode int, body string) *MockTransport {
	return &MockTransport{
		Response: &http.Response{
			StatusCode: statusCode,
			Status:     http.StatusText(statusCode),
			Header:     make(http.Header),
			Body:       http.NoBody,
		},
	}
}

// RoundTrip implements TransportManager.
func (m *MockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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

// SetRedirectPolicy implements TransportManager.
func (m *MockTransport) SetRedirectPolicy(ctx context.Context, followRedirects bool, maxRedirects int) (context.Context, func()) {
	return ctx, func() {}
}

// GetRedirectChain implements TransportManager.
func (m *MockTransport) GetRedirectChain(ctx context.Context) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	chain := make([]string, len(m.RedirectChain))
	copy(chain, m.RedirectChain)
	return chain
}

// Close implements TransportManager.
func (m *MockTransport) Close() error {
	return nil
}

// SetResponse sets the response to return for subsequent requests.
func (m *MockTransport) SetResponse(statusCode int, body string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Response = &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     make(http.Header),
		Body:       http.NoBody,
	}
}

// SetError sets the error to return for subsequent requests.
func (m *MockTransport) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Error = err
}

// GetCallCount returns the number of times RoundTrip was called.
func (m *MockTransport) GetCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.CallCount
}

// GetLastRequest returns the last request made, or nil if none.
func (m *MockTransport) GetLastRequest() *http.Request {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.Requests) == 0 {
		return nil
	}
	return m.Requests[len(m.Requests)-1]
}

// Reset clears all recorded state.
func (m *MockTransport) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Called = false
	m.CallCount = 0
	m.Requests = nil
	m.Error = nil
}
