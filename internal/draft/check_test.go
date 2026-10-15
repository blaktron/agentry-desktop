package draft

import (
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/drafter"
)

// The summary gives AI Review's check one line in the site's words
// (lib/builder-summary.ts checkLine): no count, never
// "safe", and every line says the pipeline decides at submit.
func TestCheckLineIsTheSitesOneLine(t *testing.T) {
	pass := func() *drafter.Check {
		return &drafter.Check{Rubric: &drafter.CheckRubric{Verdict: "pass"}, Draft: &drafter.CheckDraft{Verdict: "pass"}}
	}
	scan := "The pipeline scans the version, with your changes, when you submit it."
	for name, tc := range map[string]struct {
		c    func() *drafter.Check
		tone string
		text string
	}{
		"pass": {pass, "info", "AI Review read the draft as the Drafter finished it, as the pipeline will, and flagged nothing. " + scan},
		"unanswered": {func() *drafter.Check { return &drafter.Check{Unanswered: "the registry did not answer"} }, "info",
			"AI Review could not check the draft as the Drafter finished it. " + scan},
		"no verdicts": {func() *drafter.Check { return &drafter.Check{Rubric: &drafter.CheckRubric{Verdict: "pass"}} }, "info",
			"AI Review could not check the draft as the Drafter finished it. " + scan},
		"not scanned": {func() *drafter.Check { c := pass(); c.Draft.Verdict = "not_scanned"; return c }, "info",
			"AI Review could not read all of the draft as the Drafter finished it. " + scan},
		"rubric flagged": {func() *drafter.Check { c := pass(); c.Rubric.Verdict = "flagged"; return c }, "review",
			"AI Review read the draft as the Drafter finished it, as the pipeline will, and flagged something the release check looks for. " +
				"Expect the pipeline to send the version to review when you submit it. The details are under How it was drafted, in the form."},
		"both flagged": {func() *drafter.Check { c := pass(); c.Rubric.Verdict, c.Draft.Verdict = "flagged", "flagged"; return c }, "review",
			"AI Review read the draft as the Drafter finished it, as the pipeline will, and flagged something the release check looks for, " +
				"and that it may not do what you described. Expect the pipeline to send the version to review when you submit it. " +
				"The details are under How it was drafted, in the form."},
	} {
		t.Run(name, func(t *testing.T) {
			l := CheckLine(tc.c())
			if l == nil || l.Tone != tc.tone || l.Text != tc.text {
				t.Fatalf("line = %+v\nwant %s: %s", l, tc.tone, tc.text)
			}
			if strings.Contains(strings.ToLower(l.Text), "safe") {
				t.Errorf("the line says safe: %s", l.Text)
			}
		})
	}
	if CheckLine(nil) != nil {
		t.Error("a draft with no check has a line")
	}
}

func TestRubricQuestionNamesOnlyTheRubricsOwnIDs(t *testing.T) {
	if q := RubricQuestion("C3_undeclared_capability"); q != "reaching for something the contract does not declare" {
		t.Errorf("C3 = %q", q)
	}
	for _, id := range []string{"__proto__", "constructor", "C7", ""} {
		if q := RubricQuestion(id); q != "" {
			t.Errorf("%q names %q", id, q)
		}
	}
}
