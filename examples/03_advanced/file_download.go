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
	fmt.Println("=== File Download Examples ===\n ")

	// Example 1: Simple file download
	demonstrateSimpleDownload()

	// Example 2: Download with progress tracking
	demonstrateDownloadWithProgress()

	// Example 3: Download large file with custom options
	demonstrateLargeFileDownload()

	// Example 4: Resume interrupted download
	demonstrateResumeDownload()

	// Example 5: Save response to file (alternative method)
	demonstrateSaveResponseToFile()

	// Example 6: Download with authentication
	demonstrateAuthenticatedDownload()

	fmt.Println("\n=== All Download Examples Completed ===")
}

// demonstrateSimpleDownload shows basic file download
func demonstrateSimpleDownload() {
	fmt.Println("--- Example 1: Simple File Download ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create downloads directory if it doesn't exist
	if err := os.MkdirAll("downloads", 0755); err != nil {
		log.Printf("Error creating downloads directory: %v\n", err)
		return
	}

	// Download a small file using the simple DownloadFile method
	result, err := client.DownloadFile(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		"downloads/golang-readme.md",
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("✓ Downloaded: %s\n", result.FilePath)
	fmt.Printf("  Size: %s\n", formatBytes(result.BytesWritten))
	fmt.Printf("  Duration: %v\n", result.Duration)
	fmt.Printf("  Speed: %s/s\n\n", formatBytes(int64(result.AverageSpeed)))
}

// demonstrateDownloadWithProgress shows download with progress tracking
func demonstrateDownloadWithProgress() {
	fmt.Println("--- Example 2: Download with Progress Tracking ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create download options with progress callback
	opts := &httpc.DownloadOptions{
		FilePath:  "downloads/sample-file.bin",
		Overwrite: true,
		ProgressCallback: func(downloaded, total int64, speed float64) {
			if total > 0 {
				percentage := float64(downloaded) / float64(total) * 100
				fmt.Printf("\r  Progress: %.1f%% (%s / %s) - Speed: %s",
					percentage,
					formatBytes(downloaded),
					formatBytes(total),
					formatBytes(int64(speed))+"/s",
				)
			} else {
				fmt.Printf("\r  Downloaded: %s - Speed: %s",
					formatBytes(downloaded),
					formatBytes(int64(speed))+"/s",
				)
			}
		},
	}

	// Download a file with progress tracking
	// Using a reliable test file from GitHub
	result, err := client.DownloadWithOptions(
		"https://raw.githubusercontent.com/golang/go/master/LICENSE",
		opts,
		httpc.WithTimeout(60*time.Second),
	)
	if err != nil {
		log.Printf("\nError: %v\n", err)
		return
	}

	fmt.Printf("\n✓ Download completed: %s\n", result.FilePath)
	fmt.Printf("  Total size: %s\n", formatBytes(result.BytesWritten))
	fmt.Printf("  Average speed: %s/s\n\n", formatBytes(int64(result.AverageSpeed)))
}

// demonstrateLargeFileDownload shows downloading large files with custom settings
func demonstrateLargeFileDownload() {
	fmt.Println("--- Example 3: Large File Download ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Configure for large file download
	opts := &httpc.DownloadOptions{
		FilePath:  "downloads/large-file.bin",
		Overwrite: true,
		ProgressCallback: func(downloaded, total int64, speed float64) {
			if total > 0 {
				percentage := float64(downloaded) / float64(total) * 100
				fmt.Printf("\r  Downloading: %.1f%% - %s/s",
					percentage,
					formatBytes(int64(speed)),
				)
			}
		},
	}

	// Download with longer timeout for large files
	// Using a larger file from GitHub
	result, err := client.DownloadWithOptions(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		opts,
		httpc.WithTimeout(5*time.Minute),
		httpc.WithMaxRetries(3),
	)
	if err != nil {
		log.Printf("\nError: %v\n", err)
		return
	}

	fmt.Printf("\n✓ Large file downloaded successfully\n")
	fmt.Printf("  File: %s\n", result.FilePath)
	fmt.Printf("  Size: %s\n", formatBytes(result.BytesWritten))
	fmt.Printf("  Time: %v\n", result.Duration)
	fmt.Printf("  Average speed: %s/s\n\n", formatBytes(int64(result.AverageSpeed)))
}

// demonstrateResumeDownload shows resuming interrupted downloads
func demonstrateResumeDownload() {
	fmt.Println("--- Example 4: Resume Interrupted Download ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	filePath := "downloads/resume-test.bin"

	// First, simulate a partial download by downloading only part of the file
	fmt.Println("  Simulating interrupted download...")
	opts1 := &httpc.DownloadOptions{
		FilePath:  filePath,
		Overwrite: true,
	}

	// Download with a short timeout to simulate interruption
	// Using a file from GitHub
	_, err = client.DownloadWithOptions(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		opts1,
		httpc.WithTimeout(1*time.Second), // Very short timeout to interrupt
	)
	// We expect this to fail/timeout
	if err != nil {
		fmt.Printf("  Download interrupted (as expected): %v\n", err)
	}

	// Check if partial file exists
	if fileInfo, err := os.Stat(filePath); err == nil {
		fmt.Printf("  Partial file size: %s\n", formatBytes(fileInfo.Size()))

		// Now resume the download
		fmt.Println("  Resuming download...")
		opts2 := &httpc.DownloadOptions{
			FilePath:       filePath,
			ResumeDownload: true,
			ProgressCallback: func(downloaded, total int64, speed float64) {
				if total > 0 {
					percentage := float64(downloaded) / float64(total) * 100
					fmt.Printf("\r  Progress: %.1f%% - %s/s",
						percentage,
						formatBytes(int64(speed)),
					)
				}
			},
		}

		result, err := client.DownloadWithOptions(
			"https://raw.githubusercontent.com/golang/go/master/README.md",
			opts2,
			httpc.WithTimeout(5*time.Minute),
		)
		if err != nil {
			log.Printf("\nError resuming: %v\n", err)
			return
		}

		if result.StatusCode == 416 {
			fmt.Printf("\n✓ File already complete (no resume needed)\n")
		} else if result.Resumed {
			fmt.Printf("\n✓ Download resumed successfully\n")
		} else {
			fmt.Printf("\n✓ Download completed (server doesn't support resume)\n")
		}
		fmt.Printf("  Final size: %s\n\n", formatBytes(result.BytesWritten))
	} else {
		fmt.Println("  Note: Resume example skipped (partial file not created)\n ")
	}
}

// demonstrateSaveResponseToFile shows alternative method using Response.SaveToFile
func demonstrateSaveResponseToFile() {
	fmt.Println("--- Example 5: Save Response to File ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Make a regular GET request
	resp, err := client.Get("https://raw.githubusercontent.com/golang/go/master/LICENSE")
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	// Save the response body to a file
	filePath := "downloads/golang-license.txt"
	if err := resp.SaveToFile(filePath); err != nil {
		log.Printf("Error saving file: %v\n", err)
		return
	}

	fmt.Printf("✓ Response saved to: %s\n", filePath)
	fmt.Printf("  Size: %s\n\n", formatBytes(int64(len(resp.RawBody))))
}

// demonstrateAuthenticatedDownload shows downloading files with authentication
func demonstrateAuthenticatedDownload() {
	fmt.Println("--- Example 6: Authenticated Download ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Download with authentication headers
	opts := &httpc.DownloadOptions{
		FilePath:  "downloads/authenticated-file.txt",
		Overwrite: true,
	}

	result, err := client.DownloadWithOptions(
		"https://httpbin.org/get",
		opts,
		httpc.WithBearerToken("your-api-token-here"),
		httpc.WithHeader("X-Custom-Header", "custom-value"),
		httpc.WithTimeout(30*time.Second),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("✓ Authenticated download completed\n")
	fmt.Printf("  File: %s\n", result.FilePath)
	fmt.Printf("  Size: %s\n\n", formatBytes(result.BytesWritten))
}

// Helper function to format bytes in human-readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Helper function to clean up downloaded files (optional)
func cleanupDownloads() {
	downloadsDir := "downloads"
	if err := os.RemoveAll(downloadsDir); err != nil {
		log.Printf("Warning: Failed to cleanup downloads directory: %v\n", err)
	}
}

// init creates the downloads directory
func init() {
	downloadsDir := "downloads"
	if err := os.MkdirAll(downloadsDir, 0755); err != nil {
		log.Printf("Warning: Failed to create downloads directory: %v\n", err)
	}
}
