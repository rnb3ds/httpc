package main

import (
	"fmt"
	"log"
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

	opts := httpc.DefaultDownloadOptions("downloads/readme.md")
	opts.Overwrite = true

	result, err := client.DownloadFileWithOptions(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		opts,
	)
	if err != nil {
		log.Printf("   Error: %v\n", err)
		return
	}

	fmt.Printf("   ✓ Downloaded: %s\n", httpc.FormatBytes(result.BytesWritten))
	fmt.Printf("   ✓ Speed: %s\n", httpc.FormatSpeed(result.AverageSpeed))
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

	opts := httpc.DefaultDownloadOptions("downloads/test-file.txt")
	opts.Overwrite = true
	opts.ProgressInterval = 200 * time.Millisecond

	// Progress callback
	opts.ProgressCallback = func(downloaded, total int64, speed float64) {
		if total > 0 {
			percentage := float64(downloaded) / float64(total) * 100
			bar := createProgressBar(int(percentage), 40)
			fmt.Printf("\r   [%s] %.1f%% - %s - %s/s    ",
				bar,
				percentage,
				httpc.FormatBytes(downloaded),
				httpc.FormatSpeed(speed),
			)
		} else {
			fmt.Printf("\r   Downloaded: %s - %s/s    ",
				httpc.FormatBytes(downloaded),
				httpc.FormatSpeed(speed),
			)
		}
	}

	result, err := client.DownloadFileWithOptions(
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
	fmt.Printf("   ✓ Size: %s\n", httpc.FormatBytes(result.BytesWritten))
	fmt.Printf("   ✓ Average speed: %s\n", httpc.FormatSpeed(result.AverageSpeed))
}

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
