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

	wasCompressed := httpResp.Header.Get("Content-Encoding") != ""

	body, err := p.readBody(httpResp)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	contentLength := httpResp.ContentLength
	if !wasCompressed && p.config.StrictContentLength && contentLength > 0 && contentLength != int64(len(body)) {
		if httpResp.Request == nil || httpResp.Request.Method != "HEAD" {
			return nil, fmt.Errorf("content-length mismatch: expected %d, got %d", contentLength, len(body))
		}
	}

	if wasCompressed {
		contentLength = int64(len(body))
	}

	return &Response{
		StatusCode:    httpResp.StatusCode,
		Status:        httpResp.Status,
		Headers:       httpResp.Header,
		Body:          string(body),
		RawBody:       body,
		ContentLength: contentLength,
		Proto:         httpResp.Proto,
		Cookies:       httpResp.Cookies(),
	}, nil
}

// readBody reads and optionally decompresses the response body with size limits.
func (p *ResponseProcessor) readBody(httpResp *http.Response) ([]byte, error) {
	if httpResp.Body == nil {
		return nil, nil
	}

	reader := io.Reader(httpResp.Body)

	if encoding := httpResp.Header.Get("Content-Encoding"); encoding != "" {
		var err error
		reader, err = p.createDecompressor(reader, encoding)
		if err != nil {
			return nil, fmt.Errorf("failed to create decompressor for %s: %w", encoding, err)
		}
	}

	if maxSize := p.config.MaxResponseBodySize; maxSize > 0 {
		reader = io.LimitReader(reader, maxSize+1)
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if maxSize := p.config.MaxResponseBodySize; maxSize > 0 && int64(len(body)) > maxSize {
		return nil, fmt.Errorf("response body exceeds limit of %d bytes", maxSize)
	}

	return body, nil
}

// createDecompressor creates an appropriate decompressor based on the encoding type.
func (p *ResponseProcessor) createDecompressor(reader io.Reader, encoding string) (io.Reader, error) {
	switch encoding {
	case "gzip":
		return gzip.NewReader(reader)
	case "deflate":
		return flate.NewReader(reader), nil
	case "br":
		return nil, fmt.Errorf("brotli decompression not supported")
	case "compress", "x-compress":
		return nil, fmt.Errorf("LZW compression not supported")
	case "identity", "":
		return reader, nil
	default:
		return reader, nil
	}
}
