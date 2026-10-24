package httpc

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// Windows-reserved-path tests. The pure helpers (rejectAlternateDataStream,
// rejectReservedDeviceName) are exercised unconditionally — they encode the
// Windows policy itself and carry no runtime.GOOS branch — so Linux CI also
// covers the policy logic. The prepareFilePath integration cases only assert
// on windows, where the policy is active (elsewhere the check is a
// deliberate no-op so ordinary names like "com1" stay legal).

func TestRejectAlternateDataStream(t *testing.T) {
	allowed := []string{
		`C:\Users\me\report.pdf`,
		`C:\Users\me\data.tar.gz`,
		`C:`,
		`C:/Users/me/report.pdf`,
	}
	for _, p := range allowed {
		if err := rejectAlternateDataStream(p); err != nil {
			t.Errorf("rejectAlternateDataStream(%q) = %v, want nil", p, err)
		}
	}

	blocked := []string{
		`C:\Users\me\file.txt:ads`,      // classic ADS
		`C:\Users\me\file.txt:$DATA`,    // explicit stream name
		`C:\Users\me\dir:name\file.txt`, // colon in a directory component
		`file.txt:ads`,                  // relative form
		`C:\Users\me\a:b\c:d`,           // multiple colons
		`C:\Users\me\file.txt:hidden`,   // trailing stream
	}
	for _, p := range blocked {
		if err := rejectAlternateDataStream(p); err == nil {
			t.Errorf("rejectAlternateDataStream(%q) = nil, want error", p)
		}
	}
}

func TestRejectReservedDeviceName(t *testing.T) {
	blocked := []string{
		`CON`, `con`, `Con`,
		`CON.txt`, `con.tar.gz`, `NUL.log`,
		`COM1`, `com9`, `LPT1`, `lpt9`,
		`PRN`, `AUX`,
		`C:\data\CON`,           // absolute, final component only
		`C:\data\COM1.out`,      // device + extension
		`downloads/NUL.txt`,     // forward separators
		`NUL.`, `CON `, `NUL  `, // trailing dot/spaces
	}
	for _, p := range blocked {
		if err := rejectReservedDeviceName(p); err == nil {
			t.Errorf("rejectReservedDeviceName(%q) = nil, want error", p)
		}
	}

	allowed := []string{
		`console.log`, `connect.txt`, `contacts.csv`, // share a prefix, not equal
		`config.json`, `compute.go`,
		`C:\data\console.log`,
		`context_notes.txt`,
		``,
		`C:\`, `C:\data\`, // trailing separator: empty base component
		`pronto.txt`, `auxiliary.go`,
	}
	for _, p := range allowed {
		if err := rejectReservedDeviceName(p); err != nil {
			t.Errorf("rejectReservedDeviceName(%q) = %v, want nil", p, err)
		}
	}
}

func TestPrepareFilePath_WindowsReserved(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("DOS device / ADS filtering is active on windows only")
	}
	cases := []string{
		`CON`,
		`con.txt`,
		filepath.Join(t.TempDir(), "COM1"),
		filepath.Join(t.TempDir(), "report.pdf") + ":ads",
	}
	for _, p := range cases {
		_, err := prepareFilePath(p)
		if err == nil {
			t.Errorf("prepareFilePath(%q) = nil error, want rejection", p)
		}
	}
}

// TestSystemPathsPerGOOS pins the per-platform system-path denylists. The
// lists are deliberately asymmetric: darwin blocks /usr and /var wholesale
// (both SIP-protected — user writes there fail anyway), while linux blocks
// the system-OWNED subtrees under /var individually and keeps /usr/local and
// /var/tmp open (user-writable by FHS design). A future edit that drops or
// widens an entry must update this pin consciously.
func TestSystemPathsPerGOOS(t *testing.T) {
	paths := getSystemPaths()
	if len(paths) == 0 {
		t.Fatal("getSystemPaths returned no entries")
	}
	mustContain := map[string][]string{
		"windows": {"c:\\windows\\"},
		"darwin":  {"/system/", "/usr/", "/var/"},
		"linux":   {"/etc/", "/usr/bin/", "/var/lib/", "/var/log/", "/var/run/", "/var/spool/"},
	}[runtime.GOOS]
	if mustContain == nil {
		t.Skipf("no denylist pin for GOOS=%s", runtime.GOOS)
	}
	for _, want := range mustContain {
		if !slices.Contains(paths, want) {
			t.Errorf("GOOS=%s denylist missing %q: %v", runtime.GOOS, want, paths)
		}
	}
	if runtime.GOOS == "linux" {
		for _, open := range []string{"/usr/local/", "/var/tmp/"} {
			if slices.Contains(paths, open) {
				t.Errorf("linux denylist must not block the user-writable %q", open)
			}
		}
	}
}
