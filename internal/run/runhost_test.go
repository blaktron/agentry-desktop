package run

import (
	"context"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"
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
	// An application on this computer is carried: the app binds it here and
	// answers the runhost's calls over the link session it keeps open.
	d := &manifest.Manifest{Capabilities: manifest.Capabilities{Desktop: &manifest.Desktop{Servers: []manifest.DesktopServer{{ID: "blender"}}}}}
	if got := RunhostCannotCarry(d); got != "" {
		t.Fatalf("an application on this computer: %q", got)
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
	run, err := runhostCarry(home, sig, m, pol, RunhostRequest{Intake: map[string]string{"topic": "birds", "api_key": "sk-answered"}}, managed.Effective{})
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
	if _, err := runhostCarry(home, sig, m, pol, RunhostRequest{Intake: map[string]string{"api_key": "sk"}}, managed.Effective{}); err == nil || !strings.Contains(err.Error(), "OTHER_TOKEN") {
		t.Fatalf("a missing secret: %v", err)
	}
}

// A runhost run's result is its envelope's, with no plan of this computer's,
// and says Unsafe Mode as the runhost's setting was.
func TestARunhostRunsResultIsItsEnvelopes(t *testing.T) {
	env := &receipt.Envelope{Status: "ok", RunID: "r1", Ref: "acme/demo@1.0.0", Runhost: &receipt.Runhost{Name: "lab", UnsafeMode: true}}
	res := resultFromRunhost(env)
	if res.Status != "ok" || res.RunID != "r1" || !res.Unsafe {
		t.Fatalf("result %+v", res)
	}
	env.Runhost.UnsafeMode = false
	if resultFromRunhost(env).Unsafe {
		t.Fatal("Unsafe Mode off read as on")
	}
}

// A run for a runhost is bound to this app's link service as a run here
// is: with none, or with its refusal, it is refused before anything is
// sent, and the bind names the job's run and the release.
func TestARunhostRunBindsHereFirst(t *testing.T) {
	d := &manifest.Desktop{Servers: []manifest.DesktopServer{{ID: "blender"}}}
	sig := &Signature{Ref: "acme/blend@1.0.0", Digest: "sha256:aa"}
	pol := policy.Default()
	if _, err := bindRunhostDesktop(context.Background(), nil, d, pol, "job1", sig, ""); err == nil || detail(err) != receipt.DesktopUnavailable {
		t.Fatalf("no link service: %v", err)
	}
	refuse := func(context.Context, desktop.BindRequest) (*engine.DesktopLink, error) {
		return nil, &desktop.Refusal{Reason: receipt.DesktopNotInstalled, Server: "blender", Message: "start Blender"}
	}
	if _, err := bindRunhostDesktop(context.Background(), refuse, d, pol, "job1", sig, ""); err == nil || detail(err) != receipt.DesktopNotInstalled {
		t.Fatalf("a refusal: %v", err)
	}
	var got desktop.BindRequest
	bind := func(_ context.Context, req desktop.BindRequest) (*engine.DesktopLink, error) {
		got = req
		return &engine.DesktopLink{Token: "t"}, nil
	}
	link, err := bindRunhostDesktop(context.Background(), bind, d, pol, "job1", sig, "")
	if err != nil || link.Token != "t" || got.Run != "job1" || got.Ref != sig.Ref || len(got.Servers) != 1 || got.Servers[0].ID != "blender" {
		t.Fatalf("bound %+v %v from %+v", link, err, got)
	}
}

// detail is a refusal's receipt detail.
func detail(err error) string {
	if r := receipt.ReasonOf(err); r != nil {
		return r.Detail
	}
	return ""
}

// The job carries where each application's set-up came from and the
// application the person chose, so a runhost's run of an application the
// person added is held to the runhost's policy and named in its receipt as
// a run here is.
func TestTheRunhostJobCarriesWhereEachApplicationCameFrom(t *testing.T) {
	link := &engine.DesktopLink{Token: "t", Servers: map[string][]desktoplink.Tool{"my-cad": {{Name: "export_drawing", Class: "read"}}, "chosen": {{Name: "x", Class: "exec"}}},
		From: map[string]string{"my-cad": desktop.FromPerson, "chosen": desktop.FromLibrary}, Chosen: &desktop.ChosenApplication{ID: "blender", Name: "Blender"}}
	j := jobDesktop("job1", link)
	if j.Run != "job1" || j.Bound.Token != "t" || j.Bound.From["my-cad"] != desktop.FromPerson || j.Bound.Chosen == nil || j.Bound.Chosen.ID != "blender" ||
		j.Bound.Servers["my-cad"][0].Class != "read" {
		t.Fatalf("the job's binding: %+v", j.Bound)
	}
}
