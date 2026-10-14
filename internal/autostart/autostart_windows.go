//go:build windows

package autostart

import (
	"os"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey  = `Software\Microsoft\Windows\CurrentVersion\Run`
	runName = "VerPinG"
)

// IsEnabled reports whether the Run entry exists.
func IsEnabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(runName)
	return err == nil && strings.TrimSpace(v) != ""
}

// Enable writes the Run entry pointing at exePath.
func Enable(exePath string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(runName, `"`+exePath+`"`)
}

// Disable removes the Run entry (no error when absent).
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	if err := k.DeleteValue(runName); err != nil {
		return nil
	}
	return nil
}

// ExePath returns the current executable path ("" on failure).
func ExePath() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}
