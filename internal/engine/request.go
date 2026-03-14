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
	"strconv"
	"strings"

	"github.com/cybergodev/httpc/internal/types"
)

// FormData and FileData are now defined in internal/types package.
// Use types.FormData and types.FileData for type checking.

type RequestProcessor struct {
	config *Config
}

func NewRequestProcessor(config *Config) *RequestProcessor {
	return &RequestProcessor{
		config: config,
	}
}

func (p *RequestProcessor) Build(req *Request) (*http.Request, error) {
	if req.Method() == "" {
		req.SetMethod("GET")
	}

	if req.Context() == nil {
		req.SetContext(context.Background())
	}

	parsedURL, err := url.Parse(req.URL())
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if len(req.QueryParams()) > 0 {
		query := parsedURL.Query()
		for key, value := range req.QueryParams() {
			query.Add(key, formatQueryParam(value))
		}
		parsedURL.RawQuery = query.Encode()
	}

	var body io.Reader
	var contentType string

	if req.Body() != nil {
		switch v := req.Body().(type) {
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
			if req.Headers() != nil {
				existingContentType = req.Headers()["Content-Type"]
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

	httpReq, err := http.NewRequest(req.Method(), parsedURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create HTTP request failed: %w", err)
	}

	httpReq = httpReq.WithContext(req.Context())

	if contentType != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", contentType)
	}

	for key, value := range p.config.Headers {
		if httpReq.Header.Get(key) == "" {
			httpReq.Header.Set(key, value)
		}
	}

	for key, value := range req.Headers() {
		httpReq.Header.Set(key, value)
	}

	if httpReq.Header.Get("User-Agent") == "" && p.config.UserAgent != "" {
		httpReq.Header.Set("User-Agent", p.config.UserAgent)
	}

	// Add cookies to the request
	// Note: If EnableCookies is true and a CookieJar is configured,
	// the cookies will be managed by the jar automatically.
	// We still add them here for immediate use in this request.
	cookies := req.Cookies()
	for i := range cookies {
		httpReq.AddCookie(&cookies[i])
	}

	return httpReq, nil
}

// escapeQuotes escapes backslashes and double quotes in filenames per RFC 7578.
// Optimized to use single-pass with strings.Builder for better performance.
func escapeQuotes(s string) string {
	// Fast path: no escapes needed
	var hasEscape bool
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' || s[i] == '"' {
			hasEscape = true
			break
		}
	}
	if !hasEscape {
		return s
	}

	// Slow path: build escaped string
	var b strings.Builder
	b.Grow(len(s) + len(s)/10) // Pre-allocate ~10% extra for escapes

	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString("\\\\")
		case '"':
			b.WriteString("\\\"")
		default:
			b.WriteByte(s[i])
		}
	}

	return b.String()
}

// formatQueryParam converts a value to string for query parameters.
// Optimized to avoid fmt.Sprintf allocations for common types.
func formatQueryParam(v any) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case int32:
		return strconv.FormatInt(int64(val), 10)
	case uint:
		return strconv.FormatUint(uint64(val), 10)
	case uint64:
		return strconv.FormatUint(val, 10)
	case uint32:
		return strconv.FormatUint(uint64(val), 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(val), 'f', -1, 32)
	case bool:
		return strconv.FormatBool(val)
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", val)
	}
}

func isFormData(v any) bool {
	if v == nil {
		return false
	}
	// Check if it's a pointer to types.FormData
	if _, ok := v.(*types.FormData); ok {
		return true
	}
	// Fallback to reflection for compatible types from different packages
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
