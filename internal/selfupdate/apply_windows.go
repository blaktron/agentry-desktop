//go:build windows

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// windowsApplier runs the release's per-user setup executable silently.
//
// The installer does the work, which is why M2 made it per-user (D11): NSIS
// with RequestExecutionLevel user installs into %LOCALAPPDATA%\Programs and
// writes an HKCU uninstall key, so /S replaces the installed files and
// relaunches with no UAC prompt at all. A per-machine installer could not be
// updated this way and would have to ask for an administrator password,
// which plan §3 forbids.
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

func (a *windowsApplier) Apply(setupPath string) error {
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
	// the same reason the installer records InstallLocation (M2). /D must be
	// last and unquoted: that is NSIS's rule, not a style choice.
	cmd := exec.Command(setupPath, "/S", "/D="+dir) //nolint:gosec // a path this process verified and wrote
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
	// The installer replaces this executable, so this process gets out of the
	// way rather than waiting: it cannot be running from the file being
	// overwritten. The installer relaunches the app when it finishes, which
	// is what the "Launch Agentry Desktop" page of the NSIS script does under
	// /S as well.
	os.Exit(0)
	return nil
}
