//go:build examples

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cybergodev/httpc"
)

// This example demonstrates file upload and download operations
// Consolidates: file_upload.go, file_download.go

func main() {
	fmt.Println("=== File Operations Examples ===\n ")

	// Create downloads directory
	if err := os.MkdirAll("downloads", 0755); err != nil {
		log.Printf("Warning: Failed to create downloads directory: %v\n", err)
	}

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// File Upload Examples
	demonstrateFileUpload(client)

	// File Download Examples
	demonstrateFileDownload(client)

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateFileUpload shows various file upload patterns
func demonstrateFileUpload(client httpc.Client) {
	fmt.Println("--- File Upload ---")

	// 1. Single file upload
	fileContent := []byte("This is the document content.\nMultiple lines here.")
	resp, err := client.Post("https://echo.hoppscotch.io/upload",
		httpc.WithFile("file", "document.txt", fileContent),
	)
	if err != nil {
		log.Printf("Single file error: %v\n", err)
	} else {
		fmt.Printf("✓ Single file: Status %d (%d bytes)\n", resp.StatusCode(), len(fileContent))
	}

	// 2. Multiple files upload
	formData := &httpc.FormData{
		Fields: map[string]string{},
		Files: map[string]*httpc.FileData{
			"document": {
				Filename: "report.pdf",
				Content:  []byte{0x25, 0x50, 0x44, 0x46}, // PDF header
			},
			"image": {
				Filename: "photo.jpg",
				Content:  []byte{0xFF, 0xD8, 0xFF, 0xE0}, // JPEG header
			},
		},
	}
	resp, err = client.Post("https://echo.hoppscotch.io/upload",
		httpc.WithFormData(formData),
	)
	if err != nil {
		log.Printf("Multiple files error: %v\n", err)
	} else {
		fmt.Printf("✓ Multiple files: Status %d (%d files)\n", resp.StatusCode(), len(formData.Files))
	}

	// 3. File with form fields (metadata)
	formDataWithFields := &httpc.FormData{
		Fields: map[string]string{
			"title":       "My Document",
			"description": "Important document",
			"category":    "reports",
		},
		Files: map[string]*httpc.FileData{
			"file": {
				Filename:    "document.pdf",
				Content:     []byte("Document content"),
				ContentType: "application/pdf",
			},
		},
	}
	resp, err = client.Post("https://echo.hoppscotch.io/upload",
		httpc.WithFormData(formDataWithFields),
		httpc.WithBearerToken("your-token"),
	)
	if err != nil {
		log.Printf("File with fields error: %v\n", err)
	} else {
		fmt.Printf("✓ File with metadata: Status %d\n", resp.StatusCode())
	}

	// 4. Large file with timeout
	largeFile := make([]byte, 1024*1024) // 1MB
	for i := range largeFile {
		largeFile[i] = byte(i % 256)
	}
	resp, err = client.Post("https://echo.hoppscotch.io/upload",
		httpc.WithFile("file", "large.bin", largeFile),
		httpc.WithTimeout(60*time.Second),
		httpc.WithMaxRetries(2),
	)
	if err != nil {
		log.Printf("Large file error: %v\n", err)
	} else {
		fmt.Printf("✓ Large file: Status %d (%d bytes, took %v)\n\n",
			resp.StatusCode(), len(largeFile), resp.Meta.Duration)
	}
}

// demonstrateFileDownload shows various file download patterns
func demonstrateFileDownload(client httpc.Client) {
	fmt.Println("--- File Download ---")

	// 1. Simple download
	result, err := client.DownloadFile(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		"downloads/golang-readme.md",
	)
	if err != nil {
		log.Printf("Simple download error: %v\n", err)
	} else {
		fmt.Printf("✓ Simple download: %s (%s, %v)\n",
			result.FilePath,
			httpc.FormatBytes(result.BytesWritten),
			result.Duration)
	}

	// 2. Download with progress tracking
	opts := &httpc.DownloadOptions{
		FilePath:  "downloads/sample-file.bin",
		Overwrite: true,
		ProgressCallback: func(downloaded, total int64, speed float64) {
			if total > 0 {
				percentage := float64(downloaded) / float64(total) * 100
				fmt.Printf("\r  Progress: %.1f%% (%s / %s) - %s",
					percentage,
					httpc.FormatBytes(downloaded),
					httpc.FormatBytes(total),
					httpc.FormatSpeed(speed))
			}
		},
	}

	result, err = client.DownloadWithOptions(
		"https://raw.githubusercontent.com/golang/go/master/LICENSE",
		opts,
		httpc.WithTimeout(60*time.Second),
	)
	if err != nil {
		log.Printf("\nProgress download error: %v\n", err)
	} else {
		fmt.Printf("\n✓ Progress download: %s (%s, avg %s)\n",
			result.FilePath,
			httpc.FormatBytes(result.BytesWritten),
			httpc.FormatSpeed(result.AverageSpeed))
	}

	// 3. Download with authentication
	authOpts := &httpc.DownloadOptions{
		FilePath:  "downloads/authenticated-file.txt",
		Overwrite: true,
	}
	result, err = client.DownloadWithOptions(
		"https://httpbin.org/get",
		authOpts,
		httpc.WithBearerToken("your-api-token"),
		httpc.WithHeader("X-Custom", "value"),
	)
	if err != nil {
		log.Printf("Auth download error: %v\n", err)
	} else {
		fmt.Printf("✓ Authenticated download: %s (%s)\n",
			result.FilePath,
			httpc.FormatBytes(result.BytesWritten))
	}

	// 4. Save response to file (alternative method)
	resp, err := client.Get("https://raw.githubusercontent.com/golang/go/master/LICENSE")
	if err != nil {
		log.Printf("Response fetch error: %v\n", err)
	} else {
		filePath := "downloads/license.txt"
		if err := resp.SaveToFile(filePath); err != nil {
			log.Printf("Save error: %v\n", err)
		} else {
			fmt.Printf("✓ SaveToFile: %s (%s)\n",
				filePath,
				httpc.FormatBytes(int64(len(resp.RawBody()))))
		}
	}

	// 5. Resume interrupted download (demonstration)
	resumeFilePath := "downloads/resume-test.bin"
	resumeOpts := &httpc.DownloadOptions{
		FilePath:       resumeFilePath,
		ResumeDownload: true,
		Overwrite:      false,
	}
	result, err = client.DownloadWithOptions(
		"https://raw.githubusercontent.com/golang/go/master/README.md",
		resumeOpts,
		httpc.WithTimeout(5*time.Minute),
	)
	if err != nil {
		log.Printf("Resume download error: %v\n", err)
	} else {
		if result.Resumed {
			fmt.Printf("✓ Resumed download: %s (resumed from partial)\n", result.FilePath)
		} else {
			fmt.Printf("✓ Complete download: %s (no resume needed)\n", result.FilePath)
		}
	}
}
