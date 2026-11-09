package schedules

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestAScheduleMadeInUnsafeModeIsMarked(t *testing.T) {
	r := Request{Ref: "acme/triage@1.0.0", OutDir: "/home/p/out", MaxTurns: -1, AllowVulnerableMCP: true, InUnsafeMode: true,
		When: When{Kind: "daily", Time: "09:00"}}
	got, err := r.AddArgs()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "--allow-vulnerable-mcp") || !slices.Contains(got, "--in-unsafe-mode") {
		t.Fatalf("args %q", got)
	}
	r.InUnsafeMode = false
	if got, _ := r.AddArgs(); slices.Contains(got, "--in-unsafe-mode") {
		t.Fatalf("an unmarked request is marked: %q", got)
	}
}

// The mark is the app's to set: the page cannot send it.
func TestThePageCannotSetTheMark(t *testing.T) {
	var r Request
	if err := json.Unmarshal([]byte(`{"ref":"acme/triage@1.0.0","inUnsafeMode":true,"InUnsafeMode":true}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.InUnsafeMode {
		t.Fatal("the page set the Unsafe Mode mark")
	}
}
