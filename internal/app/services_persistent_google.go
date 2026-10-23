package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/blaktron/agentry-cli/pkg/persistent/assistant"
	"github.com/blaktron/agentry-cli/pkg/runner/gateway"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/runnerimage"
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

// PersistentGoogleView is an agent's Google Workspace set-up, as far as the
// app shows it: whether its client is kept and when, and the account it is
// signed in to, as gog named it. Never the client or a token.
type PersistentGoogleView struct {
	Client   bool   `json:"client"`
	ClientAt string `json:"clientAt,omitempty"`
	Account  string `json:"account,omitempty"`
	At       string `json:"at,omitempty"`
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
	return v
}

// google is the CLI's sign-in for the agent, with the engine ready and the
// runner image in place.
func (p *PersistentAPI) google(ctx context.Context, a persistent.Agent) (persistent.Google, error) {
	if p.googleSetUp != nil {
		return p.googleSetUp(ctx, a)
	}
	home, err := p.home()
	if err != nil {
		return persistent.Google{}, err
	}
	polFile, err := policy.Resolve("")
	if err != nil {
		return persistent.Google{}, err
	}
	pol, _, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return persistent.Google{}, fmt.Errorf("operator policy: %w", err)
	}
	d := p.dockerClient()
	if err := d.Available(ctx); err != nil {
		return persistent.Google{}, err
	}
	tag := pol.RunnerImageTag(runnerimage.ContentVersion())
	if err := gateway.EnsureRunnerImage(ctx, d, pol, tag, filepath.Join(home, "image"), io.Discard); err != nil {
		return persistent.Google{}, err
	}
	return persistent.Google{Docker: d, Home: home, Agent: a, Policy: pol, RunnerImage: tag, Keys: p.secretStore()}, nil
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
	if err != nil || path == "" {
		if err != nil {
			return nil, err
		}
		return p.Get(name)
	}
	b, err := readBounded(path, maxGoogleClientFile)
	if err != nil {
		return nil, err
	}
	if err := persistent.CheckGoogleClient(b); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
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

// readBounded reads a regular file of at most max bytes.
func readBounded(path string, max int64) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // a file the person picked natively
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("that is not a file")
	}
	if st.Size() > max {
		return nil, fmt.Errorf("the file is %d bytes; a Google Cloud client file is under %d", st.Size(), max)
	}
	return io.ReadAll(io.LimitReader(f, max+1))
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
	ctx, cancel := context.WithTimeout(context.Background(), persistent.GoogleSignInTimeout+5*time.Minute)
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
	browse := p.browse
	if browse == nil {
		browse = func(u string) error {
			runtime.BrowserOpenURL(p.app.ctx, u)
			return nil
		}
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
