package engine

import (
	"fmt"
	"io"
	"net/http"

	"github.com/cybergodev/httpc/internal/memory"
)

type ResponseProcessor struct {
	config        *Config
	memoryManager *memory.Manager
}

func NewResponseProcessor(config *Config, memManager *memory.Manager) *ResponseProcessor {
	return &ResponseProcessor{
		config:        config,
		memoryManager: memManager,
	}
}

func (p *ResponseProcessor) Process(httpResp *http.Response) (*Response, error) {
	if httpResp == nil {
		return nil, fmt.Errorf("HTTP response is nil")
	}

	body, err := p.readBody(httpResp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	contentLength := httpResp.ContentLength

	if contentLength > 0 && contentLength != int64(len(body)) {
		isHeadRequest := false
		if httpResp.Request != nil && httpResp.Request.Method == "HEAD" {
			isHeadRequest = true
		}

		if !isHeadRequest && p.config.StrictContentLength {
			return nil, fmt.Errorf("content-length mismatch: expected %d bytes, got %d bytes", contentLength, len(body))
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

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if p.config.MaxResponseBodySize > 0 && int64(len(body)) >= p.config.MaxResponseBodySize {
		return nil, fmt.Errorf("response body too large (limit: %d bytes)", p.config.MaxResponseBodySize)
	}

	return body, nil
}
