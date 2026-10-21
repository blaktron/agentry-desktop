package loginitem

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// path is the autostart entry: $XDG_CONFIG_HOME/autostart, else
// ~/.config/autostart.
func path() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "agentry-desktop.desktop"), nil
}

// Enable writes the entry that launches exe hidden at sign-in.
func Enable(exe string) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(desktopEntry(exe)), 0o644)
}

// Disable removes it; none is not an error.
func Disable() error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Enabled reports whether the entry is there and launches exe.
func Enabled(exe string) (bool, error) {
	p, err := path()
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(p) //nolint:gosec // the app's own entry
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(string(b), "Exec="+execQuote(exe)+" "+HiddenFlag) && !strings.Contains(string(b), "Hidden=true"), nil
}
