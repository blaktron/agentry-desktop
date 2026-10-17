package hostedrun

import "strings"

// When In Agentry is offered, and when it is offered but unavailable with a
// reason. The decision is a function of facts the app already has, so it is one
// table to read and one table to test — the app never finds out by making a
// request that 403s.

// The PAT scopes a hosted run needs, and the one a run of a release that
// declares a secret needs besides: the start sets each declared secret the
// account does not keep.
const (
	ScopeRunRead     = "run:read"
	ScopeRunWrite    = "run:write"
	ScopeSecretWrite = "secret:write"
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
	// its declared MCP servers'.
	DeclaredSecrets []string
	// OptionalSecrets are those of DeclaredSecrets a run can start without
	// (capabilities.optionalSecrets). A run carries one only where the
	// account keeps it, so only the others ask anything of the registry or
	// the token.
	OptionalSecrets []string
	// Vault is true when the registry's /meta lists `vault`: it keeps an
	// account's secrets for hosted runs.
	Vault bool
	// Scopes are the token's scopes as the registry reports them;
	// ScopesKnown is false when they could not be read.
	Scopes      []string
	ScopesKnown bool
	// Enabled is the status route's `enabled`; StatusKnown is false when
	// the route could not be read.
	Enabled     bool
	StatusKnown bool
	// SessionsEnabled is the status route's `sessions.enabled`: this deployment
	// runs hosted sessions; false from a registry that does not say so.
	// SessionAllowed is the signed-in caller's `caller.session.allowed`, whether
	// their payer's tier may start one; nil where the registry did not say.
	SessionsEnabled bool
	SessionAllowed  *bool
	// HostsOnline is the status route's sandbox hosts answering: a session is
	// refused with none (409 sessions-unavailable), never queued, since a person
	// waits on it as it starts.
	HostsOnline int
	// Schedules is true when the registry's /meta lists `schedules`: it runs a
	// QuickAgent In Agentry later or on a repeat. ScheduleAllowed is whether the
	// signed-in caller's payer may schedule one, read from the status route's
	// `caller.session.allowed`: both are billing.AboveStandard of the payer's
	// tier, Promo Pro counted (the registry's schedule check also asks that the
	// account admits a charge). The status route has no line of the schedules' own. Nil
	// where the registry did not say; the create answers schedule-tier either way.
	Schedules       bool
	ScheduleAllowed *bool
	// TargetHosts is true when the release declares a target host
	// (capabilities.targetHosts): it works a machine the person names over
	// SSH, which only a run on their own computer does, so no choice In
	// Agentry appears for it.
	TargetHosts bool
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
	// Upgrade says the way out is a higher tier: the client offers a link to the
	// registry's subscription page, opened in the system browser (creating
	// knowledge in Agentry is Pro and above).
	Upgrade bool
}

// Decide applies the rule. Not signed in, no runs extension, a release
// that does not declare the autonomous mode, or one that declares a target
// host means the choice is absent and the local modes are
// exactly as they were.
func Decide(f Facts) Offer {
	if !f.SignedIn || !f.Extension || !declares(f.Modes, "autonomous") || f.TargetHosts {
		return Offer{}
	}
	o := Offer{Offered: true}
	if o.Reason, o.Remedy = tokenAndSecrets(f, "a run"); o.Reason != "" {
		return o
	}
	switch {
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

// SessionTierReason and SessionTierRemedy say why a payer on Standard is not
// offered a session In Agentry, and the way out (the site's session tier
// sentence, in this app's words).
const (
	SessionTierReason = "Talking to an agent In Agentry is part of Pro, Business and Enterprise."
	SessionTierRemedy = "On Standard, talk to it on this computer, for no credits; or upgrade to Pro."
	// SessionNoHostReason and SessionNoHostRemedy say why no session can
	// start while no sandbox host answers, in the registry's own words for
	// its refusal.
	SessionNoHostReason = "No sandbox host is answering right now, so a session In Agentry cannot start."
	SessionNoHostRemedy = "Try again in a few minutes, or talk to it on this computer."
)

// DecideSession applies the rule for Talk to it In Agentry, a hosted session of
// the release's interactive mode. The choice appears only where the release
// declares the interactive mode, the registry's status says it runs sessions
// and says whether the caller's payer may start one; anything short of that
// leaves the local modes exactly as they were. A payer whose tier may not
// (Standard) is told so, with the way up, before anything about the token.
func DecideSession(f Facts) Offer {
	if !f.SignedIn || !f.Extension || !declares(f.Modes, ModeInteractive) ||
		!f.StatusKnown || !f.SessionsEnabled || f.SessionAllowed == nil || f.TargetHosts {
		return Offer{}
	}
	o := Offer{Offered: true}
	if !*f.SessionAllowed {
		o.Reason, o.Remedy, o.Upgrade = SessionTierReason, SessionTierRemedy, true
		return o
	}
	if o.Reason, o.Remedy = tokenAndSecrets(f, "a session"); o.Reason != "" {
		return o
	}
	if f.HostsOnline == 0 {
		o.Reason, o.Remedy = SessionNoHostReason, SessionNoHostRemedy
		return o
	}
	o.Available = true
	return o
}

// ScheduleTierReason and ScheduleTierRemedy say why a payer on Standard is
// not offered a schedule In Agentry, and the way out, in the registry's
// own words for its 403 schedule-tier.
const (
	ScheduleTierReason = "Scheduling runs In Agentry is part of Pro, Business and Enterprise."
	ScheduleTierRemedy = "On Standard, schedule it on this computer, for no credits; or upgrade to Pro."
)

// DecideSchedule applies the rule for Later and Repeat In Agentry. The choice
// appears where the account is signed in and the registry schedules runs; a
// payer on Standard is told so, with the way up, before anything about the
// token, which needs the run scopes a run In Agentry already asks for (the
// schedules routes take run:read and run:write). Whether this release can run
// In Agentry at all is Decide's, and what a fire would lack is the create's
// own check.
func DecideSchedule(f Facts) Offer {
	if !f.SignedIn || !f.Schedules || f.TargetHosts {
		return Offer{}
	}
	o := Offer{Offered: true}
	switch {
	case f.ScheduleAllowed != nil && !*f.ScheduleAllowed:
		o.Reason, o.Remedy, o.Upgrade = ScheduleTierReason, ScheduleTierRemedy, true
	case !f.ScopesKnown:
		o.Reason, o.Remedy = "This registry did not say what the stored token may do, so the app cannot tell whether it may schedule a run.", "Sign in again from Settings."
	case !has(f.Scopes, ScopeRunWrite) || !has(f.Scopes, ScopeRunRead):
		o.Reason = "The token stored for this registry does not carry " + ScopeRunRead + " and " + ScopeRunWrite + ", which scheduling a run In Agentry needs. It was minted before this app asked for them."
		o.Remedy = "Sign in again from Settings to mint a token that carries them."
	default:
		o.Available = true
	}
	return o
}

// tokenAndSecrets is what both choices ask of the release's secrets and the
// stored token: a reason and a remedy, or two empty strings when neither
// stands in the way. what names the thing started ("a run", "a session").
func tokenAndSecrets(f Facts, what string) (reason, remedy string) {
	required := f.requiredSecrets()
	secrets := len(required) > 0
	switch {
	case secrets && !f.Vault:
		// A registry without the vault holds no secret of anyone's. A release whose
		// secrets are all optional runs without them.
		return "This release requires " + secretList(required) +
				", and this registry keeps no secrets for runs In Agentry.",
			"Run it on this machine, where the key stays in the release's own profile."
	case !f.ScopesKnown:
		return "This registry did not say what the stored token may do, so the app cannot tell whether it may start " + what + ".",
			"Sign in again from Settings."
	case !has(f.Scopes, ScopeRunWrite) || !has(f.Scopes, ScopeRunRead):
		return "The token stored for this registry does not carry " + ScopeRunRead + " and " + ScopeRunWrite +
				", which " + what + " In Agentry needs. It was minted before this app asked for them.",
			"Sign in again from Settings to mint a token that carries them."
	case secrets && !has(f.Scopes, ScopeSecretWrite):
		return "This release requires " + secretList(required) +
				", and the token stored for this registry does not carry " + ScopeSecretWrite +
				", which " + what + " In Agentry needs to set a secret your account does not keep. It was minted before this app asked for it.",
			"Sign in again from Settings to mint a token that carries it."
	}
	return "", ""
}

// requiredSecrets is every declared secret a run cannot start without: those
// OptionalSecrets does not name.
func (f Facts) requiredSecrets() []string {
	var out []string
	for _, s := range f.DeclaredSecrets {
		if !has(f.OptionalSecrets, s) {
			out = append(out, s)
		}
	}
	return out
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
