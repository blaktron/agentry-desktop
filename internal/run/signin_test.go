package run

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/claudesignin"
	"github.com/blaktron/agentry-cli/pkg/runner/connection"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/harnessbuild"
	"github.com/blaktron/agentry-cli/pkg/runner/modelsignin"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
)

// claudeSubscriptionRun is the run fixture under a model setup paid by a
// Claude subscription, on a fake daemon where the Claude Code image is
// already built from its pinned artifact by this build's recipe.
func claudeSubscriptionRun(t *testing.T) (string, *dockerx.Fake, string) {
	t.Helper()
	_, regURL, fd, home := runFixture(t)
	old := connectionKeys
	connectionKeys = func() connection.Keychain { return nil }
	t.Cleanup(func() { connectionKeys = old })
	pol := cataloguedPolicy()
	pol.Model = policy.Model{Provider: policy.ProviderAnthropic, Name: "claude-sonnet-4-5", APIKeyKind: policy.KeyKindClaudeSignIn,
		ProxyAllow: []string{"api.anthropic.com:443", "platform.claude.com:443"}}
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	h, _ := pol.Images.HarnessImageFor("claude-image")
	fd.ExistingImages[h.Ref] = true
	if fd.Labels == nil {
		fd.Labels = map[string]map[string]string{}
	}
	fd.Labels[h.Ref] = map[string]string{harnessbuild.LabelArtifact: h.Artifact, harnessbuild.LabelRecipe: harnessbuild.ClaudeRecipe}
	return regURL, fd, home
}

// A run started from the desktop on a Claude subscription runs on the
// Claude Code image, not docker-agent (the desktop always ran docker-agent
// before agentry-desktop#734), and is handed the person's sign-in: its
// agent container mounts the sign-in's own volume.
func TestARunOnAClaudeSubscriptionUsesClaudeCodeAndItsSignIn(t *testing.T) {
	regURL, fd, home := claudeSubscriptionRun(t)
	installLocalFixture(t, home)
	if _, err := modelsignin.Open(home, nil).Put(modelsignin.Claude, modelsignin.Record{ConfigDir: claudesignin.Volume, Label: "person@example.com"}, modelsignin.Grant{}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	toolsServerStarts(fd, req.OutDir)
	var log strings.Builder
	res, err := Run(context.Background(), req, &log)
	t.Logf("result %+v", res)
	if r := receipt.ReasonOf(err); r != nil && r.Code == receipt.ReasonSignInMissing {
		t.Fatalf("refused as signed out: %v", err)
	}
	agent, ok := fd.FindRun("-agent")
	if !ok {
		t.Fatalf("no agent container (err %v); ops %v\n%s", err, fd.Ops(), log.String())
	}
	pol := policy.Default()
	h, _ := pol.Images.HarnessImageFor("claude-image")
	if agent.Image != h.Ref {
		t.Fatalf("the agent ran %s, not the Claude Code image %s", agent.Image, h.Ref)
	}
	if !slices.ContainsFunc(agent.Mounts, func(m dockerx.Mount) bool { return m.Source == claudesignin.Volume }) {
		t.Fatalf("the agent does not mount the sign-in's volume: %+v", agent.Mounts)
	}
	for k := range agent.Env {
		if strings.Contains(k, "API_KEY") {
			t.Fatalf("a sign-in run's agent was given %s", k)
		}
	}
}

// Signed out, the run stops before any container or image work, with the
// receipt reason that says so.
func TestARunOnAModelSignInWithoutOneStopsBeforeDocker(t *testing.T) {
	regURL, fd, home := claudeSubscriptionRun(t)
	installLocalFixture(t, home)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	_, err := Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &strings.Builder{})
	if r := receipt.ReasonOf(err); r == nil || r.Code != receipt.ReasonSignInMissing {
		t.Fatalf("err = %v", err)
	}
	for _, op := range fd.Ops() {
		if strings.HasPrefix(op, "Pull:") || strings.HasPrefix(op, "Build:") || strings.HasPrefix(op, "RunForeground:") || strings.HasPrefix(op, "RunDetached:") {
			t.Fatalf("docker was asked to %s for a run that could not start", op)
		}
	}
}
