package httpc

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// ----------------------------------------------------------------------------
// Response.String() Tests
// ----------------------------------------------------------------------------

func TestResponse_String(t *testing.T) {
	t.Run("nil response", func(t *testing.T) {
		var r *Response
		result := r.String()
		if result != "<nil Response>" {
			t.Errorf("String() = %q, want %q", result, "<nil Response>")
		}
	})

	t.Run("basic response", func(t *testing.T) {
		r := &Response{
			StatusCode:    200,
			Status:        "OK",
			ContentLength: 1024,
			Duration:      100 * time.Millisecond,
			Attempts:      1,
		}
		result := r.String()

		expected := []string{
			"Response{",
			"Status: 200 OK",
			"ContentLength: 1024",
			"Duration: 100ms",
			"Attempts: 1",
			"}",
		}

		for _, exp := range expected {
			if !strings.Contains(result, exp) {
				t.Errorf("String() missing %q, got %q", exp, result)
			}
		}
	})

	t.Run("response with headers", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("Content-Type", "application/json")
		headers.Set("X-Custom", "value")

		r := &Response{
			StatusCode:    200,
			Status:        "OK",
			Headers:       headers,
			ContentLength: 512,
			Duration:      50 * time.Millisecond,
			Attempts:      2,
		}
		result := r.String()

		if !strings.Contains(result, "Headers: 2") {
			t.Errorf("String() should contain header count, got %q", result)
		}
	})

	t.Run("response with cookies", func(t *testing.T) {
		r := &Response{
			StatusCode:    200,
			Status:        "OK",
			ContentLength: 256,
			Duration:      25 * time.Millisecond,
			Attempts:      1,
			Cookies: []*http.Cookie{
				{Name: "session", Value: "abc123"},
				{Name: "token", Value: "xyz789"},
			},
		}
		result := r.String()

		if !strings.Contains(result, "Cookies: 2") {
			t.Errorf("String() should contain cookie count, got %q", result)
		}
	})

	t.Run("response with all fields", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("Content-Type", "text/html")

		r := &Response{
			StatusCode:    404,
			Status:        "Not Found",
			Headers:       headers,
			ContentLength: 2048,
			Duration:      200 * time.Millisecond,
			Attempts:      3,
			Cookies: []*http.Cookie{
				{Name: "test", Value: "value"},
			},
		}
		result := r.String()

		expected := []string{
			"Status: 404 Not Found",
			"ContentLength: 2048",
			"Duration: 200ms",
			"Attempts: 3",
			"Headers: 1",
			"Cookies: 1",
		}

		for _, exp := range expected {
			if !strings.Contains(result, exp) {
				t.Errorf("String() missing %q, got %q", exp, result)
			}
		}
	})
}

// ----------------------------------------------------------------------------
// htmlEscape Tests
// ----------------------------------------------------------------------------

func TestHtmlEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special chars",
			input: "Hello World",
			want:  "Hello World",
		},
		{
			name:  "less than",
			input: "<div>",
			want:  "&lt;div&gt;",
		},
		{
			name:  "greater than",
			input: "a > b",
			want:  "a &gt; b",
		},
		{
			name:  "ampersand",
			input: "Tom & Jerry",
			want:  "Tom &amp; Jerry",
		},
		{
			name:  "double quote",
			input: `Say "Hello"`,
			want:  "Say &quot;Hello&quot;",
		},
		{
			name:  "single quote",
			input: "It's working",
			want:  "It&#39;s working",
		},
		{
			name:  "all special chars",
			input: `<script>alert("XSS & 'injection'")</script>`,
			want:  "&lt;script&gt;alert(&quot;XSS &amp; &#39;injection&#39;&quot;)&lt;/script&gt;",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := htmlEscape(tt.input)
			if got != tt.want {
				t.Errorf("htmlEscape() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Benchmark Tests
// ----------------------------------------------------------------------------

func BenchmarkResponse_String(b *testing.B) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Request-ID", "12345")

	r := &Response{
		StatusCode:    200,
		Status:        "OK",
		Headers:       headers,
		ContentLength: 1024,
		Duration:      100 * time.Millisecond,
		Attempts:      1,
		Cookies: []*http.Cookie{
			{Name: "session", Value: "abc123"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.String()
	}
}

func BenchmarkResponse_Html(b *testing.B) {
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	headers.Set("X-Request-ID", "12345")

	r := &Response{
		StatusCode:    200,
		Status:        "OK",
		Headers:       headers,
		Body:          `{"message": "Hello, World!"}`,
		ContentLength: 30,
		Duration:      100 * time.Millisecond,
		Attempts:      1,
		Cookies: []*http.Cookie{
			{Name: "session", Value: "abc123"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = r.Html()
	}
}

func BenchmarkHtmlEscape(b *testing.B) {
	input := `<script>alert("XSS & 'injection'")</script>`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = htmlEscape(input)
	}
}
