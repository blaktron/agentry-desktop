package run

// Run defaults' timeout and max turns (plan settings-cards.md D2,
// agentry-desktop#347): the runner policy's budgets, written through the
// policy package the CLI reads, so `agentry run` runs with them too.

import (
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// readBudgets reads the policy file back the way the CLI does.
func readBudgets(t *testing.T) policy.Budgets {
	t.Helper()
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	pol, _, err := policy.Load(path)
	if err != nil {
		t.Fatalf("the CLI cannot load the policy the desktop wrote: %v", err)
	}
	return pol.Budgets
}

func TestSetBudgetsWritesThePolicyTheCLIReads(t *testing.T) {
	isolateRunEnv(t)
	// No policy yet: the write creates it from the defaults, as the
	// provider form's does, and changes only the budgets.
	if err := SetBudgets(1800, 0); err != nil {
		t.Fatal(err)
	}
	if got := readBudgets(t); got.TimeoutSeconds != 1800 || got.MaxTurns != 0 {
		t.Fatalf("budgets read back as %+v, want 1800 s and 0 (unlimited) turns", got)
	}
	path, _ := policy.DefaultPath()
	pol, _, _ := policy.Load(path)
	if def := policy.Default(); pol.Model.Provider != def.Model.Provider || pol.Model.Name != def.Model.Name {
		t.Fatalf("the budgets write moved the model: %s %s", pol.Model.Provider, pol.Model.Name)
	}

	// A second write keeps what else the policy holds.
	pol.Model.Name = "a-model-the-person-chose"
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	if err := SetBudgets(600, 40); err != nil {
		t.Fatal(err)
	}
	after, _, _ := policy.Load(path)
	if after.Budgets.TimeoutSeconds != 600 || after.Budgets.MaxTurns != 40 {
		t.Fatalf("budgets read back as %+v, want 600 s and 40 turns", after.Budgets)
	}
	if after.Model.Name != "a-model-the-person-chose" {
		t.Fatalf("the budgets write replaced the model with %q", after.Model.Name)
	}
}

func TestSetBudgetsRefusesWhatTheCLIWouldNotRunWith(t *testing.T) {
	isolateRunEnv(t)
	if err := SetBudgets(900, 60); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name             string
		timeout, turns   int
		wantInTheRefusal string
	}{
		{"no timeout", 0, 60, "timeout"},
		{"a negative timeout", -5, 60, "timeout"},
		{"negative turns", 900, -1, "turns"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := SetBudgets(c.timeout, c.turns)
			if err == nil || !strings.Contains(err.Error(), c.wantInTheRefusal) {
				t.Fatalf("SetBudgets(%d, %d) = %v, want a refusal naming the %s", c.timeout, c.turns, err, c.wantInTheRefusal)
			}
			if got := readBudgets(t); got.TimeoutSeconds != 900 || got.MaxTurns != 60 {
				t.Fatalf("a refused write changed the policy to %+v", got)
			}
		})
	}
}
