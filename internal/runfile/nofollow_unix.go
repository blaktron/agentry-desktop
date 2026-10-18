//go:build !windows

package runfile

import "syscall"

// noFollowOpenFlags open a run's file without following a link, and without
// waiting on a FIFO.
const noFollowOpenFlags = syscall.O_NOFOLLOW | syscall.O_NONBLOCK
