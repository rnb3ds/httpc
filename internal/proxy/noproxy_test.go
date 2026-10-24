package proxy

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
)

// TestApplyNoProxy pins the platform-path NO_PROXY wrapper: NO_PROXY must be
// honored when the proxy comes from the OS (Windows registry / macOS
// networksetup), not just from HTTP(S)_PROXY — with the same canonical-port
// semantics as the environment path. Exempt hosts bypass; non-exempt hosts
// keep the platform proxy; platform bypass (fn → nil) and detection errors
// pass through unchanged.
func TestApplyNoProxy(t *testing.T) {
	proxyURL := &url.URL{Scheme: "http", Host: "proxy.corp:8080"}
	fn := func(req *http.Request) (*url.URL, error) { return proxyURL, nil }

	tests := []struct {
		name    string
		noProxy string
		url     string
		wantNil bool
	}{
		{"exempt host bypasses", "intranet.corp", "http://intranet.corp/api", true},
		{"subdomain of exempt bypasses", "intranet.corp", "http://host.intranet.corp/api", true},
		{"non-exempt host keeps proxy", "intranet.corp", "http://example.com/api", false},
		{"wildcard bypasses everything", "*", "http://anything.example/", true},
		{"canonical port: implicit https matches :443 entry", "intranet.corp:443", "https://intranet.corp/api", true},
		{"canonical port: implicit https misses :80 entry", "intranet.corp:80", "https://intranet.corp/api", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := applyNoProxy(fn, tt.noProxy)
			req, err := http.NewRequest(http.MethodGet, tt.url, nil)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			got, err := wrapped(req)
			if err != nil {
				t.Fatalf("wrapped: %v", err)
			}
			if tt.wantNil && got != nil {
				t.Errorf("got proxy %v, want direct (nil)", got)
			}
			if !tt.wantNil && got != proxyURL {
				t.Errorf("got %v, want the platform proxy", got)
			}
		})
	}

	t.Run("empty noProxy keeps the proxy for every host", func(t *testing.T) {
		wrapped := applyNoProxy(fn, "")
		req, _ := http.NewRequest(http.MethodGet, "http://intranet.corp/", nil)
		if u, err := wrapped(req); err != nil || u != proxyURL {
			t.Errorf("wrapped = %v, %v; want the platform proxy unchanged", u, err)
		}
	})

	t.Run("platform bypass passes through", func(t *testing.T) {
		bypass := func(req *http.Request) (*url.URL, error) { return nil, nil }
		wrapped := applyNoProxy(bypass, "*")
		req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
		if u, err := wrapped(req); u != nil || err != nil {
			t.Errorf("wrapped bypass = %v, %v; want nil, nil", u, err)
		}
	})

	t.Run("detection error passes through", func(t *testing.T) {
		sentinel := errors.New("platform detection failed")
		failing := func(req *http.Request) (*url.URL, error) { return nil, sentinel }
		wrapped := applyNoProxy(failing, "*")
		req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
		if _, err := wrapped(req); !errors.Is(err, sentinel) {
			t.Errorf("wrapped error = %v, want the sentinel passed through", err)
		}
	})
}
