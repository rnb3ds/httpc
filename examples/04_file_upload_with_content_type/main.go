package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/httpc"
)

func main() {
	fmt.Println("=== File Upload with ContentType Examples ===")

	// Example 1: Single file with ContentType
	exampleSingleFileWithContentType()

	// Example 2: Multiple files with different ContentTypes
	exampleMultipleFilesWithContentTypes()

	// Example 3: Mixed content types (PDF + Images)
	exampleMixedContentTypes()
}

func exampleSingleFileWithContentType() {
	fmt.Println("--- Example 1: Single File with ContentType ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Simulate PDF content
	pdfContent := []byte("%PDF-1.4 sample content")

	formData := &httpc.FormData{
		Fields: map[string]string{
			"title":       "My Document",
			"description": "Important PDF file",
		},
		Files: map[string]*httpc.FileData{
			"document": {
				Filename:    "report.pdf",
				Content:     pdfContent,
				ContentType: "application/pdf", // Explicitly set ContentType
			},
		},
	}

	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithFormData(formData),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Success: %v\n\n", resp.IsSuccess())
}

func exampleMultipleFilesWithContentTypes() {
	fmt.Println("--- Example 2: Multiple Files with Different ContentTypes ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Simulate different file types
	pdfContent := []byte("%PDF-1.4 document content")
	txtContent := []byte("Plain text content")
	jsonContent := []byte(`{"key": "value"}`)

	formData := &httpc.FormData{
		Fields: map[string]string{
			"category": "mixed-documents",
		},
		Files: map[string]*httpc.FileData{
			"pdf_file": {
				Filename:    "document.pdf",
				Content:     pdfContent,
				ContentType: "application/pdf",
			},
			"text_file": {
				Filename:    "readme.txt",
				Content:     txtContent,
				ContentType: "text/plain",
			},
			"json_file": {
				Filename:    "data.json",
				Content:     jsonContent,
				ContentType: "application/json",
			},
		},
	}

	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithFormData(formData),
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Success: %v\n\n", resp.IsSuccess())
}

func exampleMixedContentTypes() {
	fmt.Println("--- Example 3: Mixed Content Types (PDF + Images) ---")

	client, err := httpc.New()
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Simulate PDF and image content
	pdfContent := []byte("%PDF-1.4 report content")
	jpegContent := []byte("\xFF\xD8\xFF\xE0 JPEG data")
	pngContent := []byte("\x89PNG\r\n\x1a\n PNG data")

	formData := &httpc.FormData{
		Fields: map[string]string{
			"title":       "Project Report",
			"description": "Report with images",
			"public":      "false",
		},
		Files: map[string]*httpc.FileData{
			"report": {
				Filename:    "project-report.pdf",
				Content:     pdfContent,
				ContentType: "application/pdf",
			},
			"screenshot1": {
				Filename:    "screenshot1.jpg",
				Content:     jpegContent,
				ContentType: "image/jpeg",
			},
			"screenshot2": {
				Filename:    "screenshot2.png",
				Content:     pngContent,
				ContentType: "image/png",
			},
		},
	}

	resp, err := client.Post("https://echo.hoppscotch.io",
		httpc.WithFormData(formData),
		httpc.WithBearerToken("your-token-here"), // Can combine with other options
	)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Status: %d\n", resp.StatusCode)
	fmt.Printf("Success: %v\n", resp.IsSuccess())

	// Note: When ContentType is not specified, the server will use default
	// (usually application/octet-stream) or try to detect it from the file extension
	fmt.Println("\nNote: Always specify ContentType for better compatibility")
	fmt.Println("Common MIME types:")
	fmt.Println("  - application/pdf")
	fmt.Println("  - image/jpeg, image/png, image/gif")
	fmt.Println("  - text/plain, text/html, text/csv")
	fmt.Println("  - application/json, application/xml")
	fmt.Println("  - application/zip, application/octet-stream")
}

