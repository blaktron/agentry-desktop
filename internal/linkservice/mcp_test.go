package linkservice

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
)

// The client speaks the current specification to a server that does and
// falls back to the 2025-11-25 handshake for one that answers the modern
// probe with an error or with silence; the tools and calls work in each.
func TestTheClientSpeaksBothEras(t *testing.T) {
	old := probeTimeout
	probeTimeout = 300 * time.Millisecond
	t.Cleanup(func() { probeTimeout = old })
	for era, want := range map[string]string{"modern": EraModern, "legacy": EraLegacy, "silent-legacy": EraLegacy} {
		t.Run(era, func(t *testing.T) {
			s := pipeSession(t, era)
			if s.Era() != want {
				t.Fatalf("era %q, want %q", s.Era(), want)
			}
			tools, err := s.Tools(context.Background())
			if err != nil || len(tools) != len(standInTools) {
				t.Fatalf("tools %+v %v", tools, err)
			}
			a, err := s.Call(context.Background(), "get_scene_info", json.RawMessage(`{"depth":1}`))
			if err != nil || a.IsError || !strings.Contains(a.Text(), `get_scene_info({"depth":1})`) {
				t.Fatalf("call %+v %v", a, err)
			}
		})
	}
}

// A real process: the stand-in started from a launch spec, its variables
// set over the app's environment, its era found, and ended with Close.
func TestAStdioServerIsStartedAndEnded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in process check runs on Unix")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LINKSERVICE_STANDIN_ENV_ECHO", "DISABLE_TELEMETRY")
	spec := LaunchSpec{Command: []string{exe, "-test.run=^$"}, Env: map[string]string{standInEnv: "legacy", "DISABLE_TELEMETRY": "true"}}
	s, err := startStdio(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if s.Era() != EraLegacy {
		t.Fatalf("era %q", s.Era())
	}
	a, err := s.Call(context.Background(), "look", nil)
	if err != nil || !strings.Contains(a.Text(), `DISABLE_TELEMETRY="true"`) {
		t.Fatalf("call %+v %v", a, err)
	}
	ss := s.(*stdioSession)
	_ = s.Close()
	select {
	case <-ss.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the server outlived Close")
	}
	if _, err := startStdio(context.Background(), LaunchSpec{Command: []string{filepath.Join(t.TempDir(), "no-such-launcher")}}); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("a missing launcher: %v", err)
	}
}

// Over Streamable HTTP: a modern server is spoken to with the version
// header and _meta, a legacy one answers the modern request with a 400 and
// is spoken to after the handshake in its session, and an answer may come
// as an event stream.
func TestTheClientSpeaksStreamableHTTP(t *testing.T) {
	for _, era := range []string{"modern", "legacy"} {
		t.Run(era, func(t *testing.T) {
			var mu sync.Mutex
			session := ""
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				var m message
				_ = json.Unmarshal(body, &m)
				modernReq := r.Header.Get("MCP-Protocol-Version") == ModernVersion
				mu.Lock()
				sid := session
				mu.Unlock()
				reply := func(result any) {
					b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
					if m.Method == "tools/call" {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\ndata: "+string(b)+"\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write(b)
				}
				switch era {
				case "modern":
					if !modernReq || !strings.Contains(string(body), "io.modelcontextprotocol/protocolVersion") {
						http.Error(w, "bad", http.StatusBadRequest)
						return
					}
				case "legacy":
					if modernReq {
						http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
						return
					}
					if m.Method == "initialize" {
						mu.Lock()
						session = "S1"
						mu.Unlock()
						w.Header().Set("Mcp-Session-Id", "S1")
						reply(map[string]any{"protocolVersion": LegacyVersion})
						return
					}
					if r.Header.Get("Mcp-Session-Id") != sid || sid == "" {
						http.Error(w, "no session", http.StatusBadRequest)
						return
					}
					if len(m.ID) == 0 {
						w.WriteHeader(http.StatusAccepted)
						return
					}
				}
				switch m.Method {
				case "tools/list":
					reply(map[string]any{"tools": []map[string]any{{"name": "call_tool"}}})
				case "tools/call":
					reply(map[string]any{"content": []map[string]any{{"type": "text", "text": "called by " + era}}})
				default:
					w.WriteHeader(http.StatusAccepted)
				}
			}))
			defer srv.Close()
			s, err := dialHTTP(context.Background(), srv.URL+"/mcp")
			if err != nil {
				t.Fatal(err)
			}
			if s.Era() != map[string]string{"modern": EraModern, "legacy": EraLegacy}[era] {
				t.Fatalf("era %q", s.Era())
			}
			tools, err := s.Tools(context.Background())
			if err != nil || len(tools) != 1 || tools[0].Name != "call_tool" {
				t.Fatalf("tools %+v %v", tools, err)
			}
			a, err := s.Call(context.Background(), "call_tool", json.RawMessage(`{}`))
			if err != nil || a.Text() != "called by "+era {
				t.Fatalf("call %+v %v", a, err)
			}
			_ = s.Close()
		})
	}
}

// The link socket is owner-only, a socket nothing answers on is replaced,
// one something answers on is left to it, and a CLI run's bind and call
// cross it.
func TestTheLinkSocketIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions")
	}
	home, err := os.MkdirTemp("", "ls")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	stale := filepath.Join(home, "s", desktop.SocketName)
	_ = os.MkdirAll(filepath.Dir(stale), 0o700)
	l, err := net.Listen("unix", stale)
	if err != nil {
		t.Fatal(err)
	}
	if ul, ok := l.(*net.UnixListener); ok {
		ul.SetUnlinkOnClose(false)
	}
	_ = l.Close()
	ln, path, err := Listen(home)
	if err != nil {
		t.Fatalf("a socket nothing answers on: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the socket's mode: %v %v", fi.Mode(), err)
	}
	if _, _, err := Listen(home); err == nil || !strings.Contains(err.Error(), "already answers") {
		t.Fatalf("a second Listen while served: %v", err)
	}
	r := newRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.s.Serve(ctx, ln, path) }()
	c := desktop.Client{Socket: path}
	b, err := c.Bind(context.Background(), desktop.BindRequest{Run: "R", Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"look"}}}})
	if err != nil {
		t.Fatal(err)
	}
	a, err := c.Call(context.Background(), desktop.CallRequest{Run: "R", Token: b.Token, Server: "blender", Tool: "look"})
	if err != nil || a.Text() != "ran look" {
		t.Fatalf("a call over the socket: %+v %v", a, err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Serve returned %v", err)
	}
	if _, err := os.Lstat(path); err == nil {
		t.Fatal("the socket outlived the service")
	}
}
