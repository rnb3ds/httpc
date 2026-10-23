package httpc

// File-path security for downloads and Result.SaveToFile: UNC blocking,
// control-character filtering, system-directory protection, path-traversal
// detection, and symlink defense. Extracted from download.go so the download
// orchestration (state machine, ranges, checksums, progress) and the path
// policy it depends on can evolve separately — SaveToFile (result.go) shares
// the same policy through prepareFilePath and rejectSymlink.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	maxFilePathLen  = 4096
	dirPermissions  = 0755
	filePermissions = 0644
)

// getSystemPaths returns platform-specific system paths that should be protected.
func getSystemPaths() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{
			"c:\\windows\\", "c:\\system32\\",
			"c:\\program files\\", "c:\\programdata\\",
			"c:\\program files (x86)\\",
			// Env-var patterns, expanded at check time by isSystemPath via
			// os.ExpandEnv. Go only expands ${VAR}/$VAR syntax, NOT Windows
			// %VAR%, so the brace form is required here — a previous %VAR%
			// form expanded to itself and the branch was dead code. These
			// catch installs on a non-C drive where the literals above miss.
			"${SystemRoot}", "${windir}", "${ProgramFiles}", "${ProgramFiles(x86)}",
		}
	case "darwin":
		return []string{
			"/system/", "/library/", "/applications/",
			"/usr/", "/bin/", "/sbin/", "/etc/", "/var/",
			// The canonical symlink targets of /etc and /var: a path that
			// arrives already resolved (or hand-written) as /private/... must
			// be gated just like the symlink spelling.
			"/private/etc/", "/private/var/",
		}
	case "linux":
		fallthrough
	default:
		return []string{
			"/etc/", "/sys/", "/proc/", "/dev/", "/boot/",
			"/root/",
			"/usr/bin/", "/usr/sbin/", "/bin/", "/sbin/",
			"/lib/", "/lib32/", "/lib64/", "/usr/lib/", "/usr/lib32/", "/usr/lib64/",
			"/run/", "/var/run/", "/sys/fs/",
		}
	}
}

// normalizedSystemEntry holds a pre-normalized system path for fast comparison.
// envPattern is non-empty only on Windows for env-var patterns (e.g., "${SystemRoot}").
type normalizedSystemEntry struct {
	normalized string
	envPattern string // original "${VAR}" pattern, empty for literal paths
}

var (
	systemPathsOnce   sync.Once
	cachedSystemPaths []normalizedSystemEntry
)

// initNormalizedSystemPaths pre-computes normalized system paths once.
// Paths are cleaned, lowercased (Windows), and separator-normalized at init
// time so isSystemPath only needs a single HasPrefix check per entry.
func initNormalizedSystemPaths() {
	raw := getSystemPaths()
	cachedSystemPaths = make([]normalizedSystemEntry, len(raw))
	for i, p := range raw {
		// Windows env-var patterns (e.g., "${SystemRoot}") are expanded at
		// check time, so store the original pattern and skip static normalization.
		if strings.HasPrefix(p, "${") && runtime.GOOS == "windows" {
			cachedSystemPaths[i] = normalizedSystemEntry{envPattern: p}
			continue
		}
		p = filepath.Clean(p)
		// Windows AND macOS compare case-insensitively: the lists are stored
		// lowercase, and the default APFS/HFS+ filesystem resolves /Library
		// and /library identically — a lowercase-only entry list with a
		// case-SENSITIVE input comparison previously made every macOS entry
		// but "/" inert ("/Library/x" never matched "/library/"). Linux
		// filesystems are case-sensitive and keep exact comparison.
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			p = strings.ToLower(p)
		}
		if runtime.GOOS == "windows" {
			p = strings.ReplaceAll(p, "/", "\\")
		} else {
			p = strings.ReplaceAll(p, "\\", "/")
		}
		// Ensure trailing separator to prevent prefix collision
		// (e.g., "C:\Windows" must not match "C:\WindowsEvil").
		if !strings.HasSuffix(p, string(filepath.Separator)) {
			p += string(filepath.Separator)
		}
		cachedSystemPaths[i] = normalizedSystemEntry{normalized: p}
	}
}

// prepareFilePath validates and prepares file paths with security checks.
// Returns the validated absolute path that should be used for all file operations.
// SECURITY: This function implements multiple layers of protection:
// 1. UNC path blocking (prevents network resource access)
// 2. Control character filtering
// 3. Windows-only hazards: DOS device names and NTFS alternate data streams
// 4. System path protection
// 5. Path traversal detection
// 6. Symlink attack prevention
func prepareFilePath(filePath string) (string, error) {
	filePathLen := len(filePath)
	if filePathLen == 0 {
		return "", ErrEmptyFilePath
	}
	if filePathLen > maxFilePathLen {
		return "", fmt.Errorf("file path too long (max %d)", maxFilePathLen)
	}

	// Check for UNC paths
	if filePathLen >= 2 {
		if (filePath[0] == '\\' && filePath[1] == '\\') || (filePath[0] == '/' && filePath[1] == '/') {
			return "", fmt.Errorf("UNC paths not allowed for security")
		}
	}

	// Validate characters
	for i := range filePathLen {
		c := filePath[i]
		if c < 0x20 || c == 0x7F || c == 0 {
			return "", fmt.Errorf("file path contains invalid characters at position %d", i)
		}
	}

	// Windows-only hazards (no-op elsewhere): DOS device names and NTFS
	// alternate data streams. Checked on the raw input, before Clean — the
	// device-name comparison wants the caller's final path component, and
	// colon positions are Clean-invariant anyway.
	if err := checkWindowsReservedPath(filePath); err != nil {
		return "", err
	}

	cleanPath := filepath.Clean(filePath)
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	// SECURITY: Check for symlinks to prevent symlink attacks
	// An attacker could create a symlink pointing to a sensitive file
	// and trick the application into writing to that file
	if fi, err := os.Lstat(absPath); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symlink paths not allowed for security")
		}
	}

	// Check for system path access (direct path check; symlink targets
	// are checked separately by checkParentDirSymlinks below).
	if isSystemPath(absPath) {
		return "", fmt.Errorf("system path access denied for security")
	}

	// Check for path traversal: after filepath.Clean, only paths starting with ".."
	// indicate traversal above CWD. Filenames like "backup..zip" are safe.
	if !filepath.IsAbs(filePath) && strings.HasPrefix(cleanPath, "..") {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory: %w", err)
		}
		wdAbs, err := filepath.Abs(wd)
		if err != nil {
			return "", fmt.Errorf("failed to resolve working directory: %w", err)
		}

		if !strings.HasPrefix(absPath+string(filepath.Separator), wdAbs+string(filepath.Separator)) {
			return "", fmt.Errorf("path traversal detected: path outside working directory")
		}
	}

	// SECURITY: Check parent directory for symlinks as well, complementing the
	// write-to-temp-then-rename strategy in writeDownloadBody: the destination
	// file is only created via rename of a pre-validated temp sibling, so a
	// symlink swapped in after this check is never opened for writing (the
	// residual TOCTOU window affects resumed downloads, which must append to
	// the destination itself).
	dir := filepath.Dir(absPath)
	if dir != absPath { // Avoid infinite recursion at root
		if err := checkParentDirSymlinks(dir, 0); err != nil {
			return "", err
		}
	}

	// Create directories
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return "", fmt.Errorf("failed to create directories: %w", err)
	}

	return absPath, nil
}

// checkParentDirSymlinks recursively checks if any parent directory is a symlink
// to prevent symlink-based path traversal attacks.
func checkParentDirSymlinks(dir string, depth int) error {
	const maxDepth = 32
	if depth > maxDepth {
		return fmt.Errorf("directory depth limit exceeded")
	}

	// Resolve the directory to its real path
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		// If the directory doesn't exist yet, that's okay - it will be created
		if os.IsNotExist(err) {
			// Check parent recursively
			parent := filepath.Dir(dir)
			if parent != dir {
				return checkParentDirSymlinks(parent, depth+1)
			}
			return nil
		}
		return fmt.Errorf("failed to evaluate symlinks: %w", err)
	}

	// If the resolved path differs from the original, a symlink was involved
	// Check if the resolved path is in a system directory
	if resolvedDir != dir {
		if isSystemPath(resolvedDir) {
			return fmt.Errorf("symlink resolves to system path")
		}
	}

	return nil
}

// isSystemPath checks whether the given absolute path falls within a protected
// system directory. The caller must supply an already-resolved absolute path
// (e.g., from prepareFilePath) — no redundant filepath.Abs is performed.
func isSystemPath(absPath string) bool {
	systemPathsOnce.Do(initNormalizedSystemPaths)

	// Normalize the input path once for comparison
	pathNorm := absPath
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		pathNorm = strings.ToLower(pathNorm) // see initNormalizedSystemPaths
	}
	if runtime.GOOS == "windows" {
		pathNorm = strings.ReplaceAll(pathNorm, "/", "\\")
	} else {
		pathNorm = strings.ReplaceAll(pathNorm, "\\", "/")
	}
	// Ensure trailing separator for consistent prefix matching
	if !strings.HasSuffix(pathNorm, string(filepath.Separator)) {
		pathNorm += string(filepath.Separator)
	}

	for _, entry := range cachedSystemPaths {
		if entry.envPattern != "" {
			// Windows env-var pattern: expand at check time
			expanded := os.ExpandEnv(entry.envPattern)
			if expanded == entry.envPattern {
				continue // env var not set
			}
			normalized := strings.TrimRight(strings.ToLower(expanded), "\\") + "\\"
			if strings.HasPrefix(pathNorm, normalized) {
				return true
			}
			continue
		}
		if strings.HasPrefix(pathNorm, entry.normalized) {
			return true
		}
	}

	return false
}

// rejectSymlink reports an error when path itself is a symlink. Deliberately
// Lstat (the link, not its target): the destination and the predictable
// "<dest>.httpc-tmp" sibling are probed before an O_CREATE|O_TRUNC open,
// which would follow a planted link and truncate whatever it points at.
// Shared by writeDownloadBody and Result.SaveToFile — the check was
// previously maintained as two copies.
func rejectSymlink(path string) error {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlink paths not allowed for security")
	}
	return nil
}

// windowsReservedDeviceNames lists the legacy DOS device names that Windows
// resolves to devices even when the name carries an extension or trailing
// dots/spaces ("CON.txt" opens the console, "COM1 " the serial port): an
// os.OpenFile(O_CREATE|O_TRUNC) against such a name writes the response body
// to the device instead of a file, silently. Read-only after initialization —
// must not be mutated (concurrent reads).
var windowsReservedDeviceNames = func() map[string]bool {
	names := []string{"CON", "PRN", "AUX", "NUL"}
	for i := 1; i <= 9; i++ {
		names = append(names, fmt.Sprintf("COM%d", i), fmt.Sprintf("LPT%d", i))
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}()

// checkWindowsReservedPath rejects two Windows-only filename hazards the
// generic character filter cannot see: NTFS alternate data streams (a colon
// beyond a leading drive letter — "file.txt:stream" — where the body would
// vanish into a hidden stream) and DOS device names in the final component
// ("CON", "COM1", …). It is a no-op on non-Windows platforms, where colons
// and these names are ordinary filename characters ("com1" is a legal, common
// Linux filename; blocking it there would be a false positive).
func checkWindowsReservedPath(path string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	if err := rejectAlternateDataStream(path); err != nil {
		return err
	}
	if err := rejectShortNameComponents(path); err != nil {
		return err
	}
	return rejectReservedDeviceName(path)
}

// rejectShortNameComponents rejects Windows 8.3 short-name aliases in the
// DIRECTORY portion of the path ("C:\PROGRA~1\evil\x.exe" is an alias of
// "C:\Program Files"). The system-path and symlink checks compare literal
// path text, so a short-name alias of a system directory bypasses them. A
// short name is "prefix~digits" with the whole base at most 8 characters and
// an extension of at most 3; the FINAL component is exempt — the file being
// created may legitimately carry such a name, and only directory components
// can alias an existing directory. Long legitimate names like "notes~2024"
// do not match (their base exceeds 8 characters).
func rejectShortNameComponents(path string) error {
	dir := path[:strings.LastIndexAny(path, `\/`)+1]
	start := 0
	for i := 0; i <= len(dir); i++ {
		if i == len(dir) || dir[i] == '\\' || dir[i] == '/' {
			if isShortNameComponent(dir[start:i]) {
				return fmt.Errorf("Windows 8.3 short-name component %q not allowed (aliases defeat path security checks); use the full path", dir[start:i])
			}
			start = i + 1
		}
	}
	return nil
}

// isShortNameComponent reports whether comp looks like a Windows 8.3 alias
// ("PROGRA~1", "DOCUME~1", "MYFILE~1.TXT"): a base of at most 8 characters
// containing a '~' followed by digit(s) that end the base, and an extension
// of at most 3 characters.
func isShortNameComponent(comp string) bool {
	if len(comp) == 0 || len(comp) > 12 {
		return false
	}
	base, ext, hasExt := strings.Cut(comp, ".")
	tilde := strings.IndexByte(base, '~')
	if tilde <= 0 || tilde == len(base)-1 {
		return false // no '~', empty prefix (a generated alias always has
		// one), or nothing after the '~'
	}
	if strings.Contains(base[:tilde], "~") {
		return false
	}
	for i := tilde + 1; i < len(base); i++ {
		if base[i] < '0' || base[i] > '9' {
			return false
		}
	}
	return len(base) <= 8 && (!hasExt || len(ext) <= 3)
}

// rejectAlternateDataStream rejects any colon that is not the drive-letter
// separator of a local path. A valid local path has its first colon at index
// 1 ("C:" or "C:\…" / "C:/…"); a UNC path would start with a separator and is
// already rejected upstream. Everything else — "file.txt:ads", "dir:a\file",
// a second colon after a drive spec — opens an NTFS alternate data stream.
func rejectAlternateDataStream(path string) error {
	first := strings.IndexByte(path, ':')
	switch {
	case first == -1:
		return nil
	case first == 1 && (len(path) == 2 || path[2] == '\\' || path[2] == '/'):
		if strings.IndexByte(path[2:], ':') != -1 {
			return fmt.Errorf("alternate data stream syntax not allowed for security")
		}
		return nil
	default:
		return fmt.Errorf("alternate data stream syntax not allowed for security")
	}
}

// rejectReservedDeviceName rejects a final path component whose base name is
// a DOS device name. The comparison ignores an extension ("CON.txt"), case,
// and trailing dots/spaces — Windows device resolution ignores all three, so
// the filter must too. Only the final component is checked: writing
// "dir\CON\file.txt" fails at MkdirAll with an OS error long before any
// device is opened, and a directory merely NAMED like a device is harmless.
func rejectReservedDeviceName(path string) error {
	base := path[strings.LastIndexAny(path, `\/`)+1:]
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	base = strings.TrimRight(base, " .")
	if base == "" {
		return nil
	}
	if windowsReservedDeviceNames[strings.ToUpper(base)] {
		return fmt.Errorf("DOS device name %q not allowed for security", base)
	}
	return nil
}
