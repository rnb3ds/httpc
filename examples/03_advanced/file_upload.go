package main

import (
	"fmt"
	"log"
	"time"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== File Upload Examples ===\n ")

	// Example 1: Single file upload
	demonstrateSingleFile()

	// Example 2: Multiple files upload
	demonstrateMultipleFiles()

	// Example 3: File upload with form fields
	demonstrateFileWithFields()

	// Example 4: Large file upload with timeout
	demonstrateLargeFile()

	fmt.Println("\n=== All Examples Completed ===")
}

// demonstrateSingleFile shows single file upload
func demonstrateSingleFile() {
	fmt.Println("--- Example 1: Single File Upload ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Simulate file content
	fileContent := []byte("This is the content of my document.\nIt has multiple lines.")

	resp, err := client.Post("https://echo.hoppscotch.io/upload",
		httpc.WithFile("file", "document.txt", fileContent),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("File uploaded: document.txt (%d bytes)\n\n", len(fileContent))
}

// demonstrateMultipleFiles shows multiple file upload
func demonstrateMultipleFiles() {
	fmt.Println("--- Example 2: Multiple Files Upload ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Simulate multiple files
	pdfContent := []byte{0x25, 0x50, 0x44, 0x46}  // PDF header
	jpegContent := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG header

	formData := &httpc.FormData{
		Fields: map[string]string{},
		Files: map[string]*httpc.FileData{
			"document": {
				Filename:    "report.pdf",
				Content:     pdfContent,
				ContentType: "application/pdf",
			},
			"thumbnail": {
				Filename:    "preview.jpg",
				Content:     jpegContent,
				ContentType: "image/jpeg",
			},
		},
	}

	resp, err := client.Post("https://echo.hoppscotch.io/upload",
		httpc.WithFormData(formData),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Files uploaded:\n")
	fmt.Printf("  - report.pdf (%d bytes)\n", len(pdfContent))
	fmt.Printf("  - preview.jpg (%d bytes)\n\n", len(jpegContent))
}

// demonstrateFileWithFields shows file upload with form fields
func demonstrateFileWithFields() {
	fmt.Println("--- Example 3: File Upload with Form Fields ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// File content
	fileContent := []byte("Important document content")

	// Create form data with both files and fields
	formData := &httpc.FormData{
		Fields: map[string]string{
			"title":       "My Document",
			"description": "This is an important document",
			"category":    "reports",
			"public":      "false",
		},
		Files: map[string]*httpc.FileData{
			"file": {
				Filename:    "document.pdf",
				Content:     fileContent,
				ContentType: "application/pdf",
			},
		},
	}

	resp, err := client.Post("https://echo.hoppscotch.io/upload",
		httpc.WithFormData(formData),
		httpc.WithBearerToken("your-auth-token"),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Uploaded with metadata:\n")
	fmt.Printf("  Title: %s\n", formData.Fields["title"])
	fmt.Printf("  Description: %s\n", formData.Fields["description"])
	fmt.Printf("  File: document.pdf (%d bytes)\n\n", len(fileContent))
}

// demonstrateLargeFile shows large file upload with proper timeout
func demonstrateLargeFile() {
	fmt.Println("--- Example 4: Large File Upload ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Simulate a large file (1MB)
	largeFileContent := make([]byte, 1024*1024)
	for i := range largeFileContent {
		largeFileContent[i] = byte(i % 256)
	}

	resp, err := client.Post("https://echo.hoppscotch.io/upload",
		httpc.WithFile("file", "large-file.bin", largeFileContent),
		httpc.WithTimeout(60*time.Second), // Longer timeout for large files
		httpc.WithMaxRetries(2),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Large file uploaded: %d bytes\n", len(largeFileContent))
	fmt.Printf("Upload duration: %v\n\n", resp.Duration)
}
