package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
)

type RequestProcessor struct {
	config *Config
}

func NewRequestProcessor(config *Config) *RequestProcessor {
	return &RequestProcessor{
		config: config,
	}
}

func (p *RequestProcessor) Build(req *Request) (*http.Request, error) {
	if req.Method == "" {
		req.Method = "GET"
	}

	if req.Context == nil {
		req.Context = context.Background()
	}

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
						return nil, fmt.Errorf("write form field failed: %w", err)
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
						return nil, fmt.Errorf("create form file failed: %w", err)
					}

					if _, err := part.Write(fileData.Content); err != nil {
						return nil, fmt.Errorf("write file content failed: %w", err)
					}
				}

				if err := writer.Close(); err != nil {
					return nil, fmt.Errorf("close multipart writer failed: %w", err)
				}

				body = &buf
				contentType = writer.FormDataContentType()
			} else {
				existingContentType := ""
				if req.Headers != nil {
					existingContentType = req.Headers["Content-Type"]
				}

				if existingContentType == "application/xml" {
					xmlData, err := xml.Marshal(v)
					if err != nil {
						return nil, fmt.Errorf("marshal XML failed: %w", err)
					}
					body = bytes.NewReader(xmlData)
					contentType = "application/xml"
				} else {
					jsonData, err := json.Marshal(v)
					if err != nil {
						return nil, fmt.Errorf("marshal JSON failed: %w", err)
					}
					body = bytes.NewReader(jsonData)
					contentType = "application/json"
				}
			}
		}
	}

	httpReq, err := http.NewRequest(req.Method, parsedURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create HTTP request failed: %w", err)
	}

	httpReq = httpReq.WithContext(req.Context)

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
	// Note: If EnableCookies is true and a CookieJar is configured,
	// the cookies will be managed by the jar automatically.
	// We still add them here for immediate use in this request.
	for i := range req.Cookies {
		httpReq.AddCookie(&req.Cookies[i])
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

func extractFormData(v any) (*FormDataExtractor, bool) {
	// Try direct type assertion first (more efficient)
	type formDataLike interface {
		GetFields() map[string]string
		GetFiles() map[string]*FileDataExtractor
	}

	// Use JSON marshaling as fallback for compatibility
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
