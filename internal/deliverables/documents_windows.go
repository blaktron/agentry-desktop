//go:build windows

package deliverables

import (
	"golang.org/x/sys/windows"
)

// documentsDir asks Windows for FOLDERID_Documents, which follows a Documents
// folder the person has moved (to another drive, or into OneDrive), and
// falls back to the profile's Documents only when the known-folder call
// fails.
func documentsDir(goos string, getenv func(string) string, home func() (string, error)) (string, error) {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_Documents, windows.KF_FLAG_DEFAULT); err == nil && p != "" {
		return p, nil
	}
	return unixDocuments(goos, getenv, home)
}
