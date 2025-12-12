package engine

import (
	"compress/flate"
	"compress/gzip"
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

	// Check if response was compressed
	wasCompressed := httpResp.Header.Get("Content-Encoding") != ""

	body, err := p.readBody(httpResp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	contentLength := httpResp.ContentLength

	// Only validate content-length if response was not compressed
	// For compressed responses, Content-Length refers to compressed size, not decompressed
	if !wasCompressed && contentLength > 0 && contentLength != int64(len(body)) {
		isHeadRequest := httpResp.Request != nil && httpResp.Request.Method == "HEAD"
		if !isHeadRequest && p.config.StrictContentLength {
			return nil, fmt.Errorf("content-length mismatch: expected %d, got %d", contentLength, len(body))
		}
	}

	// For compressed responses, update content length to reflect decompressed size
	if wasCompressed {
		contentLength = int64(len(body))
	}

	// Shallow copy headers - they won't be modified
	resp := &Response{
		StatusCode:    httpResp.StatusCode,
		Status:        httpResp.Status,
		Headers:       httpResp.Header,
		Body:          string(body),
		RawBody:       body,
		ContentLength: contentLength,
		Proto:         httpResp.Proto,
		Cookies:       httpResp.Cookies(),
	}

	return resp, nil
}

// readBody reads and optionally decompresses the response body with size limits.
// Optimized for security and performance with comprehensive validation.
func (p *ResponseProcessor) readBody(httpResp *http.Response) ([]byte, error) {
	if httpResp.Body == nil {
		return nil, nil
	}

	maxSize := p.config.MaxResponseBodySize
	var reader io.Reader = httpResp.Body

	// Detect and decompress based on Content-Encoding header
	encoding := httpResp.Header.Get("Content-Encoding")
	if encoding != "" {
		var err error
		reader, err = p.createDecompressor(reader, encoding)
		if err != nil {
			return nil, fmt.Errorf("failed to create decompressor for %s: %w", encoding, err)
		}
	}

	// Apply size limit if configured - critical for security
	if maxSize > 0 {
		// Read one extra byte to detect size violations efficiently
		reader = io.LimitReader(reader, maxSize+1)
	}

	// Read body with error handling
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Security check: enforce size limit strictly
	if maxSize > 0 && int64(len(body)) > maxSize {
		return nil, fmt.Errorf("response body exceeds limit of %d bytes", maxSize)
	}

	return body, nil
}

// createDecompressor creates an appropriate decompressor based on the encoding type.
// Supports gzip and deflate encodings with comprehensive error handling.
// Adheres to zero external dependencies principle.
func (p *ResponseProcessor) createDecompressor(reader io.Reader, encoding string) (io.Reader, error) {
	switch encoding {
	case "gzip":
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		return gzipReader, nil

	case "deflate":
		// flate.NewReader never returns an error
		return flate.NewReader(reader), nil

	case "br":
		// Brotli not supported - would require external dependency
		return nil, fmt.Errorf("brotli decompression not supported (stdlib only, no external dependencies)")

	case "compress", "x-compress":
		// LZW compression not supported in stdlib
		return nil, fmt.Errorf("LZW compression not supported")

	case "identity", "":
		// No compression or explicit identity
		return reader, nil

	default:
		// Unknown encoding - return as-is but log warning
		return reader, nil
	}
}
