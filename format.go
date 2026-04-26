package httpc

import "fmt"

// FormatBytes formats a byte count as a human-readable string (e.g., "1.50 MB").
func FormatBytes(bytes int64) string {
	return formatUnit(float64(bytes), "B", "")
}

// FormatSpeed formats a byte-per-second rate as a human-readable string (e.g., "1.50 MB/s").
func FormatSpeed(bytesPerSecond float64) string {
	return formatUnit(bytesPerSecond, "B", "/s")
}

func formatUnit(value float64, baseUnit string, suffix string) string {
	const unit = 1024.0
	if value < unit {
		return fmt.Sprintf("%.0f %s%s", value, baseUnit, suffix)
	}

	units := [6]byte{'K', 'M', 'G', 'T', 'P', 'E'}
	div := unit
	exp := 0

	for n := value / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.2f %c%s%s", value/div, units[exp], baseUnit, suffix)
}
