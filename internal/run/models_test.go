package run

// Settings → Models: the saved models and providers through the CLI's own
// rules, against a mock keychain, a stand-in models listing and stand-in
// checkers, so no test asks a real provider or spends a plan's message.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/runner/modelcheck"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/savedmodels"
	"github.com/blaktron/agentry-cli/pkg/runner/setup"
	"github.com/blaktron/agentry-cli/pkg/schedule"
)

// lab is a runner home with a mock keychain, where an endpoint of the
// person's own at lab.example lists qwen3 and llama4 and reports a 32768
// token window; asks counts the listings, and claudeAsks the Claude
// messages sent.
type lab struct {
	home       string
	path       string
	asks       int
	claudeAsks int
	down       bool
}

func newLab(t *testing.T) *lab {
	t.Helper()
	keyring.MockInit()
	l := &lab{home: isolateRunEnv(t)}
	l.path, _ = policy.DefaultPath()
	prevList, prevCheckers := listModelsToCheck, modelCheckers
	listModelsToCheck = func(_ context.Context, provider, base, key string) (*setup.Catalog, error) {
		l.asks++
		if l.down {
			return nil, errors.New("dial tcp: connection refused")
		}
		switch {
		case provider == "custom" && strings.HasPrefix(base, "http://lab.example"):
			return &setup.Catalog{Models: []setup.Model{{ID: "qwen3"}, {ID: "llama4"}}}, nil
		case provider == "openai" && key == "sk-test-openai":
			return &setup.Catalog{Models: []setup.Model{{ID: "gpt-5.5"}}}, nil
		}
		return nil, fmt.Errorf("%s refused the key", provider)
	}
	modelCheckers = func(home string, pol *policy.Policy, path string, logw io.Writer) modelcheck.Checkers {
		c := realCheckers(home, pol, path, logw)
		c.ProbeContextWindow = func(context.Context, string, string, string) (setup.ContextWindow, error) {
			return setup.ContextWindow{Tokens: 32768, Source: "lab.example"}, nil
		}
		c.Claude = func(context.Context, string) (modelcheck.ClaudeAnswer, error) {
			l.claudeAsks++
			return modelcheck.ClaudeAnswer{OK: true, Resolved: "claude-sonnet-5-5"}, nil
		}
		return c
	}
	t.Cleanup(func() { listModelsToCheck, modelCheckers = prevList, prevCheckers })
	return l
}

var labEndpoint = NewProvider{Vendor: "custom", Way: WayKey, Name: "Lab", BaseURL: "http://lab.example:8000/v1"}

func (l *lab) policy(t *testing.T) *policy.Policy {
	t.Helper()
	pol, _, err := policy.Load(l.path)
	if err != nil {
		t.Fatal(err)
	}
	return pol
}

func mustSave(t *testing.T) func(*SaveResult, error) *SavedModel {
	return func(res *SaveResult, err error) *SavedModel {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if res.Saved == nil {
			t.Fatalf("not saved: %+v %+v", res, res.Refusal)
		}
		return res.Saved
	}
}

// A computer whose policy holds only the defaults lists no model: nobody
// chose it. The first provider and model saved take its place, and the
// model is the default; the window the endpoint reports is filled in.
func TestTheFirstSavedModelTakesTheDefaultsPlaceAndIsTheDefault(t *testing.T) {
	l := newLab(t)
	if _, _, err := policy.EnsureDefault(l.path); err != nil {
		t.Fatal(err)
	}
	list, err := ListSavedModels()
	if err != nil || list.SetUp || len(list.Models) != 0 || len(list.Providers) != 0 || list.Max != 10 {
		t.Fatalf("a defaults policy: %+v (%v)", list, err)
	}

	sm := mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3"}, io.Discard))
	if !sm.Default || sm.Provider != "custom-lab" || sm.ContextWindow != 32768 {
		t.Fatalf("saved %+v", sm)
	}
	list, err = ListSavedModels()
	if err != nil || !list.SetUp || len(list.Models) != 1 || len(list.Providers) != 1 {
		t.Fatalf("after the first: %+v (%v)", list, err)
	}
	p := list.Providers[0]
	if p.ID != "custom-lab" || p.Name != "Lab" || !p.TakesNoKey || p.State != savedmodels.Ready || len(p.Models) != 1 {
		t.Fatalf("provider %+v", p)
	}
	if _, ok := l.policy(t).FindModel("anthropic"); ok {
		t.Fatal("the defaults' model nobody chose is still listed")
	}
}

// A second model on a provider asks for no key or sign-in again; the
// eleventh is refused before its provider is asked anything.
func TestASecondModelOnAProviderAsksForNothingAndTheEleventhIsRefused(t *testing.T) {
	l := newLab(t)
	first := mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3"}, io.Discard))
	second := mustSave(t)(AddModel(context.Background(), "custom-lab", ModelDraft{Name: "Llama", Model: "llama4"}, io.Discard))
	if second.Default || second.Provider != first.Provider {
		t.Fatalf("second %+v", second)
	}
	for i := 3; i <= 10; i++ {
		mustSave(t)(AddModel(context.Background(), "custom-lab", ModelDraft{Name: fmt.Sprintf("Qwen %d", i), Model: "qwen3"}, io.Discard))
	}
	asked := l.asks
	_, err := AddModel(context.Background(), "custom-lab", ModelDraft{Name: "Eleventh", Model: "qwen3"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "10 saved models") {
		t.Fatalf("the eleventh: %v", err)
	}
	if l.asks != asked {
		t.Fatal("the eleventh asked its provider before it was refused")
	}
	if _, err := AddProviderModel(context.Background(), NewProvider{Vendor: "custom", Name: "Other", BaseURL: "http://lab.example:9000/v1"}, ModelDraft{Name: "Other", Model: "qwen3"}, io.Discard); err == nil {
		t.Fatal("an eleventh model on a new provider was taken")
	}
}

// A save the check refuses writes nothing: an endpoint that cannot be
// reached is refused with the reason and may be retried; a model the
// provider does not offer is refused with what it does offer.
func TestASaveTheCheckRefusesWritesNothing(t *testing.T) {
	l := newLab(t)
	mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3"}, io.Discard))
	before, _ := os.ReadFile(l.path)

	res, err := AddModel(context.Background(), "custom-lab", ModelDraft{Name: "Gemma", Model: "gemma4"}, io.Discard)
	if err != nil || res.Saved != nil || res.Refusal == nil || res.Refusal.Kind != modelcheck.NotOffered || res.Refusal.Retry || len(res.Refusal.Offered) != 2 {
		t.Fatalf("an id not offered: %+v (%v)", res, err)
	}
	l.down = true
	res, err = AddModel(context.Background(), "custom-lab", ModelDraft{Name: "Llama", Model: "llama4"}, io.Discard)
	if err != nil || res.Refusal == nil || res.Refusal.Kind != modelcheck.CannotCheck || !res.Refusal.Retry || !strings.Contains(res.Refusal.Reason, "connection refused") {
		t.Fatalf("an endpoint down: %+v (%v)", res, err)
	}
	if after, _ := os.ReadFile(l.path); string(after) != string(before) {
		t.Fatal("a refused save changed the policy")
	}
	l.down = false
	mustSave(t)(AddModel(context.Background(), "custom-lab", ModelDraft{Name: "Llama", Model: "llama4"}, io.Discard))

	// A new provider refused by its check is not added either.
	l.down = true
	res, err = AddProviderModel(context.Background(), NewProvider{Vendor: "custom", Name: "Box", BaseURL: "http://lab.example:9000/v1"}, ModelDraft{Name: "Box", Model: "qwen3"}, io.Discard)
	if err != nil || res.Refusal == nil {
		t.Fatalf("a new provider down: %+v (%v)", res, err)
	}
	if providers, _, _ := l.policy(t).SavedModels(); hasProvider(providers, "custom-box") {
		t.Fatal("a refused provider was added")
	}
}

// For an endpoint of the person's own, the window typed wins over the one
// the endpoint reports.
func TestATypedContextWindowOverridesTheReportedOne(t *testing.T) {
	newLab(t)
	sm := mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3", ContextWindow: 65536}, io.Discard))
	if sm.ContextWindow != 65536 {
		t.Fatalf("window %d", sm.ContextWindow)
	}
	edited := mustSave(t)(EditModel(context.Background(), sm.ID, ModelDraft{ContextWindow: 16384}, io.Discard))
	if edited.ContextWindow != 16384 || edited.ID != sm.ID {
		t.Fatalf("edited %+v", edited)
	}
}

// A Claude subscription's check sends one message on the person's plan, so
// it is not sent until they agree: the first answer is the sentence to put
// to them, and nothing is asked or written.
func TestAClaudeSubscriptionsCheckWaitsForTheAgreement(t *testing.T) {
	l := newLab(t)
	mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3"}, io.Discard))
	pol := l.policy(t)
	if err := pol.AddProvider("anthropic", policy.Provider{APIKeyKind: policy.KeyKindClaudeSignIn, ProxyAllow: policy.SignInProxyAllow(policy.KeyKindClaudeSignIn)}); err != nil {
		t.Fatal(err)
	}
	if err := policy.Save(l.path, pol); err != nil {
		t.Fatal(err)
	}

	res, err := AddModel(context.Background(), "anthropic", ModelDraft{Name: "Sonnet", Model: "sonnet"}, io.Discard)
	if err != nil || res.NeedsAgreement != modelcheck.ClaudeNotice || res.Saved != nil || l.claudeAsks != 0 {
		t.Fatalf("before the agreement: %+v (%v), %d messages", res, err, l.claudeAsks)
	}
	sm := mustSave(t)(AddModel(context.Background(), "anthropic", ModelDraft{Name: "Sonnet", Model: "sonnet", AgreedToMessage: true}, io.Discard))
	if l.claudeAsks != 1 || sm.Model != "sonnet" {
		t.Fatalf("after the agreement: %+v, %d messages", sm, l.claudeAsks)
	}
}

// Removing a saved model a schedule names lists the schedule and changes
// nothing until the person names where it goes; then the schedule names
// that model. Nothing moves to the default on its own.
func TestRemovingAModelAScheduleNamesMovesItOnlyWhereThePersonSays(t *testing.T) {
	l := newLab(t)
	qwen := mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3"}, io.Discard))
	llama := mustSave(t)(AddModel(context.Background(), "custom-lab", ModelDraft{Name: "Llama", Model: "llama4"}, io.Discard))
	gemma := mustSave(t)(AddModel(context.Background(), "custom-lab", ModelDraft{Name: "Qwen again", Model: "qwen3"}, io.Discard))
	sc := &schedule.Schedule{Name: "Morning brief", Local: l.home, Run: schedule.RunSpec{OutDir: l.home, Model: llama.ID}, Created: time.Now()}
	if err := schedule.StoreIn(l.home).Create(sc, nil); err != nil {
		t.Fatal(err)
	}
	list, _ := ListSavedModels()
	for _, m := range list.Models {
		if m.ID == llama.ID && (len(m.Users) != 1 || m.Users[0].ID != sc.ID) {
			t.Fatalf("Llama's users %+v", m.Users)
		}
	}

	out, err := RemoveSavedModel(context.Background(), llama.ID, "")
	if err != nil || len(out.InUse) != 1 || out.InUse[0].Name != "Morning brief" {
		t.Fatalf("refusal %+v (%v)", out, err)
	}
	if _, ok := l.policy(t).FindModel(llama.ID); !ok {
		t.Fatal("the refused removal removed the model")
	}
	out, err = RemoveSavedModel(context.Background(), llama.ID, gemma.ID)
	if err != nil || len(out.InUse) != 0 || len(out.Moved) != 1 || out.MovedTo != gemma.ID {
		t.Fatalf("removal %+v (%v)", out, err)
	}
	if got, _ := schedule.StoreIn(l.home).Get(sc.ID); got.Run.Model != gemma.ID {
		t.Fatalf("the schedule names %q, want %q", got.Run.Model, gemma.ID)
	}
	if l.policy(t).DefaultModel().ID != qwen.ID {
		t.Fatal("the default moved")
	}

	// The default needs the new default named.
	out, err = RemoveSavedModel(context.Background(), qwen.ID, "")
	if err == nil || !strings.Contains(err.Error(), "is the default") {
		t.Fatalf("the default with none named: %+v (%v)", out, err)
	}
	if _, err := RemoveSavedModel(context.Background(), qwen.ID, gemma.ID); err != nil {
		t.Fatal(err)
	}
	if l.policy(t).DefaultModel().ID != gemma.ID {
		t.Fatal("the new default is not the one named")
	}
}

// Removing a provider removes its saved models and forgets its key; a
// provider that holds every saved model is refused.
func TestRemovingAProviderRemovesItsModelsAndForgetsItsKey(t *testing.T) {
	newLab(t)
	mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3"}, io.Discard))
	box := NewProvider{Vendor: "custom", Name: "Box", BaseURL: "http://lab.example:9000/v1", TakesKey: true}
	if _, err := AddProviderModel(context.Background(), box, ModelDraft{Name: "Box", Model: "qwen3"}, io.Discard); err == nil || !strings.Contains(err.Error(), "holds no key") {
		t.Fatalf("a keyed endpoint with no key kept: %v", err)
	}
	if id, err := StoreProviderKey(box, "box-secret", io.Discard); err != nil || id != "custom-box" {
		t.Fatalf("store %q (%v)", id, err)
	}
	mustSave(t)(AddProviderModel(context.Background(), box, ModelDraft{Name: "Box", Model: "qwen3"}, io.Discard))
	list, _ := ListSavedModels()
	for _, p := range list.Providers {
		if p.ID == "custom-box" && (p.State != savedmodels.Ready || p.TakesNoKey || p.KeySource != "keyring") {
			t.Fatalf("box %+v", p)
		}
	}

	if _, err := RemoveProvider(context.Background(), "custom-lab", "", false, io.Discard); err == nil {
		t.Fatal("removing the default's provider with no new default was taken")
	}
	out, err := RemoveProvider(context.Background(), "custom-box", "", false, io.Discard)
	if err != nil || len(out.Removed) != 1 || !strings.Contains(out.Credential, "key removed") {
		t.Fatalf("removal %+v (%v)", out, err)
	}
	if ok, _ := keychainPresent("custom-box"); ok {
		t.Fatal("the key is still kept")
	}
	if _, err := RemoveProvider(context.Background(), "custom-lab", "", false, io.Discard); err == nil || !strings.Contains(err.Error(), "every saved model") {
		t.Fatalf("the last provider: %v", err)
	}
}

// A provider whose key is neither in its variable nor in the keychain reads
// key missing; keeping the key there is the fix, and the provider reads it
// from the keychain from then on. One provider per vendor.
func TestAProviderWithNoKeyReadsKeyMissingUntilItIsKept(t *testing.T) {
	l := newLab(t)
	t.Setenv("OPENAI_API_KEY", "")
	mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3"}, io.Discard))
	pol := l.policy(t)
	if err := pol.AddProvider("openai", policy.Provider{APIKeyEnv: "OPENAI_API_KEY", ProxyAllow: []string{"api.openai.com:443"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pol.AddModel(policy.SavedModel{Name: "GPT", Provider: "openai", Model: "gpt-5.5"}); err != nil {
		t.Fatal(err)
	}
	if err := policy.Save(l.path, pol); err != nil {
		t.Fatal(err)
	}
	state := func() string {
		list, _ := ListSavedModels()
		for _, p := range list.Providers {
			if p.ID == "openai" {
				return p.State
			}
		}
		return ""
	}
	if s := state(); s != savedmodels.KeyMissing {
		t.Fatalf("state %q", s)
	}
	res, err := CheckSavedModel(context.Background(), "GPT", false, io.Discard)
	if err != nil || res.Refusal == nil || res.Refusal.Kind != modelcheck.KeyMissing {
		t.Fatalf("check with no key: %+v (%v)", res, err)
	}
	if _, err := StoreProviderKey(NewProvider{Vendor: "openai"}, "sk-test-openai", io.Discard); err != nil {
		t.Fatal(err)
	}
	if s := state(); s != savedmodels.Ready {
		t.Fatalf("state after the key %q", s)
	}
	if res, err := CheckSavedModel(context.Background(), "GPT", false, io.Discard); err != nil || res.Saved == nil {
		t.Fatalf("check with the key: %+v (%v)", res, err)
	}
	if _, err := AddProviderModel(context.Background(), NewProvider{Vendor: "openai", Way: WayKey}, ModelDraft{Name: "Again", Model: "gpt-5.5"}, io.Discard); err == nil || !strings.Contains(err.Error(), "already has the provider openai") {
		t.Fatalf("a second openai provider: %v", err)
	}
}

// Make default moves the default, which every run, schedule and agent that
// names no model follows.
func TestMakeDefault(t *testing.T) {
	l := newLab(t)
	mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3"}, io.Discard))
	llama := mustSave(t)(AddModel(context.Background(), "custom-lab", ModelDraft{Name: "Llama", Model: "llama4"}, io.Discard))
	if err := SetDefaultModel(llama.ID); err != nil {
		t.Fatal(err)
	}
	if l.policy(t).DefaultModel().ID != llama.ID {
		t.Fatal("the default did not move")
	}
	if err := SetDefaultModel("nothing"); err == nil {
		t.Fatal("a missing model was made the default")
	}
}

// The models listing for Add a model is asked with the provider's own
// credential.
func TestAProvidersModelsAreListedWithItsOwnCredential(t *testing.T) {
	newLab(t)
	mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3"}, io.Discard))
	prev := listModels
	listModels = func(_ context.Context, provider, base, key string) (*setup.Catalog, error) {
		if provider != "custom" || base != labEndpoint.BaseURL || key != "" {
			return nil, fmt.Errorf("asked %s %s with a key: %v", provider, base, key != "")
		}
		return &setup.Catalog{Models: []setup.Model{{ID: "qwen3"}, {ID: "llama4"}}}, nil
	}
	t.Cleanup(func() { listModels = prev })
	got, err := ListProviderModels(context.Background(), "custom-lab")
	if err != nil || got.Error != "" || len(got.Models) != 2 {
		t.Fatalf("listed %+v (%v)", got, err)
	}
}

// An endpoint being added is listed with the key typed for it or none, never
// with a key this computer keeps for another provider.
func TestAnEndpointBeingAddedIsNeverSentAnotherProvidersKey(t *testing.T) {
	newLab(t)
	if err := keychainStore("custom", "kept-for-another-endpoint"); err != nil {
		t.Fatal(err)
	}
	var sent []string
	prev := listModels
	listModels = func(_ context.Context, _, _, key string) (*setup.Catalog, error) {
		sent = append(sent, key)
		return &setup.Catalog{Models: []setup.Model{{ID: "qwen3"}}}, nil
	}
	t.Cleanup(func() { listModels = prev })
	if got, err := ListEndpointModels(context.Background(), "http://new.example:8000/v1", ""); err != nil || len(got.Models) != 1 {
		t.Fatalf("listed %+v (%v)", got, err)
	}
	if _, err := ListEndpointModels(context.Background(), "http://new.example:8000/v1", "typed"); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 || sent[0] != "" || sent[1] != "typed" {
		t.Fatalf("keys sent %q", sent)
	}
	if _, err := ListEndpointModels(context.Background(), "ftp://new.example/v1", ""); err == nil {
		t.Fatal("a scheme other than http or https was asked")
	}
}
