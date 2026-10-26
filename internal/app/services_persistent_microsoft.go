package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/persistent/assistant"
	"github.com/blaktron/agentry-cli/pkg/runner/connection"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Microsoft 365's set-up in the app: the person's own Entra app, its id
// typed in a field and handed to the CLI (persistent.Microsoft.SetClient),
// or, where none is kept, Agentry's own Microsoft app when the registry
// offers one, and the sign-in by a device code, whose code and page the page shows while
// the CLI waits for Microsoft. Only Microsoft's own device-login page is
// opened. The sign-in reads as done only once the server lists the account.
// Neither runs while the agent does, since its server holds the volume they
// write.

// eventMicrosoftSignIn tells the page a waiting sign-in's code and page.
const eventMicrosoftSignIn = "persistent:microsoft-sign-in"

// PersistentMicrosoftSignInView is a waiting sign-in's code and the page to
// enter it at, for the page to show.
type PersistentMicrosoftSignInView struct {
	Agent string `json:"agent"`
	URL   string `json:"url"`
	Code  string `json:"code"`
}

// PersistentMicrosoftView is an agent's Microsoft 365 set-up, as far as the
// app shows it: the Entra app's id (an id, never a secret) and the tenant,
// the account the server lists once signed in, the signoff, and whether a
// sign-in is waiting. Stale says the app changed after the sign-in.
// SignedInWithAgentryApp says the sign-in was made with Agentry's own
// Microsoft app, which a sign-in uses where no Entra app of the person's is
// kept (MicrosoftAgentryApp says whether the registry offers it).
type PersistentMicrosoftView struct {
	ClientID               string `json:"clientID,omitempty"`
	Tenant                 string `json:"tenant,omitempty"`
	ClientAt               string `json:"clientAt,omitempty"`
	Account                string `json:"account,omitempty"`
	At                     string `json:"at,omitempty"`
	Signoff                string `json:"signoff,omitempty"`
	Stale                  bool   `json:"stale,omitempty"`
	Waiting                bool   `json:"waiting,omitempty"`
	SignedInWithAgentryApp bool   `json:"signedInWithAgentryApp,omitempty"`
}

// microsoftView is the agent's Microsoft 365 set-up, nil for an agent with
// no such server.
func microsoftView(a persistent.Agent, cfg persistent.Config) *PersistentMicrosoftView {
	if !hasImageServer(a, assistant.MS365Key) {
		return nil
	}
	in := cfg.SignIns[assistant.MS365Key]
	v := &PersistentMicrosoftView{ClientID: in.ClientID, Tenant: in.Tenant, Account: in.Account, Signoff: in.Signoff}
	if !in.Client.IsZero() {
		v.ClientAt = in.Client.Format(time.RFC3339)
	}
	if !in.At.IsZero() {
		v.At = in.At.Format(time.RFC3339)
	}
	v.Stale = v.Account != "" && in.Client.After(in.At)
	v.SignedInWithAgentryApp = v.Account != "" && in.SignedInWith != "" && in.SignedInWith != in.ClientID
	return v
}

// agentryAppWait bounds asking the registry whether it offers Agentry's own
// Microsoft app when the set-up opens.
const agentryAppWait = 15 * time.Second

// MicrosoftAgentryApp says whether the registry the app is on offers
// Agentry's own Microsoft app for the agent's sign-in: asked only when the
// Microsoft set-up opens, never to draw a view, so starting the app asks
// the registry nothing and writes nothing, and never where the person keeps
// an Entra app of their own, which comes first.
func (p *PersistentAPI) MicrosoftAgentryApp(name string) (bool, error) {
	a, err := p.agent(name)
	if err != nil {
		return false, err
	}
	if !hasImageServer(a, assistant.MS365Key) {
		return false, nil
	}
	home, err := p.home()
	if err != nil {
		return false, err
	}
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err != nil {
		return false, err
	}
	if cfg.SignIns[assistant.MS365Key].ClientID != "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), agentryAppWait)
	defer cancel()
	return p.agentryMicrosoftApp(home)(ctx) != "", nil
}

// agentryMicrosoftApp is the client id of Agentry's own Microsoft app as the
// registry the app is on serves it, or "" for none: what a sign-in uses
// where the person keeps no Entra app, with the build's own image of the
// server alone (the CLI holds it to that).
func (p *PersistentAPI) agentryMicrosoftApp(home string) func(ctx context.Context) string {
	if p.agentryApp != nil {
		return p.agentryApp
	}
	reg := ""
	if s, err := settings.Load(); err == nil {
		reg = settings.Normalize(s.DefaultRegistry)
	}
	return agentryMicrosoftAppAt(home, reg)
}

// agentryMicrosoftAppAt reads Agentry's Microsoft app's id from the registry
// at reg (GET /api/v1/connectors, without a credential, through the
// registry's own client), kept under the runner home as the CLI keeps it.
func agentryMicrosoftAppAt(home, reg string) func(ctx context.Context) string {
	if reg == "" {
		return func(context.Context) string { return "" }
	}
	public := connection.OpenPublicClients(home, reg, registry.HTTPClient)
	return func(ctx context.Context) string { return public.ForConnector(ctx, persistent.MicrosoftConnector) }
}

// microsoft is the CLI's sign-in for the agent, with the engine ready and
// the runner image in place.
func (p *PersistentAPI) microsoft(ctx context.Context, a persistent.Agent) (persistent.Microsoft, error) {
	var m persistent.Microsoft
	if p.microsoftSetUp != nil {
		var err error
		if m, err = p.microsoftSetUp(ctx, a); err != nil {
			return persistent.Microsoft{}, err
		}
	} else {
		home, pol, d, tag, err := p.oneShotReady(ctx, true)
		if err != nil {
			return persistent.Microsoft{}, err
		}
		m = persistent.Microsoft{Docker: d, Home: home, Agent: a, Policy: pol, RunnerImage: tag, Keys: p.secretStore(), Client: persistent.ClientDesktop}
	}
	m.AgentryClient = p.agentryMicrosoftApp(m.Home)
	return m, nil
}

// microsoftAgent is the agent named, if it has a Microsoft 365 server and
// does not run here.
func (p *PersistentAPI) microsoftAgent(name string) (persistent.Agent, error) {
	a, err := p.agent(name)
	if err != nil {
		return persistent.Agent{}, err
	}
	if !hasImageServer(a, assistant.MS365Key) {
		return persistent.Agent{}, fmt.Errorf("the %s has no Microsoft 365", a.Title)
	}
	if _, running := p.running(a.Name); running {
		return persistent.Agent{}, fmt.Errorf("stop the %s first: its Microsoft 365 server holds what the set-up writes", a.Title)
	}
	return a, nil
}

// SetMicrosoftClient records the person's own Entra app: its application
// (client) id, and the tenant to sign in on ("" asks Microsoft at the
// sign-in). The id is checked here and again by the CLI, which refuses the
// server's own default client.
func (p *PersistentAPI) SetMicrosoftClient(name, clientID, tenant string) (*PersistentAgentView, error) {
	a, err := p.microsoftAgent(name)
	if err != nil {
		return nil, err
	}
	if err := persistent.CheckMicrosoftClient(clientID); err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	m := persistent.Microsoft{Home: home, Agent: a, Client: persistent.ClientDesktop}
	if err := m.SetClient(clientID, tenant); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// SetMicrosoftSignoff records the line mail the server sends or drafts ends
// with; "" clears it.
func (p *PersistentAPI) SetMicrosoftSignoff(name, text string) (*PersistentAgentView, error) {
	a, err := p.microsoftAgent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	m := persistent.Microsoft{Home: home, Agent: a, Client: persistent.ClientDesktop}
	if err := m.SetSignoff(text); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// microsoftCodeRE finds the code and the page in the CLI's sentence that
// relays them.
var microsoftCodeRE = regexp.MustCompile(`open (https://\S+) and enter the code ([A-Z0-9]{6,16})\b`)

// codeRelay passes the CLI's narration to the page: the device code and its
// page, once, as an event; nothing else of it.
type codeRelay struct {
	agent string
	emit  func(PersistentMicrosoftSignInView)

	mu   sync.Mutex
	buf  strings.Builder
	sent bool
}

func (r *codeRelay) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sent || r.buf.Len() > 16<<10 {
		return len(b), nil
	}
	r.buf.Write(b)
	if m := microsoftCodeRE.FindStringSubmatch(r.buf.String()); m != nil {
		r.sent = true
		r.emit(PersistentMicrosoftSignInView{Agent: r.agent, URL: m[1], Code: m[2]})
	}
	return len(b), nil
}

// MicrosoftSignIn signs the agent's Microsoft 365 in to email by a device
// code: the page is told the code and Microsoft's page to enter it at, which
// is opened in the person's browser, and the call returns once the server
// lists the account, or the sign-in is cancelled or times out. It asks only
// for reading unless the server's "allow changes" is on.
func (p *PersistentAPI) MicrosoftSignIn(name, email string) (*PersistentAgentView, error) {
	a, err := p.microsoftAgent(name)
	if err != nil {
		return nil, err
	}
	if email = strings.TrimSpace(email); strings.HasPrefix(email, "-") {
		return nil, fmt.Errorf("%q is not an email address a Microsoft account signs in with", email)
	}
	ctx, cancel := context.WithTimeout(context.Background(), oneShotWait+persistent.MicrosoftSignInTimeout)
	defer cancel()
	p.mu.Lock()
	if p.microsoftSignIns == nil {
		p.microsoftSignIns = map[string]context.CancelFunc{}
	}
	if _, busy := p.microsoftSignIns[a.Name]; busy {
		p.mu.Unlock()
		return nil, errors.New("a Microsoft sign-in is already waiting for its code")
	}
	p.microsoftSignIns[a.Name] = cancel
	p.mu.Unlock()
	// The sign-in stops waiting before the view is read, so the view
	// returned does not say it waits.
	done := func() {
		p.mu.Lock()
		delete(p.microsoftSignIns, a.Name)
		p.mu.Unlock()
	}
	defer done()
	m, err := p.microsoft(ctx, a)
	if err != nil {
		return nil, err
	}
	m.Say = &codeRelay{agent: a.Name, emit: func(v PersistentMicrosoftSignInView) { emitEvent(p.app, eventMicrosoftSignIn, v) }}
	open := p.browse
	if open == nil {
		open = func(u string) error {
			runtime.BrowserOpenURL(p.app.ctx, u)
			return nil
		}
	}
	// The CLI opens only Microsoft's own device-login page; this holds the
	// app to the same, whatever it is handed.
	browse := func(u string) error {
		if !microsoftDeviceLogin(u) {
			return fmt.Errorf("the sign-in's page is not Microsoft's: %q", u)
		}
		return open(u)
	}
	if _, err := m.SignIn(ctx, email, browse); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, errors.New("the Microsoft sign-in was cancelled")
		}
		return nil, err
	}
	done()
	return p.Get(name)
}

// microsoftDeviceLogin says u is Microsoft's device-login page.
func microsoftDeviceLogin(u string) bool {
	switch u {
	case "https://www.microsoft.com/link", "https://microsoft.com/link", "https://microsoft.com/devicelogin", "https://www.microsoft.com/devicelogin":
		return true
	}
	return strings.HasPrefix(u, "https://login.microsoftonline.com/") || strings.HasPrefix(u, "https://login.microsoft.com/")
}

// CancelMicrosoftSignIn ends a sign-in still waiting for its code.
func (p *PersistentAPI) CancelMicrosoftSignIn(name string) {
	p.mu.Lock()
	cancel := p.microsoftSignIns[name]
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// hasImageServer says the agent offers the image server key.
func hasImageServer(a persistent.Agent, key string) bool {
	for _, s := range a.Builtin.ImageServers(policy.Images{}) {
		if s.Key == key {
			return true
		}
	}
	return false
}
