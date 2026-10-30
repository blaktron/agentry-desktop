package settings

import (
	"strings"
	"testing"
)

// The view is Terminal until one is chosen, any of the offered ones is
// kept, Terminal is stored as none, and a name this app lacks is refused on
// the way in and read as Terminal on the way out (a newer app's view).
func TestAgentViewIsKeptAndTerminalIsTheDefault(t *testing.T) {
	isolate(t)
	if got, err := AgentViewNow(); err != nil || got != AgentViewTerminal {
		t.Fatalf("no view stored reads %q, %v", got, err)
	}
	for _, v := range AgentViews {
		if err := SetAgentView(v); err != nil {
			t.Fatalf("SetAgentView(%q): %v", v, err)
		}
		if got, _ := AgentViewNow(); got != v {
			t.Fatalf("after %q = %q", v, got)
		}
	}
	if err := SetAgentView("compact"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readRaw(t), "agentView: compact") {
		t.Fatalf("not written:\n%s", readRaw(t))
	}
	if err := SetAgentView("cinema"); err == nil {
		t.Fatal("a view this app lacks was accepted")
	}
	if got, _ := AgentViewNow(); got != "compact" {
		t.Fatalf("a refused view changed the stored one to %q", got)
	}
	if err := SetAgentView(AgentViewTerminal); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(readRaw(t), "agentView:") {
		t.Fatalf("Terminal was stored:\n%s", readRaw(t))
	}
	writeRaw(t, "agentView: hologram\n")
	if got, _ := AgentViewNow(); got != AgentViewTerminal {
		t.Fatalf("a newer app's view reads %q, not Terminal", got)
	}
}
