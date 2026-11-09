package linkservice

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"

	"github.com/blaktron/agentry-desktop/internal/yourcomputer"
)

// bindBlender binds run R to Blender with the tools given.
func bindBlender(t *testing.T, r *rig, tools ...string) desktop.Bound {
	t.Helper()
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "R", Ref: "acme/scene@1.0.0", Servers: []desktop.BindServer{{ID: "blender", Tools: tools}}})
	if ref != nil {
		t.Fatalf("the bind was refused: %+v", ref)
	}
	return b
}

func call(r *rig, token, server, tool, args string) (string, *desktop.Refusal) {
	a, ref := r.s.Call(context.Background(), desktop.CallRequest{Run: "R", Token: token, Server: server, Tool: tool, Arguments: json.RawMessage(args)})
	return a.Text(), ref
}

// The link service answers only a run it bound, with that run's token,
// only for a server the run bound and only for a tool the release was
// given; everything else is refused and never reaches the installed
// server. A bind answers with the token and the tools the server listed,
// narrowed to the ones the release was given.
func TestTheLinkServiceAnswersOnlyBoundServers(t *testing.T) {
	r := newRig(t)
	b := bindBlender(t, r, "get_scene_info", "look")
	if len(b.Token) != 64 {
		t.Fatalf("the run token is %q", b.Token)
	}
	var names []string
	for _, tool := range b.Servers["blender"] {
		names = append(names, tool.Name)
	}
	if !slices.Equal(names, []string{"get_scene_info", "look"}) {
		t.Fatalf("the bound tools are %v", names)
	}
	if text, ref := call(r, b.Token, "blender", "get_scene_info", `{}`); ref != nil || text != "ran get_scene_info" {
		t.Fatalf("a bound call: %q %+v", text, ref)
	}
	for name, c := range map[string]struct{ run, token, server, tool string }{
		"another run":       {"Q", b.Token, "blender", "get_scene_info"},
		"another token":     {"R", strings.Repeat("0", 64), "blender", "get_scene_info"},
		"no token":          {"R", "", "blender", "get_scene_info"},
		"an unbound server": {"R", b.Token, "unreal", "call_tool"},
		"a tool not given":  {"R", b.Token, "blender", "execute_blender_code"},
		"a withheld tool":   {"R", b.Token, "blender", "disable_telemetry"},
		"an unknown tool":   {"R", b.Token, "blender", "rm_rf"},
	} {
		_, ref := r.s.Call(context.Background(), desktop.CallRequest{Run: c.run, Token: c.token, Server: c.server, Tool: c.tool})
		if ref == nil {
			t.Errorf("%s: the call was answered", name)
		}
	}
	if got := r.sess.called(); !slices.Equal(got, []string{"get_scene_info"}) {
		t.Fatalf("the installed server was called with %v", got)
	}
	if len(r.questions()) != 0 {
		t.Fatalf("a read call asked: %+v", r.questions())
	}
	// Unbound, the run's token answers nothing and the server ends.
	if ref := r.s.Unbind(context.Background(), desktop.UnbindRequest{Run: "R", Token: "wrong"}); ref == nil {
		t.Fatal("an unbind with another token was taken")
	}
	if ref := r.s.Unbind(context.Background(), desktop.UnbindRequest{Run: "R", Token: b.Token}); ref != nil {
		t.Fatal(ref)
	}
	if _, ref := call(r, b.Token, "blender", "get_scene_info", `{}`); ref == nil || ref.Reason != receipt.DesktopUnavailable {
		t.Fatalf("a call after unbind: %+v", ref)
	}
	if !r.sess.isClosed() {
		t.Fatal("the installed server outlived the unbind")
	}
}

// An exec-class call asks the person first, quoting what would run, and
// runs only on a Yes: declined, it is refused desktop-declined and never
// reaches the server; allowed once, the next one asks again; allowed for
// the run, no later exec call of the run asks. Read and act calls never
// ask.
func TestExecClassCallsAskFirst(t *testing.T) {
	r := newRig(t)
	b := bindBlender(t, r, "execute_blender_code", "get_scene_info", "generate_3d")
	code := `{"code":"import bpy\nbpy.ops.mesh.primitive_cube_add()"}`

	_, ref := call(r, b.Token, "blender", "execute_blender_code", code)
	if ref == nil || ref.Reason != receipt.DesktopDeclined {
		t.Fatalf("a declined exec call: %+v", ref)
	}
	q := r.questions()
	if len(q) != 1 || q[0].Application != "Blender" || q[0].Tool != "execute_blender_code" || q[0].What != "import bpy\nbpy.ops.mesh.primitive_cube_add()" || q[0].Who != "acme/scene@1.0.0" {
		t.Fatalf("the question: %+v", q)
	}
	if len(r.sess.called()) != 0 {
		t.Fatalf("a declined call reached the server: %v", r.sess.called())
	}

	r.setAnswer(ExecAllowOnce)
	if _, ref := call(r, b.Token, "blender", "execute_blender_code", code); ref != nil {
		t.Fatal(ref)
	}
	r.setAnswer(ExecAllowRun)
	if _, ref := call(r, b.Token, "blender", "execute_blender_code", code); ref != nil {
		t.Fatal(ref)
	}
	if n := len(r.questions()); n != 3 {
		t.Fatalf("questions after allow once and allow for the run: %d", n)
	}
	r.setAnswer(ExecDecline)
	for range 3 {
		if _, ref := call(r, b.Token, "blender", "execute_blender_code", code); ref != nil {
			t.Fatalf("an exec call after Allow every exec call for this run: %+v", ref)
		}
	}
	if _, ref := call(r, b.Token, "blender", "get_scene_info", `{}`); ref != nil {
		t.Fatal(ref)
	}
	if _, ref := call(r, b.Token, "blender", "generate_3d", `{"prompt":"a chair"}`); ref != nil {
		t.Fatal(ref)
	}
	if n := len(r.questions()); n != 3 {
		t.Fatalf("the run asked %d times, want 3", n)
	}
	if n := len(r.sess.called()); n != 7 {
		t.Fatalf("the server ran %d calls, want 7", n)
	}
	// With no one to ask, an exec call is declined, never run.
	none := New(Options{Applications: r.list, LookPath: r.s.opt.LookPath, Listening: r.s.opt.Listening, Launch: r.s.opt.Launch, GOOS: "linux"})
	nb, nref := none.Bind(context.Background(), desktop.BindRequest{Run: "R", Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"execute_blender_code"}}}})
	if nref != nil {
		t.Fatal(nref)
	}
	if _, ref := none.Call(context.Background(), desktop.CallRequest{Run: "R", Token: nb.Token, Server: "blender", Tool: "execute_blender_code", Arguments: json.RawMessage(code)}); ref == nil || ref.Reason != receipt.DesktopDeclined {
		t.Fatalf("an exec call with no dialog: %+v", ref)
	}
}

// Stop ends the call in flight, ends the run's installed servers, and
// refuses every later call of the run desktop-stopped, whatever its class;
// Pause refuses desktop-paused until Resume, and a stopped run cannot be
// resumed.
func TestStopRefusesEveryLaterCall(t *testing.T) {
	r := newRig(t)
	b := bindBlender(t, r, "execute_blender_code", "get_scene_info")

	if err := r.s.Pause("R"); err != nil {
		t.Fatal(err)
	}
	if _, ref := call(r, b.Token, "blender", "get_scene_info", `{}`); ref == nil || ref.Reason != receipt.DesktopPaused {
		t.Fatalf("a call while paused: %+v", ref)
	}
	if err := r.s.Resume("R"); err != nil {
		t.Fatal(err)
	}
	if _, ref := call(r, b.Token, "blender", "get_scene_info", `{}`); ref != nil {
		t.Fatalf("a call after resume: %+v", ref)
	}

	r.sess.mu.Lock()
	r.sess.block = make(chan struct{})
	r.sess.mu.Unlock()
	before := len(r.sess.called())
	var wg sync.WaitGroup
	var inFlight *desktop.Refusal
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, inFlight = call(r, b.Token, "blender", "get_scene_info", `{}`)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for len(r.sess.called()) <= before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if err := r.s.Stop("R"); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if inFlight == nil || inFlight.Reason != receipt.DesktopStopped {
		t.Fatalf("the call in flight at Stop: %+v", inFlight)
	}
	if !r.sess.isClosed() {
		t.Fatal("the installed server outlived Stop")
	}
	r.setAnswer(ExecAllowRun)
	for _, tool := range []string{"get_scene_info", "execute_blender_code"} {
		if _, ref := call(r, b.Token, "blender", tool, `{"code":"x"}`); ref == nil || ref.Reason != receipt.DesktopStopped {
			t.Fatalf("%s after Stop: %+v", tool, ref)
		}
	}
	if len(r.questions()) != 0 {
		t.Fatalf("a stopped run asked: %+v", r.questions())
	}
	if err := r.s.Resume("R"); err == nil {
		t.Fatal("a stopped run was resumed")
	}
	if ref := r.s.Unbind(context.Background(), desktop.UnbindRequest{Run: "R", Token: b.Token}); ref != nil {
		t.Fatal(ref)
	}
}

// What runs on the person's computer is the launch line and variables of
// the person's own application, whatever the bind or a call says: the bind
// names an application id and tools, a call names a tool and arguments, and
// none of it reaches the launch. A variable the person took off the
// application's server is not set.
func TestAnInstalledServerIsLaunchedFromThePersonsApplicationAlone(t *testing.T) {
	r := newRig(t)
	e := blender(t)
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{
		Run: "R", Ref: "acme/scene@1.0.0; rm -rf ~", Digest: "sha256:$(id)",
		Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"execute_blender_code", "get_scene_info"}}},
	})
	if ref != nil {
		t.Fatal(ref)
	}
	r.setAnswer(ExecAllowRun)
	if _, ref := call(r, b.Token, "blender", "execute_blender_code", `{"code":"import os; os.system('sh')","command":["sh"]}`); ref != nil {
		t.Fatal(ref)
	}
	if len(r.launches) != 1 {
		t.Fatalf("launches: %+v", r.launches)
	}
	got := r.launches[0]
	if !slices.Equal(got.Command, e.Launch.Command) {
		t.Fatalf("the launch ran %q, the application's is %q", got.Command, e.Launch.Command)
	}
	if len(got.Env) != len(e.Launch.Env) || got.Env["DISABLE_TELEMETRY"] != "true" || got.Env["BLENDER_MCP_SAFE_MODE"] != "1" {
		t.Fatalf("the launch's variables %v, the application's %v", got.Env, e.Launch.Env)
	}
	// A bind naming an id the list does not have launches nothing.
	if _, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "S", Servers: []desktop.BindServer{{ID: "sh", Tools: []string{"sh"}}}}); ref == nil {
		t.Fatal("a bind of an unknown id was taken")
	}
	if len(r.launches) != 1 {
		t.Fatalf("an unknown id launched: %+v", r.launches)
	}
	// The person takes the screening variable off: it goes, nothing else.
	if _, err := r.list.Update(func(d *yourcomputer.Doc) error {
		return d.Change("blender", func(a *yourcomputer.Application) error {
			srv := *a.Server
			srv.Env = map[string]string{"DISABLE_TELEMETRY": "true"}
			a.SetServer(&srv)
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	if _, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "T", Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"look"}}}}); ref != nil {
		t.Fatal(ref)
	}
	last := r.launches[len(r.launches)-1]
	if _, on := last.Env["BLENDER_MCP_SAFE_MODE"]; on || last.Env["DISABLE_TELEMETRY"] != "true" || !slices.Equal(last.Command, e.Launch.Command) {
		t.Fatalf("with the variable taken off the launch is %+v", last)
	}
}

// The pre-start check refuses a bind with the step the person must take:
// the launcher missing, the application's listener silent, a narrowed tool
// the server does not list, a narrowing of only withheld tools; and a
// refused bind leaves no server running.
func TestABindIsRefusedWithTheStepToTake(t *testing.T) {
	for name, tc := range map[string]struct {
		opt   func(*Options, *fakeSession)
		tools []string
		want  string
	}{
		"no launcher": {func(o *Options, _ *fakeSession) {
			o.LookPath = func(string) (string, error) { return "", errors.New("not found") }
		}, []string{"look"}, "Install uv"},
		"Blender closed": {func(o *Options, _ *fakeSession) {
			o.Listening = func(context.Context, int) error { return errors.New("refused") }
		}, []string{"look"}, "does not answer on port 9876"},
		"a tool the server does not list": {func(*Options, *fakeSession) {}, []string{"run_shell"}, "the tool run_shell of Blender, which its MCP server on this computer does not list"},
		"only withheld tools":             {func(*Options, *fakeSession) {}, []string{"disable_telemetry"}, "given no tool of Blender"},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			opt := r.s.opt
			tc.opt(&opt, r.sess)
			s := New(opt)
			_, ref := s.Bind(context.Background(), desktop.BindRequest{Run: "R", Servers: []desktop.BindServer{{ID: "blender", Tools: tc.tools}}})
			if ref == nil || ref.Reason != receipt.DesktopNotInstalled || ref.Server != "blender" || !strings.Contains(ref.Message, tc.want) {
				t.Fatalf("refusal %+v, want one saying %q", ref, tc.want)
			}
			if len(r.launches) > 0 && !r.sess.isClosed() {
				t.Fatal("a refused bind left its server running")
			}
			if len(s.Runs()) != 0 {
				t.Fatal("a refused bind is listed as bound")
			}
		})
	}
}

// At most four calls of one run are answered at once; the fifth waits.
func TestARunHasAtMostFourCallsAtOnce(t *testing.T) {
	r := newRig(t)
	b := bindBlender(t, r, "get_scene_info")
	release := make(chan struct{})
	r.sess.mu.Lock()
	r.sess.block = release
	r.sess.mu.Unlock()
	var wg sync.WaitGroup
	for range MaxCallsInFlight + 1 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = call(r, b.Token, "blender", "get_scene_info", `{}`)
		}()
	}
	time.Sleep(200 * time.Millisecond)
	if n := len(r.sess.called()); n != MaxCallsInFlight {
		t.Fatalf("%d calls in flight, want %d", n, MaxCallsInFlight)
	}
	close(release)
	wg.Wait()
	if n := len(r.sess.called()); n != MaxCallsInFlight+1 {
		t.Fatalf("%d calls ran, want %d", n, MaxCallsInFlight+1)
	}
}

// Settings' record of a run that used the computer: its calls by class and
// outcome, the exec questions and answers, how it ended.
func TestARunThatUsedTheComputerIsRecorded(t *testing.T) {
	r := newRig(t)
	path := t.TempDir() + "/desktop-runs.json"
	r.s.opt.Recent = OpenRecent(path)
	b := bindBlender(t, r, "execute_blender_code", "get_scene_info")
	_, _ = call(r, b.Token, "blender", "get_scene_info", `{}`)
	_, _ = call(r, b.Token, "blender", "execute_blender_code", `{"code":"x"}`)
	r.setAnswer(ExecAllowRun)
	_, _ = call(r, b.Token, "blender", "execute_blender_code", `{"code":"x"}`)
	_ = r.s.Unbind(context.Background(), desktop.UnbindRequest{Run: "R", Token: b.Token})
	list := OpenRecent(path).List()
	if len(list) != 1 {
		t.Fatalf("recent runs: %+v", list)
	}
	got := list[0]
	srv := got.Servers[0]
	if got.Run != "R" || got.State != "ended" || got.Ended == nil || got.ExecAsked != 2 || got.ExecDeclined != 1 || !got.ExecForRun ||
		srv.Calls["read"] != 1 || srv.Calls["exec"] != 2 || srv.Refused[receipt.DesktopDeclined] != 1 || !strings.Contains(srv.Launch, "uvx mcp-for-blender==") {
		t.Fatalf("the record: %+v", got)
	}
}

// A quit stops every bound run and records its use of the computer as
// stopped, with the calls it made; a list read by the next Agentry Desktop
// never says a run is still using the computer.
func TestAQuitRecordsEveryRunAsStopped(t *testing.T) {
	r := newRig(t)
	path := t.TempDir() + "/desktop-runs.json"
	r.s.opt.Recent = OpenRecent(path)
	b := bindBlender(t, r, "get_scene_info")
	_, _ = call(r, b.Token, "blender", "get_scene_info", `{}`)
	r.s.StopAll()
	got := OpenRecent(path).List()
	if len(got) != 1 || got[0].State != StateStopped || got[0].Ended == nil || got[0].Servers[0].Calls["read"] != 1 {
		t.Fatalf("after a quit the record is %+v", got)
	}
	// One left unended by an Agentry Desktop that died is read as interrupted.
	_ = os.WriteFile(path, []byte(`[{"run":"Q","started":"2026-11-07T12:00:00Z","state":"active","servers":[]}]`), 0o600)
	if got := OpenRecent(path).List(); len(got) != 1 || got[0].State != StateInterrupted {
		t.Fatalf("an unended record reads %+v", got)
	}
}

// The exec dialog quotes the code and every other argument the call
// carries, so nothing of it is hidden; a call longer than the dialog shows
// in full is refused without asking.
func TestTheExecQuestionHidesNothingOfTheCall(t *testing.T) {
	r := newRig(t)
	b := bindBlender(t, r, "execute_blender_code")
	_, _ = call(r, b.Token, "blender", "execute_blender_code", `{"code":"import bpy","also":"rm -rf ~"}`)
	q := r.questions()
	if len(q) != 1 || !strings.HasPrefix(q[0].What, "import bpy") || !strings.Contains(q[0].What, `also: "rm -rf ~"`) {
		t.Fatalf("the question: %+v", q)
	}
	r.setAnswer(ExecAllowRun)
	long, _ := json.Marshal(map[string]string{"code": strings.Repeat("x", maxWhat+1)})
	if _, ref := call(r, b.Token, "blender", "execute_blender_code", string(long)); ref == nil || ref.Reason != receipt.DesktopDeclined {
		t.Fatalf("a call too long to show: %+v", ref)
	}
	if len(r.questions()) != 1 || len(r.sess.called()) != 0 {
		t.Fatalf("a call too long to show was asked about or run: %d %v", len(r.questions()), r.sess.called())
	}
}

// Once the app is quitting, no run is bound.
func TestNoRunIsBoundOnceTheAppQuits(t *testing.T) {
	r := newRig(t)
	r.s.StopAll()
	if _, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "R", Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"look"}}}}); ref == nil || ref.Reason != receipt.DesktopUnavailable {
		t.Fatalf("a bind after the quit: %+v", ref)
	}
	if len(r.launches) != 0 {
		t.Fatal("a bind after the quit launched a server")
	}
}
