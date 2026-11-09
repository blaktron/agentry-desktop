package app

// The Assistant's publisher tools live: the Assistant started as the app
// starts it, signed in to a registry, asked to list the person's
// QuickAgents and fetch one, its calls answered by the app's own handler.
// Skipped unless AGENTRY_LIVE_PUBLISHER=1:
//
//	AGENTRY_LIVE_PUBLISHER=1 \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home, its policy.yaml naming the model> \
//	HOME=<a scratch home, where the Assistant's share folder is made> \
//	AGENTRY_LIVE_PUBLISHER_TOKEN=<a file holding the registry sign-in's token> \
//	AGENTRY_LIVE_PUBLISHER_REGISTRY=<the registry, default https://dev.secureagentry.net> \
//	AGENTRY_LIVE_PUBLISHER_NS=<a namespace the sign-in publishes in> \
//	AGENTRY_LIVE_PUBLISHER_REF=<one of its QuickAgents to fetch, ns/name@version> \
//	go test -run TestLiveAssistantPublisher -v -timeout 30m ./internal/app
//
// The model's key is read from the variable the policy names. The sign-in
// is kept in an in-memory keychain, so no keychain of this computer is
// written. What it holds: the Assistant's quickagent_mine and
// quickagent_fetch calls reach the app's handler and are answered, the
// fetched files are in the share folder, the fetch is kept, no dialog was
// raised for a read, and no answer the agent was given carries the token.

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveAssistantPublisher(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_PUBLISHER") != "1" {
		t.Skip("uses a model and a registry sign-in: set AGENTRY_LIVE_PUBLISHER=1 (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" {
		t.Fatalf("set %s and %s to scratch directories", configdir.EnvVar, policy.HomeEnv)
	}
	reg := cmp.Or(os.Getenv("AGENTRY_LIVE_PUBLISHER_REGISTRY"), "https://dev.secureagentry.net")
	ns, ref := os.Getenv("AGENTRY_LIVE_PUBLISHER_NS"), os.Getenv("AGENTRY_LIVE_PUBLISHER_REF")
	raw, err := os.ReadFile(os.Getenv("AGENTRY_LIVE_PUBLISHER_TOKEN"))
	if err != nil || ns == "" || ref == "" {
		t.Fatal("set AGENTRY_LIVE_PUBLISHER_TOKEN to a file holding the token, and the namespace and reference")
	}
	token := strings.TrimSpace(string(raw))

	keyring.MockInit()
	oldReg := settings.DefaultRegistryURL
	settings.DefaultRegistryURL = reg
	t.Cleanup(func() { settings.DefaultRegistryURL = oldReg })
	s, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Upsert(settings.Registry{URL: reg})
	s.DefaultRegistry = reg
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.StoreToken(reg, token); err != nil {
		t.Fatal(err)
	}

	oldEmit := emitEvent
	emitEvent = func(*App, string, ...any) {}
	t.Cleanup(func() { emitEvent = oldEmit })
	var dialogs []string
	svc := New("")
	svc.app.ctx = context.Background()
	svc.app.confirm = func(_ context.Context, title, message string) (bool, error) {
		dialogs = append(dialogs, title)
		t.Logf("dialog %q:\n%s", title, message)
		return true, nil
	}
	p := svc.persistent
	if _, err := p.SetEnabled(assistantAgent, true); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Start(assistantAgent); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = p.Stop(assistantAgent) })
	consents := len(dialogs)

	ask := "Use your quickagent_mine tool with the namespace " + ns + ", then quickagent_fetch with the reference " + ref +
		". Then tell me, word for word, the first line of each tool's answer, and the names of three files you can now see in share/quickagents/."
	if err := p.Send(assistantAgent, ask); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(dialogs) != consents {
		t.Errorf("a read raised a dialog: %v", dialogs[consents:])
	}

	home := os.Getenv(policy.HomeEnv)
	logs, _ := filepath.Glob(filepath.Join(home, "persistent", assistantAgent, "segments", "*", "receipts", "applink.log"))
	var calls string
	for _, l := range logs {
		b, _ := os.ReadFile(l)
		calls += string(b)
	}
	t.Logf("applink.log:\n%s", calls)
	if strings.Contains(calls, token) {
		t.Fatal("an answer the agent was given carries the token")
	}
	for _, tool := range []string{`"tool":"quickagent_mine"`, `"tool":"quickagent_fetch"`} {
		if !strings.Contains(calls, tool) {
			t.Errorf("the agent did not call %s", tool)
		}
	}
	nsName, _, _ := strings.Cut(ref, "@")
	share, _ := filepath.Glob(filepath.Join(os.Getenv("HOME"), "agentry", "assistantshare", "quickagents", filepath.FromSlash(nsName), "*"))
	t.Logf("in the share: %v", share)
	if len(share) == 0 {
		t.Error("nothing was fetched into the share folder")
	}
	if b, err := os.ReadFile(filepath.Join(os.Getenv(configdir.EnvVar), "assistant-fetched.json")); err != nil || !strings.Contains(string(b), ref) {
		t.Errorf("the fetch was not kept: %v", err)
	}
}
