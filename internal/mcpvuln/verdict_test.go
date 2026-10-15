package mcpvuln

import (
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/mcpinstall"
)

func score(f float64) *float64 { return &f }

// A server whose advisory the gate rates high: High at CVSS 8.7. Beside it
// a High the gate rates medium (CVSS 7.1), which runs, and must not be
// named in a refusal.
var (
	high = mcpinstall.VulnStatus{Ref: "npm:@modelcontextprotocol/sdk@1.25.1", Status: "vulnerable", MaxSeverity: "high",
		Advisories: []mcpinstall.VulnAdvisory{
			{ID: "GHSA-hi", Severity: "high", GateSeverity: "high", CVSSScore: score(8.7), FixedIn: []string{"1.25.2"}},
			{ID: "GHSA-lo", Severity: "high", GateSeverity: "medium", CVSSScore: score(7.1), FixedIn: []string{"1.26.0"}},
		}}
	medium = mcpinstall.VulnStatus{Ref: "npm:@modelcontextprotocol/sdk@1.25.2", Status: "vulnerable", MaxSeverity: "high",
		Advisories: []mcpinstall.VulnAdvisory{{ID: "GHSA-lo", Severity: "high", GateSeverity: "medium", CVSSScore: score(7.1), FixedIn: []string{"1.26.0"}}}}
	clean = mcpinstall.VulnStatus{Ref: "pypi:mcp-server-fetch@0.6.2", Status: "safe"}
)

// Switch off: a server the gate rates high is refused, and the refusal
// names the server, the advisory, the gate's rating and Unsafe Mode as the
// way through. The High the gate rates medium is not named.
func TestAServerTheGateRatesHighIsRefusedOutsideUnsafeMode(t *testing.T) {
	for name, way := range map[string]WayThrough{
		"no way":                          {},
		"consent without Unsafe Mode":     {Consented: true},
		"the CLI's policy key":            {PolicyAllows: true},
		"policy key and a page's consent": {PolicyAllows: true, Consented: true},
	} {
		_, err := Verdict([]mcpinstall.VulnStatus{clean, high}, "https://reg.example", way)
		if err == nil {
			t.Fatalf("%s: a server the gate rates high ran outside Unsafe Mode", name)
		}
		msg := err.Error()
		for _, want := range []string{"npm:@modelcontextprotocol/sdk@1.25.1", "GHSA-hi", "the gate rates high", "CVSS 8.7", "Unsafe Mode", "fixed in 1.25.2"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s: refusal %q does not name %q", name, msg, want)
			}
		}
		for _, not := range []string{"GHSA-lo", "1.26.0"} {
			if strings.Contains(msg, not) {
				t.Errorf("%s: refusal %q names %q, an advisory the gate rates medium", name, msg, not)
			}
		}
	}
	// The CLI's policy key is named, as what it is, where it is set.
	_, err := Verdict([]mcpinstall.VulnStatus{high}, "https://reg.example", WayThrough{PolicyAllows: true})
	if !strings.Contains(err.Error(), "mcpServers.allowVulnerable") {
		t.Errorf("a policy that allows vulnerable servers is not named: %v", err)
	}
	_, err = Verdict([]mcpinstall.VulnStatus{high}, "https://reg.example", WayThrough{})
	if strings.Contains(err.Error(), "mcpServers.allowVulnerable") {
		t.Errorf("a refusal offers the CLI's policy key as this app's way through: %v", err)
	}
}

// Switch on: the server still needs the person's consent for the run; with
// it, the run goes ahead and the receipt line names Unsafe Mode and the
// server that was allowed.
func TestUnsafeModeRunsItOnlyWithTheConsent(t *testing.T) {
	_, err := Verdict([]mcpinstall.VulnStatus{high}, "https://reg.example", WayThrough{Unsafe: true})
	if err == nil || !strings.Contains(err.Error(), "consent") {
		t.Fatalf("Unsafe Mode without the consent: err = %v, want a refusal that asks for it", err)
	}
	line, err := Verdict([]mcpinstall.VulnStatus{clean, high}, "https://reg.example", WayThrough{Unsafe: true, Consented: true})
	if err != nil {
		t.Fatalf("Unsafe Mode with the consent: %v", err)
	}
	for _, want := range []string{"checked against https://reg.example: ", "Unsafe Mode", "consent", "npm:@modelcontextprotocol/sdk@1.25.1", "GHSA-hi"} {
		if !strings.Contains(line, want) {
			t.Errorf("receipt line %q does not name %q", line, want)
		}
	}
	if strings.Contains(line, "allowed by policy") {
		t.Errorf("receipt line credits the policy: %q", line)
	}
}

// Medium and low run with no consent and no switch, and say nothing about
// either.
func TestAServerTheGateRatesMediumRunsWithNoWayThrough(t *testing.T) {
	for _, s := range []mcpinstall.VulnStatus{clean, medium} {
		line, err := Verdict([]mcpinstall.VulnStatus{s}, "https://reg.example", WayThrough{})
		if err != nil || !strings.HasPrefix(line, "checked against https://reg.example: ") {
			t.Fatalf("%s: %q %v", s.Ref, line, err)
		}
		if strings.Contains(line, "Unsafe Mode") || strings.Contains(line, "allowed") {
			t.Errorf("%s ran with a way through it did not need: %q", s.Ref, line)
		}
	}
}

// A registry that serves no gate rating is weighed by the advisory's own
// word: a High refuses.
func TestAnUnratedHighStillRefuses(t *testing.T) {
	old := mcpinstall.VulnStatus{Ref: "npm:minimist@1.2.5", Status: "vulnerable", MaxSeverity: "high",
		Advisories: []mcpinstall.VulnAdvisory{{ID: "GHSA-xvch-5gv4-984h", Severity: "high", FixedIn: []string{"1.2.6"}}}}
	if _, err := Verdict([]mcpinstall.VulnStatus{old}, "https://reg.example", WayThrough{}); err == nil || !strings.Contains(err.Error(), "a known high vulnerability") {
		t.Fatalf("unrated high: %v", err)
	}
}

// The consent row Unsafe Mode offers names the refusing servers without
// the switch-off remedy beside them.
func TestRefusingNamesTheServersWithoutTheWayThrough(t *testing.T) {
	got := refusingClause([]mcpinstall.VulnStatus{clean, medium, high})
	if !strings.HasPrefix(got, "npm:@modelcontextprotocol/sdk@1.25.1 has an advisory the gate rates high") || strings.Contains(got, "Unsafe Mode") || strings.Contains(got, "@1.25.2 has") {
		t.Fatalf("refusing = %q", got)
	}
}

func TestViewsRenderStatuses(t *testing.T) {
	unreachable := mcpinstall.VulnStatus{Ref: "pypi:x@1.0.0", Status: "unreachable", Err: "dial tcp: refused"}
	got := Views([]mcpinstall.VulnStatus{high, medium, unreachable})
	if len(got) != 3 || !got[0].Refuses || got[1].Refuses || got[2].Refuses || got[2].Error == "" {
		t.Fatalf("views = %+v", got)
	}
	// Each advisory carries the gate's rating beside its own word, and the
	// server the worst rating, so a High the gate rates medium never reads
	// as "high" on a server that runs.
	if got[0].Rating != "high" || got[1].Rating != "medium" || got[1].MaxSeverity != "high" {
		t.Fatalf("server ratings = %q, %q (own %q)", got[0].Rating, got[1].Rating, got[1].MaxSeverity)
	}
	lo := got[0].Advisories[1]
	if lo.Severity != "high" || lo.Rating != "medium" || lo.Refuses || lo.CVSSScore == nil || *lo.CVSSScore != 7.1 || lo.FixedIn[0] != "1.26.0" {
		t.Fatalf("advisory view = %+v", lo)
	}
	if !got[0].Advisories[0].Refuses {
		t.Fatal("the advisory the gate rates high does not say it refuses")
	}
	if got[2].Advisories == nil {
		t.Fatal("advisories must be an array for the frontend, not null")
	}
	if len(Views(nil)) != 0 {
		t.Fatal("nil statuses render as an empty list")
	}
}
