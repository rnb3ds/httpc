package httpc

import "fmt"

// FormatBytes formats a byte count as a human-readable string (e.g., "1.50 MB").
func FormatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	units := [6]byte{'K', 'M', 'G', 'T', 'P', 'E'}
	div := int64(unit)
	exp := 0

	for n := bytes / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), units[exp])
}

// FormatSpeed formats a byte-per-second rate as a human-readable string (e.g., "1.50 MB/s").
func FormatSpeed(bytesPerSecond float64) string {
	const unit = 1024.0
	if bytesPerSecond < unit {
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	}

	units := [6]string{"KB/s", "MB/s", "GB/s", "TB/s", "PB/s", "EB/s"}
	div := unit
	exp := 0

	for n := bytesPerSecond / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %s", bytesPerSecond/div, units[exp])
}
