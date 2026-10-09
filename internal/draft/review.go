package draft

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/drafter"

	"github.com/blaktron/agentry-desktop/internal/build"
)

// What Review says beside a draft, as the site's import says it
// (agentry-web lib/draft-import.ts, plan builder-rework.md §7.5, §7.6).

// MaxProblems is how many of the rules a draft still fails Review lists;
// the rest are counted.
const MaxProblems = 50

// UnmentionedHosts is every host the form's draft reaches that the
// person's own words do not name: the host itself, or a domain it sits
// under of at least two labels ("api.github.com" is named by
// "github.com"); "*" is always marked. Review marks each one, because a
// host the description never asked for is the shape a page's injection
// would take (plan §7.6). words are the person's: what they first asked,
// their Describe again note and their hints, never the earlier draft a
// Describe again quoted.
func UnmentionedHosts(m *build.Model, words string) []string {
	var reached []string
	switch m.EgressMode {
	case "all":
		reached = append(reached, "*")
	case "hosts":
		reached = append(reached, m.EgressHosts...)
	}
	for _, s := range m.MCPServers {
		reached = append(reached, s.Hosts()...)
	}
	said := strings.ToLower(words)
	seen := map[string]bool{}
	out := []string{}
	for _, h := range reached {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		if h == "*" || !named(h, said) {
			out = append(out, h)
		}
	}
	return out
}

func named(host, said string) bool {
	labels := strings.Split(host, ".")
	for i := 0; i <= len(labels)-2; i++ {
		d := strings.Join(labels[i:], ".")
		at := regexp.MustCompile(`(^|[^a-z0-9.-])` + regexp.QuoteMeta(d) + `($|[^a-z0-9-])`)
		if at.MatchString(said) {
			return true
		}
	}
	return false
}

// PersonsWords are the words a draft's hosts must be named in: what the
// person first asked, their Describe again note when this draft came of
// one, and their hints.
func PersonsWords(description string, hints []string) string {
	words := []string{}
	if follow, first, ok := SplitDescribeAgain(description); ok {
		words = append(words, follow, first)
	} else {
		words = append(words, description)
	}
	return strings.Join(append(words, hints...), "\n")
}

// validation is the part of `agentry validate --json`'s report Review
// lists, the post-step's record of the rules the rendered package fails.
type validation struct {
	Issues []struct {
		Rule     string `json:"rule"`
		Path     string `json:"path"`
		Message  string `json:"message"`
		Severity string `json:"severity"`
	} `json:"issues"`
	Inventory []struct {
		Scanner  string `json:"scanner"`
		FilePath string `json:"filePath"`
		Title    string `json:"title"`
	} `json:"inventory"`
	Findings []struct {
		Scanner   string `json:"scanner"`
		RuleID    string `json:"ruleId"`
		Severity  string `json:"severity"`
		FilePath  string `json:"filePath"`
		LineStart int    `json:"lineStart"`
		Title     string `json:"title"`
	} `json:"findings"`
}

// Problems are the rules the draft still fails, as the post-step recorded
// them, in the site's words: the manifest rules' errors, the inventory's
// and the content scan's findings. At most MaxProblems are returned, with
// the count of the rest. A report this app cannot read is said so, not
// guessed at.
func Problems(d *drafter.Draft) (list []string, more int) {
	list = []string{}
	if d.ManifestError != "" {
		list = append(list, "The draft could not be checked: "+d.ManifestError)
	}
	if len(d.Validation) == 0 || string(d.Validation) == "null" {
		return list, 0
	}
	var v validation
	if err := json.Unmarshal(d.Validation, &v); err != nil {
		return append(list, "The draft's validation report could not be read, so its problems are not listed here; Validate checks the saved draft."), 0
	}
	clip := func(s string, n int) string { return clipBytes(s, n) }
	for _, is := range v.Issues {
		if is.Severity != "error" {
			continue
		}
		at := ""
		if is.Path != "" {
			at = " " + clip(is.Path, 200)
		}
		list = append(list, fmt.Sprintf("%s%s: %s", clip(is.Rule, 40), at, clip(is.Message, maxDraftValue)))
	}
	for _, f := range v.Inventory {
		list = append(list, fmt.Sprintf("%s %s: %s", clip(f.Scanner, 40), clip(f.FilePath, 400), clip(f.Title, maxDraftValue)))
	}
	for _, f := range v.Findings {
		line := ""
		if f.LineStart > 0 {
			line = fmt.Sprintf(" line %d", f.LineStart)
		}
		list = append(list, fmt.Sprintf("%s %s (%s) in %s%s: %s", clip(f.Scanner, 40), clip(f.RuleID, 80), clip(f.Severity, 20),
			clip(f.FilePath, 400), line, clip(f.Title, maxDraftValue)))
	}
	if len(list) > MaxProblems {
		return list[:MaxProblems], len(list) - MaxProblems
	}
	return list, 0
}
