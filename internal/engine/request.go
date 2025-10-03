package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/cybergodev/httpc/internal/memory"
)

type RequestProcessor struct {
	config        *Config
	memoryManager *memory.Manager
}

func NewRequestProcessor(config *Config, memManager *memory.Manager) *RequestProcessor {
	return &RequestProcessor{
		config:        config,
		memoryManager: memManager,
	}
}

func (p *RequestProcessor) Build(req *Request) (*http.Request, error) {
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if len(req.QueryParams) > 0 {
		query := parsedURL.Query()
		for key, value := range req.QueryParams {
			query.Add(key, fmt.Sprintf("%v", value))
		}
		parsedURL.RawQuery = query.Encode()
	}

	var body io.Reader
	var contentType string

	if req.Body != nil {
		switch v := req.Body.(type) {
		case string:
			body = strings.NewReader(v)
			contentType = "text/plain"
		case []byte:
			body = bytes.NewReader(v)
			contentType = "application/octet-stream"
		case io.Reader:
			body = v
		default:
			if formData, ok := extractFormData(v); ok {
				var buf bytes.Buffer
				writer := multipart.NewWriter(&buf)

				for key, value := range formData.Fields {
					if err := writer.WriteField(key, value); err != nil {
						return nil, fmt.Errorf("failed to write form field: %w", err)
					}
				}

				for fieldName, fileData := range formData.Files {
					var part io.Writer
					var err error

					if fileData.ContentType != "" {
						h := make(textproto.MIMEHeader)
						h.Set("Content-Disposition",
							fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
								escapeQuotes(fieldName), escapeQuotes(fileData.Filename)))
						h.Set("Content-Type", fileData.ContentType)
						part, err = writer.CreatePart(h)
					} else {
						part, err = writer.CreateFormFile(fieldName, fileData.Filename)
					}

					if err != nil {
						return nil, fmt.Errorf("failed to create form file: %w", err)
					}

					if _, err := part.Write(fileData.Content); err != nil {
						return nil, fmt.Errorf("failed to write file content: %w", err)
					}
				}

				if err := writer.Close(); err != nil {
					return nil, fmt.Errorf("failed to close multipart writer: %w", err)
				}

				body = &buf
				contentType = writer.FormDataContentType()
			} else {
				jsonData, err := json.Marshal(v)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal request body: %w", err)
				}
				body = bytes.NewReader(jsonData)
				contentType = "application/json"
			}
		}
	}

	httpReq, err := http.NewRequest(req.Method, parsedURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}

	// Note: We don't create a timeout context here with defer cancel()
	// because that would cancel the context immediately when this function returns.
	// Instead, the timeout context should be created at a higher level
	// (in executeRequest or Request method) where it can live for the duration
	// of the actual HTTP request execution.
	// For now, we just use the context as-is and let the caller manage timeouts.

	httpReq = httpReq.WithContext(ctx)

	if contentType != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", contentType)
	}

	for key, value := range p.config.Headers {
		if httpReq.Header.Get(key) == "" {
			httpReq.Header.Set(key, value)
		}
	}

	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	if httpReq.Header.Get("User-Agent") == "" && p.config.UserAgent != "" {
		httpReq.Header.Set("User-Agent", p.config.UserAgent)
	}

	// Add cookies to the request
	for _, cookie := range req.Cookies {
		httpReq.AddCookie(cookie)
	}

	return httpReq, nil
}

type FormDataExtractor struct {
	Fields map[string]string
	Files  map[string]*FileDataExtractor
}

type FileDataExtractor struct {
	Filename    string
	Content     []byte
	ContentType string
}

func extractFormData(v interface{}) (*FormDataExtractor, bool) {
	jsonData, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}

	var result struct {
		Fields map[string]string `json:"Fields"`
		Files  map[string]struct {
			Filename    string `json:"Filename"`
			Content     []byte `json:"Content"`
			ContentType string `json:"ContentType"`
		} `json:"Files"`
	}

	if err := json.Unmarshal(jsonData, &result); err != nil {
		return nil, false
	}

	if result.Fields == nil && result.Files == nil {
		return nil, false
	}

	extractor := &FormDataExtractor{
		Fields: result.Fields,
		Files:  make(map[string]*FileDataExtractor),
	}

	if extractor.Fields == nil {
		extractor.Fields = make(map[string]string)
	}

	for k, v := range result.Files {
		extractor.Files[k] = &FileDataExtractor{
			Filename:    v.Filename,
			Content:     v.Content,
			ContentType: v.ContentType,
		}
	}

	return extractor, true
}

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

