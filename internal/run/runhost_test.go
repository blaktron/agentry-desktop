package run

import (
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry/pkg/manifest"
)

// A run from the app carries what a run setup gives, and a release that needs
// more is refused before anything is sent, naming what.
func TestARunhostRunRefusesWhatTheAppDoesNotCarry(t *testing.T) {
	ok := &manifest.Manifest{Intake: []manifest.IntakeQuestion{{ID: "topic"}, {ID: "pick", Type: manifest.IntakeChoice, Choices: []string{"a"}}, {ID: "key", Type: manifest.IntakeSecret}}}
	if got := RunhostCannotCarry(ok); got != "" {
		t.Fatalf("a release the app carries was refused: %s", got)
	}
	for want, m := range map[string]*manifest.Manifest{
		"a file answer":      {Intake: []manifest.IntakeQuestion{{ID: "doc", Type: manifest.IntakeFile}}},
		"a target host":      {Intake: []manifest.IntakeQuestion{{ID: "server", Type: manifest.IntakeHost}}},
		"a repository input": {Intake: []manifest.IntakeQuestion{{ID: "repo", Type: manifest.IntakeRepository}}},
	} {
		if got := RunhostCannotCarry(m); got != want {
			t.Errorf("%s: %q", want, got)
		}
	}
	m := &manifest.Manifest{}
	m.Capabilities.MCPServers = []manifest.MCPServer{{URL: "https://mcp.example.com", Auth: manifest.AuthOAuth}}
	if got := RunhostCannotCarry(m); !strings.Contains(got, "connection") {
		t.Fatalf("a connection: %q", got)
	}
}

// The job carries the intake's answers, a secret answer as the secret it
// names, and the declared secrets from the policy's source; a required
// secret this computer has none of refuses the run.
func TestARunhostRunCarriesTheAnswersAndTheSecrets(t *testing.T) {
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	home, _ := Home()
	sig := &Signature{Namespace: "acme", Name: "demo", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("a", 64), Ref: "acme/demo@1.0.0"}
	m := &manifest.Manifest{Intake: []manifest.IntakeQuestion{{ID: "topic"}, {ID: "api_key", Type: manifest.IntakeSecret}}}
	m.Capabilities.Secrets = []string{"API_KEY", "OTHER_TOKEN"}
	pol := policy.Default()
	t.Setenv("OTHER_TOKEN", "from-the-environment")
	pol.SecretSource = policy.SecretSource{Mode: "env"}
	run, err := runhostCarry(home, sig, m, pol, RunhostRequest{Intake: map[string]string{"topic": "birds", "api_key": "sk-answered"}})
	if err != nil {
		t.Fatal(err)
	}
	if run.Intake["topic"] != "birds" || run.Intake["api_key"] != "" {
		t.Fatalf("intake %v", run.Intake)
	}
	if run.Secrets["API_KEY"] != "sk-answered" || run.Secrets["OTHER_TOKEN"] != "from-the-environment" {
		t.Fatalf("secrets %v", run.Secrets)
	}
	t.Setenv("OTHER_TOKEN", "")
	if _, err := runhostCarry(home, sig, m, pol, RunhostRequest{Intake: map[string]string{"api_key": "sk"}}); err == nil || !strings.Contains(err.Error(), "OTHER_TOKEN") {
		t.Fatalf("a missing secret: %v", err)
	}
}
