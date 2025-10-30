package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/cybergodev/httpc/internal/memory"
)

// ResponseProcessor handles HTTP response processing
type ResponseProcessor struct {
	config        *Config
	memoryManager *memory.Manager
}

// NewResponseProcessor creates a new response processor with memory management
func NewResponseProcessor(config *Config, memManager *memory.Manager) *ResponseProcessor {
	return &ResponseProcessor{
		config:        config,
		memoryManager: memManager,
	}
}

// Process converts an HTTP response to our internal response format.
// The caller is responsible for closing httpResp.Body.
func (p *ResponseProcessor) Process(httpResp *http.Response) (*Response, error) {
	if httpResp == nil {
		return nil, fmt.Errorf("HTTP response is nil")
	}

	body, err := p.readBody(httpResp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Handle Content-Length validation
	contentLength := int64(-1) // Default to -1 to indicate no Content-Length header
	if contentLengthHeader := httpResp.Header.Get("Content-Length"); contentLengthHeader != "" {
		contentLength = httpResp.ContentLength

		// Validate content length if header was provided
		if contentLength >= 0 && int64(len(body)) != contentLength {
			// Log the mismatch but don't fail the request
			// In production, you might want to use a proper logger
			// For now, we'll continue with the actual body length
		}
	}

	resp := &Response{
		StatusCode:    httpResp.StatusCode,
		Status:        httpResp.Status,
		Headers:       httpResp.Header,
		Body:          string(body),
		RawBody:       body,
		ContentLength: contentLength,
		Proto:         httpResp.Proto,
		Request:       httpResp.Request,
		Response:      httpResp,
		Cookies:       httpResp.Cookies(),
	}

	return resp, nil
}

func (p *ResponseProcessor) readBody(httpResp *http.Response) ([]byte, error) {
	if httpResp.Body == nil {
		return nil, nil
	}

	var reader io.Reader = httpResp.Body
	if p.config.MaxResponseBodySize > 0 {
		reader = io.LimitReader(httpResp.Body, p.config.MaxResponseBodySize)
	}

	var ctx context.Context
	if httpResp.Request != nil {
		ctx = httpResp.Request.Context()
	} else {
		ctx = context.Background()
	}

	body, err := p.readWithContext(reader, ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("request context canceled during body read: %w", ctx.Err())
		}
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if p.config.MaxResponseBodySize > 0 && int64(len(body)) >= p.config.MaxResponseBodySize {
		return nil, fmt.Errorf("response body too large (limit: %d bytes)", p.config.MaxResponseBodySize)
	}

	return body, nil
}

func (p *ResponseProcessor) readWithContext(reader io.Reader, ctx context.Context) ([]byte, error) {
	const bufferSize = 32 * 1024
	var result []byte
	buffer := make([]byte, bufferSize)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		n, err := reader.Read(buffer)
		if n > 0 {
			result = append(result, buffer[:n]...)
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			// For content length mismatch errors, return what we've read so far
			errMsg := err.Error()
			if strings.Contains(errMsg, "unexpected EOF") ||
				strings.Contains(errMsg, "content length") ||
				strings.Contains(errMsg, "body closed") {
				// Return the data we've successfully read
				return result, nil
			}
			return nil, err
		}

		if p.config.MaxResponseBodySize > 0 && int64(len(result)) >= p.config.MaxResponseBodySize {
			break
		}
	}

	return result, nil
}
