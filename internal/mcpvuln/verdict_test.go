package mcpvuln

import (
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/mcpinstall"
)

// The desktop applies the CLI's rule (agentry-desktop#26): refuse a known
// high/critical advisory unless the request or the policy allows it, and
// say in the receipt line what was checked and why the run went ahead.
func TestVulnerabilityVerdict(t *testing.T) {
	bad := mcpinstall.VulnStatus{Ref: "npm:minimist@1.2.5", Status: "vulnerable", MaxSeverity: "critical",
		Advisories: []mcpinstall.VulnAdvisory{{ID: "GHSA-xvch-5gv4-984h", Severity: "critical", FixedIn: []string{"1.2.6"}}}}
	ok := mcpinstall.VulnStatus{Ref: "pypi:mcp-server-fetch@0.6.2", Status: "safe"}
	if line, err := Verdict([]mcpinstall.VulnStatus{ok}, "https://reg.example", false, false); err != nil || !strings.HasPrefix(line, "checked against https://reg.example: ") {
		t.Fatalf("clean: %q %v", line, err)
	}
	if _, err := Verdict([]mcpinstall.VulnStatus{ok, bad}, "https://reg.example", false, false); err == nil || !strings.Contains(err.Error(), "fixed in 1.2.6") {
		t.Fatalf("refusal: %v", err)
	}
	if line, err := Verdict([]mcpinstall.VulnStatus{bad}, "https://reg.example", true, false); err != nil || !strings.Contains(line, "allowed by the operator") {
		t.Fatalf("request override: %q %v", line, err)
	}
	if line, err := Verdict([]mcpinstall.VulnStatus{bad}, "https://reg.example", false, true); err != nil || !strings.Contains(line, "policy mcpServers.allowVulnerable") {
		t.Fatalf("policy override: %q %v", line, err)
	}
}

func TestViewsRenderStatuses(t *testing.T) {
	bad := mcpinstall.VulnStatus{Ref: "npm:minimist@1.2.5", Status: "vulnerable", MaxSeverity: "critical",
		Advisories: []mcpinstall.VulnAdvisory{{ID: "GHSA-1", Severity: "critical", Summary: "proto", FixedIn: []string{"1.2.6"}}}}
	unreachable := mcpinstall.VulnStatus{Ref: "pypi:x@1.0.0", Status: "unreachable", Err: "dial tcp: refused"}
	got := Views([]mcpinstall.VulnStatus{bad, unreachable})
	if len(got) != 2 || !got[0].Refuses || got[0].Advisories[0].FixedIn[0] != "1.2.6" || got[1].Refuses || got[1].Error == "" {
		t.Fatalf("views = %+v", got)
	}
	if got[1].Advisories == nil {
		t.Fatal("advisories must be an array for the frontend, not null")
	}
	if len(Views(nil)) != 0 {
		t.Fatal("nil statuses render as an empty list")
	}
}
