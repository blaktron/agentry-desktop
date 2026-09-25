package run

// Setup-surface tests (§P1 desktop shape): GetStatus must be a pure read,
// ApplySetup must write exactly the provider-table model block into the
// operator policy and only touch docker when asked to.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/setup"
)

// TestGetStatusReadOnlyDefaults pins req 5's status read: no policy file
// means defaults are REPORTED, and looking must not create the file.
func TestGetStatusReadOnlyDefaults(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})

	st, err := GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.PolicyExists {
		t.Fatal("no policy file exists yet")
	}
	def := policy.Default()
	if st.Provider != def.Model.Provider || st.Model != def.Model.Name || st.APIKeyEnv != def.Model.APIKeyEnv {
		t.Errorf("defaults not reported: %+v", st)
	}
	if st.TimeoutSeconds != def.Budgets.TimeoutSeconds || st.MaxTurns != def.Budgets.MaxTurns {
		t.Errorf("budget defaults not reported: %+v", st)
	}
	if !st.KeepProfilesByDefault {
		t.Error("shipped keep-profiles default is true (§P2)")
	}
	if st.DockerOK || st.DockerErr == "" {
		t.Errorf("docker failure must be surfaced: %+v", st)
	}
	if st.ImagesReady {
		t.Error("images cannot be ready without docker")
	}
	// The read must not have created the policy file.
	if _, statErr := os.Stat(st.PolicyPath); !os.IsNotExist(statErr) {
		t.Fatalf("GetStatus created %s", st.PolicyPath)
	}
}

// TestGetStatusImagesReady covers the docker-up half: images present ⇒
// ready; any one missing ⇒ not ready.
func TestGetStatusImagesReady(t *testing.T) {
	isolateRunEnv(t)
	fake := readyImagesFake()
	withFakeDocker(t, fake)

	st, err := GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.DockerOK || st.DockerErr != "" || !st.ImagesReady {
		t.Fatalf("status = %+v, want docker ok + images ready", st)
	}

	delete(fake.ExistingImages, policy.Default().Images.Gateway)
	st, err = GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.DockerOK || st.ImagesReady {
		t.Fatalf("a missing image must report not-ready: %+v", st)
	}
}

// TestGetStatusBrokenPolicyIsAnError: a policy file that fails to load is
// a fact the settings page must show, not hide behind defaults.
func TestGetStatusBrokenPolicyIsAnError(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, readyImagesFake())
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := GetStatus(context.Background()); err == nil || !strings.Contains(err.Error(), "operator policy") {
		t.Fatalf("err = %v, want the broken policy surfaced", err)
	}
}

// TestApplySetupWritesProviderTable pins §P1: choosing a provider fills
// the model block from the table, PullImages=false touches no docker, and
// everything outside the model block survives.
func TestApplySetupWritesProviderTable(t *testing.T) {
	isolateRunEnv(t)
	fake := readyImagesFake()
	withFakeDocker(t, fake)

	// Pre-write a policy with a custom budget so the preserve rule is
	// observable.
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	pre := policy.Default()
	pre.Budgets.TimeoutSeconds = 123
	if err := policy.Save(path, pre); err != nil {
		t.Fatal(err)
	}

	var log strings.Builder
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "openai"}, &log); err != nil {
		t.Fatal(err)
	}
	pol, _, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if pol.Model.Provider != "openai" || pol.Model.Name != "gpt-5.2" || pol.Model.APIKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("model block = %+v, want the openai table row", pol.Model)
	}
	if len(pol.Model.ProxyAllow) != 1 || pol.Model.ProxyAllow[0] != "api.openai.com:443" {
		t.Fatalf("proxyAllow = %v", pol.Model.ProxyAllow)
	}
	if pol.Budgets.TimeoutSeconds != 123 {
		t.Fatalf("budgets outside the model block must survive: %+v", pol.Budgets)
	}
	if len(fake.Calls) != 0 {
		t.Fatalf("PullImages=false must not touch docker: %v", fake.Ops())
	}
	if !strings.Contains(log.String(), "policy written") {
		t.Errorf("log = %q", log.String())
	}

	// GetStatus now reflects the written preference.
	st, err := GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.PolicyExists || st.Provider != "openai" || st.Model != "gpt-5.2" {
		t.Fatalf("status after setup = %+v", st)
	}
}

// TestApplySetupPullImagesBuildsRunner covers the PullImages half: with
// the pinned images present and the runner image absent, setup
// materializes the embedded context and builds the runner tag.
func TestApplySetupPullImagesBuildsRunner(t *testing.T) {
	home := isolateRunEnv(t)
	pol := policy.Default()
	fake := &dockerx.Fake{ExistingImages: map[string]bool{
		pol.Images.Gateway:            true,
		pol.Images.HarnessDockerAgent: true,
		// runner tag absent → EnsureInstalled must build it
	}}
	withFakeDocker(t, fake)

	var log strings.Builder
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", PullImages: true}, &log); err != nil {
		t.Fatal(err)
	}
	wantOp := "Build:" + runnerImageTag(pol) + "<-" + filepath.Join(home, "image")
	found := false
	for _, op := range fake.Ops() {
		if op == wantOp {
			found = true
		}
	}
	if !found {
		t.Fatalf("runner image not built: %v", fake.Ops())
	}
	if _, err := os.Stat(filepath.Join(home, "image", "Dockerfile")); err != nil {
		t.Errorf("embedded build context not materialized: %v", err)
	}
	if !strings.Contains(log.String(), "setup complete") {
		t.Errorf("log = %q", log.String())
	}
}

// TestApplySetupCustomProviderNeedsBaseURL: the form's Yes=true flow turns
// the missing-BaseURL question into a refusal instead of a hung window.
func TestApplySetupCustomProviderNeedsBaseURL(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, readyImagesFake())
	err := ApplySetup(context.Background(), SetupOpts{Provider: "custom"}, &strings.Builder{})
	if err == nil {
		t.Fatal("custom provider without a BaseURL must refuse")
	}
}

// TestApplySetupPullImagesDockerDown: the policy still lands, but the
// image half surfaces the docker failure with its class wording.
func TestApplySetupPullImagesDockerDown(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", PullImages: true}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "gateway/docker environment") {
		t.Fatalf("err = %v, want the docker-environment wrap", err)
	}
	path, perr := policy.DefaultPath()
	if perr != nil {
		t.Fatal(perr)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("the policy half must have been written before the image half failed: %v", statErr)
	}
}

// TestApplySetupStoresTheKeyBeforeThePolicy (agentry-desktop#90): with no
// keychain to reach (AGENTRY_NO_KEYRING models a headless machine), a
// store is refused by name and the policy is left as it was — the person
// asked to store the key, so nothing falls back to the variable quietly.
// The variable-only path is unchanged, and the source is recorded when
// the caller names it for a key stored elsewhere.
func TestApplySetupStoresTheKeyBeforeThePolicy(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	t.Setenv("AGENTRY_NO_KEYRING", "1")
	var log strings.Builder
	err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", StoreKey: "sk-live-secret"}, &log)
	if err == nil || !strings.Contains(err.Error(), "AGENTRY_NO_KEYRING") {
		t.Fatalf("err = %v", err)
	}
	path, _ := policy.DefaultPath()
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("a refused store wrote the policy: %s", path)
	}
	if strings.Contains(log.String(), "sk-live") {
		t.Fatalf("the key reached the log: %q", log.String())
	}
	// The variable path, untouched.
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic"}, &log); err != nil {
		t.Fatal(err)
	}
	st, err := GetStatus(context.Background())
	if err != nil || st.APIKeySource != "" || len(st.KeysStored) != 0 || st.KeyStoreErr == "" {
		t.Fatalf("status = %+v err %v (the switch must be reported, not read as 'not stored')", st, err)
	}
	// A keyring source with nothing behind it is refused: under the switch
	// the keychain cannot even be asked, and that is the reason given.
	err = ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", APIKeySource: "keyring"}, &log)
	if err == nil || !strings.Contains(err.Error(), "AGENTRY_NO_KEYRING") {
		t.Fatalf("keyring source with no key: %v", err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "apiKeySource: keyring") {
		t.Fatalf("a refused source was recorded:\n%s", raw)
	}
	// Forget under the switch is refused by name too, and changes nothing.
	before, _ := os.ReadFile(path)
	err = ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", ForgetKey: true}, &log)
	if err == nil || !strings.Contains(err.Error(), "AGENTRY_NO_KEYRING") {
		t.Fatalf("forget: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("a refused forget changed the policy")
	}
	// The form's key-only call is refused by name the same way.
	err = StoreModelKey("anthropic", "sk-live-secret", &log)
	if err == nil || !strings.Contains(err.Error(), "AGENTRY_NO_KEYRING") || strings.Contains(log.String(), "sk-live") {
		t.Fatalf("StoreModelKey: %v log=%q", err, log.String())
	}
}

// TestHomeFollowsTheRunnerHome: the desktop's runner home is the CLI's
// (policy.Home), so the two cannot resolve differently (agentry-cli#149).
func TestHomeFollowsTheRunnerHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", dir)
	h, err := Home()
	if err != nil || h != dir {
		t.Fatalf("Home = %q %v", h, err)
	}
	p, _ := policy.DefaultPath()
	if p != filepath.Join(dir, "policy.yaml") {
		t.Fatalf("policy path %q is not under the runner home", p)
	}
}

// fakeKeychain stands in for the OS keychain: a map of provider → key,
// with Present, Lookup, Store and Delete over it.
func fakeKeychain(t *testing.T, entries map[string]string) map[string]string {
	t.Helper()
	oldStore, oldDelete, oldPresent, oldLookup := keychainStore, keychainDelete, keychainPresent, keychainLookup
	keychainStore = func(provider, key string) error { entries[provider] = key; return nil }
	keychainDelete = func(provider string) error { delete(entries, provider); return nil }
	keychainPresent = func(provider string) (bool, error) { _, ok := entries[provider]; return ok, nil }
	keychainLookup = func(provider string) (string, error) {
		if v, ok := entries[provider]; ok {
			return v, nil
		}
		return "", errors.New("the OS keychain has no entry provider:" + provider)
	}
	t.Cleanup(func() {
		keychainStore, keychainDelete, keychainPresent, keychainLookup = oldStore, oldDelete, oldPresent, oldLookup
	})
	return entries
}

// The credential's kind (agentry-cli#171) is read from the value at the
// one moment this process holds it: a stored subscription token records
// apiKeyKind subscription and the status carries the sentence; a stored
// API key records nothing; forgetting the key takes its kind with it;
// the variable path keeps what the policy has. The value is in no log.
func TestApplySetupRecordsTheCredentialKindFromTheStoredKey(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	entries := fakeKeychain(t, map[string]string{})
	var log strings.Builder
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", StoreKey: "sk-ant-oat01-secret"}, &log); err != nil {
		t.Fatal(err)
	}
	if entries["anthropic"] != "sk-ant-oat01-secret" {
		t.Fatalf("stored = %v", entries)
	}
	st, err := GetStatus(context.Background())
	if err != nil || st.APIKeySource != "keyring" || st.APIKeyKind != "subscription" || !strings.Contains(st.KeyNotice, "Claude Code only") || !slices.Contains(st.KeysStored, "anthropic") {
		t.Fatalf("status = %+v err %v", st, err)
	}
	if !strings.Contains(log.String(), "Claude Code only") || strings.Contains(log.String(), "sk-ant-oat01") {
		t.Fatalf("log = %q", log.String())
	}
	path, _ := policy.DefaultPath()
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "apiKeyKind: subscription") || strings.Contains(string(raw), "sk-ant-oat01") {
		t.Fatalf("policy:\n%s", raw)
	}
	// The variable path keeps the kind the policy has.
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", Model: "claude-opus-5"}, &log); err != nil {
		t.Fatal(err)
	}
	if st, _ = GetStatus(context.Background()); st.APIKeyKind != "subscription" {
		t.Fatalf("kept: %+v", st)
	}
	// Forgetting the key takes its kind with it.
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", ForgetKey: true}, &log); err != nil {
		t.Fatal(err)
	}
	if st, _ = GetStatus(context.Background()); st.APIKeyKind != "" || st.KeyNotice != "" || len(st.KeysStored) != 0 {
		t.Fatalf("forgotten: %+v", st)
	}
	// An API key records nothing.
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", StoreKey: "sk-ant-api03-secret"}, &log); err != nil {
		t.Fatal(err)
	}
	if st, _ = GetStatus(context.Background()); st.APIKeyKind != "" || st.KeyNotice != "" {
		t.Fatalf("api key: %+v", st)
	}
}

// ListModels asks with the typed key, else what a run would read — the
// saved policy's variable, else the keychain's entry — and returns names
// and the kind; the key is in no error (agentry-desktop#131).
func TestListModelsUsesTheTypedKeyElseTheKeychains(t *testing.T) {
	isolateRunEnv(t)
	fakeKeychain(t, map[string]string{"anthropic": "sk-ant-oat01-stored"})
	var asked []string
	old := listModels
	listModels = func(_ context.Context, provider, baseURL, key string) (*setup.Catalog, error) {
		asked = append(asked, provider+"|"+baseURL+"|"+key)
		if key == "bad-key" {
			return nil, errors.New("custom: list models: 401 Unauthorized: key bad-key rejected")
		}
		return &setup.Catalog{Provider: provider, Models: []setup.Model{{ID: "m1", DisplayName: "One"}, {ID: "m2"}}}, nil
	}
	t.Cleanup(func() { listModels = old })

	cat, err := ListModels(context.Background(), "custom", "https://llm.example.net/v1", "typed-key")
	if err != nil || len(cat.Models) != 2 || cat.Models[0].DisplayName != "One" || cat.Kind != "api" || cat.Notice != "" {
		t.Fatalf("typed: %+v %v", cat, err)
	}
	cat, err = ListModels(context.Background(), "anthropic", "", "")
	if err != nil || cat.Kind != "subscription" || !strings.Contains(cat.Notice, "Claude Code only") {
		t.Fatalf("keychain: %+v %v", cat, err)
	}
	if len(asked) != 2 || asked[0] != "custom|https://llm.example.net/v1|typed-key" || asked[1] != "anthropic||sk-ant-oat01-stored" {
		t.Fatalf("asked = %v", asked)
	}
	if _, err := ListModels(context.Background(), "openai", "", ""); err == nil || !strings.Contains(err.Error(), "no key typed") {
		t.Fatalf("nothing anywhere: %v", err)
	}
	// The variable path: nothing typed, the policy names a variable that
	// is set — the value a run would read, before the keychain.
	var log strings.Builder
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "openai", APIKeyEnv: "TEST_OPENAI_KEY_131"}, &log); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_OPENAI_KEY_131", "from-the-variable")
	if cat, err := ListModels(context.Background(), "openai", "", ""); err != nil || cat.Kind != "api" || asked[len(asked)-1] != "openai||from-the-variable" {
		t.Fatalf("variable: %+v %v asked=%v", cat, err, asked)
	}
	// With the variable unset, the keychain — and the policy's provider
	// must be the one asked about, or its variable is not consulted.
	t.Setenv("TEST_OPENAI_KEY_131", "")
	if _, err := ListModels(context.Background(), "openai", "", ""); err == nil || !strings.Contains(err.Error(), "variable is unset") {
		t.Fatalf("unset variable, nothing stored: %v", err)
	}
	// The fake echoes the key on purpose: the wrapper scrubs it from
	// anything it passes on, whatever the listing did.
	_, err = ListModels(context.Background(), "custom", "https://x/v1", "bad-key")
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "bad-key") || !strings.Contains(err.Error(), "<key>") {
		t.Fatalf("error: %v", err)
	}
}

// A base URL repoints the model call — the operator's key goes wherever it
// says, and the agent's only egress becomes that host. The page shows the
// field for the custom provider only; Go used to enforce nothing, so script
// in the webview could call ApplySetup("anthropic", …, "http://attacker")
// (security review 2026-09-20 §8.4, C14; D-030).
func TestApplySetupRefusesABaseURLForAKnownProvider(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, readyImagesFake())
	for _, provider := range []string{"anthropic", "openai", ""} {
		err := ApplySetup(context.Background(), SetupOpts{Provider: provider, BaseURL: "https://models.example"}, &strings.Builder{})
		if err == nil || !strings.Contains(err.Error(), "custom") {
			t.Fatalf("provider %q with a base URL: err = %v, want a refusal naming the custom provider", provider, err)
		}
	}
}

func TestApplySetupRefusesPlainHTTPOffLoopback(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, readyImagesFake())
	for _, u := range []string{"http://models.example", "http://10.0.0.5:8000/v1", "ftp://models.example", "models.example"} {
		err := ApplySetup(context.Background(), SetupOpts{Provider: "custom", BaseURL: u}, &strings.Builder{})
		if err == nil {
			t.Fatalf("base URL %q accepted", u)
		}
	}
	for _, u := range []string{"https://models.example/v1", "http://localhost:11434/v1", "http://127.0.0.1:8000"} {
		if err := ApplySetup(context.Background(), SetupOpts{Provider: "custom", Model: "m", BaseURL: u}, &strings.Builder{}); err != nil {
			t.Fatalf("base URL %q refused: %v", u, err)
		}
	}
}

// TestStoreModelKeyThenApplySetupForTheCustomProvider (agentry-desktop#139):
// the form saves in two calls — the typed key alone (StoreModelKey), then
// the rest of the form (ApplySetup). The key-only call must not need the
// model or the URL the second call carries: for the custom provider there
// is no table default to fill them, and the save used to die there with
// "no model chosen" before the policy was ever written. The kind of the
// key (agentry-cli#171) still lands, read from what the keychain holds at
// the moment the policy is written — even across a provider switch, where
// the policy has no kind to keep.
func TestStoreModelKeyThenApplySetupForTheCustomProvider(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	entries := fakeKeychain(t, map[string]string{})
	var log strings.Builder
	path, _ := policy.DefaultPath()

	// The source the second call records needs a key behind it (D-024):
	// with nothing stored for the provider, it is refused by name.
	err := ApplySetup(context.Background(), SetupOpts{
		Provider: "custom", Model: "qwen3.8-flash", BaseURL: "https://llm.example.net/v1", APIKeySource: "keyring",
	}, &log)
	if err == nil || !strings.Contains(err.Error(), "holds no key for custom") {
		t.Fatalf("keyring source with nothing stored: %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("a refused source wrote the policy: %s", path)
	}

	// The key, alone.
	if err := StoreModelKey("custom", "typed-secret", &log); err != nil {
		t.Fatalf("StoreModelKey(custom): %v", err)
	}
	if entries["custom"] != "typed-secret" {
		t.Fatalf("stored = %v", entries)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		// A policy that names a provider it cannot yet describe (no
		// model, no URL) would be a broken policy on disk if the second
		// call never came.
		t.Fatalf("the key-only call wrote the policy: %s", path)
	}

	// The rest of the form.
	err = ApplySetup(context.Background(), SetupOpts{
		Provider: "custom", Model: "qwen3.8-flash", BaseURL: "https://llm.example.net/v1", APIKeySource: "keyring",
	}, &log)
	if err != nil {
		t.Fatalf("ApplySetup(custom): %v", err)
	}
	st, err := GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Provider != "custom" || st.Model != "qwen3.8-flash" || st.BaseURL != "https://llm.example.net/v1" || st.APIKeySource != "keyring" || !slices.Contains(st.KeysStored, "custom") || st.APIKeyKind != "" || st.KeyNotice != "" {
		t.Fatalf("status = %+v", st)
	}
	raw, _ := os.ReadFile(path)
	for _, want := range []string{"provider: custom", "name: qwen3.8-flash", "baseUrl: https://llm.example.net/v1", "apiKeySource: keyring", "llm.example.net:443"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("policy lacks %q:\n%s", want, raw)
		}
	}

	// A subscription token stored in the key-only call keeps its kind
	// through the second call, on a switch to a provider the policy has
	// no kind for.
	if err := StoreModelKey("anthropic", "sk-ant-oat01-secret", &log); err != nil {
		t.Fatalf("StoreModelKey(anthropic): %v", err)
	}
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", Model: "claude-opus-5", APIKeySource: "keyring"}, &log); err != nil {
		t.Fatalf("ApplySetup(anthropic): %v", err)
	}
	st, err = GetStatus(context.Background())
	if err != nil || st.Provider != "anthropic" || st.Model != "claude-opus-5" || st.APIKeySource != "keyring" || st.APIKeyKind != "subscription" || !strings.Contains(st.KeyNotice, "Claude Code only") || !slices.Contains(st.KeysStored, "anthropic") {
		t.Fatalf("status = %+v err %v", st, err)
	}
	raw, _ = os.ReadFile(path)
	if !strings.Contains(string(raw), "apiKeyKind: subscription") {
		t.Fatalf("policy:\n%s", raw)
	}
	// The values reached the keychain and nothing else.
	for _, secret := range []string{"typed-secret", "sk-ant-oat01"} {
		if strings.Contains(log.String(), secret) || strings.Contains(string(raw), secret) {
			t.Fatalf("%q leaked: log=%q policy=%q", secret, log.String(), raw)
		}
	}
	if !strings.Contains(log.String(), "Claude Code only") {
		t.Fatalf("the subscription sentence was not said: %q", log.String())
	}
}

// TestGetStatusListsTheProvidersWithAStoredKey (agentry-desktop#141): the
// form can show any provider in the table, so the status says which have
// a key in the keychain — a stored key is a fact about the machine, not
// about the policy's provider; the key-only call's answer lists the
// provider it stored under while the policy still names another. A
// keychain that cannot be asked ends the asking and is said once.
func TestGetStatusListsTheProvidersWithAStoredKey(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	fakeKeychain(t, map[string]string{"custom": "typed-secret", "anthropic": "sk-ant-api03-x"})
	st, err := GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(st.KeysStored, ",") != "anthropic,custom" || st.KeyStoreErr != "" {
		t.Fatalf("status = %+v", st)
	}
	// The policy names openai: nothing stored for it, the two others still listed.
	if err := ApplySetup(context.Background(), SetupOpts{Provider: "openai"}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	st, err = GetStatus(context.Background())
	if err != nil || strings.Join(st.KeysStored, ",") != "anthropic,custom" {
		t.Fatalf("status = %+v err %v", st, err)
	}
	// A key stored in the key-only call is listed at once, the policy untouched.
	if err := StoreModelKey("google", "AIza-secret", &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	st, err = GetStatus(context.Background())
	if err != nil || st.Provider != "openai" || strings.Join(st.KeysStored, ",") != "anthropic,google,custom" {
		t.Fatalf("status = %+v err %v", st, err)
	}
	// The keychain fails on the third ask: what was listed before stays,
	// the asking ends there (no fourth or fifth prompt on a locked
	// keychain), and the reason is said.
	asked := 0
	keychainPresent = func(provider string) (bool, error) {
		asked++
		if asked == 3 {
			return false, errors.New("look for " + provider + " in the OS keychain: the collection is locked")
		}
		return provider == "anthropic", nil
	}
	st, err = GetStatus(context.Background())
	if err != nil || asked != 3 || strings.Join(st.KeysStored, ",") != "anthropic" || !strings.Contains(st.KeyStoreErr, "locked") {
		t.Fatalf("asked %d, status = %+v err %v", asked, st, err)
	}
	// No keychain at all: said once, nothing listed.
	keychainPresent = func(provider string) (bool, error) {
		return false, errors.New("keychain disabled (AGENTRY_NO_KEYRING) for " + provider)
	}
	st, err = GetStatus(context.Background())
	if err != nil || len(st.KeysStored) != 0 || !strings.Contains(st.KeyStoreErr, "AGENTRY_NO_KEYRING") {
		t.Fatalf("status = %+v err %v", st, err)
	}
}

// TestGetStatusReadsTheMarkerNotTheKey (agentry-desktop#268) proves
// agentry-cli#156 at the desktop's seam: the status read goes through the
// real modelkey.Present (the keychain seam's production value) over an
// in-memory keychain, and once setup has stored a key, the answer comes
// from the presence marker beside it, never from the key's own entry.
// The key's entry is removed behind the marker's back; a read of it would
// find nothing, say "not stored", and take the marker away (Confirm), so
// "stored", twice, is the proof no read of the key happened. A key an
// older client stored with no marker is read once, and the marker it
// leaves answers from then on.
func TestGetStatusReadsTheMarkerNotTheKey(t *testing.T) {
	isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	t.Setenv(modelkey.EnvNoKeyring, "")
	keyring.MockInit()
	t.Cleanup(keyring.MockInit) // leave an empty store, never a person's keychain
	if reflect.ValueOf(keychainPresent).Pointer() != reflect.ValueOf(modelkey.Present).Pointer() {
		t.Fatal("the status seam is not modelkey.Present: this test would prove nothing")
	}

	status := func() []string {
		t.Helper()
		st, err := GetStatus(context.Background())
		if err != nil || st.KeyStoreErr != "" {
			t.Fatalf("status = %+v err %v", st, err)
		}
		return st.KeysStored
	}

	// Setup stores the key the way the form does: key and marker.
	if err := StoreModelKey("anthropic", "sk-ant-api03-secret", &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if err := keyring.Delete(modelkey.Service, modelkey.Account("anthropic")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if got := status(); !slices.Equal(got, []string{"anthropic"}) {
			t.Fatalf("with only the marker left, stored = %v: the status read the key's entry", got)
		}
	}

	// An older client's key, no marker: read once, then the marker answers.
	if err := keyring.Set(modelkey.Service, modelkey.Account("openai"), "sk-legacy"); err != nil {
		t.Fatal(err)
	}
	if got := status(); !slices.Equal(got, []string{"openai", "anthropic"}) && !slices.Equal(got, []string{"anthropic", "openai"}) {
		t.Fatalf("a marker-less key must still be found, stored = %v", got)
	}
	if err := keyring.Delete(modelkey.Service, modelkey.Account("openai")); err != nil {
		t.Fatal(err)
	}
	if got := status(); !slices.Contains(got, "openai") {
		t.Fatalf("the first read left no marker for the legacy key, stored = %v", got)
	}

	// A marker with no key behind it misleads only the status: choosing
	// the keychain as the source reads the key itself and refuses.
	err := ApplySetup(context.Background(), SetupOpts{Provider: "anthropic", APIKeySource: modelkey.SourceKeyring}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), modelkey.Account("anthropic")) {
		t.Fatalf("a keyring source on a marker alone must refuse naming the missing entry, got %v", err)
	}
}
