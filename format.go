package httpc

import "strconv"

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
		var buf [32]byte
		b := strconv.AppendFloat(buf[:0], value, 'f', 0, 64)
		b = append(b, ' ')
		b = append(b, baseUnit...)
		b = append(b, suffix...)
		return string(b)
	}

	units := [6]byte{'K', 'M', 'G', 'T', 'P', 'E'}
	div := unit
	exp := 0

	for n := value / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}

	var buf [32]byte
	b := strconv.AppendFloat(buf[:0], value/div, 'f', 2, 64)
	b = append(b, ' ')
	b = append(b, units[exp])
	b = append(b, baseUnit...)
	b = append(b, suffix...)
	return string(b)
}
