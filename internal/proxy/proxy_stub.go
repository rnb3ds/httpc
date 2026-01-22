//go:build !windows && !darwin && !linux

package proxy

// detectOther handles proxy detection for unsupported platforms
func (d *Detector) detectOther() func(*http.Request) (*url.URL, error) {
	// For unsupported platforms, just use environment variables
	return d.detectFromEnvironment()
}
