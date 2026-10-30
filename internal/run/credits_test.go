package run

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Choosing Agentry credits records model provider agentry for the
// SecureAgentry registry this build is for, with no key, once that registry
// holds a sign-in; signed out, or with a model or key, it is refused and the
// policy is not written.
func TestApplySetupRecordsAgentryCreditsForTheSignedInRegistry(t *testing.T) {
	isolateRunEnv(t)
	keyring.MockInit()
	was := settings.DefaultRegistryURL
	settings.DefaultRegistryURL = "https://secureagentry.net"
	t.Cleanup(func() { settings.DefaultRegistryURL = was })
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}

	err = ApplySetup(context.Background(), SetupOpts{Provider: policy.ProviderAgentry}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "signed in to your SecureAgentry account") {
		t.Fatalf("signed out: %v", err)
	}
	if pol, _, lerr := policy.Load(path); lerr == nil && pol.Model.Provider == policy.ProviderAgentry {
		t.Fatal("a signed-out choice wrote provider agentry")
	}

	if _, err := settings.StoreToken("https://secureagentry.net", "agy_fakesigninforthedesktoptestonly00000000000000000"); err != nil {
		t.Fatal(err)
	}
	if err := ApplySetup(context.Background(), SetupOpts{Provider: policy.ProviderAgentry, Model: "gpt-x"}, io.Discard); err == nil {
		t.Fatal("a model was taken with Agentry credits")
	}
	if err := ApplySetup(context.Background(), SetupOpts{Provider: policy.ProviderAgentry}, io.Discard); err != nil {
		t.Fatal(err)
	}
	pol, _, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	m := pol.Model
	if m.Provider != policy.ProviderAgentry || m.BaseURL != "https://secureagentry.net"+policy.AgentryInferencePath || m.APIKeyEnv != "" ||
		len(m.ProxyAllow) != 1 || m.ProxyAllow[0] != "secureagentry.net:443" {
		t.Fatalf("model block %+v", m)
	}
	if CreditsClient(pol) == nil {
		t.Fatal("no credits client for a signed-in credits policy")
	}
	if CreditsClient(policy.Default()) != nil {
		t.Fatal("a credits client for a key-paid policy")
	}
}
