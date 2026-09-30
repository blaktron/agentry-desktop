//go:build darwin

package run

import "golang.org/x/sys/unix"

// machineName reads the hardware UUID (kern.uuid), which, unlike the host
// name there, does not follow the network the machine is on.
func machineName() string {
	id, err := unix.Sysctl("kern.uuid")
	if err != nil {
		return ""
	}
	return id
}
