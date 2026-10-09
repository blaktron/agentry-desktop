//go:build windows

package clipath

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// scriptCLI is where the site's install.ps1 puts the CLI by default:
// %LOCALAPPDATA%\Programs\agentry\bin\agentry.exe (agentry-web
// scripts/install/install.ps1). "" when LOCALAPPDATA is unset.
func scriptCLI(getenv func(string) string) string {
	base := strings.TrimSpace(getenv("LOCALAPPDATA"))
	if base == "" {
		return ""
	}
	return filepath.Join(base, "Programs", "agentry", "bin", cliName)
}

// bundledBeside is the installer's $INSTDIR\bin\agentry.exe.
func bundledBeside(dir string) string { return filepath.Join(dir, BinDir, cliName) }

// refreshAllowed: the installer runs per-user (REQUEST_EXECUTION_LEVEL user),
// and the app as the person, so LOCALAPPDATA is theirs.
func refreshAllowed() bool { return true }

// createNoWindow is CREATE_NO_WINDOW: --version runs with no console window
// flashing up over the app.
const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// swapIn renames tmp over target. A running agentry.exe (an MCP session, a
// long run) cannot be replaced or deleted, but it can be renamed, as the
// installer and install.ps1 do: target goes aside to <target>.old first, and
// is put back if the new file cannot take its place.
func swapIn(tmp, target string) (aside bool, err error) {
	old := target + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.Rename(target, old); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Rename(old, target)
		return false, err
	}
	return os.Remove(old) != nil, nil
}
