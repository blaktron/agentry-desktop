package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/persistent/assistant"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Google Workspace's set-up in the app: the person's Google Cloud OAuth
// client from a file the native picker returns, and the sign-in through
// the browser. Both are the CLI's own (persistent.Google): the client file is
// read here and handed to gog on stdin, never to the page; the sign-in's
// address is opened in the person's browser and its redirect caught on this
// computer's 127.0.0.1; the sign-in reads as done only once gog names the
// account. Neither runs while the agent does, since its server holds the
// volume they write.

// maxGoogleClientFile bounds the client file read: Google's are under 1 KiB,
// and persistent.CheckGoogleClient refuses one past its own bound.
const maxGoogleClientFile = 64 << 10

// googleSignInPage is where Google's sign-in starts; nothing else is opened.
const googleSignInPage = "https://accounts.google.com/"

// eventGoogleSignIn tells the page the address of a sign-in that is
// waiting for the browser.
const eventGoogleSignIn = "persistent:google-sign-in"

// PersistentGoogleSignInView is a waiting sign-in's address, for the page
// to show as text when the browser did not open.
type PersistentGoogleSignInView struct {
	Agent string `json:"agent"`
	URL   string `json:"url"`
}

// PersistentGoogleView is an agent's Google Workspace set-up, as far as the
// app shows it: whether its client is kept and when, and the account it is
// signed in to, as gog named it. Never the client or a token.
type PersistentGoogleView struct {
	Client   bool   `json:"client"`
	ClientAt string `json:"clientAt,omitempty"`
	Account  string `json:"account,omitempty"`
	At       string `json:"at,omitempty"`
	// Stale says the client was changed after the sign-in, which must be
	// made again; Waiting that a sign-in is waiting for the browser.
	Stale   bool `json:"stale,omitempty"`
	Waiting bool `json:"waiting,omitempty"`
}

// imageServerKey is the agent's Google Workspace server's key, "" for an
// agent with none.
func imageServerKey(a persistent.Agent) string {
	for _, s := range a.Builtin.ImageServers(policy.Images{}) {
		if s.Key == assistant.GogKey {
			return s.Key
		}
	}
	return ""
}

// googleView is the agent's Google Workspace set-up, nil for an agent with
// no such server.
func googleView(a persistent.Agent, cfg persistent.Config) *PersistentGoogleView {
	key := imageServerKey(a)
	if key == "" {
		return nil
	}
	in := cfg.SignIns[key]
	v := &PersistentGoogleView{Client: !in.Client.IsZero(), Account: in.Account}
	if v.Client {
		v.ClientAt = in.Client.Format(time.RFC3339)
	}
	if !in.At.IsZero() {
		v.At = in.At.Format(time.RFC3339)
	}
	v.Stale = v.Account != "" && in.Client.After(in.At)
	return v
}

// google is the CLI's sign-in for the agent, with the engine ready and the
// runner image in place.
func (p *PersistentAPI) google(ctx context.Context, a persistent.Agent) (persistent.Google, error) {
	if p.googleSetUp != nil {
		return p.googleSetUp(ctx, a)
	}
	home, pol, d, tag, err := p.oneShotReady(ctx, true)
	if err != nil {
		return persistent.Google{}, err
	}
	return persistent.Google{Docker: d, Home: home, Agent: a, Policy: pol, RunnerImage: tag, Keys: p.secretStore(), Client: persistent.ClientDesktop}, nil
}

// googleAgent is the agent named, if it has a Google Workspace server and
// does not run here.
func (p *PersistentAPI) googleAgent(name string) (persistent.Agent, error) {
	a, err := p.agent(name)
	if err != nil {
		return persistent.Agent{}, err
	}
	if imageServerKey(a) == "" {
		return persistent.Agent{}, fmt.Errorf("the %s has no Google Workspace", a.Title)
	}
	if _, running := p.running(a.Name); running {
		return persistent.Agent{}, fmt.Errorf("stop the %s first: its Google Workspace server holds what the set-up writes", a.Title)
	}
	return a, nil
}

// pickGoogleClient opens the native picker for the client file; "" is a
// cancel.
func (p *PersistentAPI) pickGoogleClient() (string, error) {
	if p.pickFile != nil {
		return p.pickFile()
	}
	return runtime.OpenFileDialog(p.app.ctx, runtime.OpenDialogOptions{
		Title:   "Choose your Google Cloud client file (client_secret_….json)",
		Filters: []runtime.FileFilter{{DisplayName: "Google Cloud client (.json)", Pattern: "*.json"}},
	})
}

// SetGoogleClient opens the native picker for the person's Google Cloud
// OAuth client file (a Desktop app's, as Google's console downloads it) and
// hands it to the agent's Google Workspace server. The file is read here and
// never crosses to the page. A cancel changes nothing.
func (p *PersistentAPI) SetGoogleClient(name string) (*PersistentAgentView, error) {
	a, err := p.googleAgent(name)
	if err != nil {
		return nil, err
	}
	path, err := p.pickGoogleClient()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return p.Get(name)
	}
	b, ok := readRegular(path, maxGoogleClientFile)
	if !ok {
		return nil, errors.New("that is not a Google Cloud client file: pick the .json file Google's console downloaded, at most 64 KiB")
	}
	if err := persistent.CheckGoogleClient(b); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), oneShotWait)
	defer cancel()
	g, err := p.google(ctx, a)
	if err != nil {
		return nil, err
	}
	if err := g.SetClient(ctx, b); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// GoogleSignIn signs the agent's Google Workspace in to email: Google's
// sign-in opens in the person's browser, and the call returns once Google
// answers and gog names the account, or the sign-in is cancelled or times
// out. It asks for read-only access unless the server's "allow changes" is
// on.
func (p *PersistentAPI) GoogleSignIn(name, email string) (*PersistentAgentView, error) {
	a, err := p.googleAgent(name)
	if err != nil {
		return nil, err
	}
	// The address is one argument of the server's sign-in: one that reads
	// as an option is no address.
	if email = strings.TrimSpace(email); strings.HasPrefix(email, "-") {
		return nil, fmt.Errorf("%q is not an email address a Google account signs in with", email)
	}
	ctx, cancel := context.WithTimeout(context.Background(), oneShotWait+persistent.GoogleSignInTimeout)
	defer cancel()
	p.mu.Lock()
	if p.googleSignIns == nil {
		p.googleSignIns = map[string]context.CancelFunc{}
	}
	if _, busy := p.googleSignIns[a.Name]; busy {
		p.mu.Unlock()
		return nil, errors.New("a Google sign-in is already waiting for the browser")
	}
	p.googleSignIns[a.Name] = cancel
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.googleSignIns, a.Name)
		p.mu.Unlock()
	}()
	g, err := p.google(ctx, a)
	if err != nil {
		return nil, err
	}
	open := p.browse
	if open == nil {
		open = func(u string) error {
			runtime.BrowserOpenURL(p.app.ctx, u)
			return nil
		}
	}
	// Google's own sign-in page alone is opened, and the page is told its
	// address, as text, for when the browser does not open.
	browse := func(u string) error {
		if !strings.HasPrefix(u, googleSignInPage) {
			return fmt.Errorf("the sign-in's address is not Google's: %q", u)
		}
		emitEvent(p.app, eventGoogleSignIn, PersistentGoogleSignInView{Agent: a.Name, URL: u})
		return open(u)
	}
	if _, err := g.SignIn(ctx, email, browse); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, errors.New("the Google sign-in was cancelled")
		}
		return nil, err
	}
	return p.Get(name)
}

// CancelGoogleSignIn ends a sign-in still waiting for the browser.
func (p *PersistentAPI) CancelGoogleSignIn(name string) {
	p.mu.Lock()
	cancel := p.googleSignIns[name]
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
