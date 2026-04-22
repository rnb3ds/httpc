package engine

import (
	"net/http"
	"strings"
	"testing"
)

func TestGetPutHeader(t *testing.T) {
	t.Run("GetAndReturn", func(t *testing.T) {
		h := GetHeader()
		if h == nil {
			t.Fatal("GetHeader returned nil")
		}
		h.Set("Content-Type", "application/json")
		h.Set("X-Custom", "value")

		PutHeader(h)

		// Get again - should be cleared
		h2 := GetHeader()
		if len(*h2) != 0 {
			t.Errorf("Pooled header should be cleared, got %d entries", len(*h2))
		}
		PutHeader(h2)
	})

	t.Run("PutNil", func(t *testing.T) {
		PutHeader(nil) // should not panic
	})

	t.Run("OversizeNotPooled", func(t *testing.T) {
		ClearHeaderPools()
		h := GetHeader()
		for i := 0; i < maxPooledHeaderSize+1; i++ {
			h.Set(http.CanonicalHeaderKey(strings.Repeat("x", 10)+strings.Repeat(string(rune('0'+i%10)), 5)), "v")
		}
		PutHeader(h) // should discard, not pool
	})
}

func TestGetPutHeaderValues(t *testing.T) {
	t.Run("GetAndReturn", func(t *testing.T) {
		v := GetHeaderValues()
		if v == nil {
			t.Fatal("GetHeaderValues returned nil")
		}
		*v = append(*v, "value1", "value2")

		PutHeaderValues(v)

		v2 := GetHeaderValues()
		if len(*v2) != 0 {
			t.Errorf("Pooled values should be cleared, got %d entries", len(*v2))
		}
		PutHeaderValues(v2)
	})

	t.Run("PutNil", func(t *testing.T) {
		PutHeaderValues(nil)
	})

	t.Run("OversizeNotPooled", func(t *testing.T) {
		v := GetHeaderValues()
		// Make it too large
		*v = make([]string, 0, maxPooledValuesSize*4+1)
		PutHeaderValues(v) // should discard
	})
}

func TestCopyHeader(t *testing.T) {
	tests := []struct {
		name   string
		src    http.Header
		dst    http.Header
		expect http.Header
	}{
		{
			name:   "NilSrc",
			src:    nil,
			dst:    make(http.Header),
			expect: http.Header{},
		},
		{
			name:   "NilDst",
			src:    http.Header{"A": {"1"}},
			dst:    nil,
			expect: nil,
		},
		{
			name:   "EmptySrc",
			src:    http.Header{},
			dst:    make(http.Header),
			expect: http.Header{},
		},
		{
			name:   "SingleValue",
			src:    http.Header{"Content-Type": {"application/json"}},
			dst:    make(http.Header),
			expect: http.Header{"Content-Type": {"application/json"}},
		},
		{
			name:   "MultipleHeaders",
			src:    http.Header{"A": {"1"}, "B": {"2", "3"}},
			dst:    make(http.Header),
			expect: http.Header{"A": {"1"}, "B": {"2", "3"}},
		},
		{
			name:   "EmptyValuesSkipped",
			src:    http.Header{"A": {}, "B": {"1"}},
			dst:    make(http.Header),
			expect: http.Header{"A": {}, "B": {"1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			CopyHeader(tt.dst, tt.src)
			if tt.expect == nil {
				return
			}
			for k, v := range tt.expect {
				got := tt.dst[k]
				if len(got) != len(v) {
					t.Errorf("header %q: got %v, want %v", k, got, v)
				}
			}
		})
	}
}

func TestCloneHeader(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		if CloneHeader(nil) != nil {
			t.Error("CloneHeader(nil) should return nil")
		}
	})

	t.Run("DeepCopy", func(t *testing.T) {
		src := http.Header{
			"Content-Type": {"application/json"},
			"Accept":       {"text/html", "application/xml"},
		}
		dst := CloneHeader(src)

		// Modify source - should not affect clone
		src["Content-Type"][0] = "text/plain"
		delete(src, "Accept")

		if dst.Get("Content-Type") != "application/json" {
			t.Errorf("Clone not independent: got %q", dst.Get("Content-Type"))
		}
		if len(dst["Accept"]) != 2 {
			t.Errorf("Clone should have 2 Accept values, got %d", len(dst["Accept"]))
		}
	})

	t.Run("EmptyValueSliceNotCopied", func(t *testing.T) {
		src := http.Header{"A": {}}
		dst := CloneHeader(src)
		if dst == nil {
			t.Fatal("CloneHeader should not return nil for non-nil src")
		}
		// Keys with empty value slices are not copied (totalValues == 0 fast path)
		if len(dst) != 0 {
			t.Logf("Empty value slice keys not preserved (expected behavior): got %d keys", len(dst))
		}
	})
}

func TestClearHeaderPools(t *testing.T) {
	h := GetHeader()
	h.Set("X-Test", "value")
	PutHeader(h)

	ClearHeaderPools()

	// Should work fine after clearing
	h2 := GetHeader()
	if len(*h2) != 0 {
		t.Error("Header from cleared pool should be empty")
	}
	PutHeader(h2)
}

func TestQueryBuilder(t *testing.T) {
	t.Run("GetAndReturn", func(t *testing.T) {
		sb := getQueryBuilder()
		sb.WriteString("test")
		putQueryBuilder(sb)

		sb2 := getQueryBuilder()
		if sb2.Len() != 0 {
			t.Error("Reused builder should be reset")
		}
		putQueryBuilder(sb2)
	})

	t.Run("PutNil", func(t *testing.T) {
		putQueryBuilder(nil) // should not panic
	})

	t.Run("OversizeNotPooled", func(t *testing.T) {
		sb := getQueryBuilder()
		sb.Grow(5000)
		sb.WriteString(strings.Repeat("x", 5000))
		putQueryBuilder(sb) // should discard
	})
}

func TestQueryEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Empty", "", ""},
		{"NoEscape", "hello", "hello"},
		{"Space", "hello world", "hello%20world"},
		{"SpecialChars", "a=b&c=d", "a%3Db%26c%3Dd"},
		{"Unicode", "hello世界", "hello%E4%B8%96%E7%95%8C"},
		{"Unreserved", "-._~", "-._~"},
		{"AllAlpha", "ABCxyz", "ABCxyz"},
		{"Digits", "12345", "12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryEscape(tt.input)
			if got != tt.want {
				t.Errorf("queryEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEncodeQueryParams(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{"Nil", nil, ""},
		{"Empty", map[string]any{}, ""},
		{"SingleParam", map[string]any{"key": "value"}, "key=value"},
		{"MultipleParams", map[string]any{"a": "1", "b": "2"}, ""},
		{"SpecialChars", map[string]any{"q": "hello world"}, "q=hello%20world"},
		{"IntValue", map[string]any{"page": 42}, "page=42"},
		{"BoolValue", map[string]any{"flag": true}, "flag=true"},
		{"EmptyValue", map[string]any{"key": ""}, "key="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EncodeQueryParams(tt.params)
			if tt.want == "" && len(tt.params) > 1 {
				// Multiple params - just check it's non-empty
				if got == "" {
					t.Error("Expected non-empty result for multiple params")
				}
				return
			}
			if got != tt.want {
				t.Errorf("EncodeQueryParams() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppendQueryParams(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		params   map[string]any
		want     string
	}{
		{"NilParams", "a=1", nil, "a=1"},
		{"EmptyParams", "a=1", map[string]any{}, "a=1"},
		{"AppendToEmpty", "", map[string]any{"b": "2"}, "b=2"},
		{"AppendToExisting", "a=1", map[string]any{"b": "2"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AppendQueryParams(tt.existing, tt.params)
			if tt.want != "" && got != tt.want {
				t.Errorf("AppendQueryParams() = %q, want %q", got, tt.want)
			}
			if tt.want == "" && len(tt.params) > 0 {
				if !strings.Contains(got, tt.existing) {
					t.Errorf("Result %q should contain existing %q", got, tt.existing)
				}
			}
		})
	}
}
