package draft

import (
	"strings"
	"unicode/utf8"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
)

// Describe again, as the site writes it: a new draft starts from what the person first asked,
// what they want changed now, and the draft as it stands in the form (its
// agent.yaml, then its AGENT.md), within the registry's 24 KiB for a
// description.
//
// It is written about the person, in the third person, and the draft is quoted
// as data in fences. AI Review scans the description before the Drafter starts,
// and an imperative addressed to the reading model scores as an injection and
// is withheld; this third-person framing does not. The labels are the site's,
// byte for byte, so either client reads back a description the other composed.

// MaxDescription is a description's bound on the registry, and so what
// Describe again composes to.
const MaxDescription = hostedrun.MaxDraftDescriptionBytes

// MaxFollowUp is Describe again's note on what should change.
const MaxFollowUp = 4000

const (
	againAsked  = "What the person first asked for:\n"
	againChange = "What the person wants changed now:\n"
	againYAML   = "\n\nThe current draft's agent.yaml:\n\n"
	againAgent  = "\n\nThe current draft's AGENT.md:\n\n"
	blankLine   = "\n\n"
	// The site's first format, still read back from drafts made with it.
	oldHead  = "Change this QuickAgent draft as follows:\n"
	oldAsked = "\n\nWhat it was first asked to do:\n"
	oldDraft = "\n\nThe draft as it stands, agent.yaml:\n\n"
)

// DescribeAgain is the description a new draft starts from. The note is
// never cut; what is cut to fit is what the person first asked, then the
// end of the draft, inside its fence, which is always closed.
func DescribeAgain(followUp, firstDescription, manifestYAML, instructions string) string {
	change := againChange + strings.TrimSpace(followUp)
	askedRoom := MaxDescription - len(change) - len(againAsked) - len(blankLine)
	first := ""
	if f := strings.TrimSpace(firstDescription); f != "" {
		first = strings.TrimRight(clipBytes(f, max(0, askedRoom)), " \t\r\n")
	}
	out := change
	if first != "" {
		out = againAsked + first + blankLine + change
	}
	yaml := fenced(manifestYAML, "yaml", MaxDescription-len(out)-len(againYAML))
	if yaml == "" {
		return out
	}
	out += againYAML + yaml
	agent := fenced(instructions, "markdown", MaxDescription-len(out)-len(againAgent))
	if agent == "" {
		return out
	}
	return out + againAgent + agent
}

// SplitDescribeAgain reads a description DescribeAgain (or the site) wrote
// back into the person's note and what they first asked; ok is false for
// a description the person wrote themselves. The draft it carried is left
// behind, so a chain of Describe again carries the first description, not
// itself.
func SplitDescribeAgain(text string) (followUp, description string, ok bool) {
	if strings.HasPrefix(text, againAsked) || strings.HasPrefix(text, againChange) {
		at := 0
		if !strings.HasPrefix(text, againChange) {
			i := strings.Index(text, blankLine+againChange)
			if i < 0 {
				return "", "", false
			}
			at = i + len(blankLine)
		}
		rest := text[at+len(againChange):]
		if cut := strings.Index(rest, againYAML); cut >= 0 {
			rest = rest[:cut]
		}
		if strings.HasPrefix(text, againAsked) {
			description = strings.TrimSpace(text[len(againAsked):at])
		}
		return strings.TrimSpace(rest), description, true
	}
	if !strings.HasPrefix(text, oldHead) {
		return "", "", false
	}
	head := text[len(oldHead):]
	if cut := strings.Index(head, oldDraft); cut >= 0 {
		head = head[:cut]
	}
	if at := strings.Index(head, oldAsked); at >= 0 {
		return strings.TrimSpace(head[:at]), strings.TrimSpace(head[at+len(oldAsked):]), true
	}
	return strings.TrimSpace(head), "", true
}

// FirstDescription is what the person asked a draft for in their own
// words: a Describe again's first description, or the description itself.
func FirstDescription(text string) string {
	if _, first, ok := SplitDescribeAgain(text); ok {
		return first
	}
	return strings.TrimSpace(text)
}

// fenced quotes body in a backtick fence longer than any run inside it,
// cut to fit room bytes with the fence still closed; "" when not even an
// empty fence fits.
func fenced(body, lang string, room int) string {
	run, n := 2, 0
	for _, c := range body {
		if c == '`' {
			n++
			run = max(run, n)
		} else {
			n = 0
		}
	}
	f := strings.Repeat("`", run+1)
	open, closing := f+lang+"\n", "\n"+f
	space := room - len(open) - len(closing)
	if space < 0 {
		return ""
	}
	return open + clipBytes(strings.TrimRight(body, " \t\r\n"), space) + closing
}

// clipBytes cuts s to at most n bytes of UTF-8, on a character.
func clipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
