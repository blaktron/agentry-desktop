//go:build !windows

package app

import (
	"os"
	"syscall"
)

// openNoFollow opens a run's output file for its preview: never through a
// link (O_NOFOLLOW), and never waiting on a FIFO swapped in after the walk
// looked (O_NONBLOCK). readOutputFile checks it is the file the walk saw.
func openNoFollow(p string) (*os.File, error) {
	return os.OpenFile(p, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0) //nolint:gosec // under the run's own folder
}
