//go:build windows

package app

import (
	"fmt"
	"io/fs"
	"strings"

	"golang.org/x/sys/windows"
)

// volumeNameDOS asks GetFinalPathNameByHandle for the drive-letter form of a
// normalized name (VOLUME_NAME_DOS | FILE_NAME_NORMALIZED, both zero), which
// golang.org/x/sys/windows does not name.
const volumeNameDOS = 0

// resolveLinks follows every link in p, directory junctions included, to the
// path the system opens. filepath.EvalSymlinks does not follow a junction
// (a mount point), so a junction inside a picked folder could lead out of it
// unseen; instead the path is opened, with no access to its contents, and the
// handle is asked for its final name.
func resolveLinks(p string) (string, error) {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(name, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		// A *fs.PathError, as filepath.EvalSymlinks returns, so an absent
		// name still reads as fs.ErrNotExist to resolveExisting.
		return "", &fs.PathError{Op: "open", Path: p, Err: err}
	}
	defer func() { _ = windows.CloseHandle(h) }()
	buf := make([]uint16, windows.MAX_PATH)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), volumeNameDOS)
		if err != nil {
			// Not wrapped: the file exists, so this must not read as an
			// absent name that its parent could stand in for.
			return "", fmt.Errorf("could not tell which file %s is: %v", p, err)
		}
		if n < uint32(len(buf)) {
			return trimFinalPathPrefix(windows.UTF16ToString(buf[:n])), nil
		}
		buf = make([]uint16, n)
	}
}

// trimFinalPathPrefix turns the \\?\ form GetFinalPathNameByHandle returns
// into the ordinary form a picker returns, so the two compare.
func trimFinalPathPrefix(p string) string {
	switch {
	case strings.HasPrefix(p, `\\?\UNC\`):
		return `\\` + p[len(`\\?\UNC\`):]
	case strings.HasPrefix(p, `\\?\`):
		return p[len(`\\?\`):]
	}
	return p
}
