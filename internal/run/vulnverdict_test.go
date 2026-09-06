package run

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
	if line, err := vulnerabilityVerdict([]mcpinstall.VulnStatus{ok}, "https://reg.example", false, false); err != nil || !strings.HasPrefix(line, "checked against https://reg.example: ") {
		t.Fatalf("clean: %q %v", line, err)
	}
	if _, err := vulnerabilityVerdict([]mcpinstall.VulnStatus{ok, bad}, "https://reg.example", false, false); err == nil || !strings.Contains(err.Error(), "fixed in 1.2.6") {
		t.Fatalf("refusal: %v", err)
	}
	if line, err := vulnerabilityVerdict([]mcpinstall.VulnStatus{bad}, "https://reg.example", true, false); err != nil || !strings.Contains(line, "allowed by the operator") {
		t.Fatalf("request override: %q %v", line, err)
	}
	if line, err := vulnerabilityVerdict([]mcpinstall.VulnStatus{bad}, "https://reg.example", false, true); err != nil || !strings.Contains(line, "policy mcpServers.allowVulnerable") {
		t.Fatalf("policy override: %q %v", line, err)
	}
}
