package main

import (
	"os"
	"testing"
)

// TestMain points the event emitter at nothing for every test that does not
// capture events itself: a test's App carries a plain context, which the
// Wails runtime refuses with a fatal log, and a run now reports its progress
// from its first moment (agentry-desktop#465).
func TestMain(m *testing.M) {
	emitEvent = func(*App, string, ...any) {}
	os.Exit(m.Run())
}
