package app

// The person's computer live: the bound methods driven as the Library and
// the run setup drive them, against this machine's container engine, a
// QuickAgent from a registry that uses Blender, and Blender running here
// with its MCP add-on listening. Skipped unless AGENTRY_LIVE_DESKTOP=1:
//
//	AGENTRY_LIVE_DESKTOP=1 \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home, its policy.yaml naming the model> \
//	AGENTRY_LIVE_DESKTOP_OUT=<a scratch folder the runs' folders go in> \
//	AGENTRY_LIVE_DESKTOP_REGISTRY=<the registry, default https://dev.secureagentry.net> \
//	AGENTRY_LIVE_DESKTOP_REF=<a release that declares blender> \
//	AGENTRY_LIVE_DESKTOP_EXPECT=run|closed   (closed: Blender is not running) \
//	AGENTRY_LIVE_DESKTOP_ANSWERS=decline,all  (the exec dialog's answers, in order) \
//	AGENTRY_LIVE_DESKTOP_TASK=<the run's task, optional> \
//	go test -run TestLiveDesktopLink -v -timeout 60m ./internal/app
//
// What it holds: the install's consent dialog names Blender in the words
// the CLI uses; with Blender closed, a run is refused before its sandbox
// with the step to take; with it open, the run binds in this process, each
// exec call is put to the exec dialog with the code it would run and is
// answered as the answers say, the run's folder keeps
// receipts/desktop-blender.log and the receipt's desktop.blender line, and
// nothing of the run is bound afterwards.

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/linkservice"
)

func TestLiveDesktopLink(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_DESKTOP") != "1" {
		t.Skip("uses Blender on this computer: set AGENTRY_LIVE_DESKTOP=1 (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" || os.Getenv("AGENTRY_LIVE_DESKTOP_OUT") == "" {
		t.Fatalf("set %s, %s and AGENTRY_LIVE_DESKTOP_OUT to scratch directories", configdir.EnvVar, policy.HomeEnv)
	}
	reg := cmp.Or(os.Getenv("AGENTRY_LIVE_DESKTOP_REGISTRY"), "https://dev.secureagentry.net")
	ref := cmp.Or(os.Getenv("AGENTRY_LIVE_DESKTOP_REF"), "chris/desktop-link-check@1.3.0")
	expect := cmp.Or(os.Getenv("AGENTRY_LIVE_DESKTOP_EXPECT"), "run")
	task := cmp.Or(os.Getenv("AGENTRY_LIVE_DESKTOP_TASK"), "Check the Blender scene as your instructions say.")
	out := os.Getenv("AGENTRY_LIVE_DESKTOP_OUT")

	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if len(data) == 1 {
			if s, ok := data[0].(string); ok {
				t.Logf("[%s] %s", name, s)
			}
		}
	}
	t.Cleanup(func() { emitEvent = old })

	var mu sync.Mutex
	var dialogs []string
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		dialogs = append(dialogs, title+"\n"+message)
		t.Logf("dialog %q:\n%s", title, message)
		return true, nil
	}}
	answers := strings.Split(cmp.Or(os.Getenv("AGENTRY_LIVE_DESKTOP_ANSWERS"), "decline,all"), ",")
	var asked []linkservice.ExecQuestion
	app.chooseExec = func(_ context.Context, q linkservice.ExecQuestion) (linkservice.ExecAnswer, error) {
		mu.Lock()
		defer mu.Unlock()
		asked = append(asked, q)
		title, message := execDialogText(q)
		a := "decline"
		if len(asked) <= len(answers) {
			a = strings.TrimSpace(answers[len(asked)-1])
		}
		t.Logf("exec dialog %d %q, answered %s:\n%s", len(asked), title, a, message)
		switch a {
		case "once":
			return linkservice.ExecAllowOnce, nil
		case "all":
			return linkservice.ExecAllowRun, nil
		}
		return linkservice.ExecDecline, nil
	}
	r := &RunAPI{app: app}
	y := newYourComputerAPI(app, r)
	installs := &InstallAPI{app: app, engine: install.NewEngine()}

	// Plugins → Computer use, as the page reads it.
	view := y.State()
	for _, a := range view.Applications {
		t.Logf("Your computer: %s ready=%v launch=%q listening=%v: %s", a.Application, a.Ready, a.Launch, a.Listening, a.Missing)
	}

	if _, err := installs.InstallRunner(reg, ref); err != nil {
		t.Fatalf("InstallRunner: %v", err)
	}
	mu.Lock()
	consent := strings.Join(dialogs, "\n")
	mu.Unlock()
	if !strings.Contains(consent, "desktop: Blender on your computer through its MCP server (") {
		t.Fatalf("the install's consent did not name Blender in the CLI's words:\n%s", consent)
	}

	app.markPicked(out)
	res, err := r.RunAgent(reg, ref, task, nil, out, nil, 0, -1, false, nil, false, false, nil, false, nil, nil, nil, "")
	if expect == "closed" {
		if err == nil {
			t.Fatalf("a run with Blender closed started: %+v", res)
		}
		if !strings.Contains(err.Error(), "does not answer on port 9876") {
			t.Fatalf("the refusal does not say the step to take: %v", err)
		}
		t.Logf("refused before the sandbox: %v", err)
		return
	}
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	t.Logf("run %s: status %s exit %d, folder %s", res.RunID, res.Status, res.ExitCode, res.OutDir)
	logBytes, err := os.ReadFile(filepath.Join(res.ReceiptsDir, "desktop-blender.log"))
	if err != nil {
		t.Fatalf("the run kept no desktop-blender.log: %v", err)
	}
	t.Logf("receipts/desktop-blender.log:\n%s", logBytes)
	env, err := os.ReadFile(filepath.Join(res.OutDir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		CapabilitiesEnforced map[string]string `json:"capabilities_enforced"`
	}
	if err := json.Unmarshal(env, &envelope); err != nil {
		t.Fatal(err)
	}
	line := envelope.CapabilitiesEnforced["desktop.blender"]
	t.Logf("desktop.blender: %s", line)
	t.Logf("desktop.transport: %s", envelope.CapabilitiesEnforced["desktop.transport"])
	if line == "" {
		t.Fatal("the receipt has no desktop.blender line")
	}
	if len(y.link.Runs()) != 0 {
		t.Fatalf("a run is still bound after the run: %+v", y.link.Runs())
	}
	mu.Lock()
	n := len(asked)
	mu.Unlock()
	t.Logf("the exec dialog was raised %d times", n)
	for _, rr := range y.link.Recent() {
		if rr.Run == res.RunID {
			b, _ := json.Marshal(rr)
			t.Logf("Settings' record of the run: %s", b)
		}
	}
	// What Blender holds now, asked of its add-on directly.
	t.Logf("Blender's scene after the run: %s", blenderScene(t))
}

// blenderScene asks the add-on on port 9876 for the scene's objects.
func blenderScene(t *testing.T) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", "127.0.0.1:9876", 3*time.Second)
	if err != nil {
		return "(the add-on does not answer: " + err.Error() + ")"
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(20 * time.Second))
	_, _ = c.Write([]byte(`{"type":"get_scene_info","params":{}}`))
	var buf strings.Builder
	sc := bufio.NewReader(c)
	tmp := make([]byte, 64<<10)
	for {
		n, err := sc.Read(tmp)
		buf.Write(tmp[:n])
		if json.Valid([]byte(buf.String())) || err != nil {
			break
		}
	}
	s := buf.String()
	if len(s) > 2000 {
		s = s[:2000] + "…"
	}
	return s
}
