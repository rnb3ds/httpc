package fingerprint

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// TestTransport_DecodesAllAdvertisedEncodings pins the boundary between the
// transport and decode.go: the profile advertises gzip, deflate, br and zstd,
// the transport's own decompression is switched off, and decode.go must hand
// the engine plaintext for every one of them.
func TestTransport_DecodesAllAdvertisedEncodings(t *testing.T) {
	const payload = "the quick brown fox jumps over the lazy dog"

	cases := []struct {
		encoding string
		write    func(io.Writer)
	}{
		{"gzip", func(w io.Writer) {
			gw := gzip.NewWriter(w)
			_, _ = gw.Write([]byte(payload))
			_ = gw.Close()
		}},
		{"br", func(w io.Writer) {
			bw := brotli.NewWriter(w)
			_, _ = bw.Write([]byte(payload))
			_ = bw.Close()
		}},
		{"zstd", func(w io.Writer) {
			zw, err := zstd.NewWriter(w)
			if err != nil {
				panic(err)
			}
			_, _ = zw.Write([]byte(payload))
			_ = zw.Close()
		}},
	}

	for _, tc := range cases {
		t.Run(tc.encoding, func(t *testing.T) {
			ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Encoding", tc.encoding)
				tc.write(w)
			}))
			ts.EnableHTTP2 = true
			ts.StartTLS()
			defer ts.Close()

			adapter := newTestAdapter(t, "chrome", true, nil)
			defer func() { _ = adapter.Close() }()

			resp, err := adapter.RoundTrip(httptestReq(t, ts.URL+"/"))
			if err != nil {
				t.Fatalf("RoundTrip: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if string(body) != payload {
				t.Errorf("body = %q, want the decoded payload", body)
			}
			// decode.go must strip the framing so nothing above trusts the
			// encoded length.
			if ce := resp.Header.Get("Content-Encoding"); ce != "" {
				t.Errorf("Content-Encoding leaked through: %q", ce)
			}
			if resp.ContentLength != -1 {
				t.Errorf("ContentLength = %d, want -1", resp.ContentLength)
			}
		})
	}
}

// TestTransport_UnknownEncodingFailsLoudly pins decode.go's fail-loud arm
// through the real transport: an encoding the profile never advertised but a
// server sent anyway must error rather than hand over compressed bytes.
func TestTransport_UnknownEncodingFailsLoudly(t *testing.T) {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "compress")
		_, _ = io.WriteString(w, "whatever")
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	adapter := newTestAdapter(t, "chrome", true, nil)
	defer func() { _ = adapter.Close() }()

	_, err := adapter.RoundTrip(httptestReq(t, ts.URL+"/"))
	if err == nil {
		t.Fatal("unknown Content-Encoding accepted")
	}
	if !strings.Contains(err.Error(), "compress") {
		t.Errorf("error should name the encoding, got: %v", err)
	}
}
