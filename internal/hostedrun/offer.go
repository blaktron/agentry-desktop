package hostedrun

import "strings"

// When In Agentry is offered, and when it is offered but unavailable with
// a reason (plan §4.7, D6, D7; agentry-desktop#77). The decision is a
// function of facts the app already has, so it is one table to read and
// one table to test — the app never finds out by making a request that
// 403s.

// The PAT scopes a hosted run needs (agentry#348).
const (
	ScopeRunRead  = "run:read"
	ScopeRunWrite = "run:write"
)

// Facts are what the app knows about a registry and a release before it
// offers the mode.
type Facts struct {
	// SignedIn is true when this registry has a stored credential.
	SignedIn bool
	// Extension is true when the registry's /meta lists `runs`.
	Extension bool
	// Modes are the run modes the release declares.
	Modes []string
	// DeclaredSecrets are the secrets the release declares — its own and
	// its declared MCP servers' (D7).
	DeclaredSecrets []string
	// Scopes are the token's scopes as the registry reports them;
	// ScopesKnown is false when they could not be read.
	Scopes      []string
	ScopesKnown bool
	// Enabled is the status route's `enabled`; StatusKnown is false when
	// the route could not be read.
	Enabled     bool
	StatusKnown bool
}

// Offer is the answer: whether the choice appears at all, whether it can
// be chosen, and — when it cannot — why, and what to do about it.
type Offer struct {
	// Offered says the third choice appears beside the local modes.
	Offered bool
	// Available says it can be chosen.
	Available bool
	// Reason is why it cannot, in a sentence; Remedy is the way out, or
	// empty when there is none yet.
	Reason string
	Remedy string
}

// Decide applies the rule. Not signed in, no runs extension, or a release
// that does not declare the autonomous mode means the choice is absent and
// the local modes are exactly as they were.
func Decide(f Facts) Offer {
	if !f.SignedIn || !f.Extension || !declares(f.Modes, "autonomous") {
		return Offer{}
	}
	o := Offer{Offered: true}
	switch {
	case len(f.DeclaredSecrets) > 0:
		// D7: the hosted credentials vault is a separate effort, and
		// until it exists the platform holds no secret of anyone's.
		o.Reason = "This release declares " + secretList(f.DeclaredSecrets) +
			", and a run In Agentry has nowhere to keep a key of yours yet. Run it on this machine, where the key stays in the release's own profile."
	case !f.ScopesKnown:
		o.Reason = "This registry did not say what the stored token may do, so the app cannot tell whether it may start a run."
		o.Remedy = "Sign in again from Settings."
	case !has(f.Scopes, ScopeRunWrite) || !has(f.Scopes, ScopeRunRead):
		o.Reason = "The token stored for this registry does not carry " + ScopeRunRead + " and " + ScopeRunWrite +
			", which a run In Agentry needs. It was minted before this app asked for them."
		o.Remedy = "Sign in again from Settings to mint a token that carries them."
	case f.StatusKnown && !f.Enabled:
		o.Reason = "This registry is not taking hosted runs at the moment."
		o.Remedy = "Run it on this machine, or try again later."
	case !f.StatusKnown:
		o.Reason = "The registry did not answer whether it is taking hosted runs."
		o.Remedy = "Try again, or run it on this machine."
	default:
		o.Available = true
	}
	return o
}

func declares(modes []string, want string) bool { return has(modes, want) }

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// secretList names the declared secrets the way the consent panel does.
func secretList(names []string) string {
	switch len(names) {
	case 1:
		return "the key " + names[0]
	default:
		return "the keys " + strings.Join(names, ", ")
	}
}
