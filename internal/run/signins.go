// Model sign-ins on this computer (plan model-sign-ins.md §4.1, D6, D7;
// agentry-desktop#734, M9): the person's own ChatGPT, Claude or Copilot plan,
// or a connected OpenRouter, Poe or Hugging Face account, kept beside any
// number of keys, one of them the active choice in model setup. Every sign-in
// and connect runs through the CLI's own packages (pkg/mcpauth,
// pkg/runner/modelsignin, pkg/runner/claudesignin), on the store the CLI
// shares, so there is one refresher per machine. Nothing here returns a
// token, a key or an authorization code: what the page sees is the account a
// vendor names, the models a plan offers, and, for Sign in with GitHub, the
// short code GitHub asks the person to enter on its own page.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/mcpauth"
	"github.com/blaktron/agentry-cli/pkg/runner/claudesignin"
	"github.com/blaktron/agentry-cli/pkg/runner/copilotmodels"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/modelsignin"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// Seams for tests: each flow's vendor endpoints (nil in production), and
// the model key's keychain entry a connected key is kept in.
var (
	chatgptEndpoints     func(*mcpauth.ChatGPT)
	githubEndpoints      func(*mcpauth.GitHub)
	openRouterEndpoints  func(*mcpauth.OpenRouter)
	poeEndpoints         func(*mcpauth.Poe)
	huggingFaceEndpoints func(*mcpauth.HuggingFace)
	connectStoreKey      = modelkey.Store
	connectDeleteKey     = modelkey.Delete
	claudeDocker         = func() dockerx.Client { return newDockerClient() }
)

// ModelSignIn is one kept model sign-in or connected account, as model
// setup shows it. Never a token or a key.
type ModelSignIn struct {
	// Vendor is chatgpt, claude, copilot, openrouter, poe or huggingface;
	// Provider the model provider it pays for (openai, anthropic, copilot,
	// openrouter, poe, huggingface).
	Vendor   string `json:"vendor"`
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	// Connected marks a connected account: an ordinary key the vendor
	// minted, kept as a pasted key is.
	Connected bool `json:"connected"`
	// Account is how the vendor names it ("Signed in as …"); "" when it
	// named none.
	Account string `json:"account,omitempty"`
	// Broken is why the vendor refused the last refresh: sign in again.
	Broken string `json:"broken,omitempty"`
	// KeyExpiresAt is when a connected key that cannot be renewed stops
	// working (Poe's), RFC 3339.
	KeyExpiresAt string `json:"keyExpiresAt,omitempty"`
	CreatedAt    string `json:"createdAt"`
	// Image says the Claude Code image is signed in (the default); a
	// Claude sign-in made only by the installed Claude Code has it false.
	Image bool `json:"image"`
}

// signInStore is the model sign-in store this computer shares with the
// CLI, keeping secrets in the keychain and connected keys in the model
// key's own entry.
func signInStore() (*modelsignin.Store, string, error) {
	home, err := Home()
	if err != nil {
		return nil, "", err
	}
	s := modelsignin.Open(home, connectionKeys())
	s.StoreKey, s.DeleteKey = connectStoreKey, connectDeleteKey
	return s, home, nil
}

// ModelSignIns lists what this computer keeps, the CLI's included.
func ModelSignIns() ([]ModelSignIn, error) {
	s, _, err := signInStore()
	if err != nil {
		return nil, err
	}
	recs, err := s.List()
	if err != nil {
		return nil, err
	}
	out := make([]ModelSignIn, 0, len(recs))
	for _, r := range recs {
		out = append(out, ModelSignIn{
			Vendor: r.Vendor, Provider: r.Provider, Kind: r.Kind, Connected: r.Connected,
			Account: r.Label, Broken: r.Broken, KeyExpiresAt: r.KeyExpiresAt, CreatedAt: r.CreatedAt,
			Image: r.Vendor != modelsignin.Claude || r.ConfigDir != "",
		})
	}
	return out, nil
}

// SignInUI is the person's side of a sign-in: Say tells them what is
// happening, Open opens an https address in their browser, and Code shows
// GitHub's device code with the page to enter it on.
type SignInUI struct {
	Say  func(line string)
	Open func(url string)
	Code func(page, code string)
}

func (ui SignInUI) say(line string) {
	if ui.Say != nil {
		ui.Say(line)
	}
}

// open opens an https address, the only kind a vendor's sign-in page is.
func (ui SignInUI) open(raw string) {
	if u, err := url.Parse(raw); err != nil || u.Scheme != "https" || u.Host == "" {
		ui.say("The sign-in page's address is not an https address, so it was not opened.")
		return
	}
	if ui.Open != nil {
		ui.Open(raw)
	}
}

// sayWriter carries a sign-in's own lines (Claude Code's) to Say.
type sayWriter struct {
	ui   SignInUI
	line []byte
}

func (w *sayWriter) Write(p []byte) (int, error) {
	w.line = append(w.line, p...)
	for {
		i := strings.IndexByte(string(w.line), '\n')
		if i < 0 {
			break
		}
		if l := strings.TrimRight(string(w.line[:i]), "\r"); strings.TrimSpace(l) != "" {
			w.ui.say(l)
		}
		w.line = w.line[i+1:]
	}
	return len(p), nil
}

// SignedIn is a sign-in or connect just kept: the account the vendor named
// and, for a ChatGPT plan, the models it offers.
type SignedIn struct {
	Vendor  string   `json:"vendor"`
	Account string   `json:"account,omitempty"`
	Models  []string `json:"models"`
}

// SignInChatGPT is Sign in with ChatGPT (plan §4.3): OpenAI's page in the
// browser, its answer on this computer's loopback. newAccount registers
// again, for another account than this computer's.
func SignInChatGPT(ctx context.Context, ui SignInUI, newAccount bool) (*SignedIn, error) {
	s, _, err := signInStore()
	if err != nil {
		return nil, err
	}
	flow := mcpauth.ChatGPT{HTTP: vendorHTTP(), Say: ui.say, Present: ui.open}
	if chatgptEndpoints != nil {
		chatgptEndpoints(&flow)
	}
	got, err := s.SignInChatGPT(ctx, flow, newAccount)
	var ne *mcpauth.ChatGPTNotEligibleError
	if errors.As(err, &ne) || errors.Is(err, mcpauth.ErrChatGPTNoPlanScope) {
		return nil, fmt.Errorf("nothing kept: %w", err)
	}
	if err != nil {
		return nil, err
	}
	models := got.Models
	if models == nil {
		models = []string{}
	}
	return &SignedIn{Vendor: modelsignin.ChatGPT, Account: got.Record.Label, Models: models}, nil
}

// ChatGPTModels lists the models the kept ChatGPT sign-in's plan offers,
// renewing its access token as a run would.
func ChatGPTModels(ctx context.Context) ([]string, error) {
	s, _, err := signInStore()
	if err != nil {
		return nil, err
	}
	a, err := s.Use(ctx, modelsignin.ChatGPT)
	if err != nil {
		return nil, err
	}
	flow := mcpauth.ChatGPT{HTTP: vendorHTTP()}
	if chatgptEndpoints != nil {
		chatgptEndpoints(&flow)
	}
	return flow.Models(ctx, a.Token)
}

// CopilotModel is one model a Copilot plan offers, for the model field:
// its id, GitHub's display name and premium-request multiplier where
// GitHub gave them.
type CopilotModel struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Multiplier float64 `json:"multiplier"`
}

// CopilotModels lists the models the kept Copilot sign-in's plan offers for
// plan (individual, business, enterprise), asked of Copilot CLI in a run's
// container through the CLI's own copilotmodels.List (agentry-cli#808): the
// runner and Copilot CLI images are made present first, as setup makes
// them, so a first list can take minutes; logw has their progress. A plan
// the account's policy disables a model on does not list it.
func CopilotModels(ctx context.Context, plan string, logw io.Writer) ([]CopilotModel, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	pol, err := loadPolicyIfPresent()
	if err != nil {
		return nil, err
	}
	if pol == nil {
		pol = policy.Default()
	}
	got, err := copilotmodels.List(ctx, newDockerClient(), copilotmodels.Request{
		Home: home, Keys: connectionKeys(), Policy: pol, CopilotPlan: plan,
		RunnerImage: runnerImageTag(pol), ImageCtxDir: filepath.Join(home, "image"), Log: logw,
	})
	if err != nil {
		return nil, err
	}
	out := make([]CopilotModel, 0, len(got))
	for _, m := range got {
		out = append(out, CopilotModel{ID: m.ID, Name: m.Name, Multiplier: m.Multiplier})
	}
	return out, nil
}

// claude is the CLI's Claude sign-in on this computer's daemon and store.
func claude(home string) claudesignin.SignIn {
	return claudesignin.SignIn{Home: home, Docker: claudeDocker, Keys: connectionKeys()}
}

// SignInClaude is Use your Claude subscription (runs Claude Code), plan
// §4.4: Claude Code itself signs the person in, in Anthropic's own pages in
// their browser, into the Claude Code image's volume, which Agentry never
// reads (agentry-cli#848). The page is given no input: Claude Code takes
// the browser's answer on its own callback, relayed into its container, and
// a sign-in that could only finish by pasting a code ends instead.
func SignInClaude(ctx context.Context, ui SignInUI, logw io.Writer) (*SignedIn, error) {
	_, home, err := signInStore()
	if err != nil {
		return nil, err
	}
	out := io.MultiWriter(logw, &sayWriter{ui: ui})
	rec, err := claude(home).Run(ctx, claudesignin.Person{Out: out, Open: ui.open}, false)
	if err != nil {
		return nil, err
	}
	return &SignedIn{Vendor: modelsignin.Claude, Account: rec.Label, Models: []string{}}, nil
}

// SignInCopilot is Sign in with GitHub (plan §4.5): GitHub's device flow
// through Agentry's GitHub App, or its OAuth App, the fallback (D26). The
// code GitHub issues is handed to Code, which shows it and puts it on the
// clipboard (operator, 2026-10-11), and GitHub's page is opened.
func SignInCopilot(ctx context.Context, ui SignInUI, oauthApp bool) (*SignedIn, error) {
	s, _, err := signInStore()
	if err != nil {
		return nil, err
	}
	app := mcpauth.GitHubApp
	if oauthApp {
		app = mcpauth.GitHubOAuthApp
	}
	flow := mcpauth.GitHub{HTTP: vendorHTTP(), Present: func(page, code string) {
		if ui.Code != nil {
			ui.Code(page, code)
		}
		ui.open(page)
	}}
	if githubEndpoints != nil {
		githubEndpoints(&flow)
	}
	got, err := s.SignInCopilot(ctx, flow, app)
	if err != nil {
		return nil, err
	}
	return &SignedIn{Vendor: modelsignin.Copilot, Account: got.Record.Label, Models: []string{}}, nil
}

// ConnectAccount is Connect your account for OpenRouter, Poe or Hugging
// Face (plan §4.6): the vendor mints an ordinary key for Agentry in the
// person's browser, kept in the model key's keychain entry for its provider.
func ConnectAccount(ctx context.Context, ui SignInUI, vendor string) (*SignedIn, error) {
	if !modelsignin.IsConnectVendor(vendor) {
		return nil, fmt.Errorf("%s is not an account this app connects (openrouter, poe, huggingface)", vendor)
	}
	if modelkey.Disabled() {
		return nil, fmt.Errorf("connecting your account keeps the key in the keychain, which is disabled here (%s is set)", modelkey.EnvNoKeyring)
	}
	s, _, err := signInStore()
	if err != nil {
		return nil, err
	}
	kc := mcpauth.KeyConnect{HTTP: vendorHTTP(), Say: ui.say, Present: ui.open}
	var got *mcpauth.ConnectedKey
	switch vendor {
	case modelsignin.OpenRouter:
		f := mcpauth.OpenRouter{KeyConnect: kc}
		if openRouterEndpoints != nil {
			openRouterEndpoints(&f)
		}
		got, err = f.Connect(ctx)
	case modelsignin.Poe:
		var id string
		if id, err = mcpauth.PoeClientID(); err == nil {
			f := mcpauth.Poe{KeyConnect: kc}
			if poeEndpoints != nil {
				poeEndpoints(&f)
			}
			got, err = f.Connect(ctx, id)
		}
	case modelsignin.HuggingFace:
		f := mcpauth.HuggingFace{KeyConnect: kc}
		if huggingFaceEndpoints != nil {
			huggingFaceEndpoints(&f)
		}
		got, err = f.Connect(ctx, mcpauth.HuggingFaceClientID())
	}
	if err != nil {
		return nil, err
	}
	rec, err := s.KeepConnected(vendor, got)
	if err != nil {
		return nil, err
	}
	return &SignedIn{Vendor: vendor, Account: rec.Label, Models: []string{}}, nil
}

// RemoveModelSignIn removes one kept sign-in or connected account: revoked
// at the vendor where it offers that, a Claude sign-in logged out by Claude
// Code itself and its directory deleted, a connected key deleted from the
// keychain. It returns what happened, in the vendor's terms.
func RemoveModelSignIn(ctx context.Context, vendor string, logw io.Writer) (string, error) {
	s, home, err := signInStore()
	if err != nil {
		return "", err
	}
	s.EndClaude = claude(home).End(logw)
	return s.Remove(ctx, vendor, "removed in the app's model setup")
}
