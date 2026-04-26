package engine

import (
	"testing"
)

func TestResponse_StatusCodes(t *testing.T) {
	testCases := []struct {
		code   int
		status string
	}{
		{200, "200 OK"},
		{201, "201 Created"},
		{400, "400 Bad Request"},
		{404, "404 Not Found"},
		{500, "500 Internal Server Error"},
	}

	for _, tc := range testCases {
		t.Run(tc.status, func(t *testing.T) {
			resp := &Response{}
			resp.SetStatusCode(tc.code)
			resp.SetStatus(tc.status)

			if resp.StatusCode() != tc.code {
				t.Errorf("Expected status code %d, got %d", tc.code, resp.StatusCode())
			}

			if resp.Status() != tc.status {
				t.Errorf("Expected status %s, got %s", tc.status, resp.Status())
			}
		})
	}
}
