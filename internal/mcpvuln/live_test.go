package mcpvuln

import (
	"context"
	"os"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

// TestLiveCheck asks a real registry about a server version with a known
// critical advisory (minimist 1.2.5, GHSA-xvch-5gv4-984h) and about its
// fixed release, and prints what the agent page and the run panel would
// show. Skipped unless AGENTRY_LIVE_REGISTRY is set.
func TestLiveCheck(t *testing.T) {
	reg := os.Getenv("AGENTRY_LIVE_REGISTRY")
	if reg == "" {
		t.Skip("set AGENTRY_LIVE_REGISTRY to check a live registry's verdict")
	}
	res := Check(context.Background(), reg, []manifest.MCPServer{
		{NPM: "minimist", Version: "1.2.5"},
		{NPM: "minimist", Version: "1.2.6"},
	})
	if len(res.Statuses) != 2 {
		t.Fatalf("statuses = %+v", res.Statuses)
	}
	for _, s := range res.Statuses {
		t.Logf("%s: %s %s refuses=%v advisories=%d err=%q", s.Ref, s.Status, s.MaxSeverity, s.Refuses, len(s.Advisories), s.Error)
	}
	t.Logf("line: %s", res.Line)
	t.Logf("refusal: %s", res.Refusal)
	if res.Statuses[0].Status == "unreachable" {
		t.Fatalf("registry unreachable: %s", res.Statuses[0].Error)
	}
	if Check(context.Background(), reg, nil).Refusal != "" {
		t.Fatal("no declared servers must not refuse")
	}
}
