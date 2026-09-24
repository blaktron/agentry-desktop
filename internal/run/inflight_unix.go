//go:build unix

package run

import (
	"errors"
	"syscall"
)

// processExists: signal 0 checks for the process without signalling it.
// Only "no such process" is gone; a permission refusal is a process that
// exists under another user.
func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return !errors.Is(err, syscall.ESRCH)
}
