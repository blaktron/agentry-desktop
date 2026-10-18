// Package runfile reads a file of a run's output folder the way every
// reader of what an agent wrote must: by a "/"-separated path inside the
// folder, through no link at any part of it, and only a regular file.
package runfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Open opens the regular file rel of the run folder dir for reading, at most
// limit bytes long, and answers it with its size. Every part before the file
// must be a real directory and the file a regular one, a link anywhere
// refused, so a path that goes through a link the run left (outputs/docs, a
// link to ~/Documents) reaches nothing a listing would never show. The open
// goes through an os.Root on the folder as well, so a part swapped for a
// link after the checks still cannot lead out of it; the file is opened
// without following a link where the OS allows it and checked to be the one
// Lstat saw. The caller closes the file.
func Open(dir, rel string, limit int64) (*os.File, int64, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = root.Close() }()
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		fi, err := root.Lstat(filepath.FromSlash(strings.Join(parts[:i], "/")))
		if err != nil || !fi.IsDir() {
			return nil, 0, errNotReachable // a link, a junction or a file where a folder was
		}
	}
	name := filepath.FromSlash(rel)
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Size() > limit {
		return nil, 0, errNotReachable
	}
	f, err := root.OpenFile(name, os.O_RDONLY|noFollowOpenFlags, 0)
	if err != nil {
		return nil, 0, errNotReachable
	}
	if after, err := f.Stat(); err != nil || !os.SameFile(before, after) {
		_ = f.Close()
		return nil, 0, errNotReachable
	}
	return f, before.Size(), nil
}

var errNotReachable = errors.New("not a regular file reached through real folders")

// Read is the whole of such a file, at most limit bytes; ok is false when it
// cannot be read that way.
func Read(dir, rel string, limit int64) ([]byte, bool) {
	f, _, err := Open(dir, rel, limit)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, false
	}
	return b, true
}
