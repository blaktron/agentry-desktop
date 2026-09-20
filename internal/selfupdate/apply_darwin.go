//go:build darwin

package selfupdate

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// darwinApplier swaps the running .app bundle for the one inside a verified
// zip and relaunches it.
//
// A bundle swap rather than an installer, because macOS has no installer to
// run: the dmg is a folder with a shortcut in it, and "installing" is a copy.
// The zip is what this uses (see ArtifactFor): unpacking needs no mounting,
// no hdiutil and no cleanup of a leaked mount.
type darwinApplier struct {
	// bundle is the .app being replaced; empty means "work it out from the
	// running executable". A field so the acceptance can drive it.
	bundle string
	// exec runs the relaunch. A field so a test can observe it rather than
	// having the test binary replaced.
	exec func(argv0 string, argv []string, envv []string) error
}

func newApplier() Applier { return &darwinApplier{} }

// bundlePath is .../Agentry Desktop.app, derived from the running binary at
// .../Contents/MacOS/agentry-desktop.
func (a *darwinApplier) bundlePath() (string, error) {
	if a.bundle != "" {
		return a.bundle, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", err
	}
	// .../X.app/Contents/MacOS/binary → .../X.app
	dir := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(dir, ".app") {
		return "", fmt.Errorf("this copy is not running from an application bundle (%s), so there is nothing to replace", exe)
	}
	return dir, nil
}

func (a *darwinApplier) Ready() error {
	bundle, err := a.bundlePath()
	if err != nil {
		return err
	}
	// The PARENT is what must be writable: replacing a bundle means creating
	// and renaming inside the folder that holds it. A person who dragged the
	// app to ~/Applications can; one whose admin put it in /Applications
	// cannot, and is told so rather than asked for a password.
	return writableDir(filepath.Dir(bundle))
}

func (a *darwinApplier) Apply(zipPath string, relaunch bool) error {
	bundle, err := a.bundlePath()
	if err != nil {
		return err
	}
	parent := filepath.Dir(bundle)
	if err := writableDir(parent); err != nil {
		return err
	}

	// Staged beside the installed copy, not in a temp directory: the swap is
	// a rename, and a rename only works within one filesystem.
	stage, err := os.MkdirTemp(parent, ".agentry-selfupdate-*")
	if err != nil {
		return fmt.Errorf("%w: %s (%v)", ErrNotWritable, parent, err)
	}
	defer func() { _ = os.RemoveAll(stage) }()

	// ditto, not unzip: it is what wrote the archive (the Makefile's
	// package-darwin) and it keeps the symlinks, permissions and extended
	// attributes a bundle's signature covers. unzip drops some of them and
	// the result fails codesign.
	if out, err := exec.Command("/usr/bin/ditto", "-x", "-k", zipPath, stage).CombinedOutput(); err != nil {
		return fmt.Errorf("unpacking the download: %v: %s", err, strings.TrimSpace(string(out)))
	}

	// Consumed: the zip has been unpacked and nothing reads it again. Removed
	// here rather than left to a defer, because the exec below replaces this
	// process and no defer of ours will ever run — tens of megabytes would
	// stay in the temp directory after every successful update.
	_ = os.Remove(zipPath)

	unpacked, err := soleBundle(stage)
	if err != nil {
		return err
	}
	// The signature is checked again on what was unpacked. The bytes were
	// already proven against the release's signed checksums, so this is not
	// the trust decision — it catches an archive that unpacked wrongly, which
	// would otherwise be discovered by Gatekeeper after the swap, when the
	// old copy is gone.
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", unpacked).CombinedOutput(); err != nil {
		return fmt.Errorf("the unpacked application does not verify, so it has not been installed: %v: %s", err, strings.TrimSpace(string(out)))
	}

	if err := swapDir(unpacked, bundle); err != nil {
		return err
	}

	// Removed here, not by the defer above: the exec below replaces this
	// process, so no defer of ours runs and the staging directory would be
	// left inside the Applications folder after every successful update.
	_ = os.RemoveAll(stage)

	if !relaunch {
		return nil
	}
	// Relaunch by execing `open`, which starts the new copy as a fresh
	// application and lets this process exit. Not exec of the new binary
	// directly: that would inherit this process's session and the window
	// server would treat it as the same, already-quitting, application.
	run := a.exec
	if run == nil {
		run = syscall.Exec
	}
	return run("/usr/bin/open", []string{"open", "-n", bundle}, os.Environ())
}

// soleBundle finds the one .app in a freshly unpacked directory, refusing
// anything else: an archive holding two applications, or none, is not one of
// ours however well it verified.
func soleBundle(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var found []string
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			found = append(found, filepath.Join(dir, e.Name()))
		}
	}
	if len(found) != 1 {
		return "", fmt.Errorf("the download unpacked to %d application bundles, and a release has exactly one", len(found))
	}
	return found[0], nil
}
