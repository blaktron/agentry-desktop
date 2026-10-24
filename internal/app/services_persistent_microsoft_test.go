package app

import (
	"context"
	"io"
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
