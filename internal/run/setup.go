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
	BaseURL      string
	ImagesReady  bool
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
	Provider   string
	Model      string
	APIKeyEnv  string
	BaseURL    string
	PullImages bool
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
	pol, _, err := policy.EnsureDefault(policyPath)
	if err != nil {
		return fmt.Errorf("operator policy: %w", err)
	}
	if err := setup.ConfigureModel(&formPrompter{log: logw}, pol, setup.ModelOpts{
		Provider:  opts.Provider,
		Model:     opts.Model,
		APIKeyEnv: opts.APIKeyEnv,
		BaseURL:   opts.BaseURL,
		Yes:       true, // a form never mid-flight-prompts; gaps fill from the table
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
	if err := gateway.EnsureInstalled(ctx, docker, pol, runnerImageTag(pol), filepath.Join(home, "image"), logw); err != nil {
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
