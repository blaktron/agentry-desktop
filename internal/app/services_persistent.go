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
	"log"
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
	"github.com/blaktron/agentry-desktop/internal/notify"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

const (
	// eventAgentEvent carries a PersistentEventView; eventAgentState names an
	// agent that started or stopped.
	eventAgentEvent = "runner:agent-event"
	eventAgentState = "runner:agent-state"
)

// The states an agent's view reads, besides persistent.StateRunning.
const (
	agentDisabled = "disabled"
	agentStopped  = "stopped"
	agentStarting = "starting"
	// agentElsewhere: running under another client (the CLI's supervisor).
	agentElsewhere = "elsewhere"
)

// shownAgents are the persistent agents the app offers, in their order:
// Main, whose place is its Home card, then the Assistant and the Coding
// agent.
var shownAgents = []string{mainAgent, assistantAgent, coderAgent}

// coderAgent is the Coding agent's name, its built-in's.
const coderAgent = "coder"

// assistantAgent is the Assistant's name, its built-in's.
const assistantAgent = "assistant"

// transcriptLines is how much of an agent's transcript its page reloads.
const transcriptLines = 500

// stopWait bounds a stop at quit, so a stuck segment cannot hold the app.
const stopWait = 60 * time.Second

// PersistentAPI runs the app's persistent agents.
type PersistentAPI struct {
	app *App
	run *RunAPI

	mu   sync.Mutex
	sups map[string]agentSupervisor
	// starting are the starts under way, by agent; failures the last
	// start's failure of each agent whose last start failed.
	starting map[string]*startingAgent
	failures map[string]*PersistentFailureView
	// starts are the Starts under way, which a quit cancels and waits for.
	starts sync.WaitGroup
	// followers are the goroutines that carry each agent's events.
	followers sync.WaitGroup
	// lookups are the server icon lookups under way in the background.
	lookups sync.WaitGroup
	// stopping names the agents a Stop or a quit is ending, so their end
	// is not told as one they came to by themselves.
	stopping map[string]bool

	// start starts a supervisor; nil is persistent.Start. A test puts its
	// own here.
	start func(ctx context.Context, opts persistent.Options) (agentSupervisor, error)
	// docker is the engine client a reset removes the volumes through; nil
	// is the docker CLI.
	docker dockerx.Client
	// links answer Agentry Helper's tools for each agent whose app link
	// lists them, by its name; an agent without one has its calls of them
	// answered as unanswered.
	links map[string]*mainLink
	// onState is told whenever an agent starts, stops or is turned on or
	// off: the tray follows it.
	onState func()
	// iconLookups are the servers whose icon this process has looked up.
	iconLookups map[string]bool
	// pickSkillSource, checkSkill and secrets stand in for the native
	// picker, the skill's malware check and the keychain in a test; nil is
	// the real one.
	pickSkillSource func(from string) (string, error)
	checkSkill      persistent.SkillCheck
	secrets         persistent.Secrets
	// pickFolder and exportMemory stand in for the native folder picker
	// and the memory's one-shot copy in a test.
	pickFolder   func() (string, error)
	exportMemory func(ctx context.Context, home string, a persistent.Agent, dir string) (persistent.Exported, error)
	// viewing is the agent whose page the window shows while it has the
	// focus ("" for none), as the page reports it: its tasks raise no
	// notification then.
	viewing string
	// showNotice raises a desktop notification, and minimised reports
	// whether the window is minimised; nil is the platform's own. A test
	// puts its own here.
	showNotice func(notify.Notice) error
	minimised  func() bool
	// googleSignIns cancel the Google sign-ins waiting for the browser, by
	// agent. googleSetUp, pickFile and browse stand in for the CLI's
	// sign-in with its engine, the native file picker and the browser in a
	// test.
	googleSignIns map[string]context.CancelFunc
	// microsoftSignIns cancel the Microsoft sign-ins waiting for the
	// person to enter the code, by agent; microsoftSetUp stands in for the
	// CLI's in a test.
	microsoftSignIns map[string]context.CancelFunc
	microsoftSetUp   func(ctx context.Context, a persistent.Agent) (persistent.Microsoft, error)
	// agentryApp stands in for asking the registry for Agentry's own
	// Microsoft app's id in a test.
	agentryApp func(ctx context.Context) string
	// fastmailCheck stands in for asking Fastmail about a token in a test.
	fastmailCheck func(ctx context.Context, token string) error
	// calendarCheck stands in for fetching and reading a calendar feed's
	// address in a test.
	calendarCheck func(ctx context.Context, address string) (string, int, error)
	googleSetUp   func(ctx context.Context, a persistent.Agent) (persistent.Google, error)
	pickFile      func() (string, error)
	browse        func(url string) error
	// elsewhere is what the owner-lock watch last saw of each agent:
	// whether another client runs it. viewers are the app's chats with
	// those agents. held, attach, requestStop and queryStatus stand in for
	// the CLI's lock probe, its Attach, its RequestStop and its QueryStatus
	// in a test.
	elsewhere   map[string]bool
	viewers     map[string]*viewer
	held        func(home, name string) (bool, *persistent.Holder)
	attach      func(ctx context.Context, home, name string, history []session.Event) (session.Session, error)
	requestStop func(ctx context.Context, home, name string) (persistent.Stopped, error)
	queryStatus func(ctx context.Context, home, name string) (persistent.Status, error)
}

// stateChanged tells the page and the tray that an agent's state changed.
func (p *PersistentAPI) stateChanged(name string) {
	emitEvent(p.app, eventAgentState, name)
	if p.onState != nil {
		p.onState()
	}
}

// backgroundAgents are the agents that keep the app running with its window
// closed: Main lives in the window.
var backgroundAgents = []string{"assistant", coderAgent}

// keepsRunning reports whether an agent that runs in the background is
// enabled, so closing the window hides it instead of quitting.
func (p *PersistentAPI) keepsRunning() bool {
	home, err := p.home()
	if err != nil {
		return false
	}
	for _, name := range backgroundAgents {
		if cfg, err := persistent.LoadConfig(home, name); err == nil && cfg.Enabled {
			return true
		}
	}
	return false
}

// runningLabels are the agents running in this app, by name, as the tray
// names them.
func (p *PersistentAPI) runningLabels() map[string]string {
	out := map[string]string{}
	home, err := p.home()
	if err != nil {
		return out
	}
	p.mu.Lock()
	names := make([]string, 0, len(p.sups))
	for name := range p.sups {
		names = append(names, name)
	}
	p.mu.Unlock()
	for _, name := range names {
		a, err := persistent.Lookup(name)
		if err != nil {
			continue
		}
		cfg, _ := persistent.LoadConfig(home, name)
		out[name] = a.DisplayName(cfg)
	}
	return out
}

// startOnLaunch starts each background agent that is enabled and set to run
// when Agentry starts. One whose roster holds what the person has not agreed
// to on this computer is not started, since that asks in a dialog nobody
// launched the app to see; its page says to start it.
func (p *PersistentAPI) startOnLaunch() {
	home, err := p.home()
	if err != nil {
		return
	}
	for _, name := range backgroundAgents {
		a, err := persistent.Lookup(name)
		if err != nil {
			continue
		}
		cfg, err := persistent.LoadConfig(home, name)
		if err != nil || !cfg.Enabled || !cfg.RunOnStart {
			continue
		}
		if len(persistent.Unconsented(persistent.BuildRosterUnder(a, cfg, nil, malwareCheckOn()), cfg)) > 0 {
			log.Printf("agentry-desktop: %s is set to run when Agentry starts, but its roster needs your agreement: start it from its page", a.Title)
			continue
		}
		if _, err := p.Start(name); err != nil {
			log.Printf("agentry-desktop: starting %s when Agentry started: %v", a.Title, err)
		}
	}
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
	Compact(ctx context.Context) (persistent.Compacted, error)
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
	State  string `json:"state"`
	Holder string `json:"holder,omitempty"`
	// Stage is the step a start is on, in a person's words ("Getting images
	// ready"), while State is starting; empty before the first step.
	Stage string `json:"stage,omitempty"`
	// Failure is how the last start failed, until the next start.
	Failure     *PersistentFailureView `json:"failure,omitempty"`
	Personality string                 `json:"personality,omitempty"`
	// Share is the share folder; DefaultShare says it is the agent's own
	// default.
	Share        string `json:"share"`
	DefaultShare bool   `json:"defaultShare"`
	// MalwareCheck says the runner policy's malware check is on: a file
	// added to the share of an agent that checks it is checked with ClamAV
	// before the agent reads it.
	MalwareCheck bool     `json:"malwareCheck"`
	Knowledge    []string `json:"knowledge"`
	// RunOnStart says the agent starts when the app does.
	RunOnStart bool `json:"runOnStart"`
	// DevHost is the Coding agent's dev server, user@host[:port], and
	// DevHostKey the SHA256 fingerprint of its pinned host key; empty when
	// it has none.
	DevHost    string `json:"devHost,omitempty"`
	DevHostKey string `json:"devHostKey,omitempty"`
	// Ports are the Coding agent's published ports.
	Ports   []PersistentPort        `json:"ports"`
	Servers []PersistentServerView  `json:"servers"`
	Roster  []PersistentRosterEntry `json:"roster"`
	// Suggested are the servers the agent suggests and does not have.
	Suggested []PersistentSuggestionView `json:"suggested"`
	// KeepsSecrets says the agent stores secrets of the person's;
	// TakesSkills that it takes skills of the person's.
	KeepsSecrets bool `json:"keepsSecrets"`
	TakesSkills  bool `json:"takesSkills"`
	// KeepsTime says it has tasks and a heartbeat; KeepsMemory a memory to
	// export; Codes git host accounts, listed in Accounts (never a token).
	KeepsTime   bool                    `json:"keepsTime"`
	KeepsMemory bool                    `json:"keepsMemory"`
	Codes       bool                    `json:"codes"`
	Accounts    []PersistentAccountView `json:"accounts"`
	// Resets are the resets the agent has, the full one last, each with
	// what it wipes and keeps: the Assistant's three, one for the others.
	Resets []PersistentResetView `json:"resets"`
	// MailProviders are the services whose mail and calendar the agent
	// reaches, each enabled or disabled; none for an agent that reaches no
	// mail.
	MailProviders []PersistentMailProviderView `json:"mailProviders"`
	// Google is the agent's Google Workspace set-up, for an agent that has
	// that server.
	Google *PersistentGoogleView `json:"google,omitempty"`
	// Microsoft is the agent's Microsoft 365 set-up, for an agent that has
	// that server.
	Microsoft *PersistentMicrosoftView `json:"microsoft,omitempty"`
	// Fastmail is the agent's Fastmail set-up, for an agent that has that
	// server.
	Fastmail *PersistentFastmailView `json:"fastmail,omitempty"`
	// ReadsCalendars says the agent takes calendar feeds, listed in
	// CalendarFeeds by name (never an address).
	ReadsCalendars bool                         `json:"readsCalendars"`
	CalendarFeeds  []PersistentCalendarFeedView `json:"calendarFeeds"`
	Status         *PersistentStatusView        `json:"status,omitempty"`
	Lifetime       PersistentUsageView          `json:"lifetime"`
	Volumes        PersistentVolumesView        `json:"volumes"`
	// LastCompaction and LastConsolidation are the newest records of the
	// memory's upkeep, for an agent that keeps a memory; none before the
	// first.
	LastCompaction    *PersistentUpkeepView `json:"lastCompaction,omitempty"`
	LastConsolidation *PersistentUpkeepView `json:"lastConsolidation,omitempty"`
}

// PersistentUpkeepView is one run of an agent's memory upkeep, a compaction
// or the daily tidy of its memory, as its Configuration names it.
type PersistentUpkeepView struct {
	Time time.Time `json:"time"`
	// Trigger is idle, in-use, manual or consolidate.
	Trigger string `json:"trigger"`
	// Done says it finished: a compaction started a fresh conversation, a
	// tidy ran. Reason is why not when it did not.
	Done        bool   `json:"done"`
	Reason      string `json:"reason,omitempty"`
	FactsBefore int    `json:"factsBefore"`
	FactsAfter  int    `json:"factsAfter"`
}

// upkeepView is a record of the agent's receipts as the view shows it.
func upkeepView(r *persistent.UpkeepRecord) *PersistentUpkeepView {
	if r == nil {
		return nil
	}
	done := r.Swapped
	if r.Trigger == persistent.UpkeepConsolidate {
		done = r.Reason == ""
	}
	return &PersistentUpkeepView{Time: r.Time, Trigger: r.Trigger, Done: done, Reason: r.Reason, FactsBefore: r.FactsBefore, FactsAfter: r.FactsAfter}
}

// PersistentPort is one of the Coding agent's published ports: a name the
// person gave it and its number, published at 127.0.0.1 and that number.
type PersistentPort struct {
	Name string `json:"name"`
	Port int    `json:"port"`
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
	// State is on, off, pending or needs-sign-in, and for a server the
	// person added installing or failed, with Reason saying why it failed.
	State  string `json:"state"`
	On     bool   `json:"on"`
	Line   string `json:"line"`
	Fixed  bool   `json:"fixed,omitempty"`
	Reason string `json:"reason,omitempty"`
	// ShareSwitch is, for a server that may see the agent's share folder,
	// the id of its switch for that, and Share says it is on. ShareRefusal
	// is, for a server that may not, the runner's sentence saying why: a
	// remote server, one that reaches the network or one that signs in by
	// OAuth.
	ShareSwitch  string `json:"shareSwitch,omitempty"`
	Share        bool   `json:"share,omitempty"`
	ShareRefusal string `json:"shareRefusal,omitempty"`
	// IconKey names the bundled glyph a built-in's server is drawn with;
	// Icon is the kept icon of a server the person added, as a data: URL
	// (a PNG, JPEG or WebP of at most 256 KiB); neither for a monogram.
	IconKey string `json:"iconKey,omitempty"`
	Icon    string `json:"icon,omitempty"`
	// SetUpSecret and SetUpAccount are what an offered server's Set up
	// asks for: the stored secret it needs, or an account on a host.
	SetUpSecret  string `json:"setUpSecret,omitempty"`
	SetUpAccount string `json:"setUpAccount,omitempty"`
	// SetUpGoogle says the entry's Set up is Google Workspace's: its
	// client file and sign-in.
	SetUpGoogle bool `json:"setUpGoogle,omitempty"`
	// SetUpMicrosoft says the entry's Set up is Microsoft 365's: its Entra
	// app and sign-in.
	SetUpMicrosoft bool `json:"setUpMicrosoft,omitempty"`
	// SetUpFastmail says the entry's Set up is Fastmail's: its token.
	SetUpFastmail bool `json:"setUpFastmail,omitempty"`
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
	DefaultShare bool `json:"defaultShare,omitempty"`
	// RunOnStart starts the agent when the app starts.
	RunOnStart *bool `json:"runOnStart,omitempty"`
	// ClearDevHost takes the Coding agent's dev server away.
	ClearDevHost bool `json:"clearDevHost,omitempty"`
	// AddPort publishes one of the Coding agent's ports; RemovePort takes
	// one away by its name.
	AddPort         *PersistentPort   `json:"addPort,omitempty"`
	RemovePort      string            `json:"removePort,omitempty"`
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

// List is every agent the app offers, as it stands. A Helper left enabled
// beside the Assistant (by the CLI, or before the Assistant superseded it)
// is disabled first, unless it is running.
func (p *PersistentAPI) List() ([]PersistentAgentView, error) {
	if home, err := p.home(); err == nil {
		if err := p.supersedeHelper(home); err != nil {
			log.Printf("agentry-desktop: turning Agentry Helper off beside the Assistant: %v", err)
		}
	}
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
		DefaultShare: cfg.Share == "", Knowledge: append([]string{}, cfg.Knowledge...), Servers: []PersistentServerView{}, RunOnStart: cfg.RunOnStart,
		Volumes: PersistentVolumesView{Session: persistent.SessionVolume(a.Name), Data: persistent.DataVolume(a.Name)},
	}
	v.Share, _ = persistent.ShareFolder(a, cfg)
	if cfg.DevHost != nil {
		v.DevHost, v.DevHostKey = cfg.DevHost.Host, cfg.DevHost.HostKey
	}
	v.Ports = []PersistentPort{}
	for _, port := range cfg.Ports {
		v.Ports = append(v.Ports, PersistentPort{Name: port.Name, Port: port.Port})
	}
	if u, err := persistent.LoadUsage(home, a.Name); err == nil {
		v.Lifetime = PersistentUsageView{Segments: u.Segments, InputTokens: u.InputTokens, OutputTokens: u.OutputTokens, TotalTokens: u.TotalTokens, UserTurns: u.UserTurns, ToolCalls: u.ToolCalls}
	}
	v.MalwareCheck = malwareCheckOn()
	roster := persistent.BuildRosterUnder(a, cfg, nil, v.MalwareCheck)
	p.mu.Lock()
	sv, ours := p.sups[a.Name]
	st, starting := p.starting[a.Name]
	if starting {
		v.Stage = st.stage
	}
	if f := p.failures[a.Name]; f != nil {
		cp := *f
		v.Failure = &cp
	}
	p.mu.Unlock()
	switch {
	case ours:
		v.State = persistent.StateRunning
		roster = sv.Roster()
		st := sv.Status()
		v.Status = statusView(st)
	case starting:
		v.State = agentStarting
	default:
		if held, holder := p.heldElsewhere(home, a.Name); held {
			v.State = agentElsewhere
			v.Holder = holderWords(holder)
			v.Status = p.statusElsewhere(home, a.Name)
		} else if cfg.Enabled {
			v.State = agentStopped
		} else {
			v.State = agentDisabled
		}
	}
	v.Roster = rosterView(roster, cfg.Servers, factsOf(a, cfg))
	serverIcons(cfg, v.Roster)
	v.Suggested = suggestionsView(a, cfg)
	v.KeepsSecrets, v.TakesSkills = a.Builtin.KeepsSecrets(), !a.DesktopOnly
	v.KeepsTime, v.KeepsMemory, v.Codes = a.Builtin.KeepsTime(), a.Builtin.KeepsMemory(), a.Builtin.Codes()
	if v.KeepsMemory {
		compaction, consolidation := persistent.LastUpkeep(home, a.Name)
		v.LastCompaction, v.LastConsolidation = upkeepView(compaction), upkeepView(consolidation)
	}
	v.Accounts = accountViews(cfg.Accounts)
	v.Resets = resetViews(a)
	v.MailProviders = mailProviderViews(a, cfg)
	v.Microsoft = microsoftView(a, cfg)
	v.Fastmail = fastmailView(a, cfg)
	v.ReadsCalendars, v.CalendarFeeds = a.Builtin.ReadsCalendars(), calendarFeedViews(a, cfg)
	if v.Microsoft != nil {
		p.mu.Lock()
		_, v.Microsoft.Waiting = p.microsoftSignIns[a.Name]
		p.mu.Unlock()
	}
	v.Google = googleView(a, cfg)
	if v.Google != nil {
		p.mu.Lock()
		_, v.Google.Waiting = p.googleSignIns[a.Name]
		p.mu.Unlock()
	}
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

// rosterView is the roster as the page draws it; servers are the agent's
// servers as its config declares them, which say why a server may not see
// the share.
func rosterView(r persistent.Roster, servers []manifest.MCPServer, f cardFacts) []PersistentRosterEntry {
	out := make([]PersistentRosterEntry, 0, len(r))
	for _, e := range r {
		v := PersistentRosterEntry{
			ID: e.ID, Kind: string(e.Kind), Name: e.Name, GrantedBy: e.GrantedBy,
			State: string(e.State), On: e.On, Line: e.Line, Fixed: e.Fixed, Reason: e.Reason,
			ShareSwitch: e.ShareSwitch, Share: e.Share,
			IconKey: f.glyph[e.ID], SetUpSecret: f.secret[e.ID], SetUpAccount: f.account[e.ID],
			SetUpGoogle: f.google[e.ID], SetUpMicrosoft: f.microsoft[e.ID], SetUpFastmail: f.fastmail[e.ID],
		}
		if e.Kind == persistent.KindServer && e.ShareSwitch == "" {
			for _, srv := range servers {
				if persistent.ServerEntryID(srv) != e.ID {
					continue
				}
				if err := persistent.ServerMaySeeShare(srv); err != nil {
					v.ShareRefusal = err.Error()
				}
				break
			}
		}
		out = append(out, v)
	}
	return out
}

// malwareCheckOn is the malware-check setting of the runner policy the app
// starts its agents under, for a roster built while the agent does not run
// here: the share's sentence says whether ClamAV checks its files. Off
// when the policy is missing or unreadable, as a new one has it.
// It only reads: a Get never writes the default policy, which a start does.
func malwareCheckOn() bool {
	polFile, err := policy.Resolve("")
	if err != nil {
		return false
	}
	pol, _, err := policy.Load(polFile.Path)
	return err == nil && pol.MalwareCheckOn()
}

// SetEnabled enables or disables an agent. A running agent is not
// disabled: it is stopped first. Enabling the Coding agent asks first, in a
// native dialog that says what it may do; a No changes nothing and answers
// the agent as it was. The Assistant supersedes Agentry Helper, whose tools
// it lists: enabling the Assistant stops and disables the Helper, and the
// Helper is not enabled while the Assistant is.
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
	if on && a.Name == mainAgent {
		if err := helperRefused(home); err != nil {
			return nil, err
		}
	}
	if on && a.Name == assistantAgent {
		if err := p.retireHelper(home); err != nil {
			return nil, err
		}
	}
	if on && a.Name == coderAgent {
		if cfg, err := persistent.LoadConfig(home, a.Name); err == nil && !cfg.Enabled {
			share, _ := persistent.ShareFolder(a, cfg)
			err := p.app.askConsent("Enable the Coding agent?", coderWarning(share))
			if errors.Is(err, errNotConfirmed) {
				return p.Get(name)
			}
			if err != nil {
				return nil, err
			}
		}
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
	p.stateChanged(a.Name)
	return p.Get(name)
}

// helperRefused refuses Agentry Helper while the Assistant is enabled,
// naming the Assistant as the person knows it.
func helperRefused(home string) error {
	cfg, err := persistent.LoadConfig(home, assistantAgent)
	if err != nil || !cfg.Enabled {
		return err
	}
	a, err := persistent.Lookup(assistantAgent)
	if err != nil {
		return err
	}
	who := called(a, cfg)
	return fmt.Errorf("can't turn Agentry Helper on while %s is on: %s does everything the Helper does. Turn %s off first to use the Helper instead", who, who, who)
}

// retireHelper stops Agentry Helper where this app runs it and disables it,
// as the Assistant is enabled. A Helper another client runs is not stopped
// from here: that is refused.
func (p *PersistentAPI) retireHelper(home string) error {
	if _, here := p.running(mainAgent); here {
		if _, err := p.Stop(mainAgent); err != nil {
			return fmt.Errorf("stopping Agentry Helper: %w", err)
		}
	} else if held, _ := p.heldElsewhere(home, mainAgent); held {
		return errors.New("another client is running Agentry Helper: stop it there first")
	}
	return p.disableHelper(home)
}

// supersedeHelper disables Agentry Helper when it is enabled beside the
// Assistant and is not running, starting or held by another client.
func (p *PersistentAPI) supersedeHelper(home string) error {
	acfg, err := persistent.LoadConfig(home, assistantAgent)
	if err != nil || !acfg.Enabled {
		return err
	}
	p.mu.Lock()
	_, ours := p.sups[mainAgent]
	_, starting := p.starting[mainAgent]
	p.mu.Unlock()
	if ours || starting {
		return nil
	}
	if held, _ := p.heldElsewhere(home, mainAgent); held {
		return nil
	}
	return p.disableHelper(home)
}

// disableHelper disables Agentry Helper under its owner lock, if it is
// enabled.
func (p *PersistentAPI) disableHelper(home string) error {
	cfg, err := persistent.LoadConfig(home, mainAgent)
	if err != nil || !cfg.Enabled {
		return err
	}
	a, err := persistent.Lookup(mainAgent)
	if err != nil {
		return err
	}
	owner, err := persistent.Acquire(home, a.Name, persistent.ClientDesktop)
	if err != nil {
		return err
	}
	defer owner.Release()
	if cfg, err = persistent.LoadConfig(home, a.Name); err != nil {
		return err
	}
	cfg.Enabled = false
	if err := persistent.SaveConfig(home, a.Name, cfg); err != nil {
		return err
	}
	p.stateChanged(a.Name)
	return nil
}

// coderWarning is the Coding agent's enable dialog: what it may do, and
// that it is riskier than the other agents.
func coderWarning(share string) string {
	folder := "your codingshare folder"
	if share != "" {
		folder = "your " + filepath.Base(share) + " folder"
	}
	return "The Coding agent builds and runs software, reaches the web and any dev server you give it, and reads " + folder +
		" as it is. That is riskier than other agents.\n\nEnable it?"
}

// Start starts an agent: a roster with entries the person has not agreed to
// on this computer asks first, in a native dialog, and records the
// agreement. Its events then stream as runner:agent-event until it stops.
// While it starts, its view carries the step it is on, and its progress goes
// to its supervisor.log; a Stop cancels it. A start that fails keeps its
// error on the agent's view until the next.
func (p *PersistentAPI) Start(name string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	// A quit or a Stop cancels a start under way, which tears down what it
	// made, and a quit waits for it, so no segment's containers outlive the
	// app. The context is the supervisor's for the agent's whole life once
	// it starts, so it is cancelled only when the start does not succeed; a
	// running agent is ended by Stop.
	ctx, cancel := context.WithCancel(context.Background())
	started := false
	defer func() {
		if !started {
			cancel()
		}
	}()
	p.mu.Lock()
	if _, ok := p.sups[a.Name]; ok || p.starting[a.Name] != nil {
		p.mu.Unlock()
		return p.Get(name)
	}
	if p.starting == nil {
		p.starting = map[string]*startingAgent{}
	}
	st := &startingAgent{cancel: cancel, done: make(chan struct{})}
	p.starting[a.Name] = st
	delete(p.failures, a.Name)
	p.starts.Add(1)
	p.mu.Unlock()
	defer p.starts.Done()
	p.stateChanged(a.Name)
	defer func() {
		close(st.done)
		p.stateChanged(a.Name)
	}()
	logw := newSupervisorLog(home, a.Name)
	since := time.Now()
	sv, err := p.launch(ctx, home, a, logw)
	if err == nil {
		started = true
		p.follow(a.Name, sv, logw)
	}
	// The start ends here for a Stop: one that came before this reads
	// stopped, one after finds the agent running (or not) and acts on that.
	p.mu.Lock()
	stopped, stage := st.stopped, st.stage
	delete(p.starting, a.Name)
	p.mu.Unlock()
	if err == nil {
		if stopped {
			// A Stop came as the start finished: the agent stops now.
			logw.note("The start was cancelled.")
			p.markStopping(a.Name)
			_, _ = sv.Stop(context.WithoutCancel(ctx))
			p.mu.Lock()
			if p.sups[a.Name] == sv {
				delete(p.sups, a.Name)
			}
			p.mu.Unlock()
		}
		return p.Get(name)
	}
	defer logw.Close()
	if stopped {
		logw.note("The start was cancelled.")
		return p.Get(name)
	}
	var failed *startError
	if !errors.As(err, &failed) {
		// A refusal before the supervisor's start: not enabled, the
		// person's No.
		return nil, err
	}
	err = failed.err
	var held *persistent.HeldError
	if errors.As(err, &held) || ctx.Err() != nil {
		// Another client's agent, or a quit: neither is a failure of the
		// start.
		return nil, err
	}
	logw.note("The start failed: " + err.Error())
	f := startFailure(home, a.Name, stage, since, err)
	p.mu.Lock()
	if p.failures == nil {
		p.failures = map[string]*PersistentFailureView{}
	}
	p.failures[a.Name] = f
	p.mu.Unlock()
	return nil, err
}

// startError is a failure of the supervisor's start itself, as against a
// refusal before it (not enabled, a No).
type startError struct{ err error }

func (e *startError) Error() string { return e.err.Error() }
func (e *startError) Unwrap() error { return e.err }

// launch readies and starts the agent's supervisor, its progress in logw.
func (p *PersistentAPI) launch(ctx context.Context, home string, a persistent.Agent, logw io.Writer) (agentSupervisor, error) {
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err != nil {
		return nil, err
	}
	if a.Name == mainAgent {
		if err := helperRefused(home); err != nil {
			return nil, err
		}
	}
	if !cfg.Enabled && a.Name == mainAgent {
		return nil, errors.New("turn Agentry Helper on in Settings → Persistent Agents first: it is not enabled")
	}
	if !cfg.Enabled {
		return nil, fmt.Errorf("the %s is not enabled: enable it in Settings → Persistent Agents", a.Title)
	}
	if pending := persistent.Unconsented(persistent.BuildRosterUnder(a, cfg, nil, malwareCheckOn()), cfg); len(pending) > 0 {
		if err := p.app.askConsent("Start "+called(a, cfg)+"?", consentText(a.DisplayName(cfg), pending)); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			// Stopped while the dialog was up: nothing is agreed to.
			return nil, err
		}
		if err := persistent.AgreeToRoster(home, a, persistent.ClientDesktop); err != nil {
			return nil, err
		}
	}
	opts, err := p.options(home, a, logw)
	if err != nil {
		return nil, &startError{err}
	}
	sv, err := p.startSupervisor(ctx, opts)
	var consent *persistent.ConsentError
	if errors.As(err, &consent) {
		return nil, fmt.Errorf("the %s's roster changed while it started: start it again", a.Title)
	}
	if err != nil {
		return nil, &startError{err}
	}
	return sv, nil
}

// called is the agent in a sentence: the name the person gave it, or "the
// Assistant" while it has none.
func called(a persistent.Agent, c persistent.Config) string {
	if c.DisplayName != "" {
		return c.DisplayName
	}
	return "the " + a.Title
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
func (p *PersistentAPI) options(home string, a persistent.Agent, logw io.Writer) (persistent.Options, error) {
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
		Docker: p.dockerClient(), Stderr: logw, OnStage: p.stageHook(a.Name, logw), Packs: p.packs, AppLink: p.appLink(a),
	}, nil
}

// appLink is the handler of the agent's app link calls the supervisor does
// not answer itself: Agentry Helper's tools, answered by the app, for Main
// and the Assistant.
func (p *PersistentAPI) appLink(a persistent.Agent) persistent.AppLinkHandler {
	l := p.links[a.Name]
	if l == nil {
		return nil
	}
	return l.answer
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
// name until it stops, by Stop or by itself, and then it is let go and its
// log closed.
func (p *PersistentAPI) follow(name string, sv agentSupervisor, logw *supervisorLog) {
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
			// Whether a task's notice raises a notification is decided
			// before the page has the event: whoever sees the event can
			// rely on that decision being made.
			p.noticeTask(name, ev, logw)
			emitEvent(p.app, eventAgentEvent, PersistentEventView{Agent: name, Event: eventView(ev)})
		}
	}()
	go func() {
		defer p.followers.Done()
		<-sv.Done()
		unsubscribe()
		defer logw.Close()
		p.mu.Lock()
		if p.sups[name] == sv {
			delete(p.sups, name)
		}
		asked := p.stopping[name]
		delete(p.stopping, name)
		p.mu.Unlock()
		// An agent that stopped without being asked says so in its chat,
		// with why, from its segment's end.
		if !asked {
			ctx, cancel := context.WithTimeout(context.Background(), stopWait)
			env, err := sv.Stop(ctx)
			cancel()
			why := "its segment ended"
			switch {
			case err != nil:
				why = err.Error()
			case env != nil && env.Status != "":
				why = "its segment ended (" + string(env.Status) + ")"
			}
			emitEvent(p.app, eventAgentEvent, PersistentEventView{Agent: name, Event: SessionEventView{
				Kind: string(session.KindNotice), Time: time.Now().UTC().Format(time.RFC3339Nano),
				Text: "The agent stopped by itself: " + why + ". Start it again to go on; the conversation continues.",
			}})
		}
		p.stateChanged(name)
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
// conversation. An agent still starting has its start cancelled, which
// tears down what the start made, and Stop waits for it. An agent another
// client runs is asked to stop through its supervisor's socket.
func (p *PersistentAPI) Stop(name string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	st := p.starting[a.Name]
	if st != nil {
		st.stopped = true
	}
	p.mu.Unlock()
	if st != nil {
		st.cancel()
		select {
		case <-st.done:
		case <-time.After(stopWait):
			return nil, fmt.Errorf("the %s's start did not end within %s of being cancelled", a.Title, stopWait)
		}
		return p.Get(name)
	}
	sv, ok := p.running(a.Name)
	if !ok {
		// An agent another client runs is stopped through its socket.
		if home, err := p.home(); err == nil {
			if held, _ := p.heldElsewhere(home, a.Name); held {
				if err := p.stopElsewhere(home, a); err != nil {
					return nil, err
				}
			}
		}
		return p.Get(name)
	}
	p.markStopping(a.Name)
	_, err = sv.Stop(context.Background())
	p.mu.Lock()
	if p.sups[a.Name] == sv {
		delete(p.sups, a.Name)
	}
	p.mu.Unlock()
	p.stateChanged(a.Name)
	if err != nil {
		return nil, err
	}
	return p.Get(name)
}

// markStopping says a Stop is ending the agent.
func (p *PersistentAPI) markStopping(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopping == nil {
		p.stopping = map[string]bool{}
	}
	p.stopping[name] = true
}

// Send sends the person's message as the agent's next turn and returns
// once it is answered; its events stream meanwhile. A change waiting for
// the next turn is applied first, which the stream says. An agent another
// client runs takes it through its supervisor's socket.
func (p *PersistentAPI) Send(name, text string) error {
	a, err := p.agent(name)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("the message is empty")
	}
	sv, ok := p.running(a.Name)
	if !ok {
		home, err := p.home()
		if err != nil {
			return err
		}
		if held, _ := p.heldElsewhere(home, a.Name); held {
			return p.sendElsewhere(home, a, text)
		}
		return fmt.Errorf("the %s is not running: start it first", a.Title)
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
	c := persistent.Change{Name: ch.Name, Personality: ch.Personality, AddKnowledge: ch.AddKnowledge, RemoveKnowledge: ch.RemoveKnowledge, RemoveServer: ch.RemoveServer,
		RunOnStart: ch.RunOnStart, ClearDevHost: ch.ClearDevHost, RemovePort: ch.RemovePort}
	if ch.AddPort != nil {
		c.AddPort = &persistent.Port{Name: ch.AddPort.Name, Port: ch.AddPort.Port}
	}
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

// OpenPort opens one of the Coding agent's published ports in the person's
// browser: only a port of its own, switched on, while it runs.
func (p *PersistentAPI) OpenPort(name, port string) error {
	v, err := p.Get(name)
	if err != nil {
		return err
	}
	if v.State != persistent.StateRunning && v.State != agentElsewhere {
		return fmt.Errorf("the %s is not running: start it first", v.Title)
	}
	for _, e := range v.Roster {
		if e.ID != persistent.PortEntryID(port) {
			continue
		}
		if !e.On || e.State == string(persistent.StatePending) {
			return fmt.Errorf("the port %s is not published now", port)
		}
		for _, q := range v.Ports {
			if q.Name == port {
				return p.app.OpenExternal(fmt.Sprintf("http://127.0.0.1:%d/", q.Port))
			}
		}
	}
	return fmt.Errorf("the %s has no port named %s", v.Title, port)
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
	if err := p.app.askConsent("Give "+called(a, cfg)+" this?", consentText(a.DisplayName(cfg), consent.Entries)); err != nil {
		return err
	}
	c.Consent = true
	_, _, err = persistent.Configure(ctx, home, a, persistent.ClientDesktop, c)
	return err
}

// SetDevHost readies the Coding agent's dev server and saves it: answer is
// user@host[:port]; the sign-in (a key file the native picker returned and
// its passphrase, or a password) is signed in with and saved in the system
// keychain, and a host key no known_hosts file holds is put to the person
// in a native dialog. Saving it turns on a roster entry, which asks first
// as any widening does. The sign-in travels inward only.
func (p *PersistentAPI) SetDevHost(name, answer string, sign run.HostSignIn) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	if a.Name != coderAgent {
		return nil, fmt.Errorf("only the Coding agent has a dev server")
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	polFile, err := policy.Resolve("")
	if err != nil {
		return nil, err
	}
	pol, _, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return nil, fmt.Errorf("operator policy: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sign.Sudo, sign.Save = "", true
	d, err := run.ReadyDevHost(ctx, answer, sign, run.DevHost{
		Pol: pol, Home: home, Picked: p.app.keyFiles.require, Trust: p.app.trustHostKey,
	})
	if err != nil {
		return nil, err
	}
	if err := p.configure(a, persistent.Change{DevHost: d}); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// ChooseShare opens the native folder picker for the agent's share folder
// and saves the folder picked, held to the share folder's guards; a cancel
// changes nothing.
func (p *PersistentAPI) ChooseShare(name string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	// Unmarked: a folder the agent writes into is not one a path binding
	// may then take files from without the picker.
	dir, err := p.app.pickDirectoryUnmarked("The " + a.Title + "'s share folder")
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

// PersistentResetView is one reset an agent has: its kind (history, agent
// or all), its button's title, and what it wipes and keeps, which the page
// and the native confirmation both show.
type PersistentResetView struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// The kinds of reset, as the page passes them back.
const (
	resetHistory = "history"
	resetAgent   = "agent"
	resetAll     = "all"
)

// resetViews are the resets the agent has: Reset history and Reset agent
// before Reset all for one that resets in parts, Reset alone otherwise.
func resetViews(a persistent.Agent) []PersistentResetView {
	all := PersistentResetView{Kind: resetAll, Title: "Reset",
		Body: "This puts it back to its defaults. It wipes its conversation, its own files, its name and personality, the servers and knowledge packs added to it, its switches, its dev server and ports, whether it starts with Agentry, and what you agreed to, so its next start asks you again. It goes back to its default share folder. The files in the share folder you chose, the receipts of its segments and a dev server's sign-in in your keychain are kept."}
	if !a.Builtin.ResetsInParts() {
		return []PersistentResetView{all}
	}
	all.Title = "Reset all"
	all.Body = "This puts it back to its defaults. It wipes its conversation, its own files, its memory and tasks, its name and personality, the servers, skills and knowledge packs added to it, its switches, whether it starts with Agentry, and what you agreed to, so its next start asks you again. It removes the secrets you stored, its calendar feeds and its servers' sign-ins, from your keychain too, and its usage, and it goes back to its default share folder. The files in the share folder you chose and the receipts of its segments are kept."
	return []PersistentResetView{
		{Kind: resetHistory, Title: "Reset history",
			Body: "This wipes its history: its conversation, its own files, its memory, its transcript and tasks, and what it read from its share folder. It keeps everything you chose and agreed to, the secrets you stored, its calendar feeds and its servers' sign-ins, so its next start begins a new conversation and does not ask you again. Its usage, the files in its share folder and the receipts of its segments are kept."},
		{Kind: resetAgent, Title: "Reset agent",
			Body: "This wipes its history and puts everything you chose back to its default: its name and personality, the servers, skills and knowledge packs added to it, its switches, whether it starts with Agentry, and what you agreed to, so its next start asks you again. It keeps the secrets you stored, its calendar feeds and its servers' sign-ins, switched off, so turning one on asks you again and needs no new sign-in or address. Its share folder, the files in it, its usage and the receipts of its segments are kept."},
		all,
	}
}

// Reset puts an agent back to its defaults after a native confirmation:
// its conversation, its files and everything the person chose for it. One
// running here is stopped first; one another client runs is refused. Its
// share folder's files and its receipts stay.
func (p *PersistentAPI) Reset(name string) (*PersistentAgentView, error) {
	return p.reset(name, resetAll)
}

// ResetPart is one of the smaller resets of an agent that resets in parts,
// by its kind: history or agent. It asks and stops as Reset does.
func (p *PersistentAPI) ResetPart(name, kind string) (*PersistentAgentView, error) {
	if kind != resetHistory && kind != resetAgent {
		return nil, fmt.Errorf("%q is not a reset (history or agent)", kind)
	}
	return p.reset(name, kind)
}

// reset runs one of the agent's resets by its kind, after Go's native
// confirmation with that reset's own words; the kind must be one the agent
// has.
func (p *PersistentAPI) reset(name, kind string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	var choice *PersistentResetView
	for _, c := range resetViews(a) {
		if c.Kind == kind {
			choice = &c
		}
	}
	if choice == nil {
		return nil, fmt.Errorf("the %s has one reset", a.Title)
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	starting := p.starting[a.Name] != nil
	p.mu.Unlock()
	if starting {
		return nil, fmt.Errorf("the %s is starting: wait for it, then reset it", a.Title)
	}
	_, here := p.running(a.Name)
	if held, _ := persistent.Held(home, a.Name); held && !here {
		return nil, fmt.Errorf("the %s is running in another client: stop it there first", a.Title)
	}
	cfg, _ := persistent.LoadConfig(home, a.Name)
	who := called(a, cfg)
	body := choice.Body
	if here {
		body += " It is running: it is stopped first."
	}
	question := "Reset " + who + "?"
	switch kind {
	case resetHistory:
		question = "Reset " + who + "'s history?"
	case resetAgent:
		question = "Reset " + who + ", keeping its secrets and sign-ins?"
	}
	if err := p.app.askConsent(question, body); err != nil {
		return nil, err
	}
	if here {
		if _, err := p.Stop(a.Name); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	resets := map[string]func(context.Context, string, string, dockerx.Client) error{
		resetAll: persistent.Reset, resetHistory: persistent.ResetHistory, resetAgent: persistent.ResetAgent,
	}
	if err := resets[kind](ctx, home, a.Name, p.dockerClient()); err != nil {
		return nil, err
	}
	p.stateChanged(a.Name)
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
	for _, st := range p.starting {
		st.cancel()
	}
	p.mu.Unlock()
	p.starts.Wait()
	p.mu.Lock()
	sups := make([]agentSupervisor, 0, len(p.sups))
	for name, sv := range p.sups {
		sups = append(sups, sv)
		if p.stopping == nil {
			p.stopping = map[string]bool{}
		}
		p.stopping[name] = true
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
	// The chats with agents another client runs go; those agents run on.
	p.leaveViewers()
	wg.Wait()
}

// AddServer adds an MCP server to the agent's roster, in the Builder's
// server row's shape (a package of npm or PyPI at an exact version and the
// arguments its command starts with, or a remote server's address); its
// arguments are checked as the Build form checks them, it is held to the
// manifest's own rules for a declared server, and turning it on asks first,
// as any widening does.
func (p *PersistentAPI) AddServer(name string, row build.MCPServerRow) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	row = row.Declared()
	if !row.IsRemote() {
		if err := build.CheckArgs(row.Args); err != nil {
			return nil, err
		}
	}
	srv := serverOfRow(row)
	if err := p.configure(a, persistent.Change{AddServer: &srv}); err != nil {
		return nil, err
	}
	p.lookUpIcon(a.Name, srv)
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
	if len(r.Args) > 0 {
		srv.Args = slices.Clone(r.Args)
	}
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
	files, err := p.app.ChooseFiles("Add files to " + called(a, cfg) + "'s share folder")
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

// maxNameTries bounds the numbered names copyInto tries for a file.
const maxNameTries = 1000

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
	for i := 0; i < maxNameTries; i++ {
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
