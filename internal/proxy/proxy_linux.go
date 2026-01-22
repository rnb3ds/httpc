//go:build linux

package proxy

// detectPlatform reads proxy settings from Linux system settings
func (d *Detector) detectPlatform() func(*http.Request) (*url.URL, error) {
	// Linux typically uses environment variables for proxy settings
	// which are already handled by detectFromEnvironment

	// Some desktop environments (GNOME, KDE) store settings in:
	// - gsettings/dconf (GNOME)
	// - ~/.config/kioslaverc (KDE)
	// But these are typically exported to environment variables by the session

	return nil
}
