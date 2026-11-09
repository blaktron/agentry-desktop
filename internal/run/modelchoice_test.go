package run

// The Model dropdowns: what each one offers and greys, and the saved model a
// run picked is the one it compiles on, or the run is refused by its name.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/savedmodels"
)

// labWithGPT is a lab with Qwen (the default, an endpoint that takes no key)
// and GPT on OpenAI, whose key is in neither its variable nor the keychain.
func labWithGPT(t *testing.T) *lab {
	t.Helper()
	l := newLab(t)
	t.Setenv("OPENAI_API_KEY", "")
	mustSave(t)(AddProviderModel(context.Background(), labEndpoint, ModelDraft{Name: "Qwen", Model: "qwen3"}, io.Discard))
	pol := l.policy(t)
	if err := pol.AddProvider("openai", policy.Provider{APIKeyEnv: "OPENAI_API_KEY", APIKeySource: "keyring", ProxyAllow: []string{"api.openai.com:443"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pol.AddModel(policy.SavedModel{Name: "GPT", Provider: "openai", Model: "gpt-5.5"}); err != nil {
		t.Fatal(err)
	}
	if err := policy.Save(l.path, pol); err != nil {
		t.Fatal(err)
	}
	return l
}

func choice(t *testing.T, c *ModelChoices, name string) ModelChoice {
	t.Helper()
	for _, m := range c.Models {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("no choice %s in %+v", name, c.Models)
	return ModelChoice{}
}

// A dropdown starts on the default and offers every saved model; one whose
// key is not here is greyed with the reason until the key is kept.
func TestADropdownOffersEverySavedModelAndGreysOneWhoseKeyIsNotHere(t *testing.T) {
	labWithGPT(t)
	c, err := ListModelChoices()
	if err != nil {
		t.Fatal(err)
	}
	if c.Default == nil || c.Default.Name != "Qwen" || len(c.Models) != 2 {
		t.Fatalf("choices %+v", c)
	}
	if q := choice(t, c, "Qwen"); !q.Default || q.Code != "" || q.Refusal != "" {
		t.Fatalf("Qwen %+v", q)
	}
	if g := choice(t, c, "GPT"); g.Code != savedmodels.KeyMissing || !strings.Contains(g.Refusal, "Key missing") {
		t.Fatalf("GPT %+v", g)
	}
	if _, err := StoreProviderKey(NewProvider{Vendor: "openai"}, "sk-test-openai", io.Discard); err != nil {
		t.Fatal(err)
	}
	c, _ = ListModelChoices()
	if g := choice(t, c, "GPT"); g.Code != "" {
		t.Fatalf("GPT with its key %+v", g)
	}
}

// A model a sign-in pays for is greyed while the policy sets a cost budget,
// which such a run cannot be measured against; a computer that keeps no
// saved model a person chose offers none.
func TestADropdownGreysASignInsModelUnderACostBudget(t *testing.T) {
	l := labWithGPT(t)
	pol := l.policy(t)
	if err := pol.AddProvider("anthropic", policy.Provider{APIKeyKind: policy.KeyKindClaudeSignIn, ProxyAllow: []string{"api.anthropic.com:443"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := pol.AddModel(policy.SavedModel{Name: "Plan", Provider: "anthropic", Model: "sonnet"}); err != nil {
		t.Fatal(err)
	}
	pol.Budgets.MaxCostUSD = 5
	if err := policy.Save(l.path, pol); err != nil {
		t.Fatal(err)
	}
	prev := signInKept
	signInKept = func(string, string) bool { return true }
	t.Cleanup(func() { signInKept = prev })
	c, err := ListModelChoices()
	if err != nil {
		t.Fatal(err)
	}
	if p := choice(t, c, "Plan"); p.Code != CodeCostBudget {
		t.Fatalf("Plan %+v", p)
	}
	signInKept = func(string, string) bool { return false }
	c, _ = ListModelChoices()
	if p := choice(t, c, "Plan"); p.Code != savedmodels.NeedsSignIn {
		t.Fatalf("Plan with no sign-in %+v", p)
	}

	isolateRunEnv(t)
	if c, err := ListModelChoices(); err != nil || c.Default != nil || len(c.Models) != 0 {
		t.Fatalf("a computer with no saved model: %+v (%v)", c, err)
	}
}

// A run that picks a saved model compiles from the policy as that model
// reads, and nothing of another provider's; one that picks a model whose key
// is not here, or one the computer lacks, is refused by its name before
// anything is laid down. Nothing falls back to the default.
func TestAPickedModelIsTheOneTheRunIsOnOrTheRunIsRefusedByName(t *testing.T) {
	l := labWithGPT(t)
	pol := l.policy(t)

	rm, err := chooseModel(l.home, pol, "")
	if err != nil || rm.Saved.Name != "Qwen" || rm.Policy.Model.Provider != "custom" {
		t.Fatalf("the default: %+v (%v)", rm, err)
	}
	_, err = chooseModel(l.home, pol, "GPT")
	var ce *engine.ClassifiedError
	if !errors.As(err, &ce) || ce.Code != engine.ExitPolicy || !strings.Contains(err.Error(), `the saved model "GPT" (provider openai)`) || !strings.Contains(err.Error(), "Model list") {
		t.Fatalf("a picked model with no key: %v", err)
	}
	_, err = chooseModel(l.home, pol, "Gone")
	if !errors.As(err, &ce) || ce.Code != engine.ExitPolicy || !strings.Contains(err.Error(), "Gone") {
		t.Fatalf("a model the computer lacks: %v", err)
	}
	if _, err := StoreProviderKey(NewProvider{Vendor: "openai"}, "sk-test-openai", io.Discard); err != nil {
		t.Fatal(err)
	}
	rm, err = chooseModel(l.home, l.policy(t), "GPT")
	if err != nil || rm.Saved.Name != "GPT" || rm.Policy.Model.Provider != "openai" || rm.Policy.Model.Name != "gpt-5.5" || rm.Adapter != "docker-agent" {
		t.Fatalf("GPT with its key: %+v (%v)", rm, err)
	}
	if rm.Policy.Model.BaseURL != "" || strings.Contains(strings.Join(rm.Policy.Model.ProxyAllow, ","), "lab.example") {
		t.Fatalf("the run on GPT carries another provider's endpoint: %+v", rm.Policy.Model)
	}
}

// What a persistent agent's setting says of its model: the default it
// follows, the one it names, or that the one it names is gone; an id is
// resolved from a name and one the computer lacks is refused.
func TestAnAgentsModelIsDescribedAndResolved(t *testing.T) {
	labWithGPT(t)
	d := DescribeModel("")
	if d == nil || !d.FollowsDefault || d.Name != "Qwen" || d.Code != "" {
		t.Fatalf("the default %+v", d)
	}
	id, err := ResolveModel("GPT")
	if err != nil || id == "" {
		t.Fatalf("resolve GPT: %q (%v)", id, err)
	}
	if d := DescribeModel(id); d == nil || d.FollowsDefault || d.Name != "GPT" || d.Code != savedmodels.KeyMissing {
		t.Fatalf("GPT %+v", d)
	}
	if d := DescribeModel("m-0000000000000000"); d == nil || d.Code != CodeMissing {
		t.Fatalf("a gone model %+v", d)
	}
	if _, err := ResolveModel("Gone"); err == nil {
		t.Fatal("a model the computer lacks was resolved")
	}
	if id, err := ResolveModel(""); err != nil || id != "" {
		t.Fatalf("the default: %q (%v)", id, err)
	}
}

// A plan compiled for one saved model is never reused for a run on another,
// even under the same policy file: the plan, and so the receipt, names the
// saved model the run is on.
func TestAPlanIsReusedOnlyForTheSavedModelItWasCompiledOn(t *testing.T) {
	home := isolateRunEnv(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	m, meta, installDir, workspaceDir := planFixture(t)
	pol := cataloguedPolicy()
	if _, err := pol.AddModel(policy.SavedModel{Name: "Haiku", Provider: "anthropic", Model: "claude-haiku-5"}); err != nil {
		t.Fatal(err)
	}
	view := func(name string) *policy.Policy {
		t.Helper()
		rm, err := chooseModel(home, pol, name)
		if err != nil {
			t.Fatal(err)
		}
		return rm.Policy
	}
	def := pol.DefaultModel()
	plan, _, err := ensurePlan(io.Discard, home, m, meta, installDir, workspaceDir, view(""), "pol-sha", "docker-agent", nil, "autonomous", nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Harness.SavedModel == nil || plan.Harness.SavedModel.ID != def.ID {
		t.Fatalf("the default's plan names %+v, want %s", plan.Harness.SavedModel, def.ID)
	}
	plan, _, err = ensurePlan(io.Discard, home, m, meta, installDir, workspaceDir, view("Haiku"), "pol-sha", "docker-agent", nil, "autonomous", nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Harness.SavedModel == nil || plan.Harness.SavedModel.Name != "Haiku" || !strings.HasSuffix(plan.Harness.Model, "claude-haiku-5") {
		t.Fatalf("the run on Haiku reused or compiled %+v (%s)", plan.Harness.SavedModel, plan.Harness.Model)
	}
}
