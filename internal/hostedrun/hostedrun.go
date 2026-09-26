// Package hostedrun is the desktop's client for a registry's runs
// extension — the In Agentry mode (plan hosted-runs.md §4.1, §4.4, §4.5,
// D10). The app sends a task, the intake answers, the input files and the
// account's bearer token, and reads back a run's status, its receipt and
// its outputs — no log, which a pushed run does not keep (hosted runs in
// production, D12 and D16); the execution plane is the registry's, so
// nothing here starts a container, reads the operator policy or holds a
// model key.
//
// The routes are the closed build's (agentry-enterprise
// schemas/x-runs.yaml), so they are called by hand rather than through
// the core's generated client, the way services_account.go reads the
// billing account. A registry without the extension answers 404 and the
// caller treats the mode as absent.
package hostedrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The plan's input limits (§4.4), enforced in the app before an upload so
// a file over them is refused with the limit named rather than by an
// opaque 413.
const (
	// MaxInputFiles is how many files one run takes.
	MaxInputFiles = 20
	// MaxInputBytes is what they may weigh together.
	MaxInputBytes int64 = 10 << 20
)

// Statuses a run passes through (x-runs.yaml `Run.status`).
const (
	StatusQueued    = "queued"
	StatusClaimed   = "claimed"
	StatusRunning   = "running"
	StatusFinished  = "finished"
	StatusCancelled = "cancelled"
)

// Active reports whether a status is one the account still has a run in —
// the states a reopened panel reattaches to (plan §5).
func Active(status string) bool {
	switch status {
	case StatusQueued, StatusClaimed, StatusRunning:
		return true
	}
	return false
}

// Caps are the tier's ceilings for a run (x-runs.yaml `RunCaps`).
type Caps struct {
	TimeoutSeconds int   `json:"timeoutSeconds"`
	MaxTurns       int   `json:"maxTurns"`
	MaxTokens      int64 `json:"maxTokens"`
}

// Caller is what a run of the signed-in caller's would hold, the two rates
// a settlement is computed at (Knowledge D29, agentry-enterprise#171: the
// single tokensPerCredit is retired), and what a knowledge build of theirs
// holds. The numbers come from the registry so no client does credit
// arithmetic of its own (plan §4.1).
type Caller struct {
	Tier    string `json:"tier"`
	Account string `json:"account,omitempty"`
	Caps    Caps   `json:"caps"`
	// HoldCredits is 0 for a member of an unmetered organization.
	HoldCredits           int64      `json:"holdCredits"`
	InputTokensPerCredit  int64      `json:"inputTokensPerCredit"`
	OutputTokensPerCredit int64      `json:"outputTokensPerCredit"`
	Build                 *TierPrice `json:"build,omitempty"`
}

// TierPrice is a set of caps and what they hold (x-runs.yaml RunTierPrice).
type TierPrice struct {
	Caps        Caps  `json:"caps"`
	HoldCredits int64 `json:"holdCredits"`
}

// BuildPrice is what a knowledge build costs here, to anyone
// (x-runs.yaml RunsStatus.knowledgeBuild): whether the deployment takes
// one, its hold on a metered payer, and the pack scan's credit inside it
// (DEV-150).
type BuildPrice struct {
	Enabled     bool  `json:"enabled"`
	HoldCredits int64 `json:"holdCredits"`
	ScanCredits int64 `json:"scanCredits"`
}

// Status is GET /x/runs/status: whether this deployment takes hosted runs,
// the execution plane's shape, and — signed in — the caller's block.
type Status struct {
	Enabled bool `json:"enabled"`
	// WorkersOnline counts the pull workers and HostsOnline the sandbox
	// hosts the registry pushes runs to (agentry-enterprise#83); either
	// runs a queued run, so RunnersOnline is what a person is told.
	WorkersOnline int     `json:"workersOnline"`
	HostsOnline   int     `json:"hostsOnline"`
	Queued        int     `json:"queued"`
	Caller        *Caller `json:"caller,omitempty"`
	// KnowledgeBuild is absent from a registry from before the build kind.
	KnowledgeBuild *BuildPrice `json:"knowledgeBuild,omitempty"`
	// KnowledgeAttach says whether a run here may attach knowledge packs,
	// and how many (agentry-enterprise#172); absent from a registry from
	// before, where a run In Agentry attaches none.
	KnowledgeAttach *KnowledgeAttach `json:"knowledgeAttach,omitempty"`
}

// KnowledgeAttach is the status route's knowledgeAttach block.
type KnowledgeAttach struct {
	Enabled  bool `json:"enabled"`
	MaxPacks int  `json:"maxPacks"`
}

// Attached is one knowledge pack a run attaches, as the registry records
// it: the account's pack by its registry id and name, and from the run's
// hand-off to a sandbox host the digest of the tree it was given.
type Attached struct {
	Pack   string `json:"pack"`
	Name   string `json:"name"`
	Digest string `json:"digest,omitempty"`
}

// RunnersOnline is how many runners, of either kind, can take a run now.
func (s *Status) RunnersOnline() int { return s.WorkersOnline + s.HostsOnline }

// Artefact is one named object of a run: an input or an output.
type Artefact struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// Usage is the receipt's usage block as the worker reported it. A harness
// that reports the split and not the sum leaves TotalTokens at 0, which is
// why Tokens() exists.
type Usage struct {
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	TotalTokens  int64   `json:"totalTokens"`
	CostUSD      float64 `json:"costUsd"`
	WallMS       int64   `json:"wallMs"`
	Turns        int     `json:"turns"`
}

// Tokens is the run's token count for display: the reported total, or the
// sum of the split when only the split was reported.
func (u *Usage) Tokens() int64 {
	if u == nil {
		return 0
	}
	if u.TotalTokens > 0 {
		return u.TotalTokens
	}
	return u.InputTokens + u.OutputTokens
}

// The kinds of run (x-runs.yaml `Run.kind`). A registry from before the
// build kind sends none, and every row it sends is a run.
const (
	KindRun            = "run"
	KindKnowledgeBuild = "knowledge_build"
)

// KnowledgeBuild is a hosted knowledge build's record (x-runs.yaml
// `KnowledgeBuild`): the pack it builds into, the tree it was handed, and
// what became of the pack. Applied says the tree it made is the pack's now
// and was handed to the scan; Note says why not, when it is not.
type KnowledgeBuild struct {
	Pack        string `json:"pack"`
	Name        string `json:"name"`
	From        string `json:"from"`
	Applied     bool   `json:"applied"`
	Digest      string `json:"digest,omitempty"`
	Pages       int    `json:"pages,omitempty"`
	Bytes       int64  `json:"bytes,omitempty"`
	ScanCredits int64  `json:"scanCredits,omitempty"`
	Note        string `json:"note,omitempty"`
}

// Run is one hosted run as the account reads it (x-runs.yaml `Run`).
type Run struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind,omitempty"`
	Build           *KnowledgeBuild `json:"build,omitempty"`
	Ref             string          `json:"ref"`
	Digest          string          `json:"digest"`
	Mode            string          `json:"mode"`
	Status          string          `json:"status"`
	Outcome         string          `json:"outcome,omitempty"`
	ExitClass       string          `json:"exitClass,omitempty"`
	Error           string          `json:"error,omitempty"`
	CancelRequested bool            `json:"cancelRequested"`
	Caps            Caps            `json:"caps"`
	HoldCredits     int64           `json:"holdCredits"`
	SettledCredits  *int64          `json:"settledCredits,omitempty"`
	Usage           *Usage          `json:"usage,omitempty"`
	Receipt         Receipt         `json:"receipt,omitempty"`
	Inputs          []Artefact      `json:"inputs"`
	Outputs         []Artefact      `json:"outputs"`
	QueuePosition   int             `json:"queuePosition,omitempty"`
	Knowledge       []Attached      `json:"knowledge,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	StartedAt       *time.Time      `json:"startedAt,omitempty"`
	FinishedAt      *time.Time      `json:"finishedAt,omitempty"`
}

// Empty reports whether the registry kept no receipt for the run: a run
// pushed before agentry-enterprise#121, whose host sent no receipt. The
// result.json in a pushed run's output.tar is a file the agent could
// have written, so it is never read as the receipt instead.
func (r Receipt) Empty() bool {
	return len(r.CapabilitiesEnforced) == 0 && r.Attestation.State == "" && r.Harness.Adapter == "" && r.ExitClass == ""
}

// IsBuild reports whether this row is a knowledge build, not a run.
func (r *Run) IsBuild() bool { return r != nil && r.Kind == KindKnowledgeBuild }

// Terminal reports whether the run has stopped moving.
func (r *Run) Terminal() bool { return r != nil && !Active(r.Status) }

// Receipt is the runner's envelope minus its result, kept verbatim by the
// registry. Only the parts a client shows are named; the rest rides along
// untouched so nothing is summarised away (plan §9).
type Receipt struct {
	CapabilitiesEnforced map[string]string `json:"capabilities_enforced,omitempty"`
	Attestation          struct {
		State string `json:"state"`
		KeyID string `json:"keyId,omitempty"`
	} `json:"attestation"`
	Harness struct {
		Adapter string `json:"adapter,omitempty"`
		Model   string `json:"model,omitempty"`
	} `json:"harness"`
	ExitClass string `json:"exitClass,omitempty"`
}

// Input is one stored upload (x-runs.yaml `RunInput`).
type Input struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// CreateRequest is the body of POST /x/runs/: a run names a Ref; a
// knowledge build (Kind KindKnowledgeBuild) names exactly one of Name (a
// new pack), Pack (the account's pack in Agentry, by its registry id) or
// Continue (a finished or cancelled build, whose documents the registry
// copies, so it sends no Inputs), and no Ref, Task or Intake.
type CreateRequest struct {
	Kind     string            `json:"kind,omitempty"`
	Ref      string            `json:"ref,omitempty"`
	Name     string            `json:"name,omitempty"`
	Pack     string            `json:"pack,omitempty"`
	Continue string            `json:"continue,omitempty"`
	Task     string            `json:"task,omitempty"`
	Intake   map[string]string `json:"intake,omitempty"`
	Inputs   []string          `json:"inputs,omitempty"`
	// Knowledge are the registry ids of the account's packs in Agentry a
	// run attaches (agentry-enterprise#172, Knowledge M7); a build takes
	// none.
	Knowledge []string `json:"knowledge,omitempty"`
}

// Client talks to one registry's runs extension as one account.
type Client struct {
	base  string
	token string
	// HTTP bounds the short calls. Download has no whole-response
	// timeout: a pushed run's outputs come down as one bundle, whose size
	// is the run's and whose speed is the link's.
	HTTP     *http.Client
	Download *http.Client
}

// New returns a client for a registry root ("https://…", no /api path)
// and the account's PAT. An empty token is an anonymous client, which the
// status route still answers.
func New(regURL, token string) *Client {
	return &Client{
		base:     strings.TrimRight(regURL, "/"),
		token:    token,
		HTTP:     &http.Client{Timeout: 2 * time.Minute},
		Download: &http.Client{},
	}
}

// path builds an absolute URL under the extension's mount. Each segment is
// escaped, so an artefact name with slashes survives as a path and a name
// with anything else in it cannot leave the route.
func (c *Client) path(segments ...string) string {
	u := c.base + "/api/v1/x/runs"
	for _, s := range segments {
		parts := strings.Split(s, "/")
		for i, p := range parts {
			parts[i] = url.PathEscape(p)
		}
		u += "/" + strings.Join(parts, "/")
	}
	return u
}

// request issues one call with the account's bearer token.
func (c *Client) request(ctx context.Context, method, url string, body io.Reader, contentType string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// do performs a request and decodes a JSON answer, turning any non-2xx
// into a *Refusal.
func (c *Client) do(op string, req *http.Request, want int, out any) error {
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer func() { _ = res.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != want {
		return refusalFrom(op, res.StatusCode, body)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: the registry's answer could not be read: %w", op, err)
	}
	return nil
}

// Status reads GET /x/runs/status. A registry without the extension
// answers 404, which arrives as a *Refusal the caller reads as "no hosted
// runs here".
func (c *Client) Status(ctx context.Context) (*Status, error) {
	req, err := c.request(ctx, http.MethodGet, c.path("status"), nil, "")
	if err != nil {
		return nil, err
	}
	var st Status
	if err := c.do("read the hosted-run status", req, http.StatusOK, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// Create queues a run. A refusal the panel renders as a sentence (402
// credits-exhausted, 409 run-active, 409 secrets-declared, …) comes back
// as a *Refusal.
func (c *Client) Create(ctx context.Context, cr CreateRequest) (*Run, error) {
	body, err := json.Marshal(cr)
	if err != nil {
		return nil, err
	}
	// The create is POST /api/v1/x/runs/ — the trailing slash is the
	// route, not an accident.
	req, err := c.request(ctx, http.MethodPost, c.path()+"/", strings.NewReader(string(body)), "application/json")
	if err != nil {
		return nil, err
	}
	var run Run
	if err := c.do("start the run", req, http.StatusCreated, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// List returns the account's runs, newest first.
func (c *Client) List(ctx context.Context) ([]Run, error) {
	req, err := c.request(ctx, http.MethodGet, c.path()+"/", nil, "")
	if err != nil {
		return nil, err
	}
	var out struct {
		Items []Run `json:"items"`
	}
	if err := c.do("list the account's runs", req, http.StatusOK, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// Get reads one run.
func (c *Client) Get(ctx context.Context, id string) (*Run, error) {
	req, err := c.request(ctx, http.MethodGet, c.path(id), nil, "")
	if err != nil {
		return nil, err
	}
	var run Run
	if err := c.do("read the run", req, http.StatusOK, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// Cancel asks for the run to stop: queued, it is cancelled and the hold
// released; running, the cancel is recorded and the worker interrupts.
func (c *Client) Cancel(ctx context.Context, id string) (*Run, error) {
	req, err := c.request(ctx, http.MethodPost, c.path(id, "cancel"), nil, "")
	if err != nil {
		return nil, err
	}
	var run Run
	if err := c.do("cancel the run", req, http.StatusOK, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

// UploadInput stores one input file for a run this account will create.
// The body is the file; name is how the run sees it under /inputs.
func (c *Client) UploadInput(ctx context.Context, name string, body io.Reader, size int64) (*Input, error) {
	req, err := c.request(ctx, http.MethodPost, c.path("inputs")+"?name="+url.QueryEscape(name), body, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	req.ContentLength = size
	var in Input
	if err := c.do("upload "+name, req, http.StatusCreated, &in); err != nil {
		return nil, err
	}
	return &in, nil
}

// Output opens one artefact of the run. The caller closes the reader.
func (c *Client) Output(ctx context.Context, id, name string) (io.ReadCloser, error) {
	req, err := c.request(ctx, http.MethodGet, c.path(id, "outputs", name), nil, "")
	if err != nil {
		return nil, err
	}
	res, err := c.Download.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", name, err)
	}
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
		_ = res.Body.Close()
		return nil, refusalFrom("download "+name, res.StatusCode, body)
	}
	return res.Body, nil
}

// Refusal is an RFC 9457 problem from the extension, kept whole: the slug
// a caller switches on, the sentence it shows, and the billing fields a
// 402 carries.
type Refusal struct {
	Op     string
	Status int
	Slug   string
	Title  string
	Detail string
	// Balance, Hold, ResetsAt and Tier are set by a 402
	// credits-exhausted; Known says the problem carried them.
	Balance  int64
	Hold     int64
	ResetsAt string
	Tier     string
	Known    bool
}

// Sentence is the refusal as one line of prose, with no JSON in it.
func (r *Refusal) Sentence() string {
	switch {
	case r.Title != "" && r.Detail != "":
		return r.Title + ": " + r.Detail
	case r.Title != "":
		return r.Title
	case r.Detail != "":
		return r.Detail
	default:
		return fmt.Sprintf("the registry answered %d", r.Status)
	}
}

func (r *Refusal) Error() string {
	return fmt.Sprintf("%s: %s (HTTP %d)", r.Op, r.Sentence(), r.Status)
}

// refusalFrom parses a problem body; a body that is not a problem still
// yields a refusal, carrying what the registry said.
func refusalFrom(op string, status int, body []byte) *Refusal {
	r := &Refusal{Op: op, Status: status}
	var p struct {
		Type     string `json:"type"`
		Title    string `json:"title"`
		Detail   string `json:"detail"`
		Balance  *int64 `json:"balance"`
		Hold     *int64 `json:"hold"`
		ResetsAt string `json:"resetsAt"`
		Tier     string `json:"tier"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		r.Detail = strings.TrimSpace(string(body))
		if len(r.Detail) > 300 {
			r.Detail = r.Detail[:300] + "…"
		}
		return r
	}
	r.Slug = p.Type
	if i := strings.LastIndex(r.Slug, "/"); i >= 0 {
		r.Slug = r.Slug[i+1:]
	}
	r.Title, r.Detail, r.ResetsAt, r.Tier = p.Title, p.Detail, p.ResetsAt, p.Tier
	if p.Balance != nil {
		r.Balance, r.Known = *p.Balance, true
	}
	if p.Hold != nil {
		r.Hold = *p.Hold
	}
	return r
}

// RefusalOf unwraps a refusal from an error, or returns nil: the caller
// switches on the slug rather than matching on a message.
func RefusalOf(err error) *Refusal {
	var r *Refusal
	if errors.As(err, &r) {
		return r
	}
	return nil
}
