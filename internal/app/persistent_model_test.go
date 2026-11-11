package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/savedmodels"

	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// labModels writes a policy whose saved models are Qwen, the default, and
// Llama, on an endpoint of the person's own that takes no key, and GPT on
// OpenAI, whose key is in neither its variable nor the keychain.
func labModels(t *testing.T) *policy.Policy {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", "")
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	pol, _, err := policy.EnsureDefault(path)
	if err != nil {
		t.Fatal(err)
	}
	id, pr, _, err := savedmodels.CustomProvider("Lab", "http://lab.example:8000/v1", false)
	if err != nil {
		t.Fatal(err)
	}
	if err := pol.AddProvider(id, pr); err != nil {
		t.Fatal(err)
	}
	if err := pol.AddProvider("openai", policy.Provider{APIKeyEnv: "OPENAI_API_KEY", APIKeySource: "keyring", ProxyAllow: []string{"api.openai.com:443"}}); err != nil {
		t.Fatal(err)
	}
	for _, sm := range []policy.SavedModel{{Name: "Qwen", Provider: id, Model: "qwen3"}, {Name: "Llama", Provider: id, Model: "llama4"}, {Name: "GPT", Provider: "openai", Model: "gpt-5.5"}} {
		if _, err := pol.AddModel(sm); err != nil {
			t.Fatal(err)
		}
	}
	q, _ := pol.FindModel("Qwen")
	pol.Default = q.ID
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	pol, _, err = policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return pol
}

// A persistent agent is put on a saved model by its name, with nothing new
// to agree to, and its view names the model; Agentry Helper, whose other
// configuration is the app's, takes a model too. A model the computer lacks
// is refused, and the default is put back with "".
func TestAnAgentIsPutOnASavedModelWithNothingToAgreeTo(t *testing.T) {
	rig := newPersistentRig(t)
	pol := labModels(t)
	llama, _ := pol.FindModel("Llama")

	v, err := rig.p.Configure("assistant", PersistentChange{Model: modelRef("Llama")})
	if err != nil {
		t.Fatal(err)
	}
	if len(rig.dialogs()) != 0 {
		t.Fatalf("a model change asked %q", rig.dialogs())
	}
	if v.Model == nil || v.Model.ID != llama.ID || v.Model.Name != "Llama" || v.Model.FollowsDefault {
		t.Fatalf("the view's model %+v", v.Model)
	}
	if c, _ := persistent.LoadConfig(rig.home, "assistant"); c.Model != llama.ID {
		t.Fatalf("the config names %q, want Llama's %q", c.Model, llama.ID)
	}
	if v, err := rig.p.Configure("main", PersistentChange{Model: modelRef("Llama")}); err != nil || v.Model == nil || v.Model.Name != "Llama" {
		t.Fatalf("Agentry Helper on Llama: %+v (%v)", v, err)
	}
	if _, err := rig.p.Configure("assistant", PersistentChange{Model: modelRef("Gone")}); err == nil || !strings.Contains(err.Error(), "Gone") {
		t.Fatalf("a model the computer lacks: %v", err)
	}
	v, err = rig.p.Configure("assistant", PersistentChange{Model: modelRef("")})
	if err != nil || v.Model == nil || !v.Model.FollowsDefault || v.Model.Name != "Qwen" {
		t.Fatalf("back on the default: %+v (%v)", v, err)
	}
}

// The app's supervisor refuses an agent's saved model whose key is not here
// by its name, as the CLI's does, before the engine meets it.
func TestTheSupervisorRefusesAnUnusableSavedModelByName(t *testing.T) {
	rig := newPersistentRig(t)
	pol := labModels(t)
	a, err := rig.p.agent("assistant")
	if err != nil {
		t.Fatal(err)
	}
	opts, err := rig.p.options(rig.home, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.ModelUsable == nil {
		t.Fatal("the supervisor checks no model")
	}
	gpt, err := localrun.ChooseRunModel(pol, "GPT", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := opts.ModelUsable(gpt); err == nil || !strings.Contains(err.Error(), `the saved model "GPT" (provider openai)`) || !strings.Contains(err.Error(), "Model setting") {
		t.Fatalf("GPT with no key: %v", err)
	}
	qwen, _ := localrun.ChooseRunModel(pol, "Qwen", "")
	if err := opts.ModelUsable(qwen); err != nil {
		t.Fatalf("Qwen takes no key: %v", err)
	}
}

func modelRef(s string) *string { return &s }

// Recent runs name the saved model a run was on: a run here from its
// result, a scheduled one from the receipt in its folder.
func TestRecentRunsNameTheSavedModel(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "result.json"), []byte(`{"status":"ok","harness":{"adapter":"docker-agent","model":"custom/qwen3","savedModel":{"id":"a1b2c3d4","name":"Qwen"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	e := fireEntry(schedules.Fire{RunID: "r1", Outcome: schedules.Ran, OutDir: dir})
	if e.Model != "Qwen" || entryView(e).Model != "Qwen" {
		t.Fatalf("a scheduled run's entry %+v", e)
	}
	if savedModelIn(t.TempDir()) != "" {
		t.Fatal("a folder with no receipt named a model")
	}
}

// The supervisor answers an agent's quickagent_run itself, so the switch
// that lets the Assistant run QuickAgents without asking reaches it in the
// Assistant's options, read at each run; Agentry Helper's have none.
func TestTheAssistantsSupervisorReadsTheRunsWithoutAskingSwitch(t *testing.T) {
	rig := newPersistentRig(t)
	options := func(name string) persistent.Options {
		t.Helper()
		a, err := rig.p.agent(name)
		if err != nil {
			t.Fatal(err)
		}
		opts, err := rig.p.options(rig.home, a, nil)
		if err != nil {
			t.Fatal(err)
		}
		return opts
	}
	if options(mainAgent).RunsWithoutAsking != nil {
		t.Fatal("Agentry Helper's supervisor reads the switch")
	}
	unasked := options(assistantAgent).RunsWithoutAsking
	if unasked == nil || unasked() {
		t.Fatal("the Assistant runs without asking by default")
	}
	if err := settings.SetAssistantRunsWithoutAsking(true); err != nil {
		t.Fatal(err)
	}
	if !unasked() {
		t.Fatal("the switch is on and the Assistant's supervisor still asks")
	}
	if err := settings.SetAssistantRunsWithoutAsking(false); err != nil {
		t.Fatal(err)
	}
	if unasked() {
		t.Fatal("the switch is off and the Assistant's supervisor does not ask")
	}
}
