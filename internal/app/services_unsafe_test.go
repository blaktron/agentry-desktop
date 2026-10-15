package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/session"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Unsafe Mode: a run In Agentry carries nothing of it, the page cannot turn
// it on, and every consent that existed still asks.

// unsafeApp is a RunAPI under a settings file and a runner home of the
// test's own, whose dialog answers as the test says and records what it
// was asked, and whose image pull is the test's.
type unsafeApp struct {
	r      *RunAPI
	asked  []string // each dialog's title and message
	answer bool
	pulls  int
	pull   error
}

func newUnsafeApp(t *testing.T) *unsafeApp {
	t.Helper()
	t.Setenv(configdir.EnvVar, t.TempDir())
	t.Setenv(policy.HomeEnv, t.TempDir())
	u := &unsafeApp{}
	// No webview: the pull's progress lines go nowhere.
	old := emitEvent
	emitEvent = func(*App, string, ...any) {}
	t.Cleanup(func() { emitEvent = old })
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		u.asked = append(u.asked, title+"\n"+message)
		return u.answer, nil
	}}
	u.r = &RunAPI{app: app, runnerHomeDir: t.TempDir(),
		pullUnsafe: func(_ context.Context, logw io.Writer) error {
			u.pulls++
			_, _ = io.WriteString(logw, "downloading\n")
			return u.pull
		}}
	return u
}

// switchOn writes the setting as its one writer does, for a test that
// starts with Unsafe Mode already on.
func switchOn(t *testing.T) {
	t.Helper()
	if err := settings.SetUnsafeMode(true); err != nil {
		t.Fatal(err)
	}
}

func savedUnsafeMode(t *testing.T) bool {
	t.Helper()
	st, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	return st.UnsafeMode
}

// The renderer cannot turn Unsafe Mode on by itself. The binding the page
// calls is a native dialog the page cannot answer, then the image's pull;
// the setting is written only after both, and never into the runner policy.
// In the pattern of TestConsentIsANativeDialogNotAPageFlag.
func TestUnsafeModeIsTurnedOnOnlyThroughTheNativeDialog(t *testing.T) {
	u := newUnsafeApp(t)
	if v, err := (&SettingsAPI{app: u.r.app}).RunDefaults(); err != nil || v.UnsafeMode {
		t.Fatalf("with nothing saved Unsafe Mode reads %+v, %v; want off", v, err)
	}

	// The person says No: refused, nothing pulled, nothing written.
	_, err := u.r.SetUnsafeMode(true)
	if err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatalf("with the dialog answered No: err = %v, want a refusal", err)
	}
	if len(u.asked) != 1 || u.pulls != 0 || savedUnsafeMode(t) {
		t.Fatalf("after a No: dialogs %d, pulls %d, saved %v", len(u.asked), u.pulls, savedUnsafeMode(t))
	}
	// The dialog says what changes, and the image's PATH.
	for _, want := range []string{"Turn on Unsafe Mode?",
		"QuickAgents can run commands this computer has not approved.",
		"Each command is installed from Kali Linux's repository when the QuickAgent first runs.",
		"PyPI", "own PATH", "keeps everything it installed", "run_command still takes only the commands a QuickAgent declares",
		"never to a run In Agentry"} {
		if !strings.Contains(u.asked[0], want) {
			t.Errorf("the confirmation lacks %q:\n%s", want, u.asked[0])
		}
	}

	// No dialog available at all (no window): refuse, never assume yes.
	none := &RunAPI{app: &App{ctx: context.Background()}, pullUnsafe: u.r.pullUnsafe}
	if _, err := none.SetUnsafeMode(true); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("no dialog seam: %v", err)
	}
	if u.pulls != 0 || savedUnsafeMode(t) {
		t.Fatal("with no dialog the image was pulled or the switch written")
	}

	// Yes, but the image does not arrive: the switch stays off.
	u.answer, u.pull = true, errors.New("no route to ghcr.io")
	if _, err := u.r.SetUnsafeMode(true); err == nil || !strings.Contains(err.Error(), "no route") {
		t.Fatalf("a failed pull: err = %v", err)
	}
	if u.pulls != 1 || savedUnsafeMode(t) {
		t.Fatalf("after a failed pull: pulls %d, saved %v; want off", u.pulls, savedUnsafeMode(t))
	}

	// Another docker operation holds the lock: refused before any dialog.
	u.pull = nil
	u.r.mu.Lock()
	before := len(u.asked)
	_, err = u.r.SetUnsafeMode(true)
	u.r.mu.Unlock()
	if !errors.Is(err, errBusy) || len(u.asked) != before || savedUnsafeMode(t) {
		t.Fatalf("while busy: err = %v, dialogs %d→%d, saved %v", err, before, len(u.asked), savedUnsafeMode(t))
	}

	// Yes, and the image arrives: on, in desktop.yaml only.
	v, err := u.r.SetUnsafeMode(true)
	if err != nil {
		t.Fatal(err)
	}
	if !v.UnsafeMode || !savedUnsafeMode(t) || u.pulls != 2 {
		t.Fatalf("after Yes and the pull: view %+v, saved %v, pulls %d", v, savedUnsafeMode(t), u.pulls)
	}
	path, err := settings.Path()
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); !strings.Contains(string(raw), "unsafeMode: true") {
		t.Fatalf("desktop.yaml does not hold the switch:\n%s", raw)
	}
	polPath, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(polPath); err == nil && strings.Contains(strings.ToLower(string(raw)), "unsafemode") {
		t.Fatalf("the runner policy was written with the switch:\n%s", raw)
	}
	if sv, err := settingsView(); err != nil || !sv.UnsafeMode {
		t.Fatalf("the shell's settings view: %+v, %v; want Unsafe Mode on for the glow", sv, err)
	}
	// On already: asking again neither asks nor pulls.
	asked, pulls := len(u.asked), u.pulls
	if _, err := u.r.SetUnsafeMode(true); err != nil || len(u.asked) != asked || u.pulls != pulls {
		t.Fatalf("turning on what is on: err %v, dialogs %d→%d, pulls %d→%d", err, asked, len(u.asked), pulls, u.pulls)
	}

	// Off is immediate: no dialog, no docker, nothing removed.
	v, err = u.r.SetUnsafeMode(false)
	if err != nil || v.UnsafeMode || savedUnsafeMode(t) || len(u.asked) != asked || u.pulls != pulls {
		t.Fatalf("turning it off: view %+v, err %v, dialogs %d→%d, pulls %d→%d", v, err, asked, len(u.asked), pulls, u.pulls)
	}
}

// A run is an Unsafe Mode run only because desktop.yaml says so. No argument
// of RunAgent or StartSession carries it, so whatever the page sends, a run
// with the setting absent is compiled in the default posture.
func TestARunIsUnsafeOnlyFromTheSettingsFile(t *testing.T) {
	u := newUnsafeApp(t)
	var got []run.Request
	u.r.engineRun = func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		got = append(got, req)
		return &run.Result{}, nil
	}
	runIt := func() {
		t.Helper()
		// Every consent flag the page can send, accepted in the dialog. A
		// vulnerable server's consent is refused with the switch off, so it
		// is sent only while the switch is on.
		u.answer = true
		vuln := savedUnsafeMode(t)
		if _, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, map[string]string{"unsafeMode": "true"}, vuln, true, nil, false, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	runIt()
	if len(got) != 1 || got[0].Unsafe {
		t.Fatalf("with the setting absent the run was compiled with Unsafe %v", got[0].Unsafe)
	}
	switchOn(t)
	runIt()
	if len(got) != 2 || !got[1].Unsafe {
		t.Fatal("with the switch on the run was not an Unsafe Mode run")
	}
	if _, err := u.r.SetUnsafeMode(false); err != nil {
		t.Fatal(err)
	}
	runIt()
	if len(got) != 3 || got[2].Unsafe {
		t.Fatal("with the switch turned off again the run was still an Unsafe Mode run")
	}
	// A result names Unsafe Mode only when the receipt's exec line does.
	if v := runResultView(&run.Result{Unsafe: true, Exec: "UNSAFE MODE: run_command restricted to the argv0 allowlist {r2}"}); v.UnsafeExec == "" {
		t.Fatal("an Unsafe Mode run's result does not carry its exec line")
	}
	if v := runResultView(&run.Result{Exec: "run_command restricted to the argv0 allowlist {echo}"}); v.UnsafeExec != "" {
		t.Fatalf("a default-posture run's result names Unsafe Mode: %q", v.UnsafeExec)
	}
}

// With Unsafe Mode on, every consent that exists still asks, in its own
// dialog, and a No still refuses (a vulnerable server's consent exists only
// there). The dialog carries one more line naming Unsafe Mode and the
// commands its image cannot provide, and Unsafe Mode adds no dialog to a run
// that had none.
func TestEveryConsentStillAsksInUnsafeMode(t *testing.T) {
	u := newUnsafeApp(t)
	switchOn(t)
	var noticed []string
	u.r.unsafeNotice = func(_ context.Context, reg, ref string) (*run.UnsafeNotice, error) {
		noticed = append(noticed, reg+" "+ref)
		return &run.UnsafeNotice{Commands: []string{"analyzeHeadless", "r2", "zzz"}, Unplaced: []string{"zzz"}}, nil
	}
	ran := 0
	u.r.engineRun = func(context.Context, run.Request, io.Writer) (*run.Result, error) { ran++; return &run.Result{}, nil }

	for name, flags := range map[string][3]bool{
		"a release with no attestation": {true, false, false},
		"any declared MCP server the registry's gate rates high or critical, which runs only in Unsafe Mode": {false, true, false},
		"a release the registry admitted without a scan":                                                     {false, false, true},
	} {
		before := len(u.asked)
		_, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, flags[0], nil, flags[1], flags[2], nil, false, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "not confirmed") {
			t.Errorf("%s, in Unsafe Mode, with the dialog answered No: err = %v, want the refusal", name, err)
		}
		if len(u.asked) != before+1 {
			t.Fatalf("%s, in Unsafe Mode: dialogs raised = %d, want one", name, len(u.asked)-before)
		}
		msg := u.asked[len(u.asked)-1]
		for _, want := range []string{"This run accepts " + name, "The receipt will record the consent.",
			"Unsafe Mode is on", "As resolved, it declares: analyzeHeadless, r2, zzz.", "Not available in Unsafe Mode's image: zzz. The run goes on without it.", "Continue?"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s: the dialog lacks %q:\n%s", name, want, msg)
			}
		}
	}
	if ran != 0 {
		t.Fatalf("a refused consent reached the engine %d time(s)", ran)
	}
	if len(noticed) != 3 || noticed[0] != "https://r.example acme/x@1.0.0" {
		t.Fatalf("the notice was read for %q", noticed)
	}

	// A session asks the same way.
	before := len(u.asked)
	if _, err := u.r.StartSession("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, true, nil, false, false, nil, false, nil, nil); err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatalf("StartSession unsigned, in Unsafe Mode: err = %v", err)
	}
	if len(u.asked) != before+1 || !strings.Contains(u.asked[before], "Unsafe Mode is on") {
		t.Fatalf("StartSession's dialog: %q", u.asked[before:])
	}

	// A run with nothing to accept raises no dialog, Unsafe Mode or not:
	// turning it on was the confirmation ("no extra dialog per run").
	before = len(u.asked)
	if _, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, false, nil, false, false, nil, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(u.asked) != before || ran != 1 {
		t.Fatalf("a consent-free run in Unsafe Mode: dialogs %d, runs %d", len(u.asked)-before, ran)
	}

	// A release that cannot be read here still gets a dialog that names
	// Unsafe Mode; and with the switch off the dialog is the one it was.
	u.r.unsafeNotice = func(context.Context, string, string) (*run.UnsafeNotice, error) {
		return nil, errors.New("registry unreachable")
	}
	_, _ = u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, nil, false, false, nil, false, nil, nil)
	if msg := u.asked[len(u.asked)-1]; !strings.Contains(msg, "Unsafe Mode is on") {
		t.Fatalf("with the release unreadable the dialog does not name Unsafe Mode:\n%s", msg)
	}
	if _, err := u.r.SetUnsafeMode(false); err != nil {
		t.Fatal(err)
	}
	_, _ = u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, nil, false, false, nil, false, nil, nil)
	if msg := u.asked[len(u.asked)-1]; msg != "Run acme/x@1.0.0 anyway?\nThis run accepts a release with no attestation. The receipt will record the consent. Continue?" {
		t.Fatalf("with Unsafe Mode off the dialog changed:\n%s", msg)
	}
}

// In Unsafe Mode a declared server the gate rates high runs on the person's
// Yes to the native dialog: the engine gets the switch and the consent
// together, and decides on the registry's answer itself. A No reaches
// nothing.
func TestUnsafeModeRunsAVulnerableServerOnTheDialogsYes(t *testing.T) {
	u := newUnsafeApp(t)
	switchOn(t)
	u.r.unsafeNotice = func(context.Context, string, string) (*run.UnsafeNotice, error) { return &run.UnsafeNotice{}, nil }
	var got []run.Request
	u.r.engineRun = func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		got = append(got, req)
		return &run.Result{}, nil
	}
	if _, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, false, nil, true, false, nil, false, nil, nil); err == nil || len(got) != 0 {
		t.Fatalf("the dialog's No: err = %v, engine reached %d time(s)", err, len(got))
	}
	msg := u.asked[len(u.asked)-1]
	for _, want := range []string{"any declared MCP server the registry's gate rates high or critical, which runs only in Unsafe Mode", "Unsafe Mode is on"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the dialog lacks %q:\n%s", want, msg)
		}
	}
	// A chat asks the same, and a No starts nothing.
	before := len(u.asked)
	if _, err := u.r.StartSession("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, false, nil, true, false, nil, false, nil, nil); err == nil || !strings.Contains(err.Error(), "not confirmed") || len(u.asked) != before+1 {
		t.Fatalf("StartSession's No in Unsafe Mode: err = %v, dialogs %d", err, len(u.asked)-before)
	}
	u.answer = true
	if _, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, false, nil, true, false, nil, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Unsafe || !got[0].AllowVulnerableMCP {
		t.Fatalf("the engine's request after a Yes = %+v, want Unsafe and the consent", got)
	}
}

// The Review step's line: nothing while the switch is off, and the run's
// own line while it is on.
func TestTheReviewStepsNoticeIsEmptyWhileTheSwitchIsOff(t *testing.T) {
	u := newUnsafeApp(t)
	asked := 0
	u.r.unsafeNotice = func(context.Context, string, string) (*run.UnsafeNotice, error) {
		asked++
		return &run.UnsafeNotice{Commands: []string{"r2", "zzz"}, Unplaced: []string{"zzz"}}, nil
	}
	v, err := u.r.UnsafeNotice("https://r.example", "acme/x@1.0.0")
	if err != nil || v.On || v.Line != "" || asked != 0 {
		t.Fatalf("with the switch off: %+v, %v, release read %d time(s)", v, err, asked)
	}
	switchOn(t)
	v, err = u.r.UnsafeNotice("https://r.example", "acme/x@1.0.0")
	if err != nil || !v.On || !strings.Contains(v.Line, "Unsafe Mode is on") || len(v.Unplaced) != 1 || v.Unplaced[0] != "zzz" {
		t.Fatalf("with the switch on: %+v, %v", v, err)
	}
	// A release that cannot be read here still says Unsafe Mode is on.
	u.r.unsafeNotice = func(context.Context, string, string) (*run.UnsafeNotice, error) {
		return nil, errors.New("registry unreachable")
	}
	v, err = u.r.UnsafeNotice("https://r.example", "acme/x@1.0.0")
	if err != nil || !v.On || !strings.HasPrefix(v.Line, "Unsafe Mode is on") || strings.Contains(v.Line, "registry unreachable") {
		t.Fatalf("with the release unreadable: %+v, %v", v, err)
	}
}

// No other setting's save can write the switch. Every other binding loads
// the file, changes a field and saves, with no lock across the three, so a
// save that loaded the switch on before it was turned off would otherwise
// turn it back on with no dialog; and a Settings value built by hand with
// the switch set writes nothing of it.
func TestOnlySetUnsafeModeWritesTheSwitch(t *testing.T) {
	u := newUnsafeApp(t)
	s := &SettingsAPI{app: u.r.app}

	// A save cannot turn it on.
	st, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.UnsafeMode = true
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	if savedUnsafeMode(t) {
		t.Fatal("settings.Save wrote unsafeMode: true")
	}

	// A save that loaded it on, landing after it was turned off, leaves it off.
	switchOn(t)
	stale, err := settings.Load()
	if err != nil || !stale.UnsafeMode {
		t.Fatalf("the fixture: %+v, %v", stale, err)
	}
	if _, err := u.r.SetUnsafeMode(false); err != nil {
		t.Fatal(err)
	}
	stale.RunWhere = settings.RunWhereAgentry
	if err := settings.Save(stale); err != nil {
		t.Fatal(err)
	}
	if savedUnsafeMode(t) {
		t.Fatal("a stale save turned Unsafe Mode back on with no dialog")
	}
	if st, _ := settings.Load(); st.RunWhere != settings.RunWhereAgentry {
		t.Fatalf("the stale save lost its own change: %+v", st)
	}

	// And a save keeps it on while it is on: the other bindings do not turn
	// it off by accident either.
	switchOn(t)
	if _, err := s.SetRunWhere(settings.RunWhereHere); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkOnboarded(); err != nil {
		t.Fatal(err)
	}
	if !savedUnsafeMode(t) {
		t.Fatal("another setting's save turned Unsafe Mode off")
	}
	if len(u.asked) != 0 || u.pulls != 0 {
		t.Fatalf("dialogs %d, pulls %d", len(u.asked), u.pulls)
	}
}

// The consent dialog and the compile see one reading of the switch: turned
// on while a run's consent dialog is up, it is not an Unsafe Mode run,
// because its dialog carried no line saying so.
func TestTheSwitchIsReadOncePerRun(t *testing.T) {
	u := newUnsafeApp(t)
	var got []run.Request
	u.r.engineRun = func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		got = append(got, req)
		return &run.Result{}, nil
	}
	u.r.app.confirm = func(_ context.Context, title, message string) (bool, error) {
		u.asked = append(u.asked, title+"\n"+message)
		// While the person reads the dialog, the switch goes on.
		if err := settings.SetUnsafeMode(true); err != nil {
			t.Fatal(err)
		}
		return true, nil
	}
	if _, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, nil, false, false, nil, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(u.asked) != 1 || strings.Contains(u.asked[0], "Unsafe Mode") {
		t.Fatalf("the dialog: %q", u.asked)
	}
	if len(got) != 1 || got[0].Unsafe {
		t.Fatal("a run whose dialog did not name Unsafe Mode was compiled in it")
	}
}

// Unsafe Mode never reaches a run In Agentry. With the switch on, a hosted
// run's requests carry nothing of it, no local engine or docker is touched,
// and nothing about Unsafe Mode is asked or read.
func TestARunInAgentryCarriesNothingOfUnsafeMode(t *testing.T) {
	f := &hostedFake{enabled: true, outputs: map[string]string{"result.json": hostedEnvelope}}
	upstream, err := url.Parse(f.start(t))
	if err != nil {
		t.Fatal(err)
	}
	// Everything the app sends to the registry, whole.
	var mu sync.Mutex
	var sent []string
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		var hdr strings.Builder
		_ = r.Header.Write(&hdr)
		mu.Lock()
		sent = append(sent, r.Method+" "+r.URL.String()+"\n"+hdr.String()+"\n"+string(body))
		mu.Unlock()
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	reg := srv.URL

	signIn(t, reg)
	switchOn(t)
	r, events := newRunAPI(t)
	fastPoll(t)
	dialogs, notices, engines := 0, 0, 0
	r.app.confirm = func(context.Context, string, string) (bool, error) { dialogs++; return true, nil }
	r.unsafeNotice = func(context.Context, string, string) (*run.UnsafeNotice, error) {
		notices++
		return &run.UnsafeNotice{}, nil
	}
	r.engineRun = func(context.Context, run.Request, io.Writer) (*run.Result, error) {
		engines++
		return &run.Result{}, nil
	}
	r.pullUnsafe = func(context.Context, io.Writer) error { engines++; return nil }

	outDir := t.TempDir()
	r.app.markPicked(outDir)
	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "summarise INC-4821", nil, outDir, nil, nil, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if start.Refusal != "" || start.Run == nil {
		t.Fatalf("start = %+v", start)
	}
	waitForStatus(t, events, hostedrun.StatusQueued)
	f.finish(hostedrun.OutcomeSucceeded, "ok", 2)
	final := waitForFinal(t, events)
	if final.Outcome != hostedrun.OutcomeSucceeded {
		t.Fatalf("final = %+v", final)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(sent) == 0 {
		t.Fatal("the hosted run sent nothing through the recording registry")
	}
	created := false
	for _, req := range sent {
		if strings.Contains(strings.ToLower(req), "unsafe") {
			t.Errorf("a request of a run In Agentry names Unsafe Mode:\n%s", req)
		}
		created = created || strings.HasPrefix(req, "POST ") && strings.Contains(req, "summarise INC-4821")
	}
	if !created {
		t.Fatalf("the run's create request was not among the %d recorded", len(sent))
	}
	if dialogs != 0 || notices != 0 || engines != 0 {
		t.Fatalf("a run In Agentry with Unsafe Mode on: dialogs %d, notices read %d, local engine or pull %d; want none", dialogs, notices, engines)
	}
	if final.CapabilitiesEnforced["exec"] != "" && strings.Contains(final.CapabilitiesEnforced["exec"], "UNSAFE MODE") {
		t.Fatalf("the hosted receipt names Unsafe Mode: %q", final.CapabilitiesEnforced["exec"])
	}
	if !savedUnsafeMode(t) {
		t.Fatal("the fixture lost the switch: the test proved nothing")
	}
	// Every start In Agentry (a run, a session, a draft, a knowledge build)
	// is one CreateRequest, and it has nowhere to carry the switch.
	rt := reflect.TypeOf(hostedrun.CreateRequest{})
	for i := range rt.NumField() {
		if f := rt.Field(i); strings.Contains(strings.ToLower(f.Name+f.Tag.Get("json")), "unsafe") {
			t.Errorf("hostedrun.CreateRequest carries %s", f.Name)
		}
	}
}

// unsafeChat is a chat whose End is all a test needs: it ends at once.
type unsafeChat struct{ events chan session.Event }

func (c *unsafeChat) Ref() string                                    { return "acme/x@1.0.0" }
func (c *unsafeChat) Events() <-chan session.Event                   { return c.events }
func (c *unsafeChat) Send(context.Context, string) error             { return nil }
func (c *unsafeChat) AddInput(context.Context, string) error         { return nil }
func (c *unsafeChat) End(context.Context, bool) (*run.Result, error) { return &run.Result{}, nil }

// liveEvents records every runner:unsafe-live the app emits.
func liveEvents(t *testing.T) *[]bool {
	t.Helper()
	// Emitted synchronously, under the API's unsafeMu, so the test's
	// reads need no lock of their own.
	var got []bool
	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if name == eventUnsafeLive {
			got = append(got, data[0].(bool))
		}
	}
	t.Cleanup(func() { emitEvent = old })
	return &got
}

// Turning the switch off does not take the red edge and the badge from a run
// or a chat compiled in Unsafe Mode while it is live. They go when it ends.
// A run the switch was on for, whose plan Unsafe Mode did not change (no
// declared commands), is not one.
func TestTheEdgeStaysWhileARunCompiledInUnsafeModeIsLive(t *testing.T) {
	u := newUnsafeApp(t)
	u.answer = true
	events := liveEvents(t)
	switchOn(t)
	var during []bool
	compiledUnsafe := true
	u.r.engineRun = func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		req.PlanCompiled(compiledUnsafe)
		during = append(during, u.r.UnsafeLive())
		// Mid-run, the person turns the switch off.
		if _, err := u.r.SetUnsafeMode(false); err != nil {
			t.Fatal(err)
		}
		during = append(during, u.r.UnsafeLive())
		return &run.Result{Unsafe: compiledUnsafe}, nil
	}
	if _, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, nil, false, false, nil, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(during, []bool{true, true}) {
		t.Fatalf("while the Unsafe Mode run was live, UnsafeLive read %v, want true before and after the switch went off", during)
	}
	if u.r.UnsafeLive() {
		t.Fatal("UnsafeLive still true after the run ended")
	}
	if !reflect.DeepEqual(*events, []bool{true, false}) {
		t.Fatalf("runner:unsafe-live events %v, want [true false]", *events)
	}

	// The switch on, but the plan not compiled in Unsafe Mode.
	switchOn(t)
	*events, during, compiledUnsafe = nil, nil, false
	if _, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, nil, false, false, nil, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	// Marked from its start, the switch having been on, and cleared at its
	// compile, which Unsafe Mode did not change.
	if !reflect.DeepEqual(during, []bool{false, false}) || !reflect.DeepEqual(*events, []bool{true, false}) {
		t.Fatalf("a run Unsafe Mode did not change: UnsafeLive %v, events %v", during, *events)
	}

	// A run with the switch off never raises it.
	*events, during, compiledUnsafe = nil, nil, false
	if _, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, nil, false, false, nil, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(*events) != 0 {
		t.Fatalf("a run with the switch off: events %v", *events)
	}

	// A run that fails after its compile still takes the edge down.
	switchOn(t)
	*events = nil
	u.r.engineRun = func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		req.PlanCompiled(true)
		return nil, errors.New("the gateway did not start")
	}
	if _, err := u.r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, "", nil, 0, 0, true, nil, false, false, nil, false, nil, nil); err == nil {
		t.Fatal("the failing run did not fail")
	}
	if u.r.UnsafeLive() || !reflect.DeepEqual(*events, []bool{true, false}) {
		t.Fatalf("a failed Unsafe Mode run: UnsafeLive %v, events %v", u.r.UnsafeLive(), *events)
	}
}

// The same for a chat, which can be live for hours: the edge stays from
// its compile until EndSession.
func TestTheEdgeStaysWhileAChatCompiledInUnsafeModeIsLive(t *testing.T) {
	u := newUnsafeApp(t)
	u.answer = true
	events := liveEvents(t)
	switchOn(t)
	u.r.engineSession = func(_ context.Context, req run.Request, _ io.Writer) (localSession, error) {
		req.PlanCompiled(true)
		return &unsafeChat{events: make(chan session.Event)}, nil
	}
	if _, err := u.r.StartSession("https://r.example", "acme/x@1.0.0", "", nil, "", nil, -1, true, nil, false, false, nil, false, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := u.r.SetUnsafeMode(false); err != nil {
		t.Fatal(err)
	}
	if !u.r.UnsafeLive() {
		t.Fatal("the chat compiled in Unsafe Mode lost the edge when the switch went off")
	}
	close(u.r.session.(*unsafeChat).events)
	if _, err := u.r.EndSession(false); err != nil {
		t.Fatal(err)
	}
	if u.r.UnsafeLive() {
		t.Fatal("UnsafeLive still true after the chat ended")
	}
	if !reflect.DeepEqual(*events, []bool{true, false}) {
		t.Fatalf("runner:unsafe-live events %v, want [true false]", *events)
	}
}
