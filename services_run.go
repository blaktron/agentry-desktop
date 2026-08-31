// RunAPI: the bound run surface (RUNNER-PLAN-2 §P6) — setup status and
// one-click setup (§P1), MCP-profile status and guided configuration as a
// form (§P3), the tool-surface section (req 3/4), and run-with-task over
// agentry-cli/pkg/runner. Long operations stream progress as Wails events
// (`runner:setup-progress`, `runner:run-log`); secret VALUES cross this
// boundary only inward (SaveProfile) and are never returned or logged.
package main

import (
	"bytes"
	"errors"
	"io"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry-desktop/internal/run"
)

// Event names the frontend subscribes to.
const (
	eventSetupProgress = "runner:setup-progress"
	eventRunLog        = "runner:run-log"
)

// errBusy refuses a second docker-touching operation instead of silently
// queueing it behind one that can run for many minutes.
var errBusy = errors.New("another run or image setup is already in progress — wait for it to finish")

// RunAPI drives BriefAgent runs from the app.
type RunAPI struct {
	app *App
	// mu serializes the docker-touching operations: one run (or image
	// setup) at a time keeps the log streams and the engine's teardown
	// sweeps unambiguous.
	mu sync.Mutex
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
	BaseURL      string `json:"baseUrl,omitempty"`
	ImagesReady  bool   `json:"imagesReady"`
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
// stream as `runner:setup-progress` events.
func (r *RunAPI) ApplySetup(provider, model, apiKeyEnv, baseURL string, pullImages bool) (*SetupStatusView, error) {
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	err := run.ApplySetup(r.app.ctx, run.SetupOpts{
		Provider:   provider,
		Model:      model,
		APIKeyEnv:  apiKeyEnv,
		BaseURL:    baseURL,
		PullImages: pullImages,
	}, logw)
	if err != nil {
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
}

func profileStatusView(p *run.ProfileInfo) *ProfileStatusView {
	return &ProfileStatusView{
		Ref:               p.Ref,
		Digest:            p.Digest,
		DeclaredSecrets:   emptyNotNil(p.DeclaredSecrets),
		Exists:            p.Exists,
		Keep:              p.Keep,
		StoredSecretNames: emptyNotNil(p.StoredSecretNames),
		CapabilityLines:   emptyNotNil(p.CapabilityLines),
	}
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
func (r *RunAPI) RunAgent(reg, ref, task string, inputPaths []string, outDir string, keepProfile *bool, timeoutSec, maxTurns int) (*RunResultView, error) {
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
	logw := r.eventWriter(eventRunLog)
	defer logw.Flush()
	res, err := run.Run(r.app.ctx, run.Request{
		Registry:       reg,
		Ref:            ref,
		Task:           task,
		InputPaths:     inputPaths,
		OutDir:         outDir,
		KeepProfile:    keepProfile,
		TimeoutSeconds: timeoutSec,
		MaxTurns:       maxTurns,
	}, logw)
	if err != nil {
		return nil, err
	}
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
	}, nil
}

// MCPToolRow is one granted gateway tool and the declaration granting it.
type MCPToolRow struct {
	Tool      string `json:"tool"`
	GrantedBy string `json:"grantedBy"`
}

// MCPView feeds the agent page's "MCP servers & tools" section (req 3/4
// parity with the site).
type MCPView struct {
	Ref             string       `json:"ref"`
	Version         string       `json:"version"`
	ToolRows        []MCPToolRow `json:"toolRows"`
	Servers         []string     `json:"servers"`
	SecretNames     []string     `json:"secretNames"`
	DataAccess      []string     `json:"dataAccess"`
	EmbeddedTools   []string     `json:"embeddedTools"`
	CapabilityLines []string     `json:"capabilityLines"`
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
		Ref:             info.Ref,
		Version:         info.Version,
		ToolRows:        []MCPToolRow{},
		Servers:         emptyNotNil(info.Servers),
		SecretNames:     emptyNotNil(info.SecretNames),
		DataAccess:      emptyNotNil(info.DataAccess),
		EmbeddedTools:   emptyNotNil(info.EmbeddedTools),
		CapabilityLines: emptyNotNil(info.CapabilityLines),
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
// so a secret that never enters the log stream (the engine guarantees
// that) never reaches the webview.
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
	runtime.EventsEmit(w.ctx.ctx, w.event, line)
}

// Compile-time check: the engine hands us an io.Writer contract.
var _ io.Writer = (*lineEventWriter)(nil)
