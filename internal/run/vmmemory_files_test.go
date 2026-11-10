package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"

	"github.com/blaktron/agentry-desktop/internal/windowsengine"
)

// .wslconfig's memory is raised under [wsl2], kept when already as large,
// added to a [wsl2] that has none or to a file with no [wsl2], and every
// other line, comment and line ending is kept.
func TestTheWSLConfigsMemoryIsRaisedOnly(t *testing.T) {
	for _, c := range []struct {
		name, in, want string
		changed        bool
	}{
		{"no file", "", "[wsl2]\r\nmemory=8GB\r\n", true},
		{"no wsl2 section", "[experimental]\r\nautoMemoryReclaim=gradual\r\n", "[experimental]\r\nautoMemoryReclaim=gradual\r\n\r\n[wsl2]\r\nmemory=8GB\r\n", true},
		{"wsl2 without memory", "[wsl2]\r\nprocessors=4\r\n", "[wsl2]\r\nmemory=8GB\r\nprocessors=4\r\n", true},
		{"lower", "# mine\r\n[wsl2]\r\nmemory=4GB # small\r\nswap=0\r\n", "# mine\r\n[wsl2]\r\nmemory=8GB\r\nswap=0\r\n", true},
		{"lower in MB", "[wsl2]\nmemory=2048MB\n", "[wsl2]\nmemory=8GB\n", true},
		{"higher kept", "[wsl2]\r\nmemory=16GB\r\n", "[wsl2]\r\nmemory=16GB\r\n", false},
		{"equal kept", "[WSL2]\r\nMemory = 8GB\r\n", "[WSL2]\r\nMemory = 8GB\r\n", false},
		{"unreadable replaced", "[wsl2]\r\nmemory=lots\r\n", "[wsl2]\r\nmemory=8GB\r\n", true},
		{"other section's memory untouched", "[other]\r\nmemory=1GB\r\n[wsl2]\r\nmemory=2GB\r\n", "[other]\r\nmemory=1GB\r\n[wsl2]\r\nmemory=8GB\r\n", true},
		{"commented memory not read", "[wsl2]\r\n# memory=32GB\r\n", "[wsl2]\r\nmemory=8GB\r\n# memory=32GB\r\n", true},
	} {
		got, changed := raiseWSLMemory(c.in, 8)
		if got != c.want || changed != c.changed {
			t.Errorf("%s:\n got %q (%v)\nwant %q (%v)", c.name, got, changed, c.want, c.changed)
		}
	}
}

// Docker Desktop's memoryMiB is raised only, every other setting kept, and
// a file with no memoryMiB or with a byte-order mark is handled.
func TestDockerDesktopsMemoryIsRaisedOnly(t *testing.T) {
	in := []byte("\xef\xbb\xbf{\n  \"cpus\": 4,\n  \"memoryMiB\": 4096,\n  \"wslEngineEnabled\": true\n}\n")
	out, changed, err := raiseDockerDesktopMemory(in, 14*1024)
	if err != nil || !changed {
		t.Fatalf("raise: %v %v", changed, err)
	}
	s := string(out)
	if strings.HasPrefix(s, "\xef\xbb\xbf") || !strings.Contains(s, `"memoryMiB": 14336`) || !strings.Contains(s, `"cpus": 4`) || !strings.Contains(s, `"wslEngineEnabled": true`) {
		t.Fatalf("the settings after the raise:\n%s", s)
	}
	if _, changed, err = raiseDockerDesktopMemory(out, 8192); err != nil || changed {
		t.Fatalf("a lower size changed it: %v %v", changed, err)
	}
	if _, _, err = raiseDockerDesktopMemory([]byte(`{"cpus": 4}`), 8192); err != errNoMemoryKey {
		t.Fatalf("no memoryMiB: %v", err)
	}
	if _, _, err = raiseDockerDesktopMemory([]byte(`not json`), 8192); err == nil {
		t.Fatal("a broken file was accepted")
	}
}

// fakeDocker puts a docker command that records its arguments, one call a
// line, where the engine's binary is looked for.
func fakeDocker(t *testing.T) (calls string) {
	t.Helper()
	bin, calls := fakeCommand(t, "docker")
	t.Setenv(dockerx.EnvDocker, bin)
	return calls
}

// Docker Desktop is quit, its memoryMiB raised and started again; a file
// already as large is left alone and Docker Desktop is not touched.
func TestDockerDesktopIsQuitSizedAndStarted(t *testing.T) {
	path := scratchDockerDesktopSettings(t)
	if err := os.WriteFile(path, []byte(`{"cpus": 4, "memoryMiB": 4096}`), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := fakeDocker(t)
	answering(t, true)
	if !dockerDesktopSizable() {
		t.Fatal("a settings file with memoryMiB is not sizable")
	}
	if err := ResizeVM(context.Background(), VM{Kind: VMDockerDesktop, Name: "Docker Desktop", Resizable: true}, 8, 3, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if got := lines(t, calls); strings.Join(got, "|") != "desktop stop|desktop start" {
		t.Fatalf("docker was run with %q", got)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), `"memoryMiB": 8192`) || !strings.Contains(string(b), `"cpus": 4`) {
		t.Fatalf("the settings after the resize:\n%s", b)
	}
	_ = os.Remove(calls)
	if err := ResizeVM(context.Background(), VM{Kind: VMDockerDesktop, Name: "Docker Desktop", Resizable: true}, 6, 3, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(calls); err == nil {
		t.Fatal("Docker Desktop was restarted for a size it already had")
	}
}

// WSL's memory: Docker Desktop is quit, .wslconfig raised, WSL shut down and
// Docker Desktop started again, in that order.
func TestWSLIsShutDownBetweenTheQuitAndTheStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.WriteFile(filepath.Join(home, ".wslconfig"), []byte("[wsl2]\r\nmemory=4GB\r\nswap=0\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := fakeDocker(t)
	wsl, wslCalls := fakeCommand(t, "wsl.exe")
	t.Setenv("PATH", filepath.Dir(wsl)+string(os.PathListSeparator)+os.Getenv("PATH"))
	old := wslSettle
	wslSettle = time.Millisecond
	t.Cleanup(func() { wslSettle = old })
	answering(t, true)
	if err := ResizeVM(context.Background(), VM{Kind: VMWSL, Name: "Docker Desktop", Resizable: true, app: "docker-desktop"}, 14, 7, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if got := lines(t, calls); strings.Join(got, "|") != "desktop stop|desktop start" {
		t.Fatalf("docker was run with %q", got)
	}
	if got := lines(t, wslCalls); len(got) != 1 || got[0] != "--shutdown" {
		t.Fatalf("wsl.exe was run with %q", got)
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".wslconfig")); string(b) != "[wsl2]\r\nmemory=14GB\r\nswap=0\r\n" {
		t.Fatalf(".wslconfig after the resize: %q", b)
	}
}

// On Windows, Rancher Desktop answering rdctl is WSL; else Docker Desktop is
// WSL unless its settings turn the WSL 2 engine off, when its own machine
// is set in its settings; with neither, the machine is named and not sized.
func TestWindowsEnginesAreNamedByTheirMachine(t *testing.T) {
	path := scratchDockerDesktopSettings(t)
	old := windowsInstaller
	windowsInstaller = func() *windowsengine.Installer { return nil }
	t.Cleanup(func() { windowsInstaller = old })
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if v := windowsVM(context.Background()); v.Kind != VMOther || v.Resizable {
		t.Errorf("neither installed: %+v", v)
	}
	for _, c := range []struct {
		settings  string
		kind      string
		resizable bool
	}{
		{`{"wslEngineEnabled": true, "memoryMiB": 2048}`, VMWSL, true},
		{`{"memoryMiB": 2048}`, VMWSL, true},
		{`{"wslEngineEnabled": false, "memoryMiB": 2048}`, VMDockerDesktop, false},
	} {
		if err := os.WriteFile(path, []byte(c.settings), 0o600); err != nil {
			t.Fatal(err)
		}
		if v := windowsVM(context.Background()); v.Kind != c.kind || v.Resizable != c.resizable || v.Name != "Docker Desktop" {
			t.Errorf("%s: %+v", c.settings, v)
		}
	}
	if n := restartNote(VM{Kind: VMWSL, Name: "Docker Desktop"}); !strings.Contains(n, "every WSL distribution") {
		t.Errorf("the WSL question does not say what stops: %q", n)
	}
}

// scratchDockerDesktopSettings points the home and the application data
// folder at a temporary one, and returns where Docker Desktop's settings
// file is there for this system, its folder made.
func scratchDockerDesktopSettings(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	path := dockerDesktopSettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
