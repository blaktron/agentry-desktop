package app

// The Knowledge panel live: the bound methods driven as the page drives
// them, against a real registry and this machine's docker and model, with
// the person's part — the native dialogs and the pickers — answered by the
// test. Skipped unless the three variables below are set, because it needs a
// registry, docker and a model key:
//
//	AGENTRY_LIVE_KNOWLEDGE_REGISTRY=http://127.0.0.1:8080 \
//	AGENTRY_LIVE_KNOWLEDGE_TOKEN=<a PAT with knowledge:read and knowledge:write> \
//	AGENTRY_LIVE_KNOWLEDGE_REF=<ns/name@version, a published autonomous release> \
//	AGENTRY_CLI_BIN=<an agentry CLI, optional> \
//	go test -run TestLiveKnowledge -v -timeout 30m ./internal/app
//
// It uses the runner home and policy AGENTRY_RUNNER_HOME names (docker must
// see it), the model key the policy's apiKeyEnv names in the environment,
// and a second, fresh runner home for "opens on another computer". It
// makes a pack from a folder with the real Creator, runs the release with
// it attached, runs it again through the CLI's `agentry run --knowledge
// NAME` when AGENTRY_CLI_BIN names one (the other client, on the same
// runner home), exports it, uploads it, downloads and previews it on the
// second home, and deletes it everywhere; nothing of it is left.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveKnowledge(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_KNOWLEDGE_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_KNOWLEDGE_TOKEN")
	ref := os.Getenv("AGENTRY_LIVE_KNOWLEDGE_REF")
	if reg == "" || token == "" || ref == "" {
		t.Skip("set AGENTRY_LIVE_KNOWLEDGE_REGISTRY, _TOKEN and _REF to run the live Knowledge check")
	}
	name := "live-" + time.Now().UTC().Format("20060102-150405")
	emit := emitEvent
	var log []string
	emitEvent = func(_ *App, _ string, data ...any) {
		if len(data) > 0 {
			if s, ok := data[0].(string); ok {
				log = append(log, s)
			}
		}
	}
	t.Cleanup(func() { emitEvent = emit })

	keys := store.OSKeychain{} // AGENTRY_NO_KEYRING is honoured, as by the CLI
	env := knowledge.Env{
		Home: policy.Home, Keys: keys,
		TokenFor: func(u string) string { return map[bool]string{true: token}[strings.TrimRight(u, "/") == reg] },
		HTTP:     registry.HTTPClient,
	}
	var asked []string
	exportTo := t.TempDir()
	app := &App{
		ctx: context.Background(),
		confirm: func(_ context.Context, title, _ string) (bool, error) {
			asked = append(asked, title)
			return true, nil
		},
		pickDirectory: func(context.Context, string) (string, error) { return exportTo, nil },
	}
	r := &RunAPI{app: app, knowledgeEnv: &env}
	k := &KnowledgeAPI{app: app, run: r, openFolder: func(string) error { return nil }}

	// Create knowledge from a folder, as the native folder picker returns one.
	docs := t.TempDir()
	if err := os.WriteFile(filepath.Join(docs, "widget-runbook.md"), []byte(
		"# Widget service runbook\n\nWhen the widget service stops answering, restart it with `widgetctl restart`, "+
			"then check its queue depth with `widgetctl queue`. A queue above 500 means page the on-call engineer.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.markPicked(docs)
	res, err := k.Create(reg, name, []string{docs}, "", false)
	if err != nil {
		t.Fatalf("Create knowledge: %v\n%s", err, strings.Join(log, "\n"))
	}
	t.Logf("made %s: %s, %d bytes, encrypted to %s; the Creator used %d in, %d out tokens",
		res.Name, kn.CountPages(res.Pages), res.Bytes, res.Key, res.InputTokens, res.OutputTokens)
	t.Cleanup(func() {
		// Whatever happened, nothing of the pack is left behind.
		_ = env.Delete(context.Background(), reg, name, true)
	})
	l, err := k.List(reg)
	if err != nil {
		t.Fatal(err)
	}
	var kept *knowledge.Pack
	for i := range l.Packs {
		if l.Packs[i].Name == name && l.Packs[i].Local {
			kept = &l.Packs[i]
		}
	}
	if kept == nil || kept.Wrapper != string(store.WrapperRegistry) || kept.ScanState != knowledge.ScanNotScanned {
		t.Fatalf("the panel lists %+v", l.Packs)
	}

	// A run with it attached, its output into a folder the picker "chose".
	log = nil
	outDir := t.TempDir()
	app.markPicked(outDir)
	run, err := r.RunAgent(reg, ref, "Use the knowledge tools to find how to restart the widget service and what queue depth means paging someone. Answer in two sentences and cite the page.",
		nil, outDir, nil, 0, -1, true, nil, false, false, []string{name}, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("run with %s attached: %v\n%s", name, err, strings.Join(log, "\n"))
	}
	t.Logf("run %s: status %s, exit %d, %d turns\nresult: %s\nknowledge: %s", run.RunID, run.Status, run.ExitCode, run.Turns, run.ResultJSON, run.Knowledge)
	if run.Status != "ok" || !strings.Contains(run.Knowledge, name+" mounted read-only at /knowledge/"+name) {
		t.Fatalf("the run: %+v", run)
	}
	if !strings.Contains(strings.ToLower(run.ResultJSON), "restart") {
		t.Errorf("the answer does not come from the pack: %s", run.ResultJSON)
	}

	// The other client opens the same pack by name, on the same runner home.
	if cli := os.Getenv("AGENTRY_CLI_BIN"); cli != "" {
		cmd := exec.Command(cli, "run", ref, "--registry", reg, "--allow-unsigned", "--knowledge", name, "--output-format", "json", "--out", t.TempDir())
		// The CLI sends SECUREAGENTRY_TOKEN only to the SecureAgentry
		// registry its release is built for, and AGENTRY_TOKEN only to the
		// self-hosted registry AGENTRY_REGISTRY names.
		tokenEnv := []string{"AGENTRY_TOKEN=" + token, "AGENTRY_REGISTRY=" + reg}
		if settings.IsSecureAgentry(reg) {
			tokenEnv = []string{"SECUREAGENTRY_TOKEN=" + token}
		}
		cmd.Env = append(os.Environ(), tokenEnv...)
		cmd.Stdin = strings.NewReader("Using the knowledge tools, what does a queue depth above 500 mean? One sentence, citing the page.")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("agentry run --knowledge %s: %v\n%s", name, err, out)
		}
		var env struct {
			Status               string            `json:"status"`
			Result               json.RawMessage   `json:"result"`
			CapabilitiesEnforced map[string]string `json:"capabilities_enforced"`
		}
		if err := json.Unmarshal(out, &env); err != nil || env.Status != "ok" || !strings.Contains(env.CapabilitiesEnforced["knowledge"], name+" mounted read-only") {
			t.Fatalf("agentry run --knowledge %s: %v\n%s", name, err, out)
		}
		t.Logf("the CLI's run with the desktop's pack: %s", env.Result)
	}

	// Export unencrypted: asked, picked, written, revealed.
	v, err := k.Export(reg, name)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := kn.Load(v.Dir); err != nil || p.Manifest.Name != name {
		t.Fatalf("the export: %v", err)
	}

	// Upload, then a second runner home signed in to the same account.
	msg, err := k.Upload(reg, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(msg)
	other := t.TempDir()
	env2 := env
	env2.Home = func() (string, error) { return other, nil }
	for i := 0; i < 60; i++ {
		l, err := env2.List(context.Background(), reg)
		if err != nil {
			t.Fatal(err)
		}
		ready := false
		for _, p := range l.Packs {
			if p.Name == name && p.ScanState != knowledge.ScanScanning {
				ready = true
			}
		}
		if ready {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if _, err := env2.Download(context.Background(), reg, name); err != nil {
		t.Fatalf("download on a fresh runner home: %v", err)
	}
	pv, err := env2.Preview(context.Background(), reg, name)
	if err != nil || len(pv.Pages) == 0 {
		t.Fatalf("preview on a fresh runner home: %+v, %v", pv, err)
	}
	t.Logf("opened on a fresh runner home: %s, index %q", kn.CountPages(len(pv.Pages)), firstLine(pv.Index.Markdown))

	// Delete Agentry's copy, then this computer's: each place's Delete
	// removes its own copy.
	l, err = env.List(context.Background(), reg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range l.Packs {
		if p.Name == name && p.AgentryID != "" {
			if err := k.DeleteFromAgentry(reg, p.AgentryID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := k.Delete(reg, name); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Find(reg, name); err == nil {
		t.Fatal("the pack is still kept after Delete")
	}
	t.Logf("dialogs asked: %v", asked)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
