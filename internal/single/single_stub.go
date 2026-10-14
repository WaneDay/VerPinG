//go:build !windows

package single

// Acquire always succeeds on non-Windows platforms.
func Acquire() (uintptr, bool) { return 0, true }

// Release is a no-op on non-Windows platforms.
func Release(h uintptr) {}
