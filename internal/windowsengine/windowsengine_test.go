package windowsengine

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// TestHelperProcess stands in for wsl.exe, powershell.exe, msiexec.exe and
// rdctl.exe: it records its command line and exits with the code the test
// gave the command's name, copying the file the test named to the one
// named after /l*v (msiexec's log).
func TestHelperProcess(t *testing.T) {
	if os.Getenv("WINDOWSENGINE_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	args = args[1:]
	name := filepath.Base(args[0])
	f, _ := os.OpenFile(os.Getenv("WINDOWSENGINE_CALLS"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	_, _ = fmt.Fprintln(f, strings.Join(append([]string{name}, args[1:]...), " "))
	_ = f.Close()
	key := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(name + strings.Join(args[1:2], "")))
	if src := os.Getenv("WINDOWSENGINE_LOG_" + strings.ToUpper(strings.ReplaceAll(name, ".", "_"))); src != "" {
		log, _ := os.ReadFile(src) //nolint:gosec // the test's own file
		for k, a := range args {
			if a == "/l*v" && k+1 < len(args) {
				_ = os.WriteFile(args[k+1], log, 0o600)
			}
		}
	}
	code, _ := strconv.Atoi(os.Getenv("WINDOWSENGINE_EXIT_" + key))
	os.Exit(code)
}

// rig is an installer under temporary directories whose commands are the
// helper above; exits maps a command name plus its first argument
// ("wsl.exe--version") to the code it exits with.
func rig(t *testing.T, exits map[string]int, env ...string) (*Installer, func() []string) {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	i := &Installer{
		StateDir:     filepath.Join(dir, "state"),
		LocalAppData: filepath.Join(dir, "LocalAppData"),
		ProgramFiles: filepath.Join(dir, "ProgramFiles"),
		URL:          MSIURL,
		SHA512:       MSISHA512,
		BootTime:     func() (time.Time, error) { return time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC), nil },
	}
	i.Command = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestHelperProcess$", "--", name}, args...)...) //nolint:gosec // the test binary itself
		cmd.Env = append(os.Environ(), "WINDOWSENGINE_HELPER=1", "WINDOWSENGINE_CALLS="+calls)
		for k, code := range exits {
			key := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(k))
			cmd.Env = append(cmd.Env, fmt.Sprintf("WINDOWSENGINE_EXIT_%s=%d", key, code))
		}
		cmd.Env = append(cmd.Env, env...)
		return cmd
	}
	// The stand-in exits modulo 256, as any POSIX process does; the code
	// Windows would see is the one the test gave.
	was := exitCodeOf
	exitCodeOf = func(err error) int {
		c := was(err)
		for _, code := range exits {
			if code%256 == c && code > 255 {
				return code
			}
		}
		return c
	}
	t.Cleanup(func() { exitCodeOf = was })
	read := func() []string {
		b, _ := os.ReadFile(calls)
		return strings.Split(strings.TrimSpace(string(b)), "\n")
	}
	return i, read
}

func TestTheInstallerIsKeptOnlyWhenItsSHA512IsThePinnedOne(t *testing.T) {
	body := []byte("an installer")
	sum := sha512.Sum512(body)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	i, _ := rig(t, nil)
	i.URL = srv.URL + "/" + MSIName
	i.SHA512 = strings.Repeat("0", 128)
	if _, err := i.Download(context.Background()); err == nil || !strings.Contains(err.Error(), "does not match the SHA-512") {
		t.Fatalf("a file with another sum was kept: %v", err)
	}
	if entries, _ := os.ReadDir(i.StateDir); len(entries) != 0 {
		t.Fatalf("a refused download left %v", entries)
	}

	i.SHA512 = hex.EncodeToString(sum[:])
	path, err := i.Download(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != string(body) || filepath.Base(path) != MSIName {
		t.Fatalf("kept %s: %q", path, b)
	}
	// A copy already there that matches is used without fetching again.
	if _, err := i.Download(context.Background()); err != nil || hits != 2 {
		t.Fatalf("the second download fetched again (hits %d, %v)", hits, err)
	}
}

func TestTheInstallerRunsSilentlyForThisUserAndReadsItsExitCode(t *testing.T) {
	for _, c := range []struct {
		name string
		code int
		log  string
		ok   bool
		want string
	}{
		{"success", 0, "", true, ""},
		{"a restart wanted for something else", 3010, "", true, ""},
		{"another install in progress", 1618, "", false, "another installation is in progress"},
		{"WSL missing", 1603, utf16le("MSI (s): Product: Rancher Desktop -- Rancher Desktop requires Windows Subsystem for Linux 2 (WSL2) to be installed as a prerequisite."), false, ErrWSLMissing.Error()},
		{"any other failure", 1603, "Error 1925", false, "exit 1603"},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := []string{}
			if c.log != "" {
				src := filepath.Join(t.TempDir(), "log")
				if err := os.WriteFile(src, []byte(c.log), 0o600); err != nil {
					t.Fatal(err)
				}
				env = append(env, "WINDOWSENGINE_LOG_MSIEXEC_EXE="+src)
			}
			i, calls := rig(t, map[string]int{"msiexec.exe/i": c.code}, env...)
			if err := os.MkdirAll(i.StateDir, 0o700); err != nil {
				t.Fatal(err)
			}
			msi := filepath.Join(i.StateDir, MSIName)
			err := i.InstallMSI(context.Background(), msi)
			if c.ok != (err == nil) || (err != nil && !strings.Contains(err.Error(), c.want)) {
				t.Fatalf("got %v, want ok=%v %q", err, c.ok, c.want)
			}
			if c.want == ErrWSLMissing.Error() && !errors.Is(err, ErrWSLMissing) {
				t.Fatalf("a refusal for WSL is not ErrWSLMissing: %v", err)
			}
			want := "msiexec.exe " + strings.Join(MSIArgs(msi, filepath.Join(i.StateDir, logFile)), " ")
			if got := calls(); len(got) != 1 || got[0] != want {
				t.Fatalf("ran %q, want %q", got, want)
			}
			_, statErr := os.Stat(filepath.Join(i.StateDir, installedFile))
			if c.ok != (statErr == nil) {
				t.Fatalf("installed.json present=%v after ok=%v", statErr == nil, c.ok)
			}
		})
	}
	if args := MSIArgs("x.msi", "l.log"); !slices.Contains(args, "MSIINSTALLPERUSER=1") || !slices.Contains(args, "/qn") || !slices.Contains(args, "/norestart") {
		t.Fatalf("the installer's arguments %q are not a silent per-user install", args)
	}
}

func TestWSLIsReadyMissingOrOwedARestart(t *testing.T) {
	ctx := context.Background()
	// The Store's WSL answers --version.
	if i, _ := rig(t, map[string]int{"wsl.exe--version": 0}); i.WSLState(ctx) != WSLReady {
		t.Fatal("wsl --version succeeding is not ready")
	}
	// The in-box WSL has no --version but answers --status.
	if i, _ := rig(t, map[string]int{"wsl.exe--version": 1, "wsl.exe--status": 0}); i.WSLState(ctx) != WSLReady {
		t.Fatal("wsl --status succeeding is not ready")
	}
	// Neither: missing (wsl --status exits 50 when WSL is not installed).
	if i, _ := rig(t, map[string]int{"wsl.exe--version": 1, "wsl.exe--status": 50}); i.WSLState(ctx) != WSLMissing {
		t.Fatal("no WSL is not missing")
	}

	// The app installed it in this boot: a restart is owed, whatever wsl
	// says, and nothing is asked of it.
	i, calls := rig(t, map[string]int{"powershell.exe-NoProfile": 0, "wsl.exe--version": 0})
	if err := i.InstallWSL(ctx); err != nil {
		t.Fatal(err)
	}
	if got := calls(); len(got) != 1 || !strings.Contains(got[0], "--install','--no-distribution' -Verb RunAs") {
		t.Fatalf("ran %q", got)
	}
	if i.WSLState(ctx) != WSLRestartPending {
		t.Fatal("no restart owed in the boot WSL was installed in")
	}
	// After a restart the record goes and WSL is asked again.
	i.BootTime = func() (time.Time, error) { return time.Date(2026, 10, 17, 9, 30, 0, 0, time.UTC), nil }
	if i.WSLState(ctx) != WSLReady {
		t.Fatal("still owed a restart after one")
	}
	if _, err := os.Stat(filepath.Join(i.StateDir, restartFile)); !os.IsNotExist(err) {
		t.Fatalf("the restart record stayed: %v", err)
	}
}

func TestADeclinedConsentInstallsNothingAndOwesNoRestart(t *testing.T) {
	i, _ := rig(t, map[string]int{"powershell.exe-NoProfile": 1, "wsl.exe--version": 1, "wsl.exe--status": 50})
	if err := i.InstallWSL(context.Background()); err == nil || !strings.Contains(err.Error(), "consent was declined") {
		t.Fatalf("got %v", err)
	}
	if i.WSLState(context.Background()) != WSLMissing {
		t.Fatal("a declined install reads as anything but missing")
	}
}

func TestRancherDesktopIsFoundWhereverItIsInstalledAndStartedThroughRdctl(t *testing.T) {
	i, calls := rig(t, map[string]int{})
	if i.RancherInstalled() || i.InstalledByApp() {
		t.Fatal("found with nothing installed")
	}
	if want := filepath.Join(i.LocalAppData, "Programs", "Rancher Desktop", binRel, "rdctl.exe"); i.Rdctl() != want {
		t.Fatalf("rdctl %q, want the per-user install's %q", i.Rdctl(), want)
	}
	// A machine-wide install by the person: found, not the app's.
	machine := filepath.Join(i.ProgramFiles, "Rancher Desktop", binRel)
	if err := os.MkdirAll(machine, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"rdctl.exe", "docker.exe"} {
		if err := os.WriteFile(filepath.Join(machine, n), nil, 0o755); err != nil { //nolint:gosec // a stand-in executable
			t.Fatal(err)
		}
	}
	if !i.RancherInstalled() || i.InstalledByApp() || i.Rdctl() != filepath.Join(machine, "rdctl.exe") || i.Docker() != filepath.Join(machine, "docker.exe") {
		t.Fatalf("installed %v, by the app %v, rdctl %q", i.RancherInstalled(), i.InstalledByApp(), i.Rdctl())
	}

	if err := i.Start(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := i.Start(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	got := calls()
	if len(got) != 2 || got[0] != "rdctl.exe "+strings.Join(FirstStartArgs(), " ") || got[1] != "rdctl.exe "+strings.Join(StartArgs(), " ") {
		t.Fatalf("ran %q", got)
	}
	for _, want := range []string{"--container-engine.name=moby", "--kubernetes.enabled=false", "--no-modal-dialogs"} {
		if !slices.Contains(FirstStartArgs(), want) {
			t.Errorf("the first start lacks %s", want)
		}
	}
}

func TestWindowsTextIsReadAsUTF16OrAsIs(t *testing.T) {
	for _, in := range []string{string([]byte{0xFF, 0xFE}) + utf16le("needs WSL"), utf16le("needs WSL"), "needs WSL"} {
		if got := decodeText([]byte(in)); got != "needs WSL" {
			t.Errorf("decodeText(%q) = %q", in, got)
		}
	}
}

func utf16le(s string) string {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 0, 2*len(u))
	for _, r := range u {
		b = append(b, byte(r), byte(r>>8))
	}
	return string(b)
}
