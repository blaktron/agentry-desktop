package install

import (
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// CodingAgentExport is Settings → Coding agents as the install reads it
// (agentry-desktop#148, plan desktop-one-click-install.md §6, D6): the
// coding-agent export the operator policy implies — the one rule the
// runner home's export/ follows (installdir.StyleFor: a chosen Claude Code
// → the Claude skill, else a chosen Codex → AGENTS.md, else an Anthropic
// key → the Claude skill, else AGENTS.md) — the folder it is written to,
// and whether a one-click Install writes it.
type CodingAgentExport struct {
	// Target is claude-skill or agents-md.
	Target string `json:"target"`
	// Dir is the folder: ~/.claude/skills for the Claude skill (where
	// Claude Code reads; not a choice), the project directory Settings
	// holds for AGENTS.md ("" until one is chosen through the picker).
	Dir string `json:"dir"`
	// NeedsDir is true when the export is AGENTS.md and no project
	// directory is chosen yet: nothing can be written until one is.
	NeedsDir bool `json:"needsDir"`
	// Enabled is the switch: "also install into <coding agent>".
	Enabled bool `json:"enabled"`
}

// ExportSettings reads the policy and the settings into what Settings →
// Coding agents shows and what a one-click install with the switch on
// writes. A policy that is missing or cannot be read implies AGENTS.md,
// as the runner home's export does.
func ExportSettings() (CodingAgentExport, error) {
	s, err := settings.Load()
	if err != nil {
		return CodingAgentExport{}, err
	}
	x := CodingAgentExport{Target: installdir.StyleFor(loadPolicyForStyle()), Enabled: s.InstallIntoCodingAgent}
	switch x.Target {
	case manifest.InterfaceClaudeSkill:
		root, err := claudeSkillsRoot()
		if err != nil {
			return CodingAgentExport{}, err
		}
		x.Dir = root
	default:
		x.Dir = s.CodingAgentProjectDir
		x.NeedsDir = x.Dir == ""
	}
	return x, nil
}

// PlanDir is the directory to prepare the export with: the project
// directory for AGENTS.md, and nothing for the Claude skill — its Dir is
// the target's own default layout, which ResolvePaths lays out managed
// only when no directory is handed to it.
func (x CodingAgentExport) PlanDir() string {
	if x.Target == manifest.InterfaceAgentsMD {
		return x.Dir
	}
	return ""
}

// loadPolicyForStyle is the policy as the export rule reads it: the file
// when it loads, nil when it is missing or broken — the runner home's
// export takes the same view, so both agree on the style.
func loadPolicyForStyle() *policy.Policy {
	path, err := policy.DefaultPath()
	if err != nil {
		return nil
	}
	pol, _, err := policy.Load(path)
	if err != nil {
		return nil
	}
	return pol
}
