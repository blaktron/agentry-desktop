package main

import (
	"os"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
)

// TestMain points the event emitter at nothing for every test that does not
// capture events itself: a test's App carries a plain context, which the
// Wails runtime refuses with a fatal log, and a run now reports its progress
// from its first moment.
//
// It also points the config directory at a scratch one, which a test that
// needs its own overrides with t.Setenv: a run now writes the run history,
// and no test writes into the config of the person running it.
func TestMain(m *testing.M) {
	emitEvent = func(*App, string, ...any) {}
	dir, err := os.MkdirTemp("", "agentry-desktop-test-config-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv(configdir.EnvVar, dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
