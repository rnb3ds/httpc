package engine

import (
	"context"
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// scratchTransport returns a fresh canned response per call so repeated
// benchmark iterations see an independent body.
type scratchTransport struct {
	body []byte
}

func (s *scratchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	h := make(http.Header, 2)
	h.Set("Content-Type", "application/json")
	h.Set("X-Test", "1")
	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        h,
		Body:          io.NopCloser(bytes.NewReader(s.body)),
		ContentLength: int64(len(s.body)),
		Request:       req,
	}, nil
}

func (s *scratchTransport) SetRedirectPolicy(ctx context.Context, follow bool, max int) (context.Context, *redirectSettings) {
	return ctx, nil
}

func (s *scratchTransport) Close() error { return nil }

func newScratchPipelineClient(body string) *Client {
	cfg := &Config{
		Timeout:                 30 * time.Second,
		MaxResponseBodySize:     10 * 1024 * 1024,
		MaxDecompressedBodySize: 100 * 1024 * 1024,
		ValidateURL:             true,
		ValidateHeaders:         true,
	}
	c, err := NewClient(cfg, func(o *clientOptions) { o.customTransport = &scratchTransport{body: []byte(body)} })
	if err != nil {
		panic(err)
	}
	return c
}

func BenchmarkScratch_EnginePipeline(b *testing.B) {
	c := newScratchPipelineClient(`{"ok":true}`)
	defer func() { _ = c.Close() }()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		resp, err := c.Request(b.Context(), "GET", "https://api.example.com/v1/items?a=1&b=2")
		if err != nil {
			b.Fatal(err)
		}
		ReleaseResponse(resp)
	}
}

func BenchmarkScratch_EnginePipeline_LargeBody(b *testing.B) {
	c := newScratchPipelineClient(strings.Repeat("x", 8192))
	defer func() { _ = c.Close() }()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		resp, err := c.Request(b.Context(), "GET", "https://api.example.com/v1/items")
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.RawBody()
		ReleaseResponse(resp)
	}
}
