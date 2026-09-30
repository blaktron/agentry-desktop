//go:build windows

package run

import "golang.org/x/sys/windows/registry"

// machineName reads the installation's MachineGuid.
func machineName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	id, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return ""
	}
	return id
}
