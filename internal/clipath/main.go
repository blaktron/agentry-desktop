package clipath

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Main handles AddFlag and RemoveFlag for the installer: it edits the user
// Path (on Windows; elsewhere it does nothing) and reports what it did to w.
// Exit 0 on success or when there was nothing to do, 1 when the edit failed;
// the installer reports a failure and goes on, since the CLI is still in
// place and only the Path is missing.
func Main(w io.Writer, flag string) int {
	exe, err := os.Executable()
	if err != nil {
		_, _ = fmt.Fprintf(w, "locate the application: %v\n", err)
		return 1
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Join(filepath.Dir(exe), BinDir)
	var changed bool
	switch flag {
	case AddFlag:
		// Only a directory that holds the CLI goes on the Path: a build
		// without it (a PR's) adds nothing.
		if _, err := os.Stat(filepath.Join(dir, cliName)); err != nil {
			_, _ = fmt.Fprintf(w, "no agentry CLI in %s; the Path is unchanged\n", dir)
			return 0
		}
		changed, err = editUserPath(func(p string) (string, bool) { return WithEntry(p, dir, expandEnv) })
	case RemoveFlag:
		changed, err = editUserPath(func(p string) (string, bool) { return WithoutEntry(p, dir, expandEnv) })
	default:
		_, _ = fmt.Fprintf(w, "unknown flag %s\n", flag)
		return 2
	}
	if err != nil {
		_, _ = fmt.Fprintf(w, "the user Path was not changed: %v\n", err)
		return 1
	}
	switch {
	case !changed:
		_, _ = fmt.Fprintf(w, "the user Path already says what it should about %s\n", dir)
	case flag == AddFlag:
		_, _ = fmt.Fprintf(w, "added %s to the user Path\n", dir)
	default:
		_, _ = fmt.Fprintf(w, "removed %s from the user Path\n", dir)
	}
	return 0
}
