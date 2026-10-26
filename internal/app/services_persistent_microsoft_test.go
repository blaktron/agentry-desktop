package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

const testEntraApp = "11111111-2222-3333-4444-555555555555"

// TestTheMicrosoftSetUpFromTheApp: the Entra app is checked as an id before
// it goes (Softeria's own refused) and recorded with its tenant; the
// signoff is kept and cleared; the sign-in runs the CLI's, which tells the
// page the code and Microsoft's page, opens that page alone, and the view
// reads signed in once the server lists the account; an address that reads
// as an option is refused, and the Coding agent has none of it.
func TestTheMicrosoftSetUpFromTheApp(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	v, err := rig.p.Get("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if v.Microsoft == nil || v.Microsoft.ClientID != "" || v.Microsoft.Account != "" {
		t.Fatalf("a new Assistant's Microsoft set-up: %+v", v.Microsoft)
	}
	for _, bad := range []string{"084a3e9f-a9f4-43f7-89f9-d229cf97853e", "my-app"} {
		if _, err := rig.p.SetMicrosoftClient("assistant", bad, ""); err == nil {
			t.Errorf("%q was taken", bad)
		}
	}
	if v, err = rig.p.SetMicrosoftClient("assistant", testEntraApp, "consumers"); err != nil {
		t.Fatal(err)
	}
	if v.Microsoft.ClientID != testEntraApp || v.Microsoft.Tenant != "consumers" || v.Microsoft.ClientAt == "" ||
		!slices.ContainsFunc(v.MailProviders, func(p PersistentMailProviderView) bool { return p.Key == "microsoft" && p.Enabled }) {
		t.Fatalf("after the app: %+v, %+v", v.Microsoft, v.MailProviders)
	}
	if !slices.ContainsFunc(v.Roster, func(e PersistentRosterEntry) bool {
		return e.ID == "server:ms365" && e.SetUpMicrosoft && e.IconKey == "mail"
	}) {
		t.Errorf("the ms365 card: %+v", v.Roster)
	}
	if v, err = rig.p.SetMicrosoftSignoff("assistant", "Sent by my assistant"); err != nil || v.Microsoft.Signoff != "Sent by my assistant" {
		t.Fatalf("the signoff: %+v, %v", v.Microsoft, err)
	}
	if v, err = rig.p.SetMicrosoftSignoff("assistant", ""); err != nil || v.Microsoft.Signoff != "" {
		t.Fatalf("the signoff cleared: %+v, %v", v.Microsoft, err)
	}

	// The CLI's sign-in against a fake engine: the server prints its code,
	// then lists the account.
	f := &dockerx.Fake{Foreground: map[string]*dockerx.FakeForeground{"image-oneshot": {Handler: func(_ context.Context, s dockerx.RunSpec, _ io.Reader, stdout, _ io.Writer) (int, error) {
		switch {
		case slices.Contains(s.Argv, "--login"):
			_, _ = io.WriteString(stdout, "\nTo sign in, use a web browser to open the page https://microsoft.com/devicelogin and enter the code QWERTY123 to authenticate.\n")
		case slices.Contains(s.Argv, "--list-accounts"):
			_, _ = io.WriteString(stdout, `[{"username":"ana@outlook.com"}]`)
		}
		return 0, nil
	}}}}
	keys := &memSecrets{m: map[string]string{}}
	rig.p.microsoftSetUp = func(_ context.Context, a persistent.Agent) (persistent.Microsoft, error) {
		return persistent.Microsoft{Docker: f, Home: rig.home, Agent: a, Policy: policy.Default(), RunnerImage: "agentry-runner:test", Keys: keys, Client: persistent.ClientDesktop}, nil
	}
	var opened []string
	rig.p.browse = func(u string) error { opened = append(opened, u); return nil }
	var codes []PersistentMicrosoftSignInView
	var mu sync.Mutex
	old := emitEvent
	emitEvent = func(a *App, name string, data ...any) {
		if name == eventMicrosoftSignIn {
			mu.Lock()
			codes = append(codes, data[0].(PersistentMicrosoftSignInView))
			mu.Unlock()
			return
		}
		old(a, name, data...)
	}
	t.Cleanup(func() { emitEvent = old })

	for _, bad := range []string{"-x@outlook.com", "--login"} {
		if _, err := rig.p.MicrosoftSignIn("assistant", bad); err == nil || !strings.Contains(err.Error(), "not an email address") {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if v, err = rig.p.MicrosoftSignIn("assistant", "ana@outlook.com"); err != nil {
		t.Fatal(err)
	}
	if v.Microsoft.Account != "ana@outlook.com" || v.Microsoft.Waiting {
		t.Fatalf("after the sign-in: %+v", v.Microsoft)
	}
	if len(codes) != 1 || codes[0] != (PersistentMicrosoftSignInView{Agent: "assistant", URL: "https://microsoft.com/devicelogin", Code: "QWERTY123"}) {
		t.Errorf("the page was told %+v", codes)
	}
	if !slices.Equal(opened, []string{"https://microsoft.com/devicelogin"}) {
		t.Errorf("opened %v", opened)
	}

	if _, err := rig.p.SetMicrosoftClient(coderAgent, testEntraApp, ""); err == nil || !strings.Contains(err.Error(), "no Microsoft 365") {
		t.Errorf("the Coding agent: %v", err)
	}
	if c, _ := rig.p.Get(coderAgent); c.Microsoft != nil {
		t.Errorf("the Coding agent has a Microsoft set-up: %+v", c.Microsoft)
	}
	rig.p.CancelMicrosoftSignIn("assistant")
}

// TestOnlyMicrosoftsDeviceLoginIsOpenedFromTheApp: the app opens Microsoft's
// own sign-in pages and nothing else, whatever it is handed.
func TestOnlyMicrosoftsDeviceLoginIsOpenedFromTheApp(t *testing.T) {
	for u, ok := range map[string]bool{
		"https://microsoft.com/devicelogin":               true,
		"https://www.microsoft.com/link":                  true,
		"https://www.microsoft.com/linkage":               false,
		"https://login.microsoftonline.com/common/oauth2": true,
		"https://microsoft.com.example.net/devicelogin":   false,
		"http://microsoft.com/devicelogin":                false,
		"https://example.com/":                            false,
	} {
		if microsoftDeviceLogin(u) != ok {
			t.Errorf("%s: %v", u, !ok)
		}
	}
}

const testAgentryApp = "00000000-0000-4000-8000-000000001264"

// microsoftFake is an engine whose Microsoft 365 server prints its code at
// --login and lists ana@outlook.com, keeping the client id each run was
// given.
func microsoftFake(clients *[]string) *dockerx.Fake {
	return &dockerx.Fake{Foreground: map[string]*dockerx.FakeForeground{"image-oneshot": {Handler: func(_ context.Context, s dockerx.RunSpec, _ io.Reader, stdout, _ io.Writer) (int, error) {
		*clients = append(*clients, s.Env["MS365_MCP_CLIENT_ID"])
		switch {
		case slices.Contains(s.Argv, "--login"):
			_, _ = io.WriteString(stdout, "\nTo sign in, use a web browser to open the page https://microsoft.com/devicelogin and enter the code QWERTY123 to authenticate.\n")
		case slices.Contains(s.Argv, "--list-accounts"):
			_, _ = io.WriteString(stdout, `[{"username":"ana@outlook.com"}]`)
		}
		return 0, nil
	}}}}
}

// TestSignInWithAgentrysMicrosoftApp: where the registry offers Agentry's
// own Microsoft app and no Entra app is kept, the view says it is offered,
// the sign-in runs with it, and the view then says the sign-in was made
// with it; an Entra app kept later comes first, and the registry is not
// asked for a set-up that has one.
func TestSignInWithAgentrysMicrosoftApp(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	asked := 0
	rig.p.agentryApp = func(context.Context) string { asked++; return testAgentryApp }
	v, err := rig.p.Get("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if v.Microsoft == nil || v.Microsoft.ClientID != "" || !v.Microsoft.AgentryAppOffered || v.Microsoft.SignedInWithAgentryApp {
		t.Fatalf("offered Agentry's app: %+v", v.Microsoft)
	}
	var clients []string
	f := microsoftFake(&clients)
	rig.p.microsoftSetUp = func(_ context.Context, a persistent.Agent) (persistent.Microsoft, error) {
		return persistent.Microsoft{Docker: f, Home: rig.home, Agent: a, Policy: policy.Default(), RunnerImage: "agentry-runner:test", Keys: &memSecrets{m: map[string]string{}}, Client: persistent.ClientDesktop}, nil
	}
	rig.p.browse = func(string) error { return nil }
	old := emitEvent
	emitEvent = func(*App, string, ...any) {}
	t.Cleanup(func() { emitEvent = old })
	if v, err = rig.p.MicrosoftSignIn("assistant", "ana@outlook.com"); err != nil {
		t.Fatal(err)
	}
	if len(clients) != 2 || clients[0] != testAgentryApp || clients[1] != testAgentryApp {
		t.Fatalf("the server was given %q", clients)
	}
	if v.Microsoft.Account != "ana@outlook.com" || !v.Microsoft.SignedInWithAgentryApp || v.Microsoft.Stale {
		t.Fatalf("after Agentry's app's sign-in: %+v", v.Microsoft)
	}

	if _, err = rig.p.SetMicrosoftClient("assistant", testEntraApp, ""); err != nil {
		t.Fatal(err)
	}
	asked = 0
	if v, err = rig.p.Get("assistant"); err != nil {
		t.Fatal(err)
	}
	if v.Microsoft.AgentryAppOffered || asked != 0 || !v.Microsoft.SignedInWithAgentryApp || !v.Microsoft.Stale {
		t.Fatalf("with an Entra app kept since: %+v, asked %d", v.Microsoft, asked)
	}
	clients = nil
	if v, err = rig.p.MicrosoftSignIn("assistant", "ana@outlook.com"); err != nil {
		t.Fatal(err)
	}
	if len(clients) != 2 || clients[0] != testEntraApp || v.Microsoft.SignedInWithAgentryApp || v.Microsoft.Stale {
		t.Fatalf("the person's own app comes first: %q, %+v", clients, v.Microsoft)
	}
}

// TestWithNoAgentryMicrosoftAppTheCardAsksForAnEntraApp: with none offered,
// the view says so and a sign-in without an Entra app is refused, as before.
func TestWithNoAgentryMicrosoftAppTheCardAsksForAnEntraApp(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	v, err := rig.p.Get("assistant")
	if err != nil || v.Microsoft == nil || v.Microsoft.AgentryAppOffered {
		t.Fatalf("with none offered: %+v, %v", v.Microsoft, err)
	}
	var clients []string
	f := microsoftFake(&clients)
	rig.p.microsoftSetUp = func(_ context.Context, a persistent.Agent) (persistent.Microsoft, error) {
		return persistent.Microsoft{Docker: f, Home: rig.home, Agent: a, Policy: policy.Default(), RunnerImage: "agentry-runner:test", Keys: &memSecrets{m: map[string]string{}}, Client: persistent.ClientDesktop}, nil
	}
	if _, err := rig.p.MicrosoftSignIn("assistant", "ana@outlook.com"); err == nil || !strings.Contains(err.Error(), "no Entra client yet") || len(clients) != 0 {
		t.Fatalf("a sign-in with no app: %v, ran %q", err, clients)
	}
}

// TestAgentrysMicrosoftAppIsReadFromTheRegistry: the registry's connectors
// route is read without a credential and its answer kept under the runner
// home; with no registry, nothing is asked or kept.
func TestAgentrysMicrosoftAppIsReadFromTheRegistry(t *testing.T) {
	var auth []string
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/connectors" {
			http.NotFound(w, r)
			return
		}
		auth = append(auth, r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(map[string]any{"connectors": []map[string]string{
			{"id": "github"}, {"id": "microsoft-365", "publicClientId": testAgentryApp},
		}})
	}))
	t.Cleanup(reg.Close)
	home := t.TempDir()
	if id := agentryMicrosoftAppAt(home, reg.URL)(context.Background()); id != testAgentryApp {
		t.Fatalf("the registry's Microsoft app: %q", id)
	}
	if id := agentryMicrosoftAppAt(home, reg.URL)(context.Background()); id != testAgentryApp || len(auth) != 1 || auth[0] != "" {
		t.Fatalf("asked %d times with %q; then %q", len(auth), auth, id)
	}
	if kept, _ := filepath.Glob(filepath.Join(home, "connectors", "*.json")); len(kept) != 1 {
		t.Fatalf("kept under the runner home: %v", kept)
	}
	empty := t.TempDir()
	if id := agentryMicrosoftAppAt(empty, "")(context.Background()); id != "" {
		t.Fatalf("no registry gave %q", id)
	}
	if _, err := os.Stat(filepath.Join(empty, "connectors")); !os.IsNotExist(err) {
		t.Fatalf("something was kept with no registry: %v", err)
	}
}
