package linkservice

// The state of an application on this computer: what Settings → Your
// computer shows for each of the person's applications, what the run
// setup's card shows for each application a release names, and the first
// half of the pre-start check a bind makes (the second half starts the
// server).

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/blaktron/agentry-desktop/internal/library"
	"github.com/blaktron/agentry-desktop/internal/yourcomputer"
)

// AppState is one of the person's applications and its state on this
// computer.
type AppState struct {
	ID string `json:"id"`
	// Application is its name.
	Application string `json:"application"`
	// From is where its set-up came from (library, library-edited or
	// person), FromWords as the card says it, and Preset the library
	// preset it was set up from.
	From      string `json:"from"`
	FromWords string `json:"fromWords"`
	Preset    string `json:"preset,omitempty"`
	// MCP and Computer are what the person approved: its MCP server, its
	// windows.
	MCP      bool `json:"mcp"`
	Computer bool `json:"computer"`
	// HasServer says it has an MCP server set up, and Windows are the
	// identifiers its windows carry here.
	HasServer bool     `json:"hasServer"`
	Windows   []string `json:"windows"`
	// Package is a preset's pinned server, "pypi:mcp-for-blender@2.2.7".
	Package string `json:"package,omitempty"`
	// Attachment is stdio (Agentry Desktop starts the server) or
	// loopback-http (the application serves it).
	Attachment string `json:"attachment,omitempty"`
	// Launch is the command line Agentry Desktop runs, with its variables;
	// Endpoint the loopback address it connects to.
	Launch   string `json:"launch,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	// Env are the server's variables, each with what it does where the
	// library says.
	Env []EnvState `json:"env"`
	// Launcher is the program the launch starts, and LauncherFound whether
	// this computer has it.
	Launcher      string `json:"launcher,omitempty"`
	LauncherFound bool   `json:"launcherFound"`
	// Port is the application's own loopback listener, and Listening
	// whether it answers now.
	Port      int  `json:"port"`
	Listening bool `json:"listening"`
	// Tools are its tools as the person or the preset classed them.
	Tools []ToolState `json:"tools"`
	// Egress are the hosts a preset's server is known to reach from this
	// computer.
	Egress []string `json:"egress"`
	// Steps are what the person does once, from the preset.
	Steps []string `json:"steps"`
	// Ready says a run can bind its server now; Missing is the step to
	// take when it cannot.
	Ready   bool   `json:"ready"`
	Missing string `json:"missing,omitempty"`
}

// EnvState is one of a server's variables.
type EnvState struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Note  string `json:"note,omitempty"`
}

// ToolState is one of an application's tools.
type ToolState struct {
	Name string `json:"name"`
	// Class is how its calls are answered; Classed says someone gave it
	// that class, rather than it being exec for want of one.
	Class    string `json:"class"`
	Classed  bool   `json:"classed"`
	Withheld bool   `json:"withheld,omitempty"`
	Egress   bool   `json:"egress,omitempty"`
}

// Sentence is the refusal's words for a state that is not ready.
func (st AppState) Sentence() string {
	if st.Missing == "" {
		return st.Application + " is not ready on this computer"
	}
	return st.Missing
}

// States are the person's applications with their state on this computer,
// in the list's order.
func (s *Service) States(ctx context.Context) ([]AppState, error) {
	d, err := s.list()
	if err != nil {
		return nil, err
	}
	out := make([]AppState, 0, len(d.Applications))
	for _, a := range d.Applications {
		out = append(out, s.check(ctx, a))
	}
	return out, nil
}

// State is one application's state, false when the list has no such id.
func (s *Service) State(ctx context.Context, id string) (AppState, bool) {
	d, err := s.list()
	if err != nil {
		return AppState{}, false
	}
	a, ok := d.Lookup(id)
	if !ok {
		return AppState{}, false
	}
	return s.check(ctx, a), true
}

// preset is the library preset an application was set up from, if any.
func (s *Service) preset(a yourcomputer.Application) (library.Preset, bool) {
	if a.From.Preset == "" {
		return library.Preset{}, false
	}
	return s.lib.Lookup(a.From.Preset)
}

// check reads an application's state: its server's launcher and its
// listener.
func (s *Service) check(ctx context.Context, a yourcomputer.Application) AppState {
	st := AppState{
		ID: a.ID, Application: a.Name, From: a.From.Protocol(), FromWords: a.From.Words(), Preset: a.From.Preset,
		MCP: a.Approve.MCP, Computer: a.Approve.Computer, HasServer: a.Server != nil,
		Windows: append([]string{}, a.Windows...), Env: []EnvState{}, Tools: []ToolState{}, Egress: []string{}, Steps: []string{},
	}
	p, fromLibrary := s.preset(a)
	if fromLibrary {
		st.Egress = append(st.Egress, p.Egress...)
		st.Steps = append(st.Steps, p.Steps...)
	}
	for _, t := range a.Tools {
		ts := ToolState{Name: t.Name, Class: string(a.ClassOf(t.Name)), Classed: t.Class != "", Withheld: t.Withheld}
		if fromLibrary {
			if pt, ok := p.Tool(t.Name); ok {
				ts.Egress = pt.Egress
			}
		}
		st.Tools = append(st.Tools, ts)
	}
	if a.Server == nil {
		st.Missing = a.Name + " has no MCP server set up in Agentry Desktop."
		return st
	}
	srv := *a.Server
	st.Package, st.Attachment, st.Endpoint, st.Port = srv.Package, string(srv.Attachment), srv.Endpoint, srv.Listener
	for _, k := range slices.Sorted(maps.Keys(srv.Env)) {
		e := EnvState{Name: k, Value: srv.Env[k]}
		if fromLibrary {
			e.Note = p.EnvNotes[k]
		}
		st.Env = append(st.Env, e)
	}
	if srv.Attachment == library.AttachmentStdio && len(srv.Command) > 0 {
		st.Launch, st.Launcher = srv.Launch(), srv.Command[0]
		_, lerr := s.opt.LookPath(srv.Command[0])
		st.LauncherFound = lerr == nil
	}
	if srv.Listener > 0 {
		st.Listening = s.opt.Listening(ctx, srv.Listener) == nil
	}
	switch {
	case srv.Attachment == library.AttachmentStdio && !st.LauncherFound:
		st.Missing = strings.TrimSpace(fmt.Sprintf("%s's MCP server needs %s, which is not on this computer. %s", a.Name, st.Launcher, stepAt(st.Steps, 0)))
	case srv.Listener > 0 && !st.Listening:
		st.Missing = strings.TrimSpace(fmt.Sprintf("%s does not answer on port %d on this computer: open %s and start its MCP server. %s", a.Name, srv.Listener, a.Name, stepNaming(st.Steps, fmt.Sprint(srv.Listener), len(st.Steps)-1)))
	default:
		st.Ready = true
	}
	return st
}

// stepAt is the step at i, "" when there is none there.
func stepAt(steps []string, i int) string {
	if i >= 0 && i < len(steps) {
		return steps[i]
	}
	return ""
}

// stepNaming is the first step that names what, else the step at
// fallback.
func stepNaming(steps []string, what string, fallback int) string {
	for _, step := range steps {
		if strings.Contains(step, what) {
			return step
		}
	}
	return stepAt(steps, fallback)
}

// Check is an application's state on this computer, for the run setup.
func (s *Service) Check(ctx context.Context, a yourcomputer.Application) AppState {
	return s.check(ctx, a)
}

// ListServerTools starts a server the person describes, or connects to
// it, lists its tools and ends it: Add another application's Test.
func (s *Service) ListServerTools(ctx context.Context, srv yourcomputer.Server) ([]string, error) {
	if err := srv.Validate(); err != nil {
		return nil, err
	}
	a := yourcomputer.Application{Name: "the server", Server: &srv}
	if srv.Attachment == library.AttachmentStdio {
		if _, err := s.opt.LookPath(srv.Command[0]); err != nil {
			return nil, fmt.Errorf("%s is not on this computer", srv.Command[0])
		}
	}
	sess, _, err := s.open(ctx, a)
	if err != nil {
		return nil, fmt.Errorf("it did not start: %w", err)
	}
	defer func() { _ = sess.Close() }()
	listed, err := sess.Tools(ctx)
	if err != nil {
		return nil, fmt.Errorf("it started and did not list its tools: %w", err)
	}
	out := make([]string, 0, len(listed))
	for _, t := range listed {
		out = append(out, t.Name)
	}
	return out, nil
}
