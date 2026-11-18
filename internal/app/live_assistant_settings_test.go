package app

// The settings tools live: the Assistant started as the app starts it, asked
// to read the settings of Runs, try to change one and open the section, its
// calls reaching this app's handler through the runner's supervisor. Skipped
// unless AGENTRY_LIVE_SETTINGS=1:
//
//	AGENTRY_LIVE_SETTINGS=1 \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home, its policy.yaml naming the model> \
//	HOME=<a scratch home, where the Assistant's share folder is made> \
//	go test -run TestLiveAssistantSettings -v -timeout 30m ./internal/app
//
// The model's key is read from the variable the policy names. An AI Review
// key is kept in an in-memory keychain. What it holds: settings_read,
// settings_change and settings_open each reached this app's handler; the
// Runs section was opened on the page; nothing was asked or changed; and no
// answer the agent was given carries the key.

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveAssistantSettings(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_SETTINGS") != "1" {
		t.Skip("uses a model: set AGENTRY_LIVE_SETTINGS=1 (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" {
		t.Fatalf("set %s and %s to scratch directories", configdir.EnvVar, policy.HomeEnv)
	}
	keyring.MockInit()
	const key = "jev-live-settings-not-a-key-31337"
	if err := run.StoreJevKey(key, io.Discard); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	s, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.DefaultOutputDir = out
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(mustSettingsPath(t))

	var mu sync.Mutex
	var opened []any
	oldEmit := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if name == eventOpenSettings {
			mu.Lock()
			opened = append(opened, data...)
			mu.Unlock()
		}
	}
	t.Cleanup(func() { emitEvent = oldEmit })
	var asked []string
	svc := New("")
	svc.app.ctx = context.Background()
	svc.app.confirm = func(_ context.Context, title, message string) (bool, error) {
		asked = append(asked, title+"\n"+message)
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
	consents := len(asked)

	ask := "Do these three steps with your tools, in order. 1. Call settings_read with the section Runs, and tell me, word for word, the value of the setting whose id is runs.outputFolder. " +
		"2. Call settings_change with the id runs.rememberRuns and the value false, and tell me its answer word for word. " +
		"3. Call settings_open with the section Runs."
	if err := p.Send(assistantAgent, ask); err != nil {
		t.Fatalf("Send: %v", err)
	}
	home := os.Getenv(policy.HomeEnv)
	var calls string
	deadline := time.Now().Add(15 * time.Minute)
	for time.Now().Before(deadline) {
		calls = appLinkCalls(home)
		mu.Lock()
		done := len(opened) > 0
		mu.Unlock()
		if done && strings.Contains(calls, `"tool":"settings_read"`) && strings.Contains(calls, `"tool":"settings_change"`) {
			break
		}
		time.Sleep(5 * time.Second)
	}
	t.Logf("applink.log:\n%s", calls)
	for _, tool := range []string{"settings_read", "settings_change", "settings_open"} {
		if !strings.Contains(calls, `"tool":"`+tool+`"`) {
			t.Errorf("the agent's %s did not reach the app", tool)
		}
	}
	if !strings.Contains(calls, out) {
		t.Error("settings_read's answer does not carry the output folder")
	}
	if strings.Contains(calls, key) {
		t.Fatal("an answer the agent was given carries the AI Review key")
	}
	mu.Lock()
	t.Logf("opened: %v", opened)
	if len(opened) == 0 || opened[0] != "run-defaults" {
		t.Errorf("the page was asked to open %v", opened)
	}
	mu.Unlock()
	if len(asked) != consents {
		t.Errorf("a settings tool raised a dialog: %v", asked[consents:])
	}
	if after, _ := os.ReadFile(mustSettingsPath(t)); string(after) != string(before) {
		t.Errorf("the settings changed:\n%s", after)
	}
}

func mustSettingsPath(t *testing.T) string {
	t.Helper()
	p, err := settings.Path()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// appLinkCalls is every app link call the Assistant's segments logged.
func appLinkCalls(home string) string {
	logs, _ := filepath.Glob(filepath.Join(home, "persistent", assistantAgent, "segments", "*", "receipts", "applink.log"))
	var calls string
	for _, l := range logs {
		b, _ := os.ReadFile(l)
		calls += string(b)
	}
	return calls
}
