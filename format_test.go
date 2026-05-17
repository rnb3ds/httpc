package httpc

import (
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero bytes", 0, "0 B"},
		{"bytes", 500, "500 B"},
		{"kilobytes", 1024, "1.00 KB"},
		{"kilobytes with fraction", 1536, "1.50 KB"},
		{"megabytes", 1048576, "1.00 MB"},
		{"gigabytes", 1073741824, "1.00 GB"},
		{"terabytes", 1099511627776, "1.00 TB"},
		{"petabytes", 1125899906842624, "1.00 PB"},
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
		{"bytes per second", 512, "512 B/s"},
		{"kilobytes per second", 1024, "1.00 KB/s"},
		{"megabytes per second", 1048576, "1.00 MB/s"},
		{"gigabytes per second", 1073741824, "1.00 GB/s"},
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

func TestFormatBytes_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"negative bytes", -1, "-1 B"},
		{"int64 max", 9223372036854775807, "8.00 EB"},
		{"exabytes", 1152921504606846976, "1.00 EB"},
		{"one byte", 1, "1 B"},
		{"just under KB", 1023, "1023 B"},
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

func TestFormatSpeed_BoundaryConditions(t *testing.T) {
	tests := []struct {
		name           string
		bytesPerSecond float64
		want           string
	}{
		{"negative speed", -1, "-1 B/s"},
		{"terabytes per second", 1099511627776, "1.00 TB/s"},
		{"very small fraction rounds to 0", 0.5, "0 B/s"},
		{"petabytes per second", 1125899906842624, "1.00 PB/s"},
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
