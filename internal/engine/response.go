package engine

import (
	"fmt"
	"io"
	"net/http"
)

type ResponseProcessor struct {
	config *Config
}

func NewResponseProcessor(config *Config) *ResponseProcessor {
	return &ResponseProcessor{
		config: config,
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
	maxSize := p.config.MaxResponseBodySize
	if maxSize > 0 {
		reader = io.LimitReader(httpResp.Body, maxSize+1)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if maxSize > 0 && int64(len(body)) > maxSize {
		return nil, fmt.Errorf("response body exceeds limit of %d bytes", maxSize)
	}

	return body, nil
}
