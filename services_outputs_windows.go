//go:build windows

package main

import "os"

// openNoFollow opens a run's output file for its preview. Windows has no
// O_NOFOLLOW; readOutputFile's same-file check after the open is what
// refuses a file swapped for a link or a junction after the walk looked.
func openNoFollow(p string) (*os.File, error) {
	return os.Open(p) //nolint:gosec // under the run's own folder
}
