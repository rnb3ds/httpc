//go:build examples

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== HTTPC File Download Demo ===\n ")

	// Example 1: Simple download
	simpleDownload()

	// Example 2: Download with progress bar
	downloadWithProgress()

	fmt.Println("\n=== Demo Completed ===")
}

func simpleDownload() {
	fmt.Println("1. Simple File Download")
	fmt.Println("   Downloading Go README.md...")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create downloads directory if it doesn't exist
	if err := os.MkdirAll("downloads", 0755); err != nil {
		log.Printf("   Error creating downloads directory: %v\n", err)
		return
	}

	// Use the simple DownloadFile method for basic downloads
	result, err := client.DownloadFile(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		"downloads/readme.md",
	)
	if err != nil {
		log.Printf("   Error: %v\n", err)
		return
	}

	fmt.Printf("   ✓ Downloaded: %s\n", formatBytes(result.BytesWritten))
	fmt.Printf("   ✓ Speed: %s/s\n", formatBytes(int64(result.AverageSpeed)))
	fmt.Printf("   ✓ Duration: %v\n\n", result.Duration)
}

func downloadWithProgress() {
	fmt.Println("2. Download with Progress Tracking")
	fmt.Println("   Downloading test file...")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create download options with progress callback
	opts := &httpc.DownloadOptions{
		FilePath:  "downloads/test-file.txt",
		Overwrite: true,
		ProgressCallback: func(downloaded, total int64, speed float64) {
			if total > 0 {
				percentage := float64(downloaded) / float64(total) * 100
				bar := createProgressBar(int(percentage), 40)
				fmt.Printf("\r   [%s] %.1f%% - %s - %s/s    ",
					bar,
					percentage,
					formatBytes(downloaded),
					formatBytes(int64(speed)),
				)
			} else {
				fmt.Printf("\r   Downloaded: %s - %s/s    ",
					formatBytes(downloaded),
					formatBytes(int64(speed)),
				)
			}
		},
	}

	result, err := client.DownloadWithOptions(
		"https://raw.githubusercontent.com/golang/go/master/LICENSE",
		opts,
		httpc.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Printf("\n   Error: %v\n", err)
		return
	}

	fmt.Printf("\n   ✓ Download completed!\n")
	fmt.Printf("   ✓ File: %s\n", result.FilePath)
	fmt.Printf("   ✓ Size: %s\n", formatBytes(result.BytesWritten))
	fmt.Printf("   ✓ Average speed: %s/s\n", formatBytes(int64(result.AverageSpeed)))
}

// formatBytes formats bytes in human readable format
// func formatBytes(bytes int64) string {
// 	const unit = 1024
// 	if bytes < unit {
// 		return fmt.Sprintf("%d B", bytes)
// 	}
// 	div, exp := int64(unit), 0
// 	for n := bytes / unit; n >= unit; n /= unit {
// 		div *= unit
// 		exp++
// 	}
// 	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
// }

// createProgressBar creates a simple text progress bar
func createProgressBar(percentage, width int) string {
	if percentage > 100 {
		percentage = 100
	}
	if percentage < 0 {
		percentage = 0
	}

	filled := (percentage * width) / 100
	empty := width - filled

	bar := ""
	for i := 0; i < filled; i++ {
		bar += "="
	}
	if filled < width {
		bar += ">"
	}
	for i := 0; i < empty-1; i++ {
		bar += " "
	}

	return bar
}
