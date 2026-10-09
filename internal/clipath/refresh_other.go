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
// ${XDG_BIN_HOME:-$HOME/.local/bin}/agentry (agentry-web
// scripts/install/install.sh). "" on macOS, where the app carries no CLI to
// refresh it with (agentry-desktop#581), and when there is no home.
func scriptCLI(getenv func(string) string) string {
	if runtime.GOOS == "darwin" {
		return ""
	}
	if dir := strings.TrimSpace(getenv("XDG_BIN_HOME")); dir != "" && filepath.IsAbs(dir) {
		return filepath.Join(dir, cliName)
	}
	home := strings.TrimSpace(getenv("HOME"))
	if home == "" || !filepath.IsAbs(home) {
		return ""
	}
	return filepath.Join(home, ".local", "bin", cliName)
}

// bundledBeside is the packages' /usr/bin/agentry beside /usr/bin/agentry-desktop.
func bundledBeside(dir string) string {
	if runtime.GOOS == "darwin" {
		return ""
	}
	return filepath.Join(dir, cliName)
}

func hideWindow(*exec.Cmd) {}

// swapIn renames tmp over target; a running program keeps the inode it has.
func swapIn(tmp, target string) (bool, error) { return false, os.Rename(tmp, target) }
