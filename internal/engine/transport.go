package engine

import (
	"fmt"
	"net/http"

	"github.com/cybergodev/httpc/internal/connection"
)

// Transport manages HTTP transport with comprehensive security and optimal performance
type Transport struct {
	transport  *http.Transport
	httpClient *http.Client
	config     *Config
}

// NewTransport creates a new transport manager with connection pool
func NewTransport(config *Config, pool *connection.PoolManager) (*Transport, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if pool == nil {
		return nil, fmt.Errorf("connection pool cannot be nil")
	}

	// Use the optimized transport from the connection pool
	transport := pool.GetTransport()

	// Create http.Client with optional cookie jar
	httpClient := &http.Client{
		Transport: transport,
	}

	// Set cookie jar if enabled and provided
	if config.EnableCookies && config.CookieJar != nil {
		if jar, ok := config.CookieJar.(http.CookieJar); ok {
			httpClient.Jar = jar
		}
	}

	// Configure redirect policy
	if !config.FollowRedirects {
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return &Transport{
		transport:  transport,
		httpClient: httpClient,
		config:     config,
	}, nil
}

// RoundTrip executes an HTTP round trip
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("transport round trip failed: %w", err)
	}

	return resp, nil
}

// Close closes the transport and cleans up resources
func (t *Transport) Close() error {
	if t.transport != nil {
		t.transport.CloseIdleConnections()
	}
	return nil
}
