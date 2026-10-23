package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// writeTasks gives the agent a tasks file of two tasks.
func writeTasks(t *testing.T, home, agent string) {
	t.Helper()
	dir := persistent.Dir(home, agent)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `version: 1
nextId: 3
tasks:
  - id: t1
    name: Morning summary
    cron: "0 8 * * 1-5"
    zone: Europe/Oslo
    instruction: Summarise my calendar for the day.
    state: "on"
    madeBy: agent
    created: 2026-10-20T08:00:00Z
    lastFire: 2026-10-22T06:00:00Z
    lastResult: "Three meetings today; the first at 9:30."
  - id: t2
    name: Water the plants
    cron: "0 18 * * 0"
    zone: Europe/Oslo
    instruction: Remind me to water the plants.
    state: paused
    madeBy: agent
    created: 2026-10-20T08:00:00Z
`
	if err := os.WriteFile(filepath.Join(dir, "tasks.yaml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The Tasks view reads every task with its next fire and its last answer's
// first line; pause, resume and remove go through the agent's owner and are
// kept; there is no add, and an operation the view does not offer is
// refused; the Coding agent has no tasks.
func TestTheTasksViewReadsAndChangesTheAssistantsTasks(t *testing.T) {
	rig := newPersistentRig(t)
	writeTasks(t, rig.home, "assistant")
	ts, err := rig.p.Tasks("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 2 || ts[0].Name != "Morning summary" || ts[0].Cron != "0 8 * * 1-5" || ts[0].Zone != "Europe/Oslo" ||
		ts[0].LastResult != "Three meetings today; the first at 9:30." || ts[0].LastFire == "" || ts[0].Next == "" || ts[0].MadeBy != "agent" {
		t.Fatalf("tasks: %+v", ts)
	}
	if ts[1].State != persistent.TaskPaused {
		t.Fatalf("the second task's state: %q", ts[1].State)
	}
	if ts, err = rig.p.ChangeTask("assistant", "t1", persistent.TaskOpPause); err != nil || ts[0].State != persistent.TaskPaused {
		t.Fatalf("pause: %+v %v", ts, err)
	}
	if ts, err = rig.p.ChangeTask("assistant", "t2", persistent.TaskOpResume); err != nil || ts[1].State != persistent.TaskOn {
		t.Fatalf("resume: %+v %v", ts, err)
	}
	if ts, err = rig.p.ChangeTask("assistant", "t2", persistent.TaskOpRemove); err != nil || len(ts) != 1 || ts[0].ID != "t1" {
		t.Fatalf("remove: %+v %v", ts, err)
	}
	// Kept: a fresh read sees the change.
	if again, _ := rig.p.Tasks("assistant"); len(again) != 1 || again[0].State != persistent.TaskPaused {
		t.Fatalf("after the changes: %+v", again)
	}
	if _, err := rig.p.ChangeTask("assistant", "t1", "add"); err == nil {
		t.Error("an add was taken")
	}
	if ts, err := rig.p.Tasks(coderAgent); err != nil || len(ts) != 0 {
		t.Errorf("the Coding agent's tasks: %+v %v", ts, err)
	}
}

// Export memory copies into a new folder made inside the one the picker
// returns; a cancel writes nothing; an agent without a memory has none to
// export.
func TestExportMemoryMakesANewFolderInThePickedOne(t *testing.T) {
	rig := newPersistentRig(t)
	parent := t.TempDir()
	picked := ""
	rig.p.pickFolder = func() (string, error) { return picked, nil }
	var gotDir string
	rig.p.exportMemory = func(_ context.Context, _ string, a persistent.Agent, dir string) (persistent.Exported, error) {
		gotDir = dir
		if a.Name != "assistant" {
			t.Errorf("exported %s", a.Name)
		}
		return persistent.Exported{Files: 4, Bytes: 2048}, nil
	}
	if v, err := rig.p.ExportMemory("assistant"); err != nil || v.Folder != "" || gotDir != "" {
		t.Fatalf("a cancel: %+v %v (exported into %q)", v, err, gotDir)
	}
	picked = parent
	v, err := rig.p.ExportMemory("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(v.Folder) != parent || !strings.HasPrefix(filepath.Base(v.Folder), "assistant-memory-") || gotDir != v.Folder || v.Files != 4 || v.Bytes != 2048 {
		t.Fatalf("the export: %+v into %q", v, gotDir)
	}
	if _, err := rig.p.ExportMemory(coderAgent); err == nil {
		t.Error("the Coding agent's memory was exported")
	}
}

// An account's token goes into the keychain under the host and the user,
// never back to the page; the form is held to the CLI's rules; setting it
// again asks in the native dialog; a refused change puts the keychain back;
// a changed user name's old token goes; Remove takes the token away too;
// the Assistant has no accounts.
func TestAGitHostAccountKeepsItsTokenInTheKeychain(t *testing.T) {
	rig := newPersistentRig(t)
	keys := &memSecrets{}
	rig.p.secrets = keys
	form := PersistentAccountForm{Host: "github.com", Own: true, User: "my-coder-bot", Email: "bot@example.com", Token: "planted-token-1"}
	if _, err := rig.p.SetAccount("assistant", form); err == nil {
		t.Fatal("the Assistant took an account")
	}
	for _, bad := range []PersistentAccountForm{
		{Host: "github.com", Own: true, Email: "bot@example.com", Token: "planted-token-1"},
		{Host: "github.com", Own: true, User: "my-coder-bot", Token: "planted-token-1"},
		{Host: "git.example.com", Own: true, User: "bot", Email: "bot@example.com", Token: "planted-token-1"},
		{Host: "http://github.com", Own: true, User: "bot", Email: "bot@example.com", Token: "planted-token-1"},
		{Host: "github.com", Own: true, User: "bot", Email: "bot@example.com", Token: ""},
	} {
		if _, err := rig.p.SetAccount(coderAgent, bad); err == nil {
			t.Errorf("a form was taken: %+v", bad)
		}
	}
	if len(keys.m) != 0 {
		t.Fatalf("a refused form left the keychain %v", keys.m)
	}
	rig.answer = false
	if _, err := rig.p.SetAccount(coderAgent, form); !errors.Is(err, errNotConfirmed) || len(keys.m) != 0 {
		t.Fatalf("a No: %v, keychain %v", err, keys.m)
	}
	rig.answer = true
	v, err := rig.p.SetAccount(coderAgent, form)
	if err != nil {
		t.Fatal(err)
	}
	account := persistent.AccountKeychainAccount("github.com", "my-coder-bot")
	if got, _ := keys.Get(account); got != "planted-token-1" {
		t.Fatalf("the keychain holds %q", got)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "planted-token") {
		t.Fatal("the view carries the token")
	}
	if len(v.Accounts) != 1 || v.Accounts[0].Host != "github.com" || !v.Accounts[0].Own || v.Accounts[0].Name != "my-coder-bot" || v.Accounts[0].Kind != "github" {
		t.Fatalf("accounts: %+v", v.Accounts)
	}
	// Set again under the same user: the dialog asks; a No keeps the old.
	asked := len(rig.dialogs())
	rig.answer = false
	form.Token = "planted-token-2"
	if _, err := rig.p.SetAccount(coderAgent, form); !errors.Is(err, errNotConfirmed) || len(rig.dialogs()) != asked+1 {
		t.Fatalf("set again: %v", err)
	}
	if got, _ := keys.Get(account); got != "planted-token-1" {
		t.Fatalf("a refused token changed the keychain: %q", got)
	}
	// Another user name: the old user's token goes once the change stands.
	rig.answer = true
	form.User = "other-bot"
	if _, err := rig.p.SetAccount(coderAgent, form); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Get(account); err == nil {
		t.Error("the earlier user's token stayed")
	}
	if _, err := rig.p.RemoveAccount(coderAgent, "github.com"); err != nil {
		t.Fatal(err)
	}
	if len(keys.m) != 0 {
		t.Errorf("after the remove the keychain holds %v", keys.m)
	}
	if g := rig.p.AccountGuidance("github", true); !strings.Contains(g, "Its own account") {
		t.Errorf("guidance: %q", g)
	}
	if k := rig.p.AccountKind("GitLab.com"); k != "gitlab" {
		t.Errorf("kind of gitlab.com: %q", k)
	}
}
