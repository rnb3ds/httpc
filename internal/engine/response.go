package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"

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

	resp := &Response{
		StatusCode:    httpResp.StatusCode,
		Status:        httpResp.Status,
		Headers:       httpResp.Header,
		Body:          string(body),
		RawBody:       body,
		ContentLength: httpResp.ContentLength,
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

	body, err := p.readWithContext(reader, httpResp.Request.Context())
	if err != nil {
		if httpResp.Request.Context().Err() != nil {
			return nil, fmt.Errorf("request context canceled during body read: %w", httpResp.Request.Context().Err())
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
			return nil, err
		}

		if p.config.MaxResponseBodySize > 0 && int64(len(result)) >= p.config.MaxResponseBodySize {
			break
		}
	}

	return result, nil
}
