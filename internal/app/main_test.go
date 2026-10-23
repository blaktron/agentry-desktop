package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
	"github.com/zalando/go-keyring"

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
//
// The OS keychain is held in memory for the whole package, since
// settings.TokenFor reads the keychain before the settings file, by host and
// port: a test that reached it unmocked would read whatever token an earlier
// run left there for 127.0.0.1 and its test server's port, and its fake
// registry would answer that stale token with a 401 or an unreadable token
// list.
func TestMain(m *testing.M) {
	keyring.MockInit()
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
	// No test meets the Documents folder of the person running it: a
	// Deliverables folder none chose is a scratch one.
	docs := filepath.Join(dir, "documents", "Agentry")
	defaultDeliverablesRoot = func() (string, error) { return docs, nil }
	// No test looks a server's icon up in the MCP Registry.
	findIcon = func(context.Context, manifest.MCPServer) ([]byte, string, error) {
		return nil, "", errors.New("no network in tests")
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
