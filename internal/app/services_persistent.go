// The persistent agents the app runs: each a supervisor of the CLI's
// persistent package, in this process, with its own owner lock and its own
// segment's containers. None takes the run lock, so an ordinary run, chat,
// draft or build goes on beside a running agent, and a running agent goes on
// beside them. The page drives an agent by its name; its events reach the
// page as runner:agent-event, keyed by the agent, and runner:agent-state
// says one started or stopped.
//
// Each consent is the app's own native dialog, raised by the binding that
// needs it: a start whose roster turns on what the person has not agreed to
// on this computer, a change that widens it, and a reset. The share folder
// comes from the native picker alone.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/session"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

const (
	// eventAgentEvent carries an PersistentEventView; eventAgentState names an
	// agent that started or stopped.
	eventAgentEvent = "runner:agent-event"
	eventAgentState = "runner:agent-state"
)

// shownAgents are the persistent agents the app offers, in their order.
var shownAgents = []string{"assistant"}

// transcriptLines is how much of an agent's transcript its page reloads.
const transcriptLines = 500

// stopWait bounds a stop at quit, so a stuck segment cannot hold the app.
const stopWait = 60 * time.Second

// PersistentAPI runs the app's persistent agents.
type PersistentAPI struct {
	app *App
	run *RunAPI

	mu       sync.Mutex
	sups     map[string]agentSupervisor
	starting map[string]bool
	// followers are the goroutines that carry each agent's events.
	followers sync.WaitGroup

	// start starts a supervisor; nil is persistent.Start. A test puts its
	// own here.
	start func(ctx context.Context, opts persistent.Options) (agentSupervisor, error)
	// docker is the engine client a reset removes the volumes through; nil
	// is the docker CLI.
	docker dockerx.Client
}

// agentSupervisor is what the app needs of a running agent: the CLI's
// persistent.Supervisor.
type agentSupervisor interface {
	Send(ctx context.Context, text string) error
	Subscribe() (<-chan session.Event, func())
	Status() persistent.Status
	Roster() persistent.Roster
	Done() <-chan struct{}
	Stop(ctx context.Context) (*receipt.Envelope, error)
}

// PersistentAgentView is how an agent stands, for Settings and its page.
type PersistentAgentView struct {
	Name string `json:"name"`
	// Title is the agent's own name ("Assistant"); DisplayName the one the
	// person gave it, else Title.
	Title       string `json:"title"`
	DisplayName string `json:"displayName"`
	// State is disabled, stopped, starting, running, or elsewhere: running
	// under another client (the CLI's supervisor), which Holder names.
	State       string `json:"state"`
	Holder      string `json:"holder,omitempty"`
	Personality string `json:"personality,omitempty"`
	// Share is the share folder; DefaultShare says it is the agent's own
	// default.
	Share        string                  `json:"share"`
	DefaultShare bool                    `json:"defaultShare"`
	Knowledge    []string                `json:"knowledge"`
	Servers      []PersistentServerView  `json:"servers"`
	Roster       []PersistentRosterEntry `json:"roster"`
	Status       *PersistentStatusView   `json:"status,omitempty"`
	Lifetime     PersistentUsageView     `json:"lifetime"`
	Volumes      PersistentVolumesView   `json:"volumes"`
}

// PersistentServerView is an MCP server the person added.
type PersistentServerView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PersistentRosterEntry is one roster entry.
type PersistentRosterEntry struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	GrantedBy string `json:"grantedBy"`
	State     string `json:"state"`
	On        bool   `json:"on"`
	Line      string `json:"line"`
	Fixed     bool   `json:"fixed,omitempty"`
}

// PersistentStatusView is a running agent's segment.
type PersistentStatusView struct {
	Since        string `json:"since"`
	Segment      int    `json:"segment"`
	Run          string `json:"run"`
	Adapter      string `json:"adapter"`
	Model        string `json:"model"`
	Resumed      bool   `json:"resumed"`
	ResumeReason string `json:"resumeReason,omitempty"`
	Busy         bool   `json:"busy"`
	Pending      bool   `json:"pending"`
	// The segment's usage so far, as its envelope will sum it.
	InputTokens  int64   `json:"inputTokens"`
	OutputTokens int64   `json:"outputTokens"`
	TotalTokens  int64   `json:"totalTokens"`
	CostUSD      float64 `json:"costUsd"`
	UserTurns    int     `json:"userTurns"`
	ToolCalls    int     `json:"toolCalls"`
}

// PersistentUsageView is the agent's lifetime usage, summed over its
// segments.
type PersistentUsageView struct {
	Segments     int   `json:"segments"`
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	TotalTokens  int64 `json:"totalTokens"`
	UserTurns    int   `json:"userTurns"`
	ToolCalls    int   `json:"toolCalls"`
}

// PersistentVolumesView names the agent's two volumes.
type PersistentVolumesView struct {
	Session string `json:"session"`
	Data    string `json:"data"`
}

// PersistentEventView is one runner:agent-event payload.
type PersistentEventView struct {
	Agent string           `json:"agent"`
	Event SessionEventView `json:"event"`
}

// PersistentChange is one change the page asks for; a field left unset is left
// as it is.
type PersistentChange struct {
	Name        *string `json:"name,omitempty"`
	Personality *string `json:"personality,omitempty"`
	// DefaultShare puts the share folder back to the agent's default.
	DefaultShare    bool              `json:"defaultShare,omitempty"`
	AddKnowledge    string            `json:"addKnowledge,omitempty"`
	RemoveKnowledge string            `json:"removeKnowledge,omitempty"`
	RemoveServer    string            `json:"removeServer,omitempty"`
	Switch          *PersistentSwitch `json:"switch,omitempty"`
}

// PersistentSwitch turns a roster entry on or off.
type PersistentSwitch struct {
	ID string `json:"id"`
	On bool   `json:"on"`
}

func (p *PersistentAPI) home() (string, error) { return p.run.runnerHome() }

// agent is a persistent agent the app offers.
func (p *PersistentAPI) agent(name string) (persistent.Agent, error) {
	if !slices.Contains(shownAgents, name) {
		return persistent.Agent{}, fmt.Errorf("no persistent agent %q in this app", name)
	}
	return persistent.Lookup(name)
}

// List is every agent the app offers, as it stands.
func (p *PersistentAPI) List() ([]PersistentAgentView, error) {
	out := make([]PersistentAgentView, 0, len(shownAgents))
	for _, name := range shownAgents {
		v, err := p.Get(name)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// Get is one agent as it stands.
func (p *PersistentAPI) Get(name string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err != nil {
		return nil, err
	}
	v := &PersistentAgentView{
		Name: a.Name, Title: a.Title, DisplayName: a.DisplayName(cfg), Personality: cfg.Personality,
		DefaultShare: cfg.Share == "", Knowledge: append([]string{}, cfg.Knowledge...), Servers: []PersistentServerView{},
		Volumes: PersistentVolumesView{Session: persistent.SessionVolume(a.Name), Data: persistent.DataVolume(a.Name)},
	}
	v.Share, _ = persistent.ShareFolder(a, cfg)
	if u, err := persistent.LoadUsage(home, a.Name); err == nil {
		v.Lifetime = PersistentUsageView{Segments: u.Segments, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, TotalTokens: u.TotalTokens, UserTurns: u.UserTurns, ToolCalls: u.ToolCalls}
	}
	roster := persistent.BuildRoster(a, cfg, nil)
	p.mu.Lock()
	sv, ours := p.sups[a.Name]
	starting := p.starting[a.Name]
	p.mu.Unlock()
	switch {
	case ours:
		v.State = persistent.StateRunning
		roster = sv.Roster()
		st := sv.Status()
		v.Status = statusView(st)
	case starting:
		v.State = "starting"
	default:
		if held, holder := persistent.Held(home, a.Name); held {
			v.State = "elsewhere"
			if holder != nil {
				v.Holder = holder.Where()
			}
		} else if cfg.Enabled {
			v.State = "stopped"
		} else {
			v.State = "disabled"
		}
	}
	v.Roster = rosterView(roster)
	for _, e := range roster {
		if e.Kind == persistent.KindServer {
			v.Servers = append(v.Servers, PersistentServerView{ID: e.ID, Name: e.Name})
		}
	}
	return v, nil
}

func statusView(st persistent.Status) *PersistentStatusView {
	return &PersistentStatusView{
		Since: st.Since.Format(time.RFC3339), Segment: st.Segment, Run: st.Run, Adapter: st.Adapter, Model: st.Model,
		Resumed: st.Resumed, ResumeReason: st.ResumeReason, Busy: st.Busy, Pending: st.Pending,
		InputTokens: st.Usage.InputTokens, OutputTokens: st.Usage.OutputTokens, TotalTokens: st.Usage.TotalTokens,
		CostUSD: st.Usage.CostUSD, UserTurns: st.Usage.UserTurns, ToolCalls: st.Usage.Turns,
	}
}

func rosterView(r persistent.Roster) []PersistentRosterEntry {
	out := make([]PersistentRosterEntry, 0, len(r))
	for _, e := range r {
		out = append(out, PersistentRosterEntry{
			ID: e.ID, Kind: string(e.Kind), Name: e.Name, GrantedBy: e.GrantedBy,
			State: string(e.State), On: e.On, Line: e.Line, Fixed: e.Fixed,
		})
	}
	return out
}

// SetEnabled enables or disables an agent. A running agent is not
// disabled: it is stopped first.
func (p *PersistentAPI) SetEnabled(name string, on bool) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	_, here := p.running(a.Name)
	if held, _ := persistent.Held(home, a.Name); (held || here) && !on {
		return nil, fmt.Errorf("the %s is running: stop it first", a.Title)
	}
	owner, err := persistent.Acquire(home, a.Name, persistent.ClientDesktop)
	if err != nil {
		return nil, err
	}
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err == nil {
		cfg.Enabled = on
		err = persistent.SaveConfig(home, a.Name, cfg)
	}
	owner.Release()
	if err != nil {
		return nil, err
	}
	return p.Get(name)
}

// Start starts an agent: a roster with entries the person has not agreed to
// on this computer asks first, in a native dialog, and records the
// agreement. Its events then stream as runner:agent-event until it stops.
func (p *PersistentAPI) Start(name string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	if _, ok := p.sups[a.Name]; ok || p.starting[a.Name] {
		p.mu.Unlock()
		return p.Get(name)
	}
	if p.starting == nil {
		p.starting = map[string]bool{}
	}
	p.starting[a.Name] = true
	p.mu.Unlock()
	emitEvent(p.app, eventAgentState, a.Name)
	defer func() {
		p.mu.Lock()
		delete(p.starting, a.Name)
		p.mu.Unlock()
		emitEvent(p.app, eventAgentState, a.Name)
	}()
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("the %s is not enabled: enable it in Settings → Persistent Agents", a.Title)
	}
	if pending := persistent.Unconsented(persistent.BuildRoster(a, cfg, nil), cfg); len(pending) > 0 {
		if err := p.app.askConsent("Start the "+a.DisplayName(cfg)+"?", consentText(a.DisplayName(cfg), pending)); err != nil {
			return nil, err
		}
		if err := persistent.AgreeToRoster(home, a, persistent.ClientDesktop); err != nil {
			return nil, err
		}
	}
	opts, err := p.options(home, a)
	if err != nil {
		return nil, err
	}
	sv, err := p.startSupervisor(context.Background(), opts)
	var consent *persistent.ConsentError
	if errors.As(err, &consent) {
		return nil, fmt.Errorf("the %s's roster changed while it started: start it again", a.Title)
	}
	if err != nil {
		return nil, err
	}
	p.follow(a.Name, sv)
	return p.Get(name)
}

// consentText is the native dialog's message for entries to agree to: the
// roster's own sentences, one a line.
func consentText(who string, entries []persistent.RosterEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s would be given, for the first time on this computer:\n", who)
	for _, e := range entries {
		fmt.Fprintf(&b, "\n• %s", e.Line)
	}
	return b.String()
}

// options are a supervisor's options in this app: the runner home's
// policy, the machine's engine, the local store's packs under the active
// registry.
func (p *PersistentAPI) options(home string, a persistent.Agent) (persistent.Options, error) {
	polFile, err := policy.Resolve("")
	if err != nil {
		return persistent.Options{}, err
	}
	pol, sha, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return persistent.Options{}, fmt.Errorf("operator policy: %w", err)
	}
	return persistent.Options{
		Home: home, Agent: a, Client: persistent.ClientDesktop,
		Policy: pol, PolicySHA: sha, PolicyFile: &polFile, Build: version.Build(),
		Docker: p.dockerClient(), Stderr: agentLog(), Packs: p.packs,
	}, nil
}

// packs opens the packs a roster has on, from the local store under the
// registry the app is on.
func (p *PersistentAPI) packs(ctx context.Context, names []string) ([]engine.AttachedPack, func(), error) {
	reg := ""
	if s, err := settings.Load(); err == nil {
		reg = s.DefaultRegistry
	}
	return p.run.knowledge().Attach(ctx, reg, names)
}

func (p *PersistentAPI) dockerClient() dockerx.Client {
	if p.docker != nil {
		return p.docker
	}
	return dockerx.New()
}

// agentLog is where a segment's progress goes: the app's own stderr.
func agentLog() io.Writer { return os.Stderr }

// startSupervisor starts one, through the test's seam where there is one.
func (p *PersistentAPI) startSupervisor(ctx context.Context, opts persistent.Options) (agentSupervisor, error) {
	if p.start != nil {
		return p.start(ctx, opts)
	}
	sv, err := persistent.Start(ctx, opts)
	if err != nil {
		// Not the nil *Supervisor in the interface: that would read as one.
		return nil, err
	}
	return sv, nil
}

// follow keeps a started agent: its events go to the page keyed by its
// name until it stops, by Stop or by itself, and then it is let go.
func (p *PersistentAPI) follow(name string, sv agentSupervisor) {
	events, unsubscribe := sv.Subscribe()
	p.mu.Lock()
	if p.sups == nil {
		p.sups = map[string]agentSupervisor{}
	}
	p.sups[name] = sv
	p.mu.Unlock()
	p.followers.Add(2)
	go func() {
		defer p.followers.Done()
		for ev := range events {
			emitEvent(p.app, eventAgentEvent, PersistentEventView{Agent: name, Event: eventView(ev)})
		}
	}()
	go func() {
		defer p.followers.Done()
		<-sv.Done()
		unsubscribe()
		p.mu.Lock()
		if p.sups[name] == sv {
			delete(p.sups, name)
		}
		p.mu.Unlock()
		emitEvent(p.app, eventAgentState, name)
	}()
}

// running is the agent's supervisor in this app, if it runs here.
func (p *PersistentAPI) running(name string) (agentSupervisor, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	sv, ok := p.sups[name]
	return sv, ok
}

// Stop stops an agent this app runs: its segment ends without a final
// result and its volumes stay, so the next start continues the
// conversation.
func (p *PersistentAPI) Stop(name string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	sv, ok := p.running(a.Name)
	if !ok {
		return p.Get(name)
	}
	_, err = sv.Stop(context.Background())
	p.mu.Lock()
	if p.sups[a.Name] == sv {
		delete(p.sups, a.Name)
	}
	p.mu.Unlock()
	emitEvent(p.app, eventAgentState, a.Name)
	if err != nil {
		return nil, err
	}
	return p.Get(name)
}

// Send sends the person's message as the agent's next turn and returns
// once it is answered; its events stream meanwhile. A change waiting for
// the next turn is applied first, which the stream says.
func (p *PersistentAPI) Send(name, text string) error {
	a, err := p.agent(name)
	if err != nil {
		return err
	}
	sv, ok := p.running(a.Name)
	if !ok {
		return fmt.Errorf("the %s is not running: start it first", a.Title)
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("the message is empty")
	}
	return sv.Send(context.Background(), text)
}

// Configure applies a change, running or not: a running agent takes it at
// its next turn. A change that turns on what the person has not agreed to
// on this computer asks first, in a native dialog.
func (p *PersistentAPI) Configure(name string, ch PersistentChange) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	c := persistent.Change{Name: ch.Name, Personality: ch.Personality, AddKnowledge: ch.AddKnowledge, RemoveKnowledge: ch.RemoveKnowledge, RemoveServer: ch.RemoveServer}
	if ch.DefaultShare {
		empty := ""
		c.Share = &empty
	}
	if ch.Switch != nil {
		c.Switch = &persistent.Switch{ID: ch.Switch.ID, On: ch.Switch.On}
	}
	if err := p.configure(a, c); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// configure applies one change, asking the person where it widens the
// roster.
func (p *PersistentAPI) configure(a persistent.Agent, c persistent.Change) error {
	home, err := p.home()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, _, err = persistent.Configure(ctx, home, a, persistent.ClientDesktop, c)
	var consent *persistent.ConsentError
	if !errors.As(err, &consent) {
		return err
	}
	cfg, _ := persistent.LoadConfig(home, a.Name)
	if err := p.app.askConsent("Give the "+a.DisplayName(cfg)+" this?", consentText(a.DisplayName(cfg), consent.Entries)); err != nil {
		return err
	}
	c.Consent = true
	_, _, err = persistent.Configure(ctx, home, a, persistent.ClientDesktop, c)
	return err
}

// ChooseShare opens the native folder picker for the agent's share folder
// and saves the folder picked, held to the share folder's guards; a cancel
// changes nothing.
func (p *PersistentAPI) ChooseShare(name string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	dir, err := p.app.chooseDirectory("The " + a.Title + "'s share folder")
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return p.Get(name)
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	if err := persistent.CheckShareFolder(home, dir); err != nil {
		return nil, err
	}
	if err := p.configure(a, persistent.Change{Share: &dir}); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// Reset wipes a stopped agent's conversation and files, after a native
// confirmation: its two volumes and its transcript. Its receipts stay.
func (p *PersistentAPI) Reset(name string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	_, here := p.running(a.Name)
	if held, _ := persistent.Held(home, a.Name); held || here {
		return nil, fmt.Errorf("the %s is running: stop it first", a.Title)
	}
	cfg, _ := persistent.LoadConfig(home, a.Name)
	who := a.DisplayName(cfg)
	if err := p.app.askConsent("Reset the "+who+"?", "This wipes the "+who+"'s conversation and its own files (its two volumes). Its share folder and the receipts of its segments are kept. Its next start begins a new conversation."); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := persistent.Reset(ctx, home, a.Name, p.dockerClient()); err != nil {
		return nil, err
	}
	emitEvent(p.app, eventAgentState, a.Name)
	return p.Get(name)
}

// Transcript is the end of the agent's conversation, for its page to draw
// before its live events: the last transcriptLines lines.
func (p *PersistentAPI) Transcript(name string) ([]SessionEventView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	entries, err := persistent.Transcript(home, a.Name, transcriptLines)
	if err != nil {
		return nil, err
	}
	out := make([]SessionEventView, 0, len(entries))
	for _, ev := range persistent.HistoryEvents(entries) {
		out = append(out, eventView(ev))
	}
	return out, nil
}

// stopAll stops every agent this app runs, each bounded, as the app quits:
// each segment ends without a final result and its volumes stay.
func (p *PersistentAPI) stopAll() {
	p.mu.Lock()
	sups := make([]agentSupervisor, 0, len(p.sups))
	for _, sv := range p.sups {
		sups = append(sups, sv)
	}
	p.mu.Unlock()
	var wg sync.WaitGroup
	for _, sv := range sups {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), stopWait)
			defer cancel()
			_, _ = sv.Stop(ctx)
		}()
	}
	wg.Wait()
}

// AddServer adds an MCP server to the agent's roster, in the Builder's
// server row's shape (a package of npm or PyPI at an exact version, or a
// remote server's address); it is held to the manifest's own rules for a
// declared server, and turning it on asks first, as any widening does.
func (p *PersistentAPI) AddServer(name string, row build.MCPServerRow) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	srv := serverOfRow(row.Declared())
	if err := p.configure(a, persistent.Change{AddServer: &srv}); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// serverOfRow is a Builder row as the manifest declares the server.
func serverOfRow(r build.MCPServerRow) manifest.MCPServer {
	srv := manifest.MCPServer{Tools: r.Tools, Secrets: r.Secrets}
	if r.IsRemote() {
		srv.URL, srv.Auth, srv.Scopes = r.URL, manifest.MCPAuth(r.Auth), r.Scopes
		return srv
	}
	srv.Version, srv.Egress = strings.TrimSpace(r.Version), r.Egress
	if r.Source == "pypi" {
		srv.PyPI = strings.TrimSpace(r.Package)
	} else {
		srv.NPM = strings.TrimSpace(r.Package)
	}
	return srv
}

// AddFiles opens the native file picker and copies each file picked into
// the agent's share folder, beside what is there: a name already taken gets
// a number. It answers the names written; a cancel writes nothing.
func (p *PersistentAPI) AddFiles(name string) ([]string, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err != nil {
		return nil, err
	}
	share, err := persistent.ShareFolder(a, cfg)
	if err != nil {
		return nil, err
	}
	if err := persistent.CheckShareFolder(home, share); err != nil {
		return nil, err
	}
	files, err := p.app.ChooseFiles("Add files to the " + a.DisplayName(cfg) + "'s share folder")
	if err != nil || len(files) == 0 {
		return nil, err
	}
	if err := os.MkdirAll(share, 0o755); err != nil {
		return nil, err
	}
	var out []string
	for _, f := range files {
		written, err := copyInto(share, f)
		if err != nil {
			return out, fmt.Errorf("copy %s into the share folder: %w", filepath.Base(f), err)
		}
		out = append(out, written)
	}
	return out, nil
}

// copyInto copies a regular file into dir under its own name, or with a
// number where that is taken, and never over a file; it answers the name.
func copyInto(dir, src string) (string, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	in, err := os.Open(src) //nolint:gosec // a file the person picked
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	base := filepath.Base(src)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for i := 0; i < 1000; i++ {
		name := base
		if i > 0 {
			name = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			_ = os.Remove(out.Name())
			return "", err
		}
		return name, out.Close()
	}
	return "", errors.New("too many files of that name in the share folder")
}
