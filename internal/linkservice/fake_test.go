package linkservice

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry/pkg/appserver"

	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"
)

// The stand-in installed server: this test binary, started with
// standInEnv set, answers MCP on its standard input and output in the era
// the variable names, listing standInTools.
const standInEnv = "LINKSERVICE_STANDIN_SERVER"

// standInTools are the tools the stand-in lists: Blender's catalogue tools
// and one it withholds.
var standInTools = []string{"execute_blender_code", "get_scene_info", "look", "get_addon_status", "search_assets", "generate_3d", "import_asset", "disable_telemetry"}

func TestMain(m *testing.M) {
	if era := os.Getenv(standInEnv); era != "" {
		serveFakeMCP(os.Stdin, os.Stdout, era, standInTools, os.Getenv("LINKSERVICE_STANDIN_ENV_ECHO"))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// serveFakeMCP is a minimal MCP server: era "modern" answers server/discover
// and serves every request statelessly; "legacy" refuses anything before
// initialize with -32602 and serves after it; "silent-legacy" answers
// nothing before initialize. Its tools/call answers with the tool's name and
// its arguments, and with the named variable's value when echo is set.
func serveFakeMCP(r io.Reader, w io.Writer, era string, tools []string, echo string) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	var mu sync.Mutex
	send := func(v any) {
		b, _ := json.Marshal(v)
		mu.Lock()
		_, _ = w.Write(append(b, '\n'))
		mu.Unlock()
	}
	initialized := false
	for sc.Scan() {
		var m message
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		if len(m.ID) == 0 {
			if m.Method == "notifications/initialized" {
				initialized = true
			}
			continue
		}
		reply := func(result any) { send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result}) }
		fail := func(code int, msg string) {
			send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": map[string]any{"code": code, "message": msg}})
		}
		modernReq := strings.Contains(string(m.Params), "io.modelcontextprotocol/protocolVersion")
		switch era {
		case "modern":
			if m.Method == "initialize" {
				fail(-32601, "initialize is not a method of this server")
				continue
			}
			if !modernReq {
				fail(-32602, "missing _meta")
				continue
			}
		case "legacy", "silent-legacy":
			if m.Method == "initialize" {
				reply(map[string]any{"protocolVersion": LegacyVersion, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "stand-in", "version": "1"}})
				continue
			}
			if !initialized {
				if era == "legacy" {
					fail(-32602, "Invalid request parameters")
				}
				continue
			}
		}
		switch m.Method {
		case "server/discover":
			reply(map[string]any{"supportedVersions": []string{ModernVersion}, "capabilities": map[string]any{"tools": map[string]any{}}})
		case "tools/list":
			var ts []map[string]any
			for _, t := range tools {
				ts = append(ts, map[string]any{"name": t, "description": "the stand-in's " + t, "inputSchema": map[string]any{"type": "object"}})
			}
			reply(map[string]any{"tools": ts})
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(m.Params, &p)
			text := fmt.Sprintf("%s(%s) by a %s server", p.Name, p.Arguments, era)
			if echo != "" {
				text += fmt.Sprintf("; %s=%q", echo, os.Getenv(echo))
			}
			reply(map[string]any{"content": []map[string]any{{"type": "text", "text": text}}})
		default:
			fail(-32601, "no such method")
		}
	}
}

// pipeSession is an MCP session with serveFakeMCP over in-memory pipes.
func pipeSession(t *testing.T, era string) Session {
	t.Helper()
	cr, sw := io.Pipe()
	sr, cw := io.Pipe()
	go func() {
		serveFakeMCP(sr, sw, era, standInTools, "")
		_ = sw.Close()
	}()
	conn := newRPCConn(cr, cw)
	m := &mcp{t: conn}
	if err := m.negotiate(context.Background(), stdioProbe(conn)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cw.Close() })
	return &pipeSess{m: m, w: cw}
}

type pipeSess struct {
	m *mcp
	w io.Closer
}

func (p *pipeSess) Tools(ctx context.Context) ([]desktoplink.Tool, error) { return p.m.tools(ctx) }
func (p *pipeSess) Call(ctx context.Context, name string, args json.RawMessage) (desktoplink.Answer, error) {
	return p.m.callTool(ctx, name, args)
}
func (p *pipeSess) Era() string  { return p.m.era }
func (p *pipeSess) Close() error { return p.w.Close() }

// fakeSession is a session a test controls: what it lists, what each call
// answers, and whether a call blocks until its context ends.
type fakeSession struct {
	mu     sync.Mutex
	tools  []string
	calls  []string
	block  chan struct{}
	closed bool
}

func (f *fakeSession) Tools(context.Context) ([]desktoplink.Tool, error) {
	var out []desktoplink.Tool
	for _, t := range f.tools {
		out = append(out, desktoplink.Tool{Name: t, InputSchema: json.RawMessage(`{"type":"object"}`)})
	}
	return out, nil
}

func (f *fakeSession) Call(ctx context.Context, name string, args json.RawMessage) (desktoplink.Answer, error) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	block := f.block
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return desktoplink.Answer{}, ctx.Err()
		}
	}
	return desktoplink.TextAnswer("ran "+name, false), nil
}

func (f *fakeSession) Era() string { return EraLegacy }

func (f *fakeSession) Close() error {
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

func (f *fakeSession) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeSession) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

// rig is a link service over the built-in catalogue with Blender ready on
// this computer: its launcher found, its listener answering, and a fake
// session for every launch, the launches recorded.
type rig struct {
	s        *Service
	sess     *fakeSession
	launches []LaunchSpec
	asked    []ExecQuestion
	answer   ExecAnswer
	mu       sync.Mutex
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{sess: &fakeSession{tools: standInTools}, answer: ExecDecline}
	r.s = New(Options{
		LookPath:  func(string) (string, error) { return "/usr/bin/uvx", nil },
		Listening: func(context.Context, int) error { return nil },
		Launch: func(_ context.Context, spec LaunchSpec) (Session, error) {
			r.mu.Lock()
			r.launches = append(r.launches, spec)
			r.mu.Unlock()
			return r.sess, nil
		},
		Ask: func(_ context.Context, q ExecQuestion) (ExecAnswer, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.asked = append(r.asked, q)
			return r.answer, nil
		},
		GOOS: "linux",
	})
	return r
}

func (r *rig) setAnswer(a ExecAnswer) {
	r.mu.Lock()
	r.answer = a
	r.mu.Unlock()
}

func (r *rig) questions() []ExecQuestion {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ExecQuestion(nil), r.asked...)
}

// blender is the built-in catalogue's Blender entry.
func blender(t *testing.T) appserver.Server {
	t.Helper()
	e, ok := appserver.Builtin().Lookup("blender")
	if !ok {
		t.Fatal("the built-in catalogue has no blender entry")
	}
	return e
}
