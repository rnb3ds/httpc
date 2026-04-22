//go:build !windows && !darwin && !linux

package proxy

import (
	"net/http"
	"net/url"
)

// detectPlatform handles proxy detection for unsupported platforms
func (d *Detector) detectPlatform() func(*http.Request) (*url.URL, error) {
	// For unsupported platforms, just use environment variables
	return d.detectFromEnvironment()
}
