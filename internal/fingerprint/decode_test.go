package fingerprint

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

// encodedResponse builds a response carrying body bytes under the given
// Content-Encoding header value.
func encodedResponse(t *testing.T, encoding string, body []byte) *http.Response {
	t.Helper()
	return &http.Response{
		StatusCode: 200,
		Header: http.Header{
			"Content-Encoding": []string{encoding},
			"Content-Length":   []string{"0"}, // deliberately stale/wrong after encode
		},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

// TestDecodeResponse_SingleEncodings walks every supported codec end to end
// and asserts the post-decode contract: body decoded, CE/CL headers gone,
// ContentLength reset to -1.
func TestDecodeResponse_SingleEncodings(t *testing.T) {
	payload := strings.Repeat("fingerprint decode payload ", 32)

	t.Run("gzip", func(t *testing.T) {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		// Writer errors are impossible (bytes.Buffer backing); `_ =` is for errcheck.
		_, _ = zw.Write([]byte(payload))
		_ = zw.Close()
		resp := encodedResponse(t, "gzip", buf.Bytes())

		if err := decodeResponse(resp); err != nil {
			t.Fatalf("decodeResponse: %v", err)
		}
		assertDecoded(t, resp, payload)
	})

	t.Run("deflate zlib", func(t *testing.T) {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		// Writer errors are impossible (bytes.Buffer backing); `_ =` is for errcheck.
		_, _ = zw.Write([]byte(payload))
		_ = zw.Close()
		resp := encodedResponse(t, "deflate", buf.Bytes())

		if err := decodeResponse(resp); err != nil {
			t.Fatalf("decodeResponse: %v", err)
		}
		assertDecoded(t, resp, payload)
	})

	t.Run("deflate raw", func(t *testing.T) {
		var buf bytes.Buffer
		fw, _ := flate.NewWriter(&buf, 5)
		_, _ = fw.Write([]byte(payload))
		_ = fw.Close()
		resp := encodedResponse(t, "deflate", buf.Bytes())

		if err := decodeResponse(resp); err != nil {
			t.Fatalf("decodeResponse: %v", err)
		}
		assertDecoded(t, resp, payload)
	})

	t.Run("brotli", func(t *testing.T) {
		var buf bytes.Buffer
		bw := brotli.NewWriter(&buf)
		_, _ = bw.Write([]byte(payload))
		_ = bw.Close()
		resp := encodedResponse(t, "br", buf.Bytes())

		if err := decodeResponse(resp); err != nil {
			t.Fatalf("decodeResponse: %v", err)
		}
		assertDecoded(t, resp, payload)
	})

	t.Run("zstd", func(t *testing.T) {
		var buf bytes.Buffer
		zw, _ := zstd.NewWriter(&buf)
		// Writer errors are impossible (bytes.Buffer backing); `_ =` is for errcheck.
		_, _ = zw.Write([]byte(payload))
		_ = zw.Close()
		resp := encodedResponse(t, "zstd", buf.Bytes())

		if err := decodeResponse(resp); err != nil {
			t.Fatalf("decodeResponse: %v", err)
		}
		assertDecoded(t, resp, payload)
	})
}

// assertDecoded asserts the shared post-decode contract.
func assertDecoded(t *testing.T, resp *http.Response, want string) {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read decoded body: %v", err)
	}
	if string(body) != want {
		t.Fatalf("decoded body mismatch: %d bytes, prefix %q", len(body), string(body[:min(32, len(body))]))
	}
	if resp.Header.Get("Content-Encoding") != "" {
		t.Error("Content-Encoding header not stripped")
	}
	if resp.Header.Get("Content-Length") != "" {
		t.Error("Content-Length header not stripped")
	}
	if resp.ContentLength != -1 {
		t.Errorf("ContentLength = %d, want -1 (stale encoded length must not survive)", resp.ContentLength)
	}
}

// TestDecodeResponse_IdentityTokenInChain pins that an identity token inside
// a multi-token chain is skipped by the wrap loop (the identity layer encodes
// nothing) while the real codecs still decode.
func TestDecodeResponse_IdentityTokenInChain(t *testing.T) {
	payload := strings.Repeat("mixed-chain ", 32)

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte(payload))
	_ = gw.Close()

	resp := encodedResponse(t, "gzip, identity", buf.Bytes())
	if err := decodeResponse(resp); err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	assertDecoded(t, resp, payload)
}

// TestDecodeResponse_ChainReverseOrder pins RFC 9110 chain decoding: for
// "gzip, br" the gzip layer was applied FIRST and must be decoded LAST, so
// the outermost reader is the gzip decoder.
func TestDecodeResponse_ChainReverseOrder(t *testing.T) {
	payload := strings.Repeat("chain ", 64)

	// Apply encodings in wire order: gzip first, then brotli on top.
	var gbuf bytes.Buffer
	gw := gzip.NewWriter(&gbuf)
	_, _ = gw.Write([]byte(payload))
	_ = gw.Close()

	var bbuf bytes.Buffer
	bw := brotli.NewWriter(&bbuf)
	_, _ = bw.Write(gbuf.Bytes())
	_ = bw.Close()

	resp := encodedResponse(t, "gzip, br", bbuf.Bytes())
	if err := decodeResponse(resp); err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}
	assertDecoded(t, resp, payload)
}

// TestDecodeResponse_UnknownEncodingFails pins that an unknown codec fails
// the round trip loudly instead of handing back compressed bytes with the
// headers stripped (the upstream defect this port fixes).
func TestDecodeResponse_UnknownEncodingFails(t *testing.T) {
	for _, ce := range []string{"compress", "x-snappy", "gzip, compress"} {
		resp := encodedResponse(t, ce, []byte("raw-bytes"))
		err := decodeResponse(resp)
		if err == nil {
			t.Fatalf("Content-Encoding %q: expected error, got none", ce)
		}
		if !strings.Contains(err.Error(), "unsupported Content-Encoding") {
			t.Errorf("Content-Encoding %q: unexpected error %v", ce, err)
		}
	}
}

// TestDecodeResponse_IdentityPassthrough pins that identity/absent encodings
// and bodiless responses pass through untouched (headers and ContentLength
// intact — the engine's buffered fast path relies on the real length).
func TestDecodeResponse_IdentityPassthrough(t *testing.T) {
	t.Run("absent encoding", func(t *testing.T) {
		resp := &http.Response{
			Header:        http.Header{"Content-Length": []string{"5"}},
			Body:          io.NopCloser(strings.NewReader("hello")),
			ContentLength: 5,
		}
		if err := decodeResponse(resp); err != nil {
			t.Fatalf("decodeResponse: %v", err)
		}
		if resp.ContentLength != 5 || resp.Header.Get("Content-Length") != "5" {
			t.Error("identity response was modified")
		}
	})

	t.Run("identity token", func(t *testing.T) {
		resp := encodedResponse(t, "identity", []byte("hello"))
		if err := decodeResponse(resp); err != nil {
			t.Fatalf("decodeResponse: %v", err)
		}
		// An all-identity chain encodes nothing: headers and length stay.
		if resp.Header.Get("Content-Encoding") != "identity" {
			t.Errorf("identity CE token should be left as-is, got %q", resp.Header.Get("Content-Encoding"))
		}
		if resp.ContentLength != int64(len("hello")) {
			// encodedResponse seeds CL with the body length; identity must
			// not reset it.
			t.Errorf("ContentLength = %d, want untouched", resp.ContentLength)
		}
	})

	t.Run("nil body", func(t *testing.T) {
		resp := &http.Response{Header: http.Header{"Content-Encoding": []string{"gzip"}}}
		if err := decodeResponse(resp); err != nil {
			t.Fatalf("decodeResponse on nil body: %v", err)
		}
	})
}

// TestDecodeResponse_Streaming pins lazy decoding: the body is wrapped, not
// buffered — partial reads return decoded segments, and the source stream
// advances only as far as the decoder pulls it.
func TestDecodeResponse_Streaming(t *testing.T) {
	payload := strings.Repeat("streaming-chunk|", 256)

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, _ = gw.Write([]byte(payload))
	_ = gw.Close()
	encoded := buf.Bytes()

	resp := encodedResponse(t, "gzip", encoded)
	if err := decodeResponse(resp); err != nil {
		t.Fatalf("decodeResponse: %v", err)
	}

	// Read in small segments; decoded data must arrive before EOF regardless
	// of encoded framing.
	var got []byte
	small := make([]byte, 7)
	for {
		n, err := resp.Body.Read(small)
		got = append(got, small[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("streaming read: %v", err)
		}
	}
	if string(got) != payload {
		t.Fatalf("streaming decode mismatch: got %d bytes", len(got))
	}
}

// TestDecodedBody_CloseReturnsReadersToPool pins the pooled-reader Close
// contract for every pooled codec: closing a decoded body is error-free and
// returns the reader to its pool (observable as the next acquire succeeding),
// and closing twice is harmless.
func TestDecodedBody_CloseReturnsReadersToPool(t *testing.T) {
	payload := strings.Repeat("pool-roundtrip ", 16)

	for _, enc := range []struct {
		name  string
		token string
		enc   func([]byte) []byte
	}{
		{"gzip", "gzip", func(p []byte) []byte {
			var buf bytes.Buffer
			zw := gzip.NewWriter(&buf)
			_, _ = zw.Write(p)
			_ = zw.Close()
			return buf.Bytes()
		}},
		{"brotli", "br", func(p []byte) []byte {
			var buf bytes.Buffer
			bw := brotli.NewWriter(&buf)
			_, _ = bw.Write(p)
			_ = bw.Close()
			return buf.Bytes()
		}},
		{"zstd", "zstd", func(p []byte) []byte {
			var buf bytes.Buffer
			zw, _ := zstd.NewWriter(&buf)
			_, _ = zw.Write(p)
			_ = zw.Close()
			return buf.Bytes()
		}},
	} {
		t.Run(enc.name, func(t *testing.T) {
			resp := encodedResponse(t, enc.token, enc.enc([]byte(payload)))
			if err := decodeResponse(resp); err != nil {
				t.Fatalf("decodeResponse: %v", err)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read decoded body: %v", err)
			}
			if string(body) != payload {
				t.Fatalf("decoded body mismatch (%d bytes)", len(body))
			}

			// Close releases the pooled reader; exactly the engine's
			// executeRequest defer behavior.
			if err := resp.Body.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			// The reader is back in the pool: a fresh acquire must succeed
			// and decode the same payload (pool round-trip).
			rc, err := acquireDecoded(enc.token, bytes.NewReader(enc.enc([]byte(payload))))
			if err != nil {
				t.Fatalf("re-acquire after Close: %v", err)
			}
			again, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read re-acquired body: %v", err)
			}
			_ = rc.Close()
			if string(again) != payload {
				t.Fatalf("re-acquired body mismatch (%d bytes)", len(again))
			}

			// Double-close of the original wrapper must not panic or error
			// (ownership discipline makes it unreachable in production, but
			// the pooled objects tolerate it).
			if err := resp.Body.Close(); err != nil {
				t.Errorf("second Close: %v", err)
			}
		})
	}
}

// acquireDecoded dispatches to the codec-specific pooled acquire helper.
func acquireDecoded(token string, r io.Reader) (io.ReadCloser, error) {
	switch token {
	case "gzip":
		return acquireGzipReader(r)
	case "br":
		return acquireBrotliReader(r)
	default:
		return acquireZstdReader(r)
	}
}

// TestDecodedBody_CloseAfterTruncatedGzipErrors pins gzipReadCloser.Close's
// error arm: a gzip stream cut mid-body leaves the flate decompressor in an
// error state, and Close surfaces it instead of masking it.
func TestDecodedBody_CloseAfterTruncatedGzipErrors(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte(strings.Repeat("truncated-body ", 200)))
	_ = zw.Close()
	encoded := buf.Bytes()[:buf.Len()-12] // cut into the deflate stream

	resp := encodedResponse(t, "gzip", encoded)
	if err := decodeResponse(resp); err != nil {
		t.Fatalf("decodeResponse should succeed on a valid header: %v", err)
	}

	_, readErr := io.ReadAll(resp.Body)
	if readErr == nil {
		t.Fatal("truncated gzip stream should fail to read fully")
	}

	if err := resp.Body.Close(); err == nil {
		t.Error("Close after a truncated stream should surface the decompressor error")
	}
}

// TestAcquireGzipReader_InvalidHeaderFails pins the pool's error arm and its
// decodeResponse propagation: garbage input fails the gzip header read, the
// reader returns to the pool (not poisoned), and the round trip fails loudly
// instead of handing back compressed bytes.
func TestAcquireGzipReader_InvalidHeaderFails(t *testing.T) {
	t.Run("direct acquire", func(t *testing.T) {
		if _, err := acquireGzipReader(strings.NewReader("definitely not gzip")); err == nil {
			t.Fatal("garbage input accepted by acquireGzipReader")
		}
		// The pool is not poisoned: a valid stream still acquires.
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write([]byte("ok"))
		_ = zw.Close()
		rc, err := acquireGzipReader(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("acquire after failure: %v", err)
		}
		defer func() { _ = rc.Close() }()
		body, _ := io.ReadAll(rc)
		if string(body) != "ok" {
			t.Fatalf("body = %q", body)
		}
	})

	t.Run("decodeResponse propagates the decoder error", func(t *testing.T) {
		resp := encodedResponse(t, "gzip", []byte("garbage bytes"))
		err := decodeResponse(resp)
		if err == nil {
			t.Fatal("decodeResponse accepted invalid gzip")
		}
		if !strings.Contains(err.Error(), "gzip decoder") {
			t.Errorf("error lacks decoder context: %v", err)
		}
	})
}
