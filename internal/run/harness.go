// The coding agents a person may run BriefAgents through (plan
// desktop-installers.md §9, agentry-desktop#89): the host harnesses behind
// the CLI's adapters — Claude Code, Codex — probed on this machine with the
// same package `agentry doctor --adapter` uses, and the one the person
// chooses recorded in the operator policy so `agentry run --adapter <name>`
// finds it wherever it lives. The desktop's own runs use the container
// harness (ARCHITECTURE §7); choosing here configures the machine, it does
// not switch what this app runs.
package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
	"github.com/blaktron/agentry-cli/pkg/runner/harnessprobe"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// harnessProbeTimeout bounds each probe: the wizard renders off the
// interface thread, but a harness that hangs on --version must not hold
// the step for long.
const harnessProbeTimeout = 5 * time.Second

// Harness is one host harness as the wizard's Coding agent step shows it.
type Harness struct {
	// Name is the adapter name (claude, codex); DisplayName the product's
	// ("Claude Code"), from the adapter (agentry-cli#151).
	Name        string
	DisplayName string
	// Executable is the program the adapter looks for on PATH.
	Executable string
	// InstallURL is the vendor's page, for when Found is false.
	InstallURL string
	// Found, Path, Version, VersionMatches, Warnings and Reason are the
	// probe's facts (see harnessprobe.Result).
	Found          bool
	Path           string
	PathFromPolicy bool
	// Source says where Path was found — the policy, PATH, or a usual
	// install location such as a desktop app's bundled build
	// (agentry-cli#167); the wizard shows the last.
	Source         string
	Version        string
	VersionMatches bool
	Warnings       []string
	Reason         string
	// Chosen is true when the operator policy carries a harness.<name>
	// entry — the person picked this one, here or in the CLI.
	Chosen bool
	// MCPCommand is the one line that registers the agentry-mcp server
	// with this host, from the adapter (verbatim from agentry-mcp's
	// README); the app only shows it (D14: the CLI is not shipped, the
	// command is the person's).
	MCPCommand string
}

// HarnessStatus probes every host adapter the CLI registers, reading the
// operator policy when it exists (for path overrides, and to say which is
// chosen) and never creating it.
func HarnessStatus(ctx context.Context) ([]Harness, error) {
	pol, err := loadPolicyIfPresent()
	if err != nil {
		return nil, err
	}
	names := adapter.Names()
	sort.Strings(names)
	var out []Harness
	for _, name := range names {
		pctx, cancel := context.WithTimeout(ctx, harnessProbeTimeout)
		r := harnessprobe.Probe(pctx, pol, name)
		cancel()
		if !r.Known || !r.Host {
			continue
		}
		h := Harness{
			Name: name, DisplayName: r.DisplayName, Executable: r.Executable, InstallURL: r.InstallURL,
			Found: r.Found, Path: r.Path, PathFromPolicy: r.PathFromPolicy, Source: string(r.Source),
			Version: r.Version, VersionMatches: r.VersionMatches, Warnings: r.Warnings, Reason: r.Reason,
			MCPCommand: r.MCPCommand,
		}
		if pol != nil {
			_, h.Chosen = pol.Harness[name]
		}
		out = append(out, h)
	}
	return out, nil
}

// ChooseHarness records harness.<name> in the operator policy — the path
// the probe resolved, so a run launched from a GUI with no PATH of its own
// still finds it — creating the policy from defaults when there is none
// and preserving every other block. A harness that is not on this machine
// cannot be chosen: the refusal names where to get it.
func ChooseHarness(ctx context.Context, name string) error {
	pol, err := loadPolicyIfPresent()
	if err != nil {
		return err
	}
	pctx, cancel := context.WithTimeout(ctx, harnessProbeTimeout)
	r := harnessprobe.Probe(pctx, pol, name)
	cancel()
	switch {
	case !r.Known:
		return fmt.Errorf("coding agent: %s", r.Reason)
	case !r.Host:
		return fmt.Errorf("coding agent: %s is a containerised harness; there is nothing to choose", name)
	case !r.Found:
		return fmt.Errorf("coding agent: %s — get it from %s", r.Reason, r.InstallURL)
	}
	policyPath, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	pol, _, err = policy.EnsureDefault(policyPath)
	if err != nil {
		return fmt.Errorf("operator policy: %w", err)
	}
	if pol.Harness == nil {
		pol.Harness = map[string]policy.HarnessPolicy{}
	}
	hp := pol.Harness[name]
	if !r.PathFromPolicy {
		hp.Path = r.Path
	}
	pol.Harness[name] = hp
	return policy.Save(policyPath, pol)
}

// ForgetHarness removes harness.<name> from the operator policy — the
// person un-choosing a coding agent in Settings (agentry-desktop#148), so
// the export the policy implies can change: a chosen Claude Code outranks
// Codex there, and only its absence lets AGENTS.md through. Every other
// block is preserved; a name that is not chosen, or no policy at all, is
// nothing to do and creates nothing.
func ForgetHarness(name string) error {
	pol, err := loadPolicyIfPresent()
	if err != nil || pol == nil {
		return err
	}
	if _, chosen := pol.Harness[name]; !chosen {
		return nil
	}
	delete(pol.Harness, name)
	policyPath, err := policy.DefaultPath()
	if err != nil {
		return err
	}
	return policy.Save(policyPath, pol)
}

// ChosenHarnesses names the harnesses the operator policy carries an
// entry for, in order — what the person picked, here or in the CLI — by
// reading the policy alone: nothing is executed, so a recap can ask
// without running anyone's binaries. Each carries its product name from
// the adapter, so the recap says "Claude Code", not "claude".
func ChosenHarnesses() ([]Harness, error) {
	pol, err := loadPolicyIfPresent()
	if err != nil || pol == nil {
		return nil, err
	}
	names := make([]string, 0, len(pol.Harness))
	for name := range pol.Harness {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Harness, 0, len(names))
	for _, name := range names {
		h := Harness{Name: name, DisplayName: name, Chosen: true}
		if ad, aerr := adapter.Get(name); aerr == nil {
			if hh, ok := ad.(adapter.HostHarness); ok {
				h.DisplayName = hh.DisplayName()
			}
		}
		out = append(out, h)
	}
	return out, nil
}

// loadPolicyIfPresent is the harness surfaces' read of the operator
// policy: the policy when the file exists, nil when it does not, an error
// when it is broken. (GetStatus keeps its own read: it reports defaults in
// place of a missing file, which these surfaces must not.)
func loadPolicyIfPresent() (*policy.Policy, error) {
	policyPath, err := policy.DefaultPath()
	if err != nil {
		return nil, err
	}
	pol, _, err := policy.Load(policyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("operator policy: %w", err)
	}
	return pol, nil
}
