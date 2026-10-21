// Response-body decoding for the fingerprint path.
//
// The browser profiles advertise "gzip, deflate, br, zstd", but the httpc
// engine's own decompressor handles only gzip/deflate, so the fingerprint
// transport decodes every response itself and hands the engine an identity
// body (Content-Encoding/Content-Length headers removed, ContentLength
// reset), exactly where the engine reads them — on the final response
// object. Streaming stays streaming: bodies are wrapped with lazy readers,
// never buffered.
//
// Reader pools and the ownership chain ported from github.com/enetx/surf
// (middleware_response.go + pools.go), Copyright enetx/surf authors, MIT
// License. Deltas, all deliberate:
//   - Multi-token Content-Encoding chains wrap in REVERSE order. RFC 9110
//     §8.4.1 lists encodings in application order, so the last-applied
//     encoding is the first that must be decoded. (Upstream wrapped in
//     listed order, which mis-decodes chains — never triggered in practice
//     because real servers send single tokens.)
//   - An unknown encoding token in the chain fails the round trip loudly.
//     Upstream stripped the headers and silently passed compressed bytes
//     as the body — a data-integrity defect.
//   - "deflate" accepts both zlib-wrapped and raw deflate streams (header
//     sniffed before any byte is consumed), fixing upstream's zlib-only
//     strictness.

package fingerprint

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	ehttp "github.com/bogdanfinn/fhttp"
	"github.com/klauspost/compress/zstd"
)

// decodedReadCloser owns both the decoder and the encoded source it wraps.
// Closing only the decoder leaks the HTTP response body; nesting this type
// also preserves ownership for encoding chains.
//
// Close-contract: exactly-once is guaranteed by the engine's ownership
// discipline (buffered path closes once in executeRequest's defer; the
// streaming path wraps bodies in streamBodyReader, whose atomic closed flag
// serializes the caller's Close against ReleaseResponse's — see RACE-001
// and TestStreamBodyReader_CloseIdempotent). No guard is carried here for
// the same reason the engine's own pooled decompressor wrappers carry none.
type decodedReadCloser struct {
	decoder io.ReadCloser
	source  io.ReadCloser
}

func (reader *decodedReadCloser) Read(buffer []byte) (int, error) {
	return reader.decoder.Read(buffer)
}

func (reader *decodedReadCloser) Close() error {
	return errors.Join(reader.decoder.Close(), reader.source.Close())
}

var (
	// zstdDecoderPool pools zstd.Decoder instances.
	zstdDecoderPool = sync.Pool{
		New: func() any {
			// zstd.NewReader without options cannot fail (the discarded
			// error is only reachable via options like WithDecoderRaw); if it
			// ever did, the nil falls through acquireZstdReader's type
			// assertion to a fresh decoder instead of poisoning the pool.
			dec, _ := zstd.NewReader(nil)
			return dec
		},
	}

	// gzipReaderPool pools gzip.Reader instances.
	gzipReaderPool = sync.Pool{
		New: func() any {
			return new(gzip.Reader)
		},
	}

	// brotliReaderPool pools brotli.Reader instances.
	brotliReaderPool = sync.Pool{
		New: func() any {
			return brotli.NewReader(nil)
		},
	}
)

// zstdReadCloser wraps a zstd decoder and returns it to the pool on Close.
type zstdReadCloser struct {
	dec *zstd.Decoder
}

func (zr *zstdReadCloser) Read(p []byte) (int, error) { return zr.dec.Read(p) }

func (zr *zstdReadCloser) Close() error {
	if err := zr.dec.Reset(nil); err != nil {
		return err
	}

	zstdDecoderPool.Put(zr.dec)
	return nil
}

// gzipReadCloser wraps a gzip reader and returns it to the pool on Close.
type gzipReadCloser struct {
	*gzip.Reader
}

func (gr *gzipReadCloser) Close() error {
	if err := gr.Reader.Close(); err != nil {
		return err
	}

	gzipReaderPool.Put(gr.Reader)
	return nil
}

// brotliReadCloser wraps a brotli reader and returns it to the pool on Close.
type brotliReadCloser struct {
	*brotli.Reader
}

func (br *brotliReadCloser) Close() error {
	brotliReaderPool.Put(br.Reader)
	return nil
}

// acquireGzipReader gets a gzip.Reader from the pool and resets it onto r.
func acquireGzipReader(r io.Reader) (io.ReadCloser, error) {
	gr := gzipReaderPool.Get().(*gzip.Reader)
	if err := gr.Reset(r); err != nil {
		gzipReaderPool.Put(gr)
		return nil, err
	}
	return &gzipReadCloser{Reader: gr}, nil
}

// acquireBrotliReader gets a brotli.Reader from the pool and resets it onto r.
func acquireBrotliReader(r io.Reader) (io.ReadCloser, error) {
	br := brotliReaderPool.Get().(*brotli.Reader)
	if err := br.Reset(r); err != nil {
		brotliReaderPool.Put(br)
		return nil, err
	}
	return &brotliReadCloser{Reader: br}, nil
}

// acquireZstdReader gets a zstd.Decoder from the pool and resets it onto r.
func acquireZstdReader(r io.Reader) (io.ReadCloser, error) {
	dec := zstdDecoderPool.Get().(*zstd.Decoder)
	if err := dec.Reset(r); err != nil {
		zstdDecoderPool.Put(dec)
		return nil, err
	}
	return &zstdReadCloser{dec: dec}, nil
}

// newDeflateReader decodes "deflate" content: most servers send zlib-wrapped
// streams, some send raw deflate (a decades-old server quirk). The two-byte
// zlib header is sniffed through a peeking buffer before anything is
// consumed, so the raw fallback sees the complete stream.
func newDeflateReader(r io.Reader) io.ReadCloser {
	buffered := bufio.NewReader(r)
	if hdr, err := buffered.Peek(2); err == nil &&
		hdr[0]&0x0f == 8 && (uint16(hdr[0])<<8|uint16(hdr[1]))%31 == 0 {
		if zr, err := zlib.NewReader(buffered); err == nil {
			return zr
		}
	}
	return flate.NewReader(buffered)
}

// decodeResponse decodes resp.Body's Content-Encoding chain in place: the
// body is wrapped with lazy decoders (streaming preserved), the
// Content-Encoding/Content-Length headers are removed, and ContentLength is
// reset to -1 so no caller can trust the encoded length. Identity and absent
// encodings pass through untouched. An unknown encoding token fails the
// round trip rather than silently handing back compressed bytes.
func decodeResponse(resp *http.Response) error {
	// Compare both NoBody sentinels: this path only ever runs on bodies the
	// fhttp fork produced, whose NoBody is a distinct pointer from stdlib
	// http.NoBody — checking only the stdlib one never matched here.
	if resp.Body == nil || resp.Body == http.NoBody || resp.Body == ehttp.NoBody {
		return nil
	}

	encoding := resp.Header.Get("Content-Encoding")
	if encoding == "" {
		return nil
	}

	tokens := strings.Split(encoding, ",")
	reader := resp.Body

	// A chain of identity/empty tokens encodes nothing — leave the response
	// (headers and ContentLength included) exactly as it arrived.
	needsDecode := false
	for _, tok := range tokens {
		switch strings.ToLower(strings.TrimSpace(tok)) {
		case "", "identity":
		default:
			needsDecode = true
		}
	}
	if !needsDecode {
		return nil
	}

	// Wrap in reverse: the last-applied encoding is decoded first.
	for i := len(tokens) - 1; i >= 0; i-- {
		enc := strings.ToLower(strings.TrimSpace(tokens[i]))

		var decoder io.ReadCloser
		var err error

		switch enc {
		case "", "identity":
			continue
		case "deflate":
			decoder = newDeflateReader(reader)
		case "gzip":
			decoder, err = acquireGzipReader(reader)
		case "br":
			decoder, err = acquireBrotliReader(reader)
		case "zstd":
			decoder, err = acquireZstdReader(reader)
		default:
			_ = reader.Close() // best-effort: the chain is unusable after an unsupported encoding
			return fmt.Errorf("fingerprint: unsupported Content-Encoding %q in %q: cannot decode response body", enc, encoding)
		}

		if err != nil {
			_ = reader.Close() // best-effort: decoder acquisition failed, unwind the chain
			return fmt.Errorf("fingerprint: %s decoder: %w", enc, err)
		}

		reader = &decodedReadCloser{decoder: decoder, source: reader}
	}

	resp.Body = reader
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length")
	resp.ContentLength = -1

	return nil
}
