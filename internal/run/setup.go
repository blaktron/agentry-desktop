// Machine setup for BriefAgent runs (RUNNER-PLAN-2 §P1, desktop shape):
// the same pkg/runner/setup logic `agentry setup` drives, behind a form
// Prompter instead of a TTY — no parallel implementation (§P8).
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/gateway"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/setup"
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
	// KeyStored says whether the keychain holds a key for Provider;
	// KeyStoreErr is why that could not be asked (no keychain, disabled).
	// Neither ever carries the value.
	KeyStored   bool
	KeyStoreErr string
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
	if stored, kerr := modelkey.Present(pol.Model.Provider); kerr != nil {
		st.KeyStoreErr = kerr.Error()
	} else {
		st.KeyStored = stored
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
	// keychain for Provider first — the one moment the value passes
	// through this process, on its way to the keychain and nowhere else;
	// storing makes the source keyring. ForgetKey removes the stored key
	// and makes the source env.
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
	switch {
	case opts.StoreKey != "":
		if err := modelkey.Store(provider, opts.StoreKey); err != nil {
			return err
		}
		source = modelkey.SourceKeyring
		_, _ = fmt.Fprintf(logw, "model key stored in the OS keychain as %s\n", modelkey.Account(provider))
	case opts.ForgetKey:
		if err := modelkey.Delete(provider); err != nil {
			return err
		}
		source = modelkey.SourceEnv
		_, _ = fmt.Fprintf(logw, "model key removed from the OS keychain (%s)\n", modelkey.Account(provider))
	}
	// A keyring source with nothing behind it would record a fallback a
	// run cannot honour: refuse unless a key is stored (or was just now).
	if source == modelkey.SourceKeyring && opts.StoreKey == "" {
		present, perr := modelkey.Present(provider)
		switch {
		case perr != nil:
			return perr
		case !present:
			return fmt.Errorf("the OS keychain holds no key for %s: type the key to store it, or choose the environment variable", provider)
		}
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

// StoreModelKey puts key into the OS keychain for provider and records the
// keychain as the source in the operator policy — its own call, so the key
// crosses the app's IPC in the smallest message that can carry it, never
// beside the rest of the form (desktop#102). The key is written nowhere
// else; a keychain that cannot be reached is a named refusal and the
// policy is untouched.
func StoreModelKey(ctx context.Context, provider, key string, logw io.Writer) error {
	return ApplySetup(ctx, SetupOpts{Provider: provider, StoreKey: key}, logw)
}
