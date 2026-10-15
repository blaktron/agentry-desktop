package run

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// vulnManifestYAML is the fixture release with one declared MCP server.
const vulnManifestYAML = `schemaVersion: "2"
name: reporter
namespace: acme
modes: [autonomous]
version: 1.0.0
summary: "desktop run fixture with a declared server"
entrypoints:
  instruction: SKILL.md
capabilities:
  mcpServers:
    - npm: "@modelcontextprotocol/sdk"
      version: 1.25.1
publish:
  include: [agent.yaml, SKILL.md]
`

// vulnRunFixture serves the release above with the registry's record for
// its server, and a policy whose npm registry refuses at once, so a run
// the verdict lets through stops at resolution without reaching the
// network.
func vulnRunFixture(t *testing.T, scan map[string]any, policyAllows bool) string {
	t.Helper()
	isolateRunEnv(t)
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	pol := policy.Default()
	pol.MCPRegistries.NPM = "https://127.0.0.1:1"
	pol.MCPServers.AllowVulnerable = policyAllows
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	return startRunRegistry(t, &fakeRunRegistry{
		manifestDoc: manifestDocFromYAML(t, vulnManifestYAML),
		archive: buildRunArchive(t, map[string]string{
			"agent.yaml": vulnManifestYAML,
			"SKILL.md":   "# Reporter\n",
		}),
		mcpScan: scan,
	})
}

// highScan is the registry's record for a server whose advisory the gate
// rates high: High at CVSS 8.7, as dev's record for the sdk at 1.25.1.
var highScan = map[string]any{
	"status": "vulnerable", "maxSeverity": "high",
	"advisories": []map[string]any{{"id": "GHSA-hi", "severity": "high", "gateSeverity": "high", "cvssScore": 8.7, "fixedIn": []string{"1.25.2"}}},
}

// The engine holds the rule whatever the page sent: outside Unsafe Mode a
// server the gate rates high is refused, the page's consent and the CLI's
// policy key notwithstanding; in Unsafe Mode it needs the consent; with both,
// the verdict lets the run go on.
func TestTheEngineRunsAHighRatedServerOnlyInUnsafeModeWithTheConsent(t *testing.T) {
	cases := []struct {
		name                       string
		unsafe, consent, policyKey bool
		refusedFor                 string
	}{
		{"switch off", false, false, false, "runs only in Unsafe Mode"},
		{"switch off, consent sent", false, true, false, "runs only in Unsafe Mode"},
		{"switch off, the CLI's policy key", false, false, true, "runs only in Unsafe Mode"},
		{"switch on, no consent", true, false, false, "was not given your consent"},
		{"switch on, consent", true, true, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			regURL := vulnRunFixture(t, highScan, c.policyKey)
			req := request(regURL, filepath.Join(t.TempDir(), "out"))
			req.Unsafe, req.AllowVulnerableMCP = c.unsafe, c.consent
			var log strings.Builder
			_, err := Run(context.Background(), req, &log)
			if err == nil {
				t.Fatalf("the run went on: the fixture's npm registry should have stopped it\nlog:\n%s", log.String())
			}
			refused := strings.Contains(err.Error(), "refused by the registry's vulnerability scan")
			switch {
			case c.refusedFor != "" && (!refused || !strings.Contains(err.Error(), c.refusedFor)):
				t.Fatalf("err = %v, want the vulnerability refusal saying %q", err, c.refusedFor)
			case c.refusedFor == "" && refused:
				t.Fatalf("Unsafe Mode with the consent was refused: %v", err)
			}
		})
	}
}

// A server the gate rates medium runs with neither the switch nor the
// consent.
func TestTheEngineRunsAMediumRatedServerWithNoWayThrough(t *testing.T) {
	regURL := vulnRunFixture(t, map[string]any{
		"status": "vulnerable", "maxSeverity": "high",
		"advisories": []map[string]any{{"id": "GHSA-lo", "severity": "high", "gateSeverity": "medium", "cvssScore": 7.1}},
	}, false)
	var log strings.Builder
	_, err := Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &log)
	if err == nil || strings.Contains(err.Error(), "vulnerability scan") {
		t.Fatalf("err = %v, want the run past the verdict (stopped at resolution)\nlog:\n%s", err, log.String())
	}
	if !strings.Contains(log.String(), "gate-rated medium") {
		t.Fatalf("the run log does not carry the registry's rating:\n%s", log.String())
	}
}
