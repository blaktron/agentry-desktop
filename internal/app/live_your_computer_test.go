package app

// Your computer live: the person's list on this computer, the link socket
// served in this process, and the agentry command run against it, as a
// person runs a QuickAgent from a terminal while Agentry Desktop is open.
// Skipped unless AGENTRY_LIVE_YOUR_COMPUTER=1:
//
//	AGENTRY_LIVE_YOUR_COMPUTER=1 \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home, its policy.yaml naming the model> \
//	AGENTRY_LIVE_CLI=<the agentry command to run> \
//	AGENTRY_LIVE_QA=<a folder holding qa-blender, qa-everything and qa-window> \
//	AGENTRY_LIVE_OUT=<a scratch folder the runs' folders go in> \
//	AGENTRY_LIVE_MODEL_ENV=<the variable the policy reads the model key from> \
//	go test -run TestLiveYourComputer -v -timeout 60m ./internal/app
//
// The model key is read from the variable AGENTRY_LIVE_MODEL_ENV names in
// this process and handed to the agentry command alone, never to the
// applications' servers, which run with this process's environment.
//
// What it holds, in order: the first start finds Blender ready here and adds
// it unapproved; a run that uses Blender's server is refused until the person
// approves it, then runs and its receipt names the library; a reference MCP
// server the person adds by hand is tested, approved and used by a release
// naming its id, its unclassed tool asks before each call, and once classed
// read it asks no more; a policy that allows only the library's applications
// refuses that server; and an application approved for its windows alone is
// driven through computer use.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/linkservice"
)

func TestLiveYourComputer(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_YOUR_COMPUTER") != "1" {
		t.Skip("uses this computer's applications: set AGENTRY_LIVE_YOUR_COMPUTER=1 (see the file's comment)")
	}
	for _, v := range []string{configdir.EnvVar, policy.HomeEnv, "AGENTRY_LIVE_CLI", "AGENTRY_LIVE_QA", "AGENTRY_LIVE_OUT", "AGENTRY_LIVE_MODEL_ENV"} {
		if os.Getenv(v) == "" {
			t.Fatalf("set %s (see the file's comment)", v)
		}
	}
	home, cli, qa, out := os.Getenv(policy.HomeEnv), os.Getenv("AGENTRY_LIVE_CLI"), os.Getenv("AGENTRY_LIVE_QA"), os.Getenv("AGENTRY_LIVE_OUT")
	keyEnv := os.Getenv("AGENTRY_LIVE_MODEL_ENV")
	key := os.Getenv(keyEnv)
	if key == "" {
		t.Fatalf("%s holds no model key", keyEnv)
	}
	// The servers this process starts get its environment: the key is not
	// in it.
	if err := os.Unsetenv(keyEnv); err != nil {
		t.Fatal(err)
	}

	old := emitEvent
	emitEvent = func(*App, string, ...any) {}
	t.Cleanup(func() { emitEvent = old })

	var mu sync.Mutex
	var dialogs []string
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		dialogs = append(dialogs, title)
		t.Logf("dialog %q:\n%s", title, message)
		return true, nil
	}}
	var execAsked []linkservice.ExecQuestion
	app.chooseExec = func(_ context.Context, q linkservice.ExecQuestion) (linkservice.ExecAnswer, error) {
		mu.Lock()
		defer mu.Unlock()
		execAsked = append(execAsked, q)
		t.Logf("exec dialog: %s, tool %s: %s", q.Application, q.Tool, q.What)
		return linkservice.ExecAllowOnce, nil
	}
	asked := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(execAsked)
	}
	r := &RunAPI{app: app}
	y := newYourComputerAPI(app, r)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// The first start's discovery.
	y.discover(ctx)
	view := y.State()
	var blender *linkservice.AppState
	for i, a := range view.Applications {
		t.Logf("on the list: %s (%s) from %s, mcp=%v computer=%v ready=%v %s", a.ID, a.Application, a.From, a.MCP, a.Computer, a.Ready, a.Missing)
		if a.ID == "blender" {
			blender = &view.Applications[i]
		}
	}
	if blender == nil || blender.From != "library" || blender.MCP || blender.Computer {
		t.Fatalf("the discovery did not add Blender, unapproved, from the library: %+v", view.Applications)
	}

	y.start(ctx)
	t.Cleanup(y.shutdown)
	if y.socketErr != "" {
		t.Fatalf("the link socket: %s", y.socketErr)
	}

	runQA := func(label, dir string) (string, map[string]string, int) {
		t.Helper()
		folder := filepath.Join(out, label)
		_ = os.RemoveAll(folder)
		if err := os.MkdirAll(folder, 0o700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.CommandContext(ctx, cli, "run", "--local", filepath.Join(qa, dir), "--allow-unsigned", "--yes", "--out", folder) //nolint:gosec // the live test's own command and folders
		cmd.Dir = folder
		cmd.Env = append(os.Environ(), keyEnv+"="+key)
		var stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stderr, &stderr
		cmd.Stdin = strings.NewReader("")
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		lines := map[string]string{}
		if b, err := os.ReadFile(filepath.Join(folder, "result.json")); err == nil {
			var env struct {
				Caps map[string]string `json:"capabilities_enforced"`
			}
			_ = json.Unmarshal(b, &env)
			for k, v := range env.Caps {
				if strings.HasPrefix(k, "desktop.") {
					lines[k] = v
				}
			}
		}
		log := stderr.String()
		if len(log) > 4000 {
			log = log[len(log)-4000:]
		}
		t.Logf("%s: exit %d\n%s", label, code, log)
		for k, v := range lines {
			t.Logf("%s: %s: %s", label, k, v)
		}
		return stderr.String(), lines, code
	}

	// Blender before and after the person approves its server.
	if msg, _, code := runQA("blender-unapproved", "qa-blender"); code != 11 || !strings.Contains(msg, "Agents may use its MCP server") {
		t.Fatalf("an unapproved Blender: exit %d", code)
	}
	if _, err := y.SetApproval("blender", "mcp", true); err != nil {
		t.Fatal(err)
	}
	if _, lines, code := runQA("blender", "qa-blender"); code != 0 || !strings.Contains(lines["desktop.blender"], "set up from Agentry Desktop's library") {
		t.Fatalf("an approved Blender: exit %d, %v", code, lines)
	}

	// A reference MCP server the person adds by hand.
	n := NewApplication{Name: "Everything", ID: "everything", Server: "stdio", Command: []string{"npx", "-y", "@modelcontextprotocol/server-everything@2.2.1", "stdio"}}
	tested, err := y.TestServer(n)
	if err != nil || tested.Error != "" || len(tested.Tools) == 0 {
		t.Fatalf("Test: %v %+v", err, tested)
	}
	t.Logf("Test listed %v", tested.Tools)
	if _, err := y.AddApplication(n, tested.Tools); err != nil {
		t.Fatal(err)
	}
	if _, err := y.SetApproval("everything", "mcp", true); err != nil {
		t.Fatal(err)
	}
	before := asked()
	if _, lines, code := runQA("everything", "qa-everything"); code != 0 || !strings.Contains(lines["desktop.everything"], "set up by the person in Agentry Desktop, not vetted by Agentry") {
		t.Fatalf("the person's server: exit %d, %v", code, lines)
	}
	if asked() == before {
		t.Fatal("an unclassed tool did not ask")
	}
	for _, tool := range []string{"echo", "get-sum"} {
		if _, err := y.SetToolClass("everything", tool, "read"); err != nil {
			t.Fatal(err)
		}
	}
	before = asked()
	if _, _, code := runQA("everything-classed", "qa-everything"); code != 0 {
		t.Fatalf("the classed server: exit %d", code)
	}
	if asked() != before {
		t.Fatal("a tool the person classed read still asked")
	}

	// A policy that allows only the library's applications.
	polPath := filepath.Join(home, "policy.yaml")
	pol, err := os.ReadFile(polPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.WriteFile(polPath, pol, 0o600) })
	if err := os.WriteFile(polPath, append(append([]byte{}, pol...), []byte("desktop:\n  allowCustomServers: false\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if msg, _, code := runQA("everything-policy", "qa-everything"); code != 11 || !strings.Contains(msg, "allowCustomServers: false") {
		t.Fatalf("the policy did not refuse the person's server: exit %d", code)
	}
	if _, _, code := runQA("blender-policy", "qa-blender"); code != 0 {
		t.Fatalf("the policy refused the library's Blender: exit %d", code)
	}
	if err := os.WriteFile(polPath, pol, 0o600); err != nil {
		t.Fatal(err)
	}

	// An application approved for its windows alone, through computer use.
	if os.Getenv("AGENTRY_LIVE_WINDOW_APP") != "" {
		ids := strings.Split(os.Getenv("AGENTRY_LIVE_WINDOW_APP"), ",")
		if _, err := y.AddApplication(NewApplication{Name: "KCalc", ID: "kcalc", Windows: ids}, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := y.SetComputerUse(true); err != nil {
			t.Fatal(err)
		}
		if _, err := y.SetApproval("kcalc", "computer", true); err != nil {
			t.Fatal(err)
		}
		_, lines, code := runQA("window", "qa-window")
		if code != 0 || lines["desktop.computerUse"] == "" {
			t.Fatalf("computer use of a windows-only application: exit %d, %v", code, lines)
		}
	}
	for _, rr := range y.link.Recent() {
		b, _ := json.Marshal(rr)
		t.Logf("Settings' record: %s", b)
	}
	mu.Lock()
	t.Logf("native dialogs: %q", dialogs)
	mu.Unlock()
	time.Sleep(100 * time.Millisecond)
}
