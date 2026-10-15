package app

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// The availability view names what a release declares, so the panel can say
// before a start what it will ask for.
func TestHostedAvailabilityNamesTheDeclaredSecrets(t *testing.T) {
	f := &hostedFake{enabled: true, secrets: []string{"GITHUB_TOKEN", "SEARCH_KEY"}, extensions: []string{"billing", "runs", "vault"},
		scopes: []string{"agent:read", "user:profile", "run:read", "run:write", "secret:write"}}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !v.Available || strings.Join(v.DeclaredSecrets, ",") != "GITHUB_TOKEN,SEARCH_KEY" {
		t.Fatalf("view = %+v", v)
	}
}

// captureLogs sends the standard and slog loggers to a buffer for the
// test's life: whatever this process logs while a secret travels.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	oldOut, oldFlags := log.Writer(), log.Flags()
	oldSlog := slog.Default()
	log.SetOutput(&buf)
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		log.SetOutput(oldOut)
		log.SetFlags(oldFlags)
		slog.SetDefault(oldSlog)
	})
	return &buf
}

// SetHostedSecret is a call of its own, as the model key's is: the value
// reaches the vault once, with keep as a literal boolean, and is in no log
// line, no event the panel receives and no error — whatever the registry
// answers.
func TestSetHostedSecretIsItsOwnCallAndLogsNothing(t *testing.T) {
	const value = "ghp_desktop-do-not-leak-300"
	logs := captureLogs(t)
	f := &hostedFake{enabled: true}
	reg := f.start(t)
	signIn(t, reg)
	r, events := newRunAPI(t)

	if err := r.SetHostedSecret(reg, "GITHUB_TOKEN", value, false); err != nil {
		t.Fatal(err)
	}
	if err := r.SetHostedSecret(reg, "SEARCH_KEY", value, true); err != nil {
		t.Fatal(err)
	}
	for i, keep := range []bool{false, true} {
		var body map[string]any
		if err := json.Unmarshal([]byte(f.vaultSets[i].Body), &body); err != nil {
			t.Fatal(err)
		}
		if body["value"] != value || body["keep"] != keep || len(body) != 2 {
			t.Fatalf("set %d sent %v (want value and a literal keep %v)", i, body, keep)
		}
	}

	// A registry that refuses, even one that echoes the body back.
	f.mu.Lock()
	f.vaultAnswer = func(w http.ResponseWriter, body []byte) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "https://agentry.dev/problems/secret-kept", "title": "The secret is kept", "detail": "echo " + string(body)})
	}
	f.mu.Unlock()
	err := r.SetHostedSecret(reg, "GITHUB_TOKEN", value, false)
	if err == nil {
		t.Fatal("a refused set was not an error")
	}
	if strings.Contains(err.Error(), value) || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("the error = %q", err)
	}
	if strings.Contains(logs.String(), value) {
		t.Fatalf("the value was logged:\n%s", logs.String())
	}
	for _, l := range events.lines() {
		if strings.Contains(l, value) {
			t.Fatalf("an event carried the value: %q", l)
		}
	}
}

// A secret set for a start that then does not start is deleted again — the
// ones held for one run, never one the person kept — and a start that does
// start leaves them for the vault to remove when the run ends (the site does
// the same).
func TestAStartThatDoesNotStartDeletesItsHeldSecrets(t *testing.T) {
	f := &hostedFake{enabled: true, refuse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"type":"https://agentry.dev/problems/credits-exhausted","title":"Not enough credits for this run","status":402}`))
	}}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	fastPoll(t)
	if err := r.SetHostedSecret(reg, "GITHUB_TOKEN", "held", false); err != nil {
		t.Fatal(err)
	}
	if err := r.SetHostedSecret(reg, "SEARCH_KEY", "kept", true); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	r.app.markPicked(out)
	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, nil, nil, false, false, nil)
	if err != nil || start.Refusal != "credits-exhausted" {
		t.Fatalf("start = %+v, %v", start, err)
	}
	if strings.Join(f.vaultDeletes, ",") != "GITHUB_TOKEN" {
		t.Fatalf("deleted %v, want the held GITHUB_TOKEN alone", f.vaultDeletes)
	}

	// Set again, and this time the run starts: nothing is deleted.
	f.mu.Lock()
	f.refuse, f.vaultDeletes = nil, nil
	f.mu.Unlock()
	if err := r.SetHostedSecret(reg, "GITHUB_TOKEN", "held", false); err != nil {
		t.Fatal(err)
	}
	out2 := t.TempDir()
	r.app.markPicked(out2)
	if start, err = r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out2, nil, nil, false, false, nil); err != nil || start.Run == nil {
		t.Fatalf("start = %+v, %v", start, err)
	}
	if len(f.vaultDeletes) != 0 {
		t.Fatalf("a start that started deleted %v", f.vaultDeletes)
	}
	// Nor does a later discard: the run holds it now.
	r.DiscardHostedSecrets(reg)
	if len(f.vaultDeletes) != 0 {
		t.Fatalf("a discard after the start deleted %v", f.vaultDeletes)
	}
}

// secret-missing reaches the panel with the names to ask for.
func TestStartHostedRunReportsTheMissingSecrets(t *testing.T) {
	f := &hostedFake{enabled: true, refuse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"type":"https://agentry.dev/problems/secret-missing","title":"A declared secret is missing","status":409,"missing":["GITHUB_TOKEN"],"secrets":["GITHUB_TOKEN"]}`))
	}}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	out := t.TempDir()
	r.app.markPicked(out)
	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, nil, nil, false, false, nil)
	if err != nil || start.Refusal != "secret-missing" || strings.Join(start.Missing, ",") != "GITHUB_TOKEN" {
		t.Fatalf("start = %+v, %v", start, err)
	}
}

// The token goes to no URL Settings would refuse: plain http to a host off
// this machine is refused before any token is looked up.
func TestSetHostedSecretHoldsTheURLRule(t *testing.T) {
	signIn(t, "https://registry.example")
	r, _ := newRunAPI(t)
	for _, call := range []func() error{
		func() error { return r.SetHostedSecret("http://registry.example", "GITHUB_TOKEN", "v", false) },
		func() error { return r.DeleteHostedSecret("http://registry.example", "GITHUB_TOKEN") },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "https") {
			t.Fatalf("plain http off this machine: %v", err)
		}
	}
}

// A create whose answer never came back may have made the run, which holds
// the secrets: they are left to it (or to the vault's sweep), not deleted
// from under it.
func TestACreateWithNoAnswerLeavesTheHeldSecrets(t *testing.T) {
	f := &hostedFake{enabled: true, refuse: func(http.ResponseWriter) { panic(http.ErrAbortHandler) }}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	if err := r.SetHostedSecret(reg, "GITHUB_TOKEN", "held", false); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	r.app.markPicked(out)
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, nil, nil, false, false, nil); err == nil {
		t.Fatal("a create with no answer was not an error")
	}
	if len(f.vaultDeletes) != 0 {
		t.Fatalf("deleted %v under a run that may exist", f.vaultDeletes)
	}
	r.DiscardHostedSecrets(reg)
	if len(f.vaultDeletes) != 0 {
		t.Fatalf("a later discard deleted %v", f.vaultDeletes)
	}
}

// A start asked to ask for the optional keys sends ask_optional_secrets, and
// one not asked sends nothing of it; the registry's secret-missing reaches
// the panel with the names it asks for and the optional ones among them,
// which the ask may leave empty.
func TestAStartAsksForTheOptionalKeysOnlyWhenTold(t *testing.T) {
	f := &hostedFake{enabled: true}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	fastPoll(t)
	out := t.TempDir()
	r.app.markPicked(out)
	f.mu.Lock()
	f.refuse = func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"type":"https://agentry.dev/problems/secret-missing","title":"A secret is missing","status":409,"missing":["OPT_KEY","REQ_KEY"],"secrets":["OPT_KEY","REQ_KEY"],"optional":["OPT_KEY"]}`))
	}
	f.mu.Unlock()
	start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out, nil, nil, false, true, nil)
	if err != nil || start.Refusal != "secret-missing" || strings.Join(start.Missing, ",") != "OPT_KEY,REQ_KEY" || strings.Join(start.Optional, ",") != "OPT_KEY" {
		t.Fatalf("start = %+v, %v", start, err)
	}
	f.mu.Lock()
	asked := len(f.creates) == 1 && strings.Contains(f.creates[0], `"ask_optional_secrets":true`)
	f.refuse = nil
	f.created = nil
	f.mu.Unlock()
	if !asked {
		t.Fatal("an asking start did not send ask_optional_secrets")
	}
	out2 := t.TempDir()
	r.app.markPicked(out2)
	if start, err = r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, out2, nil, nil, false, false, nil); err != nil || start.Run == nil {
		t.Fatalf("the start made again = %+v, %v", start, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.created == nil || len(f.creates) != 2 || strings.Contains(f.creates[1], "ask_optional_secrets") {
		t.Fatalf("the start made again asked for the optional keys: %v", f.creates)
	}
}
