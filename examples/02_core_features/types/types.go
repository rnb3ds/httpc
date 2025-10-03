package types

// APIResponse represents the response structure from echo.hoppscotch.io
// This type is shared across all examples in this directory
type APIResponse struct {
	Method  string            `json:"method"`
	Args    map[string]string `json:"args"`
	Data    string            `json:"data"`
	Headers map[string]string `json:"headers"`
	Path    string            `json:"path"`
}

// User represents a user data structure for JSON examples
type User struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

// Person represents a person structure for XML examples
type Person struct {
	Name string `xml:"name"`
	Age  int    `xml:"age"`
	City string `xml:"city"`
}
