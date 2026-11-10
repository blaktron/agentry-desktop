// The Model dropdowns: which saved model a run, a schedule or a persistent
// agent runs on. Each starts on the default and offers every saved model,
// with the ones that cannot be used for a run on this computer marked and
// the reason. A run that names a saved model runs on it or is refused by
// its name; nothing falls back to the default. The rules are the CLI's
// (localrun.ChooseRunModel, savedmodels.Usable), called in process.
package run

import (
	"errors"
	"fmt"
	"io"

	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/modelsignin"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/savedmodels"
)

// pickAnother is the fix, in a refusal, of running on another saved model.
const pickAnother = "pick another saved model in the Model list"

// ModelChoice is one saved model as a Model dropdown offers it.
type ModelChoice struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Default  bool   `json:"default"`
	// Code is why it cannot be used now: savedmodels' key-missing or
	// needs-sign-in, or cost-budget; empty when it can.
	Code string `json:"code,omitempty"`
	// Refusal is that reason in a sentence.
	Refusal string `json:"refusal,omitempty"`
}

// ModelChoices are a computer's saved models as its dropdowns offer them.
type ModelChoices struct {
	// Default is the default saved model, which an empty pick follows;
	// nil when the computer keeps none a person chose.
	Default *ModelChoice  `json:"default,omitempty"`
	Models  []ModelChoice `json:"models"`
}

// The codes of a saved model a run cannot use beyond savedmodels' own.
const (
	// CodeCostBudget: the policy's cost budget cannot apply to a model a
	// sign-in pays for, which reports tokens and not dollars.
	CodeCostBudget = "cost-budget"
	// CodeCannotTell: whether its credential is here could not be read
	// (the keychain did not answer, or the policy does not resolve it).
	CodeCannotTell = "cannot-tell"
	// CodeMissing: the saved model named is not on this computer.
	CodeMissing = "missing"
)

// The seams the tests replace: no keychain, no sign-in store.
var (
	keyKept    = modelkey.Present
	signInKept = func(home, kind string) bool {
		_, err := modelsignin.Open(home, connectionKeys()).Get(modelsignin.VendorOf(kind))
		return !errors.Is(err, modelsignin.ErrNone)
	}
)

// usableHere is savedmodels.Usable on this computer, with this app's
// keychain and its sign-in store; fix is the refusal's way out.
func usableHere(home string, rm localrun.RunModel, fix string) error {
	return savedmodels.Usable(home, rm, fix, savedmodels.Here{SignInKept: signInKept, KeyKept: keyKept})
}

// chooseModel is the saved model a run naming ref (an id or a name; empty
// for the default) runs on: the policy as that model reads it and the
// harness its credential takes. A ref the computer lacks is refused (exit
// 11), and so is a named model whose credential is not here, by its name,
// before anything is laid down for the run.
func chooseModel(home string, pol *policy.Policy, ref string) (localrun.RunModel, error) {
	rm, err := localrun.ChooseRunModel(pol, ref, "")
	if err != nil {
		return rm, err
	}
	if ref != "" {
		if err := usableHere(home, rm, pickAnother); err != nil {
			return rm, err
		}
		if err := costBudgetRefusal(rm); err != nil {
			return rm, engine.Classify(engine.ExitPolicy, err)
		}
	}
	return rm, nil
}

// costBudgetRefusal refuses a run on a sign-in's saved model while the
// policy holds a cost budget, which such a run cannot be measured against.
func costBudgetRefusal(rm localrun.RunModel) error {
	m := rm.Policy.Model
	kind := policy.KeyKind(rm.Policy.Harness[rm.Adapter], m)
	if rm.Policy.Budgets.MaxCostUSD > 0 && policy.IsModelSignIn(kind) {
		return fmt.Errorf("the saved model %q cannot be used while the policy's budgets.maxCostUsd is set: %s", choiceName(rm.Saved), policy.MaxCostSignInSentence)
	}
	return nil
}

func choiceName(sm policy.SavedModel) string {
	if sm.Name != "" {
		return sm.Name
	}
	return sm.ID
}

// refusalOf is why a run on rm cannot start on this computer now, as a
// dropdown says it: a code and a sentence; empty when it can.
func refusalOf(home string, pol *policy.Policy, rm localrun.RunModel) (code, sentence string) {
	if err := usableHere(home, rm, pickAnother); err != nil {
		if u, ok := savedmodels.IsUnusable(err); ok {
			switch u.Code {
			case savedmodels.KeyMissing:
				return u.Code, "Key missing: add its key in Settings → Models."
			case savedmodels.NeedsSignIn:
				return u.Code, "Needs sign-in: sign in again in Settings → Models."
			}
		}
		return CodeCannotTell, err.Error()
	}
	if rm.Policy.Model.Provider == policy.ProviderAgentry {
		providers, _, _ := pol.SavedModels()
		if st := savedmodels.ProviderState(home, rm.Saved.Provider, providers[rm.Saved.Provider], registryToken); st.Code != savedmodels.Ready {
			return st.Code, "Needs sign-in: Agentry credits are paid through your Agentry account, and this computer is signed out."
		}
	}
	if err := costBudgetRefusal(rm); err != nil {
		return CodeCostBudget, "A model a plan pays for reports tokens, not dollars, so it cannot run while the policy sets a cost budget (budgets.maxCostUsd)."
	}
	return "", ""
}

// ListModelChoices are this computer's saved models as a Model dropdown
// offers them, the default first named, each marked with why it cannot be
// used now. A computer whose policy holds only the defaults' model offers
// none: every run there is on that model.
func ListModelChoices() (*ModelChoices, error) {
	pol, _, chosen, err := readModelsPolicy()
	if err != nil {
		return nil, err
	}
	out := &ModelChoices{Models: []ModelChoice{}}
	if !chosen {
		return out, nil
	}
	home, err := Home()
	if err != nil {
		return nil, err
	}
	_, models, def := pol.SavedModels()
	for _, sm := range models {
		c := ModelChoice{ID: sm.ID, Name: sm.Name, Provider: sm.Provider, Model: sm.Model, Default: sm.ID == def}
		if rm, err := localrun.ChooseRunModel(pol, sm.ID, ""); err != nil {
			c.Code, c.Refusal = CodeCannotTell, err.Error()
		} else {
			c.Code, c.Refusal = refusalOf(home, pol, rm)
		}
		out.Models = append(out.Models, c)
		if c.Default {
			d := c
			out.Default = &d
		}
	}
	return out, nil
}

// ModelInUse is the saved model something runs on, as its setting shows
// it: one it names, or the default it follows.
type ModelInUse struct {
	// ID is the saved model it names; "" when it follows the default.
	ID string `json:"id"`
	// Name, Provider and Model are the saved model it runs on now.
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	// FollowsDefault says it names none and runs on the default.
	FollowsDefault bool `json:"followsDefault"`
	// Code and Refusal say why a turn or a run on it is refused now:
	// missing (the computer no longer keeps the saved model it names), or
	// a ModelChoice's code.
	Code    string `json:"code,omitempty"`
	Refusal string `json:"refusal,omitempty"`
}

// DescribeModel is the saved model something naming ref (an id; "" for the
// default) runs on, with why it would be refused now; nil on a computer that
// keeps no saved model a person chose.
func DescribeModel(ref string) *ModelInUse {
	pol, _, chosen, err := readModelsPolicy()
	if err != nil || !chosen {
		return nil
	}
	home, err := Home()
	if err != nil {
		return nil
	}
	out := &ModelInUse{ID: ref, FollowsDefault: ref == ""}
	rm, err := localrun.ChooseRunModel(pol, ref, "")
	if err != nil {
		out.Code, out.Refusal = CodeMissing, "The saved model it names is no longer on this computer: pick another."
		return out
	}
	out.Name, out.Provider, out.Model = rm.Saved.Name, rm.Saved.Provider, rm.Saved.Model
	out.Code, out.Refusal = refusalOf(home, pol, rm)
	return out
}

// ResolveModel is the id of the saved model ref names on this computer, by
// its id or its name; "" stays "" (the default). One it does not keep is
// refused by name.
func ResolveModel(ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	pol, _, _, err := readModelsPolicy()
	if err != nil {
		return "", err
	}
	sm, ok := pol.FindModel(ref)
	if !ok {
		return "", engine.Classify(engine.ExitPolicy, fmt.Errorf("no saved model %q on this computer: Settings → Models lists them", ref))
	}
	return sm.ID, nil
}

// AgentModelUsable is the check a persistent agent's segment makes before
// it starts, on this computer: the saved model's key or sign-in is here, or
// the turn is refused by the model's name, with the way out of moving the
// agent in its Model setting.
func AgentModelUsable(home string) func(localrun.RunModel) error {
	return func(rm localrun.RunModel) error {
		return usableHere(home, rm, "move the agent to another saved model in its Model setting")
	}
}

// ModelsSetUp reports whether a person chose pol's model: it keeps saved
// models of its own, or its model block is not the defaults'.
func ModelsSetUp(pol *policy.Policy) bool { return setUp(pol) }

// RenewedHere says whether the connected account vendor's token is renewed
// on this computer at each run's start, which a runhost could not do for
// its copy.
func RenewedHere(vendor string) bool {
	home, err := Home()
	if err != nil {
		return false
	}
	rec, err := modelsignin.Open(home, connectionKeys()).Get(vendor)
	return err == nil && rec.SecretIn != ""
}

// heldManaged is the managed settings held on this computer; a test
// replaces it.
var heldManaged = managed.Current

// presetModel is the saved model a run of the QuickAgent ref is on when an
// organization assigned it with a preset model (localrun.PresetModel): with
// none picked in the Model list, the saved model that is the preset's;
// picked, the person's, unless a locked preset fixes another. A locked
// preset model no saved model is refuses the run, a default one leaves it
// on the pick with a warning on w, and the managed model rows still hold
// whatever is chosen.
func presetModel(pol *policy.Policy, ref, picked string, w io.Writer) (string, error) {
	model, warning, err := localrun.PresetModel(pol, heldManaged(), ref, picked, "")
	var conflict *localrun.PresetModelConflict
	var missing *localrun.PresetModelMissing
	switch {
	case errors.As(err, &conflict):
		return "", engine.Classify(engine.ExitPolicy, fmt.Errorf(
			"the model picked for this run conflicts with %s's managed settings, which fix the model of %s at %s's %s (quickagents): pick that one in the Model list",
			conflict.Organization, conflict.Ref, conflict.Want.Provider, conflict.Want.Model))
	case errors.As(err, &missing):
		return "", engine.Classify(engine.ExitPolicy, fmt.Errorf("%s: save it in Settings → Models and keys first", missing.Error()))
	case err != nil:
		return "", err
	}
	if warning != "" && w != nil {
		_, _ = fmt.Fprintf(w, "warning: %s\n", warning)
	}
	return model, nil
}
