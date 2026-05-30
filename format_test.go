package httpc

import (
	"math"
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero bytes", 0, "0 B"},
		{"one byte", 1, "1 B"},
		{"bytes", 500, "500 B"},
		{"just under KB", 1023, "1023 B"},
		{"kilobytes", 1024, "1.00 KB"},
		{"kilobytes with fraction", 1536, "1.50 KB"},
		{"megabytes", 1048576, "1.00 MB"},
		{"gigabytes", 1073741824, "1.00 GB"},
		{"terabytes", 1099511627776, "1.00 TB"},
		{"petabytes", 1125899906842624, "1.00 PB"},
		{"exabytes", 1152921504606846976, "1.00 EB"},
		{"int64 max", math.MaxInt64, "8.00 EB"},
		{"int64 min", math.MinInt64, "-9223372036854775808 B"},
		{"negative bytes", -1, "-1 B"},
		{"1000 bytes under 1KB", 1000, "1000 B"},
		{"2048 bytes", 2048, "2.00 KB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("FormatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		name           string
		bytesPerSecond float64
		want           string
	}{
		{"zero", 0, "0 B/s"},
		{"very small fraction rounds to 0", 0.5, "0 B/s"},
		{"bytes per second", 512, "512 B/s"},
		{"kilobytes per second", 1024, "1.00 KB/s"},
		{"megabytes per second", 1048576, "1.00 MB/s"},
		{"gigabytes per second", 1073741824, "1.00 GB/s"},
		{"terabytes per second", 1099511627776, "1.00 TB/s"},
		{"petabytes per second", 1125899906842624, "1.00 PB/s"},
		{"negative speed", -1, "-1 B/s"},
		{"exabytes per second", float64(1152921504606846976), "1.00 EB/s"},
		{"NaN", math.NaN(), "NaN KB/s"},
		{"positive infinity", math.Inf(1), "+Inf EB/s"},
		{"negative infinity", math.Inf(-1), "-Inf B/s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSpeed(tt.bytesPerSecond)
			if got != tt.want {
				t.Errorf("FormatSpeed(%v) = %q, want %q", tt.bytesPerSecond, got, tt.want)
			}
		})
	}
}
