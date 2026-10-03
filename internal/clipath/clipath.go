// Package clipath puts the agentry CLI the Windows installer carries on the
// person's user Path, and takes it off again (agentry-desktop#580; plan
// agentry-notes/plans/cli-on-path.md §6, D3; D-060).
//
// The installer runs `agentry-desktop.exe --add-cli-to-path` after it has
// laid the files down, and the uninstaller `--remove-cli-from-path` before it
// removes them, as it already hands the settings and the Docker images to the
// app (internal/uninstall). The edit is the app's, not NSIS's, because an
// NSIS string holds 1,024 characters by default and a person's Path can be
// longer: a script that read it, appended and wrote it back would cut it
// short. Here the value is read and written whole, unexpanded, keeping its
// registry type, so an entry such as %USERPROFILE%\bin stays as it was.
//
// The entry is the bin directory beside the running executable. It is
// appended, never prepended: a CLI the person installed on purpose (the
// site's install.ps1 prepends its own directory) wins over the copy the app
// carries. On every other OS both flags do nothing and exit 0: the Linux
// packages install /usr/bin/agentry, and macOS offers the command from the
// app (agentry-desktop#581).
package clipath

import (
	"strings"
)

// The flags the installer and the uninstaller pass.
const (
	AddFlag    = "--add-cli-to-path"
	RemoveFlag = "--remove-cli-from-path"
)

// BinDir is the directory, beside the executable, the CLI is installed in.
const BinDir = "bin"

// same reports whether a Path entry names dir: case-insensitively, as
// Windows compares paths, ignoring a trailing backslash, and after expanding
// environment references through expand, so %LOCALAPPDATA%\…\bin matches
// the same directory spelled out.
func same(entry, dir string, expand func(string) string) bool {
	norm := func(s string) string {
		return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), `\/`))
	}
	e := norm(entry)
	if e == "" {
		return false
	}
	return e == norm(dir) || norm(expand(entry)) == norm(dir)
}

// WithEntry returns path with dir appended, and whether that changed it.
// Entries already there keep their spelling and their order; an empty
// path becomes dir alone; a path that already names dir is unchanged.
func WithEntry(path, dir string, expand func(string) string) (string, bool) {
	for _, e := range strings.Split(path, ";") {
		if same(e, dir, expand) {
			return path, false
		}
	}
	if strings.TrimSpace(path) == "" {
		return dir, true
	}
	return strings.TrimRight(path, ";") + ";" + dir, true
}

// WithoutEntry returns path with every entry naming dir removed, and whether
// that changed it. The other entries keep their spelling and their order, and
// empty entries the person had are kept, since only dir is the app's to
// remove.
func WithoutEntry(path, dir string, expand func(string) string) (string, bool) {
	parts := strings.Split(path, ";")
	kept := parts[:0:0]
	changed := false
	for _, e := range parts {
		if same(e, dir, expand) {
			changed = true
			continue
		}
		kept = append(kept, e)
	}
	if !changed {
		return path, false
	}
	return strings.Join(kept, ";"), true
}
