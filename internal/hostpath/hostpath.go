// Package hostpath gives the app the PATH a terminal would have given it,
// for the programs it runs, on a platform whose GUI launch hands it
// less.
//
// macOS: an app opened from Finder or the Dock inherits launchd's PATH,
// /usr/bin:/bin:/usr/sbin:/sbin, not the shell's. Docker Desktop installs
// its CLI into ~/.docker/bin (or /usr/local/bin, when it was given admin
// rights), so the engine check's exec.LookPath("docker") fails and the app
// reports Docker missing with the engine running.
//
// Windows: a process inherits its parent's environment, and nothing on the
// way re-reads the registry. A self-update starts the installer from the
// running app and the installer starts the new app, so every release runs
// with the PATH of the very first launch. When Docker Desktop is installed
// after that, or moves (its per-user install lives under
// %LOCALAPPDATA%\Programs\DockerDesktop, the machine-wide one under
// %ProgramFiles%\Docker\Docker), the app never sees the new directory,
// though Explorer and every new terminal do.
//
// Naming the binary alone would not do on either: Docker Desktop's
// config.json says credsStore "desktop", and the CLI looks
// docker-credential-desktop up on PATH for every pull. So the directory goes
// on PATH, and every docker child inherits it.
package hostpath

import (
	"os"
	"path"
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

// windowsDirs are where a Windows machine's container engines put their
// CLIs, each under the environment variable it names: Docker Desktop's
// machine-wide and per-user installs, then Rancher Desktop's two. They
// follow the registry's PATH, which normally holds the one installed.
var windowsDirs = []struct{ env, rel string }{
	{"ProgramFiles", `Docker\Docker\resources\bin`},
	{"LOCALAPPDATA", `Programs\DockerDesktop\resources\bin`},
	{"ProgramFiles", `Rancher Desktop\resources\resources\win32\bin`},
	{"LOCALAPPDATA", `Programs\Rancher Desktop\resources\resources\win32\bin`},
}

// ForGUI extends this process's PATH with the directories a terminal opened
// now would find programs in, each only if it exists and is not on PATH
// already. On macOS those are darwinDirs; on Windows, the machine's and the
// user's Path as the registry holds them now, then windowsDirs. A Linux
// desktop session hands an app the PATH a terminal has, so there it does
// nothing. main runs it first, before anything resolves a program, and
// Docker detection runs it again before each check, so a Docker installed
// or moved while the app is open is found without a restart.
func ForGUI() {
	if p, ok := extendFor(runtime.GOOS, os.Getenv("PATH"), candidates(runtime.GOOS), isDir); ok {
		_ = os.Setenv("PATH", p)
	}
}

// candidates are the directories ForGUI offers on goos, read from this
// machine.
func candidates(goos string) []string {
	switch goos {
	case "darwin":
		home, _ := os.UserHomeDir()
		return darwinCandidates(home)
	case "windows":
		return windowsCandidates(registryPath(), os.Getenv)
	}
	return nil
}

// darwinCandidates resolves darwinDirs against home, skipping the
// home-relative ones when there is no home. Slash paths, so the macOS rule
// tests the same on any OS.
func darwinCandidates(home string) []string {
	var dirs []string
	for _, d := range darwinDirs {
		if !strings.HasPrefix(d, "/") {
			if home == "" {
				continue
			}
			d = path.Join(home, d)
		}
		dirs = append(dirs, d)
	}
	return dirs
}

// windowsCandidates is the registry's PATH entries (machine, then user, as
// Windows builds a new process's PATH) followed by windowsDirs resolved
// through getenv, skipping one whose variable is unset.
func windowsCandidates(registry []string, getenv func(string) string) []string {
	var dirs []string
	for _, d := range registry {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, d)
		}
	}
	for _, d := range windowsDirs {
		base := getenv(d.env)
		if base == "" {
			continue
		}
		dirs = append(dirs, strings.TrimRight(base, `\/`)+`\`+d.rel)
	}
	return dirs
}

// extendFor is ForGUI's rule with its inputs passed in: the PATH to set,
// and whether it differs from pathList.
func extendFor(goos, pathList string, dirs []string, isDir func(string) bool) (string, bool) {
	var out string
	switch goos {
	case "darwin":
		out = extend(pathList, dirs, isDir, ":", path.Clean)
	case "windows":
		out = extend(pathList, dirs, isDir, ";", windowsKey)
	default:
		return pathList, false
	}
	return out, out != pathList
}

// extend appends to pathList each of dirs that isDir accepts and that
// pathList does not already hold, keeping pathList's own entries first so a
// PATH the person set (an app started from a terminal) still decides which
// program runs. sep is the platform's list separator and key says when two
// entries name the same directory, so each rule is testable on any OS.
func extend(pathList string, dirs []string, isDir func(string) bool, sep string, key func(string) string) string {
	have := map[string]bool{}
	var parts []string
	if pathList != "" {
		parts = strings.Split(pathList, sep)
	}
	for _, p := range parts {
		have[key(p)] = true
	}
	for _, d := range dirs {
		k := key(d)
		if have[k] || !isDir(d) {
			continue
		}
		have[k] = true
		parts = append(parts, d)
	}
	return strings.Join(parts, sep)
}

// windowsKey is when two Windows PATH entries name the same directory:
// case does not count, nor does a trailing separator or a slash for a
// backslash.
func windowsKey(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "/", `\`)
	return strings.ToLower(strings.TrimRight(p, `\`))
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
