//go:build windows

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// windowsApplier runs the release's per-user setup executable silently.
//
// The installer does the work, which is why it is per-user: NSIS with
// RequestExecutionLevel user installs into %LOCALAPPDATA%\Programs and
// writes an HKCU uninstall key, so /S replaces the installed files and
// relaunches with no UAC prompt at all. A per-machine installer could not be
// updated this way and would have to ask for an administrator password,
// which the updater must never do.
type windowsApplier struct {
	// installDir is where this copy is installed; empty means "the directory
	// of the running executable". A field so the acceptance can drive it.
	installDir string
	// start runs the installer. A field so a test can observe the command
	// rather than run an installer.
	start func(cmd *exec.Cmd) error
}

func newApplier() Applier { return &windowsApplier{} }

func (a *windowsApplier) dir() (string, error) {
	if a.installDir != "" {
		return a.installDir, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}

func (a *windowsApplier) Ready() error {
	dir, err := a.dir()
	if err != nil {
		return err
	}
	// The installer replaces the files in place, so this process must be able
	// to write there. A machine-wide install under Program Files cannot, and
	// the person is told to download it rather than asked to elevate.
	return writableDir(dir)
}

func (a *windowsApplier) Apply(setupPath string, relaunch bool) error {
	dir, err := a.dir()
	if err != nil {
		return err
	}
	if err := writableDir(dir); err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(setupPath), ".exe") {
		return fmt.Errorf("%s is not an installer", filepath.Base(setupPath))
	}

	// /S is NSIS's silent flag, and /D sets the install directory so the
	// update lands where this copy already is rather than at the default —
	// the same reason the installer records InstallLocation.
	//
	// The command line is built by hand because NSIS requires /D to be LAST,
	// UNQUOTED and absolute, and Go's argument escaping does the opposite:
	// the default install directory is %LOCALAPPDATA%\Programs\Agentry
	// Desktop, which contains a space, so exec.Command would emit
	// "/D=C:\...\Agentry Desktop" — quoted, which NSIS documents as
	// unsupported and silently ignores. Ignoring it would install to the
	// default location instead of over this copy, leaving two installations.
	// A trailing backslash would also break it, so it is trimmed.
	target := strings.TrimRight(dir, `\`)
	cmd := exec.Command(setupPath) //nolint:gosec // a path this process verified and wrote
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: fmt.Sprintf(`"%s" /S /D=%s`, setupPath, target),
	}
	cmd.Dir = filepath.Dir(setupPath)

	start := a.start
	if start == nil {
		start = func(c *exec.Cmd) error { return c.Start() }
	}
	if err := start(cmd); err != nil {
		return fmt.Errorf("starting the installer: %w", err)
	}

	// The downloaded installer is deliberately NOT removed: it is the program
	// that is about to run, and Windows holds a running executable open. It
	// is written under the user's temp directory, which Windows cleans.
	//
	// This process then gets out of the way rather than waiting, because
	// Windows will not let the installer overwrite an executable that is
	// still running — and the installer relaunches the app itself, from the
	// .onInstSuccess of project.nsi, which fires under /S where the finish
	// page does not. Without relaunch, it is the caller's business to exit;
	// the app is quitting anyway.
	//
	// UNVERIFIED in CI: there is no Windows CI runner, so the sequence above
	// — silent install over a running copy, then relaunch — is reasoned from
	// NSIS's documented behaviour. The acceptance in
	// scripts/acceptance/windows-install.ps1 is where it is observed.
	if relaunch {
		os.Exit(0)
	}
	return nil
}
