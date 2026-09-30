package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// documentsDir is the person's Documents folder, where Save's picker opens:
// on Linux the XDG user directory when user-dirs.dirs names one, else
// ~/Documents where it exists, else the home folder. It is a starting
// point for the native picker and nothing more: the person picks.
func documentsDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	if d := xdgDocumentsDir(home); d != "" {
		return d
	}
	if d := filepath.Join(home, "Documents"); isDir(d) {
		return d
	}
	return home
}

// xdgDocumentsDir reads XDG_DOCUMENTS_DIR from user-dirs.dirs, where
// Linux desktops record a Documents folder that may be translated or moved.
func xdgDocumentsDir(home string) string {
	cfg := os.Getenv("XDG_CONFIG_HOME")
	if cfg == "" {
		cfg = filepath.Join(home, ".config")
	}
	f, err := os.Open(filepath.Join(cfg, "user-dirs.dirs")) //nolint:gosec // the person's own config
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "XDG_DOCUMENTS_DIR=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		if rest, ok := strings.CutPrefix(v, "$HOME"); ok {
			v = home + rest
		}
		// A value of $HOME itself means the desktop has no Documents folder.
		if filepath.Clean(v) != filepath.Clean(home) && isDir(v) {
			return v
		}
	}
	return ""
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
