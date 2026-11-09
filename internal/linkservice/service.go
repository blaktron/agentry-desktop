// Package linkservice is Agentry Desktop's link service: the far end of the
// desktop link, which lets a run use applications on the person's own
// computer through the MCP servers installed there for them.
//
// A run binds before its sandbox starts, naming its release and the
// applications it uses by id. Each id resolves to an application on the
// person's list in Settings → Your computer (by its id, or by the
// person's choice for that QuickAgent), which must be approved for what the
// run uses of it. For each server, the link service checks the person's
// computer is ready (the launcher present, the application's own loopback
// listener answering), starts the server the person's list holds for it as
// the person, and speaks MCP to it. The run gets a token and each server's
// tools, less those the person withheld; every call carries the token. A
// call is answered only for a run the service bound, only for a server the
// run bound, only for a tool the run was given, and then by the class the
// person or the library gave it: a read or an act runs, an exec (code or
// commands the application runs, and any tool nobody has classed) is asked
// about first in a native dialog, which can allow every exec call for the
// rest of the run. Pause and Stop refuse
// every later call; Stop also ends the calls in flight and the run's
// servers. At unbind the servers end.
//
// The package has no window of its own: the native dialog is the Asker the
// app gives it.
package linkservice

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"

	"github.com/blaktron/agentry-desktop/internal/library"
	"github.com/blaktron/agentry-desktop/internal/yourcomputer"
)

// MaxCallsInFlight bounds one run's calls at once.
const MaxCallsInFlight = 4

// The person's answers to an exec call's dialog.
type ExecAnswer int

const (
	// ExecDecline refuses the call.
	ExecDecline ExecAnswer = iota
	// ExecAllowOnce runs this call.
	ExecAllowOnce
	// ExecAllowRun runs this call and every later exec call of the run
	// without asking.
	ExecAllowRun
)

// ExecQuestion is what an exec call's dialog says: who asks, in which
// application, through which tool, and what would run.
type ExecQuestion struct {
	// Who is the QuickAgent (its ref) or the persistent agent asking.
	Who         string
	Application string
	Tool        string
	// What is the code or command the call would run, as the agent sent
	// it, cut at a bound.
	What string
}

// Asker puts an exec call to the person. The app's is a native dialog the
// page cannot answer; with none, an exec call is declined.
type Asker func(ctx context.Context, q ExecQuestion) (ExecAnswer, error)

// Applications is the person's list of applications.
type Applications interface {
	Load() (yourcomputer.Doc, error)
	Update(func(*yourcomputer.Doc) error) (yourcomputer.Doc, error)
}

// Candidate is one of the person's applications a choice may name.
type Candidate struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ChooseQuestion asks the person which of their applications a release's
// entry is: an application they choose, or an id their list does not have.
type ChooseQuestion struct {
	// Who is the QuickAgent (its ref) or the persistent agent asking.
	Who string
	// Entry is the id the release names, "" for an application the person
	// chooses.
	Entry string
	// Use is what the run uses of it: its MCP server or its windows.
	Use string
	// Candidates are the person's applications approved for that use.
	Candidates []Candidate
}

// Chooser puts a choice to the person and answers the chosen
// application's id.
type Chooser func(ctx context.Context, q ChooseQuestion) (string, error)

// The uses a choice is made for.
const (
	UseMCP      = "its MCP server"
	UseComputer = "its windows"
)

// Options are what the link service is built with.
type Options struct {
	// Applications is the person's list; nil is an empty one.
	Applications Applications
	// Library is the library of presets, read for a preset's steps; nil
	// is the one in use, library.Current.
	Library *library.Library
	// Choose asks the person which of their applications an entry is; nil
	// asks no one, and such a bind is refused with the step to take.
	Choose Chooser
	// Ask is the exec call's dialog.
	Ask Asker
	// Launch starts a stdio entry's server; nil is the process launcher.
	Launch Launcher
	// DialHTTP opens a session with a loopback HTTP entry; nil is the
	// Streamable HTTP client.
	DialHTTP func(ctx context.Context, endpoint string) (Session, error)
	// LookPath finds a launcher on the person's PATH; nil is exec.LookPath.
	LookPath func(string) (string, error)
	// Listening says the application's loopback listener answers; nil
	// dials 127.0.0.1 on the port.
	Listening func(ctx context.Context, port int) error
	// Recent keeps the runs that used the person's computer; nil keeps
	// none.
	Recent *Recent
	// GOOS is the platform the library's platforms are matched against;
	// "" is this one.
	GOOS string
	// Changed, when set, is told a binding started, changed state or
	// ended, for the page to read the state again.
	Changed func()
	// Computer is computer use; its zero value refuses it.
	Computer ComputerOptions
}

// Service is the link service. It implements desktop.Handler.
type Service struct {
	opt  Options
	mu   sync.Mutex
	runs map[string]*binding
	// closed is set as the app quits: no run is bound after it.
	closed bool
}

// New builds a link service.
func New(opt Options) *Service {
	s := &Service{opt: opt, runs: map[string]*binding{}}
	if s.opt.Launch == nil {
		s.opt.Launch = startStdio
	}
	if s.opt.DialHTTP == nil {
		s.opt.DialHTTP = func(ctx context.Context, endpoint string) (Session, error) { return dialHTTP(ctx, endpoint) }
	}
	if s.opt.LookPath == nil {
		s.opt.LookPath = exec.LookPath
	}
	if s.opt.Listening == nil {
		s.opt.Listening = dialLoopback
	}
	if s.opt.GOOS == "" {
		s.opt.GOOS = runtime.GOOS
	}
	return s
}

var _ desktop.Handler = (*Service)(nil)

// The states of a binding.
const (
	StateActive  = "active"
	StatePaused  = "paused"
	StateStopped = "stopped"
)

// binding is one bound run.
type binding struct {
	run, ref, agent string
	token           string
	servers         map[string]*boundServer
	order           []string
	// ctx ends with Stop and unbind: every call in flight is cut with it.
	ctx    context.Context
	cancel context.CancelFunc
	slots  chan struct{}
	// askMu holds one exec dialog at a time for the run, so an answer that
	// allows the rest of the run is seen by the calls waiting behind it.
	askMu      sync.Mutex
	mu         sync.Mutex
	state      string
	execForRun bool
	// record is the run's entry in recent, which holds its lock.
	record *RecentRun
	recent *Recent
	// computer is the run's computer use, if it bound it; computerMu
	// holds one computer-use action at a time.
	computer   *computerBinding
	computerMu sync.Mutex
}

// boundServer is one application's server a run bound.
type boundServer struct {
	// key is the entry the run names it by: the application's id, or
	// chosen.
	key     string
	entry   yourcomputer.Application
	allowed map[string]library.Class
	launch  string
	mu      sync.Mutex
	sess    Session
	tools   []desktoplink.Tool
	// lazy is a persistent agent's server: started at its first call, and
	// the application's presence read at each call rather than at bind.
	lazy bool
	// execAgreed says the person agreed that its code tools run without
	// asking each time.
	execAgreed bool
}

// refusal is a desktop.Refusal for a server.
func refusal(reason, server, format string, args ...any) *desktop.Refusal {
	return &desktop.Refusal{Reason: reason, Server: server, Message: fmt.Sprintf(format, args...)}
}

// Bind checks each of the run's application servers on this computer,
// starts each one's installed server and answers with the run's token and
// the tools each listed, narrowed to the ones the release was given; or
// refuses with the step the person must take.
func (s *Service) Bind(ctx context.Context, req desktop.BindRequest) (desktop.Bound, *desktop.Refusal) {
	if req.Run == "" || (len(req.Servers) == 0 && req.ComputerUse == "") {
		return desktop.Bound{}, refusal(receipt.DesktopNotInstalled, "", "a bind names a run and at least one application server or computer use")
	}
	s.mu.Lock()
	_, taken := s.runs[req.Run]
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return desktop.Bound{}, refusal(receipt.DesktopUnavailable, "", "Agentry Desktop is quitting")
	}
	if taken {
		return desktop.Bound{}, refusal(receipt.DesktopNotInstalled, "", "the run %s is already bound to Agentry Desktop", req.Run)
	}
	b := &binding{run: req.Run, ref: req.Ref, agent: req.Agent, servers: map[string]*boundServer{}, slots: make(chan struct{}, MaxCallsInFlight), state: StateActive}
	b.ctx, b.cancel = context.WithCancel(context.Background())
	fail := func(r *desktop.Refusal) (desktop.Bound, *desktop.Refusal) {
		b.closeServers()
		b.cancel()
		return desktop.Bound{}, r
	}
	bound := desktop.Bound{Servers: map[string][]desktoplink.Tool{}}
	for _, want := range req.Servers {
		if _, dup := b.servers[want.ID]; dup {
			continue
		}
		prep := s.prepare
		if req.Lazy {
			prep = s.prepareLazy
		}
		bs, r := prep(ctx, req, want)
		if r != nil {
			return fail(r)
		}
		// Only an agent's segment, bound lazily, carries the person's
		// agreement: a run's code always asks.
		bs.execAgreed = want.ExecAgreed && req.Lazy && req.Agent != ""
		b.servers[want.ID] = bs
		b.order = append(b.order, want.ID)
		bound.Servers[want.ID] = bs.tools
		if req.Protocol >= desktop.Protocol {
			bound.Servers[want.ID] = classed(bs.entry, bs.tools)
			if bound.From == nil {
				bound.From = map[string]string{}
			}
			bound.From[want.ID] = bs.entry.From.Protocol()
			if want.Chosen {
				bound.Chosen = &desktop.ChosenApplication{ID: bs.entry.ID, Name: bs.entry.Name}
			}
		}
	}
	if req.ComputerUse != "" {
		cb, r := s.bindComputer(ctx, req, b)
		if r != nil {
			return fail(r)
		}
		bound.Computer = cb
	}
	tok, err := newToken()
	if err != nil {
		return fail(refusal(receipt.DesktopUnavailable, "", "Agentry Desktop could not make the run's token: %v", err))
	}
	b.token, bound.Token = tok, tok
	b.record, b.recent = s.opt.Recent.Begin(b.summary()), s.opt.Recent
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fail(refusal(receipt.DesktopUnavailable, "", "Agentry Desktop is quitting"))
	}
	if _, taken := s.runs[req.Run]; taken {
		s.mu.Unlock()
		return fail(refusal(receipt.DesktopNotInstalled, "", "the run %s is already bound to Agentry Desktop", req.Run))
	}
	s.runs[req.Run] = b
	s.mu.Unlock()
	s.changed()
	return bound, nil
}

// classed is a bound server's tools with the class the person or the
// library gives each, exec for one nobody classed, for a run that names the
// link protocol's revision: a run of an earlier one decodes the answer
// strictly and is sent none.
func classed(a yourcomputer.Application, tools []desktoplink.Tool) []desktoplink.Tool {
	out := make([]desktoplink.Tool, len(tools))
	for i, t := range tools {
		out[i] = t
		out[i].Class = string(a.ClassOf(t.Name))
	}
	return out
}

// List is the applications a QuickAgent may use on this computer, for the
// roster of a persistent agent and agentry doctor: each on the person's
// list, where its set-up came from, what the person approved (its windows
// only while computer use is on), and whether its server is ready now, with
// the step to take when not.
func (s *Service) List(ctx context.Context) (desktop.ApplicationList, *desktop.Refusal) {
	d, err := s.list()
	if err != nil {
		return desktop.ApplicationList{}, refusal(receipt.DesktopUnavailable, "", "%v", err)
	}
	computer := s.opt.Computer.Enabled != nil && s.opt.Computer.Enabled()
	out := desktop.ApplicationList{Applications: []desktop.Application{}}
	for _, a := range d.Applications {
		st := s.check(ctx, a)
		app := desktop.Application{ID: a.ID, Name: a.Name, From: a.From.Protocol(), MCP: a.UsesMCP(), Computer: computer && a.UsesWindows(), Ready: st.Ready}
		if !st.Ready && a.Server != nil {
			app.Step = st.Sentence()
		}
		if a.Server == nil {
			app.Ready = len(a.Windows) > 0
		}
		out.Applications = append(out.Applications, app)
	}
	return out, nil
}

// list reads the person's list.
func (s *Service) list() (yourcomputer.Doc, error) {
	if s.opt.Applications == nil {
		return yourcomputer.Doc{}, nil
	}
	return s.opt.Applications.Load()
}

// resolve is the person's application a release's entry names, for a use:
// the application with that id on the list, else the person's choice for
// this QuickAgent, remembered or asked now. An id the list does not have
// and nobody chose is refused with the step: a library preset's set-up, or
// adding the application.
func (s *Service) resolve(ctx context.Context, req desktop.BindRequest, id string, chosen bool, use string) (yourcomputer.Application, *desktop.Refusal) {
	d, err := s.list()
	if err != nil {
		return yourcomputer.Application{}, refusal(receipt.DesktopUnavailable, id, "%v", err)
	}
	entry, key := id, id
	if chosen {
		entry, key = "", spec.DesktopChosenID
	}
	if !chosen {
		if a, ok := d.Lookup(id); ok {
			return a, nil
		}
	}
	approved := func(a yourcomputer.Application) bool {
		if use == UseComputer {
			return a.UsesWindows()
		}
		return a.UsesMCP()
	}
	if a, ok := d.Choice(req.Ref, key); ok && approved(a) {
		return a, nil
	}
	var cands []Candidate
	for _, a := range d.Applications {
		if approved(a) {
			cands = append(cands, Candidate{ID: a.ID, Name: a.Name})
		}
	}
	if s.opt.Choose != nil && len(cands) > 0 {
		who := cmp.Or(req.Agent, req.Ref)
		picked, err := s.opt.Choose(ctx, ChooseQuestion{Who: who, Entry: entry, Use: use, Candidates: cands})
		if err == nil {
			for _, c := range cands {
				if c.ID == picked {
					a, _ := d.Lookup(picked)
					if req.Ref != "" && s.opt.Applications != nil {
						_, _ = s.opt.Applications.Update(func(d *yourcomputer.Doc) error {
							d.Choose(req.Ref, key, picked)
							return nil
						})
					}
					return a, nil
				}
			}
		}
		if chosen {
			return yourcomputer.Application{}, refusal(receipt.DesktopDeclined, key, "you did not choose an application for this run")
		}
		return yourcomputer.Application{}, refusal(receipt.DesktopDeclined, key, "you did not choose which of your applications %s is", id)
	}
	if chosen {
		return yourcomputer.Application{}, refusal(receipt.DesktopNotGranted, key,
			"this QuickAgent uses an application you choose, through %s, and none of your applications is approved for that: approve one in Agentry Desktop's Settings → Your computer, and run again", use)
	}
	if p, ok := s.library().Lookup(id); ok && p.OfferedOn(s.opt.GOOS) {
		return yourcomputer.Application{}, refusal(receipt.DesktopNotInstalled, id,
			"this QuickAgent uses %s, which is not set up in Agentry Desktop: add it from the library in Settings → Your computer, approve it, and run again", p.Application)
	}
	return yourcomputer.Application{}, refusal(receipt.DesktopNotInstalled, id,
		"this QuickAgent uses an application called %s, which is not on your list in Agentry Desktop: add it in Settings → Your computer with the id %s, or choose which of yours it is when the run starts", id, id)
}

// resolveServer is the application whose server a release's entry uses,
// with the server set up and approved.
func (s *Service) resolveServer(ctx context.Context, req desktop.BindRequest, want desktop.BindServer) (yourcomputer.Application, *desktop.Refusal) {
	key := want.ID
	if want.Chosen {
		key = spec.DesktopChosenID
	}
	a, r := s.resolve(ctx, req, want.ID, want.Chosen, UseMCP)
	if r != nil {
		return a, r
	}
	switch {
	case a.Server == nil:
		return a, refusal(receipt.DesktopNotInstalled, key, "%s has no MCP server set up in Agentry Desktop: add its server in Settings → Your computer, and run again", a.Name)
	case !a.Approve.MCP:
		return a, refusal(receipt.DesktopNotGranted, key, "agents may not use %s's MCP server: turn on \"Agents may use its MCP server\" for %s in Agentry Desktop's Settings → Your computer, and run again", a.Name, a.Name)
	}
	return a, nil
}

// allowedTools are the tools a bind gives a run of an application: the
// release's narrowing, or every tool listed, less those the person
// withheld, each with its class.
func allowedTools(a yourcomputer.Application, narrowing []string, listed []string) map[string]library.Class {
	out := map[string]library.Class{}
	names := listed
	if len(narrowing) > 0 {
		names = narrowing
	}
	for _, n := range names {
		if !a.Withholds(n) {
			out[n] = a.ClassOf(n)
		}
	}
	return out
}

// knownTools are an application's tools the list knows, less those
// withheld: what a lazy binding stands on before its server starts.
func knownTools(a yourcomputer.Application) []string {
	var out []string
	for _, t := range a.Tools {
		if !t.Withheld {
			out = append(out, t.Name)
		}
	}
	return out
}

// prepareLazy prepares a persistent agent's application server: the
// application on the list, its server approved, and the application need
// not be running. When it is, its installed server is started now and
// lists its tools; when it is not, the narrowing, or the tools the list
// knows, stand for them and the server starts at the first call that finds
// the application running.
func (s *Service) prepareLazy(ctx context.Context, req desktop.BindRequest, want desktop.BindServer) (*boundServer, *desktop.Refusal) {
	a, r := s.resolveServer(ctx, req, want)
	if r != nil {
		return nil, r
	}
	if st := s.check(ctx, a); st.Ready {
		bs, r := s.prepare(ctx, req, want)
		if r == nil {
			bs.lazy = true
			return bs, nil
		}
	}
	bs := &boundServer{key: bindKey(want), entry: a, allowed: allowedTools(a, want.Tools, knownTools(a)), lazy: true}
	for _, name := range slices.Sorted(maps.Keys(bs.allowed)) {
		bs.tools = append(bs.tools, desktoplink.Tool{Name: name, Description: a.Name + " on this computer, through Agentry Desktop",
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`)})
	}
	if len(bs.allowed) == 0 {
		return nil, refusal(receipt.DesktopNotInstalled, bs.key, "%s is not running, and Agentry Desktop knows none of its tools yet: open %s and turn its switch on again", a.Name, a.Name)
	}
	return bs, nil
}

// bindKey is the entry a run names a server by.
func bindKey(want desktop.BindServer) string {
	if want.Chosen {
		return spec.DesktopChosenID
	}
	return want.ID
}

// started readies a lazy server for a call: the application running, and
// its installed server started when it has none; the step to take while
// the application is not running. A server started for a binding that ends
// meanwhile (a Stop) is closed, never kept.
func (bs *boundServer) started(ctx context.Context, s *Service, b *binding) *desktop.Refusal {
	if !bs.lazy {
		return nil
	}
	// Whether the application is running is read before each call, even
	// where the installed server is running already.
	a := bs.entry
	if st := s.check(ctx, a); !st.Ready {
		return refusal(receipt.DesktopNotInstalled, bs.key, "%s", st.Sentence())
	}
	bs.mu.Lock()
	has := bs.sess != nil
	bs.mu.Unlock()
	if has {
		return nil
	}
	octx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(b.ctx, cancel)
	defer stop()
	sess, launch, err := s.open(octx, a)
	if err != nil {
		return refusal(receipt.DesktopNotInstalled, bs.key, "%s's MCP server did not start on this computer: %v", a.Name, err)
	}
	if _, err := sess.Tools(octx); err != nil {
		_ = sess.Close()
		return refusal(receipt.DesktopNotInstalled, bs.key, "%s's MCP server did not list its tools: %v", a.Name, err)
	}
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if bs.sess != nil || b.ctx.Err() != nil {
		_ = sess.Close()
		if b.ctx.Err() != nil {
			return refusal(receipt.DesktopStopped, bs.key, "the use of your computer ended")
		}
		return nil
	}
	bs.sess, bs.launch = sess, launch
	return nil
}

// forget closes a lazy server's session after its application closed: the
// next call that finds the application running starts a fresh one.
func (bs *boundServer) forget() {
	bs.mu.Lock()
	sess := bs.sess
	bs.sess = nil
	bs.mu.Unlock()
	if sess != nil {
		_ = sess.Close()
	}
}

// prepare is the pre-start check of one application's server and its
// start: the application on the person's list and its server approved, the
// launcher present, the application's listener answering, the server
// started; then the run is given the tools it lists, narrowed to the
// release's narrowing, less those the person withheld. A narrowed tool the
// server does not list refuses the bind, naming it.
func (s *Service) prepare(ctx context.Context, req desktop.BindRequest, want desktop.BindServer) (*boundServer, *desktop.Refusal) {
	a, r := s.resolveServer(ctx, req, want)
	if r != nil {
		return nil, r
	}
	key := bindKey(want)
	st := s.check(ctx, a)
	if !st.Ready {
		return nil, refusal(receipt.DesktopNotInstalled, key, "%s", st.Sentence())
	}
	sess, launch, err := s.open(ctx, a)
	if err != nil {
		return nil, refusal(receipt.DesktopNotInstalled, key, "%s's MCP server did not start on this computer: %v", a.Name, err)
	}
	listed, err := sess.Tools(ctx)
	if err != nil {
		_ = sess.Close()
		return nil, refusal(receipt.DesktopNotInstalled, key, "%s's MCP server did not list its tools: %v", a.Name, err)
	}
	names := make([]string, 0, len(listed))
	for _, t := range listed {
		names = append(names, t.Name)
	}
	for _, n := range want.Tools {
		if !slices.Contains(names, n) {
			_ = sess.Close()
			return nil, refusal(receipt.DesktopNotInstalled, key, "this QuickAgent uses the tool %s of %s, which its MCP server on this computer does not list", n, a.Name)
		}
	}
	bs := &boundServer{key: key, entry: a, allowed: allowedTools(a, want.Tools, names)}
	for _, t := range listed {
		if _, ok := bs.allowed[t.Name]; ok {
			bs.tools = append(bs.tools, t)
		}
	}
	if len(bs.tools) == 0 {
		_ = sess.Close()
		return nil, refusal(receipt.DesktopNotInstalled, key, "the run was given no tool of %s: its MCP server listed none it may use", a.Name)
	}
	bs.sess, bs.launch = sess, launch
	return bs, nil
}

// open starts the application's server, or connects to the application's
// own, from the person's list alone.
func (s *Service) open(ctx context.Context, a yourcomputer.Application) (Session, string, error) {
	if a.Server == nil {
		return nil, "", fmt.Errorf("%s has no MCP server set up", a.Name)
	}
	switch a.Server.Attachment {
	case library.AttachmentStdio:
		spec, err := launchSpec(a.Name, *a.Server)
		if err != nil {
			return nil, "", err
		}
		sess, err := s.opt.Launch(ctx, spec)
		return sess, spec.Line(), err
	case library.AttachmentLoopbackHTTP:
		sess, err := s.opt.DialHTTP(ctx, a.Server.Endpoint)
		return sess, a.Server.Endpoint, err
	}
	return nil, "", fmt.Errorf("the attachment %q is not one Agentry Desktop speaks", a.Server.Attachment)
}

// Call answers one tool call: the run bound, its token, the server bound,
// the tool given, the run not paused or stopped, and an exec call asked
// about first.
func (s *Service) Call(ctx context.Context, req desktop.CallRequest) (desktoplink.Answer, *desktop.Refusal) {
	b := s.binding(req.Run, req.Token)
	if b == nil {
		return desktoplink.Answer{}, refusal(receipt.DesktopUnavailable, req.Server, "Agentry Desktop has not bound this run: it was unbound, or Agentry Desktop was restarted")
	}
	if req.Server == spec.DesktopComputerID {
		return s.callComputer(ctx, b, req)
	}
	bs, ok := b.servers[req.Server]
	if !ok {
		return desktoplink.Answer{}, refusal(receipt.DesktopNotInstalled, req.Server, "the run did not bind the application server %q", req.Server)
	}
	class, ok := bs.allowed[req.Tool]
	if !ok {
		return desktoplink.Answer{}, refusal(receipt.DesktopNotInstalled, req.Server, "the run was not given %s's tool %q", bs.entry.Name, req.Tool)
	}
	if r := b.refusedByState(bs); r != nil {
		b.count(req.Server, class, r.Reason)
		return desktoplink.Answer{}, r
	}
	select {
	case b.slots <- struct{}{}:
	case <-ctx.Done():
		return desktoplink.Answer{}, refusal(receipt.DesktopUnavailable, req.Server, "the call was ended before it ran")
	case <-b.ctx.Done():
		r := b.refusedByState(bs)
		if r == nil {
			r = refusal(receipt.DesktopStopped, req.Server, "the run's use of your computer ended")
		}
		b.count(req.Server, class, r.Reason)
		return desktoplink.Answer{}, r
	}
	defer func() { <-b.slots }()
	if r := bs.started(ctx, s, b); r != nil {
		b.count(req.Server, class, r.Reason)
		return desktoplink.Answer{}, r
	}
	if class == library.ClassExec && !bs.execAgreed {
		if r := s.askExec(ctx, b, bs, req); r != nil {
			b.count(req.Server, class, r.Reason)
			return desktoplink.Answer{}, r
		}
		// A pause while the person was asked holds this call too.
		if r := b.refusedByState(bs); r != nil {
			b.count(req.Server, class, r.Reason)
			return desktoplink.Answer{}, r
		}
	}
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(b.ctx, cancel)
	defer stop()
	a, err := bs.call(callCtx, s, req.Tool, req.Arguments)
	if r := b.refusedByState(bs); r != nil && b.ctx.Err() != nil {
		b.count(req.Server, class, r.Reason)
		return desktoplink.Answer{}, r
	}
	if err != nil && bs.lazy {
		// An application closed since the last call is the step to take,
		// not a failure of the agent's.
		if st := s.check(ctx, bs.entry); !st.Ready {
			bs.forget()
			r := refusal(receipt.DesktopNotInstalled, req.Server, "%s", st.Sentence())
			b.count(req.Server, class, r.Reason)
			return desktoplink.Answer{}, r
		}
	}
	if err != nil {
		b.count(req.Server, class, "error")
		return desktoplink.TextAnswer(fmt.Sprintf("%s's installed server did not answer: %v", bs.entry.Name, err), true), nil
	}
	b.count(req.Server, class, "")
	return fitAnswer(a), nil
}

// askExec puts an exec call to the person unless the run's exec calls are
// allowed already: nil when it may run.
func (s *Service) askExec(ctx context.Context, b *binding, bs *boundServer, req desktop.CallRequest) *desktop.Refusal {
	b.askMu.Lock()
	defer b.askMu.Unlock()
	b.mu.Lock()
	allowed := b.execForRun
	b.mu.Unlock()
	if allowed {
		return nil
	}
	if s.opt.Ask == nil {
		return refusal(receipt.DesktopDeclined, req.Server, "no one could be asked to run this code inside %s, so it did not run", bs.entry.Name)
	}
	who := b.ref
	if b.agent != "" {
		who = b.agent
	}
	askCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(b.ctx, cancel)
	defer stop()
	what, ok := whatWouldRun(req.Arguments)
	if !ok {
		return refusal(receipt.DesktopDeclined, req.Server, "the call was too long to show you in full (more than %d characters), so it was not asked about and did not run inside %s", maxWhat, bs.entry.Name)
	}
	ans, err := s.opt.Ask(askCtx, ExecQuestion{Who: who, Application: bs.entry.Name, Tool: req.Tool, What: what})
	if r := b.refusedByState(bs); r != nil {
		return r
	}
	switch {
	case err != nil:
		s.opt.Recent.update(true, func() { b.record.exec(false) })
		return refusal(receipt.DesktopDeclined, req.Server, "the question to run this code inside %s was not answered, so it did not run", bs.entry.Name)
	case ans == ExecAllowRun:
		b.mu.Lock()
		b.execForRun = true
		b.mu.Unlock()
		s.opt.Recent.update(true, b.record.execForRun)
	case ans == ExecAllowOnce:
		s.opt.Recent.update(true, func() { b.record.exec(true) })
	default:
		s.opt.Recent.update(true, func() { b.record.exec(false) })
		s.changed()
		return refusal(receipt.DesktopDeclined, req.Server, "the person declined to run this code inside %s", bs.entry.Name)
	}
	s.changed()
	return nil
}

// maxWhat bounds what an exec dialog quotes: a call longer than the
// dialog shows in full is not asked about.
const maxWhat = 20000

// whatWouldRun is what an exec call's dialog quotes: the code or command
// argument as it is, then every other argument, so nothing of what the call
// carries is hidden from the person; false when it is longer than the
// dialog shows.
func whatWouldRun(args json.RawMessage) (string, bool) {
	var m map[string]json.RawMessage
	if len(args) == 0 || string(args) == "null" {
		return "(no arguments)", true
	}
	if json.Unmarshal(args, &m) != nil {
		return string(args), len(args) <= maxWhat
	}
	var out strings.Builder
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	main := ""
	for _, k := range []string{"code", "script", "command", "python"} {
		var v string
		if raw, ok := m[k]; ok && json.Unmarshal(raw, &v) == nil {
			main = k
			out.WriteString(v)
			break
		}
	}
	for _, k := range keys {
		if k == main {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(k + ": " + string(m[k]))
	}
	return out.String(), out.Len() <= maxWhat
}

// fitAnswer keeps an answer under the line the desktop link carries.
func fitAnswer(a desktoplink.Answer) desktoplink.Answer {
	b, err := json.Marshal(a)
	if err == nil && len(b) < desktoplink.MaxLine-4096 {
		return a
	}
	return desktoplink.TextAnswer(fmt.Sprintf("the installed server's answer was %d bytes, more than the desktop link carries in one answer; ask for less at a time", len(b)), true)
}

// call calls one tool on the server, starting it again once if it ended.
func (bs *boundServer) call(ctx context.Context, s *Service, tool string, args json.RawMessage) (desktoplink.Answer, error) {
	bs.mu.Lock()
	sess := bs.sess
	bs.mu.Unlock()
	if sess == nil {
		return desktoplink.Answer{}, errors.New("the server was ended")
	}
	a, err := sess.Call(ctx, tool, args)
	if err == nil || ctx.Err() != nil || !strings.Contains(err.Error(), "ended") {
		return a, err
	}
	fresh, _, oerr := s.open(ctx, bs.entry)
	if oerr != nil {
		return desktoplink.Answer{}, fmt.Errorf("%v, and it did not start again: %w", err, oerr)
	}
	bs.mu.Lock()
	if bs.sess == nil {
		bs.mu.Unlock()
		_ = fresh.Close()
		return desktoplink.Answer{}, errors.New("the server was ended")
	}
	old := bs.sess
	bs.sess = fresh
	bs.mu.Unlock()
	_ = old.Close()
	return fresh.Call(ctx, tool, args)
}

// Unbind ends a run's binding and its servers.
func (s *Service) Unbind(_ context.Context, req desktop.UnbindRequest) *desktop.Refusal {
	b := s.binding(req.Run, req.Token)
	if b == nil {
		return refusal(receipt.DesktopUnavailable, "", "Agentry Desktop has not bound this run")
	}
	s.mu.Lock()
	if s.runs[req.Run] == b {
		delete(s.runs, req.Run)
	}
	s.mu.Unlock()
	b.cancel()
	b.closeServers()
	b.mu.Lock()
	state := b.state
	b.mu.Unlock()
	s.opt.Recent.update(true, func() { b.record.end(state) })
	s.changed()
	return nil
}

// binding is the run's binding when the token is its own, compared in
// constant time.
func (s *Service) binding(run, token string) *binding {
	s.mu.Lock()
	b := s.runs[run]
	s.mu.Unlock()
	if b == nil || token == "" || subtle.ConstantTimeCompare([]byte(b.token), []byte(token)) != 1 {
		return nil
	}
	return b
}

// Pause refuses the run's later calls until Resume.
func (s *Service) Pause(run string) error { return s.setState(run, StatePaused) }

// Resume lets a paused run's calls through again; a stopped run stays
// stopped.
func (s *Service) Resume(run string) error { return s.setState(run, StateActive) }

// Stop ends the run's calls in flight and its servers, and refuses every
// later call of the run.
func (s *Service) Stop(run string) error { return s.setState(run, StateStopped) }

// StopAll stops every bound run as the app quits, and records each run's
// use of the computer as ended by it: no run can be answered once the app
// is gone.
func (s *Service) StopAll() {
	s.mu.Lock()
	s.closed = true
	runs := make([]*binding, 0, len(s.runs))
	for _, b := range s.runs {
		runs = append(runs, b)
	}
	s.mu.Unlock()
	for _, b := range runs {
		_ = s.Stop(b.run)
		s.opt.Recent.update(true, func() { b.record.end(StateStopped) })
	}
}

func (s *Service) setState(run, state string) error {
	s.mu.Lock()
	b := s.runs[run]
	s.mu.Unlock()
	if b == nil {
		return fmt.Errorf("no run %s is using your computer", run)
	}
	b.mu.Lock()
	switch {
	case b.state == StateStopped:
		b.mu.Unlock()
		if state == StateStopped {
			return nil
		}
		return errors.New("the run's use of your computer was stopped and cannot be resumed")
	case state == b.state:
		b.mu.Unlock()
		return nil
	}
	b.state = state
	b.mu.Unlock()
	s.opt.Recent.update(true, func() { b.record.setState(state) })
	if state == StateStopped {
		b.cancel()
		b.closeServers()
	}
	s.changed()
	return nil
}

// refusedByState is the refusal a paused or stopped run gets, nil while
// it is active.
func (b *binding) refusedByState(bs *boundServer) *desktop.Refusal {
	b.mu.Lock()
	state := b.state
	b.mu.Unlock()
	switch state {
	case StatePaused:
		return refusal(receipt.DesktopPaused, bs.key, "the person paused this run's use of %s on their computer", bs.entry.Name)
	case StateStopped:
		return refusal(receipt.DesktopStopped, bs.key, "the person stopped this run's use of %s on their computer", bs.entry.Name)
	}
	return nil
}

// count records one call's outcome for Settings' recent runs: "" is
// answered, "error" not answered, else a refusal's reason.
func (b *binding) count(server string, class library.Class, outcome string) {
	b.recent.update(false, func() { b.record.call(server, string(class), outcome) })
}

func (b *binding) closeServers() {
	if c := b.computer; c != nil {
		// The action in flight was cut by the binding's context; the
		// session closes once it has let go of every key and button.
		go func() {
			b.computerMu.Lock()
			defer b.computerMu.Unlock()
			c.close()
		}()
	}
	for _, bs := range b.servers {
		bs.mu.Lock()
		sess := bs.sess
		bs.sess = nil
		bs.mu.Unlock()
		if sess != nil {
			_ = sess.Close()
		}
	}
}

// summary is the binding as Settings' recent runs begin it.
func (b *binding) summary() RecentRun {
	r := RecentRun{Run: b.run, Ref: b.ref, Agent: b.agent, State: StateActive}
	for _, id := range b.order {
		bs := b.servers[id]
		r.Servers = append(r.Servers, RecentServer{ID: id, Application: bs.entry.Name, Package: serverPackage(bs.entry), Launch: bs.launch, Era: bs.era(), Calls: map[string]int{}, Refused: map[string]int{}})
	}
	if b.computer != nil {
		r.Servers = append(r.Servers, RecentServer{ID: spec.DesktopComputerID, Application: "Computer use (" + b.computer.scope + " scope)", Calls: map[string]int{}, Refused: map[string]int{}})
	}
	return r
}

func (bs *boundServer) era() string {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if bs.sess == nil {
		return ""
	}
	return bs.sess.Era()
}

// Bound is a run the service has bound, as the page shows it.
type Bound struct {
	Run          string   `json:"run"`
	Ref          string   `json:"ref"`
	Agent        string   `json:"agent,omitempty"`
	Applications []string `json:"applications"`
	State        string   `json:"state"`
	ExecForRun   bool     `json:"execForRun"`
	// ComputerUse is the scope of the run's computer use, empty without
	// it, and ComputerApps the applications it is held to.
	ComputerUse  string   `json:"computerUse,omitempty"`
	ComputerApps []string `json:"computerApps,omitempty"`
}

// Runs are the runs bound now.
func (s *Service) Runs() []Bound {
	s.mu.Lock()
	bs := make([]*binding, 0, len(s.runs))
	for _, b := range s.runs {
		bs = append(bs, b)
	}
	s.mu.Unlock()
	out := make([]Bound, 0, len(bs))
	for _, b := range bs {
		v := Bound{Run: b.run, Ref: b.ref, Agent: b.agent}
		for _, id := range b.order {
			v.Applications = append(v.Applications, b.servers[id].entry.Name)
		}
		if b.computer != nil {
			v.ComputerUse, v.ComputerApps = b.computer.scope, b.computerApps()
		}
		b.mu.Lock()
		v.State, v.ExecForRun = b.state, b.execForRun
		b.mu.Unlock()
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b Bound) int { return strings.Compare(a.Run, b.Run) })
	return out
}

// Recent are the runs that used the computer, the newest first.
func (s *Service) Recent() []RecentRun { return s.opt.Recent.List() }

func (s *Service) changed() {
	if s.opt.Changed != nil {
		s.opt.Changed()
	}
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// DialLoopback says something listens on the loopback port.
func DialLoopback(ctx context.Context, port int) error { return dialLoopback(ctx, port) }

// dialLoopback says something listens on the loopback port.
func dialLoopback(ctx context.Context, port int) error {
	d := net.Dialer{Timeout: 2 * time.Second}
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return c.Close()
}

// serverPackage names a preset's pinned server, "" for one the person set
// up.
func serverPackage(a yourcomputer.Application) string {
	if a.Server == nil {
		return ""
	}
	return a.Server.Package
}
