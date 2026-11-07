package linkservice

// The state of an application on this computer: what Settings → Your
// computer shows for each entry of the catalogue, what the run setup's card
// shows for each application a release declares, and the first half of the
// pre-start check a bind makes (the second half starts the server).

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/blaktron/agentry/pkg/appserver"
)

// AppState is one application server's state on this computer.
type AppState struct {
	ID          string `json:"id"`
	Application string `json:"application"`
	// Offered says the catalogue offers the entry on this computer's
	// system.
	Offered bool `json:"offered"`
	// Package is the pinned installed server, "pypi:mcp-for-blender@2.2.7".
	Package string `json:"package"`
	Source  string `json:"source,omitempty"`
	// Attachment is stdio (Agentry Desktop starts the server) or
	// loopback-http (the application serves it).
	Attachment string `json:"attachment"`
	// Launch is the command line Agentry Desktop runs, with its variables;
	// Endpoint the loopback address it connects to.
	Launch   string `json:"launch,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	// Launcher is the program the launch starts, and LauncherFound whether
	// this computer has it.
	Launcher      string `json:"launcher,omitempty"`
	LauncherFound bool   `json:"launcherFound"`
	// Port is the application's own loopback listener, and Listening
	// whether it answers now.
	Port      int  `json:"port"`
	Listening bool `json:"listening"`
	// ScreeningVariable is the variable that turns the server's own script
	// screening on ("" for none), and ScreeningOff whether the person
	// turned it off.
	ScreeningVariable string `json:"screeningVariable,omitempty"`
	ScreeningOff      bool   `json:"screeningOff"`
	// Tools are the catalogue's tools with their classes.
	Tools []ToolState `json:"tools"`
	// Egress are the hosts the server is known to reach from this computer.
	Egress []string `json:"egress"`
	// Steps are what the person does once.
	Steps []string `json:"steps"`
	// Ready says a run can bind it now; Missing is the step to take when it
	// cannot.
	Ready   bool   `json:"ready"`
	Missing string `json:"missing,omitempty"`
}

// ToolState is one of an entry's tools.
type ToolState struct {
	Name   string `json:"name"`
	Class  string `json:"class"`
	Egress bool   `json:"egress,omitempty"`
}

// Sentence is the refusal's words for a state that is not ready.
func (st AppState) Sentence(e appserver.Server) string {
	if st.Missing == "" {
		return e.Application + " is not ready on this computer"
	}
	return st.Missing
}

// States are every catalogue entry's state on this computer, in the
// catalogue's order.
func (s *Service) States(ctx context.Context) []AppState {
	all := s.cat.All()
	out := make([]AppState, 0, len(all))
	for _, e := range all {
		out = append(out, s.check(ctx, e))
	}
	return out
}

// State is one entry's state, false when the catalogue has no such id.
func (s *Service) State(ctx context.Context, id string) (AppState, bool) {
	e, ok := s.cat.Lookup(id)
	if !ok {
		return AppState{}, false
	}
	return s.check(ctx, e), true
}

// check reads an entry's state: the platform, the launcher, the
// application's listener.
func (s *Service) check(ctx context.Context, e appserver.Server) AppState {
	st := AppState{
		ID: e.ID, Application: e.Application, Package: e.Package.Ref(), Source: e.Package.Source,
		Attachment: string(e.Attachment), Endpoint: e.Endpoint, Port: e.Listener.Port,
		ScreeningVariable: ScreeningVariable(e.ID), ScreeningOff: s.screeningOff(e.ID),
		Egress: append([]string{}, e.Egress...), Steps: append([]string{}, e.Steps...),
		Offered: slices.Contains(e.Platforms, s.opt.GOOS),
	}
	for _, t := range e.Tools {
		st.Tools = append(st.Tools, ToolState{Name: t.Name, Class: string(t.Class), Egress: t.Egress})
	}
	if !st.Offered {
		st.Missing = fmt.Sprintf("Agentry Desktop offers %s's installed server on %s, not on this computer's system.", e.Application, strings.Join(e.Platforms, ", "))
		return st
	}
	if spec, err := launchSpec(e, st.ScreeningOff); err == nil {
		st.Launch, st.Launcher = spec.Line(), spec.Command[0]
		_, lerr := s.opt.LookPath(spec.Command[0])
		st.LauncherFound = lerr == nil
	}
	if e.Listener.Port > 0 {
		st.Listening = s.opt.Listening(ctx, e.Listener.Port) == nil
	}
	switch {
	case e.Attachment == appserver.AttachmentStdio && !st.LauncherFound:
		st.Missing = fmt.Sprintf("%s's installed server needs %s, which is not on this computer. %s", e.Application, st.Launcher, stepAt(e, 0))
	case e.Listener.Port > 0 && !st.Listening:
		st.Missing = fmt.Sprintf("%s does not answer on port %d on this computer: open %s and start its MCP server. %s", e.Application, e.Listener.Port, e.Application, stepNaming(e, fmt.Sprint(e.Listener.Port), len(e.Steps)-1))
	default:
		st.Ready = true
	}
	return st
}

// stepAt is the entry's step at i, "" when it has none there.
func stepAt(e appserver.Server, i int) string {
	if i >= 0 && i < len(e.Steps) {
		return e.Steps[i]
	}
	return ""
}

// stepNaming is the first of the entry's steps that names what, else the
// step at fallback.
func stepNaming(e appserver.Server, what string, fallback int) string {
	for _, step := range e.Steps {
		if strings.Contains(step, what) {
			return step
		}
	}
	if fallback >= 0 && fallback < len(e.Steps) {
		return e.Steps[fallback]
	}
	return ""
}
