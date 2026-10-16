//go:build !windows

package clipath

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// scriptCLI is where the site's install.sh puts the CLI by default:
// ${XDG_BIN_HOME:-$HOME/.local/bin}/agentry. "" when there is no home. On
// macOS the app's own command is a link there to the CLI the bundle carries,
// which Refresh leaves alone; a copy the script put there is refreshed as on
// Linux.
func scriptCLI(getenv func(string) string) string {
	if dir := strings.TrimSpace(getenv("XDG_BIN_HOME")); dir != "" && filepath.IsAbs(dir) {
		return filepath.Join(dir, cliName)
	}
	home := strings.TrimSpace(getenv("HOME"))
	if home == "" || !filepath.IsAbs(home) {
		return ""
	}
	return filepath.Join(home, ".local", "bin", cliName)
}

// packagedDir is where the .deb and .rpm put the app and the CLI.
const packagedDir = "/usr/bin"

// bundledBeside is the CLI carried beside the app's executable in dir: on
// Linux the packages' /usr/bin/agentry, and only for the app the packages
// installed (a tarball or a build run from a home directory has no
// pipeline-carried CLI beside it, and whatever file sits there is not copied
// onto the PATH); on macOS the bundle's Contents/Resources/bin/agentry, for
// an executable in <bundle>.app/Contents/MacOS.
func bundledBeside(dir string) string {
	dir = filepath.Clean(dir)
	if runtime.GOOS == "darwin" {
		contents := filepath.Dir(dir)
		if filepath.Base(dir) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(filepath.Dir(contents), ".app") {
			return ""
		}
		return filepath.Join(contents, "Resources", "bin", cliName)
	}
	if dir != packagedDir {
		return ""
	}
	return filepath.Join(packagedDir, cliName)
}

// refreshAllowed is false for root: the refresh runs one person's script copy
// and writes it, which an app started under sudo or pkexec must not do with
// another account's HOME.
func refreshAllowed() bool { return os.Geteuid() != 0 }

func hideWindow(*exec.Cmd) {}

// swapIn renames tmp over target; a running program keeps the inode it has.
func swapIn(tmp, target string) (bool, error) { return false, os.Rename(tmp, target) }
