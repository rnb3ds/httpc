package httpc

import (
	"errors"
	"fmt"
)

var (
	// ErrClientClosed is returned when attempting to use a closed client.
	// This occurs after calling Close() on a client instance.
	ErrClientClosed = errors.New("client is closed")

	// ErrNilConfig is returned when a nil configuration is provided.
	// Always provide a valid Config or use DefaultConfig().
	ErrNilConfig = errors.New("config cannot be nil")

	// ErrInvalidURL is returned when URL validation fails.
	// URLs must have a valid scheme (http/https) and host.
	ErrInvalidURL = errors.New("invalid URL")

	// ErrInvalidHeader is returned when header validation fails.
	// Headers must not contain control characters or exceed size limits.
	ErrInvalidHeader = errors.New("invalid header")

	// ErrInvalidTimeout is returned when timeout is negative or exceeds limits.
	// Timeout must be between 0 and 30 minutes.
	ErrInvalidTimeout = errors.New("invalid timeout")

	// ErrInvalidRetry is returned when retry configuration is invalid.
	// MaxRetries must be 0-10, BackoffFactor must be 1.0-10.0.
	ErrInvalidRetry = errors.New("invalid retry configuration")

	// ErrEmptyFilePath is returned when file path is empty.
	// Provide a valid file path for download operations.
	ErrEmptyFilePath = errors.New("file path cannot be empty")

	// ErrFileExists is returned when file already exists and Overwrite is false.
	// Set Overwrite=true or ResumeDownload=true in DownloadOptions.
	ErrFileExists = errors.New("file already exists")

	// ErrResponseBodyEmpty is returned when attempting to parse empty response body.
	// Check response.RawBody before calling JSON() or other parsing methods.
	ErrResponseBodyEmpty = errors.New("response body is empty")

	// ErrResponseBodyTooLarge is returned when response body exceeds size limit.
	// Increase MaxResponseBodySize in Config or reduce response size.
	ErrResponseBodyTooLarge = errors.New("response body too large")
)

// HTTPError represents an HTTP error response
type HTTPError struct {
	StatusCode int
	Status     string
	URL        string
	Method     string
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s %s", e.StatusCode, e.Method, e.URL)
}
