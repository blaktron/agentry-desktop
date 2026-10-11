package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

// Build and Edit take the intake types that ask for an input source
// (agentry-desktop#806, DEV-240): a repository or url question is checked,
// saved and loaded back with no refusal, takes no choices, and the core's
// V13 warns on a repository question whose release takes files but not
// code.
func TestSourceQuestionsAreAuthored(t *testing.T) {
	m := model()
	m.Intake = []IntakeRow{
		{ID: "codebase", Prompt: "The repository to audit", Type: "repository", Required: true},
		{ID: "report", Prompt: "A report to read", Type: "url"},
	}
	res := m.Check()
	if len(res.Problems) != 0 {
		t.Fatalf("problems: %v", res.Problems)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "V13") || strings.Contains(w, "inputs.kinds") {
			t.Fatalf("a release with no inputs.kinds takes any kind, so no V13 warning: %q", res.Warnings)
		}
	}
	dir := t.TempDir()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := manifest.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Intake; len(got) != 2 || got[0].Type != manifest.IntakeRepository || got[1].Type != manifest.IntakeURL {
		t.Fatalf("saved intake %+v", got)
	}
	back, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if back.Intake[0].Type != "repository" || back.Intake[1].Type != "url" || len(back.Check().Problems) != 0 {
		t.Fatalf("loaded back: %+v, problems %v", back.Intake, back.Check().Problems)
	}

	// Choices apply to a choice question alone.
	for _, typ := range []string{"repository", "url"} {
		c := model()
		c.Intake = []IntakeRow{{ID: "src", Prompt: "Where", Type: typ, Choices: []string{"a", "b"}}}
		if p := c.Check().Problems[string(SectionQuestions)]; !strings.Contains(p, "choices only apply to a choice question") {
			t.Errorf("%s with choices: %q", typ, p)
		}
	}

	// V13: inputs.kinds that take files but not code.
	w := model()
	w.Intake = []IntakeRow{{ID: "codebase", Prompt: "The repository", Type: "repository", Required: true}}
	w.InputKinds = []string{"documents"}
	res = w.Check()
	if len(res.Problems) != 0 || !strings.Contains(strings.Join(res.Warnings, "\n"), "inputs.kinds does not take code") {
		t.Fatalf("documents only: problems %v, warnings %q", res.Problems, res.Warnings)
	}
	w.InputKinds = []string{"documents", "code"}
	if res = w.Check(); strings.Contains(strings.Join(res.Warnings, "\n"), "inputs.kinds does not take code") {
		t.Fatalf("with code: warnings %q", res.Warnings)
	}
}

// Edit carries a release's inputs block unchanged (agentry-desktop#822):
// the form has no editor for it, and a save that dropped it would change
// what files the release takes and how its agent reads them.
func TestTheInputsBlockSurvivesLoadAndSave(t *testing.T) {
	dir := t.TempDir()
	m := model()
	m.InputKinds, m.InputView = []string{"code", "documents"}, "text"
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	back, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(back.InputKinds, ",") != "code,documents" || back.InputView != "text" {
		t.Fatalf("loaded inputs: %q %q", back.InputKinds, back.InputView)
	}
	if err := Save(dir, back); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	parsed, err := manifest.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Inputs == nil || strings.Join(kindStrings(parsed.Inputs.Kinds), ",") != "code,documents" || string(parsed.Inputs.View) != "text" {
		t.Fatalf("re-saved inputs: %+v", parsed.Inputs)
	}

	// A release without the block gains none.
	plain := t.TempDir()
	if err := Save(plain, model()); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(plain, "agent.yaml"))
	if strings.Contains(string(raw), "inputs:") {
		t.Fatalf("a release with no inputs block gained one:\n%s", raw)
	}
}

func kindStrings[T ~string](in []T) []string {
	out := make([]string, len(in))
	for i, k := range in {
		out[i] = string(k)
	}
	return out
}
