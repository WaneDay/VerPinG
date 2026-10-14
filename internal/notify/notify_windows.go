//go:build windows

package notify

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Popup shows a native Windows message box.
func Popup(title, text string) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	msgbox := user32.NewProc("MessageBoxW")
	uTitle, _ := windows.UTF16PtrFromString(title)
	uText, _ := windows.UTF16PtrFromString(text)
	// MB_OK | MB_ICONWARNING | MB_TOPMOST
	_, _, _ = msgbox.Call(0,
		uintptr(unsafe.Pointer(uText)),
		uintptr(unsafe.Pointer(uTitle)),
		uintptr(0x00000000|0x00000030|0x00040000))
}
