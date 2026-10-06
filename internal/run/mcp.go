// Manifest-capability → tool-surface mapping for the "MCP servers & tools"
// section (RUNNER-PLAN-2 §P6, req 3/4 parity with the site). The gateway
// rows mirror the runner's own compiler (agentry-cli pkg/runner/compile
// toolSpec + runnerimage/toolsrv GrantedTools) and the web page's
// lib/mcp.ts; the embedded list is computed by the CLI's actual exporter
// mapping (pkg/export.HostToolGrants) — this section must never describe
// a tool set that differs from what `agentry run` / `agentry install`
// would actually do.
package run

import (
	"context"
	"github.com/blaktron/agentry-desktop/internal/mcpview"
	"github.com/blaktron/agentry-desktop/internal/mcpvuln"
	"sort"
	"strings"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/export"
)

// ToolRow is one granted gateway tool and why it exists.
type ToolRow struct {
	Tool      string
	GrantedBy string
}

// MCPInfo is everything the agent page's "MCP servers & tools" section
// shows, derived entirely from the latest published manifest.
type MCPInfo struct {
	Ref     string
	Version string
	// ToolRows are the companion-server tools the runner grants.
	ToolRows []ToolRow
	// Servers are the MCP servers involved in a run: the companion server's
	// line, then one line per Declared entry, in the same order.
	Servers []string
	// Declared is each declared MCP server's pin, tools and plain env
	// (agentry-desktop#62), for the lines Servers ends with.
	Declared []mcpview.Server
	// SecretNames are the declared secret env NAMES (never values).
	SecretNames []string
	// DataAccess lists declared semantic categories (operator-cataloged
	// servers may be attached for them; undeclared ⇒ never wired in).
	DataAccess []string
	// EmbeddedTools is the host built-in allowed-tools an embedded install
	// grants (pkg/export.HostToolGrants).
	EmbeddedTools []string
	// CapabilityLines is the declared contract, rendered.
	CapabilityLines []string
	// Vulnerabilities is the registry's verdict on each declared MCP server
	// (agentry#177); VulnerabilityLine is the receipt line a run would
	// record, Refusal the refusal a run meets with Unsafe Mode off ("" when
	// it would run), and Refusing the refusing servers alone, which the
	// consent Unsafe Mode offers names (D-055).
	Vulnerabilities   []mcpvuln.Status
	VulnerabilityLine string
	Refusal           string
	Refusing          string
}

// Requirements resolves ns/name's latest published release and computes
// its tool surface.
func Requirements(ctx context.Context, regURL, ns, name string) (*MCPInfo, error) {
	sig, err := ResolveSignature(ctx, regURL, ns+"/"+name, false)
	if err != nil {
		return nil, err
	}
	return requirementsOf(ctx, regURL, sig), nil
}

// RunRequirements is Requirements for the release a run of ref would run:
// a local install of it first (agentry-desktop#425), else the registry's.
// The run panel reads this; the published agent's page reads Requirements.
func RunRequirements(ctx context.Context, regURL, ref string) (*MCPInfo, error) {
	sig, err := resolveForRun(ctx, regURL, ref, false)
	if err != nil {
		return nil, err
	}
	return requirementsOf(ctx, regURL, sig), nil
}

func requirementsOf(ctx context.Context, regURL string, sig *Signature) *MCPInfo {
	c := sig.Manifest.Capabilities
	info := &MCPInfo{
		Ref:      sig.Ref,
		Version:  sig.Version,
		ToolRows: gatewayToolRows(c),
		Servers: []string{
			"QuickAgent Tools — the runner's companion server, served through a per-run Docker MCP Gateway; its tool surface is exactly the table above",
		},
		SecretNames:     append([]string(nil), c.Secrets...),
		EmbeddedTools:   export.HostToolGrants(c),
		CapabilityLines: capabilityLines(c),
		Declared:        mcpview.Of(c),
	}
	for _, d := range c.DataAccess {
		info.DataAccess = append(info.DataAccess, string(d))
	}
	// Declared MCP servers (RUNNER-PLAN-3): installed with the agent from
	// the operator's registries, loaded for this exact release only.
	info.Servers = append(info.Servers, declaredServerLines(info.Declared)...)
	vr := mcpvuln.Check(ctx, regURL, c.MCPServers)
	info.Vulnerabilities, info.VulnerabilityLine, info.Refusal, info.Refusing = vr.Statuses, vr.Line, vr.Refusal, vr.Refusing
	return info
}

// declaredServerLines is one line per declared server, in order: the page
// renders each server's env beneath its line by that order
// (agentry-desktop#62).
func declaredServerLines(declared []mcpview.Server) []string {
	lines := make([]string, 0, len(declared))
	for _, srv := range declared {
		desc := srv.ID + " — declared by the manifest; installed per-signature from the operator's configured registries"
		if len(srv.Tools) > 0 {
			desc += "; tools filtered to " + strings.Join(srv.Tools, ", ")
		}
		lines = append(lines, desc)
	}
	return lines
}

// gatewayToolRows maps the contract to the tools the per-run gateway
// serves: the always-on report/input channels (§P4) plus one row per
// declared capability. Rows that do not apply are omitted, not shown
// empty — an undeclared capability is a tool that does not exist.
func gatewayToolRows(c manifest.Capabilities) []ToolRow {
	rows := []ToolRow{
		{Tool: "write_report", GrantedBy: "always granted — reports land in the run's output folder"},
		{Tool: "list_inputs", GrantedBy: "always granted — operator-provided input files, read-only"},
		{Tool: "read_input", GrantedBy: "always granted — operator-provided input files, read-only"},
	}
	if scope, ok := fsScope(c.Filesystem.Read); ok {
		rows = append(rows,
			ToolRow{Tool: "fs_read", GrantedBy: "filesystem.read — " + scope},
			ToolRow{Tool: "fs_list", GrantedBy: "filesystem.read — " + scope},
		)
	}
	if scope, ok := fsScope(c.Filesystem.Write); ok {
		rows = append(rows, ToolRow{Tool: "fs_write", GrantedBy: "filesystem.write — " + scope})
	}
	if eg := c.Network.Egress; !eg.IsZero() && !eg.None && len(eg.Hosts) > 0 {
		note := "network.egress — pinned to " + strings.Join(eg.Hosts, ", ")
		for _, h := range eg.Hosts {
			if h == "*" {
				note = `network.egress — unrestricted ("*") — refused at run time unless the operator's policy allows wildcard egress`
				break
			}
		}
		rows = append(rows, ToolRow{Tool: "fetch", GrantedBy: note})
	}
	if x := c.Exec; !x.IsZero() && !x.None && len(x.Commands) > 0 {
		rows = append(rows, ToolRow{
			Tool:      "run_command",
			GrantedBy: "exec.commands — restricted to " + strings.Join(x.Commands, ", "),
		})
	}
	return rows
}

// fsScope renders one filesystem direction's scope; ok=false means the
// direction grants nothing.
func fsScope(a manifest.FSAccess) (string, bool) {
	switch {
	case a.IsZero(), a.Mode == manifest.FSModeNone:
		return "", false
	case a.Mode == manifest.FSModePaths:
		return "only " + strings.Join(a.Paths, ", "), true
	default:
		return "the whole task workspace", true
	}
}

// capabilityLines renders the declared contract as stable lines — the
// CLI's §5.4 display (cmd/agentry caps.go), replicated because cmd/agentry
// is package main (same call as D-006).
func capabilityLines(c manifest.Capabilities) []string {
	if isZeroCapabilities(c) {
		return []string{"none declared (pure text agent)"}
	}
	lines := []string{
		"network.egress: " + renderEgress(c.Network.Egress),
		"filesystem.read: " + renderFSAccess(c.Filesystem.Read),
		"filesystem.write: " + renderFSAccess(c.Filesystem.Write),
		"exec: " + renderExec(c.Exec),
		"secrets: " + renderList(c.Secrets),
		"dataAccess: " + renderDataAccess(c.DataAccess),
	}
	for _, srv := range c.MCPServers {
		line := "mcpServer: " + srv.SourceID() + "@" + srv.Version
		if srv.IsRemote() {
			// A remote server has no package version (agentry-cli#663): it
			// is its address, at its vendor, signed in to as it declares.
			line = "mcpServer: " + srv.URL + " (remote, at its vendor; auth: " + string(srv.Auth) + ")"
		}
		if len(srv.Egress) > 0 {
			line += " (egress: " + renderList(srv.Egress) + ")"
		}
		if len(srv.Secrets) > 0 {
			line += " (secrets: " + renderList(srv.Secrets) + ")"
		}
		lines = append(lines, line)
	}
	// Shipped knowledge (plan knowledge.md §4.6): packs inside the release
	// the agent reads, part of what is consented to (the CLI's line).
	if len(c.Knowledge) > 0 {
		lines = append(lines, "knowledge: ships "+strings.Join(c.Knowledge, ", "))
	}
	return lines
}

// allSecretNames is the union of agent-level and per-server secret names
// (RUNNER-PLAN-3) — what the guided form collects and validates against.
func allSecretNames(m *manifest.Manifest) []string {
	set := map[string]bool{}
	for _, n := range m.Capabilities.Secrets {
		set[n] = true
	}
	for _, srv := range m.Capabilities.MCPServers {
		for _, n := range srv.Secrets {
			set[n] = true
		}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func isZeroCapabilities(c manifest.Capabilities) bool {
	if len(c.MCPServers) > 0 {
		return false
	}
	return c.Network.Egress.IsZero() &&
		c.Filesystem.Read.IsZero() && c.Filesystem.Write.IsZero() &&
		c.Exec.IsZero() &&
		len(c.Secrets) == 0 && len(c.DataAccess) == 0 && len(c.Knowledge) == 0
}

func renderEgress(e manifest.Egress) string {
	if e.None || len(e.Hosts) == 0 {
		return "none"
	}
	s := strings.Join(e.Hosts, ", ")
	for _, h := range e.Hosts {
		if h == "*" {
			return s + " (unrestricted — policy-gated)"
		}
	}
	return s
}

func renderFSAccess(a manifest.FSAccess) string {
	switch {
	case a.IsZero(), a.Mode == manifest.FSModeNone:
		return "none"
	case len(a.Paths) > 0:
		return "paths: " + strings.Join(a.Paths, ", ")
	default:
		return string(a.Mode)
	}
}

func renderExec(x manifest.ExecCapability) string {
	if x.IsZero() || x.None || len(x.Commands) == 0 {
		return "none"
	}
	return "commands: " + strings.Join(x.Commands, ", ")
}

func renderList(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}

func renderDataAccess(items []manifest.DataAccess) string {
	if len(items) == 0 {
		return "none"
	}
	out := make([]string, len(items))
	for i, d := range items {
		out[i] = string(d)
	}
	return strings.Join(out, ", ")
}
