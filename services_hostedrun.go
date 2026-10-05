// In Agentry: a hosted run of a published release, started with the app's
// token, watched on the registry, its outputs and receipt downloaded
// into the run folder the person picked (plan hosted-runs.md §4.7, D10;
// agentry-desktop#77).
//
// Nothing here touches docker, the operator policy or the run lock: the
// execution plane lives beside the registry's daemon (D3), so a hosted run
// and a local run cannot wait on each other. The app sends the task, the
// intake answers, the input files and the account's bearer token, and
// nothing else — no model key, no worker token.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// eventHostedRun carries the hosted run's state to the panel. A run In
// Agentry has no log to send (D12, D16); runner:run-log is the local
// engine's alone.
const eventHostedRun = "runner:hosted-run"

// emitEvent is the Wails emitter, a package variable purely as a test seam
// (the hosted lifecycle tests follow a run with no webview attached).
// Production code never reassigns it.
var emitEvent = func(app *App, name string, data ...any) {
	if app == nil || app.ctx == nil {
		return
	}
	runtime.EventsEmit(app.ctx, name, data...)
}

// hostedPoll is how often a run In Agentry is re-read while it moves; a
// test shortens it.
var hostedPoll = 3 * time.Second

// hostedFollow is the In-Agentry run this app is following. RunAPI's
// hostedMu guards the pointer to it; the fields are written once, at the
// start of the follow, and read after.
type hostedFollow struct {
	registry string
	id       string
	outDir   string
	cancel   context.CancelFunc
	done     chan struct{}
	// session says the run is a hosted session: its conversation is
	// streamed as well as its state (services_hostedrun_session.go), and
	// replay asks the stream to start again from the host's first frame
	// held, for a chat that has just opened.
	session bool
	replay  chan struct{}
	ref     string
	// idle is a session's idle close in seconds, as its record says.
	idle int
	// lost is why a session's stream stopped before its end, once it has:
	// a chat that opens after that is told again (ReplayHostedSession).
	// Guarded by RunAPI's hostedMu.
	lost string
}

// HostedCapsView is a tier's ceilings for one run, as the person is told
// them: time and turns. The registry's token ceiling is not carried: a
// hosted run is stated in credits, time and turns only (plan
// scan-and-run-tweaks.md track B, agentry-desktop#457).
type HostedCapsView struct {
	TimeoutSeconds int `json:"timeoutSeconds"`
	MaxTurns       int `json:"maxTurns"`
}

// hostedCaps is the panel's reading of a tier's caps.
func hostedCaps(c hostedrun.Caps) *HostedCapsView {
	return &HostedCapsView{TimeoutSeconds: c.TimeoutSeconds, MaxTurns: c.MaxTurns}
}

// HostedAvailabilityView answers "is In Agentry offered here, and can it
// be chosen?" — and carries what the panel states before anything is
// created: the hold, the balance, the caps, the input limits.
type HostedAvailabilityView struct {
	// Offered says the choice appears beside the local modes; Available
	// says it can be chosen. Offered without Available is the
	// unavailable-with-reason state.
	Offered   bool   `json:"offered"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
	Remedy    string `json:"remedy,omitempty"`

	SignedIn  bool `json:"signedIn"`
	Extension bool `json:"extension"`
	Enabled   bool `json:"enabled"`
	// RunnersOnline counts what can take a run now: the pull workers and
	// the sandbox hosts the registry pushes to.
	RunnersOnline int `json:"runnersOnline"`
	Queued        int `json:"queued"`

	// Tier, HoldCredits and Caps are the registry's numbers for a run of
	// this account's; the app does no arithmetic of its own with them. The
	// rates a settlement is computed at are the registry's and are not
	// carried (plan scan-and-run-tweaks.md D9).
	Tier        string          `json:"tier,omitempty"`
	HoldCredits int64           `json:"holdCredits"`
	Caps        *HostedCapsView `json:"caps,omitempty"`
	// Balance is the account's credits; BalanceKnown is false when the
	// registry has no billing extension or the read failed.
	Balance      int64 `json:"balance"`
	BalanceKnown bool  `json:"balanceKnown"`
	// KnowledgeAttach says a run In Agentry here may attach the account's
	// packs kept in Agentry (agentry-enterprise#172), at most MaxPacks:
	// the registry says so and the token reads knowledge. KnowledgeNote
	// says why not where the registry would but the token cannot.
	KnowledgeAttach bool   `json:"knowledgeAttach"`
	MaxPacks        int    `json:"maxPacks,omitempty"`
	KnowledgeNote   string `json:"knowledgeNote,omitempty"`

	MaxInputFiles int   `json:"maxInputFiles"`
	MaxInputBytes int64 `json:"maxInputBytes"`

	// DeclaredSecrets are the secrets the release declares, its own and
	// its MCP servers', by name: a secret the account keeps in Agentry is
	// used as it is, one it does not keep is asked for at the start
	// (agentry-desktop#300).
	DeclaredSecrets []string `json:"declaredSecrets,omitempty"`

	// Session is Talk to it In Agentry, a hosted session of the release's
	// interactive mode (agentry-desktop#403): whether it is offered, and
	// the registry's terms for one. Nil where it is not offered.
	Session *HostedSessionOfferView `json:"session,omitempty"`

	// PublisherEarns says the release's publisher earns a share of the
	// credits runs of it spend, which the run setup says under what a run
	// costs (plan creator-monetization.md D10; agentry-desktop#613). Never
	// for the reader's own QuickAgent or their organization's.
	PublisherEarns bool `json:"publisherEarns"`
}

// HostedArtefactView is one artefact of a run and, once downloaded, where
// it landed on this machine.
type HostedArtefactView struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Path  string `json:"path,omitempty"`
}

// HostedRunView is one hosted run as the panel shows it: the identity and
// status while it moves, and the §R8 result once it is over — a local
// run's facts in credits, time and turns, never tokens.
type HostedRunView struct {
	ID     string `json:"id"`
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
	Status string `json:"status"`
	// State is the badge's word, the site's own: queued, running, done —
	// pass, done — fail, cancelled.
	State string `json:"state"`
	// StatusLine says where the run stands, in a sentence.
	StatusLine      string `json:"statusLine"`
	QueuePosition   int    `json:"queuePosition,omitempty"`
	CancelRequested bool   `json:"cancelRequested"`

	// Outcome, ExitClass and OutcomeLine are the finished run's verdict
	// (plan §5): the registry's word for it, the runner's exit class
	// verbatim, and the sentence that says what it means.
	Outcome     string `json:"outcome,omitempty"`
	ExitClass   string `json:"exitClass,omitempty"`
	OutcomeLine string `json:"outcomeLine,omitempty"`
	OutcomeTone string `json:"outcomeTone,omitempty"`
	Error       string `json:"error,omitempty"`

	// Turns and WallMS are the run's use as the person is told it; its
	// tokens and their dollar cost stay with the registry, which settles
	// them in credits (agentry-desktop#457).
	Turns  int   `json:"turns"`
	WallMS int64 `json:"wallMs"`

	// HoldCredits is what the run holds; SettledCredits what it cost once
	// it finished, both as the account reports them. Settled says the
	// settlement has been reported.
	HoldCredits    int64 `json:"holdCredits"`
	SettledCredits int64 `json:"settledCredits"`
	Settled        bool  `json:"settled"`

	// ReceiptKept says the registry holds a receipt for the run. A pushed
	// run's is the host's cut of the CLI's stdout envelope
	// (agentry-enterprise#121); a run the registry kept none for — one
	// pushed before that — has the panel say so rather than read one from
	// a file in the outputs.
	ReceiptKept bool `json:"receiptKept"`
	// CapabilitiesEnforced is the receipt's block, listed verbatim; its
	// knowledge line is also KnowledgeLine, which the result view shows
	// under the result as a local run's is.
	CapabilitiesEnforced map[string]string `json:"capabilitiesEnforced,omitempty"`
	KnowledgeLine        string            `json:"knowledgeLine,omitempty"`
	// Knowledge are the packs the run attaches, as the registry records
	// them: each by name, and from its hand-off the digest of the tree it
	// was given (agentry-desktop#275).
	Knowledge        []HostedPackView `json:"knowledge,omitempty"`
	AttestationState string           `json:"attestationState,omitempty"`
	Adapter          string           `json:"adapter,omitempty"`

	// ResultJSON is result.json as the run wrote it, rendered by the same
	// component a local run's result is.
	ResultJSON string `json:"resultJson,omitempty"`
	// OutDir is the folder the artefacts landed in; Outputs what landed.
	OutDir  string               `json:"outDir,omitempty"`
	Outputs []HostedArtefactView `json:"outputs"`
	// DownloadError says why an artefact did not land, when one did not.
	DownloadError string `json:"downloadError,omitempty"`

	Caps *HostedCapsView `json:"caps,omitempty"`
	// Mode is the run's mode: "interactive" for a hosted session, whose
	// Session record says what its time and tokens have cost.
	Mode string `json:"mode,omitempty"`
	// Task is the run's task, for a chat that reopens on a session.
	Task       string             `json:"task,omitempty"`
	Session    *HostedSessionView `json:"session,omitempty"`
	StartedAt  string             `json:"startedAt,omitempty"`
	CreatedAt  string             `json:"createdAt,omitempty"`
	FinishedAt string             `json:"finishedAt,omitempty"`
	// Final says this is the last word on the run: it is over and its
	// artefacts are downloaded.
	Final bool `json:"final"`
}

// HostedPackView is one knowledge pack a run In Agentry attaches.
type HostedPackView struct {
	Name   string `json:"name"`
	Digest string `json:"digest,omitempty"`
}

// HostedStartView is what starting (or reattaching to) a hosted run
// returns. A registry that refuses the create answers as the Build page's
// publish does: a refusal slug and the sentence to show, not an error.
type HostedStartView struct {
	Run *HostedRunView `json:"run,omitempty"`
	// Refusal is the problem's slug (credits-exhausted, run-active,
	// secrets-declared, mode-not-declared…); Detail the sentence.
	Refusal string `json:"refusal,omitempty"`
	Detail  string `json:"detail,omitempty"`
	// Balance and ResetsAt come from a credits-exhausted refusal. Where
	// the subscription is managed is deliberately not here: the panel asks
	// SettingsAPI.OpenUpgrade at the moment a person clicks, so the
	// registry's answer is checked against its own origin before anything
	// opens (agentry-enterprise#41).
	Balance  int64  `json:"balance,omitempty"`
	Hold     int64  `json:"hold,omitempty"`
	ResetsAt string `json:"resetsAt,omitempty"`
	// Missing are the declared secrets a secret-missing refusal names: the
	// ones the panel asks for (agentry-desktop#300).
	Missing []string `json:"missing,omitempty"`
}

// HostedAvailability answers the panel's first question about a release:
// is In Agentry offered here, can it be chosen, and — either way — what
// does a run of this account's hold and what is the balance.
func (r *RunAPI) HostedAvailability(reg, ref string) (*HostedAvailabilityView, error) {
	reg = settings.Normalize(reg)
	ctx := r.ctx()
	token := settings.TokenFor(reg)
	v := &HostedAvailabilityView{
		SignedIn:      token != "",
		MaxInputFiles: hostedrun.MaxInputFiles,
		MaxInputBytes: hostedrun.MaxInputBytes,
	}
	facts := hostedrun.Facts{SignedIn: v.SignedIn}
	var exts []string
	if v.SignedIn {
		var err error
		exts, err = extensionsOf(ctx, reg)
		if err == nil {
			facts.Extension = hasString(exts, "runs")
			facts.Vault = hasString(exts, "vault")
		}
		v.Extension = facts.Extension
	}
	// A registry with no runs extension, or nobody signed in: the mode is
	// absent and nothing else is asked of the registry.
	if !facts.SignedIn || !facts.Extension {
		o := hostedrun.Decide(facts)
		v.Offered, v.Available, v.Reason, v.Remedy = o.Offered, o.Available, o.Reason, o.Remedy
		return v, nil
	}

	// What the release declares, from the resolve alone: reading the
	// operator policy or an MCP profile to answer this would make the mode
	// depend on the machine it is offered on.
	modes, secrets, err := run.Declared(ctx, reg, ref)
	if err != nil {
		return nil, err
	}
	facts.Modes, facts.DeclaredSecrets = modes, secrets
	v.DeclaredSecrets = secrets

	facts.Scopes, facts.ScopesKnown = tokenScopes(ctx, reg, token)

	client := hostedrun.New(reg, token)
	var terms *hostedrun.SessionTerms
	if st, err := client.Status(ctx); err == nil {
		facts.Enabled, facts.StatusKnown = st.Enabled, true
		if st.Sessions != nil {
			facts.SessionsEnabled, terms = st.Sessions.Enabled, st.Sessions
		}
		facts.HostsOnline = st.HostsOnline
		if st.Caller != nil && st.Caller.Session != nil {
			allowed := st.Caller.Session.Allowed
			facts.SessionAllowed = &allowed
		}
		v.Enabled, v.RunnersOnline, v.Queued = st.Enabled, st.RunnersOnline(), st.Queued
		if st.Caller != nil {
			v.Tier, v.HoldCredits = st.Caller.Tier, st.Caller.HoldCredits
			v.Caps = hostedCaps(st.Caller.Caps)
		}
		if ka := st.KnowledgeAttach; ka != nil && ka.Enabled {
			if facts.ScopesKnown && !slices.Contains(facts.Scopes, hostedrun.ScopeKnowledgeRead) {
				v.KnowledgeNote = "This computer's token for " + reg + " does not carry " + hostedrun.ScopeKnowledgeRead + ", which attaching knowledge to a run In Agentry needs. It was minted before this app asked for it: sign in again to attach packs."
			} else {
				v.KnowledgeAttach, v.MaxPacks = true, ka.MaxPacks
			}
		}
	}
	if sub, err := fetchSubscription(ctx, reg, token); err == nil && sub != nil {
		v.Balance, v.BalanceKnown = sub.Balance, true
	}

	o := hostedrun.Decide(facts)
	v.Offered, v.Available, v.Reason, v.Remedy = o.Offered, o.Available, o.Reason, o.Remedy
	v.Session = sessionOffer(hostedrun.DecideSession(facts), terms)
	if v.Offered {
		v.PublisherEarns = publisherEarnsHere(ctx, reg, token, ref, exts)
	}
	return v, nil
}

// StartHostedRun uploads the run's input files, creates the run and starts
// following it. The panel has shown the declared contract and collected
// the intake answers first: nothing is created until Run is clicked.
//
// packs names the account's knowledge packs to attach (agentry-desktop#275;
// plan knowledge.md §4.5): each must be one this computer's Knowledge list
// shows as kept in Agentry and scanned there, and it is sent as the
// registry's id for it — a name the list does not show that way is refused
// here, before anything is uploaded, so the page cannot attach what it was
// not offered. The registry checks again that each is the account's and
// ready.
//
// The secrets this app set for the start and did not keep
// (SetHostedSecret with keep false) are deleted again when the run does
// not start, whatever stopped it; when it starts, the vault removes them
// as the run ends (plan hosted-credentials-vault.md D12).
func (r *RunAPI) StartHostedRun(reg, ref, task string, inputPaths []string, outDir string, intakeAnswers map[string]string, packs []string, writeInVoice bool) (*HostedStartView, error) {
	return r.startHosted(reg, ref, task, inputPaths, outDir, intakeAnswers, packs, "", writeInVoice)
}

// startHosted is StartHostedRun and StartHostedSession: the same gates, the
// same uploads and the same create, which mode makes a run ("") or a hosted
// session (hostedrun.ModeInteractive).
//
// writeInVoice asks the run to write in the account's live Writing Voice
// (plan writing-voice.md §5, D5): the registry checks there is one, and at
// the hand-off gives the run the live version as it stands then.
func (r *RunAPI) startHosted(reg, ref, task string, inputPaths []string, outDir string, intakeAnswers map[string]string, packs []string, mode string, writeInVoice bool) (view *HostedStartView, err error) {
	reg = settings.Normalize(reg)
	ctx := r.ctx()
	// runMayExist: the registry created the run, or a create's answer never
	// came back. Either way the run may hold the secrets, so they are left
	// to it, or to the vault's sweep, rather than deleted from under it,
	// whatever this app does next (a folder it cannot make, another run
	// already followed).
	runMayExist := false
	defer func() {
		if (err == nil && view != nil && view.Run != nil) || runMayExist {
			r.forgetHeld(reg)
			return
		}
		r.DiscardHostedSecrets(reg)
	}()
	token := settings.TokenFor(reg)
	if token == "" {
		return nil, fmt.Errorf("sign in to %s to run In Agentry", reg)
	}
	if err := r.hostedIdle(); err != nil {
		return nil, err
	}
	// The paths came from a native picker, or they are not used (D-030).
	for _, p := range inputPaths {
		if err := r.app.requirePicked("input", p); err != nil {
			return nil, err
		}
	}
	if err := r.app.requireOutputDir(outDir); err != nil {
		return nil, err
	}
	// A release the registry admitted without a scan is not run In
	// Agentry from here (M23, D-029): the platform records no consent for
	// it yet (agentry-enterprise#55), and a run nobody consented to would
	// carry a receipt that says nothing of it. The page withholds the
	// mode; this is the same rule in Go, for any caller of the binding.
	if sig, err := run.ResolveSignature(ctx, reg, ref, false); err == nil && sig.Unscanned() {
		return nil, fmt.Errorf("%s was admitted by the registry without a scan; a run In Agentry is not offered for an unscanned release until the platform records that consent (agentry-enterprise#55) — run it on this machine, where the consent is yours", ref)
	}
	attach, err := r.packsInAgentry(reg, packs)
	if err != nil {
		return nil, err
	}
	files, err := hostedrun.StageInputs(inputPaths)
	if err != nil {
		return nil, err
	}
	client := hostedrun.New(reg, token)
	ids, err := client.UploadInputs(ctx, files)
	if err != nil {
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return r.refusalView(refusal), nil
		}
		return nil, err
	}
	created, err := client.Create(ctx, hostedrun.CreateRequest{Ref: ref, Task: task, Intake: intakeAnswers, Inputs: ids, Knowledge: attach, Mode: mode, Voice: writeInVoice})
	if err != nil {
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return r.refusalView(refusal), nil
		}
		runMayExist = true
		return nil, err
	}
	runMayExist = true
	dir, err := hostedOutDir(outDir, created)
	if err != nil {
		return nil, err
	}
	if dir, err = r.follow(reg, token, dir, created); err != nil {
		return nil, err
	}
	return &HostedStartView{Run: hostedRunView(created, dir, nil, "", "", false)}, nil
}

// packsInAgentry resolves the pack names a run In Agentry attaches to the
// registry's ids for them, from the Knowledge list for reg: each once, each
// kept in Agentry and scanned there. None is none.
func (r *RunAPI) packsInAgentry(reg string, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	list, err := r.knowledge().List(r.ctx(), reg)
	if err != nil {
		return nil, fmt.Errorf("list the knowledge packs to attach: %w", err)
	}
	var ids []string
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		// A name a person's own pack and an organization's shared one
		// both hold in Agentry is refused rather than guessed.
		if n := inAgentryNamed(list.Packs, name); n > 1 {
			return nil, fmt.Errorf("more than one knowledge pack named %s is kept in Agentry (yours and an organization's, or two organizations'): rename one to attach it to a run In Agentry", name)
		}
		found := false
		for _, p := range list.Packs {
			if p.Name != name || !p.InAgentry {
				continue
			}
			found = true
			if !p.AgentryReady || p.AgentryID == "" {
				return nil, fmt.Errorf("the knowledge pack %s is not ready in Agentry: attach it once Agentry has scanned it", name)
			}
			ids = append(ids, p.AgentryID)
			break
		}
		if !found {
			return nil, fmt.Errorf("the knowledge pack %s is not kept in Agentry; upload it from the Knowledge page to attach it to a run In Agentry", name)
		}
	}
	return ids, nil
}

// inAgentryNamed counts the packs of name kept in Agentry, by their copy
// there: a pack kept in both places is one.
func inAgentryNamed(packs []knowledge.Pack, name string) int {
	ids := map[string]bool{}
	for _, p := range packs {
		if p.Name == name && p.InAgentry && p.AgentryID != "" {
			ids[p.AgentryID] = true
		}
	}
	return len(ids)
}

// AttachHostedRun reattaches to this account's active run, if there is
// one, rather than starting a second (one run per account, D11). It
// returns a view with no run when the account has none.
func (r *RunAPI) AttachHostedRun(reg, outDir string) (*HostedStartView, error) {
	// The folder a reattached run's outputs download into, and that the
	// result view lists and reveals afterwards, is one a native picker
	// returned, as for StartHostedRun (D-030, D-035): never a path the page
	// typed.
	if err := r.app.requireOutputDir(outDir); err != nil {
		return nil, err
	}
	reg = settings.Normalize(reg)
	ctx := r.ctx()
	token := settings.TokenFor(reg)
	if token == "" {
		return &HostedStartView{}, nil
	}
	client := hostedrun.New(reg, token)
	if cur := r.hostedCurrent(); cur != nil {
		if cur.registry != reg {
			return &HostedStartView{}, nil
		}
		return r.followedView(ctx, client, cur), nil
	}
	runs, err := client.List(ctx)
	if err != nil {
		// A registry that refuses the list — an old token, or no runs
		// extension — simply has no run to reattach to.
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return &HostedStartView{}, nil
		}
		return nil, err
	}
	active := activeRun(runs)
	if active == nil {
		return &HostedStartView{}, nil
	}
	dir, err := r.attachDir(reg, outDir, active)
	if err != nil {
		return nil, err
	}
	if dir, err = r.follow(reg, token, dir, active); err != nil {
		return nil, err
	}
	return &HostedStartView{Run: hostedRunView(active, dir, nil, "", "", false)}, nil
}

// activeRun is the account's run or chat in flight that a run setup or
// the cards follow, if it has one: a knowledge build in flight is the
// Knowledge panel's (its result is a pack, not outputs in a run folder),
// and a draft in flight the Build tab's (its result is a draft in the
// form).
func activeRun(runs []hostedrun.Run) *hostedrun.Run {
	for i := range runs {
		run := &runs[i]
		if !hostedrun.Active(run.Status) || run.IsBuild() || run.IsDraft() {
			continue
		}
		return run
	}
	return nil
}

// followedView is the run this app already follows, read again.
func (r *RunAPI) followedView(ctx context.Context, client *hostedrun.Client, cur *hostedFollow) *HostedStartView {
	fresh, err := client.Get(ctx, cur.id)
	if err != nil {
		return &HostedStartView{} // already following it; the watch reports its state
	}
	return &HostedStartView{Run: hostedRunView(fresh, cur.outDir, nil, "", "", false)}
}

// attachDir is the folder a run attached to downloads into: the one it
// was started to when this app lost touch with it (claimFollow drops it
// once the run is followed again), else dir as hostedOutDir makes it one of
// the run's own.
func (r *RunAPI) attachDir(reg, dir string, run *hostedrun.Run) (string, error) {
	r.hostedMu.Lock()
	lost, ok := r.lostDirs[lostKey{reg, run.ID}]
	r.hostedMu.Unlock()
	if ok {
		return lost, nil
	}
	return hostedOutDir(dir, run)
}

// lostKey names a run this app lost touch with: the registry that listed
// it and its id.
type lostKey struct{ registry, id string }

// AttachHostedRunAtStart follows this account's run or chat In Agentry
// already in flight when the app starts, so that it is followed in its card
// whether or not a run setup is ever opened (agentry-desktop#466, #467;
// plan desktop-review-key-and-run-cards.md D15). It takes a run or a chat
// and nothing else: a knowledge build is the Knowledge panel's and a draft
// the Build tab's.
// No folder was picked at start, so Go chooses the one the run downloads
// into: the folder it was started to when this app lost touch with it,
// else Run defaults' saved folder, a folder of the run's own inside it
// (D-044), else the default under Downloads (D-030: never a path the page
// names). The card's Follow it again asks the same.
func (r *RunAPI) AttachHostedRunAtStart(reg string) (*HostedStartView, error) {
	reg = settings.Normalize(reg)
	ctx := r.ctx()
	token := settings.TokenFor(reg)
	if settings.ValidateURL(reg) != nil || token == "" {
		return &HostedStartView{}, nil
	}
	client := hostedrun.New(reg, token)
	if cur := r.hostedCurrent(); cur != nil {
		if cur.registry != reg {
			return &HostedStartView{}, nil
		}
		return r.followedView(ctx, client, cur), nil
	}
	runs, err := client.List(ctx)
	if err != nil {
		// A registry that refuses the list, or cannot be reached as the
		// app starts, has no run to attach to now; opening a run setup
		// asks again.
		return &HostedStartView{}, nil //nolint:nilerr // nothing to attach to
	}
	active := activeRun(runs)
	if active == nil {
		return &HostedStartView{}, nil
	}
	saved := ""
	if st, err := settings.Load(); err == nil {
		saved = st.DefaultOutputDir
	}
	dir, err := r.attachDir(reg, saved, active)
	if err != nil {
		return nil, err
	}
	if dir, err = r.follow(reg, token, dir, active); err != nil {
		return nil, err
	}
	return &HostedStartView{Run: hostedRunView(active, dir, nil, "", "", false)}, nil
}

// CancelHostedRun asks the registry to stop the run this app is following.
// Queued, it is cancelled and the hold released; running, the worker
// interrupts the agent and the finish settles what it used.
func (r *RunAPI) CancelHostedRun() (*HostedRunView, error) {
	cur := r.hostedCurrent()
	if cur == nil {
		return nil, fmt.Errorf("no run In Agentry is in progress")
	}
	token := settings.TokenFor(cur.registry)
	if token == "" {
		return nil, fmt.Errorf("sign in to %s to cancel this run", cur.registry)
	}
	res, err := hostedrun.New(cur.registry, token).Cancel(r.ctx(), cur.id)
	if err != nil {
		return nil, err
	}
	return hostedRunView(res, cur.outDir, nil, "", "", false), nil
}

// followHosted watches the run in the background and reports every status
// change and the final result as runner:hosted-run. There is no log to
// forward: a pushed run keeps none (hosted runs in production, D12, D16),
// so the panel shows where the run stands and, at the end, what it
// produced.
func (r *RunAPI) followHosted(reg, token, outDir string, started *hostedrun.Run) (string, *hostedFollow) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &hostedFollow{registry: reg, id: started.ID, ref: started.Ref, outDir: outDir, cancel: cancel, done: make(chan struct{})}
	if cur := r.claimFollow(f); cur != nil {
		cancel()
		return cur.outDir, cur
	}

	client := hostedrun.New(reg, token)
	// Read here, not in the goroutine: a test shortens it for its life.
	poll := hostedPoll
	go func() {
		defer close(f.done)
		defer cancel()
		defer func() {
			r.hostedMu.Lock()
			if r.hosted == f {
				r.hosted = nil
			}
			r.hostedMu.Unlock()
		}()
		r.watchAndDeliver(ctx, client, reg, started, outDir, poll)
	}()
	return outDir, nil
}

// watchAndDeliver re-reads a run until it is over, reporting each move,
// then delivers it. A run the registry stopped answering about while it
// was still moving is said to be so, and nothing is downloaded, rather
// than a run that is not over shown as finished; reopening the panel
// reattaches to it.
func (r *RunAPI) watchAndDeliver(ctx context.Context, client *hostedrun.Client, reg string, started *hostedrun.Run, outDir string, poll time.Duration) {
	final, err := client.Watch(ctx, started.ID, hostedrun.WatchOptions{
		Poll: poll,
		OnStatus: func(run *hostedrun.Run) {
			emitEvent(r.app, eventHostedRun, hostedRunView(run, outDir, nil, "", "", false))
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		last, gerr := client.Get(context.Background(), started.ID)
		if gerr != nil {
			emitEvent(r.app, eventHostedRun, hostedRunView(started, outDir, nil, "", err.Error(), true))
			return
		}
		if !last.Terminal() {
			// A run or a chat is followed again from its card, into the
			// folder it was started to, which this app keeps for it by
			// registry and id.
			r.hostedMu.Lock()
			if r.lostDirs == nil {
				r.lostDirs = map[lostKey]string{}
			}
			r.lostDirs[lostKey{reg, started.ID}] = outDir
			r.hostedMu.Unlock()
			emitEvent(r.app, eventHostedRun, hostedRunView(last, outDir, nil, "",
				"lost touch with the registry while the run was "+hostedrun.State(last)+" ("+err.Error()+"); follow it again to see how it ends", true))
			return
		}
		final = last
	}
	r.deliver(client, final, outDir)
}

// deliver brings a finished run's artefacts down into the folder the person
// picked and reports the run's last word, from which the receipt and the
// result render.
func (r *RunAPI) deliver(client *hostedrun.Client, final *hostedrun.Run, outDir string) {
	got, derr := client.DownloadOutputs(context.Background(), final, outDir)
	// Whatever landed is this run's folder, for the result view. The id
	// is the registry's, so it is remembered only when it is an id.
	if validRunID(final.ID) {
		r.rememberFolder(final.ID, outDir)
	}
	msg := ""
	if derr != nil {
		msg = derr.Error()
	}
	emitEvent(r.app, eventHostedRun, hostedRunView(final, outDir, got, resultText(got), msg, true))
}

// hostedIdle refuses a second hosted run from this app while one is being
// followed — the registry allows one active run per account anyway (D11),
// and this says so before the upload rather than after it.
func (r *RunAPI) hostedIdle() error {
	if r.hostedCurrent() != nil {
		return fmt.Errorf("a run In Agentry is already in progress — wait for it to finish, or cancel it")
	}
	return nil
}

func (r *RunAPI) hostedCurrent() *hostedFollow {
	r.hostedMu.Lock()
	defer r.hostedMu.Unlock()
	return r.hosted
}

// ctx is the app's lifecycle context, or a background one in tests.
func (r *RunAPI) ctx() context.Context {
	if r.app != nil && r.app.ctx != nil {
		return r.app.ctx
	}
	return context.Background()
}

// refusalView renders a refusal the way the Build page renders the
// allowance's: a slug, a sentence, and — for credits — the way onward.
func (r *RunAPI) refusalView(refusal *hostedrun.Refusal) *HostedStartView {
	v := &HostedStartView{
		Refusal: refusal.Slug, Detail: refusal.Sentence(),
		Balance: refusal.Balance, Hold: refusal.Hold, ResetsAt: refusal.ResetsAt,
		Missing: refusal.Missing,
	}
	// No destination here: the panel asks SettingsAPI.OpenUpgrade for it
	// at the moment a person clicks, so the registry's answer is checked
	// against its own origin before anything opens (agentry-enterprise#41).
	// Carrying the URL in the view would put an unchecked one in the front
	// end for nobody to use.
	return v
}

// hostedOutDir is the folder a hosted run's artefacts land in: the one the
// person picked, a folder of its own inside Run defaults' saved one, or the
// same default a local run would have written to.
func hostedOutDir(dir string, r *hostedrun.Run) (string, error) {
	// The registry names the run, and the name becomes part of a folder's
	// name here: one that is not an id (a "../", a ".app", a comma) is
	// refused before any folder is made from it.
	if !validRunID(r.ID) {
		return "", fmt.Errorf("the registry named this run %q, which is not a run id", r.ID)
	}
	ns, name := "", ""
	if ref, err := manifest.ParseRef(r.Ref); err == nil {
		ns, name = ref.Namespace, ref.Name
	}
	// Run defaults' saved folder is every run's, so a run In Agentry lands
	// in a folder of its own inside it, as a local run does (D-044).
	if isSavedOutputDir(dir) {
		dir = run.RunFolder(dir, ns, name, r.ID)
	}
	return run.OutputDir(dir, ns, name, r.ID)
}

// resultText finds result.json among the downloaded artefacts and returns
// its bytes, which the panel renders exactly as a local run's result,
// without the envelope's token counts (resultWithoutTokens).
func resultText(got []hostedrun.Downloaded) string {
	for _, a := range got {
		if a.Name != "result.json" {
			continue
		}
		b, err := os.ReadFile(a.Path) //nolint:gosec // a path this app just wrote under the run folder
		if err != nil {
			return ""
		}
		return resultWithoutTokens(b)
	}
	return ""
}

// resultWithoutTokens is a hosted run's result.json as the page gets it.
// On a sandbox host that file is the CLI's run/v1 envelope, whose usage
// block carries the run's token counts and their dollar cost; a run In
// Agentry is stated in credits, so they are taken out before the page sees
// it (agentry-desktop#457). The file in the run folder keeps them, as a
// downloaded receipt does (plan scan-and-run-tweaks.md D11). Anything that
// is not a JSON object is passed on as it is: it carries no usage block.
func resultWithoutTokens(b []byte) string {
	var env map[string]json.RawMessage
	if json.Unmarshal(b, &env) != nil || env["usage"] == nil {
		return string(b)
	}
	var usage map[string]json.RawMessage
	if json.Unmarshal(env["usage"], &usage) != nil {
		delete(env, "usage")
	} else {
		for k := range usage {
			if k != "turns" && k != "wallMs" {
				delete(usage, k)
			}
		}
		env["usage"], _ = json.Marshal(usage)
	}
	out, err := json.Marshal(env)
	if err != nil {
		return ""
	}
	return string(out)
}

// hostedRunView flattens a run for the panel.
func hostedRunView(r *hostedrun.Run, outDir string, got []hostedrun.Downloaded, result, downloadErr string, final bool) *HostedRunView {
	if r == nil {
		return nil
	}
	verdict := hostedrun.Read(r)
	v := &HostedRunView{
		ID: r.ID, Ref: r.Ref, Digest: r.Digest, Status: r.Status, State: hostedrun.State(r),
		StatusLine: verdict.Line, QueuePosition: r.QueuePosition, CancelRequested: r.CancelRequested,
		Outcome: r.Outcome, ExitClass: r.ExitClass, Error: r.Error,
		HoldCredits: r.HoldCredits,
		Caps:        hostedCaps(r.Caps),
		OutDir:      outDir, Outputs: []HostedArtefactView{}, ResultJSON: result,
		DownloadError: downloadErr, Final: final,
		CreatedAt: rfc3339(r.CreatedAt),
		Mode:      r.Mode, Task: r.Task, Session: sessionView(r.Session),
	}
	if r.StartedAt != nil {
		v.StartedAt = rfc3339(*r.StartedAt)
	}
	if r.Status == hostedrun.StatusFinished || r.Status == hostedrun.StatusCancelled {
		v.OutcomeLine, v.OutcomeTone = verdict.Line, verdict.Tone
		if v.Outcome == "" {
			v.Outcome = verdict.Outcome
		}
	}
	if r.FinishedAt != nil {
		v.FinishedAt = rfc3339(*r.FinishedAt)
	}
	if u := r.Usage; u != nil {
		v.Turns, v.WallMS = u.Turns, u.WallMS
	}
	if r.SettledCredits != nil {
		v.SettledCredits, v.Settled = *r.SettledCredits, true
	}
	v.ReceiptKept = !r.Receipt.Empty()
	v.CapabilitiesEnforced = r.Receipt.CapabilitiesEnforced
	v.KnowledgeLine = r.Receipt.CapabilitiesEnforced["knowledge"]
	for _, k := range r.Knowledge {
		v.Knowledge = append(v.Knowledge, HostedPackView{Name: k.Name, Digest: k.Digest})
	}
	v.AttestationState = r.Receipt.Attestation.State
	v.Adapter = r.Receipt.Harness.Adapter
	if len(got) == 0 {
		if downloadErr != "" && final {
			// Nothing came down: listing what the registry holds would read
			// as saved. The error says what happened instead.
			return v
		}
		for _, a := range r.Outputs {
			v.Outputs = append(v.Outputs, HostedArtefactView{Name: a.Name, Bytes: a.Bytes})
		}
		return v
	}
	for _, a := range got {
		v.Outputs = append(v.Outputs, HostedArtefactView{Name: a.Name, Bytes: a.Bytes, Path: a.Path})
	}
	return v
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// extensionsOf reads a registry's /meta extension list — the way the
// account surface finds the billing extension.
func extensionsOf(ctx context.Context, url string) ([]string, error) {
	client, err := registry.New(url, "")
	if err != nil {
		return nil, err
	}
	meta, err := client.GetMetaWithResponse(ctx)
	if err != nil || meta.JSON200 == nil || meta.JSON200.Extensions == nil {
		return nil, fmt.Errorf("read %s/api/v1/meta", strings.TrimRight(url, "/"))
	}
	return *meta.JSON200.Extensions, nil
}

// tokenScopes reports what the stored token may do, read from the
// account's own token list and matched by the token's display prefix —
// never by making a request to a runs route to see whether it 403s.
func tokenScopes(ctx context.Context, url, token string) ([]string, bool) {
	client, err := registry.New(url, token)
	if err != nil {
		return nil, false
	}
	res, err := client.ListTokensWithResponse(ctx)
	if err != nil || res.StatusCode() != http.StatusOK || res.JSON200 == nil {
		return nil, false
	}
	prefix := token
	if len(prefix) > patDisplayPrefixLen {
		prefix = prefix[:patDisplayPrefixLen]
	}
	for _, p := range *res.JSON200 {
		if p.Prefix != prefix {
			continue
		}
		scopes := make([]string, 0, len(p.Scopes))
		for _, s := range p.Scopes {
			scopes = append(scopes, string(s))
		}
		return scopes, true
	}
	return nil, false
}

// patDisplayPrefixLen is the core's PAT display prefix (schemas/openapi
// `Pat.prefix`: "First 12 characters").
const patDisplayPrefixLen = 12

func hasString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
