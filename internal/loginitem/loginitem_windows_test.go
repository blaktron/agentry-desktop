package loginitem

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The Run value is set, read back as on for the app it names and off for
// another, and deleted; deleting none is not an error. The test uses a key
// of its own under the current user, never the real Run key.
func TestTheRunValueIsSetReadAndDeleted(t *testing.T) {
	was := runKey
	runKey = `Software\AgentryDesktopTest\Run`
	t.Cleanup(func() {
		runKey = was
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\AgentryDesktopTest\Run`)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\AgentryDesktopTest`)
	})
	exe := `C:\Program Files\Agentry\agentry-desktop.exe`
	if on, err := Enabled(exe); err != nil || on {
		t.Fatalf("before: %v %v", on, err)
	}
	if err := Enable(exe); err != nil {
		t.Fatal(err)
	}
	if on, err := Enabled(exe); err != nil || !on {
		t.Fatalf("after Enable: %v %v", on, err)
	}
	if on, _ := Enabled(`D:\other\agentry-desktop.exe`); on {
		t.Fatal("a value for another copy reads as this one's")
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
