//go:build !windows

package sysproxy

// Enabled always returns false on non-Windows platforms.
func Enabled() bool { return false }
