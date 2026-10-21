package loginitem

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// label is the LaunchAgent's label.
const label = "net.secureagentry.agentry-desktop"

// path is the LaunchAgent: ~/Library/LaunchAgents/<label>.plist.
func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

// Enable writes the LaunchAgent that launches exe hidden at sign-in; launchd
// reads it at the next sign-in.
func Enable(exe string) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(launchAgent(label, exe)), 0o644)
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

// Enabled reports whether the LaunchAgent is there and launches exe.
func Enabled(exe string) (bool, error) {
	p, err := path()
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(p) //nolint:gosec // the app's own LaunchAgent
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.Contains(string(b), "<string>"+xmlEscape(exe)+"</string>"), nil
}
