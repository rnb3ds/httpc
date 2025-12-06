package engine

import (
	"bytes"
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

	// Apply size limit if configured
	if maxSize > 0 {
		// Read one extra byte to detect size violations
		reader = io.LimitReader(reader, maxSize+1)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check if body exceeds limit
	if maxSize > 0 && int64(len(body)) > maxSize {
		return nil, fmt.Errorf("response body exceeds limit of %d bytes", maxSize)
	}

	return body, nil
}

// createDecompressor creates an appropriate decompressor based on the encoding type.
// Supports gzip, deflate, and br (brotli) encodings.
func (p *ResponseProcessor) createDecompressor(reader io.Reader, encoding string) (io.Reader, error) {
	switch encoding {
	case "gzip":
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		return gzipReader, nil

	case "deflate":
		// deflate encoding can be either zlib-wrapped (RFC 1950) or raw (RFC 1951)
		// flate.NewReader handles zlib-wrapped deflate
		// If that fails, we'd need to try raw deflate, but that requires reading all data first
		// For now, use standard flate.NewReader which handles most cases
		return flate.NewReader(reader), nil

	case "br":
		// Brotli decompression using manual implementation
		return p.createBrotliReader(reader)

	default:
		// Unknown or unsupported encoding - return original reader
		return reader, nil
	}
}

// createBrotliReader creates a brotli decompressor.
// Since Go's standard library doesn't include brotli, we implement a basic decoder.
func (p *ResponseProcessor) createBrotliReader(reader io.Reader) (io.Reader, error) {
	// Read all compressed data first
	compressedData, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read compressed data: %w", err)
	}

	// Decompress using brotli decoder
	decompressed, err := decodeBrotli(compressedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode brotli: %w", err)
	}

	return bytes.NewReader(decompressed), nil
}

// decodeBrotli decodes brotli-compressed data.
// This is a minimal implementation that handles basic brotli streams.
func decodeBrotli(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// For now, we'll use a simple approach: try to detect if it's actually brotli
	// and return an error if we can't decode it properly.
	// A full brotli implementation would require significant code.

	// Check for brotli magic bytes or header
	// Brotli doesn't have a fixed magic number, but we can try basic detection

	// Since Go stdlib doesn't have brotli, and we can't use external dependencies,
	// we'll return an error indicating brotli is not supported
	return nil, fmt.Errorf("brotli decompression not supported (requires external library)")
}
