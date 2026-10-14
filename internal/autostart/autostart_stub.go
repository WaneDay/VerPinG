//go:build !windows

package autostart

// IsEnabled always returns false on non-Windows platforms.
func IsEnabled() bool { return false }

// Enable is a no-op on non-Windows platforms.
func Enable(exePath string) error { return nil }

// Disable is a no-op on non-Windows platforms.
func Disable() error { return nil }

// ExePath returns an empty string on non-Windows platforms.
func ExePath() string { return "" }
