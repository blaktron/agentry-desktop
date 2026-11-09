package codingtools

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// isolate points the home directory, the platform, PATH lookups and the
// environment at the test's own, and swaps the catalog for cat when given.
func isolate(t *testing.T, platform string, onPath map[string]string, env map[string]string, cat ...Tool) string {
	t.Helper()
	home := t.TempDir()
	oldHome, oldGOOS, oldLook, oldEnv, oldCat, oldRun, oldStat := homeDir, goos, lookPath, getenv, catalog, runCommand, stat
	t.Cleanup(func() {
		homeDir, goos, lookPath, getenv, catalog, runCommand, stat = oldHome, oldGOOS, oldLook, oldEnv, oldCat, oldRun, oldStat
	})
	// Only the test's own directories exist: a tool installed on the
	// machine running the tests (Homebrew's claude on a Mac runner) must
	// not be found.
	tmp := os.TempDir()
	stat = func(p string) (os.FileInfo, error) {
		if !strings.HasPrefix(p, tmp) && !strings.HasPrefix(p, home) {
			return nil, os.ErrNotExist
		}
		return os.Stat(p)
	}
	homeDir = func() (string, error) { return home, nil }
	goos = platform
	lookPath = func(name string) (string, error) {
		if p, ok := onPath[name]; ok {
			return p, nil
		}
		return "", exec.ErrNotFound
	}
	getenv = func(k string) string { return env[k] }
	if len(cat) > 0 {
		catalog = cat
	}
	runCommand = func(context.Context, string, []string) ([]byte, int, error) {
		t.Fatal("a test ran a command it did not expect")
		return nil, 0, nil
	}
	return home
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestFindPrefersPATHThenUsualLocations(t *testing.T) {
	tool := Tool{ID: "x", Name: "X", Kind: KindCLI, Commands: []string{"x"}, Locations: Paths{"*": {"~/.x/bin/x"}}}
	isolate(t, "linux", map[string]string{"x": "/usr/bin/x"}, nil)
	if f, ok := Find(tool); !ok || f.Path != "/usr/bin/x" || f.Source != SourcePath {
		t.Fatalf("PATH: got %+v %v", f, ok)
	}

	home := isolate(t, "linux", nil, nil)
	if _, ok := Find(tool); ok {
		t.Fatal("found a tool that is nowhere")
	}
	touch(t, filepath.Join(home, ".x", "bin", "x"))
	if f, ok := Find(tool); !ok || f.Path != filepath.Join(home, ".x", "bin", "x") || f.Source != SourceUsual {
		t.Fatalf("usual location: got %+v %v", f, ok)
	}
}

func TestFindCommandLineToolNeedsAFileButAnAppMayBeABundle(t *testing.T) {
	home := isolate(t, "darwin", nil, nil)
	if err := os.MkdirAll(filepath.Join(home, "Applications", "X.app"), 0o755); err != nil {
		t.Fatal(err)
	}
	cli := Tool{ID: "c", Kind: KindCLI, Locations: Paths{"darwin": {"~/Applications/X.app"}}}
	app := Tool{ID: "a", Kind: KindApp, Locations: Paths{"darwin": {"~/Applications/X.app"}}}
	if _, ok := Find(cli); ok {
		t.Fatal("a directory was taken for a command-line tool")
	}
	if _, ok := Find(app); !ok {
		t.Fatal("an app bundle was not found")
	}
}

func TestLocationsPerPlatformAndVariables(t *testing.T) {
	tool := Tool{ID: "x", Kind: KindApp, Locations: Paths{
		"windows": {"${LOCALAPPDATA}/Programs/x/X.exe"},
		"*":       {"~/never"},
	}}
	local := t.TempDir()
	isolate(t, "windows", nil, map[string]string{})
	touch(t, filepath.Join(local, "Programs", "x", "X.exe"))
	if _, ok := Find(tool); ok {
		t.Fatal("an unset variable was expanded")
	}
	isolate(t, "windows", nil, map[string]string{"LOCALAPPDATA": local})
	if f, ok := Find(tool); !ok || f.Path != filepath.Join(local, "Programs", "x", "X.exe") {
		t.Fatalf("got %+v %v", f, ok)
	}
}

func TestNewestVersionedBuild(t *testing.T) {
	home := isolate(t, "linux", nil, nil)
	dir := filepath.Join(home, ".claude", "remote", "ccd-cli")
	for _, n := range []string{"2.1.9-aaa", "2.1.321-c6c6b56", "2.1.40", "notes.txt"} {
		touch(t, filepath.Join(dir, n))
	}
	p, ok := firstExisting([]string{"~/.claude/remote/ccd-cli/*"}, true)
	if !ok || filepath.Base(p) != "2.1.321-c6c6b56" {
		t.Fatalf("got %q %v", p, ok)
	}
}

func TestPluginMarkers(t *testing.T) {
	home := isolate(t, "linux", nil, nil)
	byDir := Tool{ID: "d", Package: "d", Markers: Paths{"*": {"~/.d/extensions/agentry"}}}
	byText := Tool{ID: "t", Package: "t", Markers: Paths{"*": {"~/.t/installed.json"}}, MarkerText: `"agentry@agentry"`}
	none := Tool{ID: "n", Package: "n"}
	if Plugin(byDir) != PluginAbsent || Plugin(byText) != PluginAbsent {
		t.Fatal("absent plugins read as present")
	}
	if Plugin(none) != PluginUnknown {
		t.Fatal("a tool with no markers must read unknown")
	}
	if err := os.MkdirAll(filepath.Join(home, ".d", "extensions", "agentry"), 0o755); err != nil {
		t.Fatal(err)
	}
	if Plugin(byDir) != PluginInstalled {
		t.Fatal("the extension directory was not read")
	}
	if err := os.MkdirAll(filepath.Join(home, ".t"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(home, ".t", "installed.json"), []byte(`{"plugins":{"other@x":{}}}`), 0o600)
	if Plugin(byText) != PluginAbsent {
		t.Fatal("another plugin was taken for Agentry")
	}
	_ = os.WriteFile(filepath.Join(home, ".t", "installed.json"), []byte(`{"plugins":{"agentry@agentry":{}}}`), 0o600)
	if Plugin(byText) != PluginInstalled {
		t.Fatal("the plugin list was not read")
	}
}

func TestInstallerUsesTheCommandLineToolThenTheAppsBundledBuild(t *testing.T) {
	cli := Tool{ID: "cli", Name: "Cli", Kind: KindCLI, Commands: []string{"cli"}, Steps: [][]string{{"plugin", "add"}}}
	app := Tool{ID: "app", Name: "App", Kind: KindApp, Installer: "cli", Steps: [][]string{{"plugin", "add"}},
		Bundled: Paths{"*": {"~/App/bin/cli"}}}
	isolate(t, "linux", map[string]string{"cli": "/bin/cli"}, nil, cli, app)
	exe, via, err := Installer(app)
	if err != nil || exe != "/bin/cli" || via == nil || via.ID != "cli" {
		t.Fatalf("got %q %+v %v", exe, via, err)
	}

	isolate(t, "linux", nil, nil, cli, app)
	if _, _, err := Installer(app); err == nil || !strings.Contains(err.Error(), "cli command") {
		t.Fatalf("no installer: got %v", err)
	}
	home := isolate(t, "linux", nil, nil, cli, app)
	touch(t, filepath.Join(home, "App", "bin", "cli"))
	exe, via, err = Installer(app)
	if err != nil || exe != filepath.Join(home, "App", "bin", "cli") || via != nil {
		t.Fatalf("bundled: got %q %+v %v", exe, via, err)
	}

	manual := Tool{ID: "m", Name: "M", Kind: KindEditor, Manual: "open it"}
	if _, _, err := Installer(manual); !errors.Is(err, ErrNoInstall) {
		t.Fatalf("manual: got %v", err)
	}
}

func TestPlanInstallRefusesUnknownAndMissingTools(t *testing.T) {
	cli := Tool{ID: "cli", Name: "Cli", Kind: KindCLI, Commands: []string{"cli"}, Steps: [][]string{{"add"}}}
	isolate(t, "linux", nil, nil, cli)
	if _, err := PlanInstall("nope"); err == nil {
		t.Fatal("an unknown tool was planned")
	}
	if _, err := PlanInstall("cli"); err == nil {
		t.Fatal("a tool that is not here was planned")
	}
}

func TestRunRunsEachStepAsArgvAndStopsAtAFailure(t *testing.T) {
	cli := Tool{ID: "cli", Name: "Cli", Kind: KindCLI, Commands: []string{"cli"},
		Steps: [][]string{{"marketplace", "add", "owner/repo"}, {"install", "agentry@agentry"}, {"never"}}}
	isolate(t, "linux", map[string]string{"cli": "/opt/my tools/cli"}, nil, cli)
	var ran [][]string
	runCommand = func(_ context.Context, exe string, argv []string) ([]byte, int, error) {
		if exe != "/opt/my tools/cli" {
			t.Fatalf("ran %q", exe)
		}
		ran = append(ran, argv)
		if argv[0] == "install" {
			return []byte("boom\n"), 2, errors.New("exit status 2")
		}
		return []byte("ok\n"), 0, nil
	}
	p, err := PlanInstall("cli")
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Commands()[0]; got != `"/opt/my tools/cli" marketplace add owner/repo` {
		t.Fatalf("command line: %s", got)
	}
	res, err := Run(context.Background(), p)
	if err == nil || len(res) != 2 || len(ran) != 2 {
		t.Fatalf("got %v %+v %v", err, res, ran)
	}
	if res[1].ExitCode != 2 || res[1].Output != "boom" {
		t.Fatalf("failed step: %+v", res[1])
	}
}

func TestCommandEnvPutsTheToolsDirectoryFirst(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	env := commandEnv(filepath.Join("/opt", "node", "bin", "qwen"))
	var path string
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			path = kv
		}
	}
	if path != "PATH="+filepath.Join("/opt", "node", "bin")+string(os.PathListSeparator)+"/usr/bin" {
		t.Fatalf("got %q", path)
	}
}
