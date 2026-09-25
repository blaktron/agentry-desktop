// Machine setup for BriefAgent runs (RUNNER-PLAN-2 §P1, desktop shape):
// the same pkg/runner/setup logic `agentry setup` drives, behind a form
// Prompter instead of a TTY — no parallel implementation (§P8).
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/gateway"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/setup"
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

// Status is the setup state of this machine: can it run BriefAgents, and
// with which LLM provider (§P1, req 5).
type Status struct {
	DockerOK     bool
	DockerErr    string
	PolicyPath   string
	PolicyExists bool
	Provider     string
	Model        string
	APIKeyEnv    string
	// APIKeySource is "keyring" when the OS keychain is the fallback for
	// an unset variable; "" when the environment is the only source
	// (plan desktop-installers.md §9, D12; agentry-cli#148).
	APIKeySource string
	// KeysStored lists every provider the setup form can show that has a
	// key in the keychain, so the form's badge is right for the provider
	// chosen and not only for the policy's (agentry-desktop#141);
	// KeyStoreErr is why the keychain could not be asked (none, disabled),
	// in which case the list is not complete. Neither carries a value.
	KeysStored  []string
	KeyStoreErr string
	// APIKeyKind is the policy's apiKeyKind (agentry-cli#171): "" or
	// "api", or "subscription" for an Anthropic subscription token, which
	// drives Claude Code alone; KeyNotice is the sentence to show beside
	// the field for that kind, "" otherwise.
	APIKeyKind  string
	KeyNotice   string
	BaseURL     string
	ImagesReady bool
	// Budget defaults, for the run form's placeholders.
	TimeoutSeconds int
	MaxTurns       int
	// KeepProfilesByDefault seeds keep toggles (§P2).
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
	// One ask per provider. Each reads the presence marker setup stores
	// beside the key, never the key itself (modelkey.Present,
	// agentry-cli#156): only a key stored before markers existed is read,
	// once, to write its marker. A marker outlives a key removed outside
	// agentry, so this can say "stored" until setup stores a key again or
	// `agentry doctor` confirms the entry; the apply step reads the key
	// itself (keychainLookup) and is not misled.
	//
	// An absent entry is an answer; an error is the keychain itself —
	// none, disabled, locked — and would be the same error, and on a
	// locked Secret Service the same prompt, for every provider after it,
	// so the first one ends the asking and is said.
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
	st.BaseURL = pol.Model.BaseURL
	st.TimeoutSeconds = pol.Budgets.TimeoutSeconds
	st.MaxTurns = pol.Budgets.MaxTurns
	st.KeepProfilesByDefault = pol.KeepProfilesByDefault()

	docker := newDockerClient()
	if err := docker.Available(ctx); err != nil {
		st.DockerErr = err.Error()
		return st, nil
	}
	st.DockerOK = true
	st.ImagesReady = imagesReady(ctx, docker, pol)
	return st, nil
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
// the pinned gateway and harness images plus the locally built runner
// image (§R7).
func imagesReady(ctx context.Context, docker dockerx.Client, pol *policy.Policy) bool {
	for _, ref := range []string{pol.Images.Gateway, pol.Images.HarnessDockerAgent, runnerImageTag(pol)} {
		ok, err := docker.ImageExists(ctx, ref)
		if err != nil || !ok {
			return false
		}
	}
	return true
}

// SetupOpts is the settings form's model block (§P1 provider table; empty
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
}

// ApplySetup writes the LLM provider preference into the operator policy
// (preserving everything outside the model block) and, when PullImages,
// stands up the gateway plane immediately — §P1's two halves. Image
// pull/build progress streams to logw.
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
	// A base URL repoints the model call — the key goes wherever it says,
	// and the agent's only egress becomes that host — so it is judged
	// before anything is stored: the custom provider only (the page shows
	// the field for that one; Go enforced nothing, so script in the
	// webview could aim a known provider at any host), and https, or
	// plain http to loopback for a local server (security review
	// 2026-09-20 §8.4; D-030).
	if err := checkBaseURL(provider, opts.BaseURL); err != nil {
		return err
	}
	// The kind (agentry-cli#171) is read from the value while this
	// process holds it — a key stored now, or below the one the keychain
	// holds; a forgotten key takes its kind with it; the variable path
	// knows nothing and keeps what the policy has (setup.ConfigureModel's
	// rule for the same provider).
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
	// A keyring source with nothing behind it would record a fallback a
	// run cannot honour: refuse unless a key is stored (or was just now).
	// When one is, its kind is read from the stored value — the key came
	// through StoreModelKey in a call of its own (agentry-desktop#139),
	// and on a provider switch the policy has no kind to keep.
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
	docker := newDockerClient()
	// Setup prepares the whole plane before any agent is chosen, so the
	// harness image is wanted here whatever a later run turns out to use.
	if err := gateway.EnsureInstalled(ctx, docker, pol, runnerImageTag(pol), filepath.Join(home, "image"), logw,
		true); err != nil {
		return fmt.Errorf("gateway/docker environment: %w", err)
	}
	_, _ = fmt.Fprintln(logw, "setup complete — gateway, harness, and runner images ready; BriefAgents can run")
	return nil
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
// key turned out to be (agentry-cli#171) with the sentence for a
// subscription token, so the form can say it before anything is stored.
type Catalog struct {
	Provider string
	Models   []setup.Model
	Kind     string
	Notice   string
}

// ListModels asks provider's models listing with key — the typed key, or
// when none is typed the one a run would read (the saved policy's
// variable, else the OS keychain's entry for the provider) — and
// returns what it listed (agentry-cli#170, agentry-desktop#131). The key
// crosses this call inward and reaches nothing but the provider's endpoint
// (setup.ListModels: https only off loopback, no redirect followed, the
// key in no error); the catalog carries names, never the key.
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
	out := &Catalog{Provider: cat.Provider, Models: cat.Models, Kind: policy.CredentialKindOf(provider, key)}
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
// it, never beside the rest of the form (desktop#102). The key is written
// nowhere else, and the policy is not touched: the custom provider has no
// table defaults to fill a key-only call's gaps, so the model, the URL and
// the keyring source travel in the ApplySetup the form makes next, which
// records them (agentry-desktop#139). A keychain that cannot be reached is
// a named refusal.
func StoreModelKey(provider, key string, logw io.Writer) error {
	_, err := storeModelKey(provider, key, logw)
	return err
}

// storeModelKey is the one store path: the keychain write, the line that
// says so, and the kind of what was stored (agentry-cli#171) with its
// sentence for a subscription token.
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
// provider (its endpoint is the provider's), and for the custom provider
// https, or http to loopback.
func checkBaseURL(provider, baseURL string) error {
	if baseURL == "" {
		return nil
	}
	if provider != "custom" {
		return fmt.Errorf("a base URL is for the custom provider only; %q reaches its own endpoint", provider)
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("the base URL must be an absolute https URL: %q", baseURL)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("the base URL must use https off this machine: %q would send the model key in clear", baseURL)
	default:
		return fmt.Errorf("the base URL must use https: %q", baseURL)
	}
}

func isLoopbackHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
