//go:build windows

package run

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// stillActive is STILL_ACTIVE, the exit code of a process still running.
const stillActive = 259

// processExists opens the process for a limited query. Only "no such
// process" (ERROR_INVALID_PARAMETER for an unknown pid) is gone; a refusal
// is a process that exists. An opened process that has exited (a handle
// someone still holds keeps its record) is gone too.
func processExists(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // a pid read back from our own record
	if err != nil {
		return !errors.Is(err, windows.ERROR_INVALID_PARAMETER)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return true
	}
	return code == stillActive
}

// ownedByMe: the system's temp directory is the user's own here (%TEMP%).
func ownedByMe(os.FileInfo) bool { return true }
