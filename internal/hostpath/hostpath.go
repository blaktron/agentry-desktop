// Package hostpath gives the app the PATH a terminal would have given it,
// for the programs it runs, on a platform whose GUI launch hands it less
// (agentry-desktop#573).
//
// An app opened from Finder or the Dock inherits launchd's PATH,
// /usr/bin:/bin:/usr/sbin:/sbin, not the shell's. Docker Desktop installs
// its CLI into ~/.docker/bin (or /usr/local/bin, when it was given admin
// rights), so the engine check's exec.LookPath("docker") fails and the app
// reports Docker missing with the engine running. Naming the binary alone
// would not do: Docker Desktop's config.json says credsStore "desktop", and
// the CLI looks docker-credential-desktop up on PATH for every pull. So the
// directory goes on PATH, and every docker child inherits it.
package hostpath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// darwinDirs are where a Mac's container engines put their CLIs, in the
// order they are tried: Docker Desktop's user-level and admin installs,
// Homebrew on Apple silicon (the CLI that Colima drives), Docker Desktop's
// own bundle (present even when it installed no links), OrbStack, and
// Rancher Desktop. Relative entries are under the home directory.
var darwinDirs = []string{
	".docker/bin",
	"/usr/local/bin",
	"/opt/homebrew/bin",
	"/Applications/Docker.app/Contents/Resources/bin",
	".orbstack/bin",
	".rd/bin",
}

// ForGUI extends this process's PATH on macOS with each of darwinDirs that
// exists here and is not on it already. Elsewhere it does nothing: a Linux
// desktop session and Windows both hand an app the PATH a terminal has.
// It runs once, first thing in main, before anything resolves a program.
func ForGUI() {
	home, _ := os.UserHomeDir()
	if p, ok := extendFor(runtime.GOOS, home, os.Getenv("PATH"), isDir); ok {
		_ = os.Setenv("PATH", p)
	}
}

// extendFor is ForGUI's rule with its inputs passed in: the PATH to set,
// and whether it differs from path.
func extendFor(goos, home, path string, isDir func(string) bool) (string, bool) {
	if goos != "darwin" {
		return path, false
	}
	var dirs []string
	for _, d := range darwinDirs {
		if !filepath.IsAbs(d) {
			if home == "" {
				continue
			}
			d = filepath.Join(home, d)
		}
		dirs = append(dirs, d)
	}
	out := Extend(path, dirs, isDir)
	return out, out != path
}

// Extend appends to path each of dirs that isDir accepts and that path does
// not already hold, keeping path's own entries first so a PATH the person
// set (an app started from a terminal) still decides which program runs.
func Extend(path string, dirs []string, isDir func(string) bool) string {
	sep := string(os.PathListSeparator)
	have := map[string]bool{}
	var parts []string
	if path != "" {
		parts = strings.Split(path, sep)
	}
	for _, p := range parts {
		have[filepath.Clean(p)] = true
	}
	for _, d := range dirs {
		d = filepath.Clean(d)
		if have[d] || !isDir(d) {
			continue
		}
		have[d] = true
		parts = append(parts, d)
	}
	return strings.Join(parts, sep)
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
