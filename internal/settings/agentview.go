package settings

import (
	"fmt"
	"slices"
)

// The views a persistent agent's page offers, in its View menu's order.
// Terminal is the page's own and the default; frontend/src/agentView.ts
// keeps the same list.
const AgentViewTerminal = "terminal"

var AgentViews = []string{AgentViewTerminal, "compact", "chat", "focus", "workspace"}

// AgentViewNow is the stored view, Terminal when none is stored or the
// stored one is not a view this app has (a newer app's).
func AgentViewNow() (string, error) {
	s, err := Load()
	if err != nil {
		return "", err
	}
	if s.AgentView == "" || !slices.Contains(AgentViews, s.AgentView) {
		return AgentViewTerminal, nil
	}
	return s.AgentView, nil
}

// SetAgentView stores one of AgentViews; Terminal is stored as none.
func SetAgentView(name string) error {
	if !slices.Contains(AgentViews, name) {
		return fmt.Errorf("%q is not a view Agentry Desktop has", name)
	}
	s, err := Load()
	if err != nil {
		return err
	}
	if name == AgentViewTerminal {
		name = ""
	}
	if s.AgentView == name {
		return nil
	}
	s.AgentView = name
	return Save(s)
}
