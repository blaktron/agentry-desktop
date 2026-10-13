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

	"github.com/blaktron/agentry-desktop/internal/registry"
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

// Caps are the tier's ceilings for a run (x-runs.yaml `RunCaps`), as this
// app reads them: time and turns. A run In Agentry is stated in credits,
// time and turns only (plan scan-and-run-tweaks.md track B,
// agentry-desktop#457), so the token ceiling the registry may still send is
// not decoded.
type Caps struct {
	TimeoutSeconds int `json:"timeoutSeconds"`
	MaxTurns       int `json:"maxTurns"`
}

// Caller is what a run of the signed-in caller's would hold and what a
// knowledge build, a draft, a session and a Voice Writer run of theirs
// hold or may do. The numbers come from the registry so no client does
// credit arithmetic of its own (plan §4.1). The rates a settlement is
// computed at are the registry's and are not decoded (plan
// scan-and-run-tweaks.md D9).
type Caller struct {
	Tier    string `json:"tier"`
	Account string `json:"account,omitempty"`
	Caps    Caps   `json:"caps"`
	// HoldCredits is 0 for a member of an unmetered organization.
	HoldCredits int64      `json:"holdCredits"`
	Build       *TierPrice `json:"build,omitempty"`
	// Draft is what a hosted draft of the caller's holds, and its caps
	// (agentry-enterprise#183); nil from a registry from before the draft
	// kind.
	Draft *TierPrice `json:"draft,omitempty"`
	// Session says whether the caller's payer may start a hosted session
	// (agentry-enterprise#286); nil from a registry that predates it, which
	// this app offers no session against.
	Session *CallerSession `json:"session,omitempty"`
	// Voice is what the caller's payer may still start of Voice Writer
	// runs this billing cycle (agentry-enterprise#306); nil where the
	// deployment runs none.
	Voice *CallerVoice `json:"voice,omitempty"`
	// Revise says whether the caller's payer may revise a QuickAgent In
	// Agentry (Edit with AI, agentry-enterprise#297): Pro and above. Whom
	// it may revise is per QuickAgent, answered at create. Nil from a
	// registry that predates it, which this app offers no revision against.
	Revise *CallerRevise `json:"revise,omitempty"`
}

// CallerRevise is x-runs.yaml's caller.revise.
type CallerRevise struct {
	Allowed bool `json:"allowed"`
}

// TierPrice is a set of caps and what they hold (x-runs.yaml RunTierPrice).
type TierPrice struct {
	Caps        Caps  `json:"caps"`
	HoldCredits int64 `json:"holdCredits"`
	// Allowed is on a knowledge build's line alone: whether the caller's
	// payer may create knowledge in Agentry (DEV-162); nil from a registry
	// that predates it.
	Allowed *bool `json:"allowed,omitempty"`
}

// BuildPrice is what a knowledge build costs here, to anyone
// (x-runs.yaml RunsStatus.knowledgeBuild): whether the deployment takes
// one, its hold on a metered payer, and the pack scan's credit inside it
// (DEV-150).
type BuildPrice struct {
	Enabled     bool  `json:"enabled"`
	HoldCredits int64 `json:"holdCredits"`
	ScanCredits int64 `json:"scanCredits"`
	// DocumentsRequired is false since agentry-enterprise#296 (plan
	// knowledge-and-drafter.md D15): a build takes documents, a brief, or
	// both. Absent (nil) from a registry that predates it, which needs
	// documents.
	DocumentsRequired *bool `json:"documentsRequired,omitempty"`
	// MaxBriefBytes is a brief's bound there, beside DocumentsRequired.
	MaxBriefBytes int `json:"maxBriefBytes,omitempty"`
	// Web is Search the web on a build (D16, D18), nil from a registry
	// that predates it.
	Web *BuildWebTerms `json:"web,omitempty"`
}

// TakesBrief reports whether the registry builds without documents.
func (p *BuildPrice) TakesBrief() bool {
	return p != nil && p.DocumentsRequired != nil && !*p.DocumentsRequired
}

// BuildWebTerms is the status route's Search the web terms for a build:
// whether this deployment searches for one, the bounds and price of its
// searches, and the note a client shows beside the choice.
type BuildWebTerms struct {
	Enabled          bool   `json:"enabled"`
	MaxSearches      int    `json:"maxSearches"`
	CreditsPerSearch int64  `json:"creditsPerSearch"`
	MaxPages         int    `json:"maxPages"`
	Note             string `json:"note"`
}

// Status is GET /x/runs/status: whether this deployment takes hosted runs,
// the execution plane's shape, and — signed in — the caller's block.
type Status struct {
	Enabled bool `json:"enabled"`
	// HostsOnline counts the sandbox hosts the registry pushes runs to
	// (agentry-enterprise#83), the one kind of runner since the pull
	// worker went (agentry-enterprise#86): the route's workersOnline is
	// always 0 and is not read (agentry-desktop#882).
	HostsOnline int     `json:"hostsOnline"`
	Queued      int     `json:"queued"`
	Caller      *Caller `json:"caller,omitempty"`
	// KnowledgeBuild is absent from a registry from before the build kind.
	KnowledgeBuild *BuildPrice `json:"knowledgeBuild,omitempty"`
	// KnowledgeAttach says whether a run here may attach knowledge packs,
	// and how many (agentry-enterprise#172); absent from a registry from
	// before, where a run In Agentry attaches none.
	KnowledgeAttach *KnowledgeAttach `json:"knowledgeAttach,omitempty"`
	// Draft is a hosted draft's terms here (agentry-enterprise#183), one
	// set for every tier; absent from a registry from before the draft
	// kind, which offers no Describe it In Agentry.
	Draft *DraftTerms `json:"draft,omitempty"`
	// Sessions is a hosted session's terms here (agentry-enterprise#276);
	// absent from a registry from before sessions.
	Sessions *SessionTerms `json:"sessions,omitempty"`
	// VoiceBuild is the Voice Writer's terms here and whether a run may
	// attach a voice (agentry-enterprise#306); nil from a registry that
	// predates it.
	VoiceBuild *VoiceTerms `json:"voiceBuild,omitempty"`
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

// RunnersOnline is how many runners can take a run now: the sandbox hosts.
func (s *Status) RunnersOnline() int { return s.HostsOnline }

// Artefact is one named object of a run: an input or an output.
type Artefact struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

// Usage is the receipt's usage block as this app reads it: the turns and
// the wall time. The token counts and their dollar cost the registry may
// still send are not decoded: a run In Agentry is settled and stated in
// credits (agentry-desktop#457).
type Usage struct {
	WallMS int64 `json:"wallMs"`
	Turns  int   `json:"turns"`
}

// The kinds of run (x-runs.yaml `Run.kind`). A registry from before the
// build kind sends none, and every row it sends is a run.
const (
	KindRun            = "run"
	KindKnowledgeBuild = "knowledge_build"
	KindDraft          = "draft"
	// KindVoiceBuild is a hosted Voice Writer run (agentry-enterprise#306;
	// voice.go).
	KindVoiceBuild = "voice_build"
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
	// Web is a build that could search the web's record of it
	// (agentry-enterprise#296); nil for a build without the web.
	Web *BuildWeb `json:"web,omitempty"`
}

// BuildWeb is what a web build's record counts: the searches the provider
// answered, the page reads the registry let through, and AI Review's reads
// of what came back and what it withheld.
type BuildWeb struct {
	Searched int `json:"searched"`
	Pages    int `json:"pages"`
	Scanned  int `json:"scanned"`
	Withheld int `json:"withheld"`
}

// Run is one hosted run as the account reads it (x-runs.yaml `Run`).
type Run struct {
	ID    string          `json:"id"`
	Kind  string          `json:"kind,omitempty"`
	Build *KnowledgeBuild `json:"build,omitempty"`
	Draft *Draft          `json:"draft,omitempty"`
	// Task is the run's task; a draft's is its description.
	Task            string     `json:"task,omitempty"`
	Ref             string     `json:"ref"`
	Digest          string     `json:"digest"`
	Mode            string     `json:"mode"`
	Status          string     `json:"status"`
	Outcome         string     `json:"outcome,omitempty"`
	ExitClass       string     `json:"exitClass,omitempty"`
	Error           string     `json:"error,omitempty"`
	CancelRequested bool       `json:"cancelRequested"`
	Caps            Caps       `json:"caps"`
	HoldCredits     int64      `json:"holdCredits"`
	SettledCredits  *int64     `json:"settledCredits,omitempty"`
	Usage           *Usage     `json:"usage,omitempty"`
	Receipt         Receipt    `json:"receipt,omitempty"`
	Inputs          []Artefact `json:"inputs"`
	Outputs         []Artefact `json:"outputs"`
	QueuePosition   int        `json:"queuePosition,omitempty"`
	Knowledge       []Attached `json:"knowledge,omitempty"`
	// Session is a hosted session's record; nil on every run that is not
	// one (agentry-enterprise#276).
	Session *Session `json:"session,omitempty"`
	// VoiceBuild is a hosted Voice Writer run's record; Voice is the
	// Writing Voice a run or a session attached (agentry-enterprise#306).
	VoiceBuild *VoiceBuild    `json:"voiceBuild,omitempty"`
	Voice      *AttachedVoice `json:"voice,omitempty"`
	CreatedAt  time.Time      `json:"createdAt"`
	StartedAt  *time.Time     `json:"startedAt,omitempty"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
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

// IsDraft reports whether this row is a hosted draft of the Drafter.
func (r *Run) IsDraft() bool { return r != nil && r.Kind == KindDraft }

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
// copies, so it sends no Inputs), and no Ref, Task or Intake; a draft
// (Kind KindDraft) carries a Description, its Hints and, since
// agentry-enterprise#295, the Knowledge it reads.
type CreateRequest struct {
	Kind        string            `json:"kind,omitempty"`
	Description string            `json:"description,omitempty"`
	Hints       []string          `json:"hints,omitempty"`
	Ref         string            `json:"ref,omitempty"`
	Name        string            `json:"name,omitempty"`
	Pack        string            `json:"pack,omitempty"`
	Continue    string            `json:"continue,omitempty"`
	Task        string            `json:"task,omitempty"`
	Intake      map[string]string `json:"intake,omitempty"`
	Inputs      []string          `json:"inputs,omitempty"`
	// Knowledge are the registry ids of the account's packs in Agentry a
	// run attaches (agentry-enterprise#172, Knowledge M7); a build takes
	// none.
	Knowledge []string `json:"knowledge,omitempty"`
	// Mode is ModeInteractive for a hosted session of Ref's release
	// (agentry-enterprise#276); empty is a run.
	Mode string `json:"mode,omitempty"`
	// Voice asks a run or a session to write in the runner's live Writing
	// Voice; Op, Brief and Note are a Voice Writer run's (KindVoiceBuild:
	// create, improve or edit; agentry-enterprise#306).
	Voice bool `json:"voice,omitempty"`
	// AskOptionalSecrets asks the registry to refuse a run or a session,
	// 409 secret-missing, while the account keeps no value for one of the
	// release's optional secrets too (agentry-enterprise#517): the token
	// cannot list the account's secrets (DEV-165), so this is how a start
	// learns which optional keys to ask for (agentry-desktop#795).
	AskOptionalSecrets bool `json:"ask_optional_secrets,omitempty"`
	// SourceTokens are a run's access tokens for its input sources, by
	// the id of the repository or url question its Intake answers
	// (agentry-enterprise#516, agentry-desktop#833): the registry holds
	// each in the vault for this run alone and hands it to that source's
	// fetch on the sandbox host. Never in Intake, never logged, and
	// scrubbed from any refusal the create answers.
	SourceTokens map[string]string `json:"sourceTokens,omitempty"`
	Op           string            `json:"op,omitempty"`
	Brief        string            `json:"brief,omitempty"`
	Note         string            `json:"note,omitempty"`
	// Web is a knowledge build's Search the web choice
	// (agentry-enterprise#296); nil leaves it to the registry's default, on
	// for a new pack with no documents and off otherwise. A knowledge
	// build's Brief is what the pack should cover and for whom.
	Web *bool `json:"web,omitempty"`
	// Revises, Instructions and Problems make a draft a revision of a
	// QuickAgent's current release (agentry-enterprise#297, R3; Edit with
	// AI, agentry-desktop#501): Revises is its ref, ns/name@version, and a
	// revision carries no Description, Hints or Knowledge.
	Revises      string `json:"revises,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	Problems     string `json:"problems,omitempty"`
}

// IsSession reports whether this row is a hosted session.
func (r *Run) IsSession() bool { return r != nil && r.Mode == ModeInteractive }

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
		HTTP:     &http.Client{Timeout: 2 * time.Minute, CheckRedirect: registry.KeepTokenHome},
		Download: &http.Client{CheckRedirect: registry.KeepTokenHome},
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
		// A registry that echoed the body must not put a token in what
		// the app shows (D-024's rule for a value sent once).
		if r := RefusalOf(err); r != nil {
			for _, tok := range cr.SourceTokens {
				r = scrubRefusal(r, tok)
			}
			return nil, r
		}
		for _, tok := range cr.SourceTokens {
			err = scrub(err, tok)
		}
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
	// Missing are the declared secrets a 409 secret-missing names: the
	// ones the start asks for (agentry-desktop#300); for a 409
	// connection-missing, the servers still to connect (agentry-desktop#667);
	// for a schedule's 409 secret-not-kept, its `names`: the secrets set for
	// one run only, which a schedule cannot use (agentry-desktop#745).
	Missing []string
	// Optional are the release's optional secrets a secrets refusal names
	// (`optional`): those of Missing a run can start without
	// (agentry-desktop#795).
	Optional []string
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
		Type     string   `json:"type"`
		Title    string   `json:"title"`
		Detail   string   `json:"detail"`
		Balance  *int64   `json:"balance"`
		Hold     *int64   `json:"hold"`
		ResetsAt string   `json:"resetsAt"`
		Tier     string   `json:"tier"`
		Missing  []string `json:"missing"`
		Names    []string `json:"names"`
		Optional []string `json:"optional"`
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
	r.Title, r.Detail, r.ResetsAt, r.Tier, r.Missing, r.Optional = p.Title, p.Detail, p.ResetsAt, p.Tier, p.Missing, p.Optional
	if len(r.Missing) == 0 {
		r.Missing = p.Names
	}
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
