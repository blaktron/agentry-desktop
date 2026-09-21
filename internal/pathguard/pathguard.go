// Package pathguard is the one refusal every recursive delete of a derived
// directory in the desktop passes: uninstall's, and install's clearing of a
// managed skill directory before it is rewritten. It lived in uninstall
// alone until the security review of 2026-09-20 (§8.2) found install
// clearing a directory named by an unvalidated registry response with no
// such check.
package pathguard

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
)

// RefuseDangerousDir refuses a directory this code must never delete
// recursively: one that is not absolute, a filesystem root, the home
// directory itself, or an ancestor of it. The relocation variables are the
// person's to set, so a directory anywhere else is theirs to lose.
func RefuseDangerousDir(dir, home string) error {
	if dir == "" || !filepath.IsAbs(dir) {
		return errors.New("not an absolute path")
	}
	clean := filepath.Clean(dir)
	if filepath.Dir(clean) == clean {
		return errors.New("a filesystem root")
	}
	home = filepath.Clean(home)
	if runtime.GOOS == "windows" {
		// Paths compare case-insensitively there: c:\users\bob is C:\Users\Bob.
		clean, home = strings.ToLower(clean), strings.ToLower(home)
	}
	rel, err := filepath.Rel(clean, home)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("the home directory or one of its ancestors")
	}
	return nil
}

// Under reports whether p is strictly inside dir (after cleaning both):
// the check an export makes before writing a file at a manifest-named path.
func Under(dir, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(p))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}
