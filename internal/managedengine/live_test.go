package managedengine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// The live check, on a Mac, with the real publishers: the three pinned
// files are fetched and verified, the engine's VM starts under a temporary
// engine directory, the engine's own docker command answers through the
// socket and runs a container that reads a file the Mac shares, the VM
// stops and Remove leaves nothing. It downloads about 80 MB and boots a
// virtual machine, so it runs only when asked:
//
//	AGENTRY_LIVE_MANAGED_ENGINE=1 go test ./internal/managedengine -run TestLiveManagedEngineOnThisMac -v -timeout 20m
//
// By default it runs where the app runs the engine (the application
// support directory, the VM's state in ~/.agentry-vm), so the paths the
// app uses are the ones proved, and it refuses to start where either is
// already there: the person's own engine is never removed by a test.
// AGENTRY_LIVE_MANAGED_ENGINE_DIR names another engine directory instead
// (under the home directory, which the VM shares), with the VM's state
// under it; everything it makes is removed at the end.
func TestLiveManagedEngineOnThisMac(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_MANAGED_ENGINE") == "" {
		t.Skip("set AGENTRY_LIVE_MANAGED_ENGINE=1 to fetch the engine and boot its VM on this Mac")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("the managed engine runs on macOS")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	e, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("AGENTRY_LIVE_MANAGED_ENGINE_DIR"); dir != "" {
		e = &Engine{Dir: dir}
	}
	for _, d := range []string{e.Dir, e.vmDir()} {
		if _, err := os.Lstat(d); err == nil {
			t.Fatalf("%s is already there; remove the app's engine first, or name another directory with AGENTRY_LIVE_MANAGED_ENGINE_DIR", d)
		}
	}
	e.Ping, e.Log = dockerx.PingSocket, os.Stderr
	t.Logf("engine directory %s, VM directory %s", e.Dir, e.vmDir())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		_ = e.Remove(ctx)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	start := time.Now()
	if err := e.Install(ctx); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !e.Installed() || !e.Current() {
		t.Fatal("not installed and current after the install")
	}
	t.Logf("installed in %s under %s", time.Since(start).Round(time.Second), e.Dir)

	start = time.Now()
	if err := e.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Logf("started and answering in %s at %s", time.Since(start).Round(time.Second), e.Socket())
	// Lima's home is the one the engine names, not Colima's default under
	// its own home.
	if _, err := os.Stat(filepath.Join(e.limaHome(), limaInstance)); err != nil {
		t.Errorf("Lima's instance is not under %s: %v", e.limaHome(), err)
	}
	if _, err := os.Stat(filepath.Join(e.colimaHome(), "_lima")); err == nil {
		t.Errorf("Lima's home landed under Colima's, %s", filepath.Join(e.colimaHome(), "_lima"))
	}
	if !e.Running(ctx) {
		t.Error("Running false after the start")
	}

	// The engine's own docker command, by endpoint alone and with the
	// engine's own docker configuration: the person's context and their
	// config.json (whose credential helper may be Docker Desktop's) are
	// not consulted and not changed.
	before := dockerContext(t)
	personal := readFile(t, filepath.Join(home, ".docker", "config.json"))
	docker := func(args ...string) string {
		cmd := exec.CommandContext(ctx, e.Docker(), args...) //nolint:gosec // the engine's own command
		cmd.Env = append(os.Environ(), "DOCKER_HOST="+e.Endpoint(), "DOCKER_CONFIG="+e.DockerConfigDir())
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	t.Logf("server: %s", docker("version", "--format", "{{.Server.Version}} ({{.Server.Os}}/{{.Server.Arch}})"))
	info := docker("info", "--format", "{{.Name}} {{.KernelVersion}} {{.OperatingSystem}}")
	t.Logf("daemon: %s", info)
	// A file under the home directory is read by a container: the share
	// that runs depend on.
	marker := filepath.Join(e.Dir, "shared.txt")
	if err := os.WriteFile(marker, []byte("shared from the Mac\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := lastLine(docker("run", "--rm", "-v", e.Dir+":/shared:ro", "alpine:3.22", "cat", "/shared/shared.txt"))
	if got != "shared from the Mac" {
		t.Errorf("the container read %q", got)
	}
	// Inside the VM the daemon's socket is /var/run/docker.sock, which the
	// run's docker filter container mounts.
	got = lastLine(docker("run", "--rm", "-v", "/var/run/docker.sock:/var/run/docker.sock", "alpine:3.22", "sh", "-c", "test -S /var/run/docker.sock && echo socket"))
	if got != "socket" {
		t.Errorf("the VM's docker socket: %q", got)
	}
	if after := dockerContext(t); after != before {
		t.Errorf("the person's docker context changed: %q -> %q", before, after)
	}
	if now := readFile(t, filepath.Join(home, ".docker", "config.json")); now != personal {
		t.Errorf("the person's docker config.json changed")
	}
	if _, err := os.Stat(filepath.Join(home, ".docker", "contexts", "meta")); err == nil {
		out, _ := exec.Command("grep", "-rl", "colima", filepath.Join(home, ".docker", "contexts")).Output()
		if strings.TrimSpace(string(out)) != "" {
			t.Errorf("a colima context landed in the person's docker contexts:\n%s", out)
		}
	}

	if err := e.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if e.Running(ctx) {
		t.Error("Running true after the stop")
	}
	cachesBefore := colimaCaches(home)
	if err := e.Remove(ctx); err != nil {
		t.Fatalf("remove: %v", err)
	}
	for _, d := range []string{e.Dir, e.vmDir()} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("%s is still there: %v", d, err)
		}
	}
	_ = os.Remove(filepath.Dir(e.Dir))
	// Nothing of Colima's landed in the person's own caches or homes.
	if after := colimaCaches(home); after != cachesBefore {
		t.Errorf("colima's caches or homes changed: %q -> %q", cachesBefore, after)
	}
	// Nothing of Lima's host agents is left running for this engine.
	out, _ := exec.Command("pgrep", "-fl", "limactl").Output()
	if strings.Contains(string(out), e.Dir) || strings.Contains(string(out), e.vmDir()) {
		t.Errorf("a Lima host agent of this engine outlived Remove:\n%s", out)
	}
}

// dockerContext is the person's current docker context name, "" without a
// docker CLI on PATH.
func dockerContext(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("docker")
	if err != nil {
		return ""
	}
	out, err := exec.Command(path, "context", "show").Output() //nolint:gosec // the person's own docker, read only
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// readFile is a file's content, "" when it is not there.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // a path under the test's home
	if err != nil {
		return ""
	}
	return string(b)
}

// lastLine is the last non-empty line of a command's output: a pull's
// progress comes before what the container printed.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// colimaCaches says which of Colima's and Lima's own directories exist
// under the home directory.
func colimaCaches(home string) string {
	var out []string
	for _, p := range []string{".colima", ".lima", ".cache/colima", "Library/Caches/colima", ".cache/lima", "Library/Caches/lima"} {
		if _, err := os.Stat(filepath.Join(home, p)); err == nil {
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}
