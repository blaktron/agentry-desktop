package hostedrun

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// A draft In Agentry: the built-in Drafter run on a sandbox host against
// the platform's model, for credits, with every text it reads scanned by
// AI Review through the registry. It is a run of kind draft in the same
// runs extension, so it shares a run's one active row per account; it
// takes a description and hints rather than a release, and its one output
// is the sealed output.tar holding draft.json.

// The registry's bounds on a draft's words (x-runs.yaml CreateRun), held
// here so a draft over them is refused with the limit named before
// anything is sent.
const (
	MaxDraftDescriptionBytes = 24 << 10
	MaxDraftHints            = 10
	MaxDraftHintBytes        = 500
)

// DraftTerms is the status route's draft block: whether the deployment
// drafts, and one set of terms for every tier.
type DraftTerms struct {
	Enabled          bool  `json:"enabled"`
	Caps             Caps  `json:"caps"`
	HoldCredits      int64 `json:"holdCredits"`
	TokenCredits     int64 `json:"tokenCredits"`
	MaxSearches      int   `json:"maxSearches"`
	CreditsPerSearch int64 `json:"creditsPerSearch"`
	MaxPages         int   `json:"maxPages"`
	AIReviewCredits  int64 `json:"aiReviewCredits"`
	// Knowledge is whether a draft here reads knowledge packs, and how many
	// and how much in all; nil from a registry that does not send it.
	Knowledge *DraftKnowledge `json:"knowledge,omitempty"`
}

// DraftKnowledge is the status route's draft.knowledge: at most MaxPacks
// packs, MaxBytes in all, each counted with the scan record its sandbox
// host writes beside it.
type DraftKnowledge struct {
	Enabled  bool  `json:"enabled"`
	MaxPacks int   `json:"maxPacks"`
	MaxBytes int64 `json:"maxBytes"`
}

// Draft is a hosted draft's record (x-runs.yaml Draft): the terms it was
// held on, what the registry admitted and scanned for it, and its
// settlement's parts. SearchCredits and AIReviewCredits are nil until it
// settles.
type Draft struct {
	Hints            []string `json:"hints,omitempty"`
	TokenCredits     int64    `json:"tokenCredits"`
	MaxSearches      int      `json:"maxSearches"`
	CreditsPerSearch int64    `json:"creditsPerSearch"`
	MaxPages         int      `json:"maxPages"`
	Searches         int      `json:"searches"`
	Pages            int      `json:"pages"`
	RegistryReads    int      `json:"registryReads"`
	Descriptions     int      `json:"descriptions"`
	Searched         int      `json:"searched"`
	Scanned          int      `json:"scanned"`
	Withheld         int      `json:"withheld"`
	SearchCredits    *int64   `json:"searchCredits,omitempty"`
	AIReviewCredits  *int64   `json:"aiReviewCredits,omitempty"`
	// Revises is the release a revision drafts the next version of, as the
	// registry resolved it: its ref and archive digest. Nil for a draft from a
	// description.
	Revises *Revises `json:"revises,omitempty"`
	// Instructions and Problems are the person's two texts of a revision.
	Instructions string `json:"instructions,omitempty"`
	Problems     string `json:"problems,omitempty"`
}

// Revises is a revision's release (x-runs.yaml DraftRevises).
type Revises struct {
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
}

// DraftFacts are what the app knows about a registry before it offers a
// draft In Agentry.
type DraftFacts struct {
	// SignedIn is true when this registry has a stored credential.
	SignedIn bool
	// Runs is true when /meta lists the `runs` extension.
	Runs bool
	// Scopes are the token's scopes; ScopesKnown is false when they could
	// not be read.
	Scopes      []string
	ScopesKnown bool
	// Terms is the status route's draft block, nil when the route did not
	// carry one; StatusKnown is false when the route did not answer.
	Terms       *DraftTerms
	StatusKnown bool
	// Hold is the caller's draft line, nil when the route did not carry it.
	Hold *TierPrice
	// Active is the account's run, build or draft in flight, nil for none.
	Active *Run
	// Revise is the status route's caller.revise (Edit with AI), nil when
	// the route did not carry it.
	Revise *CallerRevise
}

// DecideDraft applies the rule. Signed out, the choice is offered but
// cannot be chosen, since a draft In Agentry spends credits and needs an
// account; a registry without the runs extension offers it not at all.
// Otherwise it says in a sentence why it cannot be chosen when it cannot,
// in a run's words wherever the reason is the same.
func DecideDraft(f DraftFacts) Offer {
	if !f.SignedIn {
		return Offer{Offered: true, Reason: "Drafting In Agentry spends credits, so it needs an account.", Remedy: "Sign in from Settings."}
	}
	if !f.Runs {
		return Offer{}
	}
	o := Offer{Offered: true}
	switch {
	case !f.ScopesKnown:
		o.Reason = "This registry did not say what the stored token may do, so the app cannot tell whether it may start a draft."
		o.Remedy = "Sign in again from Settings."
	case !has(f.Scopes, ScopeRunWrite) || !has(f.Scopes, ScopeRunRead):
		o.Reason = "The token stored for this registry does not carry " + ScopeRunRead + " and " + ScopeRunWrite +
			", which a draft In Agentry needs. It was minted before this app asked for them."
		o.Remedy = "Sign in again from Settings to mint a token that carries them."
	case !f.StatusKnown:
		o.Reason = "The registry did not answer whether it is taking drafts."
		o.Remedy = "Try again, or draft it on this computer."
	case f.Terms == nil || !f.Terms.Enabled || f.Hold == nil:
		o.Reason = "This registry is not taking drafts right now."
		o.Remedy = "Draft it on this computer, or write it yourself step by step."
	case f.Active != nil:
		o.Reason = "This account already has " + ActiveWhat(f.Active) + " In Agentry in flight. Agentry runs one at a time."
		o.Remedy = "Wait for it to finish, or cancel it."
	default:
		o.Available = true
	}
	return o
}

// ActiveWhat names what an account has in flight: a run, a build or a
// draft.
func ActiveWhat(r *Run) string {
	switch {
	case r.IsBuild():
		return "a build"
	case r.IsDraft():
		return "a draft"
	case r.IsVoiceBuild():
		return "a Voice Writer run"
	}
	return "a run"
}

// DraftHoldSentence is the site's sentence for a draft's hold, protected copy
// there, from the registry's numbers alone: AI Review's credit and the
// searches are inside the hold, never on top of it.
func DraftHoldSentence(holdCredits int64, t DraftTerms) string {
	if holdCredits == 0 {
		return "This draft holds no credits: the organization that pays for your drafts is not metered."
	}
	return fmt.Sprintf("This draft holds %d credits and settles from what it uses: its model's work, up to %d web searches at %s each, and %s for AI Review, never more than it holds.",
		holdCredits, t.MaxSearches, creditCount(t.CreditsPerSearch), creditCount(t.AIReviewCredits))
}

// DraftInputsSentence is the site's honest line beside Draft it: where the
// description goes, and what AI Review does and does not do.
const DraftInputsSentence = "Your description and the Drafter's searches go to a search provider, to AI Review (TypeSafe's Jev) and to the sites the Drafter reads, so the description should hold no secret. AI Review reads every result before the Drafter's model does, and holds back what it flags."

// DraftSettledParts names a settled draft's searches' and AI Review's
// parts as the record wrote them, the site's words; a part the record did
// not write is not guessed.
func DraftSettledParts(d *Draft) []string {
	if d == nil {
		return nil
	}
	var out []string
	if d.SearchCredits != nil {
		searches := fmt.Sprintf("%d searches", d.Searched)
		if d.Searched == 1 {
			searches = "1 search"
		}
		out = append(out, fmt.Sprintf("%d for %s", *d.SearchCredits, searches))
	}
	if d.AIReviewCredits != nil {
		out = append(out, fmt.Sprintf("%d for AI Review", *d.AIReviewCredits))
	}
	return out
}

func creditCount(n int64) string {
	if n == 1 {
		return "1 credit"
	}
	return fmt.Sprintf("%d credits", n)
}

// CheckDraft holds a draft's words to the registry's bounds before
// anything is sent: the description is text of at most 24 KiB, and each
// hint one line of at most 500 bytes, at most ten of them, blank ones
// dropped. It returns them as they are sent.
func CheckDraft(description string, hints []string) (string, []string, error) {
	description = strings.TrimSpace(description)
	if description == "" {
		return "", nil, fmt.Errorf("describe what the QuickAgent should do: the description is empty")
	}
	if !utf8.ValidString(description) {
		return "", nil, fmt.Errorf("the description must be text (UTF-8)")
	}
	if len(description) > MaxDraftDescriptionBytes {
		return "", nil, fmt.Errorf("a description is at most %d KiB; this one is %d KiB", MaxDraftDescriptionBytes>>10, (len(description)+1023)>>10)
	}
	var kept []string
	for _, h := range hints {
		h = strings.Join(strings.Fields(h), " ")
		if h == "" {
			continue
		}
		if !utf8.ValidString(h) {
			return "", nil, fmt.Errorf("a hint must be text (UTF-8)")
		}
		if len(h) > MaxDraftHintBytes {
			return "", nil, fmt.Errorf("a hint is at most %d bytes, its label included; shorten it", MaxDraftHintBytes)
		}
		kept = append(kept, h)
	}
	if len(kept) > MaxDraftHints {
		return "", nil, fmt.Errorf("a draft takes at most %d hints", MaxDraftHints)
	}
	return description, kept, nil
}
