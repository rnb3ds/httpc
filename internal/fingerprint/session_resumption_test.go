package fingerprint_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/cybergodev/httpc/internal/fingerprint"
)

// TestSessionResumption pins the session-ticket behavior end to end against
// a local TLS server, observed SERVER-side (Request.TLS.DidResume): with the
// cache on (the default) a fresh connection to the same host resumes the
// session with a PSK the way a real browser does; with DisableSessionTickets
// every connection is a full handshake.
//
// Each request is followed by CloseIdleConnections so the next request must
// dial fresh — an idle-connection reuse would never exercise the ticket
// cache. The ticket arrives asynchronously in TLS 1.3 (NewSessionTicket
// after the handshake), so the resuming case gets several attempts and must
// observe at least one resumed connection; the disabled case must observe
// none, ever. The client-side response TLS state is not used as the signal:
// the transport fork does not reliably populate Response.TLS.
func TestSessionResumption(t *testing.T) {
	var mu sync.Mutex
	seen := make(map[string]bool) // RemoteAddr of connections already counted
	var connections, resumed int

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !seen[r.RemoteAddr] {
			seen[r.RemoteAddr] = true
			connections++
			if r.TLS != nil && r.TLS.DidResume {
				resumed++
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tests := []struct {
		name       string
		disable    bool
		wantResume bool
	}{
		{"session tickets on (default)", false, true},
		{"session tickets disabled", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mu.Lock()
			seen = make(map[string]bool)
			connections, resumed = 0, 0
			mu.Unlock()

			adapter := fingerprint.NewAdapter(fingerprint.Config{
				PresetName:            "chrome",
				InsecureSkipVerify:    true, // httptest self-signed cert
				AllowPrivateIPs:       true, // 127.0.0.1
				EnableHTTP2:           false,
				DisableSessionTickets: tt.disable,
			})
			defer func() { _ = adapter.Close() }()

			client := &http.Client{Transport: adapter}

			for range 5 {
				resp, err := client.Get(server.URL)
				if err != nil {
					t.Fatalf("request: %v", err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()

				// Force the next request onto a fresh connection.
				adapter.CloseIdleConnections()
			}

			mu.Lock()
			defer mu.Unlock()
			if connections < 2 {
				t.Fatalf("connection recycling failed: %d connections for 5 forced dials", connections)
			}
			gotResume := resumed > 0
			if gotResume != tt.wantResume {
				t.Errorf("resumed connections = %d of %d, want resumed>0 = %v", resumed, connections, tt.wantResume)
			}
		})
	}
}
