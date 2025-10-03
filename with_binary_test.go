package httpc

import (
	"testing"
)

func TestWithBinary(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		contentType []string
		wantCT      string
	}{
		{
			name:        "with specific content type",
			data:        []byte{0x89, 0x50, 0x4E, 0x47},
			contentType: []string{"image/png"},
			wantCT:      "image/png",
		},
		{
			name:        "without content type defaults to octet-stream",
			data:        []byte{0x01, 0x02, 0x03},
			contentType: nil,
			wantCT:      "application/octet-stream",
		},
		{
			name:        "with empty content type defaults to octet-stream",
			data:        []byte{0x01, 0x02, 0x03},
			contentType: []string{""},
			wantCT:      "application/octet-stream",
		},
		{
			name:        "with PDF content type",
			data:        []byte{0x25, 0x50, 0x44, 0x46},
			contentType: []string{"application/pdf"},
			wantCT:      "application/pdf",
		},
		{
			name:        "with audio content type",
			data:        []byte{0xFF, 0xFB, 0x90, 0x00},
			contentType: []string{"audio/mpeg"},
			wantCT:      "audio/mpeg",
		},
		{
			name:        "with video content type",
			data:        []byte{0x00, 0x00, 0x00, 0x18},
			contentType: []string{"video/mp4"},
			wantCT:      "video/mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &Request{}
			var opt RequestOption
			if tt.contentType == nil {
				opt = WithBinary(tt.data)
			} else {
				opt = WithBinary(tt.data, tt.contentType...)
			}
			opt(req)

			// Check body is set correctly
			bodyBytes, ok := req.Body.([]byte)
			if !ok {
				t.Errorf("Body is not []byte type")
				return
			}

			if len(bodyBytes) != len(tt.data) {
				t.Errorf("Body length = %d, want %d", len(bodyBytes), len(tt.data))
			}

			for i, b := range bodyBytes {
				if b != tt.data[i] {
					t.Errorf("Body[%d] = %x, want %x", i, b, tt.data[i])
				}
			}

			// Check Content-Type header
			if req.Headers == nil {
				t.Error("Headers map is nil")
				return
			}

			ct, exists := req.Headers["Content-Type"]
			if !exists {
				t.Error("Content-Type header not set")
				return
			}

			if ct != tt.wantCT {
				t.Errorf("Content-Type = %q, want %q", ct, tt.wantCT)
			}
		})
	}
}

func TestWithBinaryEmptyData(t *testing.T) {
	req := &Request{}
	opt := WithBinary([]byte{})
	opt(req)

	bodyBytes, ok := req.Body.([]byte)
	if !ok {
		t.Error("Body is not []byte type")
		return
	}

	if len(bodyBytes) != 0 {
		t.Errorf("Body length = %d, want 0", len(bodyBytes))
	}

	if req.Headers["Content-Type"] != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want %q", req.Headers["Content-Type"], "application/octet-stream")
	}
}

func TestWithBinaryLargeData(t *testing.T) {
	// Test with 1MB of data
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	req := &Request{}
	opt := WithBinary(largeData)
	opt(req)

	bodyBytes, ok := req.Body.([]byte)
	if !ok {
		t.Error("Body is not []byte type")
		return
	}

	if len(bodyBytes) != len(largeData) {
		t.Errorf("Body length = %d, want %d", len(bodyBytes), len(largeData))
	}
}

func TestWithBinaryPreservesExistingHeaders(t *testing.T) {
	req := &Request{
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
			"Authorization":   "Bearer token",
		},
	}

	data := []byte{0x01, 0x02, 0x03}
	opt := WithBinary(data, "image/png")
	opt(req)

	// Check Content-Type is set
	if req.Headers["Content-Type"] != "image/png" {
		t.Errorf("Content-Type = %q, want %q", req.Headers["Content-Type"], "image/png")
	}

	// Check existing headers are preserved
	if req.Headers["X-Custom-Header"] != "custom-value" {
		t.Error("Existing X-Custom-Header was not preserved")
	}

	if req.Headers["Authorization"] != "Bearer token" {
		t.Error("Existing Authorization header was not preserved")
	}
}

func TestWithBinaryOverwritesContentType(t *testing.T) {
	req := &Request{
		Headers: map[string]string{
			"Content-Type": "text/plain",
		},
	}

	data := []byte{0x89, 0x50, 0x4E, 0x47}
	opt := WithBinary(data, "image/png")
	opt(req)

	// Content-Type should be overwritten
	if req.Headers["Content-Type"] != "image/png" {
		t.Errorf("Content-Type = %q, want %q", req.Headers["Content-Type"], "image/png")
	}
}

func BenchmarkWithBinary(b *testing.B) {
	data := make([]byte, 1024) // 1KB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &Request{}
		opt := WithBinary(data)
		opt(req)
	}
}

func BenchmarkWithBinaryWithContentType(b *testing.B) {
	data := make([]byte, 1024) // 1KB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &Request{}
		opt := WithBinary(data, "application/octet-stream")
		opt(req)
	}
}

func BenchmarkWithBinaryLarge(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &Request{}
		opt := WithBinary(data)
		opt(req)
	}
}
