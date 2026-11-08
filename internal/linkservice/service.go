// Package linkservice is Agentry Desktop's link service: the far end of the
// desktop link, which lets a run use applications on the person's own
// computer through the MCP servers installed there for them.
//
// A run binds before its sandbox starts, naming its release and the
// application servers it uses by their catalogue ids. For each, the link
// service checks the person's computer is ready (the launcher present, the
// application's own loopback listener answering), starts the server the
// app's catalogue pins for that id as the person, speaks MCP to it, and
// checks the tools it lists are the catalogue's. The run gets a token and
// each server's tools; every call carries the token. A call is answered
// only for a run the service bound, only for a server the run bound, only
// for a tool the release was given, and then by its class in the
// catalogue: a read or an act runs, an exec (code or commands the
// application runs) is asked about first in a native dialog, which can
// allow every exec call for the rest of the run. Pause and Stop refuse
// every later call; Stop also ends the calls in flight and the run's
// servers. At unbind the servers end.
//
// The package has no window of its own: the native dialog is the Asker the
// app gives it.
package linkservice

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry/pkg/appserver"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"
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

// Options are what the link service is built with.
type Options struct {
	// Catalogue is the catalogue of application servers the app was built
	// with; zero is the core's built-in one.
	Catalogue *appserver.Catalogue
	// Ask is the exec call's dialog.
	Ask Asker
	// ScreeningOff says the person turned off an application server's own
	// script screening in Plugins → Computer use; nil is never.
	ScreeningOff func(id string) bool
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
	// GOOS is the platform the catalogue's platforms are matched against;
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
	cat  appserver.Catalogue
	mu   sync.Mutex
	runs map[string]*binding
	// closed is set as the app quits: no run is bound after it.
	closed bool
}

// New builds a link service.
func New(opt Options) *Service {
	s := &Service{opt: opt, runs: map[string]*binding{}}
	if opt.Catalogue != nil {
		s.cat = *opt.Catalogue
	} else {
		s.cat = appserver.Builtin()
	}
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

// boundServer is one application server a run bound.
type boundServer struct {
	entry   appserver.Server
	allowed map[string]appserver.Tool
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
		bs, r := prep(ctx, want)
		if r != nil {
			return fail(r)
		}
		// Only an agent's segment, bound lazily, carries the person's
		// agreement: a run's code always asks.
		bs.execAgreed = want.ExecAgreed && req.Lazy && req.Agent != ""
		b.servers[want.ID] = bs
		b.order = append(b.order, want.ID)
		bound.Servers[want.ID] = bs.tools
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

// prepareLazy prepares a persistent agent's application server: the
// catalogue knows it for this platform and gives the agent's tools, and the
// application need not be running. When it is, its installed server is
// started now and lists its tools; when it is not, the catalogue's names
// stand for them and the server starts at the first call that finds the
// application running.
func (s *Service) prepareLazy(ctx context.Context, want desktop.BindServer) (*boundServer, *desktop.Refusal) {
	e, ok := s.cat.Lookup(want.ID)
	if !ok {
		return nil, refusal(receipt.DesktopNotInstalled, want.ID, "this Agentry Desktop does not know the application server %q: update Agentry Desktop", want.ID)
	}
	if st := s.check(ctx, e); st.Ready {
		bs, r := s.prepare(ctx, want)
		if r == nil {
			bs.lazy = true
			return bs, nil
		}
	}
	if !slices.Contains(e.Platforms, s.opt.GOOS) {
		return nil, refusal(receipt.DesktopNotInstalled, want.ID, "Agentry Desktop offers %s's installed server on %s, not on this computer's system", e.Application, strings.Join(e.Platforms, ", "))
	}
	bs := &boundServer{entry: e, allowed: map[string]appserver.Tool{}, lazy: true}
	for _, name := range want.Tools {
		t, ok := e.Tool(name)
		if !ok {
			return nil, refusal(receipt.DesktopNotInstalled, want.ID, "Agentry Desktop's catalogue gives %s no tool %q: update Agentry Desktop", e.Application, name)
		}
		bs.allowed[name] = t
		bs.tools = append(bs.tools, desktoplink.Tool{Name: name, Description: e.Application + " on this computer, through Agentry Desktop",
			InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true}`)})
	}
	if len(bs.allowed) == 0 {
		return nil, refusal(receipt.DesktopNotInstalled, want.ID, "the agent was given no tool of %s", e.Application)
	}
	return bs, nil
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
	e := bs.entry
	if st := s.check(ctx, e); !st.Ready {
		return refusal(receipt.DesktopNotInstalled, e.ID, "%s", st.Sentence(e))
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
	sess, launch, err := s.open(octx, e)
	if err != nil {
		return refusal(receipt.DesktopNotInstalled, e.ID, "%s's installed server (%s) did not start on this computer: %v", e.Application, e.Package.Ref(), err)
	}
	listed, err := sess.Tools(octx)
	if err != nil {
		_ = sess.Close()
		return refusal(receipt.DesktopNotInstalled, e.ID, "%s's installed server did not list its tools: %v", e.Application, err)
	}
	for _, t := range listed {
		if !e.Offers(t.Name) && !slices.Contains(e.Withheld, t.Name) {
			_ = sess.Close()
			return refusal(receipt.DesktopNotInstalled, e.ID, "%s's installed server lists a tool the catalogue does not know (%s): it is not the server Agentry pinned, %s", e.Application, t.Name, e.Package.Ref())
		}
	}
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if bs.sess != nil || b.ctx.Err() != nil {
		_ = sess.Close()
		if b.ctx.Err() != nil {
			return refusal(receipt.DesktopStopped, e.ID, "the use of your computer ended")
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

// prepare is the pre-start check of one application server and its start:
// the catalogue knows it for this platform and gives the release's tools,
// the launcher is present, the application's listener answers, the server
// starts and lists only tools the catalogue knows.
func (s *Service) prepare(ctx context.Context, want desktop.BindServer) (*boundServer, *desktop.Refusal) {
	e, ok := s.cat.Lookup(want.ID)
	if !ok {
		return nil, refusal(receipt.DesktopNotInstalled, want.ID, "this Agentry Desktop does not know the application server %q: update Agentry Desktop", want.ID)
	}
	if !slices.Contains(e.Platforms, s.opt.GOOS) {
		return nil, refusal(receipt.DesktopNotInstalled, want.ID, "Agentry Desktop offers %s's installed server on %s, not on this computer's system", e.Application, strings.Join(e.Platforms, ", "))
	}
	bs := &boundServer{entry: e, allowed: map[string]appserver.Tool{}}
	for _, name := range want.Tools {
		t, ok := e.Tool(name)
		if !ok {
			return nil, refusal(receipt.DesktopNotInstalled, want.ID, "Agentry Desktop's catalogue gives %s no tool %q: update Agentry Desktop", e.Application, name)
		}
		bs.allowed[name] = t
	}
	if len(bs.allowed) == 0 {
		return nil, refusal(receipt.DesktopNotInstalled, want.ID, "the run was given no tool of %s", e.Application)
	}
	st := s.check(ctx, e)
	if !st.Ready {
		return nil, refusal(receipt.DesktopNotInstalled, want.ID, "%s", st.Sentence(e))
	}
	sess, launch, err := s.open(ctx, e)
	if err != nil {
		return nil, refusal(receipt.DesktopNotInstalled, want.ID, "%s's installed server (%s) did not start on this computer: %v", e.Application, e.Package.Ref(), err)
	}
	listed, err := sess.Tools(ctx)
	if err != nil {
		_ = sess.Close()
		return nil, refusal(receipt.DesktopNotInstalled, want.ID, "%s's installed server did not list its tools: %v", e.Application, err)
	}
	for _, t := range listed {
		if !e.Offers(t.Name) && !slices.Contains(e.Withheld, t.Name) {
			_ = sess.Close()
			return nil, refusal(receipt.DesktopNotInstalled, want.ID, "%s's installed server lists a tool the catalogue does not know (%s): it is not the server Agentry pinned, %s", e.Application, t.Name, e.Package.Ref())
		}
		if _, ok := bs.allowed[t.Name]; ok {
			bs.tools = append(bs.tools, t)
		}
	}
	if len(bs.tools) == 0 {
		_ = sess.Close()
		return nil, refusal(receipt.DesktopNotInstalled, want.ID, "%s's installed server listed none of the tools the run was given", e.Application)
	}
	bs.sess, bs.launch = sess, launch
	return bs, nil
}

// open starts the entry's server, or connects to the application's own,
// from the catalogue entry alone.
func (s *Service) open(ctx context.Context, e appserver.Server) (Session, string, error) {
	switch e.Attachment {
	case appserver.AttachmentStdio:
		spec, err := launchSpec(e, s.screeningOff(e.ID))
		if err != nil {
			return nil, "", err
		}
		sess, err := s.opt.Launch(ctx, spec)
		return sess, spec.Line(), err
	case appserver.AttachmentLoopbackHTTP:
		sess, err := s.opt.DialHTTP(ctx, e.Endpoint)
		return sess, e.Endpoint, err
	}
	return nil, "", fmt.Errorf("the catalogue's attachment %q is not one Agentry Desktop speaks", e.Attachment)
}

func (s *Service) screeningOff(id string) bool {
	return s.opt.ScreeningOff != nil && s.opt.ScreeningOff(id)
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
	tool, ok := bs.allowed[req.Tool]
	if !ok {
		return desktoplink.Answer{}, refusal(receipt.DesktopNotInstalled, req.Server, "the run was not given %s's tool %q", bs.entry.Application, req.Tool)
	}
	if r := b.refusedByState(bs.entry); r != nil {
		b.count(req.Server, tool.Class, r.Reason)
		return desktoplink.Answer{}, r
	}
	select {
	case b.slots <- struct{}{}:
	case <-ctx.Done():
		return desktoplink.Answer{}, refusal(receipt.DesktopUnavailable, req.Server, "the call was ended before it ran")
	case <-b.ctx.Done():
		r := b.refusedByState(bs.entry)
		if r == nil {
			r = refusal(receipt.DesktopStopped, req.Server, "the run's use of your computer ended")
		}
		b.count(req.Server, tool.Class, r.Reason)
		return desktoplink.Answer{}, r
	}
	defer func() { <-b.slots }()
	if r := bs.started(ctx, s, b); r != nil {
		b.count(req.Server, tool.Class, r.Reason)
		return desktoplink.Answer{}, r
	}
	if tool.Class == appserver.ClassExec && !bs.execAgreed {
		if r := s.askExec(ctx, b, bs, req); r != nil {
			b.count(req.Server, tool.Class, r.Reason)
			return desktoplink.Answer{}, r
		}
		// A pause while the person was asked holds this call too.
		if r := b.refusedByState(bs.entry); r != nil {
			b.count(req.Server, tool.Class, r.Reason)
			return desktoplink.Answer{}, r
		}
	}
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(b.ctx, cancel)
	defer stop()
	a, err := bs.call(callCtx, s, req.Tool, req.Arguments)
	if r := b.refusedByState(bs.entry); r != nil && b.ctx.Err() != nil {
		b.count(req.Server, tool.Class, r.Reason)
		return desktoplink.Answer{}, r
	}
	if err != nil && bs.lazy {
		// An application closed since the last call is the step to take,
		// not a failure of the agent's.
		if st := s.check(ctx, bs.entry); !st.Ready {
			bs.forget()
			r := refusal(receipt.DesktopNotInstalled, req.Server, "%s", st.Sentence(bs.entry))
			b.count(req.Server, tool.Class, r.Reason)
			return desktoplink.Answer{}, r
		}
	}
	if err != nil {
		b.count(req.Server, tool.Class, "error")
		return desktoplink.TextAnswer(fmt.Sprintf("%s's installed server did not answer: %v", bs.entry.Application, err), true), nil
	}
	b.count(req.Server, tool.Class, "")
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
		return refusal(receipt.DesktopDeclined, req.Server, "no one could be asked to run this code inside %s, so it did not run", bs.entry.Application)
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
		return refusal(receipt.DesktopDeclined, req.Server, "the call was too long to show you in full (more than %d characters), so it was not asked about and did not run inside %s", maxWhat, bs.entry.Application)
	}
	ans, err := s.opt.Ask(askCtx, ExecQuestion{Who: who, Application: bs.entry.Application, Tool: req.Tool, What: what})
	if r := b.refusedByState(bs.entry); r != nil {
		return r
	}
	switch {
	case err != nil:
		s.opt.Recent.update(true, func() { b.record.exec(false) })
		return refusal(receipt.DesktopDeclined, req.Server, "the question to run this code inside %s was not answered, so it did not run", bs.entry.Application)
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
		return refusal(receipt.DesktopDeclined, req.Server, "the person declined to run this code inside %s", bs.entry.Application)
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
func (b *binding) refusedByState(e appserver.Server) *desktop.Refusal {
	b.mu.Lock()
	state := b.state
	b.mu.Unlock()
	switch state {
	case StatePaused:
		return refusal(receipt.DesktopPaused, e.ID, "the person paused this run's use of %s on their computer", e.Application)
	case StateStopped:
		return refusal(receipt.DesktopStopped, e.ID, "the person stopped this run's use of %s on their computer", e.Application)
	}
	return nil
}

// count records one call's outcome for Settings' recent runs: "" is
// answered, "error" not answered, else a refusal's reason.
func (b *binding) count(server string, class appserver.Class, outcome string) {
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
		r.Servers = append(r.Servers, RecentServer{ID: id, Application: bs.entry.Application, Package: bs.entry.Package.Ref(), Launch: bs.launch, Era: bs.era(), Calls: map[string]int{}, Refused: map[string]int{}})
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
			v.Applications = append(v.Applications, b.servers[id].entry.Application)
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

// dialLoopback says something listens on the loopback port.
func dialLoopback(ctx context.Context, port int) error {
	d := net.Dialer{Timeout: 2 * time.Second}
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return c.Close()
}
