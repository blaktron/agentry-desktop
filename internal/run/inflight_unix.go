//go:build unix

package run

import (
	"errors"
	"os"
	"syscall"
)

// processExists: signal 0 checks for the process without signalling it.
// Only "no such process" is gone; a permission refusal is a process that
// exists under another user.
func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return !errors.Is(err, syscall.ESRCH)
}

// ownedByMe says whether this process's user owns fi: the system's temp
// directory is shared between users here.
func ownedByMe(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}
