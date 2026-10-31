package run

// The status says whether a run on this computer has a way to pay for its
// model, by the same rule the CLI's preflight applies, and what is
// missing when it has none, in a sentence that never carries a value.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/modelsignin"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/setup"
)

func modelStatus(t *testing.T) *Status {
	t.Helper()
	st, err := GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func withSignInRecord(t *testing.T, rec *modelsignin.Record, err error) {
	t.Helper()
	old := signInRecord
	signInRecord = func(string) (*modelsignin.Record, error) { return rec, err }
	t.Cleanup(func() { signInRecord = old })
}

// The default policy pays with a key in a variable: unset, the gap names
// the variable and the provider; set, the model is ready.
func TestModelReadinessFollowsTheKeyVariable(t *testing.T) {
	jevKeychain(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	def := policy.Default().Model
	t.Setenv(def.APIKeyEnv, "")
	st := modelStatus(t)
	if st.ModelReady {
		t.Fatalf("ready with no key anywhere: %+v", st)
	}
	for _, want := range []string{"$" + def.APIKeyEnv, def.Provider} {
		if !strings.Contains(st.ModelGap, want) {
			t.Fatalf("gap %q does not name %q", st.ModelGap, want)
		}
	}
	// The engine's state is reported beside it, not instead of it.
	if st.DockerOK {
		t.Fatal("the fake engine refuses, and the status says it answers")
	}
	t.Setenv(def.APIKeyEnv, "sk-ant-api-from-the-shell")
	if st := modelStatus(t); !st.ModelReady || st.ModelGap != "" {
		t.Fatalf("a key in the variable is not a model ready: %+v", st)
	}
}

// With the keychain as the policy's source, a stored key is enough, and a
// key stored for another provider is not.
func TestModelReadinessReadsTheKeychainWhereThePolicySaysSo(t *testing.T) {
	jevKeychain(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	t.Setenv("OPENAI_API_KEY", "")
	var log strings.Builder
	if err := StoreModelKey("openai", "sk-openai-example", &log); err != nil {
		t.Fatal(err)
	}
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "openai", Model: "gpt-5", APIKeySource: "keyring"}, &log); err != nil {
		t.Fatal(err)
	}
	st := modelStatus(t)
	if !st.ModelReady {
		t.Fatalf("a key in the keychain is not a model ready: %+v", st)
	}
	if strings.Contains(st.ModelGap+st.Provider+st.Model, "sk-openai-example") {
		t.Fatal("the status carries the key")
	}
	// The key forgotten, the policy still names the keychain as the
	// source, and the gap says no key is saved.
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "openai", ForgetKey: true}, &log); err != nil {
		t.Fatal(err)
	}
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "openai", APIKeySource: "keyring"}, &log); err == nil {
		t.Fatal("the keychain source was recorded with nothing stored")
	}
	st = modelStatus(t)
	if st.ModelReady {
		t.Fatalf("ready after the key was forgotten: %+v", st)
	}
	if !strings.Contains(st.ModelGap, "$OPENAI_API_KEY") || !strings.Contains(st.ModelGap, "openai") {
		t.Fatalf("gap %q", st.ModelGap)
	}
}

// A model sign-in kind needs its sign-in kept and working.
func TestModelReadinessNeedsTheKeptSignIn(t *testing.T) {
	jevKeychain(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	pol := policy.Default()
	pol.Model.Provider = "openai"
	pol.Model.Name = "gpt-5"
	pol.Model.APIKeyKind = policy.KeyKindChatGPTPlan
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	withSignInRecord(t, nil, modelsignin.ErrNone)
	st := modelStatus(t)
	if st.ModelReady || !strings.Contains(st.ModelGap, "ChatGPT plan sign-in") || !strings.Contains(st.ModelGap, "none is kept") {
		t.Fatalf("no sign-in kept: ready %v gap %q", st.ModelReady, st.ModelGap)
	}
	withSignInRecord(t, &modelsignin.Record{Vendor: modelsignin.ChatGPT, Broken: "invalid_grant"}, nil)
	st = modelStatus(t)
	if st.ModelReady || !strings.Contains(st.ModelGap, "no longer works") || !strings.Contains(st.ModelGap, "invalid_grant") {
		t.Fatalf("broken sign-in: ready %v gap %q", st.ModelReady, st.ModelGap)
	}
	withSignInRecord(t, nil, errors.New("the store is unreadable"))
	if st = modelStatus(t); st.ModelReady || !strings.Contains(st.ModelGap, "could not be read") {
		t.Fatalf("unreadable store: ready %v gap %q", st.ModelReady, st.ModelGap)
	}
	withSignInRecord(t, &modelsignin.Record{Vendor: modelsignin.ChatGPT, Label: "someone@example.com"}, nil)
	if st = modelStatus(t); !st.ModelReady || st.ModelGap != "" {
		t.Fatalf("a kept sign-in is not a model ready: %+v", st)
	}
}

// Agentry credits need this computer signed in to the registry the model
// endpoint is under; no sign-in there is the gap, with the registry named.
func TestModelReadinessNeedsTheRegistrySignInForCredits(t *testing.T) {
	jevKeychain(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	pol := policy.Default()
	pol.Model.Provider = policy.ProviderAgentry
	pol.Model.Name = setup.AgentryModelName
	pol.Model.APIKeyEnv = ""
	pol.Model.BaseURL = "https://registry.example.net" + policy.AgentryInferencePath
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	st := modelStatus(t)
	if st.ModelReady || !strings.Contains(st.ModelGap, "Agentry credits") || !strings.Contains(st.ModelGap, "https://registry.example.net") {
		t.Fatalf("signed out of the registry: ready %v gap %q", st.ModelReady, st.ModelGap)
	}
}
