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
		if runtime.GOOS == "windows" {
			p = strings.ToLower(p)
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
// 3. System path protection
// 4. Path traversal detection
// 5. Symlink attack prevention
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
	if runtime.GOOS == "windows" {
		pathNorm = strings.ToLower(pathNorm)
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
