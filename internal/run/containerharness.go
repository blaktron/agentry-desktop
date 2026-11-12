package run

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// HarnessChoice is Settings → Models' harness: the container harness the
// runner policy chooses, the two it is chosen from, and the harness a run on
// the default model takes, which is the vendor's own for a model paid by a
// model sign-in whatever the choice says. It is what `agentry harness
// --json` reports, read through the same packages.
type HarnessChoice struct {
	// Harness is the container harness the policy chooses: docker-agent
	// when the policy does not name one.
	Harness string `json:"harness"`
	// Choices are the container harnesses, the default first.
	Choices []string `json:"choices"`
	// RunsOn is the harness a run on the default model uses when it names
	// none: Harness, or the vendor's for a model paid by a model sign-in.
	RunsOn string `json:"runsOn"`
	// PolicyPath is the policy file that holds the choice.
	PolicyPath string `json:"policyPath"`
}

// ContainerHarness reads the container harness this computer's runs use.
// With no policy file yet it is the default, and nothing is written.
func ContainerHarness() (*HarnessChoice, error) {
	pol, path, _, err := readModelsPolicy()
	if err != nil {
		return nil, err
	}
	return harnessChoice(pol, path), nil
}

// SetContainerHarness chooses the container harness runs, chats and
// persistent agents on this computer use from their next start, as
// `agentry harness use` does. The default is written as no key at all, so
// a policy that never chose stays as it was and an older CLI or app on the
// same computer keeps reading it; choosing the default with no policy file
// writes nothing. Every other key of the policy is kept.
func SetContainerHarness(name string) (*HarnessChoice, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || policy.CheckContainerHarness(name) != nil {
		return nil, fmt.Errorf("%q is not a container harness: choose %s", name, strings.Join(policy.ContainerHarnesses(), " or "))
	}
	path, err := policy.DefaultPath()
	if err != nil {
		return nil, err
	}
	if name == policy.DockerAgentAdapter {
		pol, _, err := policy.Load(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			return harnessChoice(policy.Default(), path), nil
		case err != nil:
			return nil, fmt.Errorf("operator policy: %w", err)
		}
		if pol.ContainerHarness == "" {
			return harnessChoice(pol, path), nil
		}
		pol.ContainerHarness = ""
		if err := policy.Save(path, pol); err != nil {
			return nil, err
		}
		return harnessChoice(pol, path), nil
	}
	pol, _, err := policy.EnsureDefault(path)
	if err != nil {
		return nil, fmt.Errorf("operator policy: %w", err)
	}
	pol.ContainerHarness = name
	if err := policy.Save(path, pol); err != nil {
		return nil, err
	}
	return harnessChoice(pol, path), nil
}

func harnessChoice(pol *policy.Policy, path string) *HarnessChoice {
	return &HarnessChoice{
		Harness:    pol.RunContainerHarness(),
		Choices:    policy.ContainerHarnesses(),
		RunsOn:     adapter.DefaultFor(pol),
		PolicyPath: path,
	}
}
