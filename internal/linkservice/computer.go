package linkservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/blaktron/agentry/pkg/appserver"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
)

// Computer use. A run whose release declares computer use asks for it at
// bind, in the release's scope. The link service binds it only when the
// person has turned computer use on in Plugins → Computer use, opens the
// window system's capture and input for the bound applications' windows
// (asking the person through the system's own dialogs where it asks), and
// answers each computer-use call through package computeruse, refused like
// every other call while the run is paused or stopped.

// ComputerApp is an application whose windows a run's computer use may
// see and use: its name, and the window classes its windows carry.
type ComputerApp struct {
	Name    string
	Classes []string
}

// ComputerRequest is what a backend is opened for.
type ComputerRequest struct {
	Scope string
	Apps  []ComputerApp
}

// ComputerOptions are the link service's computer use.
type ComputerOptions struct {
	// Enabled says the person turned computer use on; nil is off.
	Enabled func() bool
	// Keep says the person keeps what the agent saw; nil is off.
	Keep func() bool
	// Open opens the window system's capture and input; nil is this
	// platform's.
	Open func(ctx context.Context, req ComputerRequest) (computeruse.Backend, error)
	// StopKey names the key that stops computer use, as the person is told.
	StopKey string
	// AskScreen asks the person, in a native dialog, whether the run named
	// may see and use the whole screen; nil refuses screen scope.
	AskScreen func(ctx context.Context, who string) error
}

// computerWindows name each catalogue application's windows on every
// system, matched without regard to case against what the system gives: on
// Linux its X11 WM_CLASS and its Wayland app id, on macOS its bundle
// identifier and its process name, on Windows its executable's file name.
var computerWindows = map[string][]string{
	"blender": {"blender", "Blender", "org.blender.Blender", "org.blenderfoundation.blender", "blender.exe"},
	"unreal":  {"UnrealEditor", "com.epicgames.UnrealEditor", "UnrealEditor.exe"},
}

// computerBinding is a run's bound computer use.
type computerBinding struct {
	scope   string
	apps    []string
	tools   map[string]string
	backend computeruse.Backend
	agent   *computeruse.Agent
	once    sync.Once
}

func (c *computerBinding) close() {
	if c == nil {
		return
	}
	c.once.Do(func() { _ = c.backend.Close() })
}

// bindComputer opens a run's computer use, or refuses it with the step.
func (s *Service) bindComputer(ctx context.Context, req desktop.BindRequest, b *binding) (*desktop.ComputerBound, *desktop.Refusal) {
	server := spec.DesktopComputerID
	if req.ComputerUse != computeruse.ScopeApp && req.ComputerUse != computeruse.ScopeScreen {
		return nil, refusal(receipt.DesktopNotGranted, server, "computer use is in app or screen scope, not %q", req.ComputerUse)
	}
	if s.opt.Computer.Enabled == nil || !s.opt.Computer.Enabled() {
		return nil, refusal(receipt.DesktopNotGranted, server, "computer use is off on this computer: turn it on in Agentry Desktop's Settings, Your computer, and run again")
	}
	if req.ComputerUse == computeruse.ScopeScreen {
		// Whatever allowed the whole screen where the run was compiled,
		// the person says so here too, for this run.
		who := req.Agent
		if who == "" {
			who = req.Ref
		}
		if s.opt.Computer.AskScreen == nil {
			return nil, refusal(receipt.DesktopNotGranted, server, "no one could be asked to let this run see your whole screen, so it may not")
		}
		if err := s.opt.Computer.AskScreen(ctx, who); err != nil {
			return nil, refusal(receipt.DesktopDeclined, server, "you did not let %s see and use your whole screen", who)
		}
	}
	tools := map[string]string{}
	for _, name := range req.ComputerTools {
		class, ok := desktoplink.ComputerToolClass(name)
		if !ok {
			return nil, refusal(receipt.DesktopNotGranted, server, "%q is not a computer-use tool this Agentry Desktop answers: update Agentry Desktop", name)
		}
		tools[name] = class
	}
	if len(tools) == 0 {
		return nil, refusal(receipt.DesktopNotGranted, server, "the run asked for computer use with no tool")
	}
	open := s.opt.Computer.Open
	if open == nil {
		open = openComputer
	}
	creq := ComputerRequest{Scope: req.ComputerUse}
	var names []string
	for _, id := range b.order {
		e := b.servers[id].entry
		creq.Apps = append(creq.Apps, ComputerApp{Name: e.Application, Classes: computerWindows[e.ID]})
		names = append(names, e.Application)
	}
	backend, err := open(ctx, creq)
	if err != nil {
		reason := receipt.DesktopNotGranted
		return nil, refusal(reason, server, "Agentry Desktop cannot do computer use here: %v", trimNotGranted(err))
	}
	opt := computeruse.Options{Scope: req.ComputerUse, Gate: func() error {
		if r := b.computerState(); r != nil {
			return &computeruse.Refusal{Reason: r.Reason, Message: r.Message}
		}
		return nil
	}}
	keep := s.opt.Computer.Keep != nil && s.opt.Computer.Keep() && req.Reports != ""
	if keep {
		opt.Keep = req.Reports
	}
	b.computer = &computerBinding{scope: req.ComputerUse, apps: names, tools: tools, backend: backend, agent: computeruse.New(backend, opt)}
	windows := names
	if req.ComputerUse == computeruse.ScopeScreen {
		windows = []string{"the whole display"}
	} else if len(windows) == 0 {
		windows = []string{"the window picked when the run started"}
	}
	return &desktop.ComputerBound{
		Scope:           req.ComputerUse,
		Windows:         windows,
		MaxEdge:         b.computer.agent.MaxEdge(),
		Enforced:        backend.Enforced(),
		Session:         backend.Session(),
		KeepScreenshots: keep,
		StopKey:         stopKeyWords(s.opt.Computer.StopKey),
	}, nil
}

// stopKeyWords is the stop key as the receipt says it: the desktop may not
// let Agentry Desktop bind it.
func stopKeyWords(key string) string {
	if key == "" {
		return ""
	}
	return key + ", where the desktop lets Agentry Desktop bind it (the tray and the banner stop as well)"
}

// trimNotGranted drops the sentinel's own words from a backend's refusal.
func trimNotGranted(err error) string {
	msg := err.Error()
	prefix := computeruse.ErrNotGranted.Error() + ": "
	if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
		return msg[len(prefix):]
	}
	return msg
}

// computerState is the refusal a paused or stopped run's computer use
// gets, nil while it is active.
func (b *binding) computerState() *desktop.Refusal {
	b.mu.Lock()
	state := b.state
	b.mu.Unlock()
	switch state {
	case StatePaused:
		return refusal(receipt.DesktopPaused, spec.DesktopComputerID, "the person paused this run's use of their computer")
	case StateStopped:
		return refusal(receipt.DesktopStopped, spec.DesktopComputerID, "the person stopped this run's use of their computer")
	}
	return nil
}

// callComputer answers one computer-use call.
func (s *Service) callComputer(ctx context.Context, b *binding, req desktop.CallRequest) (desktoplink.Answer, *desktop.Refusal) {
	server := spec.DesktopComputerID
	c := b.computer
	if c == nil {
		return desktoplink.Answer{}, refusal(receipt.DesktopNotGranted, server, "the run did not bind computer use")
	}
	class, ok := c.tools[req.Tool]
	if !ok {
		return desktoplink.Answer{}, refusal(receipt.DesktopNotGranted, server, "the run was not given the computer-use tool %q", req.Tool)
	}
	cls := appserver.Class(class)
	if r := b.computerState(); r != nil {
		b.count(server, cls, r.Reason)
		return desktoplink.Answer{}, r
	}
	select {
	case b.slots <- struct{}{}:
	case <-ctx.Done():
		return desktoplink.Answer{}, refusal(receipt.DesktopUnavailable, server, "the call was ended before it ran")
	case <-b.ctx.Done():
		r := b.computerState()
		if r == nil {
			r = refusal(receipt.DesktopStopped, server, "the run's use of your computer ended")
		}
		b.count(server, cls, r.Reason)
		return desktoplink.Answer{}, r
	}
	defer func() { <-b.slots }()
	// One computer-use action at a time: two at once could interleave
	// their inputs.
	b.computerMu.Lock()
	defer b.computerMu.Unlock()
	if r := b.computerState(); r != nil {
		b.count(server, cls, r.Reason)
		return desktoplink.Answer{}, r
	}
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(b.ctx, cancel)
	defer stop()
	a, err := c.agent.Call(callCtx, req.Tool, req.Arguments)
	if r := b.computerState(); r != nil && b.ctx.Err() != nil {
		b.count(server, cls, r.Reason)
		return desktoplink.Answer{}, r
	}
	var cr *computeruse.Refusal
	switch {
	case errors.As(err, &cr):
		b.count(server, cls, cr.Reason)
		return desktoplink.Answer{}, refusal(cr.Reason, server, "%s", cr.Message)
	case errors.Is(err, computeruse.ErrNotGranted):
		b.count(server, cls, receipt.DesktopNotGranted)
		return desktoplink.Answer{}, refusal(receipt.DesktopNotGranted, server, "%s", trimNotGranted(err))
	case err != nil:
		b.count(server, cls, "error")
		return desktoplink.TextAnswer(fmt.Sprintf("the action was not done: %v", err), true), nil
	}
	b.count(server, cls, "")
	return fitAnswer(a), nil
}

// Tokens keeps the desktop portal's restore tokens in a file of their own,
// so the person is asked once rather than at every run.
type Tokens struct {
	path string
	mu   sync.Mutex
}

// NewTokens keeps the tokens at path.
func NewTokens(path string) *Tokens { return &Tokens{path: path} }

func (t *Tokens) read() map[string]string {
	m := map[string]string{}
	if t == nil || t.path == "" {
		return m
	}
	b, err := os.ReadFile(t.path)
	if err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

// Get is a token, or "".
func (t *Tokens) Get(key string) string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.read()[key]
}

// Set keeps a token.
func (t *Tokens) Set(key, token string) {
	if t == nil || t.path == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	m := t.read()
	m[key] = token
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(t.path), 0o700)
	tmp := t.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, t.path)
	}
}

// Forget drops every token: the person is asked again at the next run.
func (t *Tokens) Forget() {
	if t == nil || t.path == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_ = os.Remove(t.path)
}

// computerApps are the applications a binding's computer use is held to.
func (b *binding) computerApps() []string {
	if b.computer == nil {
		return nil
	}
	return slices.Clone(b.computer.apps)
}

// Support is what computer use can do on this session.
type Support struct {
	// Session names the display: "X11", "Wayland (KDE)".
	Session string `json:"session"`
	// Ready says computer use can be bound here.
	Ready bool `json:"ready"`
	// Problem says why not, and what to do.
	Problem string `json:"problem,omitempty"`
	// Holds says what the window system holds of app scope here.
	Holds string `json:"holds,omitempty"`
	// Permissions are the system's privacy permissions computer use needs
	// here, each granted or not; none where the system asks for none
	// ahead of a run.
	Permissions []Permission `json:"permissions,omitempty"`
}

// Permission is one privacy permission of the system's.
type Permission struct {
	// Key names it to OpenComputerPrivacy: PermissionScreen or
	// PermissionAccessibility.
	Key string `json:"key"`
	// Name is the system's own name for it, as its settings show it.
	Name string `json:"name"`
	// Granted says the system has granted it to Agentry Desktop.
	Granted bool `json:"granted"`
	// Why says what computer use needs it for.
	Why string `json:"why"`
}

// The permissions macOS asks for.
const (
	PermissionScreen        = "screen"
	PermissionAccessibility = "accessibility"
)
