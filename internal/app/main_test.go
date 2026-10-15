package app

import (
	"os"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// TestMain points the event emitter at nothing for every test that does not
// capture events itself: a test's App carries a plain context, which the
// Wails runtime refuses with a fatal log, and a run now reports its progress
// from its first moment.
//
// It also points the config directory at a scratch one, which a test that
// needs its own overrides with t.Setenv: a run now writes the run history,
// and no test writes into the config of the person running it. And it sets
// the default registry a SecureAgentry build bakes in.
func TestMain(m *testing.M) {
	emitEvent = func(*App, string, ...any) {}
	dir, err := os.MkdirTemp("", "agentry-desktop-test-config-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv(configdir.EnvVar, dir)
	// The services are tested as a SecureAgentry build runs them: its
	// default registry is the SecureAgentry sign-in, so a test's own registry
	// is the one self-hosted registry. A build from source defaults to a
	// registry on this machine, which takes that place itself.
	settings.DefaultRegistryURL = "https://secureagentry.net"
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
