package run

// A run's output folder (D-044): Run defaults' saved folder is one every run
// shares, so each run is given a folder of its own inside it; a folder picked
// for one run is that run's.

import (
	"path/filepath"
	"testing"
)

func TestAFolderEveryRunSharesGetsARunFolderEach(t *testing.T) {
	root := t.TempDir()
	sig := &Signature{Namespace: "acme", Name: "notes"}
	a, err := resolveOutDir(root, true, sig, "run0001")
	if err != nil {
		t.Fatal(err)
	}
	b, err := resolveOutDir(root, true, sig, "run0002")
	if err != nil {
		t.Fatal(err)
	}
	if a != filepath.Join(root, "acme-notes-run0001") || b != filepath.Join(root, "acme-notes-run0002") {
		t.Fatalf("run folders %q and %q, want acme-notes-<runid> under %q", a, b, root)
	}
	// A folder picked for one run is that run's.
	if c, err := resolveOutDir(root, false, sig, "run0003"); err != nil || c != root {
		t.Fatalf("a picked folder resolved to %q, %v; want it as it is", c, err)
	}
}
