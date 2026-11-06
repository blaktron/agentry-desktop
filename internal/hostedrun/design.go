package hostedrun

import (
	"strings"
)

// The Report Designer In Agentry. A Report Designer run is a run of kind
// design_build in the runs extension: Design (a whole report design from a
// brief, optionally starting from one of the account's designs, with
// reference images, a logo and web addresses) or Cover (a custom cover page
// for one of the account's designs, with images the cover may draw), each at
// one flat price on every tier. What it makes is saved to the account's
// report design library as a new design, never the default; a run that saved
// nothing is refunded in full.

// ScopeUserProfile is the scope a Report Designer run needs beside run:read
// and run:write: its design is read from and saved to the account's report
// design library.
const ScopeUserProfile = "user:profile"

// KindDesignBuild is a hosted Report Designer run.
const KindDesignBuild = "design_build"

// The Report Designer's two jobs (the create request's op).
const (
	DesignOp = "design"
	CoverOp  = "cover"
)

// MaxDesignBriefBytes caps a brief (the registry's 4 KiB).
const MaxDesignBriefBytes = 4 << 10

// DesignTerms are the status route's designBuild block: whether this
// deployment runs the Report Designer, its flat price, its caps, how many
// images a run takes beside a Design's logo, a brief's bound, its jobs, and
// the web addresses a Design reads.
type DesignTerms struct {
	Enabled       bool     `json:"enabled"`
	Credits       int64    `json:"credits"`
	Caps          Caps     `json:"caps"`
	MaxImages     int      `json:"maxImages"`
	MaxBriefBytes int      `json:"maxBriefBytes"`
	Ops           []string `json:"ops"`
	// URLs are the web addresses a Design takes, each page read by AI
	// Review before the run sees it; nil from a registry that takes none.
	URLs *VoiceURLTerms `json:"urls,omitempty"`
}

// TakesURLs reports whether the registry's Report Designer reads web
// addresses, answering its terms.
func (t *DesignTerms) TakesURLs() (*VoiceURLTerms, bool) {
	if t == nil || !t.Enabled || t.URLs == nil || !t.URLs.Enabled || t.URLs.Max <= 0 {
		return nil, false
	}
	return t.URLs, true
}

// Runs reports whether the registry's Report Designer does the job op: a
// registry that lists no jobs does both.
func (t *DesignTerms) Runs(op string) bool {
	if t == nil || !t.Enabled {
		return false
	}
	if len(t.Ops) == 0 {
		return op == DesignOp || op == CoverOp
	}
	for _, o := range t.Ops {
		if o == op {
			return true
		}
	}
	return false
}

// CallerDesign is what a Report Designer run would hold on the caller's
// payer (0 for a member of an unmetered organization).
type CallerDesign struct {
	HoldCredits int64 `json:"holdCredits"`
}

// DesignBuild is a hosted Report Designer run's record. Applied says the
// design it made is a design of the account's library now, never the
// default, with DesignID and Name; Note says, in one word, why not.
type DesignBuild struct {
	Op       string `json:"op"`
	From     string `json:"from,omitempty"`
	Logo     string `json:"logo,omitempty"`
	Applied  bool   `json:"applied"`
	DesignID string `json:"designId,omitempty"`
	Name     string `json:"name,omitempty"`
	Note     string `json:"note,omitempty"`
	// URLs are the web addresses it was given; Pages the page reads
	// admitted of MaxPages, Scanned the texts AI Review read and Withheld
	// those it kept from the Report Designer.
	URLs     []string `json:"urls,omitempty"`
	MaxPages int      `json:"maxPages,omitempty"`
	Pages    int      `json:"pages,omitempty"`
	Scanned  int      `json:"scanned,omitempty"`
	Withheld int      `json:"withheld,omitempty"`
}

// IsDesignBuild reports whether this row is a hosted Report Designer run.
func (r *Run) IsDesignBuild() bool { return r != nil && r.Kind == KindDesignBuild }

// DesignFacts are what the app knows about a registry before it offers the
// Report Designer In Agentry.
type DesignFacts struct {
	// SignedIn is true when this registry has a stored credential; Library
	// is whether it keeps report designs; Runs is /meta's runs extension.
	SignedIn bool
	Library  bool
	Runs     bool
	// Scopes are the token's scopes; ScopesKnown is false when they could
	// not be read.
	Scopes      []string
	ScopesKnown bool
	// Terms is the status route's designBuild block, nil when it did not
	// say; StatusKnown is false when the route did not answer.
	Terms       *DesignTerms
	StatusKnown bool
	// Active is the account's run in flight, nil for none.
	Active *Run
	// Full says the library already holds the most designs it keeps, so a
	// run would have nowhere to save what it made.
	Full bool
}

// DecideDesign applies the rule for one job. Signed out, or a registry that
// keeps no report designs, means In Agentry is absent and the Report
// Designer runs on this computer. Otherwise it appears, and says in a
// sentence why it cannot be chosen when it cannot.
func DecideDesign(f DesignFacts, op string) Offer {
	if !f.SignedIn || !f.Library {
		return Offer{}
	}
	o := Offer{Offered: true}
	switch {
	case !f.Runs:
		o.Reason = "This registry does not run anything In Agentry, so the Report Designer runs on this computer here."
	case !f.ScopesKnown:
		o.Reason = "This registry did not say what the stored token may do, so the app cannot tell whether it may start the Report Designer."
		o.Remedy = "Sign in again from Settings."
	case !has(f.Scopes, ScopeRunWrite) || !has(f.Scopes, ScopeRunRead) || !has(f.Scopes, ScopeUserProfile):
		o.Reason = "The token stored for this registry does not carry " + strings.Join([]string{ScopeRunRead, ScopeRunWrite}, ", ") +
			" and " + ScopeUserProfile + ", which the Report Designer In Agentry needs. It was minted before this app asked for them."
		o.Remedy = "Sign in again from Settings to mint a token that carries them."
	case !f.StatusKnown:
		o.Reason = "The registry did not answer whether it is taking hosted runs."
		o.Remedy = "Try again, or run the Report Designer on this computer."
	case f.Terms == nil || !f.Terms.Enabled:
		o.Reason = "This registry is not running the Report Designer at the moment."
		o.Remedy = "Run it on this computer, or try again later."
	case !f.Terms.Runs(op):
		o.Reason = "This registry's Report Designer does not write " + opWhat(op) + "."
		o.Remedy = "Run it on this computer."
	case f.Full:
		o.Reason = "Your library holds the most report designs it keeps, so the Report Designer would have nowhere to save what it makes."
		o.Remedy = "Delete a design first."
	case f.Active != nil:
		o.Reason = "This account already has " + ActiveWhat(f.Active) + " In Agentry in flight. Agentry runs one at a time."
		o.Remedy = "Wait for it to finish, or cancel it."
	default:
		o.Available = true
	}
	return o
}

func opWhat(op string) string {
	if op == CoverOp {
		return "custom covers"
	}
	return "whole designs"
}

// DesignNoteSentence says why a finished Report Designer run saved no design
// (x-runs.yaml DesignBuild.note); each was refunded in full. "" for a note
// this app does not know.
func DesignNoteSentence(note string) string {
	switch note {
	case "nothing":
		return "The Report Designer wrote no design, so nothing was saved. Its credits were refunded."
	case "invalid":
		return "The Report Designer's output held no design your library could keep, so nothing was saved. Its credits were refunded."
	case "gone":
		return "The design it was to work on was deleted before it started, so nothing was saved. Its credits were refunded."
	case "full":
		return "Your library already held the most designs it keeps, so nothing was saved. Its credits were refunded."
	case "unavailable":
		return "Agentry could not keep report designs when it finished, so nothing was saved. Its credits were refunded."
	}
	return ""
}
