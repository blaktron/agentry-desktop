//go:build unix

package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
	"github.com/blaktron/agentry-cli/pkg/runner/compile"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
)

// TestRunTakesTheHostRelayLock is desktop#258's acceptance, as the engine
// has narrowed it: the desktop passes the runner home it shares with the
// CLI as engine.Params.RunnerHome, where the engine takes
// <runner home>/relays.lock for a relay run that shares the host's one
// relay network (agentry-cli#268). Behind the daemon filter a run's relays
// are on networks of its own and no lock is taken: on Linux since
// agentry-cli#220 (DR-22 narrowed, the CLI's v0.6.0), and on macOS since the
// filter runs there as a container of the run's (agentry-cli#322, v0.6.117;
// agentry-desktop#878). So on every platform this test runs on, a held lock
// refuses nothing and the run makes its own two relay networks. The lock is
// held here the way the CLI's engine holds it, flock on the same file; the
// desktop's own params come from prepare, the path Run and StartSession
// both take.
func TestRunTakesTheHostRelayLock(t *testing.T) {
	_, regURL, fd, home := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")

	var log strings.Builder
	p, err := prepare(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &log, manifest.ModeAutonomous)
	if err != nil {
		t.Fatalf("prepare: %v\nlog:\n%s", err, log.String())
	}
	defer finishProfile(&log, p.prof, p.profDir, p.keep)
	if p.params.RunnerHome != home {
		t.Fatalf("engine.Params.RunnerHome = %q, want the shared runner home %q", p.params.RunnerHome, home)
	}

	// Give the prepared run a declared server that reaches its host through
	// a relay (egress declared), compiled the way ensurePlan compiles.
	srv := spec.MCPServerSpec{
		Name: "mcp-npm-scope-server-x", Source: "npm", Package: "@scope/server-x", Version: "1.2.3",
		Image: "node:22-alpine", Command: []string{"/opt/agentry-mcp/npm-scope-server-x/node_modules/.bin/server-x"},
		StoreSubdir: "npm-scope-server-x", Egress: []string{"api.example"},
	}
	planDir := t.TempDir()
	plan, err := compile.Compile(compile.Inputs{
		Manifest: p.m, Mode: manifest.ModeAutonomous, Digest: p.plan.Digest, Source: "registry",
		WorkspaceDir: p.params.WorkspaceSrc, Policy: p.params.Policy, PolicySHA: p.plan.PolicySHA256,
		AdapterName: adapter.DefaultName, RunnerImage: runnerImageTag(p.params.Policy),
		MCPServers: []spec.MCPServerSpec{srv},
	}, planDir)
	if err != nil {
		t.Fatal(err)
	}
	p.params.Plan, p.params.PlanDir, p.params.ServersDir = plan, planDir, t.TempDir()

	lock, err := os.OpenFile(filepath.Join(home, engine.RelayLockFile), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	_, err = engine.Run(context.Background(), p.docker, p.params)
	if err != nil {
		t.Fatalf("behind the docker filter a relay run's networks are its own (agentry-cli#220, #322), so a held %s must refuse nothing, got %v", engine.RelayLockFile, err)
	}
	var own []string
	for _, op := range fd.Ops() {
		if strings.HasPrefix(op, "NetworkCreate") && strings.Contains(op, "-relays") {
			own = append(own, op)
		}
	}
	if len(own) < 2 {
		t.Fatalf("a relay run behind the filter makes its own two relay networks (agentry-cli#220); saw %v in %v", own, fd.Ops())
	}
}
