// Declared-MCP-server resolution for desktop runs (RUNNER-PLAN-3),
// mirroring the CLI: reuse the signature store's recorded resolutions
// offline; consult the configured registries only when the store is
// incomplete. EnsureInstalled later refuses recorded-pin drift.
package run

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/mcpinstall"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

func ensureServerResolutions(ctx context.Context, logw io.Writer, serversDir string,
	pol *policy.Policy, m *manifest.Manifest,
) ([]mcpinstall.Resolved, error) {
	declared := m.Capabilities.MCPServers
	stored := make([]mcpinstall.Resolved, 0, len(declared))
	for _, srv := range declared {
		meta, err := mcpinstall.LoadMeta(serversDir, mcpinstall.IDFor(srv))
		if err != nil {
			return nil, err
		}
		pkg := srv.NPM
		if pkg == "" {
			pkg = srv.PyPI
		}
		if meta == nil || meta.Package != pkg || meta.Version != srv.Version {
			stored = nil
			break
		}
		stored = append(stored, *meta)
	}
	if len(stored) == len(declared) && len(declared) > 0 {
		return stored, nil
	}
	_, _ = fmt.Fprintf(logw, "resolving %d declared MCP server(s) against the configured registries\n", len(declared))
	return mcpinstall.ResolveAll(ctx, nil, pol.MCPRegistries, declared)
}

// vulnerabilityVerdict mirrors the CLI's decision (agentry-cli#59): the
// receipt line, and an error when a declared server carries a known high or
// critical advisory that nothing overrides. Overrides are named in the line.
func vulnerabilityVerdict(statuses []mcpinstall.VulnStatus, registry string, allowRequest, allowPolicy bool) (string, error) {
	line := "checked against " + registry + ": " + mcpinstall.Summary(statuses)
	var refusing []mcpinstall.VulnStatus
	for _, s := range statuses {
		if s.Refuses() {
			refusing = append(refusing, s)
		}
	}
	if len(refusing) == 0 {
		return line, nil
	}
	switch {
	case allowRequest:
		return line + "; run allowed by the operator (allow vulnerable MCP servers)", nil
	case allowPolicy:
		return line + "; run allowed by policy mcpServers.allowVulnerable", nil
	}
	parts := make([]string, 0, len(refusing))
	for _, s := range refusing {
		p := s.Ref + " has a known " + s.MaxSeverity + " vulnerability"
		var ids []string
		for _, a := range s.Advisories {
			if a.Severity == "high" || a.Severity == "critical" {
				ids = append(ids, a.ID)
			}
		}
		if len(ids) > 0 {
			p += " (" + strings.Join(ids, ", ") + ")"
		}
		if fixed := s.FixedIn(); len(fixed) > 0 {
			p += ", fixed in " + strings.Join(fixed, ", ")
		}
		parts = append(parts, p)
	}
	return line, fmt.Errorf("declared MCP server refused by the registry's vulnerability scan: %s — pin a fixed version, or allow vulnerable MCP servers for this run (policy mcpServers.allowVulnerable)",
		strings.Join(parts, "; "))
}
