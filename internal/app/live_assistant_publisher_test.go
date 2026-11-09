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
//	AGENTRY_LIVE_PUBLISHER_REVISE=<one of its QuickAgents to revise In Agentry, ns/name> \
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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveAssistantPublisher(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_PUBLISHER") != "1" {
		t.Skip("uses a model and a registry sign-in: set AGENTRY_LIVE_PUBLISHER=1 (see the file's comment)")
	}
	_, ns, token, p, dialogs := liveAssistant(t)
	ref := os.Getenv("AGENTRY_LIVE_PUBLISHER_REF")
	if ref == "" {
		t.Fatal("set AGENTRY_LIVE_PUBLISHER_REF")
	}
	consents := len(*dialogs)
	ask := "Use your quickagent_mine tool with the namespace " + ns + ", then quickagent_fetch with the reference " + ref +
		". Then tell me, word for word, the first line of each tool's answer, and the names of three files you can now see in share/quickagents/."
	if err := p.Send(assistantAgent, ask); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(*dialogs) != consents {
		t.Errorf("a read raised a dialog: %v", (*dialogs)[consents:])
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

// TestLiveAssistantForkAndSubmit: the Assistant forks a public QuickAgent
// into the person's namespace, fetches the fork, changes it and submits a
// new version of it, then writes a new QuickAgent and submits it, each
// write asked in a dialog (answered Yes here and logged). Skipped unless
// AGENTRY_LIVE_PUBLISHER=1 and AGENTRY_LIVE_PUBLISHER_FORK names a public
// QuickAgent outside AGENTRY_LIVE_PUBLISHER_NS, which is where the fork and
// the new QuickAgent go. What it holds: both dialogs were raised naming
// the Assistant, the fork has the new version, and the new QuickAgent
// exists private.
func TestLiveAssistantForkAndSubmit(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_PUBLISHER") != "1" || os.Getenv("AGENTRY_LIVE_PUBLISHER_FORK") == "" {
		t.Skip("uses a model and a registry sign-in: set AGENTRY_LIVE_PUBLISHER=1 and AGENTRY_LIVE_PUBLISHER_FORK (see the file's comment)")
	}
	reg, ns, token, p, dialogs := liveAssistant(t)
	origin := os.Getenv("AGENTRY_LIVE_PUBLISHER_FORK")
	stamp := strconv.FormatInt(time.Now().Unix(), 36)
	fork, fresh := "m3-fork-"+stamp, "m3-new-"+stamp
	ask := "Fork " + origin + " into the namespace " + ns + " with the name " + fork + ", using quickagent_fork. Then quickagent_fetch " + ns + "/" + fork +
		". In share/quickagents/" + ns + "/" + fork + "/README.md add a last line saying \"Checked by the Assistant.\", and in its agent.yaml move the version to the next patch version. Then quickagent_submit share/quickagents/" + ns + "/" + fork +
		". Tell me what each tool answered."
	// Files fetched into the share are checked before the fetch answers,
	// so the whole of it is one turn.
	if err := p.Send(assistantAgent, ask); err != nil {
		t.Fatalf("Send: %v", err)
	}
	manifest := "schemaVersion: \"2\"\nname: " + fresh + "\nnamespace: " + ns + "\nmodes: [autonomous]\nversion: 0.1.0\nsummary: \"Says hello to the person (live check)\"\nlicense: MIT\nentrypoints:\n  instruction: INSTRUCTION.md\ncapabilities:\n  network:\n    egress: none\n  filesystem:\n    read: none\n    write: none\npublish:\n  include: [agent.yaml, INSTRUCTION.md, LICENSE]\n"
	ask = "Make a new QuickAgent: write these three files into share/quickagents/" + ns + "/" + fresh + "/, exactly as given. agent.yaml:\n\n" + manifest +
		"\nINSTRUCTION.md: \"Say hello to the person in one sentence.\"\nLICENSE: \"MIT License\"\n\nThen quickagent_submit share/quickagents/" + ns + "/" + fresh + " and tell me what it answered."
	if err := p.Send(assistantAgent, ask); err != nil {
		t.Fatalf("Send: %v", err)
	}
	t.Logf("dialogs: %q", *dialogs)
	var forkAsked, submitAsked int
	for _, d := range *dialogs {
		if strings.Contains(d, " asks to fork "+origin+" into "+ns+"/"+fork) {
			forkAsked++
		}
		if strings.Contains(d, " asks to submit "+ns+"/") {
			submitAsked++
		}
	}
	if forkAsked != 1 || submitAsked != 2 {
		t.Errorf("asked to fork %d times and to submit %d times, want 1 and 2", forkAsked, submitAsked)
	}
	ops := appPublisherOps{pub: &PublisherAPI{app: &App{ctx: context.Background()}}}
	vs, err := ops.versions(reg, ns, fork)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		t.Logf("%s/%s@%s: %s", ns, fork, v.Version, v.Status)
	}
	if len(vs) < 2 {
		t.Errorf("the fork has %d versions, want its first and the Assistant's", len(vs))
	}
	exists, private, err := ops.standing(reg, ns, fresh)
	if err != nil || !exists || !private {
		t.Errorf("the new QuickAgent: exists %v, private %v, %v", exists, private, err)
	}
	_ = token
}

// TestLiveAssistantRevise: the Assistant fetches one of the person's
// QuickAgents, has the Drafter revise it In Agentry, moves its version on
// and submits it, each write asked in a dialog (answered Yes here and
// logged). Skipped unless AGENTRY_LIVE_PUBLISHER=1 and
// AGENTRY_LIVE_PUBLISHER_REVISE names a QuickAgent in
// AGENTRY_LIVE_PUBLISHER_NS whose payer may revise In Agentry. What it
// holds: the revision and the submission were each asked once, naming the
// Assistant, the revision ran In Agentry, and the QuickAgent has a new
// version.
func TestLiveAssistantRevise(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_PUBLISHER") != "1" || os.Getenv("AGENTRY_LIVE_PUBLISHER_REVISE") == "" {
		t.Skip("uses a model, a registry sign-in and credits: set AGENTRY_LIVE_PUBLISHER=1 and AGENTRY_LIVE_PUBLISHER_REVISE (see the file's comment)")
	}
	reg, _, token, p, dialogs := liveAssistant(t)
	target := os.Getenv("AGENTRY_LIVE_PUBLISHER_REVISE")
	ns, name, _ := strings.Cut(target, "/")
	ops := appPublisherOps{pub: &PublisherAPI{app: &App{ctx: context.Background()}}}
	before, err := ops.versions(reg, ns, name)
	if err != nil {
		t.Fatal(err)
	}
	ask := "Use quickagent_fetch with " + target + ". Then use quickagent_revise on share/quickagents/" + target + " with where set to agentry and these instructions for the Drafter: \"End every answer with a one-line summary that starts with Summary:.\" " +
		"When it answers, read share/quickagents/" + target + "/agent.yaml, set its version to the next patch version after the latest one quickagent_versions lists, and quickagent_submit share/quickagents/" + target +
		". Do it all in this one reply, and tell me what quickagent_revise and quickagent_submit answered."
	if err := p.Send(assistantAgent, ask); err != nil {
		t.Fatalf("Send: %v", err)
	}
	t.Logf("dialogs: %q", *dialogs)
	var reviseAsked, submitAsked int
	for _, d := range *dialogs {
		if strings.Contains(d, " asks to revise "+target+" with the Drafter") {
			reviseAsked++
		}
		if strings.Contains(d, " asks to submit "+target) {
			submitAsked++
		}
	}
	if reviseAsked != 1 || submitAsked != 1 {
		t.Errorf("asked to revise %d times and to submit %d times, want 1 and 1", reviseAsked, submitAsked)
	}
	after, err := ops.versions(reg, ns, name)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range after {
		t.Logf("%s@%s: %s", target, v.Version, v.Status)
	}
	if len(after) != len(before)+1 {
		t.Errorf("%s has %d versions, want %d", target, len(after), len(before)+1)
	}
	_ = token
}

// liveAssistant signs in to the registry through an in-memory keychain,
// starts the Assistant as the app wires it, and answers every dialog Yes,
// keeping each one's title and message.
func liveAssistant(t *testing.T) (reg, ns, token string, p *PersistentAPI, dialogs *[]string) {
	t.Helper()
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" {
		t.Fatalf("set %s and %s to scratch directories", configdir.EnvVar, policy.HomeEnv)
	}
	reg = cmp.Or(os.Getenv("AGENTRY_LIVE_PUBLISHER_REGISTRY"), "https://dev.secureagentry.net")
	ns = os.Getenv("AGENTRY_LIVE_PUBLISHER_NS")
	raw, err := os.ReadFile(os.Getenv("AGENTRY_LIVE_PUBLISHER_TOKEN"))
	if err != nil || ns == "" {
		t.Fatal("set AGENTRY_LIVE_PUBLISHER_TOKEN to a file holding the token, and the namespace")
	}
	token = strings.TrimSpace(string(raw))
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
	var asked []string
	svc := New("")
	svc.app.ctx = context.Background()
	svc.app.confirm = func(_ context.Context, title, message string) (bool, error) {
		asked = append(asked, title+"\n"+message)
		t.Logf("dialog %q:\n%s", title, message)
		return true, nil
	}
	p = svc.persistent
	if _, err := p.SetEnabled(assistantAgent, true); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Start(assistantAgent); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _, _ = p.Stop(assistantAgent) })
	return reg, ns, token, p, &asked
}
