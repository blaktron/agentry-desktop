package hostedrun

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Edit with AI: a revision of a QuickAgent's current release, drafted In
// Agentry for a payer above Standard, or on this computer for anyone who may
// publish to it with their own keys.

// MaxRevisionTextBytes bounds each of the person's two texts, as the
// registry and the CLI (drafter.MaxRevisionTextBytes) hold them.
const MaxRevisionTextBytes = 4000

// CheckRevision holds the person's two texts, Additional instructions and
// Problems with this draft, to the registry's rule before anything is sent
// (the site's holds the same rule): line
// breaks made LF, each trimmed, at least one given, text, no control
// character but a line break or a tab, and at most MaxRevisionTextBytes
// each. It returns them as they are sent; the same rule holds them on this
// computer, so both ways take the same words.
func CheckRevision(instructions, problems string) (string, string, error) {
	instructions = strings.TrimSpace(strings.ReplaceAll(instructions, "\r\n", "\n"))
	problems = strings.TrimSpace(strings.ReplaceAll(problems, "\r\n", "\n"))
	if instructions == "" && problems == "" {
		return "", "", errors.New("say what should change, or what is wrong with this version: both texts are empty")
	}
	for _, t := range []struct{ name, text string }{{"Additional instructions", instructions}, {"Problems with this draft", problems}} {
		if !utf8.ValidString(t.text) {
			return "", "", fmt.Errorf("%s must be text (UTF-8)", t.name)
		}
		if len(t.text) > MaxRevisionTextBytes {
			return "", "", fmt.Errorf("%s is at most %d bytes; this is %d", t.name, MaxRevisionTextBytes, len(t.text))
		}
		for _, r := range t.text {
			if r != '\n' && r != '\t' && unicode.IsControl(r) {
				return "", "", fmt.Errorf("%s holds a control character; take it out", t.name)
			}
		}
	}
	return instructions, problems, nil
}

// DecideRevise is the rule for Edit with AI In Agentry, on a draft's facts
// (DecideDraft) and the status route's caller.revise: offered where a
// draft is, and available where a draft is and the payer may revise. A
// Standard payer is told it is Pro and above, with the way out: the same
// revision on this computer with their own keys, or a higher tier. A
// registry that sends no caller.revise offers no revision In Agentry.
func DecideRevise(f DraftFacts) Offer {
	o := DecideDraft(f)
	if !o.Offered || !f.SignedIn || !o.Available {
		return o
	}
	switch {
	case f.Revise == nil:
		return Offer{}
	case !f.Revise.Allowed:
		return Offer{Offered: true, Reason: "Edit with AI In Agentry is part of Pro and above.",
			Remedy: "Edit it with AI on this computer, with your own keys, or see Pro.", Upgrade: true}
	}
	return o
}
