package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDocumentsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if got := documentsDir(); got != home {
		t.Errorf("no Documents folder: %q, want the home folder", got)
	}
	if err := os.Mkdir(filepath.Join(home, "Documents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := documentsDir(); got != filepath.Join(home, "Documents") {
		t.Errorf("~/Documents: %q", got)
	}
	// A desktop that moved or translated it says so in user-dirs.dirs.
	if err := os.MkdirAll(filepath.Join(home, "Dokumente"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "user-dirs.dirs"), []byte("XDG_DESKTOP_DIR=\"$HOME/Desktop\"\nXDG_DOCUMENTS_DIR=\"$HOME/Dokumente\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := documentsDir(); got != filepath.Join(home, "Dokumente") {
		t.Errorf("user-dirs.dirs: %q", got)
	}
}
