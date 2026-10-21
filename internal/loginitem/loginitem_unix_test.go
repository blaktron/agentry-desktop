//go:build linux || darwin

package loginitem

import (
	"os"
	"testing"
)

// The entry is written, read back as on for the app it names and off for
// another, and removed; removing none is not an error.
func TestTheEntryIsWrittenReadAndRemoved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	exe := "/opt/agentry/agentry-desktop"
	if on, err := Enabled(exe); err != nil || on {
		t.Fatalf("before: %v %v", on, err)
	}
	if err := Enable(exe); err != nil {
		t.Fatal(err)
	}
	p, _ := path()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("no entry at %s: %v", p, err)
	}
	if on, err := Enabled(exe); err != nil || !on {
		t.Fatalf("after Enable: %v %v", on, err)
	}
	if on, _ := Enabled("/elsewhere/agentry-desktop"); on {
		t.Fatal("an entry for another copy of the app reads as this one's")
	}
	if err := Disable(); err != nil {
		t.Fatal(err)
	}
	if on, _ := Enabled(exe); on {
		t.Fatal("still on after Disable")
	}
	if err := Disable(); err != nil {
		t.Fatalf("a second Disable: %v", err)
	}
}
