package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The swap and the writability check are plain file operations, so they are
// tested on whatever platform runs the tests. The platform-specific appliers
// around them (running an NSIS installer, execing `open`) are not: they need
// the platform, and the acceptance scripts are where they are proven.

func TestSwapDirReplacesTheInstalledCopy(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "Agentry Desktop.app")
	src := filepath.Join(root, "staged.app")
	writeTree(t, dst, map[string]string{"Contents/MacOS/app": "old", "Contents/Info.plist": "old plist"})
	writeTree(t, src, map[string]string{"Contents/MacOS/app": "new", "Contents/Resources/extra": "new only"})

	if err := swapDir(src, dst); err != nil {
		t.Fatalf("swapDir: %v", err)
	}

	if got := read(t, filepath.Join(dst, "Contents/MacOS/app")); got != "new" {
		t.Errorf("the installed copy is %q, want the new one", got)
	}
	if _, err := os.Stat(filepath.Join(dst, "Contents/Resources/extra")); err != nil {
		t.Error("a file only the new copy has is missing, so the swap merged rather than replaced")
	}
	// A file only the OLD copy had must be gone: a swap replaces.
	if _, err := os.Stat(filepath.Join(dst, "Contents/Info.plist")); !os.IsNotExist(err) {
		t.Error("a file from the old copy survived, so the swap merged rather than replaced")
	}
	// Nothing is left lying beside it.
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.Name() != "Agentry Desktop.app" {
			t.Errorf("the swap left %s behind", e.Name())
		}
	}
}

// The property that matters when something goes wrong: a failed swap must
// leave a working application, not an empty space where one was.
func TestSwapDirPutsTheOldCopyBackWhenTheNewOneCannotBeMoved(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "Agentry Desktop.app")
	writeTree(t, dst, map[string]string{"Contents/MacOS/app": "old"})
	// A source that does not exist: the rename into place fails after the old
	// copy has already been moved aside.
	src := filepath.Join(root, "never-staged.app")

	err := swapDir(src, dst)
	if err == nil {
		t.Fatal("swapping in a source that does not exist reported success")
	}
	if got := read(t, filepath.Join(dst, "Contents/MacOS/app")); got != "old" {
		t.Fatalf("the old copy was not put back; the person is left with no application. dst holds %q", got)
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.Name() != "Agentry Desktop.app" {
			t.Errorf("a half-finished swap left %s behind", e.Name())
		}
	}
}

func TestWritableDirAcceptsADirectoryThisProcessCanWrite(t *testing.T) {
	if err := writableDir(t.TempDir()); err != nil {
		t.Fatalf("writableDir on a fresh temp dir: %v", err)
	}
}

func TestWritableDirRefusesWhatCannotBeWritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, which can write anything; the refusal cannot be observed")
	}
	dir := filepath.Join(t.TempDir(), "readonly")
	if err := os.Mkdir(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := writableDir(dir)
	if !errors.Is(err, ErrNotWritable) {
		t.Fatalf("err = %v, want ErrNotWritable", err)
	}
	// The message must name the directory: a person told "cannot write" with
	// no path has been told nothing they can act on.
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("message %q does not name the directory", err)
	}
}

func TestWritableDirRefusesSomewhereThatDoesNotExist(t *testing.T) {
	if err := writableDir(filepath.Join(t.TempDir(), "no-such-place")); !errors.Is(err, ErrNotWritable) {
		t.Fatalf("err = %v, want ErrNotWritable", err)
	}
}

// Linux never applies: a .deb or .rpm is the package manager's.
func TestLinuxApplierRefuses(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("this platform does apply")
	}
	a := ApplierFor()
	if err := a.Ready(); !errors.Is(err, ErrNotApplicable) {
		t.Errorf("Ready() = %v, want ErrNotApplicable", err)
	}
	if err := a.Apply("/tmp/whatever", true); !errors.Is(err, ErrNotApplicable) {
		t.Errorf("Apply() = %v, want ErrNotApplicable", err)
	}
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}
