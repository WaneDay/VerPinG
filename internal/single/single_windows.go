//go:build windows

package single

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const mutexName = `Local\VerPinGApp`

var (
	kernel32         = windows.NewLazySystemDLL("kernel32.dll")
	procCreateMutexW = kernel32.NewProc("CreateMutexW")
	procSetLastError = kernel32.NewProc("SetLastError")
	procGetLastError = kernel32.NewProc("GetLastError")
)

// Acquire creates the single-instance mutex.
// It returns the handle and true when this is the first instance; when another
// instance already owns the mutex it returns 0 and false.
func Acquire() (uintptr, bool) {
	name, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return 0, false
	}
	// Clear the thread's last error so a successful CreateMutexW (which leaves
	// it untouched) is not mistaken for ERROR_ALREADY_EXISTS.
	procSetLastError.Call(0)
	h, _, _ := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return 0, false
	}
	if e, _, _ := procGetLastError.Call(); e == uintptr(windows.ERROR_ALREADY_EXISTS) {
		_ = windows.CloseHandle(windows.Handle(h))
		return 0, false
	}
	return h, true
}

// Release closes the mutex handle.
func Release(h uintptr) {
	if h != 0 {
		_ = windows.CloseHandle(windows.Handle(h))
	}
}
