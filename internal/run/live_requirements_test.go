package run

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestLiveRequirements resolves a published release on a real registry and
// prints the tool surface and the vulnerability verdict the agent page
// shows. Skipped unless AGENTRY_LIVE_REGISTRY and AGENTRY_LIVE_REF are set.
func TestLiveRequirements(t *testing.T) {
	reg, ref := os.Getenv("AGENTRY_LIVE_REGISTRY"), os.Getenv("AGENTRY_LIVE_REF")
	if reg == "" || ref == "" {
		t.Skip("set AGENTRY_LIVE_REGISTRY and AGENTRY_LIVE_REF (ns/name)")
	}
	ns, name, _ := strings.Cut(ref, "/")
	info, err := Requirements(context.Background(), reg, ns, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %d tool row(s), %d declared server status(es), line=%q refusal=%q",
		info.Ref, len(info.ToolRows), len(info.Vulnerabilities), info.VulnerabilityLine, info.Refusal)
	if info.Vulnerabilities == nil {
		t.Fatal("statuses must be a list for the frontend")
	}
}
