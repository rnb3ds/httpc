package validation

import (
	"testing"
)

func BenchmarkSanitizeURL_Plain(b *testing.B) {
	url := "https://api.example.com/v1/users"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = SanitizeURL(url)
	}
}

func BenchmarkSanitizeURL_WithQuery(b *testing.B) {
	url := "https://api.example.com/v1/users?page=1&limit=10&sort=name"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = SanitizeURL(url)
	}
}

func BenchmarkSanitizeURL_WithSensitiveQuery(b *testing.B) {
	url := "https://api.example.com/v1/users?page=1&token=secret123&api_key=mykey"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = SanitizeURL(url)
	}
}

func BenchmarkSanitizeURL_WithCredentials(b *testing.B) {
	url := "https://user:password@api.example.com/v1/users?page=1"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = SanitizeURL(url)
	}
}

func BenchmarkSanitizeURL_FastPath(b *testing.B) {
	// No @, ?, #, or space — should hit fast path
	url := "https://api.example.com/v1/users/123/profile"

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = SanitizeURL(url)
	}
}

func BenchmarkIsSensitiveQueryParamCI(b *testing.B) {
	names := []string{"token", "page", "limit", "Authorization", "Content-Type", "session_id"}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _, name := range names {
			isSensitiveQueryParamCI(name)
		}
	}
}
