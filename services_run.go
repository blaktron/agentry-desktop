// RunAPI: the bound run surface (RUNNER-PLAN-2 §P6) — setup status and
// one-click setup (§P1), MCP-profile status and guided configuration as a
// form (§P3), the tool-surface section (req 3/4), and run-with-task over
// agentry-cli/pkg/runner. Long operations stream progress as Wails events
// (`runner:setup-progress`, `runner:run-log`); secret VALUES cross this
// boundary only inward (SaveProfile) and are never returned or logged.
package main

import (
	"bytes"
	"context"
	"errors"
	"github.com/blaktron/agentry-desktop/internal/mcpview"
	"github.com/blaktron/agentry-desktop/internal/mcpvuln"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// Event names the frontend subscribes to.
const (
	eventSetupProgress = "runner:setup-progress"
	eventRunLog        = "runner:run-log"
	// eventSessionEvent carries an interactive session's events (JSON
	// SessionEventView) to the chat view; runner:run-log stays autonomous.
	eventSessionEvent = "runner:session-event"
)

// errBusy refuses a second docker-touching operation instead of silently
// queueing it behind one that can run for many minutes.
var errBusy = errors.New("another run or image setup is already in progress — wait for it to finish")

// RunAPI drives BriefAgent runs from the app.
type RunAPI struct {
	app *App
	// mu serializes the docker-touching operations: one run (or image
	// setup) at a time keeps the log streams and the engine's teardown
	// sweeps unambiguous. An interactive session holds it from
	// StartSession to EndSession (D-017: one session per app).
	mu sync.Mutex
	// session is the live interactive session, nil otherwise.
	session *run.Session
	// hosted is the In-Agentry run this app is following (services_
	// hostedrun.go), under its own lock: a hosted run touches no docker,
	// so it must not wait on — or be waited on by — the run lock above.
	hostedMu sync.Mutex
	hosted   *hostedFollow

	// live is the run or session in flight, so quitting can end it and
	// sweep what it built (services_quit.go, agentry-desktop#125).
	liveMu sync.Mutex
	live   *liveRun
	// Engine seams for tests; nil means internal/run.
	newRunID  func() (string, error)
	engineRun func(context.Context, run.Request, io.Writer) (*run.Result, error)
	sweep     func(context.Context, string) error
	// inFlightDir holds the crash-path records (services_quit.go,
	// agentry-desktop#198); "" is the runner home's desktop-runs.
	inFlightDir string

	// folders are the run folders of the results Go produced, by run id,
	// for the result view's two bindings (services_outputs.go); openFolder
	// is the file-manager seam a test substitutes.
	foldersMu  sync.Mutex
	folders    map[string]string
	openFolder func(dir string) error
}

// SessionView is what StartSession returns.
type SessionView struct {
	Ref string `json:"ref"`
}

// SessionEventView is one runner:session-event payload — the session's
// event, flattened for the chat view.
type SessionEventView struct {
	Kind         string  `json:"kind"`
	Time         string  `json:"time"`
	Text         string  `json:"text,omitempty"`
	Synthetic    bool    `json:"synthetic,omitempty"`
	StopReason   string  `json:"stopReason,omitempty"`
	Path         string  `json:"path,omitempty"`
	Digest       string  `json:"digest,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	InputTokens  int64   `json:"inputTokens,omitempty"`
	OutputTokens int64   `json:"outputTokens,omitempty"`
	CostUSD      float64 `json:"costUsd,omitempty"`
}

// StartSession starts an interactive run (plan §7): the same install,
// consent-recorded contract, images, profile and intake as RunAgent, then
// the engine's session; events stream as runner:session-event until
// EndSession. The run lock is held for the session's whole life.
func (r *RunAPI) StartSession(reg, ref, task string, inputPaths []string, outDir string, keepProfile *bool, timeoutSec, maxTurns int, acceptUnsigned bool, intakeAnswers map[string]string, allowVulnerableMCP, acceptUnscanned bool) (*SessionView, error) {
	if err := r.runGates(ref, inputPaths, outDir, acceptUnsigned, allowVulnerableMCP, acceptUnscanned); err != nil {
		return nil, err
	}
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	// The session's own context, the parent of everything the engine
	// starts for it: quitting while the session is still starting cancels
	// it (services_quit.go); EndSession releases it after End.
	ctx, cancel := context.WithCancel(r.app.ctx)
	live, err := r.beginRun(ref, cancel)
	if err != nil {
		cancel()
		r.mu.Unlock()
		return nil, err
	}
	logw := r.eventWriter(eventRunLog)
	s, err := run.StartSession(ctx, run.Request{
		RunID:              live.id,
		Registry:           reg,
		Ref:                ref,
		Task:               task,
		InputPaths:         inputPaths,
		OutDir:             outDir,
		KeepProfile:        keepProfile,
		TimeoutSeconds:     timeoutSec,
		MaxTurns:           maxTurns,
		AcceptUnsigned:     acceptUnsigned,
		AcceptUnscanned:    acceptUnscanned,
		AllowVulnerableMCP: allowVulnerableMCP,
		Intake:             intakeAnswers,
		PathPicked:         r.app.requirePicked,
	}, logw)
	logw.Flush()
	if err != nil {
		cancel()
		r.endRun(live)
		r.mu.Unlock()
		return nil, err
	}
	r.session = s
	go func() {
		for ev := range s.Events() {
			runtime.EventsEmit(r.app.ctx, eventSessionEvent, SessionEventView{
				Kind: string(ev.Kind), Time: ev.Time.Format(time.RFC3339Nano), Text: ev.Text, Synthetic: ev.Synthetic,
				StopReason: ev.StopReason, Path: ev.Path, Digest: ev.Digest, Reason: string(ev.Reason),
				InputTokens: ev.InputTokens, OutputTokens: ev.OutputTokens, CostUSD: ev.CostUSD,
			})
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
		l.cancel()
		r.endRun(l)
	}
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return r.resultView(res), nil
}

// SetupStatusView is the machine's run-readiness (§P1).
type SetupStatusView struct {
	DockerOK     bool   `json:"dockerOk"`
	DockerErr    string `json:"dockerErr,omitempty"`
	PolicyPath   string `json:"policyPath"`
	PolicyExists bool   `json:"policyExists"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	APIKeyEnv    string `json:"apiKeyEnv"`
	// APIKeySource is "keyring" when the OS keychain is the fallback for
	// an unset variable; KeysStored which of the providers the form can
	// show have a key in it, so the badge follows the provider chosen and
	// not the policy's (desktop#141); KeyStoreErr why the keychain could
	// not be asked. No value, ever.
	APIKeySource string   `json:"apiKeySource,omitempty"`
	KeysStored   []string `json:"keysStored"`
	KeyStoreErr  string   `json:"keyStoreErr,omitempty"`
	// APIKeyKind is "subscription" for a stored Anthropic subscription
	// token (agentry-cli#171), and KeyNotice the sentence to show beside
	// the field for it; both empty for an API key.
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
		PolicyPath:            s.PolicyPath,
		PolicyExists:          s.PolicyExists,
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
// (the same policy.yaml the CLI reads, req 5) and optionally prepares the
// gateway plane now. Image pulls/builds can take minutes; progress lines
// stream as `runner:setup-progress` events. apiKeySource is "" (keep), "env"
// or "keyring" (refused when no key is stored); forgetKey removes the stored
// key and makes the source env. The key itself never rides this call: see
// StoreModelKey.
func (r *RunAPI) ApplySetup(provider, model, apiKeyEnv, baseURL string, pullImages bool, apiKeySource string, forgetKey bool) (*SetupStatusView, error) {
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
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
	return r.SetupStatus()
}

// HarnessView is one coding agent — a host harness behind a CLI adapter —
// as the wizard's Coding agent step shows it (agentry-desktop#89). Facts
// from the CLI's harnessprobe; no secret is anywhere near this surface.
type HarnessView struct {
	Name           string `json:"name"`
	DisplayName    string `json:"displayName"`
	Executable     string `json:"executable"`
	InstallURL     string `json:"installUrl"`
	Found          bool   `json:"found"`
	Path           string `json:"path,omitempty"`
	PathFromPolicy bool   `json:"pathFromPolicy"`
	// Source says where the executable was found: "policy", "PATH" or
	// "known location" (a usual install directory, a desktop app's
	// bundled build — agentry-cli#167).
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
// policy write takes the run lock so it cannot interleave with ApplySetup's;
// errBusy's wording covers the run case, which is the one a person can hit.
func (r *RunAPI) ChooseHarness(name string) ([]HarnessView, error) {
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
	if err := run.ChooseHarness(r.app.ctx, name); err != nil {
		return nil, err
	}
	return r.HarnessStatus()
}

// ForgetHarness removes harness.<name> from the operator policy — Settings
// → Coding agents un-choosing one (agentry-desktop#148), so the export the
// policy implies can change — and reads the policy back. Nothing is
// executed. The policy write takes the run lock like ChooseHarness's.
func (r *RunAPI) ForgetHarness(name string) ([]HarnessView, error) {
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
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
}

// ModelView is one listed model.
type ModelView struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
}

// ListModels asks the provider's models listing with the typed key (or,
// when none is typed, the one the keychain holds) and returns the ids —
// the Get models button, which is also the endpoint test
// (agentry-desktop#131). Like StoreModelKey, the one call besides it that
// carries a key: three arguments, so a dispatch error's log line
// (desktop#102) has as little to carry as possible. The key reaches the
// provider's endpoint and nothing else; the answer carries names.
func (r *RunAPI) ListModels(provider, baseURL, key string) (*CatalogView, error) {
	cat, err := run.ListModels(r.app.ctx, provider, baseURL, key)
	if err != nil {
		return nil, err
	}
	v := &CatalogView{Provider: cat.Provider, Models: make([]ModelView, 0, len(cat.Models)), Kind: cat.Kind, Notice: cat.Notice}
	for _, m := range cat.Models {
		v.Models = append(v.Models, ModelView{ID: m.ID, DisplayName: m.DisplayName})
	}
	return v, nil
}

// StoreModelKey puts the model key for provider into the OS keychain — the
// one call that carries a key, kept to two arguments so a dispatch error's
// log line (desktop#102) has as little to carry as possible. The key is
// written nowhere else, and the policy is untouched: the ApplySetup the
// form makes next records the keyring as the source, beside the model and
// the URL this call does not carry (agentry-desktop#139). The status it
// returns lists the provider just stored under, so the form knows the key
// is on this machine even when that next call is refused (desktop#141).
func (r *RunAPI) StoreModelKey(provider, key string) (*SetupStatusView, error) {
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.StoreModelKey(provider, key, logw); err != nil {
		return nil, err
	}
	return r.SetupStatus()
}

// ProfileStatusView is a BriefAgent signature's MCP-profile state (§P2).
// Secret NAMES only — no surface ever returns a value.
type ProfileStatusView struct {
	Ref               string   `json:"ref"`
	Digest            string   `json:"digest"`
	DeclaredSecrets   []string `json:"declaredSecrets"`
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
	// Guide is the author's words for the run setup (manifest v2's guide,
	// DEV-141; agentry-desktop#228); nil when the release carries none.
	// The page shows it as text, never as markup it trusts.
	Guide *GuideView `json:"guide,omitempty"`
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
}

func profileStatusView(p *run.ProfileInfo) *ProfileStatusView {
	v := &ProfileStatusView{
		Ref:               p.Ref,
		Digest:            p.Digest,
		DeclaredSecrets:   emptyNotNil(p.DeclaredSecrets),
		Exists:            p.Exists,
		Keep:              p.Keep,
		StoredSecretNames: emptyNotNil(p.StoredSecretNames),
		CapabilityLines:   emptyNotNil(p.CapabilityLines),
		Intake:            []IntakeQuestionView{},
		Modes:             emptyNotNil(p.Modes),
		Opening:           p.Opening,
	}
	for _, q := range p.Intake {
		v.Intake = append(v.Intake, IntakeQuestionView{ID: q.ID, Prompt: q.Prompt, Type: string(q.Type), Required: q.Required, Choices: emptyNotNil(q.Choices)})
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
// guided form (§P3) shows before asking for keys.
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
	Status           string  `json:"status"`
	ExitCode         int     `json:"exitCode"`
	Error            string  `json:"error,omitempty"`
	RunID            string  `json:"runId"`
	Ref              string  `json:"ref"`
	Digest           string  `json:"digest"`
	ResultJSON       string  `json:"resultJson"`
	OutDir           string  `json:"outDir"`
	ReceiptsDir      string  `json:"receiptsDir"`
	RunPlanSHA       string  `json:"runPlanSha"`
	InputTokens      int64   `json:"inputTokens"`
	OutputTokens     int64   `json:"outputTokens"`
	CostUSD          float64 `json:"costUsd"`
	WallMS           int64   `json:"wallMs"`
	Turns            int     `json:"turns"`
	AttestationState string  `json:"attestationState"`
}

// RunAgent executes one BriefAgent run: resolve → digest + DSSE verify →
// materialize into the shared ~/.agentry/runner cache → compile/reuse the
// run plan → ensure images → run in the isolated container topology →
// apply the profile keep choice. Log lines stream as `runner:run-log`
// events. timeoutSec > 0 overrides the policy timeout; maxTurns >= 0
// overrides the tool-call budget (0 = unlimited, -1 = policy default).
// acceptUnsigned is the explicit consent for a release without an
// attestation; without it such a run is refused in Go (desktop#3).
// acceptUnscanned is the consent for a release the registry admitted
// without a scan (M23, D-029) — its own, which acceptUnsigned does not
// give — enforced in Go the same way, on every run from the cache too.
func (r *RunAPI) RunAgent(reg, ref, task string, inputPaths []string, outDir string, keepProfile *bool, timeoutSec, maxTurns int, acceptUnsigned bool, intakeAnswers map[string]string, allowVulnerableMCP, acceptUnscanned bool) (*RunResultView, error) {
	if err := r.runGates(ref, inputPaths, outDir, acceptUnsigned, allowVulnerableMCP, acceptUnscanned); err != nil {
		return nil, err
	}
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
	// The run's own context: quitting cancels it, the engine returns and
	// tears down, and shutdown waits for that (services_quit.go).
	ctx, cancel := context.WithCancel(r.app.ctx)
	defer cancel()
	live, err := r.beginRun(ref, cancel)
	if err != nil {
		return nil, err
	}
	defer r.endRun(live)
	logw := r.eventWriter(eventRunLog)
	defer logw.Flush()
	res, err := r.runEngine(ctx, run.Request{
		RunID:              live.id,
		Registry:           reg,
		Ref:                ref,
		Task:               task,
		InputPaths:         inputPaths,
		OutDir:             outDir,
		KeepProfile:        keepProfile,
		TimeoutSeconds:     timeoutSec,
		MaxTurns:           maxTurns,
		AcceptUnsigned:     acceptUnsigned,
		AcceptUnscanned:    acceptUnscanned,
		AllowVulnerableMCP: allowVulnerableMCP,
		Intake:             intakeAnswers,
		PathPicked:         r.app.requirePicked,
	}, logw)
	if err != nil {
		return nil, err
	}
	return r.resultView(res), nil
}

// runResultView flattens the engine result for the frontend.
func runResultView(res *run.Result) *RunResultView {
	return &RunResultView{
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
		WallMS:           res.WallMS,
		Turns:            res.Turns,
		AttestationState: res.AttestationState,
	}
}

// MCPToolRow is one granted gateway tool and the declaration granting it.
type MCPToolRow struct {
	Tool      string `json:"tool"`
	GrantedBy string `json:"grantedBy"`
}

// MCPView feeds the agent page's "MCP servers & tools" section (req 3/4
// parity with the site).
type MCPView struct {
	Ref      string       `json:"ref"`
	Version  string       `json:"version"`
	ToolRows []MCPToolRow `json:"toolRows"`
	Servers  []string     `json:"servers"`
	// DeclaredServers is each declared MCP server's pin, tools and plain
	// env; Servers ends with one line per entry, in order
	// (agentry-desktop#62).
	DeclaredServers []mcpview.Server `json:"declaredServers"`
	SecretNames     []string         `json:"secretNames"`
	DataAccess      []string         `json:"dataAccess"`
	EmbeddedTools   []string         `json:"embeddedTools"`
	CapabilityLines []string         `json:"capabilityLines"`
	// Vulnerabilities is the registry's verdict on each declared MCP server;
	// Refusal is the text a run meets by default (agentry#177).
	Vulnerabilities   []mcpvuln.Status `json:"vulnerabilities"`
	VulnerabilityLine string           `json:"vulnerabilityLine,omitempty"`
	Refusal           string           `json:"refusal,omitempty"`
}

// GatewayRequirements computes the tool surface the latest published
// version's declared contract becomes — through the runner's per-run
// gateway, and as host built-ins in an embedded install.
func (r *RunAPI) GatewayRequirements(reg, ns, name string) (*MCPView, error) {
	info, err := run.Requirements(r.app.ctx, reg, ns, name)
	if err != nil {
		return nil, err
	}
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
	}
	if v.Vulnerabilities == nil {
		v.Vulnerabilities = []mcpvuln.Status{}
	}
	for _, row := range info.ToolRows {
		v.ToolRows = append(v.ToolRows, MCPToolRow{Tool: row.Tool, GrantedBy: row.GrantedBy})
	}
	return v, nil
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
	buf   bytes.Buffer
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
	emitEvent(w.ctx, w.event, line)
}

// Compile-time check: the engine hands us an io.Writer contract.
var _ io.Writer = (*lineEventWriter)(nil)

// runGates is what RunAgent and StartSession check before the engine is
// asked for anything (D-030): every path came from a native picker, and a
// consent the page passed is put to the person in a native dialog. The
// page's checkboxes remain what they were — the record of what is being
// accepted — and the dialog is the gate.
func (r *RunAPI) runGates(ref string, inputPaths []string, outDir string, acceptUnsigned, allowVulnerableMCP, acceptUnscanned bool) error {
	for _, p := range inputPaths {
		if err := r.app.requirePicked("input", p); err != nil {
			return err
		}
	}
	if err := r.app.requirePicked("output directory", outDir); err != nil {
		return err
	}
	var accepting []string
	if acceptUnsigned {
		accepting = append(accepting, "a release with no attestation")
	}
	if acceptUnscanned {
		accepting = append(accepting, "a release the registry admitted without a scan")
	}
	if allowVulnerableMCP {
		accepting = append(accepting, "a declared MCP server with a known vulnerability")
	}
	if len(accepting) == 0 {
		return nil
	}
	return r.app.askConsent("Run "+ref+" anyway?",
		"This run accepts "+strings.Join(accepting, ", and ")+". The receipt will record the consent. Continue?")
}
