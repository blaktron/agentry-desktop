//go:build unix

package run

import "syscall"

// terminateProcess sends the gentle signal; false when there is no such
// process to wait for.
func terminateProcess(pid int) bool {
	return syscall.Kill(pid, syscall.SIGTERM) == nil
}
