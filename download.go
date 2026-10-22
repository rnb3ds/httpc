package httpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cybergodev/httpc/internal/engine"
)

// DownloadProgressCallback is called during file download to report progress.
// Parameters: downloaded bytes, total bytes, current speed in bytes/second.
type DownloadProgressCallback func(downloaded, total int64, speed float64)

// ChecksumAlgorithm specifies the hash algorithm for download integrity verification.
type ChecksumAlgorithm string

const (
	// ChecksumSHA256 uses SHA-256 for integrity verification.
	ChecksumSHA256 ChecksumAlgorithm = "sha256"

	// maxDrainBodySize caps how many response-body bytes are read and discarded
	// on error paths to allow connection reuse. Bounded to avoid spending
	// unbounded time/memory draining an oversized error body; the remainder is
	// left unread and the connection is not reused in that case.
	maxDrainBodySize = 1 << 20 // 1 MiB

	// downloadTmpSuffix names the temp sibling that non-resumed downloads
	// write to before an atomic rename into the final location (see
	// writeDownloadBody). A crashed download leaves this behind; the next
	// attempt O_TRUNCs it, so the suffix doubles as self-healing cleanup.
	downloadTmpSuffix = ".httpc-tmp"
)

// DownloadConfig configures file download behavior.
// Use DefaultDownloadConfig() to get a configuration with sensible defaults.
//
// Size limit: downloads are subject to the client's Security.MaxResponseBodySize
// (default 10 MB). A body exceeding the limit fails with a
// "streamed response body exceeds size limit" error — it is never silently
// truncated. Raise the limit on Config.Security for large-file workloads.
type DownloadConfig struct {
	// FilePath is the destination path for the downloaded file.
	FilePath string
	// ProgressCallback is called periodically during download to report progress.
	ProgressCallback DownloadProgressCallback
	// Overwrite allows overwriting an existing file at FilePath.
	Overwrite bool
	// ResumeDownload attempts to resume a previously interrupted download.
	ResumeDownload bool
	// Checksum is the expected hex-encoded checksum of the downloaded file.
	// When set, the file is verified after download completes. A mismatch
	// fails the download: fresh downloads remove their temp file (any
	// pre-existing destination is untouched), resumed downloads are
	// truncated back to the offset the session started at — preserving the
	// previously downloaded prefix for the next resume attempt.
	Checksum string
	// ChecksumAlgorithm specifies the hash algorithm for verification.
	// Currently only "sha256" is supported. Default: "sha256".
	ChecksumAlgorithm ChecksumAlgorithm
}

// DefaultDownloadConfig returns a DownloadConfig with default settings.
// Overwrite and ResumeDownload are both false by default.
// Caller must set FilePath before use.
//
// Example:
//
//	cfg := httpc.DefaultDownloadConfig()
//	cfg.FilePath = "/downloads/file.zip"
//	cfg.Overwrite = true
//	result, err := httpc.Download(context.Background(), url, cfg)
func DefaultDownloadConfig() *DownloadConfig {
	return &DownloadConfig{
		Overwrite:         false,
		ResumeDownload:    false,
		ChecksumAlgorithm: ChecksumSHA256,
	}
}

// DownloadResult contains information about a completed download.
type DownloadResult struct {
	// FilePath is the path where the file was saved.
	FilePath string
	// BytesWritten is the number of bytes written by this download. A resumed
	// download counts only the bytes it appended; the file's total size is
	// the resumed prefix plus BytesWritten (the final ProgressCallback
	// invocation reports that total).
	BytesWritten int64
	// Duration is the total time taken for the download.
	Duration time.Duration
	// AverageSpeed is the average download speed in bytes/second.
	AverageSpeed float64
	// StatusCode is the HTTP status code of the download response.
	StatusCode int
	// ContentLength is the Content-Length reported by the server.
	ContentLength int64
	// Resumed indicates whether the download was resumed from a previous partial download.
	Resumed bool
	// ResponseCookies contains cookies returned by the download response.
	ResponseCookies []*http.Cookie
	// ActualChecksum is the computed checksum of the downloaded file (hex-encoded).
	// Only set when DownloadConfig.Checksum is provided.
	ActualChecksum string
	// Proto is the HTTP protocol version of the response (e.g., "HTTP/1.1", "HTTP/2.0").
	Proto string
	// ResponseHeaders contains the response headers from the download.
	ResponseHeaders http.Header
	// RequestURL is the actual URL that was requested.
	RequestURL string
	// RequestMethod is the HTTP method used for the download.
	RequestMethod string
	// RequestHeaders contains the request headers that were sent.
	RequestHeaders http.Header
}

// Download downloads a file from url to the path specified in cfg, using the
// default client with the given context and request options.
//
// Download is the single canonical download entry point across the package,
// the Client interface, and DomainClient: one signature collapses the former
// {config} × {context} variant matrix. Callers that already hold a client or
// DomainClient use the Download method of the same signature.
//
// cfg must be non-nil; cfg.FilePath must be set (ErrEmptyFilePath otherwise).
// Use context.Background() when no cancellation or timeout is required. Pass
// options for headers, authentication, query parameters, etc.
// The download is subject to the client's Security.MaxResponseBodySize
// (default 10 MB); oversize bodies fail with an error — see DownloadConfig.
func Download(ctx context.Context, url string, cfg *DownloadConfig, options ...RequestOption) (*DownloadResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("download config cannot be nil")
	}
	client, options, err := resolvePackageClient(options)
	if err != nil {
		return nil, err
	}
	return client.Download(ctx, url, cfg, options...)
}

// Download downloads a file from url to the path specified in cfg, using the
// client's configuration with the given context and request options.
// cfg must be non-nil; cfg.FilePath must be set (ErrEmptyFilePath otherwise).
// Use DefaultDownloadConfig() as the starting point, then set FilePath and any
// of ProgressCallback / Overwrite / ResumeDownload / Checksum as needed.
func (c *clientImpl) Download(ctx context.Context, url string, cfg *DownloadConfig, options ...RequestOption) (*DownloadResult, error) {
	return c.downloadFile(ctx, url, cfg, options...)
}

// downloadLocks serializes downloads to the same destination path within this
// process (see downloadFile). Keyed by absolute path; entries are removed on
// release so repeatedly downloading to fresh paths (timestamped files) does
// not grow the map unboundedly.
var downloadLocks sync.Map // map[string]*sync.Mutex

// acquireDownloadLock tries to claim the download slot for path. ok is false
// when a download to the same path is already running in this process, in
// which case no lock is held and release is nil.
func acquireDownloadLock(path string) (release func(), ok bool) {
	key, err := filepath.Abs(path)
	if err != nil {
		key = filepath.Clean(path)
	}
	if runtime.GOOS == "windows" {
		// Windows paths are case-insensitive: without normalizing, two
		// spellings of one path ("FILE.bin" vs "file.bin") would take
		// different locks and bypass the serialization.
		key = strings.ToLower(key)
	}
	v, _ := downloadLocks.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	if !mu.TryLock() {
		return nil, false
	}
	return func() {
		mu.Unlock()
		// Best-effort entry cleanup: if another goroutine LoadOrStore'd this
		// same entry while the lock was held, CompareAndDelete fails and that
		// goroutine keeps using it — a later acquirer then creates a fresh
		// entry, degrading (only for that pair) to the pre-lock behavior. It
		// can never block or deadlock.
		downloadLocks.CompareAndDelete(key, mu)
	}, true
}

func (c *clientImpl) downloadFile(ctx context.Context, url string, opts *DownloadConfig, options ...RequestOption) (result *DownloadResult, err error) {
	// SEC-003: default panic safety net for the download path. Mirrors the guard in
	// clientImpl.Request. Deferred cleanups registered below (engine.ReleaseResponse,
	// bodyReader.Close) run during unwinding before this recover (LIFO), so streaming
	// resources are released even when a panic is converted to an error.
	defer func() {
		if r := recover(); r != nil {
			result = nil
			err = panicToError(r)
		}
	}()
	if opts == nil {
		return nil, fmt.Errorf("download config cannot be nil")
	}
	if opts.FilePath == "" {
		return nil, ErrEmptyFilePath
	}

	// Serialize downloads to the same destination path within this process:
	// two concurrent downloads would open the same temp sibling (O_TRUNC) or
	// append to the same file (O_APPEND) and corrupt each other's writes.
	// Fail fast with ErrDownloadInProgress instead of blocking — the caller
	// can retry when the first download finishes. Cross-process coordination
	// remains the caller's responsibility.
	unlock, locked := acquireDownloadLock(opts.FilePath)
	if !locked {
		return nil, fmt.Errorf("%w: %s", ErrDownloadInProgress, opts.FilePath)
	}
	defer unlock()

	filePath, resumeOffset, options, err := prepareResumeState(opts.FilePath, opts, options)
	if err != nil {
		return nil, err
	}

	// Use streaming mode to avoid buffering the entire response body into memory.
	streamOptions := append(slices.Clone(options), WithStreamBody(true))

	rawResp, err := c.executeRequest(ctx, "GET", url, streamOptions)
	if err != nil {
		return nil, fmt.Errorf("download request failed: %w", err)
	}
	if rawResp == nil {
		return nil, fmt.Errorf("download request returned nil response")
	}

	engResp, ok := rawResp.(*engine.Response)
	if !ok {
		// Non-engine responses come from middleware that wraps the engine.Response.
		// Downloads require direct access to the body reader, which is only
		// available on *engine.Response.
		releaseResponseMutator(rawResp)
		return nil, fmt.Errorf("download is not compatible with middleware that wraps ResponseMutator")
	}

	// Register the release defer BEFORE extracting fields: extractDownloadFields
	// reads several accessors on the pooled *engine.Response, and a panic there
	// would otherwise leak the response (the safety net above converts the panic
	// to an error but, without this defer already registered, ReleaseResponse
	// would never run). extractDownloadFields nils the body reader on the
	// response, so ReleaseResponse will not close it; the dedicated bodyReader
	// defer below owns the reader lifetime.
	defer engine.ReleaseResponse(engResp)
	df := extractDownloadFields(engResp)
	if df.bodyReader != nil {
		defer func() { _ = df.bodyReader.Close() }() // best-effort cleanup
	}

	resumed := resumeOffset > 0 && df.statusCode == http.StatusPartialContent

	// 416 gets its own diagnosis before the generic "no range support"
	// error: with a resume offset it typically means the local file already
	// matches (or exceeds) the remote size, not that the server lacks range
	// support. handleDownloadStatus drains the body and gives the specific
	// message; this also makes its 416 branch reachable.
	if resumeOffset > 0 && df.statusCode == http.StatusRequestedRangeNotSatisfiable {
		return nil, handleDownloadStatus(df.statusCode, df.bodyReader, resumeOffset)
	}

	// When resume was requested but the server did not return 206, first
	// distinguish WHY: a 2xx (200) means no range support, while a 4xx/5xx is
	// an ordinary error status whose specific diagnosis (and body preview)
	// comes from handleDownloadStatus — reporting "does not support range
	// requests" for a 404/500 would be misleading.
	if resumeOffset > 0 && !resumed && df.statusCode != http.StatusOK {
		return nil, handleDownloadStatus(df.statusCode, df.bodyReader, resumeOffset)
	}
	if resumeOffset > 0 && !resumed {
		// 200 instead of 206: no range support. Truncating the existing
		// partial file would silently destroy data the user intended to
		// resume, so fail and leave it untouched.
		drainBody(df.bodyReader)
		return nil, fmt.Errorf("server does not support range requests (status %d); cannot resume download", df.statusCode)
	}

	// A 206 response must start exactly at the local file length. Without
	// this check a server (or tampered response) starting at byte 0 would be
	// appended after the existing prefix, silently corrupting the file.
	var contentRangeTotal int64 = -1
	if resumed {
		total, err := validateResumeContentRange(df.responseHeaders, resumeOffset)
		if err != nil {
			drainBody(df.bodyReader)
			return nil, err
		}
		contentRangeTotal = total
	}

	// Validate response status
	if err := handleDownloadStatus(df.statusCode, df.bodyReader, resumeOffset); err != nil {
		return nil, err
	}

	if df.bodyReader == nil {
		return nil, fmt.Errorf("download response has no body reader")
	}

	downloadStart := time.Now()
	result, writeErr := writeDownloadBody(downloadWriteParams{
		bodyReader:        df.bodyReader,
		filePath:          filePath,
		opts:              opts,
		resumed:           resumed,
		resumeOffset:      resumeOffset,
		statusCode:        df.statusCode,
		contentLength:     df.contentLength,
		downloadStart:     downloadStart,
		responseCookies:   df.responseCookies,
		contentRangeTotal: contentRangeTotal,
	})
	if writeErr != nil {
		return nil, writeErr
	}
	result.Proto = df.proto
	result.ResponseHeaders = df.responseHeaders
	result.RequestURL = df.requestURL
	result.RequestMethod = df.requestMethod
	result.RequestHeaders = df.requestHeaders
	return result, nil
}

// prepareResumeState validates the file path and calculates resume state.
// Returns the validated file path, resume offset, updated options, and any error.
func prepareResumeState(filePath string, opts *DownloadConfig, options []RequestOption) (string, int64, []RequestOption, error) {
	validatedPath, err := prepareFilePath(filePath)
	if err != nil {
		return "", 0, nil, fmt.Errorf("failed to prepare file path: %w", err)
	}

	var resumeOffset int64
	if fileInfo, err := os.Stat(validatedPath); err == nil {
		if fileInfo.IsDir() {
			return "", 0, nil, fmt.Errorf("path is a directory, not a file: %s", validatedPath)
		}
		if !opts.Overwrite && !opts.ResumeDownload {
			return "", 0, nil, fmt.Errorf("%w: %s", ErrFileExists, validatedPath)
		}
		// ResumeDownload takes precedence over Overwrite when both are set:
		// the existing file is extended rather than replaced.
		if opts.ResumeDownload {
			resumeOffset = fileInfo.Size()
			rangeOption := WithHeader("Range", fmt.Sprintf("bytes=%d-", resumeOffset))
			combined := make([]RequestOption, len(options), len(options)+1)
			copy(combined, options)
			options = append(combined, rangeOption)
		}
	}

	return validatedPath, resumeOffset, options, nil
}

// downloadFields holds extracted fields from an engine.Response for download processing.
// Transfers body reader ownership to the caller.
type downloadFields struct {
	bodyReader      io.ReadCloser
	statusCode      int
	contentLength   int64
	responseCookies []*http.Cookie
	proto           string
	requestURL      string
	requestMethod   string
	responseHeaders http.Header
	requestHeaders  http.Header
}

// extractDownloadFields extracts download-relevant fields from an engine Response.
// Transfers body reader and header ownership to the caller, avoiding Clone
// allocations — ReleaseResponse will zero the Response afterward, so the
// transferred maps are the sole surviving reference. The caller must call
// engine.ReleaseResponse(engResp) after the body reader is no longer needed.
func extractDownloadFields(engResp *engine.Response) downloadFields {
	// Transfer body reader ownership; nil prevents ReleaseResponse from closing it.
	df := downloadFields{
		bodyReader:      engResp.RawBodyReader(),
		statusCode:      engResp.StatusCode(),
		contentLength:   engResp.ContentLength(),
		responseCookies: engResp.Cookies(),
		proto:           engResp.Proto(),
		requestURL:      engResp.RequestURL(),
		requestMethod:   engResp.RequestMethod(),
		responseHeaders: engResp.TransferHeaders(),
		requestHeaders:  engResp.TransferRequestHeaders(),
	}
	engResp.SetRawBodyReader(nil)

	return df
}

// drainBody reads and discards up to maxDrainBodySize bytes so an unused
// response body does not kill connection reuse. Best-effort by design —
// single definition for the four drain sites in the download path.
func drainBody(r io.Reader) {
	if r == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(r, maxDrainBodySize)) // best-effort drain for connection reuse
}

// handleDownloadStatus validates the HTTP response status for a download request.
// Returns an error for 416 Range Not Satisfiable (with body drained),
// an error for unexpected status codes (with body drained),
// or nil for 200 OK / 206 Partial Content (body left intact for caller).
func handleDownloadStatus(statusCode int, bodyReader io.Reader, resumeOffset int64) error {
	if resumeOffset > 0 && statusCode == http.StatusRequestedRangeNotSatisfiable {
		drainBody(bodyReader)
		return fmt.Errorf("server cannot satisfy range request (416): local file already has %d bytes — the remote file may be smaller or unchanged; delete the local file or use Overwrite to restart", resumeOffset)
	}

	if statusCode != http.StatusOK && statusCode != http.StatusPartialContent {
		var bodyPreview string
		if bodyReader != nil {
			// io.ReadAll handles short reads; a single Read may legally
			// return fewer bytes than requested, which used to leave the
			// preview empty on fragmented network bodies.
			preview, _ := io.ReadAll(io.LimitReader(bodyReader, 512))
			if len(preview) > 0 {
				bodyPreview = string(preview)
				if len(bodyPreview) > 200 {
					bodyPreview = bodyPreview[:200] + "..."
				}
			}
			drainBody(bodyReader)
		}
		if bodyPreview != "" {
			return fmt.Errorf("unexpected status code: %d: %s", statusCode, bodyPreview)
		}
		return fmt.Errorf("unexpected status code: %d", statusCode)
	}

	return nil
}

// validateResumeContentRange verifies a 206 response honors the resume offset.
// RFC 9110 requires Content-Range on 206 responses; a start different from the
// local file length means the body begins at another byte, and appending it
// would silently corrupt the file.
//
// It also returns the declared total size when the server provides one
// ("bytes <start>-<end>/<total>"; total may be "*" or absent, in which case
// -1 is returned) so the copy loop can detect a short body.
func validateResumeContentRange(headers http.Header, resumeOffset int64) (int64, error) {
	cr := headers.Get("Content-Range")
	if cr == "" {
		return 0, fmt.Errorf("resume failed: 206 response is missing Content-Range header")
	}
	// Format: "bytes <start>-<end>/<total>" (total may be "*").
	var start int64
	if n, err := fmt.Sscanf(cr, "bytes %d-", &start); err != nil || n != 1 {
		return 0, fmt.Errorf("resume failed: malformed Content-Range %q", cr)
	}
	if start != resumeOffset {
		return 0, fmt.Errorf("resume failed: server returned range starting at byte %d, local file has %d bytes", start, resumeOffset)
	}
	if _, totalStr, ok := strings.Cut(cr, "/"); ok {
		if total, err := strconv.ParseInt(strings.TrimSpace(totalStr), 10, 64); err == nil {
			return total, nil
		}
	}
	return -1, nil
}

// downloadWriteParams groups writeDownloadBody's inputs; nine positional
// parameters were easy to mis-order at call sites.
type downloadWriteParams struct {
	bodyReader      io.Reader
	filePath        string
	opts            *DownloadConfig
	resumed         bool
	resumeOffset    int64
	statusCode      int
	contentLength   int64
	downloadStart   time.Time
	responseCookies []*http.Cookie
	// contentRangeTotal is the total size declared by a 206 response's
	// Content-Range (-1 when unknown); a short body is detected against it.
	contentRangeTotal int64
}

// writeDownloadBody streams the response body to a file and returns download statistics.
//
// SECURITY: non-resumed downloads write to a "<path>.httpc-tmp" sibling and are
// atomically renamed into place only after the body completes and any checksum
// verifies. Both the destination (prepareFilePath) and the predictable temp
// sibling (Lstat'd before the O_TRUNC open) reject symlinks, and an
// interrupted or checksum-failing download never truncates the destination.
// Resumed downloads must append to the destination itself and keep that
// pre-validated path.
func writeDownloadBody(p downloadWriteParams) (*DownloadResult, error) {
	bodyReader, filePath, opts := p.bodyReader, p.filePath, p.opts
	resumed, resumeOffset, statusCode := p.resumed, p.resumeOffset, p.statusCode
	contentLength, downloadStart, responseCookies := p.contentLength, p.downloadStart, p.responseCookies
	contentRangeTotal := p.contentRangeTotal
	// Validate checksum algorithm BEFORE touching the destination file.
	// A configuration error must not truncate (O_TRUNC) an existing file.
	// Unconditional in Checksum: a typo'd algorithm with no Checksum set
	// previously passed silently, leaving the caller believing a checksum
	// would be verified.
	if opts.ChecksumAlgorithm != "" && opts.ChecksumAlgorithm != ChecksumSHA256 {
		return nil, fmt.Errorf("unsupported checksum algorithm: %s", opts.ChecksumAlgorithm)
	}

	// Seed the checksum BEFORE opening the destination for writing: an error
	// while hashing the resume prefix must not leak the destination's append
	// handle (which locks the file on Windows until the GC finalizer runs).
	var hasher hash.Hash
	if opts.Checksum != "" {
		// Algorithm already validated above; both SHA-256 and the empty
		// default map to SHA-256.
		hasher = sha256.New()
		// When resuming, seed the hasher with the already-downloaded prefix so
		// the final sum covers the complete file — opts.Checksum is by
		// convention the hash of the whole content, while the response body
		// only carries the bytes after resumeOffset.
		if resumed {
			prefix, openErr := os.Open(filePath)
			if openErr != nil {
				return nil, fmt.Errorf("failed to open existing file for checksum: %w", openErr)
			}
			_, hashErr := io.Copy(hasher, prefix)
			_ = prefix.Close() // best-effort; read-only handle
			if hashErr != nil {
				return nil, fmt.Errorf("failed to hash existing file prefix: %w", hashErr)
			}
		}
	}

	// Non-resumed writes target the temp sibling; the final rename happens
	// after checksum verification below. Cleanup paths remove the temp file,
	// leaving the destination (if any) untouched.
	writePath := filePath
	if !resumed {
		writePath = filePath + downloadTmpSuffix
		// SECURITY: mirror prepareFilePath's destination check on the temp
		// sibling. The name is fully predictable and O_CREATE|O_TRUNC follows
		// a symlink: a planted "<dest>.httpc-tmp -> elsewhere" would truncate
		// whatever it points at, then rename the link onto the destination.
		if err := rejectSymlink(writePath); err != nil {
			return nil, err
		}
	}

	var file *os.File
	var err error
	if resumed {
		file, err = os.OpenFile(writePath, os.O_WRONLY|os.O_APPEND, filePermissions)
	} else {
		file, err = os.OpenFile(writePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePermissions)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	// Panic safety net: io.Copy below drives the caller's ProgressCallback
	// (user code that may panic); the explicit Close calls on the error paths
	// must keep running before their os.Remove (an open handle keeps the file
	// undeletable on Windows), so this defer only fires when a panic skips
	// them — otherwise the fd is held until the GC finalizer runs.
	defer func() {
		if file != nil {
			_ = file.Close() // best-effort: only reached when a panic skipped the explicit closes
		}
	}()
	// Stream body directly from network to file — no full-body buffering.
	// When checksum verification is requested, hash the data as it passes through.
	var writer io.Writer = file
	if hasher != nil {
		writer = io.MultiWriter(file, hasher)
	}
	if opts.ProgressCallback != nil {
		totalSize := downloadTotalSize(contentLength, resumeOffset, resumed)
		writer = &progressWriter{
			w:            writer,
			callback:     opts.ProgressCallback,
			total:        totalSize,
			offset:       resumeOffset,
			startTime:    time.Now(),
			lastCallback: time.Now(),
		}
	}

	bytesWritten, err := io.Copy(writer, bodyReader)
	if err != nil {
		_ = file.Close() // best-effort cleanup on write failure
		if !resumed {
			_ = os.Remove(writePath) // best-effort cleanup of partial file
		}
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Sync and close file before potential checksum-based removal.
	// On failure, remove the partial file (unless resuming, where the
	// pre-existing bytes should be preserved for the next resume attempt).
	if syncErr := file.Sync(); syncErr != nil {
		_ = file.Close() // best-effort cleanup on sync failure
		if !resumed {
			_ = os.Remove(writePath) // don't leave a truncated/corrupt file
		}
		return nil, fmt.Errorf("failed to sync file: %w", syncErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		if !resumed {
			_ = os.Remove(writePath)
		}
		return nil, fmt.Errorf("failed to close file: %w", closeErr)
	}
	file = nil // closed above; keeps the panic-safety defer a no-op

	// A 206 response that declared a total must deliver the full remaining
	// range: io.Copy treats a short body as a normal EOF, and without this
	// check a truncated file would report success (unless a checksum
	// happens to catch it).
	if contentRangeTotal > 0 && resumeOffset+bytesWritten != contentRangeTotal {
		if resumed {
			// Mirror the checksum-mismatch posture: restore the file to its
			// pre-attempt state so a later resume can retry.
			if truncErr := os.Truncate(filePath, resumeOffset); truncErr != nil {
				return nil, fmt.Errorf("server sent %d bytes, Content-Range declared %d total; failed to restore file to offset %d: %w",
					resumeOffset+bytesWritten, contentRangeTotal, resumeOffset, truncErr)
			}
		} else {
			_ = os.Remove(writePath)
		}
		return nil, fmt.Errorf("server sent %d bytes; Content-Range declared %d total — download truncated",
			resumeOffset+bytesWritten, contentRangeTotal)
	}

	// Compute actual checksum if hashing was enabled
	var actualChecksum string
	if hasher != nil {
		actualChecksum = hex.EncodeToString(hasher.Sum(nil))
	}

	// Verify checksum if expected value is provided. TrimSpace on the
	// expected value: checksums pasted from config files/CLIs routinely
	// carry a trailing newline, and failing on whitespace — with an error
	// that shows the "wrong" hash — misleads more than it helps.
	expectedChecksum := strings.ToLower(strings.TrimSpace(opts.Checksum))
	if expectedChecksum != "" && actualChecksum != expectedChecksum {
		if resumed {
			// Preserve the pre-existing prefix: truncate back to the offset
			// this session started at, restoring the file to exactly its
			// pre-attempt state so a later resume can retry from there.
			// Deleting the whole file — as the fresh-download branch below
			// does — would also destroy the previously downloaded prefix the
			// resume feature exists to protect (and which the write/sync
			// error paths above deliberately preserve). The mismatch may be
			// caused solely by the bytes appended in this attempt.
			if truncErr := os.Truncate(filePath, resumeOffset); truncErr != nil {
				return nil, fmt.Errorf("checksum mismatch (expected %s, got %s) and failed to restore file to offset %d: %w",
					expectedChecksum, actualChecksum, resumeOffset, truncErr)
			}
			return nil, fmt.Errorf("checksum mismatch: expected %s, got %s; file truncated to previous offset %d for a fresh resume",
				expectedChecksum, actualChecksum, resumeOffset)
		}
		_ = os.Remove(writePath) // remove corrupted download (fresh downloads write to the temp sibling)
		return nil, fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}

	duration := time.Since(downloadStart)
	avgSpeed := calculateSpeed(bytesWritten, duration)

	// Final callback with complete stats, BEFORE the rename: every stat
	// (bytes, total, speed) is already final here, and a panic in user
	// callback code must not flip an already-verified download into an error
	// after the file has landed at its final path — "callback panicked"
	// implies "file not finalized", same as during the copy.
	if opts.ProgressCallback != nil {
		totalSize := downloadTotalSize(contentLength, resumeOffset, resumed)
		opts.ProgressCallback(resumeOffset+bytesWritten, totalSize, avgSpeed)
	}

	// Atomically move the verified temp file into its final location.
	if !resumed {
		if renameErr := os.Rename(writePath, filePath); renameErr != nil {
			_ = os.Remove(writePath) // best-effort cleanup
			return nil, fmt.Errorf("failed to finalize download %s: %w", filePath, renameErr)
		}
	}

	return &DownloadResult{
		FilePath:        filePath,
		BytesWritten:    bytesWritten,
		Duration:        duration,
		AverageSpeed:    avgSpeed,
		StatusCode:      statusCode,
		ContentLength:   contentLength,
		Resumed:         resumed,
		ResponseCookies: responseCookies,
		ActualChecksum:  actualChecksum,
	}, nil
}

func calculateSpeed(bytes int64, duration time.Duration) float64 {
	if duration.Seconds() > 0 {
		return float64(bytes) / duration.Seconds()
	}
	return 0
}

// progressWriter wraps an io.Writer to invoke progress callbacks during download.
// Callbacks fire at most once per progressInterval to avoid overhead on fast networks.
type progressWriter struct {
	w            io.Writer
	callback     DownloadProgressCallback
	total        int64
	offset       int64
	written      int64
	startTime    time.Time
	lastCallback time.Time
}

// downloadTotalSize computes the total size reported to progress callbacks:
// the response Content-Length (negative for chunked responses) plus the
// pre-existing resumed prefix, when the server reports a total at all.
// Shared by the streaming progressWriter setup and the final callback —
// previously two identical copies that could drift.
func downloadTotalSize(contentLength, resumeOffset int64, resumed bool) int64 {
	total := contentLength
	if resumed && total > 0 {
		total += resumeOffset
	}
	return total
}

const progressInterval = 200 * time.Millisecond

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	if n > 0 {
		pw.written += int64(n)
		if now := time.Now(); now.Sub(pw.lastCallback) >= progressInterval {
			speed := calculateSpeed(pw.written, now.Sub(pw.startTime))
			pw.callback(pw.offset+pw.written, pw.total, speed)
			pw.lastCallback = now
		}
	}
	return n, err
}
