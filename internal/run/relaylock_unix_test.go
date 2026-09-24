//go:build unix

package run

import (
	"context"
	"errors"
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

// TestRunTakesTheHostRelayLock is desktop#258's acceptance: the desktop
// passes the runner home it shares with the CLI as engine.Params.RunnerHome,
// so a desktop run whose plan needs relays is refused (exit 13) while
// another run on the host holds <runner home>/relays.lock, and no gateway
// starts beside it (agentry-cli#268). The lock is held here the way the
// CLI's engine holds it, flock on the same file; the desktop's own params
// come from prepare, the path Run and StartSession both take.
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
	var ce *engine.ClassifiedError
	if !errors.As(err, &ce) || ce.Code != engine.ExitGateway || !strings.Contains(err.Error(), engine.RelayLockFile) {
		t.Fatalf("a relay run while the host relay lock is held must be refused with exit %d naming %s, got %v", engine.ExitGateway, engine.RelayLockFile, err)
	}
	for _, op := range fd.Ops() {
		if strings.HasPrefix(op, "RunDetached:") && strings.Contains(op, "gateway") {
			t.Fatalf("a gateway started while another run held the relay lock: %v", fd.Ops())
		}
	}
}
