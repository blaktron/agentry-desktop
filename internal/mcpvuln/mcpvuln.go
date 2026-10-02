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
// Severity is the advisory's own word; Rating is what the registry's gate
// rates it, which is what a run weighs (agentry-cli#501, DR-60): a High
// below CVSS 8.5 is rated medium and runs.
type Advisory struct {
	ID        string   `json:"id"`
	Severity  string   `json:"severity"`
	Rating    string   `json:"rating"`
	CVSSScore *float64 `json:"cvssScore,omitempty"`
	// Refuses says this advisory alone refuses a run: rated high or
	// critical.
	Refuses bool     `json:"refuses"`
	Summary string   `json:"summary"`
	FixedIn []string `json:"fixedIn"`
}

// Status is the registry's answer for one declared server, as the
// frontend shows it.
type Status struct {
	// Ref is "npm:pkg@1.2.3" / "pypi:pkg@1.2.3".
	Ref string `json:"ref"`
	// Status is safe, vulnerable, error, unknown or unreachable (see
	// mcpinstall.VulnStatus).
	Status string `json:"status"`
	// MaxSeverity is the worst of the advisories' own words; Rating the
	// worst the gate rates them, the one to show beside the server.
	MaxSeverity string     `json:"maxSeverity,omitempty"`
	Rating      string     `json:"rating,omitempty"`
	Advisories  []Advisory `json:"advisories"`
	CheckedAt   string     `json:"checkedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
	// Refuses says this status alone refuses a run outside Unsafe Mode.
	Refuses bool `json:"refuses"`
}

// rank orders the ratings, worst last.
var rank = map[string]int{"info": 1, "low": 2, "medium": 3, "high": 4, "critical": 5}

// Views renders the runner's statuses for the frontend (never nil).
func Views(statuses []mcpinstall.VulnStatus) []Status {
	out := make([]Status, 0, len(statuses))
	for _, s := range statuses {
		v := Status{Ref: s.Ref, Status: s.Status, MaxSeverity: s.MaxSeverity, Error: s.Err, Refuses: s.Refuses(), Advisories: []Advisory{}}
		if !s.CheckedAt.IsZero() {
			v.CheckedAt = s.CheckedAt.Format(time.RFC3339)
		}
		for _, a := range s.Advisories {
			r := a.Rating()
			if rank[r] > rank[v.Rating] {
				v.Rating = r
			}
			v.Advisories = append(v.Advisories, Advisory{ID: a.ID, Severity: a.Severity, Rating: r, CVSSScore: a.CVSSScore,
				Refuses: a.Refuses(), Summary: a.Summary, FixedIn: append([]string{}, a.FixedIn...)})
		}
		out = append(out, v)
	}
	return out
}

// Result is the verdict for one release: the statuses, the receipt line,
// and the refusal a run meets with Unsafe Mode off ("" when the release
// may run).
type Result struct {
	Statuses []Status
	Line     string
	Refusal  string
}

// Check asks the registry about the declared servers and words the
// verdict as a run with Unsafe Mode off would meet it, which is what the
// agent page and the install wizard show; the run panel offers the consent
// when the switch is on. No servers declared means an empty Result.
func Check(ctx context.Context, registry string, servers []manifest.MCPServer) Result {
	if len(servers) == 0 {
		return Result{Statuses: []Status{}}
	}
	statuses := mcpinstall.CheckVulnerabilities(ctx, nil, registry, servers)
	res := Result{Statuses: Views(statuses)}
	line, err := Verdict(statuses, registry, Way{PolicyAllows: AllowedByPolicy()})
	res.Line = line
	if err != nil {
		res.Refusal = err.Error()
	}
	return res
}

// AllowedByPolicy reads the operator policy's mcpServers.allowVulnerable
// without failing: an unreadable policy is the default one. This app only
// names it: it is the CLI's way through, not this app's.
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

// Way is what a run on this computer brings to a declared server the
// registry's gate rates high or critical (agentry-desktop#459, D-055):
// such a server runs only in Unsafe Mode, and there only with the person's
// consent for the run, asked in a native dialog. The CLI keeps its flag
// and the policy key; a run In Agentry has no way through and never builds
// a Way.
type Way struct {
	// Unsafe is Unsafe Mode, as the run read the switch.
	Unsafe bool
	// Consented is the person's yes for this run, which the native dialog
	// asked (D-030).
	Consented bool
	// PolicyAllows is the runner policy's mcpServers.allowVulnerable,
	// which lets the CLI run the server. This app names it in a refusal
	// and does not take it.
	PolicyAllows bool
}

// Verdict is the run's decision on the registry's answer: the receipt
// line, and an error when a declared server carries an advisory the gate
// rates high or critical (the advisory's own word from a registry that
// serves no rating; agentry-cli#501) and the run has no way through. A run
// let through names Unsafe Mode and each server it let through, with the
// advisories that refused it.
func Verdict(statuses []mcpinstall.VulnStatus, registry string, way Way) (string, error) {
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
	if way.Unsafe && way.Consented {
		allowed := make([]string, 0, len(refusing))
		for _, s := range refusing {
			labels := make([]string, 0, len(s.Refusing()))
			for _, a := range s.Refusing() {
				labels = append(labels, a.Label())
			}
			allowed = append(allowed, s.Ref+" ("+strings.Join(labels, ", ")+")")
		}
		return line + "; run allowed in Unsafe Mode with the person's consent for this run: " + strings.Join(allowed, "; "), nil
	}
	parts := make([]string, 0, len(refusing))
	for _, s := range refusing {
		p := s.Ref + " has " + s.Describe(s.Refusing())
		if fixed := s.FixedIn(); len(fixed) > 0 {
			p += ", fixed in " + strings.Join(fixed, ", ")
		}
		parts = append(parts, p)
	}
	how := " — on this computer such a server runs only in Unsafe Mode (Settings → Run defaults), which asks your consent for each run; or run a release that pins a fixed version"
	if way.Unsafe {
		how = " — Unsafe Mode is on, and this run was not given your consent for it: tick \u201cAllow vulnerable MCP servers for this run\u201d and confirm the dialog; or run a release that pins a fixed version"
	}
	if way.PolicyAllows {
		how += " (the runner policy's mcpServers.allowVulnerable lets the CLI run it; this app's way through is Unsafe Mode)"
	}
	return line, fmt.Errorf("declared MCP server refused by the registry's vulnerability scan: %s%s", strings.Join(parts, "; "), how)
}
