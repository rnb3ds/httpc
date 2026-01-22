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
	"reflect"
	"strings"
)

type FormData struct {
	Fields map[string]string
	Files  map[string]*FileData
}

type FileData struct {
	Filename    string
	Content     []byte
	ContentType string
}

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
			} else if isFormData(v) {
				var buf bytes.Buffer
				writer := multipart.NewWriter(&buf)

				fieldsVal := reflect.ValueOf(v).Elem().FieldByName("Fields")
				filesVal := reflect.ValueOf(v).Elem().FieldByName("Files")

				if fieldsVal.IsValid() && fieldsVal.Kind() == reflect.Map {
					for _, key := range fieldsVal.MapKeys() {
						value := fieldsVal.MapIndex(key).String()
						if err := writer.WriteField(key.String(), value); err != nil {
							return nil, fmt.Errorf("write form field failed: %w", err)
						}
					}
				}

				if filesVal.IsValid() && filesVal.Kind() == reflect.Map {
					for _, key := range filesVal.MapKeys() {
						fileDataValue := filesVal.MapIndex(key)
						if !fileDataValue.IsValid() || fileDataValue.IsNil() {
							continue
						}
						fileDataElem := fileDataValue.Elem()

						filename := ""
						var content []byte
						contentType := ""

						if f := fileDataElem.FieldByName("Filename"); f.IsValid() && f.Kind() == reflect.String {
							filename = f.String()
						}
						if f := fileDataElem.FieldByName("Content"); f.IsValid() && f.Kind() == reflect.Slice {
							content = f.Bytes()
						}
						if f := fileDataElem.FieldByName("ContentType"); f.IsValid() && f.Kind() == reflect.String {
							contentType = f.String()
						}

						var part io.Writer
						var err error

						if contentType != "" {
							h := make(textproto.MIMEHeader)
							h.Set("Content-Disposition",
								fmt.Sprintf(`form-data; name="%s"; filename="%s"`,
									escapeQuotes(key.String()), escapeQuotes(filename)))
							h.Set("Content-Type", contentType)
							part, err = writer.CreatePart(h)
						} else {
							part, err = writer.CreateFormFile(key.String(), filename)
						}

						if err != nil {
							return nil, fmt.Errorf("create form file failed: %w", err)
						}

						if _, err := part.Write(content); err != nil {
							return nil, fmt.Errorf("write file content failed: %w", err)
						}
					}
				}

				if err := writer.Close(); err != nil {
					return nil, fmt.Errorf("close multipart writer failed: %w", err)
				}

				body = &buf
				contentType = writer.FormDataContentType()
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

func escapeQuotes(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}

func isFormData(v any) bool {
	if v == nil {
		return false
	}
	t := reflect.TypeOf(v)
	if t.Kind() != reflect.Ptr {
		return false
	}
	t = t.Elem()
	if t.Kind() != reflect.Struct {
		return false
	}
	if t.Name() != "FormData" {
		return false
	}
	_, hasFields := t.FieldByName("Fields")
	_, hasFiles := t.FieldByName("Files")
	return hasFields && hasFiles
}
