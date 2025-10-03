package pool

import (
	"strings"
	"sync"
)

// RequestPool manages reusable request objects to reduce allocations
type RequestPool struct {
	pool sync.Pool
}

// Get retrieves a request object from the pool
func (p *RequestPool) Get() *PooledRequest {
	return p.pool.Get().(*PooledRequest)
}

// Put returns a request object to the pool after resetting it
func (p *RequestPool) Put(req *PooledRequest) {
	req.Reset()
	p.pool.Put(req)
}

// PooledRequest represents a reusable request object
type PooledRequest struct {
	Method      string
	URL         string
	Headers     map[string]string
	QueryParams map[string]any
	Body        any
	Timeout     int64 // Use int64 for better performance
	MaxRetries  int
	Context     interface{} // context.Context

	// Authentication
	BasicAuth   *BasicAuth
	BearerToken string
}

// Reset clears the request object for reuse
func (r *PooledRequest) Reset() {
	r.Method = ""
	r.URL = ""
	r.Body = nil
	r.Timeout = 0
	r.MaxRetries = 0
	r.Context = nil
	r.BasicAuth = nil
	r.BearerToken = ""

	// Clear maps but keep capacity
	for k := range r.Headers {
		delete(r.Headers, k)
	}
	for k := range r.QueryParams {
		delete(r.QueryParams, k)
	}
}

// BasicAuth contains basic authentication credentials
type BasicAuth struct {
	Username string
	Password string
}

// BufferPool manages reusable byte buffers for response body reading
type BufferPool struct {
	pool sync.Pool
}

// Get retrieves a buffer from the pool
func (p *BufferPool) Get() []byte {
	return p.pool.Get().([]byte)
}

// Put returns a buffer to the pool
func (p *BufferPool) Put(buf []byte) {
	// Only return buffers of expected size to maintain pool efficiency
	if cap(buf) >= 32*1024 && cap(buf) <= 64*1024 {
		p.pool.Put(buf[:cap(buf)])
	}
}

// ResponsePool manages reusable response objects
type ResponsePool struct {
	pool sync.Pool
}

// Get retrieves a response object from the pool
func (p *ResponsePool) Get() *PooledResponse {
	return p.pool.Get().(*PooledResponse)
}

// Put returns a response object to the pool after resetting it
func (p *ResponsePool) Put(resp *PooledResponse) {
	resp.Reset()
	p.pool.Put(resp)
}

// PooledResponse represents a reusable response object
type PooledResponse struct {
	StatusCode    int
	Status        string
	Headers       map[string][]string
	Body          string
	RawBody       []byte
	ContentLength int64
	Proto         string
	Duration      int64 // nanoseconds for better performance
	Attempts      int
	Request       interface{} // *http.Request
	Response      interface{} // *http.Response
}

// Reset clears the response object for reuse
func (r *PooledResponse) Reset() {
	r.StatusCode = 0
	r.Status = ""
	r.Headers = nil
	r.Body = ""
	r.RawBody = nil
	r.ContentLength = 0
	r.Proto = ""
	r.Duration = 0
	r.Attempts = 0
	r.Request = nil
	r.Response = nil
}

// StringBuilderPool manages reusable string builders for efficient string operations
type StringBuilderPool struct {
	pool sync.Pool
}

// Get retrieves a string builder from the pool
func (p *StringBuilderPool) Get() *strings.Builder {
	return p.pool.Get().(*strings.Builder)
}

// Put returns a string builder to the pool after resetting it
func (p *StringBuilderPool) Put(sb *strings.Builder) {
	sb.Reset()
	p.pool.Put(sb)
}
