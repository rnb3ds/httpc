package engine

// Pooled reader/wrapper mechanics: Read/EOF semantics, release discipline
// (pool return only in Close, never at EOF), double-close safety, and the
// concurrent-close races the delayed HTTP/2 writeLoop Close provokes.
// All body kinds share the single pooledBody wrapper; the per-kind put
// functions (strings/bytes readers, multipart/JSON buffers) are exercised
// through their constructors.
// Consolidated from coverage_test.go, request_test.go, pools_test.go, and
// response_decompression_test.go (same subject area).

import (
	"bytes"
	"compress/gzip"
	"io"
	"sync"
	"testing"
)

// TestPooledBody_MultipartBuffer validates multipart buffer pool read behavior.
func TestPooledBody_MultipartBuffer(t *testing.T) {
	t.Run("Normal read", func(t *testing.T) {
		buf := bytes.NewBufferString("multipart data")
		reader := getPooledMultipartBufferWrapper(buf)

		p := make([]byte, 32)
		n, err := reader.Read(p)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if n == 0 {
			t.Error("Expected to read some bytes")
		}
	})

	t.Run("Read from nil inner", func(t *testing.T) {
		reader := &pooledBody{}
		p := make([]byte, 10)
		_, err := reader.Read(p)
		if err != io.EOF {
			t.Errorf("Expected io.EOF for nil buffer, got %v", err)
		}
	})

	t.Run("Read until EOF defers pool return to Close", func(t *testing.T) {
		buf := getMultipartBuffer()
		buf.WriteString("data")
		reader := getPooledMultipartBufferWrapper(buf)

		// Read everything
		_, _ = io.ReadAll(reader)

		// EOF must NOT release: net/http closes the body after reading it
		// (later, and on HTTP/2 from another goroutine). Releasing at EOF
		// would recycle the wrapper while a delayed Close can still arrive
		// and release a wrapper already reused by another request.
		if reader.inner == nil {
			t.Error("inner must stay owned after EOF read — release belongs to Close")
		}
		if reader.released.Load() {
			t.Error("released must stay false after EOF read — release belongs to Close")
		}

		_ = reader.Close()

		if reader.inner != nil {
			t.Error("Expected inner to be nil after Close")
		}
		if !reader.released.Load() {
			t.Error("Expected released to be true after Close")
		}
	})
}

// TestPooledBody_JSONBuffer validates JSON buffer pool read behavior.
func TestPooledBody_JSONBuffer(t *testing.T) {
	t.Run("Normal read", func(t *testing.T) {
		buf := bytes.NewBufferString(`{"key":"value"}`)
		reader := getPooledJSONBufferWrapper(buf)

		p := make([]byte, 64)
		n, err := reader.Read(p)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if n == 0 {
			t.Error("Expected to read some bytes")
		}
	})

	t.Run("Read from nil inner", func(t *testing.T) {
		reader := &pooledBody{}
		p := make([]byte, 10)
		_, err := reader.Read(p)
		if err != io.EOF {
			t.Errorf("Expected io.EOF for nil buffer, got %v", err)
		}
	})

	t.Run("Read until EOF defers pool return to Close", func(t *testing.T) {
		buf := getJSONBuffer()
		buf.WriteString(`{"test":true}`)
		reader := getPooledJSONBufferWrapper(buf)

		// Read everything
		_, _ = io.ReadAll(reader)

		// EOF must NOT release — see the matching multipart subtest for the
		// net/http close-after-EOF rationale.
		if reader.inner == nil {
			t.Error("inner must stay owned after EOF read — release belongs to Close")
		}
		if reader.released.Load() {
			t.Error("released must stay false after EOF read — release belongs to Close")
		}

		_ = reader.Close()

		if reader.inner != nil {
			t.Error("Expected inner to be nil after Close")
		}
		if !reader.released.Load() {
			t.Error("Expected released to be true after Close")
		}
	})
}

// TestPooledBody_Len pins the content-length probe shared by setContentLength:
// every wrapped inner type exposes its remaining length, and a nil/released
// wrapper reports zero.
func TestPooledBody_Len(t *testing.T) {
	if got := (&pooledBody{}).Len(); got != 0 {
		t.Errorf("nil-inner Len = %d, want 0", got)
	}
	r := getPooledStringsReader("hello")
	if got := r.(*pooledBody).Len(); got != 5 {
		t.Errorf("strings Len = %d, want 5", got)
	}
	b := getPooledBytesReader([]byte("1234567"))
	if got := b.(*pooledBody).Len(); got != 7 {
		t.Errorf("bytes Len = %d, want 7", got)
	}
}

// TestPooledStringsReader_NoReleaseOnEOF pins the release discipline of the
// request-body reader wrappers: the pool return must happen ONLY in Close,
// never at EOF in Read. net/http reads the body to EOF and closes it later
// (on HTTP/2, from the writeLoop goroutine); releasing at EOF would put the
// wrapper back in the pool while a delayed Close can still arrive, and after
// another request recycles the wrapper that stale Close would release the
// new request's live reader.
func TestPooledStringsReader_NoReleaseOnEOF(t *testing.T) {
	r := getPooledStringsReader("hello world")
	pr := r.(*pooledBody)

	if _, err := io.ReadAll(pr); err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if pr.released.Load() {
		t.Fatal("reader released at EOF: release must happen only in Close")
	}

	_ = pr.Close()
	if !pr.released.Load() {
		t.Fatal("Close did not release the reader back to the pool")
	}
}

// TestPooledReaders_ReadEOF consolidates the former TestPooledStringsReader
// and TestPooledBytesReader: normal reads deliver the full payload, and a
// read after EOF returns io.EOF. The strings reader's release discipline
// (pool return only in Close, never at EOF) is pinned separately by
// TestPooledStringsReader_NoReleaseOnEOF in this file.
func TestPooledReaders_ReadEOF(t *testing.T) {
	tests := []struct {
		name   string
		reader io.Reader
		want   string
	}{
		{"strings reader", getPooledStringsReader("hello world"), "hello world"},
		{"bytes reader", getPooledBytesReader([]byte("byte data")), "byte data"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := io.ReadAll(tt.reader)
			if err != nil {
				t.Fatalf("ReadAll error: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("read %q, want %q", data, tt.want)
			}
			// Second read must return EOF: the exhausted inner reader yields
			// EOF on every subsequent read (it is only nilled at Close).
			p := make([]byte, 10)
			if _, err := tt.reader.Read(p); err != io.EOF {
				t.Errorf("read after EOF: got %v, want io.EOF", err)
			}
		})
	}
}

// TestPooledLimitReader validates the limit reader behavior.
func TestPooledLimitReader(t *testing.T) {
	t.Run("Limit enforcement", func(t *testing.T) {
		src := bytes.NewBufferString("hello world")
		lr := getLimitReader(src, 5)
		defer putLimitReader(lr)

		data, err := io.ReadAll(lr)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if len(data) > 5 {
			t.Errorf("Expected at most 5 bytes, got %d", len(data))
		}
	})

	t.Run("Nil reader", func(t *testing.T) {
		putLimitReader(nil) // should not panic
	})
}

// TestPooledGzipReader_Close validates the pooled gzip reader close behavior.
func TestPooledGzipReader_Close(t *testing.T) {
	t.Run("Nil reader", func(t *testing.T) {
		r := &pooledGzipReader{}
		err := r.Close()
		if err != nil {
			t.Errorf("Expected nil error for nil reader, got %v", err)
		}
	})

	t.Run("Normal close", func(t *testing.T) {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		_, _ = gw.Write([]byte("test"))
		_ = gw.Close()

		gr, err := gzip.NewReader(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("Failed to create gzip reader: %v", err)
		}

		r := &pooledGzipReader{Reader: gr}
		err = r.Close()
		if err != nil {
			t.Errorf("Unexpected error on close: %v", err)
		}
		if r.Reader != nil {
			t.Error("Expected reader to be nil after close")
		}
	})
}

// TestPooledFlateReader validates the pooled flate reader behavior.
func TestPooledFlateReader(t *testing.T) {
	t.Run("Nil reader Read", func(t *testing.T) {
		r := &pooledFlateReader{reader: nil}
		p := make([]byte, 10)
		_, err := r.Read(p)
		if err != io.EOF {
			t.Errorf("Expected io.EOF, got %v", err)
		}
	})

	t.Run("Nil reader Close", func(t *testing.T) {
		r := &pooledFlateReader{reader: nil}
		err := r.Close()
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})
}

// io.Closer is tested for all pooled reader/buffer types via table-driven test.
func TestPooledReaders_Close(t *testing.T) {
	t.Run("StringsReader close before reading", func(t *testing.T) {
		reader := getPooledStringsReader("hello").(*pooledBody)
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if reader.inner != nil {
			t.Error("Expected inner to be nil after Close")
		}
	})

	t.Run("StringsReader double close", func(t *testing.T) {
		reader := getPooledStringsReader("hello").(*pooledBody)
		_ = reader.Close()
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error on double close: %v", err)
		}
	})

	t.Run("BytesReader close before reading", func(t *testing.T) {
		reader := getPooledBytesReader([]byte("hello")).(*pooledBody)
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if reader.inner != nil {
			t.Error("Expected inner to be nil after Close")
		}
	})

	t.Run("BytesReader double close", func(t *testing.T) {
		reader := getPooledBytesReader([]byte("hello")).(*pooledBody)
		_ = reader.Close()
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error on double close: %v", err)
		}
	})

	t.Run("MultipartBuffer close owned", func(t *testing.T) {
		buf := getMultipartBuffer()
		buf.WriteString("multipart data")
		reader := getPooledMultipartBufferWrapper(buf)
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if reader.inner != nil {
			t.Error("Expected inner to be nil after Close")
		}
	})

	t.Run("MultipartBuffer close nil", func(t *testing.T) {
		reader := &pooledBody{}
		reader.released.Store(true) // already released: Close must be a no-op
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("MultipartBuffer close not-owned", func(t *testing.T) {
		buf := bytes.NewBufferString("data")
		reader := getPooledMultipartBufferWrapper(buf)
		reader.released.Store(true)
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("MultipartBuffer double close", func(t *testing.T) {
		buf := getMultipartBuffer()
		buf.WriteString("data")
		reader := getPooledMultipartBufferWrapper(buf)
		_ = reader.Close()
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error on double close: %v", err)
		}
	})

	t.Run("JSONBuffer close owned", func(t *testing.T) {
		buf := getJSONBuffer()
		buf.WriteString(`{"test": true}`)
		reader := getPooledJSONBufferWrapper(buf)
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if reader.inner != nil {
			t.Error("Expected inner to be nil after Close")
		}
	})

	t.Run("JSONBuffer close nil", func(t *testing.T) {
		reader := &pooledBody{}
		reader.released.Store(true) // already released: Close must be a no-op
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("JSONBuffer close not-owned", func(t *testing.T) {
		buf := bytes.NewBufferString(`{"data":1}`)
		reader := getPooledJSONBufferWrapper(buf)
		reader.released.Store(true)
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("JSONBuffer double close", func(t *testing.T) {
		buf := getJSONBuffer()
		buf.WriteString(`{"test": true}`)
		reader := getPooledJSONBufferWrapper(buf)
		_ = reader.Close()
		if err := reader.Close(); err != nil {
			t.Errorf("Unexpected error on double close: %v", err)
		}
	})
}

// TestPooledBodyWrappers_ConcurrentClose pins the C3 regression: the JSON and
// multipart wrappers must release exactly once even when closers race — the
// delayed HTTP/2 writeLoop Close can arrive concurrently with the request
// lifecycle's Close. Under -race this also detects the data race the previous
// check-then-act bool guard allowed.
func TestPooledBodyWrappers_ConcurrentClose(t *testing.T) {
	run := func(name string, mk func() io.Closer) {
		t.Run(name, func(t *testing.T) {
			for i := 0; i < 64; i++ {
				c := mk()
				var start sync.WaitGroup
				start.Add(1)
				var wg sync.WaitGroup
				for g := 0; g < 4; g++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						start.Wait()
						_ = c.Close()
					}()
				}
				start.Done()
				wg.Wait()
			}
		})
	}
	run("JSONBuffer", func() io.Closer {
		buf := getJSONBuffer()
		buf.WriteString(`{"race":true}`)
		return getPooledJSONBufferWrapper(buf)
	})
	run("MultipartBuffer", func() io.Closer {
		buf := getMultipartBuffer()
		buf.WriteString("multipart race")
		return getPooledMultipartBufferWrapper(buf)
	})
}

// TestPooledReaders_ConcurrentClose pins the release guard against the
// delayed-close race: the transport's writeLoop goroutine may Close a request
// body concurrently with another Close from the request lifecycle. With the
// pre-atomic check-then-act flag this test trips the race detector and can
// double-Put the wrapper (one live object handed to two requests); the CAS
// guard makes exactly one closer win.
func TestPooledReaders_ConcurrentClose(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		for _, r := range []io.Reader{
			getPooledStringsReader("payload"),
			getPooledBytesReader([]byte("payload")),
		} {
			closer := r.(io.Closer)
			wg.Add(4)
			for g := 0; g < 4; g++ {
				go func() {
					defer wg.Done()
					_ = closer.Close() // best-effort; guard must make losers no-ops
				}()
			}
		}
	}
	wg.Wait()

	// Wrappers still behave after a (single) release: reads return EOF
	// instead of panicking on the nilled reader.
	rc := getPooledBytesReader([]byte("x")).(io.ReadCloser)
	_ = rc.Close()
	if _, err := rc.Read(make([]byte, 1)); err != io.EOF {
		t.Errorf("Read after Close = %v, want io.EOF", err)
	}
}
