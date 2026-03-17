package security

import (
	"net/url"
	"testing"
)

func TestNewDomainWhitelist(t *testing.T) {
	tests := []struct {
		name      string
		domains   []string
		wantLen   int
		wantWCLen int
	}{
		{
			name:      "empty whitelist",
			domains:   nil,
			wantLen:   0,
			wantWCLen: 0,
		},
		{
			name:      "single exact domain",
			domains:   []string{"example.com"},
			wantLen:   1,
			wantWCLen: 0,
		},
		{
			name:      "single wildcard domain",
			domains:   []string{"*.example.com"},
			wantLen:   0,
			wantWCLen: 1,
		},
		{
			name:      "mixed domains",
			domains:   []string{"example.com", "*.test.org", "api.example.com", "*.example.com"},
			wantLen:   2,
			wantWCLen: 2,
		},
		{
			name:      "domains with whitespace",
			domains:   []string{"  example.com  ", "  *.test.org  "},
			wantLen:   1,
			wantWCLen: 1,
		},
		{
			name:      "case insensitive",
			domains:   []string{"EXAMPLE.com", "*.TEST.org"},
			wantLen:   1,
			wantWCLen: 1,
		},
		{
			name:      "empty strings are ignored",
			domains:   []string{"", "example.com", ""},
			wantLen:   1,
			wantWCLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wl := NewDomainWhitelist(tt.domains...)
			if wl == nil {
				t.Fatal("expected non-nil whitelist")
			}

			exact, wildcards := wl.Domains()
			if len(exact) != tt.wantLen {
				t.Errorf("exact matches = %d, want %d", len(exact), tt.wantLen)
			}
			if len(wildcards) != tt.wantWCLen {
				t.Errorf("wildcards = %d, want %d", len(wildcards), tt.wantWCLen)
			}
		})
	}
}

func TestDomainWhitelist_IsAllowed(t *testing.T) {
	wl := NewDomainWhitelist(
		"example.com",
		"api.example.com",
		"*.test.org",
		"*.example.net",
	)

	tests := []struct {
		hostname string
		want     bool
	}{
		// Exact matches
		{"example.com", true},
		{"api.example.com", true},
		{"EXAMPLE.com", true}, // case insensitive
		{"API.EXAMPLE.COM", true},

		// Wildcard matches
		{"test.org", true},          // *.test.org matches test.org
		{"sub.test.org", true},      // *.test.org matches sub.test.org
		{"deep.sub.test.org", true}, // *.test.org matches deep.sub.test.org
		{"example.net", true},       // *.example.net matches example.net
		{"sub.example.net", true},   // *.example.net matches sub.example.net

		// Non-matches
		{"other.com", false},
		{"notexample.com", false},
		{"example.com.evil.org", false},
		{"test.org.evil.org", false},
		{"", false},

		// Wildcard doesn't match different TLD
		{"test.com", false},
		{"example.org", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			got := wl.IsAllowed(tt.hostname)
			if got != tt.want {
				t.Errorf("IsAllowed(%q) = %v, want %v", tt.hostname, got, tt.want)
			}
		})
	}
}

func TestDomainWhitelist_Nil(t *testing.T) {
	var wl *DomainWhitelist

	// Nil whitelist allows all domains
	if !wl.IsAllowed("example.com") {
		t.Error("nil whitelist should allow all domains")
	}
}

func TestDomainWhitelist_Add(t *testing.T) {
	wl := NewDomainWhitelist("example.com")

	// Add exact domain
	wl.Add("newdomain.com")
	if !wl.IsAllowed("newdomain.com") {
		t.Error("expected newdomain.com to be allowed after Add")
	}

	// Add wildcard domain
	wl.Add("*.wildcard.org")
	if !wl.IsAllowed("sub.wildcard.org") {
		t.Error("expected sub.wildcard.org to be allowed after Add")
	}

	// Add empty string (should be ignored)
	wl.Add("")
	if wl.IsAllowed("") {
		t.Error("empty string should not be allowed")
	}
}

func TestDomainWhitelist_Remove(t *testing.T) {
	wl := NewDomainWhitelist("example.com", "*.test.org")

	// Remove exact domain
	wl.Remove("example.com")
	if wl.IsAllowed("example.com") {
		t.Error("expected example.com to be removed")
	}

	// Remove wildcard domain
	wl.Remove("*.test.org")
	if wl.IsAllowed("sub.test.org") {
		t.Error("expected sub.test.org to be removed")
	}

	// Verify other domains still work
	wl.Add("other.com")
	wl.Remove("nonexistent.com")
	if !wl.IsAllowed("other.com") {
		t.Error("expected other.com to still be allowed")
	}
}

func TestValidateRedirectWhitelist(t *testing.T) {
	wl := NewDomainWhitelist("example.com", "*.trusted.org")

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "exact match",
			url:     "https://example.com/path",
			wantErr: false,
		},
		{
			name:    "wildcard match",
			url:     "https://sub.trusted.org/path",
			wantErr: false,
		},
		{
			name:    "not in whitelist",
			url:     "https://evil.com/path",
			wantErr: true,
		},
		{
			name:    "nil URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "empty hostname",
			url:     "https:///path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var targetURL *url.URL
			if tt.url != "" {
				var err error
				targetURL, err = url.Parse(tt.url)
				if err != nil {
					t.Fatalf("failed to parse URL: %v", err)
				}
			}

			err := ValidateRedirectWhitelist(targetURL, wl)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}

	// Test nil whitelist
	t.Run("nil whitelist", func(t *testing.T) {
		targetURL, _ := url.Parse("https://anydomain.com/path")
		err := ValidateRedirectWhitelist(targetURL, nil)
		if err != nil {
			t.Errorf("nil whitelist should allow all domains, got error: %v", err)
		}
	})
}

func TestDomainWhitelist_Concurrency(t *testing.T) {
	wl := NewDomainWhitelist("example.com")

	// Concurrent reads and writes
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			wl.Add("test.com")
			wl.Remove("test.com")
		}
		done <- true
	}()

	// Reader goroutines
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = wl.IsAllowed("example.com")
				_ = wl.IsAllowed("test.com")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 11; i++ {
		<-done
	}
}

// ============================================================================
// BENCHMARKS
// ============================================================================

func BenchmarkNewDomainWhitelist(b *testing.B) {
	domains := []string{
		"example.com",
		"api.example.com",
		"*.test.org",
		"*.example.net",
		"trusted.com",
		"*.cdn.example.com",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewDomainWhitelist(domains...)
	}
}

func BenchmarkNewDomainWhitelist_Large(b *testing.B) {
	// Create a larger list of domains (100 domains)
	domains := make([]string, 100)
	for i := 0; i < 100; i++ {
		if i%4 == 0 {
			domains[i] = "*.example" + string(rune('0'+i%10)) + ".com"
		} else {
			domains[i] = "domain" + string(rune('0'+i%10)) + ".example.com"
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = NewDomainWhitelist(domains...)
	}
}

func BenchmarkDomainWhitelist_IsAllowed(b *testing.B) {
	wl := NewDomainWhitelist(
		"example.com",
		"api.example.com",
		"*.test.org",
		"*.example.net",
		"trusted.com",
		"*.cdn.example.com",
	)

	testCases := []struct {
		name     string
		hostname string
	}{
		{"exact_match", "example.com"},
		{"wildcard_match", "sub.test.org"},
		{"no_match", "other.com"},
		{"uppercase", "EXAMPLE.COM"},
		{"with_whitespace", "  example.com  "},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = wl.IsAllowed(tc.hostname)
			}
		})
	}
}

func BenchmarkDomainWhitelist_IsAllowed_Parallel(b *testing.B) {
	wl := NewDomainWhitelist(
		"example.com",
		"api.example.com",
		"*.test.org",
		"*.example.net",
	)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		hostnames := []string{
			"example.com",
			"sub.test.org",
			"other.com",
			"api.example.com",
		}
		i := 0
		for pb.Next() {
			_ = wl.IsAllowed(hostnames[i%len(hostnames)])
			i++
		}
	})
}

func BenchmarkDomainWhitelist_Add(b *testing.B) {
	wl := NewDomainWhitelist()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		wl.Add("newdomain.com")
	}
}

func BenchmarkDomainWhitelist_Remove(b *testing.B) {
	// Pre-populate with many domains
	domains := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		domains[i] = "domain" + string(rune('0'+i%10)) + ".com"
	}
	wl := NewDomainWhitelist(domains...)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		wl.Remove(domains[i%len(domains)])
	}
}

func BenchmarkNormalizeDomain(b *testing.B) {
	testCases := []struct {
		name   string
		domain string
	}{
		{"already_normalized", "example.com"},
		{"uppercase", "EXAMPLE.COM"},
		{"with_whitespace", "  example.com  "},
		{"complex", "  API.Example.COM  "},
		{"empty", ""},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = normalizeDomain(tc.domain)
			}
		})
	}
}
