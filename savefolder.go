package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The Build tab writes a QuickAgent without asking where (agentry-desktop#585,
// D-060): Save, Install and Submit put it in a folder of its own under the
// default output folder, the one a run writes into. Go names and makes that
// folder, so the page still names no path (D-030): it passes the QuickAgent's
// name, and gets back a folder that did not exist before the call.

// saveRoot is the default output folder: Settings → Run defaults → Output
// folder when one is saved, else ~/Downloads/agentry-out, as for a run.
func saveRoot() (string, error) {
	if st, err := settings.Load(); err == nil && st.DefaultOutputDir != "" {
		return st.DefaultOutputDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, "Downloads", "agentry-out"), nil
}

// saveFolderName turns what the page passed into one path element: lower
// case letters, digits, '-', '_' and '.', anything else a '-', with no
// leading or trailing '-' or '.'. A name with nothing left is "quickagent".
func saveFolderName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	s := strings.Trim(b.String(), "-.")
	if len(s) > 64 {
		s = strings.Trim(s[:64], "-.")
	}
	if s == "" {
		return "quickagent"
	}
	return s
}

// newFolderIn makes a new, empty folder in root named for name: name, or
// name-2, name-3… when that is taken. It never returns a folder that existed
// before the call, so nothing already there is written over.
func newFolderIn(root, name string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve the output folder: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create the output folder: %w", err)
	}
	base := saveFolderName(name)
	for i := 1; i <= 999; i++ {
		n := base
		if i > 1 {
			n = fmt.Sprintf("%s-%d", base, i)
		}
		p := filepath.Join(abs, n)
		err := os.Mkdir(p, 0o755)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("create %s: %w", p, err)
		}
	}
	return "", fmt.Errorf("%s already holds 999 folders named for %s; clear some out", abs, base)
}

// NewSaveFolder makes the folder the Build tab writes a QuickAgent into at
// its first Save, Install or Submit: new and empty, under the default output
// folder, named for name (agentry-desktop#585). The folder is admitted for
// this session's bindings as a picked one is, since Go chose it.
func (b *BuildAPI) NewSaveFolder(name string) (string, error) {
	root, err := saveRoot()
	if err != nil {
		return "", err
	}
	dir, err := newFolderIn(root, name)
	if err != nil {
		return "", err
	}
	b.app.markPicked(dir)
	return dir, nil
}
