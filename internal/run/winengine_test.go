package run

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"

	"github.com/blaktron/agentry-desktop/internal/windowsengine"
)

// The card's state on Windows: an engine answering wins; Rancher Desktop
// installed and silent is stopped; a restart owed after WSL's install says
// so; the offer where the build can install and Docker Desktop is not
// installed (that one is started, not replaced); otherwise the hand-off.
func TestTheWindowsEngineStates(t *testing.T) {
	for _, tc := range []struct {
		ok   bool
		w    WindowsEngine
		want string
	}{
		{true, WindowsEngine{Supported: true, RancherInstalled: true}, StateReady},
		{true, WindowsEngine{}, StateReady},
		{false, WindowsEngine{Supported: true, RancherInstalled: true}, StateWindowsStopped},
		{false, WindowsEngine{Supported: true, WSL: "restartPending"}, StateWindowsRestart},
		{false, WindowsEngine{Supported: true, WSL: "missing"}, StateWindowsOffer},
		{false, WindowsEngine{Supported: true, WSL: "ready"}, StateWindowsOffer},
		{false, WindowsEngine{Supported: true, WSL: "ready", DockerDesktopInstalled: true}, StateManual},
		{false, WindowsEngine{}, StateManual},
	} {
		if got := windowsState(tc.ok, tc.w); got != tc.want {
			t.Errorf("windowsState(%v, %+v) = %s, want %s", tc.ok, tc.w, got, tc.want)
		}
	}
}

// windowsRig is an installer under a temporary directory whose commands
// are shell stand-ins that record their command line and exit with the
// code exits gives "<name> <first argument>"; installer files are served by
// a local server with the pinned sum replaced by the served body's.
func windowsRig(t *testing.T, exits map[string]int) (*windowsengine.Installer, func() []string) {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	body := []byte("rancher desktop's installer")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(srv.Close)
	sum := sha512.Sum512(body)
	boot := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	i := &windowsengine.Installer{
		StateDir: filepath.Join(dir, "state"), LocalAppData: filepath.Join(dir, "LocalAppData"), ProgramFiles: filepath.Join(dir, "ProgramFiles"),
		URL: srv.URL + "/" + windowsengine.MSIName, SHA512: hex.EncodeToString(sum[:]),
		BootTime: func() (time.Time, error) { return boot, nil },
	}
	i.Command = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		base := filepath.Base(name)
		first := ""
		if len(args) > 0 {
			first = args[0]
		}
		line := strings.TrimSpace(base + " " + strings.Join(args, " "))
		if f, err := os.OpenFile(calls, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil { //nolint:gosec // the test's own file
			_, _ = fmt.Fprintln(f, line)
			_ = f.Close()
		}
		script := ":"
		// rdctl's install leaves its commands where Rancher Desktop puts them.
		if base == "msiexec.exe" && exits[base+" "+first] == 0 {
			bin := filepath.Join(i.LocalAppData, "Programs", "Rancher Desktop", "resources", "resources", "win32", "bin")
			script += fmt.Sprintf("; mkdir -p %q && : > %q", bin, filepath.Join(bin, "rdctl.exe"))
		}
		return exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("%s; exit %d", script, exits[base+" "+first]))
	}
	saved := windowsInstaller
	windowsInstaller = func() *windowsengine.Installer { return i }
	t.Cleanup(func() { windowsInstaller = saved })
	read := func() []string {
		b, _ := os.ReadFile(calls)
		return strings.Split(strings.TrimSpace(string(b)), "\n")
	}
	return i, read
}

// From a computer with no engine and no WSL: the first click installs WSL
// (Windows' own consent prompt) and says a restart is owed, which the card
// then shows; after the restart the second click downloads, installs for
// this user, starts Rancher Desktop with its dockerd engine and waits for
// detection to find it; the app then starts it when it opens.
func TestInstallingTheWindowsEngineTakesWSLThenARestartThenRancherDesktop(t *testing.T) {
	withEngines(t, engineFixture{})
	savedDD := dockerDesktopInstalled
	dockerDesktopInstalled = func() bool { return false }
	t.Cleanup(func() { dockerDesktopInstalled = savedDD })
	i, calls := windowsRig(t, map[string]int{"wsl.exe --version": 1, "wsl.exe --status": 50})
	ctx := context.Background()

	if got := windowsState(false, windowsStory(ctx)); got != StateWindowsOffer {
		t.Fatalf("before: %s", got)
	}
	if err := InstallWindowsEngine(ctx, io.Discard); !errors.Is(err, ErrWindowsRestartNeeded) {
		t.Fatalf("the first click: %v", err)
	}
	if got := calls(); got[len(got)-1] != "powershell.exe -NoProfile -NonInteractive -Command "+windowsengine.InstallWSLScript {
		t.Fatalf("ran %q", got)
	}
	if got := windowsState(false, windowsStory(ctx)); got != StateWindowsRestart {
		t.Fatalf("after WSL's install: %s", got)
	}
	// A click before the restart does nothing but say it again.
	if err := InstallWindowsEngine(ctx, io.Discard); !errors.Is(err, ErrWindowsRestartNeeded) {
		t.Fatalf("a click before the restart: %v", err)
	}

	// Windows restarted; WSL answers now.
	i.BootTime = func() (time.Time, error) { return time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC), nil }
	i2, calls2 := windowsRig(t, map[string]int{"wsl.exe --version": 0})
	i2.StateDir, i2.BootTime = i.StateDir, i.BootTime
	savedDetect := windowsDetect
	windowsDetect = func(context.Context) Engine {
		for _, c := range calls2() {
			if strings.HasPrefix(c, "rdctl.exe start") {
				return Engine{Found: true, How: "the docker CLI's context"}
			}
		}
		return Engine{}
	}
	t.Cleanup(func() { windowsDetect = savedDetect })
	if err := InstallWindowsEngine(ctx, io.Discard); err != nil {
		t.Fatal(err)
	}
	got := calls2()
	if len(got) != 3 || !strings.HasPrefix(got[1], "msiexec.exe /i "+filepath.Join(i2.StateDir, windowsengine.MSIName)) ||
		got[2] != "rdctl.exe "+strings.Join(windowsengine.FirstStartArgs(), " ") {
		t.Fatalf("ran %q", got)
	}
	if !strings.Contains(got[1], "MSIINSTALLPERUSER=1") {
		t.Errorf("not a per-user install: %q", got[1])
	}
	if !i2.InstalledByApp() {
		t.Fatal("the app does not know it installed Rancher Desktop")
	}
	if w := windowsStory(ctx); !w.RancherInstalled || !w.InstalledByApp || windowsState(false, w) != StateWindowsStopped {
		t.Fatalf("installed and silent reads as %+v (%s)", w, windowsState(false, w))
	}
}

// An installer that does not match the pinned sum installs nothing.
func TestAWindowsInstallerWithAnotherSumIsNotRun(t *testing.T) {
	withEngines(t, engineFixture{})
	i, calls := windowsRig(t, map[string]int{"wsl.exe --version": 0})
	i.SHA512 = strings.Repeat("0", 128)
	if err := InstallWindowsEngine(context.Background(), io.Discard); err == nil || !strings.Contains(err.Error(), "SHA-512") {
		t.Fatalf("got %v", err)
	}
	for _, c := range calls() {
		if strings.HasPrefix(c, "msiexec") || strings.HasPrefix(c, "rdctl") {
			t.Fatalf("ran %q after a sum mismatch", c)
		}
	}
}

// At launch the app starts Rancher Desktop only where it installed it, no
// engine answers and none is named in its environment.
func TestTheAppStartsRancherDesktopAtLaunchOnlyWhereItInstalledIt(t *testing.T) {
	withEngines(t, engineFixture{})
	i, calls := windowsRig(t, map[string]int{"rdctl.exe api": 1})
	savedWait, savedDetect := windowsReadyWait, windowsDetect
	windowsReadyWait = 10 * time.Millisecond
	windowsDetect = func(context.Context) Engine { return Engine{} }
	t.Cleanup(func() { windowsReadyWait, windowsDetect = savedWait, savedDetect })

	StartWindowsEngineAtLaunch(context.Background(), io.Discard)
	if c := calls(); len(c) != 1 || c[0] != "" {
		t.Fatalf("started with nothing installed: %q", c)
	}
	// Installed by the person: left to them.
	bin := filepath.Join(i.LocalAppData, "Programs", "Rancher Desktop", "resources", "resources", "win32", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "rdctl.exe"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	StartWindowsEngineAtLaunch(context.Background(), io.Discard)
	if c := calls(); len(c) != 1 || c[0] != "" {
		t.Fatalf("started the person's own install: %q", c)
	}
	// Installed by the app and not running: given its provisioning script,
	// then started in the background with its saved settings.
	if err := os.MkdirAll(i.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(i.StateDir, "installed.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	StartWindowsEngineAtLaunch(context.Background(), io.Discard)
	if c := calls(); len(c) != 2 || c[0] != "rdctl.exe api /v1/backend_state" || c[1] != "rdctl.exe "+strings.Join(windowsengine.StartArgs(), " ") {
		t.Fatalf("ran %q", c)
	}
	if b, _ := os.ReadFile(i.ProvisionPath()); string(b) != windowsengine.HealthcheckFix {
		t.Fatalf("the provisioning script is %q", b)
	}
}

// installedByTheApp makes the rig's Rancher Desktop one the app installed.
func installedByTheApp(t *testing.T, i *windowsengine.Installer) {
	t.Helper()
	bin := filepath.Join(i.LocalAppData, "Programs", "Rancher Desktop", "resources", "resources", "win32", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "rdctl.exe"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(i.StateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(i.StateDir, "installed.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A Rancher Desktop that runs while its engine answers nothing is quit and
// started again by Start: rdctl start alone would only apply settings to
// it, and its provisioning script takes effect only at a start.
func TestStartRestartsARunningRancherDesktopWhoseEngineIsSilent(t *testing.T) {
	withEngines(t, engineFixture{})
	i, calls := windowsRig(t, map[string]int{})
	installedByTheApp(t, i)
	savedWait, savedDetect := windowsReadyWait, windowsDetect
	windowsReadyWait = time.Minute
	windowsDetect = func(context.Context) Engine {
		for _, c := range calls() {
			if strings.HasPrefix(c, "rdctl.exe start") {
				return Engine{Found: true}
			}
		}
		return Engine{}
	}
	t.Cleanup(func() { windowsReadyWait, windowsDetect = savedWait, savedDetect })

	if err := StartWindowsEngine(context.Background(), io.Discard); err != nil {
		t.Fatal(err)
	}
	want := []string{"rdctl.exe api /v1/backend_state", "rdctl.exe shutdown", "rdctl.exe " + strings.Join(windowsengine.StartArgs(), " ")}
	if got := calls(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("ran %q, want %q", got, want)
	}
	if b, _ := os.ReadFile(i.ProvisionPath()); string(b) != windowsengine.HealthcheckFix {
		t.Fatalf("the provisioning script is %q", b)
	}
}

// At launch, an engine that answers is left alone once its provisioning
// script is in place; without the script it is restarted once, since it
// would stop answering about a minute after its start.
func TestAtLaunchAnAnsweringRancherDesktopIsRestartedOnlyToAddItsScript(t *testing.T) {
	withEngines(t, engineFixture{docker: &contextDocker{Fake: &dockerx.Fake{}}, onPath: []string{"docker"}})
	i, calls := windowsRig(t, map[string]int{})
	installedByTheApp(t, i)
	savedWait, savedDetect := windowsReadyWait, windowsDetect
	windowsReadyWait = time.Minute
	windowsDetect = func(context.Context) Engine { return Engine{Found: true} }
	t.Cleanup(func() { windowsReadyWait, windowsDetect = savedWait, savedDetect })

	StartWindowsEngineAtLaunch(context.Background(), io.Discard)
	want := []string{"rdctl.exe api /v1/backend_state", "rdctl.exe shutdown", "rdctl.exe " + strings.Join(windowsengine.StartArgs(), " ")}
	if got := calls(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("without its script: ran %q, want %q", got, want)
	}
	StartWindowsEngineAtLaunch(context.Background(), io.Discard)
	if got := calls(); len(got) != len(want) {
		t.Fatalf("with its script: ran %q", got[len(want):])
	}
}
