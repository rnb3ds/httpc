package engine

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/httpc/internal/connection"
)

// TestBuild_ReplayableReaderGetBody pins the 307/308 parity fix: the three
// concrete reader types net/http's NewRequest makes replayable
// (*strings.Reader, *bytes.Reader, *bytes.Buffer) now get GetBody here too —
// previously they fell into the generic io.Reader bucket and a bodied POST
// receiving 307/308 surfaced the 3xx unfollowed, a silent divergence from
// plain net/http for exactly the types the stdlib replays.
func TestBuild_ReplayableReaderGetBody(t *testing.T) {
	tests := []struct {
		name    string
		body    io.Reader
		wantGet bool
	}{
		{"*strings.Reader is replayable", strings.NewReader("payload"), true},
		{"*bytes.Reader is replayable", bytes.NewReader([]byte("payload")), true},
		{"*bytes.Buffer is replayable", bytes.NewBufferString("payload"), true},
		{"generic reader is not replayable", struct{ io.Reader }{strings.NewReader("payload")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := AcquireRequest()
			defer ReleaseRequest(req)
			req.SetMethod("POST")
			req.SetURL("https://example.com/upload")
			req.SetContext(context.Background())
			req.SetBody(tt.body)

			httpReq, err := newRequestProcessor(&Config{Timeout: 30 * time.Second, FollowRedirects: true}).Build(req)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if tt.wantGet && httpReq.GetBody == nil {
				t.Fatal("net/http-replayable reader body must get GetBody for 307/308 hops")
			}
			if !tt.wantGet && httpReq.GetBody != nil {
				t.Fatal("generic io.Reader body must stay non-replayable")
			}
			if tt.wantGet {
				rc, err := httpReq.GetBody()
				if err != nil {
					t.Fatalf("GetBody: %v", err)
				}
				got, _ := io.ReadAll(rc)
				if string(got) != "payload" {
					t.Errorf("replayed body = %q, want %q", got, "payload")
				}
			}
		})
	}
}

// TestHTTPClientNoTimeoutInvariant pins the pooled-header-poisoning defense:
// the stdlib Client wrapping the transport must NEVER carry a Timeout (a
// non-zero deadline makes Client.send fork the request, sharing the engine's
// pooled http.Header map with a Result-bound fork). See the INVARIANT comment
// in newTransport.
func TestHTTPClientNoTimeoutInvariant(t *testing.T) {
	pool, err := connection.NewPoolManager(connection.DefaultConfig())
	if err != nil {
		t.Fatalf("NewPoolManager: %v", err)
	}
	defer func() { _ = pool.Close() }()
	tr, err := newTransport(&Config{}, pool)
	if err != nil {
		t.Fatalf("newTransport: %v", err)
	}
	if tr.httpClient.Timeout != 0 {
		t.Errorf("httpClient.Timeout = %v, must stay 0 (request deadlines are owned by the engine budget)", tr.httpClient.Timeout)
	}
}
