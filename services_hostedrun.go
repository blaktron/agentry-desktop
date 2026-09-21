// In Agentry: a hosted run of a published release, started with the app's
// token, streamed from the registry, its outputs and receipt downloaded
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
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// eventHostedRun carries the hosted run's state to the panel; the log
// itself rides runner:run-log, the event the local engine feeds, so one
// view shows either kind of run.
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

// hostedFollow is the In-Agentry run this app is following. RunAPI's
// hostedMu guards the pointer to it; the fields are written once, at the
// start of the follow, and read after.
type hostedFollow struct {
	registry string
	id       string
	outDir   string
	cancel   context.CancelFunc
	done     chan struct{}
}

// HostedCapsView is a tier's ceilings for one run.
type HostedCapsView struct {
	TimeoutSeconds int   `json:"timeoutSeconds"`
	MaxTurns       int   `json:"maxTurns"`
	MaxTokens      int64 `json:"maxTokens"`
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

	SignedIn      bool `json:"signedIn"`
	Extension     bool `json:"extension"`
	Enabled       bool `json:"enabled"`
	WorkersOnline int  `json:"workersOnline"`
	Queued        int  `json:"queued"`

	// Tier, HoldCredits, TokensPerCredit and Caps are the registry's
	// numbers for a run of this account's; the app does no arithmetic of
	// its own with them.
	Tier            string          `json:"tier,omitempty"`
	HoldCredits     int64           `json:"holdCredits"`
	TokensPerCredit int64           `json:"tokensPerCredit"`
	Caps            *HostedCapsView `json:"caps,omitempty"`
	// Balance is the account's credits; BalanceKnown is false when the
	// registry has no billing extension or the read failed.
	Balance      int64 `json:"balance"`
	BalanceKnown bool  `json:"balanceKnown"`

	MaxInputFiles int   `json:"maxInputFiles"`
	MaxInputBytes int64 `json:"maxInputBytes"`
}

// HostedArtefactView is one artefact of a run and, once downloaded, where
// it landed on this machine.
type HostedArtefactView struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
	Path  string `json:"path,omitempty"`
}

// HostedRunView is one hosted run as the panel shows it: the identity and
// status while it moves, and the §R8 result once it is over — the same
// facts a local run's result view carries, plus the credits.
type HostedRunView struct {
	ID     string `json:"id"`
	Ref    string `json:"ref"`
	Digest string `json:"digest"`
	Status string `json:"status"`
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

	Turns        int     `json:"turns"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	TotalTokens  int64   `json:"totalTokens"`
	CostUSD      float64 `json:"costUsd"`
	WallMS       int64   `json:"wallMs"`

	// HoldCredits is what the run holds; SettledCredits what it cost once
	// it finished, both as the account reports them. Settled says the
	// settlement has been reported.
	HoldCredits    int64 `json:"holdCredits"`
	SettledCredits int64 `json:"settledCredits"`
	Settled        bool  `json:"settled"`

	// CapabilitiesEnforced is the receipt's block, listed verbatim.
	CapabilitiesEnforced map[string]string `json:"capabilitiesEnforced,omitempty"`
	AttestationState     string            `json:"attestationState,omitempty"`
	Adapter              string            `json:"adapter,omitempty"`

	// ResultJSON is result.json as the run wrote it, rendered by the same
	// component a local run's result is.
	ResultJSON string `json:"resultJson,omitempty"`
	// OutDir is the folder the artefacts landed in; Outputs what landed.
	OutDir  string               `json:"outDir,omitempty"`
	Outputs []HostedArtefactView `json:"outputs"`
	// DownloadError says why an artefact did not land, when one did not.
	DownloadError string `json:"downloadError,omitempty"`

	Caps       *HostedCapsView `json:"caps,omitempty"`
	CreatedAt  string          `json:"createdAt,omitempty"`
	FinishedAt string          `json:"finishedAt,omitempty"`
	// Final says this is the last word on the run: it is over and its
	// artefacts are downloaded.
	Final bool `json:"final"`
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
	if v.SignedIn {
		exts, err := extensionsOf(ctx, reg)
		if err == nil {
			facts.Extension = hasString(exts, "runs")
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

	facts.Scopes, facts.ScopesKnown = tokenScopes(ctx, reg, token)

	client := hostedrun.New(reg, token)
	if st, err := client.Status(ctx); err == nil {
		facts.Enabled, facts.StatusKnown = st.Enabled, true
		v.Enabled, v.WorkersOnline, v.Queued = st.Enabled, st.WorkersOnline, st.Queued
		if st.Caller != nil {
			v.Tier, v.HoldCredits, v.TokensPerCredit = st.Caller.Tier, st.Caller.HoldCredits, st.Caller.TokensPerCredit
			v.Caps = &HostedCapsView{TimeoutSeconds: st.Caller.Caps.TimeoutSeconds, MaxTurns: st.Caller.Caps.MaxTurns, MaxTokens: st.Caller.Caps.MaxTokens}
		}
	}
	if sub, err := fetchSubscription(ctx, reg, token); err == nil && sub != nil {
		v.Balance, v.BalanceKnown = sub.Balance, true
	}

	o := hostedrun.Decide(facts)
	v.Offered, v.Available, v.Reason, v.Remedy = o.Offered, o.Available, o.Reason, o.Remedy
	return v, nil
}

// StartHostedRun uploads the run's input files, creates the run and starts
// following it. The panel has shown the declared contract and collected
// the intake answers first: nothing is created until Run is clicked.
func (r *RunAPI) StartHostedRun(reg, ref, task string, inputPaths []string, outDir string, intakeAnswers map[string]string) (*HostedStartView, error) {
	reg = settings.Normalize(reg)
	ctx := r.ctx()
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
	if err := r.app.requirePicked("output directory", outDir); err != nil {
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
	files, err := hostedrun.StageInputs(inputPaths)
	if err != nil {
		return nil, err
	}
	client := hostedrun.New(reg, token)
	logw := r.eventWriter(eventRunLog)
	defer logw.Flush()

	ids, err := client.UploadInputs(ctx, files, func(line string) { _, _ = fmt.Fprintln(logw, line) })
	if err != nil {
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return r.refusalView(refusal), nil
		}
		return nil, err
	}
	created, err := client.Create(ctx, hostedrun.CreateRequest{Ref: ref, Task: task, Intake: intakeAnswers, Inputs: ids})
	if err != nil {
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return r.refusalView(refusal), nil
		}
		return nil, err
	}
	dir, err := hostedOutDir(outDir, created)
	if err != nil {
		return nil, err
	}
	_, _ = fmt.Fprintf(logw, "run %s queued In Agentry — %s, holding %d credits\n", created.ID, created.Ref, created.HoldCredits)
	r.followHosted(reg, token, dir, created)
	return &HostedStartView{Run: hostedRunView(created, dir, nil, "", "", false)}, nil
}

// AttachHostedRun reattaches to this account's active run, if there is
// one, rather than starting a second (one run per account, D11). It
// returns a view with no run when the account has none.
func (r *RunAPI) AttachHostedRun(reg, outDir string) (*HostedStartView, error) {
	reg = settings.Normalize(reg)
	ctx := r.ctx()
	token := settings.TokenFor(reg)
	if token == "" {
		return &HostedStartView{}, nil
	}
	if cur := r.hostedCurrent(); cur != nil {
		fresh, err := hostedrun.New(reg, token).Get(ctx, cur.id)
		if err != nil {
			return &HostedStartView{}, nil //nolint:nilerr // already following it; the stream reports its state
		}
		return &HostedStartView{Run: hostedRunView(fresh, cur.outDir, nil, "", "", false)}, nil
	}
	client := hostedrun.New(reg, token)
	runs, err := client.List(ctx)
	if err != nil {
		// A registry that refuses the list — an old token, or no runs
		// extension — simply has no run to reattach to.
		if refusal := hostedrun.RefusalOf(err); refusal != nil {
			return &HostedStartView{}, nil
		}
		return nil, err
	}
	for i := range runs {
		active := &runs[i]
		if !hostedrun.Active(active.Status) {
			continue
		}
		dir, err := hostedOutDir(outDir, active)
		if err != nil {
			return nil, err
		}
		r.followHosted(reg, token, dir, active)
		return &HostedStartView{Run: hostedRunView(active, dir, nil, "", "", false)}, nil
	}
	return &HostedStartView{}, nil
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

// followHosted streams the run in the background: every log line as
// runner:run-log (the view a local run feeds), every status change and the
// final result as runner:hosted-run.
func (r *RunAPI) followHosted(reg, token, outDir string, started *hostedrun.Run) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &hostedFollow{registry: reg, id: started.ID, outDir: outDir, cancel: cancel, done: make(chan struct{})}
	r.hostedMu.Lock()
	r.hosted = f
	r.hostedMu.Unlock()

	client := hostedrun.New(reg, token)
	logw := r.eventWriter(eventRunLog)
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
		final, err := client.Follow(ctx, started.ID, hostedrun.FollowOptions{
			OnEvent: func(ev hostedrun.Event) {
				if ev.Line == "" {
					return
				}
				_, _ = fmt.Fprintln(logw, ev.Line)
			},
			OnStatus: func(run *hostedrun.Run) {
				emitEvent(r.app, eventHostedRun, hostedRunView(run, outDir, nil, "", "", false))
			},
		})
		logw.Flush()
		if err != nil {
			last, gerr := client.Get(context.Background(), started.ID)
			if gerr != nil {
				emitEvent(r.app, eventHostedRun, hostedRunView(started, outDir, nil, "", err.Error(), true))
				return
			}
			final = last
		}
		// The run is over: its artefacts come down into the folder the
		// person picked, and the receipt and result render from there.
		got, derr := client.DownloadOutputs(context.Background(), final, outDir)
		msg := ""
		if derr != nil {
			msg = derr.Error()
		}
		emitEvent(r.app, eventHostedRun, hostedRunView(final, outDir, got, resultText(got), msg, true))
	}()
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
	}
	// No destination here: the panel asks SettingsAPI.OpenUpgrade for it
	// at the moment a person clicks, so the registry's answer is checked
	// against its own origin before anything opens (agentry-enterprise#41).
	// Carrying the URL in the view would put an unchecked one in the front
	// end for nobody to use.
	return v
}

// hostedOutDir is the folder a hosted run's artefacts land in: the one the
// person picked, or the same default a local run would have written to.
func hostedOutDir(dir string, r *hostedrun.Run) (string, error) {
	ns, name := "", ""
	if ref, err := manifest.ParseRef(r.Ref); err == nil {
		ns, name = ref.Namespace, ref.Name
	}
	return run.OutputDir(dir, ns, name, r.ID)
}

// resultText finds result.json among the downloaded artefacts and returns
// its bytes, which the panel renders exactly as a local run's result.
func resultText(got []hostedrun.Downloaded) string {
	for _, a := range got {
		if a.Name != "result.json" {
			continue
		}
		b, err := os.ReadFile(a.Path) //nolint:gosec // a path this app just wrote under the run folder
		if err != nil {
			return ""
		}
		return string(b)
	}
	return ""
}

// hostedRunView flattens a run for the panel.
func hostedRunView(r *hostedrun.Run, outDir string, got []hostedrun.Downloaded, result, downloadErr string, final bool) *HostedRunView {
	if r == nil {
		return nil
	}
	verdict := hostedrun.Read(r)
	v := &HostedRunView{
		ID: r.ID, Ref: r.Ref, Digest: r.Digest, Status: r.Status,
		StatusLine: verdict.Line, QueuePosition: r.QueuePosition, CancelRequested: r.CancelRequested,
		Outcome: r.Outcome, ExitClass: r.ExitClass, Error: r.Error,
		HoldCredits: r.HoldCredits,
		Caps:        &HostedCapsView{TimeoutSeconds: r.Caps.TimeoutSeconds, MaxTurns: r.Caps.MaxTurns, MaxTokens: r.Caps.MaxTokens},
		OutDir:      outDir, Outputs: []HostedArtefactView{}, ResultJSON: result,
		DownloadError: downloadErr, Final: final,
		CreatedAt: rfc3339(r.CreatedAt),
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
		v.Turns, v.InputTokens, v.OutputTokens = u.Turns, u.InputTokens, u.OutputTokens
		v.TotalTokens, v.CostUSD, v.WallMS = u.Tokens(), u.CostUSD, u.WallMS
	}
	if r.SettledCredits != nil {
		v.SettledCredits, v.Settled = *r.SettledCredits, true
	}
	v.CapabilitiesEnforced = r.Receipt.CapabilitiesEnforced
	v.AttestationState = r.Receipt.Attestation.State
	v.Adapter = r.Receipt.Harness.Adapter
	if len(got) == 0 {
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
