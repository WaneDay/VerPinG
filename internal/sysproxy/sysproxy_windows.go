//go:build windows

package sysproxy

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const ieSettings = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`

// Enabled reports whether the Windows system proxy is enabled with a server set.
func Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, ieSettings, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	enable, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil || enable == 0 {
		return false
	}
	server, _, err := k.GetStringValue("ProxyServer")
	if err != nil {
		return false
	}
	return strings.TrimSpace(server) != ""
}
