package draft

import (
	"strings"

	"github.com/blaktron/agentry-cli/pkg/drafter"
)

// Line is AI Review's check of a finished draft as the summary's one line:
// its tone ("info", or "review" when the version is headed for review) and
// its text.
type Line struct {
	Tone string `json:"tone"`
	Text string `json:"text"`
}

// CheckLine is AI Review's check of the finished draft (aiReview.check)
// as the summary gives it, in the site's words, with no count: AI Review's counts stay out of sight. The
// details are in the form view's folded "How it was drafted". A pass is
// "flagged nothing", never a word about the draft itself. Every line says the
// check read the draft as the Drafter finished it, and that the pipeline
// decides at submit. A draft with no check has no line.
func CheckLine(c *drafter.Check) *Line {
	if c == nil {
		return nil
	}
	read := "AI Review read the draft as the Drafter finished it, as the pipeline will"
	scan := "The pipeline scans the version, with your changes, when you submit it."
	switch c.Word() {
	case drafter.CheckFlagged:
		var what []string
		if c.Rubric.Verdict == drafter.CheckFlagged {
			what = append(what, "something the release check looks for")
		}
		if c.Draft.Verdict == drafter.CheckFlagged {
			what = append(what, "that it may not do what you described")
		}
		return &Line{Tone: "review", Text: read + ", and flagged " + strings.Join(what, ", and ") +
			". Expect the pipeline to send the version to review when you submit it. The details are under How it was drafted, in the form."}
	case drafter.CheckNotScanned:
		return &Line{Tone: "info", Text: "AI Review could not read all of the draft as the Drafter finished it. " + scan}
	case drafter.CheckPass:
		return &Line{Tone: "info", Text: read + ", and flagged nothing. " + scan}
	}
	// Unanswered, or a check with no verdicts.
	return &Line{Tone: "info", Text: "AI Review could not check the draft as the Drafter finished it. " + scan}
}

// rubricQuestions are the release rubric's questions (the core's pkg/judge
// RubricID*), named for a person, as the site names them.
var rubricQuestions = map[string]string{
	"C1_injection":             "instructions that could take over the agent",
	"C2_exfiltration":          "sending data somewhere it should not go",
	"C3_undeclared_capability": "reaching for something the contract does not declare",
	"C4_user_deception":        "misleading the person who runs it",
	"C5_obfuscation":           "hidden or disguised content",
	"C6_destructive_actions":   "destructive actions",
}

// RubricQuestion names a rubric question the draft's check scored, or ""
// for an id that is not one of the rubric's.
func RubricQuestion(id string) string { return rubricQuestions[id] }
