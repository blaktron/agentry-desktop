// Package mcpvuln is the desktop's view of the registry's vulnerability
// verdict on a release's declared MCP servers (agentry#177, DR-16): the
// same decision the CLI's `agentry run` makes, shared by the run path,
// the install wizard and the agent page so all three say one thing.
package mcpvuln

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/mcpinstall"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry/pkg/manifest"
)

// Advisory is one advisory the registry reports for a server version.
type Advisory struct {
	ID       string   `json:"id"`
	Severity string   `json:"severity"`
	Summary  string   `json:"summary"`
	FixedIn  []string `json:"fixedIn"`
}

// Status is the registry's answer for one declared server, as the
// frontend shows it.
type Status struct {
	// Ref is "npm:pkg@1.2.3" / "pypi:pkg@1.2.3".
	Ref string `json:"ref"`
	// Status is safe, vulnerable, error, unknown or unreachable (see
	// mcpinstall.VulnStatus).
	Status      string     `json:"status"`
	MaxSeverity string     `json:"maxSeverity,omitempty"`
	Advisories  []Advisory `json:"advisories"`
	CheckedAt   string     `json:"checkedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
	// Refuses says this status alone refuses a run by default.
	Refuses bool `json:"refuses"`
}

// Views renders the runner's statuses for the frontend (never nil).
func Views(statuses []mcpinstall.VulnStatus) []Status {
	out := make([]Status, 0, len(statuses))
	for _, s := range statuses {
		v := Status{Ref: s.Ref, Status: s.Status, MaxSeverity: s.MaxSeverity, Error: s.Err, Refuses: s.Refuses(), Advisories: []Advisory{}}
		if !s.CheckedAt.IsZero() {
			v.CheckedAt = s.CheckedAt.Format(time.RFC3339)
		}
		for _, a := range s.Advisories {
			v.Advisories = append(v.Advisories, Advisory{ID: a.ID, Severity: a.Severity, Summary: a.Summary, FixedIn: append([]string{}, a.FixedIn...)})
		}
		out = append(out, v)
	}
	return out
}

// Result is the verdict for one release: the statuses, the receipt line,
// and the refusal text when nothing overrides a known high or critical
// advisory ("" when the release may run).
type Result struct {
	Statuses []Status
	Line     string
	Refusal  string
}

// Check asks the registry about the declared servers and applies the
// policy's override (mcpServers.allowVulnerable) but no per-run one, which
// is what the agent page and the install wizard show: what a run would do
// unless the person ticks the override in the run panel. No servers
// declared means an empty Result.
func Check(ctx context.Context, registry string, servers []manifest.MCPServer) Result {
	if len(servers) == 0 {
		return Result{Statuses: []Status{}}
	}
	statuses := mcpinstall.CheckVulnerabilities(ctx, nil, registry, servers)
	res := Result{Statuses: Views(statuses)}
	line, err := Verdict(statuses, registry, false, AllowedByPolicy())
	res.Line = line
	if err != nil {
		res.Refusal = err.Error()
	}
	return res
}

// AllowedByPolicy reads the operator policy's mcpServers.allowVulnerable
// without failing: an unreadable policy is the default one.
func AllowedByPolicy() bool {
	path, err := policy.DefaultPath()
	if err != nil {
		return policy.Default().MCPServers.AllowVulnerable
	}
	pol, _, err := policy.Load(path)
	if err != nil {
		return policy.Default().MCPServers.AllowVulnerable
	}
	return pol.MCPServers.AllowVulnerable
}

// Verdict mirrors the CLI's decision (agentry-cli#59): the receipt line,
// and an error when a declared server carries a known high or critical
// advisory that nothing overrides. Overrides are named in the line.
func Verdict(statuses []mcpinstall.VulnStatus, registry string, allowRequest, allowPolicy bool) (string, error) {
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
