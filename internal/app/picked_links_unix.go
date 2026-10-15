//go:build !windows

package app

import "path/filepath"

// resolveLinks follows every symbolic link in p to the path the system opens.
func resolveLinks(p string) (string, error) {
	return filepath.EvalSymlinks(p)
}
