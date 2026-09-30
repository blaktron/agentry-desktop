//go:build linux

package run

import (
	"os"
	"strings"
)

// machineName reads systemd's machine id, which is made once at install and
// stays through renames and reboots. A container or a system without one
// has none, and "" is returned.
func machineName() string {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if b, err := os.ReadFile(p); err == nil {
			if id := strings.TrimSpace(string(b)); id != "" {
				return id
			}
		}
	}
	return ""
}
