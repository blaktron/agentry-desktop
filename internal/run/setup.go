// Machine setup for QuickAgent runs: the same pkg/runner/setup logic
// `agentry setup` drives, behind a form Prompter instead of a TTY — no
// parallel implementation.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
	"github.com/blaktron/agentry-cli/pkg/runner/claudesignin"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/gateway"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/modelsignin"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/setup"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The keychain and the models listing, as seams: the tests have no
// keychain and no provider to ask, and a test must never touch a person's.
var (
	keychainStore   = modelkey.Store
	keychainDelete  = modelkey.Delete
	keychainPresent = modelkey.Present
	keychainLookup  = modelkey.Lookup
	listModels      = setup.ListModels
)

// Status is the setup state of this machine: can it run QuickAgents, and
// with which LLM provider.
type Status struct {
	DockerOK  bool
	DockerErr string
	// Engine is the container engine detection found, and the traps it saw.
	Engine       Engine
	PolicyPath   string
	PolicyExists bool
	// ProviderSetUp: somebody set the provider up, as opposed to the policy merely
	// existing. Saving the AI Review key, choosing a coding agent and the first
	// run each write the policy from defaults, and a page that read PolicyExists
	// as "a provider was chosen" would say so of a person who chose none. It is
	// true when the policy's model block is not the default's: a provider, a
	// model, a key source or an endpoint was chosen. It reads no keychain and no
	// variable, so nothing but a change to that block can change it; the default
	// provider kept on purpose with its key in the variable is the one choice it
	// cannot tell from no choice.
	ProviderSetUp bool
	Provider      string
	Model         string
	APIKeyEnv     string
	// APIKeySource is "keyring" when the OS keychain is the fallback for an
	// unset variable; "" when the environment is the only source.
	APIKeySource string
	// KeysStored lists every provider the setup form can show that has a key in
	// the keychain, so the form's badge is right for the provider chosen and not
	// only for the policy's; KeyStoreErr is why the keychain could not be asked
	// (none, disabled), in which case the list is not complete. Neither carries a
	// value.
	KeysStored  []string
	KeyStoreErr string
	// APIKeyKind is the policy's apiKeyKind: "" or "api", or "subscription"
	// for an Anthropic subscription token, which drives Claude Code alone;
	// KeyNotice is the sentence to show beside the field for that kind, ""
	// otherwise.
	APIKeyKind  string
	KeyNotice   string
	BaseURL     string
	ImagesReady bool
	// ModelReady: a run on this computer has a way to pay for its model
	// (modelReadiness has the rule); ModelGap says in one sentence what is
	// missing when it is not, so the run setup can offer the model setup
	// before the run. It never carries a value.
	ModelReady bool
	ModelGap   string
	// Budget defaults, for the run form's placeholders.
	TimeoutSeconds int
	MaxTurns       int
	// KeepProfilesByDefault seeds keep toggles.
	KeepProfilesByDefault bool
}

// GetStatus reports the machine's run-readiness without writing anything:
// the policy file is read if present and defaults are shown if not.
func GetStatus(ctx context.Context) (*Status, error) {
	policyPath, err := policy.DefaultPath()
	if err != nil {
		return nil, err
	}
	st := &Status{PolicyPath: policyPath}

	pol := policy.Default()
	if _, err := os.Stat(policyPath); err == nil {
		st.PolicyExists = true
		loaded, _, lerr := policy.Load(policyPath)
		if lerr != nil {
			// A broken policy file is a fact the settings page must show,
			// not hide behind defaults.
			return nil, fmt.Errorf("operator policy: %w", lerr)
		}
		pol = loaded
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat policy: %w", err)
	}
	st.Provider = pol.Model.Provider
	st.Model = pol.Model.Name
	st.APIKeyEnv = pol.Model.APIKeyEnv
	st.APIKeySource = pol.Model.APIKeySource
	st.APIKeyKind = pol.Model.APIKeyKind
	if st.APIKeyKind == policy.KeyKindSubscription {
		st.KeyNotice = policy.SubscriptionTokenSentence
	}
	// One ask per provider. Each reads the presence marker setup stores beside
	// the key, never the key itself (modelkey.Present): only a key stored without
	// a marker is read, once, to write its marker. A marker outlives a key
	// removed outside agentry, so this can say "stored" until setup stores a key
	// again or `agentry doctor` confirms the entry; the apply step reads the key
	// itself (keychainLookup) and is not misled.
	//
	// An absent entry is an answer; an error is the keychain itself — none,
	// disabled, locked — and would be the same error, and on a locked Secret
	// Service the same prompt, for every provider after it, so the first one ends
	// the asking and is said.
	for _, provider := range providersToAsk(pol.Model.Provider) {
		stored, kerr := keychainPresent(provider)
		if kerr != nil {
			st.KeyStoreErr = kerr.Error()
			break
		}
		if stored {
			st.KeysStored = append(st.KeysStored, provider)
		}
	}
	st.ProviderSetUp = st.PolicyExists && !reflect.DeepEqual(pol.Model, policy.Default().Model)
	st.ModelReady, st.ModelGap = modelReadiness(pol, st.KeysStored, st.KeyStoreErr)
	st.BaseURL = pol.Model.BaseURL
	st.TimeoutSeconds = pol.Budgets.TimeoutSeconds
	st.MaxTurns = pol.Budgets.MaxTurns
	st.KeepProfilesByDefault = pol.KeepProfilesByDefault()

	// The engine: the one adopted, when it still answers, else found anew.
	st.Engine = DetectEngine(ctx, false)
	if !st.Engine.Found {
		st.DockerErr = st.Engine.Err
		if st.DockerErr == "" {
			st.DockerErr = errNoEngine.Error()
		}
		return st, nil
	}
	st.DockerOK = true
	st.ImagesReady = imagesReady(ctx, newDockerClient(), pol)
	return st, nil
}

// signInRecord is the kept model sign-in for a vendor, modelsignin.ErrNone
// for none: a seam, so a test needs no store on disk.
var signInRecord = func(vendor string) (*modelsignin.Record, error) {
	s, _, err := signInStore()
	if err != nil {
		return nil, err
	}
	return s.Get(vendor)
}

// modelReadiness is whether a run on this computer has a way to pay for
// its model, and what is missing when it has none. keysStored and
// keyStoreErr are the keychain's answers GetStatus already gathered, so
// the keychain is asked once. The rule is the CLI preflight's: a model
// sign-in kind needs its sign-in kept and working; Agentry credits need a
// sign-in to the registry the model endpoint is under; a key comes from
// the variable the policy names, set in this process, else from the
// keychain where that is the policy's source. The variable, the source
// and the kind are read as a run reads them: the harness block of the
// adapter a run here would use, where it sets them, else the model's.
func modelReadiness(pol *policy.Policy, keysStored []string, keyStoreErr string) (bool, string) {
	m := pol.Model
	if m.Provider == "" || m.Name == "" {
		return false, "No model provider is chosen for runs on this computer."
	}
	hp := pol.Harness[adapter.DefaultFor(pol)]
	kind := policy.KeyKind(hp, m)
	if vendor := modelsignin.VendorOf(kind); vendor != "" {
		spends := policy.SignInSentence(kind)
		r, err := signInRecord(vendor)
		switch {
		case errors.Is(err, modelsignin.ErrNone):
			return false, fmt.Sprintf("Runs on this computer spend %s, and none is kept here: sign in to use it.", spends)
		case err != nil:
			return false, fmt.Sprintf("Runs on this computer spend %s, and the kept sign-in could not be read: %v.", spends, err)
		case r.Broken != "":
			return false, fmt.Sprintf("Runs on this computer spend %s, and the kept sign-in no longer works (%s): sign in again.", spends, r.Broken)
		}
		return true, ""
	}
	if m.Provider == policy.ProviderAgentry {
		reg, err := policy.AgentryRegistry(m)
		if err != nil {
			return false, fmt.Sprintf("Runs on this computer are paid with Agentry credits, and the policy's model endpoint is not a registry's: %v.", err)
		}
		if CreditsClient(pol) == nil {
			return false, fmt.Sprintf("Runs on this computer are paid with your Agentry credits at %s, and this computer is not signed in there.", reg)
		}
		return true, ""
	}
	keyEnv := policy.KeyEnv(hp, m)
	if keyEnv == "" {
		return false, fmt.Sprintf("The runner policy names no variable for the %s key.", m.Provider)
	}
	if os.Getenv(keyEnv) != "" {
		return true, ""
	}
	if policy.KeySource(hp, m) != modelkey.SourceKeyring {
		return false, fmt.Sprintf("$%s is not set in this app's environment, and the policy reads the %s key from the environment only.", keyEnv, m.Provider)
	}
	for _, p := range keysStored {
		if p == m.Provider {
			return true, ""
		}
	}
	if keyStoreErr != "" {
		return false, fmt.Sprintf("$%s is not set in this app's environment, and the OS keychain could not be asked for the %s key: %s.", keyEnv, m.Provider, keyStoreErr)
	}
	return false, fmt.Sprintf("$%s is not set in this app's environment, and no %s key is saved on this computer.", keyEnv, m.Provider)
}

// providersToAsk is the setup table plus custom (setup.Providers), and the
// policy's provider when the table does not list it (the policy admits a
// name the harness knows), so a key stored under it is listed too.
func providersToAsk(policyProvider string) []string {
	out := setup.Providers()
	for _, p := range out {
		if p == policyProvider {
			return out
		}
	}
	return append(out, policyProvider)
}

// imagesReady reports whether the three images a run needs are present:
// the pinned gateway image, the harness image model setup's choice runs on
// (docker-agent's for a key, a model sign-in's own), and the locally built
// runner image.
func imagesReady(ctx context.Context, docker dockerx.Client, pol *policy.Policy) bool {
	harness := pol.Images.HarnessDockerAgent
	if name := adapter.DefaultFor(pol); name != adapter.DefaultName {
		h, ok := pol.Images.HarnessImageFor(name)
		if !ok {
			return false
		}
		harness = h.Ref
	}
	for _, ref := range []string{pol.Images.Gateway, harness, runnerImageTag(pol)} {
		ok, err := docker.ImageExists(ctx, ref)
		if err != nil || !ok {
			return false
		}
	}
	return true
}

// SetupOpts is the settings form's model block (the provider table; empty
// fields fall back to the table defaults, custom needs BaseURL).
type SetupOpts struct {
	Provider  string
	Model     string
	APIKeyEnv string
	// APIKeySource is "" (keep the policy's for the same provider), "env"
	// or "keyring". StoreKey, when set, is the key to put into the OS
	// keychain for Provider first, on its way to the keychain and nowhere
	// else; storing makes the source keyring (the form stores through
	// StoreModelKey instead, ahead of this call). ForgetKey removes the
	// stored key and makes the source env.
	APIKeySource string
	StoreKey     string
	ForgetKey    bool
	BaseURL      string
	PullImages   bool
	// APIKeyKind names a model sign-in as the way to pay (chatgpt-plan,
	// claude-signin, copilot-signin): the sign-in is made beforehand
	// (SignInChatGPT, SignInClaude, SignInCopilot), and this records it as
	// the active choice. "" or api is a key.
	APIKeyKind string
	// Connect records a connected account (OpenRouter, Poe, Hugging Face)
	// as the way to pay, connected beforehand (ConnectAccount).
	Connect bool
	// CopilotPlan is the Copilot plan the account is on (individual,
	// business, enterprise), which names the Copilot API host runs reach.
	CopilotPlan string
}

// ApplySetup writes the LLM provider preference into the operator policy
// (preserving everything outside the model block) and, when PullImages,
// stands up the gateway plane immediately. Image pull/build progress
// streams to logw.
func ApplySetup(ctx context.Context, opts SetupOpts, logw io.Writer) error {
	home, err := Home()
	if err != nil {
		return err
	}
	policyPath, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	// The key goes to the keychain before the policy is touched — before
	// it is even created from defaults: a store that fails leaves the
	// policy exactly as it was and says why, rather than recording a
	// source the machine cannot honour. The person asked to store it, so
	// this does not quietly fall back to the variable.
	source := opts.APIKeySource
	provider := opts.Provider
	if provider == "" {
		if existing, perr := loadPolicyIfPresent(); perr == nil && existing != nil {
			provider = existing.Model.Provider
		} else {
			provider = policy.Default().Model.Provider
		}
	}
	// A base URL repoints the model call — the key goes wherever it says, and
	// the agent's only egress becomes that host — so it is judged before
	// anything is stored: the custom provider only (the page shows the field for
	// that one; Go enforces it too, so script in the webview cannot aim a known
	// provider at any host), and http or https (plain http off loopback is
	// accepted, with the form's warning).
	if err := checkBaseURL(provider, opts.BaseURL); err != nil {
		return err
	}
	if policy.IsModelSignIn(opts.APIKeyKind) || opts.Connect {
		return applySignInSetup(ctx, home, policyPath, provider, opts, logw)
	}
	if provider == policy.ProviderAgentry {
		return applyCreditsSetup(ctx, home, policyPath, opts, logw)
	}
	// The kind is read from the value while this process holds it — a key
	// stored now, or below the one the keychain holds; a forgotten key takes
	// its kind with it; the variable path knows nothing and keeps what the
	// policy has (setup.ConfigureModel's rule for the same provider).
	kind := ""
	switch {
	case opts.StoreKey != "":
		k, err := storeModelKey(provider, opts.StoreKey, logw)
		if err != nil {
			return err
		}
		source = modelkey.SourceKeyring
		kind = k
	case opts.ForgetKey:
		if err := keychainDelete(provider); err != nil {
			return err
		}
		source = modelkey.SourceEnv
		// "api" is the default the policy leaves unwritten
		// (setup.ConfigureModel), so this is how the kind is dropped.
		kind = policy.KeyKindAPI
		_, _ = fmt.Fprintf(logw, "model key removed from the OS keychain (%s)\n", modelkey.Account(provider))
	}
	// A keyring source with nothing behind it would record a fallback a run
	// cannot honour: refuse unless a key is stored (or was just now). When one
	// is, its kind is read from the stored value — the key came through
	// StoreModelKey in a call of its own, and on a provider switch the policy has
	// no kind to keep.
	if source == modelkey.SourceKeyring && opts.StoreKey == "" {
		present, perr := keychainPresent(provider)
		switch {
		case perr != nil:
			return perr
		case !present:
			return fmt.Errorf("the OS keychain holds no key for %s: type the key to store it, or choose the environment variable", provider)
		}
		stored, lerr := keychainLookup(provider)
		if lerr != nil {
			return lerr
		}
		kind = policy.CredentialKindOf(provider, stored)
	}
	pol, _, err := policy.EnsureDefault(policyPath)
	if err != nil {
		return fmt.Errorf("operator policy: %w", err)
	}
	if err := setup.ConfigureModel(&formPrompter{log: logw}, pol, setup.ModelOpts{
		Provider:     provider, // the one the key was stored under, never another
		Model:        opts.Model,
		APIKeyEnv:    opts.APIKeyEnv,
		APIKeySource: source,
		APIKeyKind:   kind,
		BaseURL:      opts.BaseURL,
		Yes:          true, // a form never mid-flight-prompts; gaps fill from the table
	}); err != nil {
		return err
	}
	if err := policy.Save(policyPath, pol); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "policy written: %s\n", policyPath)

	if !opts.PullImages {
		return nil
	}
	return prepareImages(ctx, home, pol, logw)
}

// prepareImages stands up the gateway plane and the harness image model
// setup's choice runs on, and no other harness's: the pinned
// docker-agent image for a key, or the Codex, Claude Code or Copilot CLI
// image for a model sign-in, pulled or built on this machine from its
// pinned artifact.
func prepareImages(ctx context.Context, home string, pol *policy.Policy, logw io.Writer) error {
	docker := newDockerClient()
	name := adapter.DefaultFor(pol)
	ownImage := name == adapter.DefaultName
	if err := gateway.EnsureInstalled(ctx, docker, pol, runnerImageTag(pol), filepath.Join(home, "image"), logw,
		ownImage); err != nil {
		return fmt.Errorf("gateway/docker environment: %w", err)
	}
	if !ownImage {
		h, ok := pol.Images.HarnessImageFor(name)
		if !ok {
			return fmt.Errorf("the policy names no %s harness image", name)
		}
		if err := gateway.EnsureHarnessImage(ctx, docker, name, h, logw); err != nil {
			return fmt.Errorf("the %s harness image: %w", name, err)
		}
	}
	_, _ = fmt.Fprintln(logw, "setup complete — gateway, harness, and runner images ready; QuickAgents can run")
	return nil
}

// errSignInFirst is setup's refusal of a choice nothing is signed in to:
// the page signs in, then saves.
func errSignInFirst(what string) error {
	return fmt.Errorf("%s: sign in first, then save", what)
}

// applySignInSetup records a model sign-in or a connected account as model
// setup's active choice, through the CLI's own setup.ConfigureModel, with
// what this computer keeps standing in for its sign-in: setup signs nothing
// in here, and refuses a choice that is not signed in.
func applySignInSetup(ctx context.Context, home, policyPath, provider string, opts SetupOpts, logw io.Writer) error {
	store := modelsignin.Open(home, nil)
	kept := func(vendor string) string {
		r, err := store.Get(vendor)
		if err != nil || r.Broken != "" {
			return ""
		}
		if r.Label == "" {
			return "an account " + vendor + " did not name"
		}
		return r.Label
	}
	pol, _, err := policy.EnsureDefault(policyPath)
	if err != nil {
		return fmt.Errorf("operator policy: %w", err)
	}
	err = setup.ConfigureModel(&formPrompter{log: logw}, pol, setup.ModelOpts{
		Provider:       provider,
		Model:          opts.Model,
		APIKeyKind:     opts.APIKeyKind,
		CopilotPlan:    opts.CopilotPlan,
		ConnectAccount: opts.Connect,
		Yes:            true,
		ChatGPT: setup.ChatGPTOffer{
			Kept: func() string { return kept(modelsignin.ChatGPT) },
			SignIn: func(fresh bool) (string, []string, error) {
				acct := kept(modelsignin.ChatGPT)
				if fresh || acct == "" {
					return "", nil, errSignInFirst("Sign in with ChatGPT")
				}
				models, _ := ChatGPTModels(ctx) // the model is the form's; the list only names a default
				return acct, models, nil
			},
		},
		Claude: setup.ClaudeOffer{
			Kept:   func() string { return claudesignin.Kept(home) },
			SignIn: func() (string, error) { return "", errSignInFirst(setup.ClaudeSignInLabel) },
		},
		Copilot: setup.CopilotOffer{
			Kept:   func() string { return kept(modelsignin.Copilot) },
			SignIn: func() (string, error) { return "", errSignInFirst("Sign in with GitHub") },
		},
		Connect: setup.ConnectOffer{
			Kept:    func(p string) string { return kept(modelsignin.ConnectedVendor(p)) },
			Connect: func(p string) (string, error) { return "", errSignInFirst(setup.ConnectLabel) },
		},
	})
	if err != nil {
		return err
	}
	if err := policy.Save(policyPath, pol); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "policy written: %s\n", policyPath)
	if !opts.PullImages {
		return nil
	}
	return prepareImages(ctx, home, pol, logw)
}

// formPrompter satisfies setup.Prompter for a GUI whose answers arrived as
// form fields before the flow started: Ask keeps defaults, AskSecret
// leaves values unset, Confirm keeps defaults, Say streams to the log.
// With ModelOpts.Yes=true the flow never actually asks; this exists so a
// future question added to the flow degrades to its default instead of
// hanging a window.
type formPrompter struct{ log io.Writer }

func (f *formPrompter) Ask(_ string, def string) (string, error) { return def, nil }
func (f *formPrompter) AskSecret(string) (string, error)         { return "", nil }
func (f *formPrompter) Confirm(_ string, def bool) (bool, error) { return def, nil }
func (f *formPrompter) Say(text string)                          { _, _ = fmt.Fprintln(f.log, text) }

// Catalog is what ListModels returns: the provider's models, and what the
// key turned out to be with the sentence for a subscription token, so the
// form can say it before anything is stored.
type Catalog struct {
	Provider string
	Models   []setup.Model
	Kind     string
	Notice   string
	// Warning is the plain-http sentence for the endpoint the key was sent
	// to, "" for https and loopback.
	Warning string
}

// ListModels asks provider's models listing with key — the typed key, or
// when none is typed the one a run would read (the saved policy's
// variable, else the OS keychain's entry for the provider) — and returns
// what it listed. The key crosses this call inward and reaches nothing but
// the provider's endpoint (setup.ListModels: http or https, a plain-http
// endpoint off this machine carried out with its warning, no redirect
// followed, the key in no error); the catalog carries names, never the
// key.
func ListModels(ctx context.Context, provider, baseURL, key string) (*Catalog, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	key = strings.TrimSpace(key)
	if key == "" {
		// Nothing typed: the order a run resolves in (engine hostSecret)
		// — the variable the saved policy names, from the environment
		// this app was launched from, then the keychain.
		if v := keyFromPolicyVariable(provider); v != "" {
			key = v
		} else {
			stored, err := keychainLookup(provider)
			if err != nil {
				return nil, fmt.Errorf("no key typed, the saved policy's variable is unset, and %w", err)
			}
			key = stored
		}
	}
	cat, err := listModels(ctx, provider, baseURL, key)
	if err != nil {
		// setup.ListModels scrubs the key from what it returns; this
		// wrapper says so by doing it again over anything it passes on.
		return nil, errors.New(strings.ReplaceAll(err.Error(), key, "<key>"))
	}
	out := &Catalog{Provider: cat.Provider, Models: cat.Models, Kind: policy.CredentialKindOf(provider, key), Warning: cat.Warning}
	if out.Kind == policy.KeyKindSubscription {
		out.Notice = policy.SubscriptionTokenSentence
	}
	return out, nil
}

// keyFromPolicyVariable is the value of the variable the policy names for
// provider, from this process's environment; "" when the policy names
// another provider, no variable, or the variable is unset.
func keyFromPolicyVariable(provider string) string {
	pol, err := loadPolicyIfPresent()
	if err != nil || pol == nil || strings.ToLower(pol.Model.Provider) != provider || pol.Model.APIKeyEnv == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(pol.Model.APIKeyEnv))
}

// StoreModelKey puts key into the OS keychain for provider — its own call,
// so the key crosses the app's IPC in the smallest message that can carry
// it, never beside the rest of the form. The key is written nowhere else,
// and the policy is not touched: the custom provider has no table defaults
// to fill a key-only call's gaps, so the model, the URL and the keyring
// source travel in the ApplySetup the form makes next, which records them.
// A keychain that cannot be reached is a named refusal.
func StoreModelKey(provider, key string, logw io.Writer) error {
	if _, err := storeModelKey(provider, key, logw); err != nil {
		return err
	}
	// A pasted key takes the place of a connected account's key in the same
	// keychain entry, so the connect is forgotten, as the CLI's setup does:
	// what it says names the vendor's own page.
	if setup.IsConnectProvider(provider) {
		if s, _, err := signInStore(); err == nil {
			if outcome, err := s.ForgetConnected(context.Background(), modelsignin.ConnectedVendor(provider)); err == nil && outcome != "" {
				_, _ = fmt.Fprintln(logw, outcome)
			}
		}
	}
	return nil
}

// storeModelKey is the one store path: the keychain write, the line that
// says so, and the kind of what was stored with its sentence for a
// subscription token.
func storeModelKey(provider, key string, logw io.Writer) (string, error) {
	if err := keychainStore(provider, key); err != nil {
		return "", err
	}
	_, _ = fmt.Fprintf(logw, "model key stored in the OS keychain as %s\n", modelkey.Account(provider))
	kind := policy.CredentialKindOf(provider, key)
	if kind == policy.KeyKindSubscription {
		_, _ = fmt.Fprintln(logw, policy.SubscriptionTokenSentence)
	}
	return kind, nil
}

// SetListModelsForTest points the listing at a stand-in for the test's
// duration (the bound surface's tests have no provider to ask).
func SetListModelsForTest(t interface{ Cleanup(func()) }, f func(context.Context, string, string, string) (*setup.Catalog, error)) {
	old := listModels
	listModels = f
	t.Cleanup(func() { listModels = old })
}

// checkBaseURL is ApplySetup's rule for a model base URL: none for a known
// provider (its endpoint is the provider's), and for the custom provider an
// absolute http or https URL with a host. Plain http off this machine is
// accepted: it is stored, and the form says what it costs (EndpointWarning);
// any other scheme is refused.
func checkBaseURL(provider, baseURL string) error {
	if baseURL == "" {
		return nil
	}
	if provider != "custom" {
		return fmt.Errorf("a base URL is for the custom provider only; %q reaches its own endpoint", provider)
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("the base URL must be an absolute http(s) URL: %q", baseURL)
	}
	switch u.Scheme {
	case "https", "http":
		return nil
	default:
		return fmt.Errorf("the base URL must use https (or http): %q", baseURL)
	}
}

// EndpointWarning is the sentence the form shows for a model base URL the
// key would reach over plain http off this machine, "" otherwise: the
// CLI's wording (setup.PlainHTTPWarning), so both clients say the same
// thing.
func EndpointWarning(baseURL string) string { return setup.PlainHTTPWarning(baseURL) }

// errCreditsSignIn is the Agentry credits choice made with no sign-in to
// the SecureAgentry registry this build is for.
var errCreditsSignIn = errors.New("paying with Agentry credits needs you signed in to your SecureAgentry account: sign in first, then choose it again")

// applyCreditsSetup records model provider agentry: the model is paid with
// the credits of the account this computer is signed in to at the
// SecureAgentry registry, and nothing is asked or stored, since each run
// opens a credits session with that sign-in and the registry chooses the
// model. A form never passes a key, model or base URL with it.
func applyCreditsSetup(ctx context.Context, home, policyPath string, opts SetupOpts, logw io.Writer) error {
	if opts.Model != "" || opts.APIKeyEnv != "" || opts.StoreKey != "" || opts.ForgetKey || opts.BaseURL != "" || opts.APIKeySource != "" {
		return errors.New("paying with Agentry credits takes no model, key or endpoint: the registry chooses the model")
	}
	reg := settings.SecureAgentryRegistry()
	if reg == "" || settings.TokenFor(reg) == "" {
		return errCreditsSignIn
	}
	pol, _, err := policy.EnsureDefault(policyPath)
	if err != nil {
		return fmt.Errorf("operator policy: %w", err)
	}
	if err := setup.ConfigureModel(&formPrompter{log: logw}, pol, setup.ModelOpts{
		Provider: policy.ProviderAgentry, AgentryRegistry: reg, Yes: true,
	}); err != nil {
		return err
	}
	if err := policy.Save(policyPath, pol); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(logw, "policy written: %s\n", policyPath)
	if !opts.PullImages {
		return nil
	}
	return prepareImages(ctx, home, pol, logw)
}
