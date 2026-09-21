package pathguard

import (
	"path/filepath"
	"testing"
)

func TestUnder(t *testing.T) {
	dir := filepath.Join(string(filepath.Separator), "home", "me", ".claude", "skills", "demo")
	for p, want := range map[string]bool{
		filepath.Join(dir, "SKILL.md"):        true,
		filepath.Join(dir, "docs", "more.md"): true,
		dir:                                   false,
		filepath.Join(dir, ".."):              false,
		filepath.Join(dir, "..", "other", "SKILL.md"):         false,
		filepath.Join(dir, "..", "..", "..", "..", ".bashrc"): false,
		filepath.Join(string(filepath.Separator), "etc"):      false,
	} {
		if got := Under(dir, p); got != want {
			t.Errorf("Under(%s, %s) = %v, want %v", dir, p, got, want)
		}
	}
}
