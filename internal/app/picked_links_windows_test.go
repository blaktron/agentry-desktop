package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// junction makes link a directory junction to target, as mklink /J does;
// a junction needs no privilege, so failing to make one fails the test.
func junction(t *testing.T, link, target string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J %s %s: %v: %s", link, target, err, out)
	}
}

// filepath.EvalSymlinks does not follow a junction, so a check built on it
// alone would take picked\j\id_rsa to lie under picked.
func TestAJunctionOutOfAPickedFolderIsRefused(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "id_rsa"), []byte("not for upload"), 0o600); err != nil {
		t.Fatal(err)
	}
	picked := t.TempDir()
	junction(t, filepath.Join(picked, "j"), outside)
	gone := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(gone, 0o700); err != nil {
		t.Fatal(err)
	}
	junction(t, filepath.Join(picked, "dangling"), gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	app := &App{ctx: context.Background()}
	app.markPicked(picked)
	through := filepath.Join(picked, "j", "id_rsa")
	for name, p := range map[string]string{
		"a file through the junction":                  through,
		"the junction itself":                          filepath.Join(picked, "j"),
		"a file not made yet under the junction":       filepath.Join(picked, "j", "new", "out.txt"),
		"a junction whose target is gone":              filepath.Join(picked, "dangling"),
		"a file under a junction whose target is gone": filepath.Join(picked, "dangling", "out.txt"),
	} {
		if err := app.requirePicked("input", p); err == nil {
			t.Errorf("%s (%s) passed", name, p)
		}
	}
	if err := app.requirePicked("input", through); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("a file through the junction: %v", err)
	}

	// A binding refuses it too, before anything runs.
	r := &RunAPI{app: app}
	if _, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", []string{through}, "", nil, 0, 0, false, nil, false, false, nil, false, nil, nil); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("RunAgent with an input through a junction: %v", err)
	}
}

// A folder picked through a junction, as a redirected Documents folder
// is, still admits what lies under it, and so does a junction that stays
// inside the picked folder.
func TestAFolderPickedThroughAJunctionStillAdmitsItsFiles(t *testing.T) {
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "sub", "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	docs := filepath.Join(t.TempDir(), "Documents")
	junction(t, docs, real)
	junction(t, filepath.Join(real, "inner"), filepath.Join(real, "sub"))
	app := &App{ctx: context.Background()}
	app.markPicked(docs)
	for _, p := range []string{
		filepath.Join(docs, "sub", "a.txt"),
		filepath.Join(docs, "inner", "a.txt"),
		filepath.Join(docs, "new", "out.txt"),
		docs,
	} {
		if err := app.requirePicked("input", p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
}

func TestTheFinalPathLosesItsPrefix(t *testing.T) {
	for in, want := range map[string]string{
		`\\?\C:\Users\me\Documents`:     `C:\Users\me\Documents`,
		`\\?\UNC\server\share\notes.md`: `\\server\share\notes.md`,
		`C:\plain`:                      `C:\plain`,
	} {
		if got := trimFinalPathPrefix(in); got != want {
			t.Errorf("trimFinalPathPrefix(%q) = %q, want %q", in, got, want)
		}
	}
}

// A reserved device name under a picked folder never passes as the device:
// NUL opens the null device, whose handle has no final path, so it is
// refused; a name Windows takes as a device there is refused the same way,
// and one it takes as an ordinary name passes only as a name not made yet.
func TestADeviceNameUnderAPickedFolderIsNeverTheDevice(t *testing.T) {
	picked := t.TempDir()
	app := &App{ctx: context.Background()}
	app.markPicked(picked)
	if err := app.requirePicked("input", filepath.Join(picked, "NUL")); err == nil {
		t.Error("NUL under a picked folder passed")
	}
	for _, name := range []string{"CON", "CON.txt", "COM1", "AUX", "PRN"} {
		p := filepath.Join(picked, name)
		if err := app.requirePicked("input", p); err == nil {
			if _, serr := os.Lstat(p); !errors.Is(serr, fs.ErrNotExist) {
				t.Errorf("%s passed though it names something that exists: %v", name, serr)
			}
		}
	}
}
