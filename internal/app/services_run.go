// RunAPI: the bound run surface — setup status and one-click setup,
// MCP-profile status and guided configuration as a form, the tool-surface
// section, and run-with-task over agentry-cli/pkg/runner. Long operations
// stream progress as Wails events (`runner:setup-progress`,
// `runner:unsafe-progress`, a chat's `runner:run-log`, a run's
// `runner:run-progress` and `runner:run-line`); secret VALUES cross this
// boundary only inward (SaveProfile) and are never returned or logged.
package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/session"

	"github.com/blaktron/agentry-desktop/internal/dockerimages"
	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/mcpview"
	"github.com/blaktron/agentry-desktop/internal/mcpvuln"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/voice"
)

// Event names the frontend subscribes to.
const (
	eventSetupProgress = "runner:setup-progress"
	// eventUnsafeProgress carries the Unsafe Mode image's pull to the Run
	// defaults card (services_unsafe.go).
	eventUnsafeProgress = "runner:unsafe-progress"
	// eventUnsafeLive says a run or chat compiled in Unsafe Mode has become
	// live (true) or has ended (false): the shell keeps the red edge and the
	// badge up while one is, switch or no switch (services_unsafe.go).
	eventUnsafeLive = "runner:unsafe-live"
	eventRunLog     = "runner:run-log"
	// eventSessionEvent carries an interactive session's events (JSON
	// SessionEventView) to the chat view; runner:run-log carries a chat's
	// setup log, and a run's log goes by its id (runner:run-line).
	eventSessionEvent = "runner:session-event"
)

// errBusy refuses a docker-touching operation other than a run while
// another job holds the run lock, instead of silently queueing it behind one
// that can run for many minutes; a run waits its turn in the queue instead
// (services_runqueue.go). The refusal a person sees names the job holding
// the lock (busyError, services_runflow.go); errBusy is what it is for
// errors.Is.
var errBusy = errors.New("another job is under way on this computer, and one job runs here at a time")

// RunAPI drives QuickAgent runs from the app.
type RunAPI struct {
	app *App
	// mu serializes the docker-touching operations: one run (or image setup)
	// at a time keeps the log streams and the engine's teardown sweeps
	// unambiguous. An interactive session holds it from StartSession to
	// EndSession (one session per app).
	mu sync.Mutex
	// images lists and removes Docker images (Run defaults' Docker images);
	// nil is the docker CLI. A test passes its own.
	images dockerimages.Docker
	// session is the live interactive session, nil otherwise.
	session localSession
	// knowledgeEnv is the local knowledge store's wiring; nil is this
	// app's (internal/knowledge.Default). A test passes its own.
	knowledgeEnv *knowledge.Env
	// hosted is the In-Agentry run this app is following (services_
	// hostedrun.go), under its own lock: a hosted run touches no docker,
	// so it must not wait on — or be waited on by — the run lock above.
	hostedMu sync.Mutex
	hosted   *hostedFollow
	// lostDirs are the folders of runs In Agentry this app lost touch with
	// while they were still moving, by run id: following one again brings
	// its outputs down where it was started to.
	lostDirs map[lostKey]string
	// held are the secrets this app set for a start In Agentry and did not
	// keep, by registry: deleted again if the start does not start, left for
	// the vault to remove when the run ends if it does. Names only.
	heldMu sync.Mutex
	held   map[string]map[string]bool
	// awaiting is a start In Agentry refused 409 connection-missing, kept so
	// it can be made again once the vault holds the connection, with the
	// inputs it already uploaded (services_hostedrun_connect.go).
	awaitMu  sync.Mutex
	awaiting *awaitingStart

	// holder is what holds mu, for the busy refusal and InFlight
	// (services_runflow.go), under its own lock.
	holderMu sync.Mutex
	holder   *jobHolder
	// queue is the runs waiting for mu, in the order they were started
	// (services_runqueue.go): release hands mu to the first of them.
	// Taken before holderMu. queueClosed is set as the app quits, when
	// nothing more waits.
	queueMu     sync.Mutex
	queue       []*queuedRun
	queueClosed bool
	// queueEmitMu orders the queue's reports, so the page never hears an
	// older one after a newer.
	queueEmitMu sync.Mutex

	// live is the run or session in flight, so quitting can end it and sweep
	// what it built (services_quit.go).
	liveMu sync.Mutex
	live   *liveRun
	// unsafeMu serializes a live run's Unsafe Mode flag with the event that
	// tells the shell, so the shell never hears them out of order
	// (setUnsafeLive, endRun). Taken before liveMu.
	unsafeMu sync.Mutex
	// Engine seams for tests; nil means internal/run.
	newRunID      func() (string, error)
	engineRun     func(context.Context, run.Request, io.Writer) (*run.Result, error)
	engineSession func(context.Context, run.Request, io.Writer) (localSession, error)
	sweep         func(context.Context, string) error
	// runnerHomeDir is the runner home a test gives the app, "" for this
	// machine's: where the crash-path records are kept, in its desktop-runs
	// (services_quit.go), and where a run's files are looked for when it is
	// settled and swept.
	runnerHomeDir string

	// Unsafe Mode's seams (services_unsafe.go); nil means internal/run.
	pullUnsafe   func(context.Context, io.Writer) error
	unsafeNotice func(context.Context, string, string) (*run.UnsafeNotice, error)

	// folders are the run folders of the results Go produced, by run id,
	// for the result view's two bindings (services_outputs.go); openFolder
	// is the file-manager seam a test substitutes.
	foldersMu sync.Mutex
	folders   map[string]string
	// localRuns are the runs this computer's engine ran, by run id.
	localRuns map[string]bool
	// downloading holds the runs In Agentry whose Download output is under
	// way, so a second click is refused (services_history.go).
	downloading sync.Map
	openFolder  func(dir string) error
	// openFile opens a PDF copy in the system viewer and saveFile asks
	// where to save one (services_pdf.go); nil are the OS's. pdfDir holds
	// the copies OpenRunPDF made, removed at quit.
	openFile func(path string) error
	saveFile func(name string) (string, error)
	// revealFile shows a deliverable's copy in the file manager
	// (services_deliverables.go); nil is the OS's.
	revealFile func(path string) error
	pdfMu      sync.Mutex
	pdfDir     string
	// openBrowser opens a consent address (services_connections.go); nil
	// is the system browser. A test substitutes its own.
	openBrowser func(url string) error
}

// localSession is the engine's interactive session on this computer as the
// app drives it (*run.Session); a test substitutes its own, as it does the
// engine's run.
type localSession interface {
	Ref() string
	Events() <-chan session.Event
	Send(ctx context.Context, text string) error
	AddInput(ctx context.Context, path string) error
	End(ctx context.Context, finish bool) (*run.Result, error)
}

// SessionView is what StartSession returns.
type SessionView struct {
	Ref string `json:"ref"`
}

// SessionEventView is one runner:session-event payload — the session's
// event, flattened for the chat view.
type SessionEventView struct {
	// Seq is a hosted session's frame sequence number (0 on this
	// machine): a chat that replays a session keeps each one once.
	Seq       uint64 `json:"seq,omitempty"`
	Kind      string `json:"kind"`
	Time      string `json:"time"`
	Text      string `json:"text,omitempty"`
	Synthetic bool   `json:"synthetic,omitempty"`
	// Task is, on a persistent agent's user turn that its heartbeat sent,
	// the id of the task: the turn is the agent's own, not the person's.
	Task string `json:"task,omitempty"`
	// Upkeep is, on a persistent agent's user turn, the runner's mark of
	// an upkeep turn (a compaction or a memory tidy): the turn is the
	// runner's, not the person's.
	Upkeep     bool   `json:"upkeep,omitempty"`
	StopReason string `json:"stopReason,omitempty"`
	Path       string `json:"path,omitempty"`
	Digest     string `json:"digest,omitempty"`
	Reason     string `json:"reason,omitempty"`
	// InputTokens, OutputTokens, TotalTokens and CostUSD are a usage
	// event's, from a session on this computer only; a session In Agentry's
	// frames carry none.
	InputTokens  int64   `json:"inputTokens,omitempty"`
	OutputTokens int64   `json:"outputTokens,omitempty"`
	TotalTokens  int64   `json:"totalTokens,omitempty"`
	CostUSD      float64 `json:"costUsd,omitempty"`
}

// eventView flattens a session event for the chat view.
func eventView(ev session.Event) SessionEventView {
	return SessionEventView{
		Kind: string(ev.Kind), Time: ev.Time.Format(time.RFC3339Nano), Text: ev.Text, Synthetic: ev.Synthetic, Task: ev.Task, Upkeep: ev.Upkeep,
		StopReason: ev.StopReason, Path: ev.Path, Digest: ev.Digest, Reason: string(ev.Reason),
		InputTokens: ev.InputTokens, OutputTokens: ev.OutputTokens, TotalTokens: ev.TotalTokens, CostUSD: ev.CostUSD,
	}
}

// StartSession starts an interactive run: the same install, consent-recorded
// contract, images, profile and intake as RunAgent, then the engine's
// session; events stream as runner:session-event until EndSession. The run
// lock is held for the session's whole life.
//
// A chat takes no timeout from the page: it has no wall clock and no count
// of the person's turns, and its one limit is Run defaults' Chat idle
// timeout, none by default, which Go reads here from desktop.yaml. maxTurns
// is the tool-call budget, as a run's: -1 keeps the policy's, 0 lifts it.
func (r *RunAPI) StartSession(reg, ref, task string, inputPaths []string, outDir string, keepProfile *bool, maxTurns int, acceptUnsigned bool, intakeAnswers map[string]string, allowVulnerableMCP, acceptUnscanned bool, knowledge []string, writeInVoice bool, sourceTokens map[string]string, hostSignIns map[string]run.HostSignIn, destinations []string) (*SessionView, error) {
	idle, err := chatIdleTimeout()
	if err != nil {
		return nil, err
	}
	// The switch is read once: the dialog that carries its line and the
	// compile that obeys it see the same answer, whatever is turned on or
	// off while the dialog is up.
	unsafe := unsafeModeOn()
	if err := r.localRunGates(reg, ref, inputPaths, outDir, acceptUnsigned, allowVulnerableMCP, acceptUnscanned, unsafe); err != nil {
		return nil, err
	}
	if err := r.acquire(jobChat, "a chat with "+ref); err != nil {
		return nil, err
	}
	// The session's own context, the parent of everything the engine
	// starts for it: quitting while the session is still starting cancels
	// it (services_quit.go); EndSession releases it after End.
	ctx, cancel := context.WithCancel(r.app.ctx)
	live, err := r.beginRun(jobChat, ref, cancel)
	if err != nil {
		cancel()
		r.release()
		return nil, err
	}
	live.registry = reg
	// A chat that read the switch on is an Unsafe Mode chat from here until
	// its compile says otherwise.
	r.setUnsafeLive(live, unsafe)
	// The attached packs' opened copies live as long as the session: its
	// endRun, from EndSession or quitting, removes them.
	packs, err := r.attach(ctx, live, reg, knowledge)
	var voiced *engine.AttachedVoice
	if err == nil {
		voiced, err = r.voiceFor(ctx, reg, writeInVoice)
	}
	if err != nil {
		cancel()
		r.endRun(live)
		r.release()
		return nil, err
	}
	logw := r.eventWriter(eventRunLog)
	s, err := r.startEngineSession(ctx, run.Request{
		RunID:                  live.id,
		InFlightDir:            r.inFlightDir(),
		Registry:               reg,
		Ref:                    ref,
		Task:                   task,
		InputPaths:             inputPaths,
		Knowledge:              packs,
		Voice:                  voiced,
		ReportTheme:            r.reportThemeFor(ctx, reg, logw),
		OutDir:                 outDir,
		OutDirIsRoot:           isSavedOutputDir(outDir),
		DeliverTo:              deliverFolder(),
		Destinations:           destinations,
		KeepProfile:            keepProfile,
		MaxTurns:               maxTurns,
		ChatIdleTimeoutSeconds: idle,
		AcceptUnsigned:         acceptUnsigned,
		AcceptUnscanned:        acceptUnscanned,
		AllowVulnerableMCP:     allowVulnerableMCP,
		Intake:                 intakeAnswers,
		SourceTokens:           sourceTokens,
		HostSignIns:            hostSignIns,
		TrustHostKey:           r.app.trustHostKey,
		KeyFilePicked:          r.app.keyFiles.require,
		PathPicked:             r.app.requirePicked,
		Unsafe:                 unsafe,
		PlanCompiled:           func(u bool) { r.setUnsafeLive(live, u) },
	}, logw)
	logw.Flush()
	if err != nil {
		cancel()
		r.endRun(live)
		r.release()
		return nil, err
	}
	r.session = s
	if savesHostSignIn(hostSignIns) {
		resealSchedules(r.app)
	}
	go func() {
		for ev := range s.Events() {
			runtime.EventsEmit(r.app.ctx, eventSessionEvent, eventView(ev))
		}
	}()
	return &SessionView{Ref: s.Ref()}, nil
}

// errNoSession says nothing is running interactively.
var errNoSession = errors.New("no interactive session is running")

// SendTurn delivers one user message and returns when the agent's turn ends.
func (r *RunAPI) SendTurn(text string) error {
	s := r.session
	if s == nil {
		return errNoSession
	}
	return s.Send(r.app.ctx, text)
}

// AddSessionInput gives the agent one more file, read-only under /inputs.
func (r *RunAPI) AddSessionInput(path string) error {
	s := r.session
	if s == nil {
		return errNoSession
	}
	return s.AddInput(r.app.ctx, path)
}

// EndSession ends the session (finish = ask for the final result first),
// writes receipts and the envelope, tears the sandbox down, and releases
// the run lock. Calling it after the engine ended the session itself
// returns the same result rather than failing.
func (r *RunAPI) EndSession(finish bool) (*RunResultView, error) {
	s := r.session
	if s == nil {
		return nil, errNoSession
	}
	res, err := s.End(r.app.ctx, finish)
	r.session = nil
	if l := r.current(); l != nil {
		// A chat's end is how the person ends it, not a stop: its outcome
		// is the engine's.
		recordLocal(context.Background(), runhistory.KindChat, l, s.Ref(), res)
		l.cancel()
		r.endRun(l)
	}
	r.release()
	if err != nil {
		return nil, err
	}
	return r.resultView(res), nil
}

// SetupStatusView is the machine's run-readiness.
type SetupStatusView struct {
	DockerOK  bool   `json:"dockerOk"`
	DockerErr string `json:"dockerErr,omitempty"`
	// Engine is the container engine detection found, and what it saw.
	Engine       EngineView `json:"engine"`
	PolicyPath   string     `json:"policyPath"`
	PolicyExists bool       `json:"policyExists"`
	// ProviderSetUp: the policy's model block is not the default's, so
	// somebody chose a provider, a model, a key source or an endpoint; a
	// policy another step wrote from defaults is not that.
	ProviderSetUp bool   `json:"providerSetUp"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	APIKeyEnv     string `json:"apiKeyEnv"`
	// APIKeySource is "keyring" when the OS keychain is the fallback for an
	// unset variable; KeysStored which of the providers the form can show
	// have a key in it, so the badge follows the provider chosen and not the
	// policy's; KeyStoreErr why the keychain could not be asked. No value,
	// ever.
	APIKeySource string   `json:"apiKeySource,omitempty"`
	KeysStored   []string `json:"keysStored"`
	KeyStoreErr  string   `json:"keyStoreErr,omitempty"`
	// APIKeyKind is "subscription" for a stored Anthropic subscription
	// token, and KeyNotice the sentence to show beside the field for it;
	// both empty for an API key.
	APIKeyKind  string `json:"apiKeyKind,omitempty"`
	KeyNotice   string `json:"keyNotice,omitempty"`
	BaseURL     string `json:"baseUrl,omitempty"`
	ImagesReady bool   `json:"imagesReady"`
	// Policy budget defaults, shown as the run form's placeholders.
	TimeoutSeconds int `json:"timeoutSeconds"`
	MaxTurns       int `json:"maxTurns"`
	// KeepProfilesByDefault seeds keep toggles when no profile exists.
	KeepProfilesByDefault bool `json:"keepProfilesByDefault"`
}

func setupStatusView(s *run.Status) *SetupStatusView {
	return &SetupStatusView{
		DockerOK:              s.DockerOK,
		DockerErr:             s.DockerErr,
		Engine:                engineView(s.Engine),
		PolicyPath:            s.PolicyPath,
		PolicyExists:          s.PolicyExists,
		ProviderSetUp:         s.ProviderSetUp,
		Provider:              s.Provider,
		Model:                 s.Model,
		APIKeyEnv:             s.APIKeyEnv,
		APIKeySource:          s.APIKeySource,
		KeysStored:            emptyNotNil(s.KeysStored),
		KeyStoreErr:           s.KeyStoreErr,
		APIKeyKind:            s.APIKeyKind,
		KeyNotice:             s.KeyNotice,
		BaseURL:               s.BaseURL,
		ImagesReady:           s.ImagesReady,
		TimeoutSeconds:        s.TimeoutSeconds,
		MaxTurns:              s.MaxTurns,
		KeepProfilesByDefault: s.KeepProfilesByDefault,
	}
}

// SetupStatus reports docker availability, the configured LLM provider,
// and whether the gateway/harness/runner images are present. Read-only:
// it never creates the policy file.
func (r *RunAPI) SetupStatus() (*SetupStatusView, error) {
	s, err := run.GetStatus(r.app.ctx)
	if err != nil {
		return nil, err
	}
	return setupStatusView(s), nil
}

// ApplySetup writes the LLM provider preference into the operator policy
// (the same policy.yaml the CLI reads) and optionally prepares the gateway
// plane now. Image pulls/builds can take minutes; progress lines stream as
// `runner:setup-progress` events. apiKeySource is "" (keep), "env" or
// "keyring" (refused when no key is stored); forgetKey removes the stored
// key and makes the source env. The key itself never rides this call: see
// StoreModelKey.
func (r *RunAPI) ApplySetup(provider, model, apiKeyEnv, baseURL string, pullImages bool, apiKeySource string, forgetKey bool) (*SetupStatusView, error) {
	if err := r.acquire(jobSetup, "the model and image setup"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	err := run.ApplySetup(r.app.ctx, run.SetupOpts{
		Provider:     provider,
		Model:        model,
		APIKeyEnv:    apiKeyEnv,
		APIKeySource: apiKeySource,
		ForgetKey:    forgetKey,
		BaseURL:      baseURL,
		PullImages:   pullImages,
	}, logw)
	if err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.SetupStatus()
}

// HarnessView is one coding agent — a host harness behind a CLI adapter — as
// the wizard's Coding agent step shows it. Facts from the CLI's
// harnessprobe; no secret is anywhere near this surface.
type HarnessView struct {
	Name           string `json:"name"`
	DisplayName    string `json:"displayName"`
	Executable     string `json:"executable"`
	InstallURL     string `json:"installUrl"`
	Found          bool   `json:"found"`
	Path           string `json:"path,omitempty"`
	PathFromPolicy bool   `json:"pathFromPolicy"`
	// Source says where the executable was found: "policy", "PATH" or "known
	// location" (a usual install directory, a desktop app's bundled build).
	Source         string   `json:"source,omitempty"`
	Version        string   `json:"version,omitempty"`
	VersionMatches bool     `json:"versionMatches"`
	Warnings       []string `json:"warnings,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	Chosen         bool     `json:"chosen"`
	MCPCommand     string   `json:"mcpCommand"`
}

func harnessViews(hs []run.Harness) []HarnessView {
	out := make([]HarnessView, 0, len(hs))
	for _, h := range hs {
		out = append(out, HarnessView{
			Name: h.Name, DisplayName: h.DisplayName, Executable: h.Executable, InstallURL: h.InstallURL,
			Found: h.Found, Path: h.Path, PathFromPolicy: h.PathFromPolicy, Source: h.Source,
			Version: h.Version, VersionMatches: h.VersionMatches, Warnings: h.Warnings, Reason: h.Reason,
			Chosen: h.Chosen, MCPCommand: h.MCPCommand,
		})
	}
	return out
}

// HarnessStatus probes the coding agents the CLI's adapters can drive
// (Claude Code, Codex) on this machine. Read-only: it never creates the
// policy file and runs nothing but each harness's version flag.
func (r *RunAPI) HarnessStatus() ([]HarnessView, error) {
	hs, err := run.HarnessStatus(r.app.ctx)
	if err != nil {
		return nil, err
	}
	return harnessViews(hs), nil
}

// ChosenHarnesses names the harnesses the operator policy carries an entry
// for, reading the policy only — the Done recap asks this, so finishing the
// wizard never executes a harness.
func (r *RunAPI) ChosenHarnesses() ([]HarnessView, error) {
	hs, err := run.ChosenHarnesses()
	if err != nil {
		return nil, err
	}
	return harnessViews(hs), nil
}

// ChooseHarness records harness.<name> in the operator policy (the path the
// probe resolved), so `agentry run --adapter <name>` finds the harness the
// person picked. It refuses a harness that is not on this machine. The
// policy write takes the run lock so it cannot interleave with ApplySetup's,
// and is refused with what holds it.
func (r *RunAPI) ChooseHarness(name string) ([]HarnessView, error) {
	if err := r.acquire(jobSetup, "a change to the coding agents"); err != nil {
		return nil, err
	}
	defer r.release()
	if err := run.ChooseHarness(r.app.ctx, name); err != nil {
		return nil, err
	}
	return r.HarnessStatus()
}

// ForgetHarness removes harness.<name> from the operator policy — Settings →
// Coding tools un-choosing one, so the export the policy implies can change
// — and reads the policy back. Nothing is executed. The policy write takes
// the run lock like ChooseHarness's.
func (r *RunAPI) ForgetHarness(name string) ([]HarnessView, error) {
	if err := r.acquire(jobSetup, "a change to the coding agents"); err != nil {
		return nil, err
	}
	defer r.release()
	if err := run.ForgetHarness(name); err != nil {
		return nil, err
	}
	return r.ChosenHarnesses()
}

// CatalogView is what ListModels returns: the provider's models by id
// (with a display name where the provider gives one), what the key
// turned out to be, and the sentence for a subscription token.
type CatalogView struct {
	Provider string      `json:"provider"`
	Models   []ModelView `json:"models"`
	Kind     string      `json:"kind"`
	Notice   string      `json:"notice,omitempty"`
	// Warning is the plain-http sentence for the endpoint asked, "" for
	// https and loopback.
	Warning string `json:"warning,omitempty"`
}

// ModelView is one listed model, with its price in US dollars a million
// tokens where the provider lists one ("0.10").
type ModelView struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
	InputUSD    string `json:"inputUsd,omitempty"`
	OutputUSD   string `json:"outputUsd,omitempty"`
}

// ListModels asks the provider's models listing with the typed key (or, when
// none is typed, the one the keychain holds) and returns the ids — the Get
// models button, which is also the endpoint test. Like StoreModelKey, the
// one call besides it that carries a key: three arguments, so a dispatch
// error's log line has as little to carry as possible. The key reaches the
// provider's endpoint and nothing else; the answer carries names.
func (r *RunAPI) ListModels(provider, baseURL, key string) (*CatalogView, error) {
	cat, err := run.ListModels(r.app.ctx, provider, baseURL, key)
	if err != nil {
		return nil, err
	}
	v := &CatalogView{Provider: cat.Provider, Models: make([]ModelView, 0, len(cat.Models)), Kind: cat.Kind, Notice: cat.Notice, Warning: cat.Warning}
	for _, m := range cat.Models {
		v.Models = append(v.Models, ModelView{ID: m.ID, DisplayName: m.DisplayName, InputUSD: m.InputUSD, OutputUSD: m.OutputUSD})
	}
	return v, nil
}

// EndpointWarning is the sentence the setup form shows under a custom
// endpoint's URL when the key would reach it over plain http off this
// machine, and "" for https, loopback, or anything not yet a URL. It carries
// no key and asks no network.
func (r *RunAPI) EndpointWarning(baseURL string) string {
	return run.EndpointWarning(baseURL)
}

// StoreModelKey puts the model key for provider into the OS keychain — the
// one call that carries a key, kept to two arguments so a dispatch error's
// log line has as little to carry as possible. The key is written nowhere
// else, and the policy is untouched: the ApplySetup the form makes next
// records the keyring as the source, beside the model and the URL this call
// does not carry. The status it returns lists the provider just stored
// under, so the form knows the key is on this machine even when that next
// call is refused.
func (r *RunAPI) StoreModelKey(provider, key string) (*SetupStatusView, error) {
	if err := r.acquire(jobSetup, "a change to a model key"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.StoreModelKey(provider, key, logw); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.SetupStatus()
}

// JevKeyView is where the person's own AI Review key stands: saved in this
// machine's keychain or not, whether the policy reads it from there, and
// whether the variable a draft reads first is set. Never the key.
type JevKeyView struct {
	Stored      bool   `json:"stored"`
	Source      string `json:"source,omitempty"`
	EnvName     string `json:"envName"`
	EnvSet      bool   `json:"envSet"`
	Ready       bool   `json:"ready"`
	KeyStoreErr string `json:"keyStoreErr,omitempty"`
}

// JevKeyStatus reports the AI Review key's standing. Read-only: it reads the
// keychain's presence marker, never the key, and creates no policy.
func (r *RunAPI) JevKeyStatus() (*JevKeyView, error) {
	st, err := run.GetJevKeyStatus()
	if err != nil {
		return nil, err
	}
	return jevKeyView(st), nil
}

func jevKeyView(st *run.JevKeyStatus) *JevKeyView {
	return &JevKeyView{Stored: st.Stored, Source: st.Source, EnvName: st.EnvName, EnvSet: st.EnvSet, Ready: st.Ready(), KeyStoreErr: st.KeyStoreErr}
}

// StoreJevKey puts the person's TypeSafe Jev key into the OS keychain and
// records the keychain as its source in the runner policy. It is the one
// call that carries the key, kept to one argument so a dispatch error's log
// line has as little to carry as possible; the key is written nowhere else.
// A keychain that cannot be reached is a named refusal, and the policy is
// left as it was.
func (r *RunAPI) StoreJevKey(key string) (*JevKeyView, error) {
	if err := r.acquire(jobSetup, "a change to the AI Review key"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.StoreJevKey(key, logw); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.JevKeyStatus()
}

// ForgetJevKey removes the AI Review key from the keychain and the keychain
// from the policy's sources.
func (r *RunAPI) ForgetJevKey() (*JevKeyView, error) {
	if err := r.acquire(jobSetup, "a change to the AI Review key"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.ForgetJevKey(logw); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.JevKeyStatus()
}

// JevKeyTestView is Test key's answer in plain words, and which key it
// tried: "the keychain" or the variable's name.
type JevKeyTestView struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	From    string `json:"from,omitempty"`
}

// TestJevKey makes one small call to TypeSafe with the key a draft on this
// computer would scan with, and says the answer: it works, TypeSafe refused
// it (401, 403), is limiting it (429), or could not be reached. The key goes
// to TypeSafe's endpoint and nowhere else; the answer carries no key.
func (r *RunAPI) TestJevKey() (*JevKeyTestView, error) {
	res, err := run.TestJevKey(r.app.ctx)
	if err != nil {
		return nil, err
	}
	return &JevKeyTestView{OK: res.OK, Message: res.Message, From: res.From}, nil
}

// HybridKeyView is where the person's own Hybrid Analysis key stands: saved
// in this machine's keychain or not, whether the policy reads it from there,
// and whether the variable a run reads first is set. Never the key.
type HybridKeyView struct {
	Stored      bool   `json:"stored"`
	Source      string `json:"source,omitempty"`
	EnvName     string `json:"envName"`
	EnvSet      bool   `json:"envSet"`
	Ready       bool   `json:"ready"`
	KeyStoreErr string `json:"keyStoreErr,omitempty"`
}

// HybridKeyStatus reports the Hybrid Analysis key's standing. Read-only: it
// reads the keychain's presence marker, never the key, and creates no
// policy.
func (r *RunAPI) HybridKeyStatus() (*HybridKeyView, error) {
	st, err := run.GetHybridKeyStatus()
	if err != nil {
		return nil, err
	}
	return &HybridKeyView{Stored: st.Stored, Source: st.Source, EnvName: st.EnvName, EnvSet: st.EnvSet, Ready: st.Ready(), KeyStoreErr: st.KeyStoreErr}, nil
}

// StoreHybridKey puts the person's Hybrid Analysis key into the OS keychain
// and records the keychain as its source in the runner policy. It is the
// one call that carries the key, kept to one argument as StoreJevKey is; a
// keychain that cannot be reached is a named refusal, and the policy is
// left as it was.
func (r *RunAPI) StoreHybridKey(key string) (*HybridKeyView, error) {
	if err := r.acquire(jobSetup, "a change to the Hybrid Analysis key"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.StoreHybridKey(key, logw); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.HybridKeyStatus()
}

// ForgetHybridKey removes the Hybrid Analysis key from the keychain and the
// keychain from the policy's sources.
func (r *RunAPI) ForgetHybridKey() (*HybridKeyView, error) {
	if err := r.acquire(jobSetup, "a change to the Hybrid Analysis key"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.ForgetHybridKey(logw); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.HybridKeyStatus()
}

// HybridKeyTestView is Test key's answer in plain words, and which key it
// tried: "the keychain" or the variable's name.
type HybridKeyTestView struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	From    string `json:"from,omitempty"`
}

// TestHybridKey looks up one known hash (the EICAR test file's) with the
// key a run on this computer would use, and says the answer: it works,
// Hybrid Analysis refused it, is limiting it, or could not be reached. The
// hash and the key go to Hybrid Analysis and nowhere else; the answer
// carries no key.
func (r *RunAPI) TestHybridKey() (*HybridKeyTestView, error) {
	res, err := run.TestHybridKey(r.app.ctx)
	if err != nil {
		return nil, err
	}
	return &HybridKeyTestView{OK: res.OK, Message: res.Message, From: res.From}, nil
}

// SearchProviderView is one web search provider a person may choose: its
// name as the policy says it, as a person knows it, and the variable a draft
// reads its key from first.
type SearchProviderView struct {
	Name    string `json:"name"`
	Label   string `json:"label"`
	EnvName string `json:"envName"`
}

// SearchKeyView is where the person's own web search key stands: the
// provider the policy names, whether its key is saved in this machine's
// keychain and read from there, whether the variable a draft reads first is
// set, and the providers the runner policy admits. Never the key.
type SearchKeyView struct {
	Provider      string               `json:"provider,omitempty"`
	ProviderLabel string               `json:"providerLabel,omitempty"`
	Stored        bool                 `json:"stored"`
	Source        string               `json:"source,omitempty"`
	EnvName       string               `json:"envName,omitempty"`
	EnvSet        bool                 `json:"envSet"`
	Ready         bool                 `json:"ready"`
	KeyStoreErr   string               `json:"keyStoreErr,omitempty"`
	Providers     []SearchProviderView `json:"providers"`
}

// SearchKeyStatus reports the web search key's standing. Read-only: it
// reads the keychain's presence marker, never the key, and creates no
// policy.
func (r *RunAPI) SearchKeyStatus() (*SearchKeyView, error) {
	st, err := run.GetSearchKeyStatus()
	if err != nil {
		return nil, err
	}
	v := &SearchKeyView{Provider: st.Provider, ProviderLabel: st.ProviderLabel, Stored: st.Stored, Source: st.Source,
		EnvName: st.EnvName, EnvSet: st.EnvSet, Ready: st.Ready(), KeyStoreErr: st.KeyStoreErr}
	for _, p := range run.SearchProviders() {
		v.Providers = append(v.Providers, SearchProviderView{Name: p.Name, Label: p.Label, EnvName: p.EnvName})
	}
	return v, nil
}

// StoreSearchKey puts the person's key for provider into the OS keychain
// and records the provider, and the keychain as its source, in the runner
// policy. It is the one call that carries the key; the key is written
// nowhere else. A keychain that cannot be reached is a named refusal, and
// the policy is left as it was.
func (r *RunAPI) StoreSearchKey(provider, key string) (*SearchKeyView, error) {
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.StoreSearchKey(provider, key, logw); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.SearchKeyStatus()
}

// ForgetSearchKey removes the web search key from the keychain and the
// provider from the policy.
func (r *RunAPI) ForgetSearchKey() (*SearchKeyView, error) {
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.ForgetSearchKey(logw); err != nil {
		return nil, err
	}
	resealSchedules(r.app)
	return r.SearchKeyStatus()
}

// SearchKeyTestView is Test key's answer in plain words, and which key it
// tried: "the keychain" or the variable's name.
type SearchKeyTestView struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	From    string `json:"from,omitempty"`
}

// TestSearchKey makes one search with the key a draft on this computer
// would use, and says the provider's answer. The key goes to the provider's
// endpoint and nowhere else; the answer carries no key.
func (r *RunAPI) TestSearchKey() (*SearchKeyTestView, error) {
	res, err := run.TestSearchKey(r.app.ctx)
	if err != nil {
		return nil, err
	}
	return &SearchKeyTestView{OK: res.OK, Message: res.Message, From: res.From}, nil
}

// ProfileStatusView is a QuickAgent signature's MCP-profile state. Secret
// NAMES only — no surface ever returns a value.
type ProfileStatusView struct {
	Ref             string   `json:"ref"`
	Digest          string   `json:"digest"`
	DeclaredSecrets []string `json:"declaredSecrets"`
	// OptionalSecrets are those of DeclaredSecrets a run can start without.
	OptionalSecrets   []string `json:"optionalSecrets"`
	Exists            bool     `json:"exists"`
	Keep              bool     `json:"keep"`
	StoredSecretNames []string `json:"storedSecretNames"`
	CapabilityLines   []string `json:"capabilityLines"`
	// Intake are the release's up-front questions (manifest v2), answered
	// in the run panel before either mode starts.
	Intake []IntakeQuestionView `json:"intake"`
	// Modes are the run modes the release declares; the panel offers
	// exactly these (a single one preselected).
	Modes []string `json:"modes"`
	// Opening is the manifest's interactive opening line, if any.
	Opening string `json:"opening"`
	// Guide is the author's words for the run setup (manifest v2's guide);
	// nil when the release carries none. The page shows the overview as
	// restricted markdown and the rest as text, never as markup the author
	// controls.
	Guide *GuideView `json:"guide,omitempty"`
	// ShipsKnowledge names the knowledge packs the release ships, mounted in
	// every run of it on this computer.
	ShipsKnowledge []string `json:"shipsKnowledge"`
	// AcceptsKnowledge is the release's ask for a pack of the person's own,
	// shown in the Knowledge step and never to the agent; nil when none.
	AcceptsKnowledge *KnowledgeAcceptsView `json:"acceptsKnowledge,omitempty"`
}

// KnowledgeAcceptsView is the manifest's knowledge.accepts: one plain line
// for the Knowledge step, and whether the step may be skipped.
type KnowledgeAcceptsView struct {
	Prompt   string `json:"prompt"`
	Required bool   `json:"required"`
}

// GuideView is manifest v2's guide block for the run setup: Overview on
// Before you start (markdown, rendered without raw HTML or images),
// Prepare as its "What to have ready" list, Files on Files, Task as the
// task field's placeholder, and Connect as each declared key's help on
// Connect tools.
type GuideView struct {
	Overview string            `json:"overview"`
	Prepare  []string          `json:"prepare"`
	Files    string            `json:"files"`
	Task     string            `json:"task"`
	Connect  map[string]string `json:"connect"`
}

// IntakeQuestionView is one intake question for the pre-run form.
type IntakeQuestionView struct {
	ID       string   `json:"id"`
	Prompt   string   `json:"prompt"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Choices  []string `json:"choices"`
	// Site says a url question is the browser's site to test
	// (capabilities.browser.sites): asked as an http or https address, never
	// as an input source to fetch with a token.
	Site bool `json:"site,omitempty"`
}

func profileStatusView(p *run.ProfileInfo) *ProfileStatusView {
	v := &ProfileStatusView{
		Ref:               p.Ref,
		Digest:            p.Digest,
		DeclaredSecrets:   emptyNotNil(p.DeclaredSecrets),
		OptionalSecrets:   emptyNotNil(p.OptionalSecrets),
		Exists:            p.Exists,
		Keep:              p.Keep,
		StoredSecretNames: emptyNotNil(p.StoredSecretNames),
		CapabilityLines:   emptyNotNil(p.CapabilityLines),
		Intake:            []IntakeQuestionView{},
		Modes:             emptyNotNil(p.Modes),
		Opening:           p.Opening,
		ShipsKnowledge:    emptyNotNil(p.ShipsKnowledge),
	}
	if a := p.AcceptsKnowledge; a != nil {
		v.AcceptsKnowledge = &KnowledgeAcceptsView{Prompt: a.Prompt, Required: a.Required}
	}
	for _, q := range p.Intake {
		site := q.Type == manifest.IntakeURL && slices.Contains(p.BrowserSites, q.ID)
		v.Intake = append(v.Intake, IntakeQuestionView{ID: q.ID, Prompt: q.Prompt, Type: string(q.Type), Required: q.Required, Choices: emptyNotNil(q.Choices), Site: site})
	}
	if g := p.Guide; g != nil {
		v.Guide = &GuideView{Overview: g.Overview, Prepare: emptyNotNil(g.Prepare), Files: g.Files, Task: g.Task, Connect: map[string]string{}}
		for name, help := range g.Connect {
			v.Guide.Connect[name] = help
		}
	}
	return v
}

// ProfileStatus resolves ref against the registry and reports whether the
// exact release (ns/name@version + digest) has a stored MCP profile, what
// secrets its contract declares, and the contract itself — everything the
// guided form shows before asking for keys.
func (r *RunAPI) ProfileStatus(reg, ref string) (*ProfileStatusView, error) {
	p, err := run.ProfileStatus(r.app.ctx, reg, ref)
	if err != nil {
		return nil, err
	}
	return profileStatusView(p), nil
}

// SaveProfile stores the guided form's answers as the signature's MCP
// profile (dir 0700, file 0600). Empty values mean "resolve from the
// environment at run time" and are recorded as absent; an existing
// profile's CreatedAt is preserved.
func (r *RunAPI) SaveProfile(reg, ref string, secrets map[string]string, keep bool) (*ProfileStatusView, error) {
	p, err := run.SaveProfile(r.app.ctx, reg, ref, secrets, keep)
	if err != nil {
		return nil, err
	}
	return profileStatusView(p), nil
}

// RunResultView is the §R8 envelope, flattened. It is returned for failed
// runs too (status "error" + exit class) so the receipts stay reachable.
type RunResultView struct {
	Status       string  `json:"status"`
	ExitCode     int     `json:"exitCode"`
	Error        string  `json:"error,omitempty"`
	RunID        string  `json:"runId"`
	Ref          string  `json:"ref"`
	Digest       string  `json:"digest"`
	ResultJSON   string  `json:"resultJson"`
	OutDir       string  `json:"outDir"`
	ReceiptsDir  string  `json:"receiptsDir"`
	RunPlanSHA   string  `json:"runPlanSha"`
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	CostUSD      float64 `json:"costUsd"`
	WallMS       int64   `json:"wallMs"`
	Turns        int     `json:"turns"`
	// Credits is what a run paid with Agentry credits was charged
	// (paidWithCredits), the registry's figure; both are absent for a run
	// paid with a key or a sign-in, whose result names no credits.
	Credits          int64  `json:"credits,omitempty"`
	PaidWithCredits  bool   `json:"paidWithCredits,omitempty"`
	AttestationState string `json:"attestationState"`
	// Policy is the operator policy file the run enforced, the envelope's
	// block as it is: omitted when the envelope carries none.
	Policy *receipt.Policy `json:"policy,omitempty"`
	// Knowledge is the receipt's knowledge line: the packs mounted, how, and
	// the tools that read them; omitted when none was attached.
	Knowledge string `json:"knowledge,omitempty"`
	// UnsafeExec is the receipt's exec line when the run was compiled in
	// Unsafe Mode: what was installed from where, what the image could not
	// provide and what did not install; omitted for every other run.
	UnsafeExec string `json:"unsafeExec,omitempty"`
	// Decode is the receipt's decode line: how the input files were read,
	// and what ClamAV and the Hybrid Analysis lookup made of them; omitted
	// for a run with no files.
	Decode string `json:"decode,omitempty"`
	// ExitClass names exitCode in the CLI's words; Stage and Reason say
	// where and why a failed run stopped, in its closed words: a blocked
	// run's reason says whether the QuickAgent or the runner's tools stopped
	// it.
	ExitClass string          `json:"exitClass,omitempty"`
	Stage     string          `json:"stage,omitempty"`
	Reason    *receipt.Reason `json:"reason,omitempty"`
}

// RunAgent executes one QuickAgent run: resolve → digest + DSSE verify →
// materialize into the shared ~/.agentry/runner cache → compile/reuse the
// run plan → ensure images → run in the isolated container topology → apply
// the profile keep choice. The run is followed by its id: its progress as
// `runner:run-progress`, its log lines as `runner:run-line`
// (services_runflow.go). timeoutSec > 0 overrides the policy timeout;
// maxTurns >= 0 overrides the tool-call budget (0 = unlimited, -1 = policy
// default). acceptUnsigned is the explicit consent for a release without an
// attestation; without it such a run is refused in Go. acceptUnscanned is
// the consent for a release the registry admitted without a scan — its own,
// which acceptUnsigned does not give — enforced in Go the same way, on every
// run from the cache too. knowledge names the knowledge packs kept on this
// computer to attach: each is opened from the local store once the gates
// have passed, and its opened copy is removed when the run returns.
// sourceTokens are access tokens for this run's input sources, by question
// id: each reaches that source's fetch step alone, and a source given none
// uses the key saved for its host (services_sources.go).
func (r *RunAPI) RunAgent(reg, ref, task string, inputPaths []string, outDir string, keepProfile *bool, timeoutSec, maxTurns int, acceptUnsigned bool, intakeAnswers map[string]string, allowVulnerableMCP, acceptUnscanned bool, knowledge []string, writeInVoice bool, sourceTokens map[string]string, hostSignIns map[string]run.HostSignIn, destinations []string) (*RunResultView, error) {
	return r.runAgent(runHooks{}, reg, ref, task, inputPaths, outDir, keepProfile, timeoutSec, maxTurns, acceptUnsigned, intakeAnswers, allowVulnerableMCP, acceptUnscanned, knowledge, writeInVoice, sourceTokens, hostSignIns, destinations)
}

// runHooks are what a run Go starts on its own behalf (Main's) needs that a
// run from the page does not.
type runHooks struct {
	// began is told the run's id once it has begun.
	began func(runID string)
	// queued is told the run's id when it waits behind another job for
	// the run lock; began follows once it starts.
	queued func(runID string)
	// inputsIn is the folder the run's input files were taken from in Go,
	// a persistent agent's share folder: each must be a regular file in it,
	// not reached through a link, in place of a picked one.
	inputsIn string
}

// runAgent is RunAgent with its hooks.
func (r *RunAPI) runAgent(h runHooks, reg, ref, task string, inputPaths []string, outDir string, keepProfile *bool, timeoutSec, maxTurns int, acceptUnsigned bool, intakeAnswers map[string]string, allowVulnerableMCP, acceptUnscanned bool, knowledge []string, writeInVoice bool, sourceTokens map[string]string, hostSignIns map[string]run.HostSignIn, destinations []string) (*RunResultView, error) {
	// The switch is read once: the dialog that carries its line and the
	// compile that obeys it see the same answer, whatever is turned on or
	// off while the dialog is up.
	unsafe := unsafeModeOn()
	picked := inputPaths
	if h.inputsIn != "" {
		for _, p := range inputPaths {
			if err := fileIn(h.inputsIn, p); err != nil {
				return nil, err
			}
		}
		picked = nil
	}
	if err := r.localRunGates(reg, ref, picked, outDir, acceptUnsigned, allowVulnerableMCP, acceptUnscanned, unsafe); err != nil {
		return nil, err
	}
	// The run's id is known before it waits, so the page can follow a
	// queued run, and cancel it, by its id.
	id, err := r.mintRunID()
	if err != nil {
		return nil, err
	}
	var queued func()
	if h.queued != nil {
		queued = func() { h.queued(id) }
	}
	if err := r.acquireRun(id, ref, queued); err != nil {
		return nil, err
	}
	defer r.release()
	// The run's own context: quitting and Stop cancel it, the engine
	// returns and tears down, and shutdown waits for that
	// (services_quit.go).
	ctx, cancel := context.WithCancel(r.app.ctx)
	defer cancel()
	live, err := r.beginRunAs(jobRun, ref, id, cancel)
	if err != nil {
		return nil, err
	}
	defer r.endRun(live)
	live.registry = reg
	// A run that read the switch on is an Unsafe Mode run from here until
	// its compile says otherwise.
	r.setUnsafeLive(live, unsafe)
	// The run's first report says it has begun, before anything is prepared:
	// the page's store follows it by its id from here.
	meter := r.runMeter(live.id, ref)
	meter.Emit()
	if h.began != nil {
		h.began(live.id)
	}
	packs, err := r.attach(ctx, live, reg, knowledge)
	if err != nil {
		return nil, err
	}
	voiced, err := r.voiceFor(ctx, reg, writeInVoice)
	if err != nil {
		return nil, err
	}
	logw := r.runLineWriter(live.id)
	defer logw.Flush()
	usage, stopUsage := usageOf(meter)
	defer stopUsage()
	// The agent's tool calls, their results and its messages join the run
	// log as they happen, through a writer of their own: the engine writes
	// its progress to logw from another goroutine. Every line it is given
	// is whole, so it holds nothing to flush.
	agentw := r.runLineWriter(live.id)
	agentLog := newRunEventLog(func(line string) { _, _ = agentw.Write([]byte(line + "\n")) })
	res, err := r.runEngine(ctx, run.Request{
		OnBudgets:          budgetsOf(meter),
		OnUsage:            usage,
		OnEvent:            agentLog.Receive,
		RunID:              live.id,
		InFlightDir:        r.inFlightDir(),
		Registry:           reg,
		Ref:                ref,
		Task:               task,
		InputPaths:         inputPaths,
		Knowledge:          packs,
		Voice:              voiced,
		ReportTheme:        r.reportThemeFor(ctx, reg, logw),
		OutDir:             outDir,
		OutDirIsRoot:       isSavedOutputDir(outDir),
		DeliverTo:          deliverFolder(),
		Destinations:       destinations,
		KeepProfile:        keepProfile,
		TimeoutSeconds:     timeoutSec,
		MaxTurns:           maxTurns,
		AcceptUnsigned:     acceptUnsigned,
		AcceptUnscanned:    acceptUnscanned,
		AllowVulnerableMCP: allowVulnerableMCP,
		Intake:             intakeAnswers,
		SourceTokens:       sourceTokens,
		HostSignIns:        hostSignIns,
		TrustHostKey:       r.app.trustHostKey,
		KeyFilePicked:      r.app.keyFiles.require,
		PathPicked:         r.app.requirePicked,
		Unsafe:             unsafe,
		PlanCompiled:       func(u bool) { r.setUnsafeLive(live, u) },
	}, logw)
	// The run has returned: what it said reaches the log before its result.
	agentLog.Close()
	recordLocal(ctx, runhistory.KindRun, live, ref, res)
	// A sign-in the run saved reaches the schedules that run while the
	// person is signed out, as every other key change does.
	if savesHostSignIn(hostSignIns) {
		resealSchedules(r.app)
	}
	if err != nil {
		return nil, err
	}
	return r.resultView(res), nil
}

// runResultView flattens the engine result for the frontend.
func runResultView(res *run.Result) *RunResultView {
	v := &RunResultView{
		Status:           res.Status,
		ExitCode:         res.ExitCode,
		Error:            res.Error,
		RunID:            res.RunID,
		Ref:              res.Ref,
		Digest:           res.Digest,
		ResultJSON:       res.ResultJSON,
		OutDir:           res.OutDir,
		ReceiptsDir:      res.ReceiptsDir,
		RunPlanSHA:       res.RunPlanSHA,
		InputTokens:      res.InputTokens,
		OutputTokens:     res.OutputTokens,
		CostUSD:          res.CostUSD,
		Credits:          res.Credits,
		PaidWithCredits:  res.PaidWithCredits,
		WallMS:           res.WallMS,
		Turns:            res.Turns,
		AttestationState: res.AttestationState,
		Policy:           res.Policy,
		Knowledge:        res.Knowledge,
		Decode:           res.Decode,
		ExitClass:        res.ExitClass,
		Stage:            res.Stage,
		Reason:           res.Reason,
	}
	if res.Unsafe {
		v.UnsafeExec = res.Exec
	}
	return v
}

// MCPToolRow is one granted gateway tool and the declaration granting it.
type MCPToolRow struct {
	Tool      string `json:"tool"`
	GrantedBy string `json:"grantedBy"`
}

// MCPView feeds the agent page's "MCP servers & tools" section (parity with
// the site).
type MCPView struct {
	Ref      string       `json:"ref"`
	Version  string       `json:"version"`
	ToolRows []MCPToolRow `json:"toolRows"`
	Servers  []string     `json:"servers"`
	// DeclaredServers is each declared MCP server's pin, tools and plain
	// env; Servers ends with one line per entry, in order.
	DeclaredServers []mcpview.Server `json:"declaredServers"`
	SecretNames     []string         `json:"secretNames"`
	DataAccess      []string         `json:"dataAccess"`
	EmbeddedTools   []string         `json:"embeddedTools"`
	CapabilityLines []string         `json:"capabilityLines"`
	// Vulnerabilities is the registry's verdict on each declared MCP server;
	// Refusal is the text a run meets by default.
	Vulnerabilities   []mcpvuln.Status `json:"vulnerabilities"`
	VulnerabilityLine string           `json:"vulnerabilityLine,omitempty"`
	Refusal           string           `json:"refusal,omitempty"`
	// Refusing names the refusing servers alone, for the consent row Unsafe
	// Mode offers.
	Refusing string `json:"refusing,omitempty"`
}

// GatewayRequirements computes the tool surface the latest published
// version's declared contract becomes — through the runner's per-run
// gateway, and as host built-ins in an embedded install.
func (r *RunAPI) GatewayRequirements(reg, ns, name string) (*MCPView, error) {
	info, err := run.Requirements(r.app.ctx, reg, ns, name)
	if err != nil {
		return nil, err
	}
	return mcpViewOf(info), nil
}

// ChangedInstallView is an installed release that changed after it was laid
// down, as the run setup shows it beside Repair: the release, what changed,
// and whether Repair can lay it down again (a registry release) or the way
// back is its folder (a QuickAgent installed from one).
type ChangedInstallView struct {
	Ref     string   `json:"ref"`
	Changed []string `json:"changed"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	// Record says the integrity record or install.json itself failed:
	// "install-json-changed" or "record-invalid", else empty.
	Record string `json:"record,omitempty"`
	// Local: installed from a folder, laid down again from there (Repair),
	// never from a registry.
	Local  bool   `json:"local"`
	Repair string `json:"repair,omitempty"`
}

// InstalledChanges reads whether the release a run of ref would run is
// installed on this computer and changed after it was laid down: nil when it
// is whole or not installed yet. The run setup asks before Run and again
// after a run is refused, and offers Repair.
func (r *RunAPI) InstalledChanges(reg, ref string) (*ChangedInstallView, error) {
	c, err := run.InstalledChanges(r.app.ctx, reg, ref)
	if err != nil || c == nil {
		return nil, err
	}
	v := install.ChangedInstallOf(c)
	return &ChangedInstallView{Ref: c.Ref, Changed: v.Changed, Added: v.Added, Removed: v.Removed, Record: v.Record, Local: c.Repair != "", Repair: c.Repair}, nil
}

// RunRequirements is the tool surface of the release a run of ref would run:
// a QuickAgent installed from a folder on this computer when there is one,
// else the registry's. The run panel reads this; the published agent's page
// reads GatewayRequirements.
func (r *RunAPI) RunRequirements(reg, ref string) (*MCPView, error) {
	info, err := run.RunRequirements(r.app.ctx, reg, ref)
	if err != nil {
		return nil, err
	}
	return mcpViewOf(info), nil
}

func mcpViewOf(info *run.MCPInfo) *MCPView {
	v := &MCPView{
		Ref:               info.Ref,
		Version:           info.Version,
		ToolRows:          []MCPToolRow{},
		Servers:           emptyNotNil(info.Servers),
		DeclaredServers:   info.Declared,
		SecretNames:       emptyNotNil(info.SecretNames),
		DataAccess:        emptyNotNil(info.DataAccess),
		EmbeddedTools:     emptyNotNil(info.EmbeddedTools),
		CapabilityLines:   emptyNotNil(info.CapabilityLines),
		Vulnerabilities:   info.Vulnerabilities,
		VulnerabilityLine: info.VulnerabilityLine,
		Refusal:           info.Refusal,
		Refusing:          info.Refusing,
	}
	if v.Vulnerabilities == nil {
		v.Vulnerabilities = []mcpvuln.Status{}
	}
	for _, row := range info.ToolRows {
		v.ToolRows = append(v.ToolRows, MCPToolRow{Tool: row.Tool, GrantedBy: row.GrantedBy})
	}
	return v
}

// emptyNotNil keeps JSON arrays arrays — the frontend maps over them.
func emptyNotNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// eventWriter forwards whole log lines to the frontend as Wails events.
func (r *RunAPI) eventWriter(event string) *lineEventWriter {
	return &lineEventWriter{ctx: r.app, event: event}
}

// lineEventWriter buffers writes and emits one event per completed line;
// Flush emits any trailing partial line. It writes nothing anywhere else,
// so a secret that never enters the log stream (the engine keeps it out)
// never reaches the webview.
type lineEventWriter struct {
	ctx   *App
	event string
	// wrap, when set, makes each line's payload (a run's line with its id).
	wrap func(line string) any
	buf  bytes.Buffer
}

func (w *lineEventWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			// Partial line: keep it buffered for the next write.
			w.buf.Reset()
			w.buf.WriteString(line)
			break
		}
		w.emit(line[:len(line)-1])
	}
	return len(p), nil
}

// Flush emits a trailing partial line, if any.
func (w *lineEventWriter) Flush() {
	if w.buf.Len() > 0 {
		w.emit(w.buf.String())
		w.buf.Reset()
	}
}

func (w *lineEventWriter) emit(line string) {
	if line == "" {
		return
	}
	// Through the one emitter (services_hostedrun.go), so a run followed
	// without a webview — the hosted lifecycle tests — can read what the
	// panel would have been shown.
	if w.wrap != nil {
		emitEvent(w.ctx, w.event, w.wrap(line))
		return
	}
	emitEvent(w.ctx, w.event, line)
}

// Compile-time check: the engine hands us an io.Writer contract.
var _ io.Writer = (*lineEventWriter)(nil)

// runGates is the gates of a run whose registry the caller does not name.
// RunAgent and StartSession call localRunGates; this form is what the tests
// of the gates call, and it is kept with its signature.
func (r *RunAPI) runGates(ref string, inputPaths []string, outDir string, acceptUnsigned, allowVulnerableMCP, acceptUnscanned bool) error {
	return r.localRunGates("", ref, inputPaths, outDir, acceptUnsigned, allowVulnerableMCP, acceptUnscanned, unsafeModeOn())
}

// localRunGates is what RunAgent and StartSession check before the engine is
// asked for anything: every path came from a native picker, and a consent
// the page passed is put to the person in a native dialog. The page's
// checkboxes remain what they were — the record of what is being accepted —
// and the dialog is the gate.
//
// While Unsafe Mode is on (unsafe, which the caller read once), the dialog
// carries one more paragraph naming it and the declared commands its image
// cannot provide. That paragraph adds no dialog of its own: turning Unsafe
// Mode on was the native confirmation, and a run with nothing to accept
// still raises none.
//
// A declared MCP server the registry's gate rates high or critical runs on
// this computer only in Unsafe Mode, so with the switch off the page's
// consent for one is refused here, before any dialog: there is nothing to
// ask. The engine refuses the server again on the registry's own answer
// whatever the page sent.
func (r *RunAPI) localRunGates(reg, ref string, inputPaths []string, outDir string, acceptUnsigned, allowVulnerableMCP, acceptUnscanned, unsafe bool) error {
	for _, p := range inputPaths {
		if err := r.app.requirePicked("input", p); err != nil {
			return err
		}
	}
	if err := r.app.requireOutputDir(outDir); err != nil {
		return err
	}
	if allowVulnerableMCP && !unsafe {
		return errors.New("a declared MCP server the registry's gate rates high or critical runs on this computer only in Unsafe Mode, and Unsafe Mode is off: turn it on in Settings → Run defaults, or run a release that pins a fixed version")
	}
	var accepting []string
	if acceptUnsigned {
		accepting = append(accepting, "a release with no attestation")
	}
	if acceptUnscanned {
		accepting = append(accepting, "a release the registry admitted without a scan")
	}
	if allowVulnerableMCP {
		// Go has not looked at the servers here, so the line asserts none:
		// the engine decides on the registry's answer, and the receipt
		// names each server it let through.
		accepting = append(accepting, "any declared MCP server the registry's gate rates high or critical, which runs only in Unsafe Mode")
	}
	if len(accepting) == 0 {
		return nil
	}
	message := "This run accepts " + strings.Join(accepting, ", and ") + ". The receipt will record the consent."
	if unsafe {
		message += "\n\n" + r.unsafeNoticeFor(reg, ref).Line() + "\n\n"
	} else {
		message += " "
	}
	return r.app.askConsent("Run "+ref+" anyway?", message+"Continue?")
}

// knowledge is the local knowledge store's wiring (internal/knowledge).
func (r *RunAPI) knowledge() knowledge.Env { return knowledgeEnv(r.knowledgeEnv) }

// knowledgeEnv is a test's wiring, or this app's.
func knowledgeEnv(e *knowledge.Env) knowledge.Env {
	if e != nil {
		return *e
	}
	return knowledge.Default()
}

// asRun runs f as the run in flight under the run lock, as a local run
// runs (a knowledge build is one): refused while another holds the lock;
// its context cancelled, its run id's containers swept and waited for when
// the app quits (services_quit.go); f's log streamed as event.
func (r *RunAPI) asRun(kind jobKind, what, ref, event string, f func(ctx context.Context, runID string, logw io.Writer) error) error {
	if err := r.acquire(kind, what); err != nil {
		return err
	}
	defer r.release()
	ctx, cancel := context.WithCancel(r.app.ctx)
	defer cancel()
	live, err := r.beginRun(kind, ref, cancel)
	if err != nil {
		return err
	}
	defer r.endRun(live)
	logw := r.eventWriter(event)
	defer logw.Flush()
	err = f(ctx, live.id, logw)
	recordBuiltin(ctx, live, err)
	return err
}

// voiceEnv is the Writing Voice's wiring: the knowledge store's, whose key
// wrapping the voice shares. The Voice panel and a run read it the same.
func (r *RunAPI) voiceEnv() voice.Env { return voice.Env{Env: r.knowledge()} }

// voiceFor is this computer's Writing Voice for a run that writes in it: the
// person ticked Write in my voice in the run setup, which starts ticked from
// the voice's own switch. nil when the run does not ask; a refusal when it
// asks and this computer keeps none, or the one it keeps will not open.
func (r *RunAPI) voiceFor(ctx context.Context, reg string, want bool) (*engine.AttachedVoice, error) {
	if !want {
		return nil, nil
	}
	v, err := r.voiceEnv().Attach(ctx, settings.Normalize(reg))
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, errors.New("this run was asked to write in your voice, and this computer keeps no Writing Voice: untick Write in my voice, or make one on the Voice page")
	}
	return v, nil
}
