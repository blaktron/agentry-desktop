//go:build darwin

package selfupdate

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These drive the REAL darwinApplier against a real bundle in a temporary
// Applications folder. They exist because a shell acceptance that
// re-implemented the swap in bash would pass with apply_darwin.go deleted.
// A test that cannot fail when the code is wrong is worse than no test, so
// the shell script runs this.
//
// What is still not covered here is the relaunch: it ends in an exec of
// `open`, which needs a console session CI does not have.
// The exec is a field, so these observe what would have been run.

// fakeBundle builds a .app with a real ad-hoc signature, which is what a
// release carries and what the applier re-verifies.
func fakeBundle(t *testing.T, dir, name, marker string) string {
	t.Helper()
	app := filepath.Join(dir, name)
	macos := filepath.Join(app, "Contents", "MacOS")
	if err := os.MkdirAll(macos, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(macos, "agentry-desktop"), []byte("#!/bin/sh\necho "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>agentry-desktop</string>
<key>CFBundleExecutable</key><string>agentry-desktop</string>
<key>CFBundleIdentifier</key><string>net.secureagentry.desktop.test</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", app).CombinedOutput(); err != nil {
		t.Fatalf("ad-hoc signing the test bundle: %v: %s", err, out)
	}
	return app
}

// zipOf packages a bundle the way the Makefile's package-darwin does.
func zipOf(t *testing.T, app, out string) string {
	t.Helper()
	if err := exec.Command("/usr/bin/ditto", "-c", "-k", "--keepParent", app, out).Run(); err != nil {
		t.Fatalf("zipping the test bundle: %v", err)
	}
	return out
}

func TestDarwinApplierSwapsTheBundleAndKeepsNothingBehind(t *testing.T) {
	root := t.TempDir()
	apps := filepath.Join(root, "Applications")
	if err := os.Mkdir(apps, 0o755); err != nil {
		t.Fatal(err)
	}
	installed := fakeBundle(t, apps, "agentry-desktop.app", "old")
	// A file only the installed copy has, so a merge can be told from a swap.
	stale := filepath.Join(installed, "Contents", "Resources", "only-in-the-old-one")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	newSrc := fakeBundle(t, t.TempDir(), "agentry-desktop.app", "new")
	zip := zipOf(t, newSrc, filepath.Join(t.TempDir(), "release.zip"))

	var ranArgv []string
	a := &darwinApplier{
		bundle: installed,
		exec: func(argv0 string, argv []string, _ []string) error {
			ranArgv = append([]string{argv0}, argv...)
			return nil
		},
	}
	if err := a.Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if err := a.Apply(zip, true); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(installed, "Contents", "MacOS", "agentry-desktop"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "new") {
		t.Error("the installed bundle is not the one that was applied")
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("a file from the replaced copy survived: the swap merged")
	}
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", installed).CombinedOutput(); err != nil {
		t.Errorf("the installed bundle does not verify after the swap: %v: %s", err, out)
	}

	// Nothing beside the application: not the staging directory, not the
	// moved-aside copy. This is the assertion the shell acceptance could not
	// make, because it cleaned its own staging directory.
	entries, _ := os.ReadDir(apps)
	for _, e := range entries {
		if e.Name() != "agentry-desktop.app" {
			t.Errorf("the swap left %s in the Applications folder", e.Name())
		}
	}
	// And the downloaded zip is consumed.
	if _, err := os.Stat(zip); !os.IsNotExist(err) {
		t.Error("the downloaded archive was left behind")
	}

	if len(ranArgv) == 0 || !strings.Contains(strings.Join(ranArgv, " "), "open") {
		t.Errorf("the relaunch was not attempted: %v", ranArgv)
	}
}

// Quitting replaces the application and stops; it does not reopen what the
// person just closed.
func TestDarwinApplierDoesNotRelaunchWhenNotAsked(t *testing.T) {
	root := t.TempDir()
	apps := filepath.Join(root, "Applications")
	if err := os.Mkdir(apps, 0o755); err != nil {
		t.Fatal(err)
	}
	installed := fakeBundle(t, apps, "agentry-desktop.app", "old")
	zip := zipOf(t, fakeBundle(t, t.TempDir(), "agentry-desktop.app", "new"), filepath.Join(t.TempDir(), "r.zip"))

	ran := false
	a := &darwinApplier{bundle: installed, exec: func(string, []string, []string) error { ran = true; return nil }}
	if err := a.Apply(zip, false); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if ran {
		t.Error("quitting reopened the application")
	}
	body, _ := os.ReadFile(filepath.Join(installed, "Contents", "MacOS", "agentry-desktop"))
	if !strings.Contains(string(body), "new") {
		t.Error("the swap did not happen")
	}
}

// A copy the person cannot replace is refused, and nothing is touched.
func TestDarwinApplierRefusesAnUnwritableParent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which can write anything")
	}
	root := t.TempDir()
	apps := filepath.Join(root, "Applications")
	if err := os.Mkdir(apps, 0o755); err != nil {
		t.Fatal(err)
	}
	installed := fakeBundle(t, apps, "agentry-desktop.app", "old")
	zip := zipOf(t, fakeBundle(t, t.TempDir(), "agentry-desktop.app", "new"), filepath.Join(t.TempDir(), "r.zip"))
	if err := os.Chmod(apps, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(apps, 0o700) })

	a := &darwinApplier{bundle: installed, exec: func(string, []string, []string) error {
		t.Error("it relaunched after refusing")
		return nil
	}}
	if err := a.Ready(); !errors.Is(err, ErrNotWritable) {
		t.Fatalf("Ready() = %v, want ErrNotWritable", err)
	}
	if err := a.Apply(zip, true); !errors.Is(err, ErrNotWritable) {
		t.Fatalf("Apply() = %v, want ErrNotWritable", err)
	}
	_ = os.Chmod(apps, 0o700)
	body, _ := os.ReadFile(filepath.Join(installed, "Contents", "MacOS", "agentry-desktop"))
	if !strings.Contains(string(body), "old") {
		t.Error("the installed copy was changed despite the refusal")
	}
}

// An archive that does not hold exactly one application is refused, whatever
// else it verified.
func TestDarwinApplierRefusesAnArchiveThatIsNotOneApplication(t *testing.T) {
	root := t.TempDir()
	apps := filepath.Join(root, "Applications")
	if err := os.Mkdir(apps, 0o755); err != nil {
		t.Fatal(err)
	}
	installed := fakeBundle(t, apps, "agentry-desktop.app", "old")

	// Two bundles in one archive.
	src := t.TempDir()
	fakeBundle(t, src, "one.app", "a")
	fakeBundle(t, src, "two.app", "b")
	zip := filepath.Join(t.TempDir(), "two.zip")
	if err := exec.Command("/usr/bin/ditto", "-c", "-k", src, zip).Run(); err != nil {
		t.Fatal(err)
	}

	a := &darwinApplier{bundle: installed, exec: func(string, []string, []string) error { return nil }}
	err := a.Apply(zip, true)
	if err == nil || !strings.Contains(err.Error(), "application bundles") {
		t.Fatalf("err = %v, want the two-bundle archive refused", err)
	}
	body, _ := os.ReadFile(filepath.Join(installed, "Contents", "MacOS", "agentry-desktop"))
	if !strings.Contains(string(body), "old") {
		t.Error("the installed copy was replaced from an archive that was refused")
	}
}
