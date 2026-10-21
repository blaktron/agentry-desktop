//go:build linux || darwin

package app

import "testing"

// Start Agentry when you sign in writes the login item for this copy of the
// app, reads it back, and removes it.
func TestStartAgentryWhenYouSignInIsWrittenAndRemoved(t *testing.T) {
	r := newBGRig(t)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	was := executable
	executable = func() (string, error) { return "/opt/agentry/agentry-desktop", nil }
	t.Cleanup(func() { executable = was })
	if on, err := r.p.StartsAtSignIn(); err != nil || on {
		t.Fatalf("before: %v %v", on, err)
	}
	if on, err := r.p.SetStartsAtSignIn(true); err != nil || !on {
		t.Fatalf("on: %v %v", on, err)
	}
	if on, err := r.p.SetStartsAtSignIn(false); err != nil || on {
		t.Fatalf("off: %v %v", on, err)
	}
}
