package httpc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A planted symlink at the predictable "<dest>.httpc-tmp" name must be
// rejected before any O_TRUNC open: O_CREATE|O_TRUNC follows the link and
// would truncate whatever it points at, then rename the link entry onto the
// destination.
func TestWriteDownloadBody_RejectsSymlinkedTempSibling(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "file.bin")
	if err := os.WriteFile(dest, []byte("victim"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := dest + downloadTmpSuffix
	if err := os.Symlink(dest, link); err != nil {
		t.Skipf("symlink creation unavailable on this system: %v", err)
	}

	_, err := writeDownloadBody(downloadWriteParams{
		bodyReader: strings.NewReader("data"),
		filePath:   dest,
		opts:       &DownloadConfig{},
		statusCode: 200,
	})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v, want symlink rejection", err)
	}
	got, readErr := os.ReadFile(dest)
	if readErr != nil || string(got) != "victim" {
		t.Fatalf("victim altered: readErr=%v content=%q", readErr, got)
	}
}

// SaveToFile shares the same temp-sibling scheme and must share the check.
func TestSaveToFile_RejectsSymlinkedTempSibling(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "saved.bin")
	if err := os.WriteFile(dest, []byte("victim"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := dest + downloadTmpSuffix
	if err := os.Symlink(dest, link); err != nil {
		t.Skipf("symlink creation unavailable on this system: %v", err)
	}

	r := &Result{Response: &ResponseInfo{RawBody: []byte("hello")}}
	err := r.SaveToFile(dest)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v, want symlink rejection", err)
	}
	got, readErr := os.ReadFile(dest)
	if readErr != nil || string(got) != "victim" {
		t.Fatalf("victim altered: readErr=%v content=%q", readErr, got)
	}
}

// panickingReader simulates user code failing mid-copy (a panicking
// ProgressCallback unwinds through the same frames).
type panickingReader struct{}

func (panickingReader) Read([]byte) (int, error) { panic("boom") }

// A panic between open and the explicit closes must not leak the file
// handle: writeDownloadBody's panic-safety defer closes it during unwinding.
// On Windows a still-open (delete-pending) handle blocks reopening the path
// by name, which is what this test observes.
func TestWriteDownloadBody_PanicClosesFileHandle(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "panic.bin")

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_, _ = writeDownloadBody(downloadWriteParams{
			bodyReader: panickingReader{},
			filePath:   dest,
			opts:       &DownloadConfig{},
			statusCode: 200,
		})
	}()
	if recovered == nil {
		t.Fatal("expected panic to propagate to the caller")
	}

	tmp := dest + downloadTmpSuffix
	_ = os.Remove(tmp) // succeeds (delete-pending) even with a leaked handle
	// A leaked handle leaves the file delete-pending, so creating a new file
	// at the same path fails; with the defer's close it succeeds.
	if err := os.WriteFile(tmp, []byte("fresh"), filePermissions); err != nil {
		t.Fatalf("temp path not reusable (handle leaked?): %v", err)
	}
}
