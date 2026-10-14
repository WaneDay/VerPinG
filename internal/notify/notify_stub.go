//go:build !windows

package notify

// Popup is a no-op on non-Windows platforms.
func Popup(title, text string) {}
