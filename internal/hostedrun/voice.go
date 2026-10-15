package hostedrun

import (
	"fmt"
	"strings"
	"time"
)

// The Writing Voice In Agentry. A Voice Writer run is a run of kind
// voice_build in the runs extension: Create (samples and a brief), Improve
// (more samples and a brief, over the current voice) or AI Edit (a note),
// each at one flat price, AI Review included. Its voice becomes the
// account's pending version, which the core's AI Review alone makes live. A
// run or a session asks for the runner's live voice with voice: true.

// The PAT scopes a Voice Writer run needs beside run:read and run:write,
// and the one a run that writes in the voice needs.
const (
	ScopeVoiceRead  = "voice:read"
	ScopeVoiceWrite = "voice:write"
)

// The Voice Writer's three jobs (the create request's op).
const (
	VoiceCreate  = "create"
	VoiceImprove = "improve"
	VoiceEdit    = "edit"
)

// MaxVoiceBriefBytes caps a brief or a note (the registry's 4 KiB).
const MaxVoiceBriefBytes = 4 << 10

// VoiceTerms are the status route's voiceBuild block: the flat price, the
// caps, the inputs and the text the Voice Writer reads, Standard's runs a
// cycle, AI Review's credit for a voice saved outside a Voice Writer run,
// and whether a run here may attach a voice.
type VoiceTerms struct {
	Enabled          bool  `json:"enabled"`
	Credits          int64 `json:"credits"`
	Caps             Caps  `json:"caps"`
	MaxInputs        int   `json:"maxInputs"`
	MaxTextBytes     int64 `json:"maxTextBytes"`
	MaxBriefBytes    int   `json:"maxBriefBytes"`
	StandardPerCycle int   `json:"standardPerCycle"`
	AIReviewCredits  int64 `json:"aiReviewCredits"`
	Attach           struct {
		Enabled bool `json:"enabled"`
	} `json:"attach"`
}

// CallerVoice is the signed-in caller's Voice Writer line: whether their
// payer may start one now, and on Standard (Limited) how many of this
// cycle's are used and when the next comes.
type CallerVoice struct {
	Allowed     bool       `json:"allowed"`
	Limited     bool       `json:"limited"`
	Used        int        `json:"used"`
	Max         int        `json:"max"`
	NextAt      *time.Time `json:"nextAt,omitempty"`
	HoldCredits int64      `json:"holdCredits"`
}

// VoiceBuild is a hosted Voice Writer run's record. Applied says its
// VOICE.md is the account's pending version now, handed to AI Review; Note
// says, in one word, why not.
type VoiceBuild struct {
	Op           string `json:"op"`
	MaxTextBytes int64  `json:"maxTextBytes,omitempty"`
	From         string `json:"from,omitempty"`
	Applied      bool   `json:"applied"`
	Digest       string `json:"digest,omitempty"`
	Bytes        int    `json:"bytes,omitempty"`
	Note         string `json:"note,omitempty"`
}

// AttachedVoice is the Writing Voice a run attached: from the hand-off,
// the digest of the live voice it was given.
type AttachedVoice struct {
	Digest string `json:"digest,omitempty"`
}

// IsVoiceBuild reports whether this row is a hosted Voice Writer run.
func (r *Run) IsVoiceBuild() bool { return r != nil && r.Kind == KindVoiceBuild }

// VoiceFacts are what the app knows about a registry before it offers the
// Voice Writer In Agentry.
type VoiceFacts struct {
	// SignedIn is true when this registry has a stored credential; Voice
	// is /meta's features.voice (the core keeps voices); Runs is /meta's
	// runs extension.
	SignedIn bool
	Voice    bool
	Runs     bool
	// Scopes are the token's scopes; ScopesKnown is false when they could
	// not be read.
	Scopes      []string
	ScopesKnown bool
	// Terms is the status route's voiceBuild block, nil when it did not
	// say; StatusKnown is false when the route did not answer.
	Terms       *VoiceTerms
	StatusKnown bool
	// Caller is the caller's voice line, nil when the route named none.
	Caller *CallerVoice
	// Active is the account's run in flight, nil for none.
	Active *Run
	// HasVoice says the account keeps a voice to improve or edit; an
	// Improve or an AI Edit needs one, a Create does not.
	HasVoice bool
}

// VoiceLimitSentence is Standard's refusal before a run, with when the
// next is theirs.
func VoiceLimitSentence(c CallerVoice) string {
	times := "once a billing cycle"
	switch {
	case c.Max == 2:
		times = "twice a billing cycle"
	case c.Max > 2:
		times = fmt.Sprintf("%d times a billing cycle", c.Max)
	}
	s := "On Standard you can run the Voice Writer in Agentry " + times + ", and this cycle's runs are used."
	if c.NextAt != nil {
		s += " The next is yours on " + c.NextAt.UTC().Format("2 January 2006") + "."
	}
	return s
}

// DecideVoice applies the rule for one job. Signed out, or a registry that
// keeps no voices, means the choice is absent and the Voice Writer runs on
// this computer, as it always can. Otherwise the choice appears, and says
// in a sentence why it cannot be chosen when it cannot.
func DecideVoice(f VoiceFacts, job string) Offer {
	if !f.SignedIn || !f.Voice {
		return Offer{}
	}
	o := Offer{Offered: true}
	switch {
	case !f.Runs:
		o.Reason = "This registry does not run anything In Agentry, so the Voice Writer runs on this computer here."
	case !f.ScopesKnown:
		o.Reason = "This registry did not say what the stored token may do, so the app cannot tell whether it may start the Voice Writer."
		o.Remedy = "Sign in again from Settings."
	case !has(f.Scopes, ScopeRunWrite) || !has(f.Scopes, ScopeRunRead) || !has(f.Scopes, ScopeVoiceRead) || !has(f.Scopes, ScopeVoiceWrite):
		o.Reason = "The token stored for this registry does not carry " + strings.Join([]string{ScopeRunRead, ScopeRunWrite, ScopeVoiceRead}, ", ") +
			" and " + ScopeVoiceWrite + ", which the Voice Writer In Agentry needs. It was minted before this app asked for them."
		o.Remedy = "Sign in again from Settings to mint a token that carries them."
	case !f.StatusKnown:
		o.Reason = "The registry did not answer whether it is taking hosted runs."
		o.Remedy = "Try again, or run the Voice Writer on this computer."
	case f.Terms == nil || !f.Terms.Enabled:
		o.Reason = "This registry is not running the Voice Writer at the moment."
		o.Remedy = "Run it on this computer, or try again later."
	case job != VoiceCreate && !f.HasVoice:
		o.Reason = "Your account keeps no Writing Voice to improve or edit."
		o.Remedy = "Create one, or submit this computer's."
	case f.Caller != nil && !f.Caller.Allowed:
		o.Reason = VoiceLimitSentence(*f.Caller)
		o.Remedy = "Edit your voice by hand, or run the Voice Writer on this computer for no credits; or upgrade to Pro."
		o.Upgrade = true
	case f.Active != nil:
		o.Reason = "This account already has " + ActiveWhat(f.Active) + " In Agentry in flight. Agentry runs one at a time."
		o.Remedy = "Wait for it to finish, or cancel it."
	default:
		o.Available = true
	}
	return o
}

// VoiceNoteSentence says why a finished Voice Writer run left no pending
// voice (x-runs.yaml VoiceBuild.note); each was refunded in full. "" for a
// note this app does not know.
func VoiceNoteSentence(note string) string {
	switch note {
	case "nothing":
		return "The Voice Writer wrote no voice, so nothing changed. Its credits were refunded."
	case "invalid":
		return "The Voice Writer's output held no voice Agentry could keep. Its credits were refunded."
	case "gone":
		return "The voice it was to work on was deleted before it started. Its credits were refunded."
	case "unavailable":
		return "Agentry could not keep voices when it finished, so nothing changed. Its credits were refunded."
	}
	return ""
}
