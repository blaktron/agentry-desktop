package run

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/mcpauth"
	"github.com/blaktron/agentry-cli/pkg/mcpauth/mcpauthtest"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/modelsignin"
)

// TestLiveCopilotModels asks the real Copilot CLI image for a plan's
// models through this app's own CopilotModels (agentry-desktop#811), on
// this machine's docker, with a sign-in kept from a fake GitHub: Copilot
// CLI is handed the fake's token over the session, reaches the real
// api.github.com through the run's proxy, and GitHub refuses it, which
// the app says as "sign in again". Without an account that is as far as
// a list goes; the list itself is M11's (agentry-cli#744). It may build
// the runner and Copilot CLI images, so it runs only when asked:
// AGENTRY_LIVE_COPILOT=1 on a host with docker and the network.
func TestLiveCopilotModels(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_COPILOT") != "1" {
		t.Skip("drives the real Copilot CLI image: set AGENTRY_LIVE_COPILOT=1")
	}
	if err := dockerx.New().Available(t.Context()); err != nil {
		t.Skipf("no docker: %v", err)
	}
	signInFixture(t)
	t.Setenv("AGENTRY_GITHUB_APP_CLIENT_ID", mcpauthtest.GitHubClientID)
	v := mcpauthtest.NewGitHub(t)
	old := githubEndpoints
	githubEndpoints = func(f *mcpauth.GitHub) {
		fake := v.Flow()
		present := f.Present
		*f = fake
		f.Present = present
	}
	t.Cleanup(func() { githubEndpoints = old })
	t.Cleanup(modelsignin.RegisterVendor(modelsignin.Copilot, modelsignin.CopilotVendor{Flow: v.Flow()}))
	var b signInPerson
	if _, err := SignInCopilot(context.Background(), b.ui(nil), false); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	models, err := CopilotModels(t.Context(), "individual", &log)
	t.Logf("models %v, err %v\nlog: %s", models, err, log.String())
	if err == nil || !strings.Contains(err.Error(), "sign in again") {
		t.Fatalf("want GitHub to refuse the fake's token, got %v", err)
	}
	noSecret(t, []any{err.Error(), log.String()}, v.AccessPrefix, v.RefreshPrefix)
}
