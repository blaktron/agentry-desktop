package main

// Input sources live (agentry-desktop#794; plan input-sources.md M6): the
// bound methods driven as the run setup drives them, against this
// machine's Docker and a real git host, with a QuickAgent installed from a
// folder that asks for a repository. Skipped unless AGENTRY_LIVE_SOURCES=1,
// because it clones a private repository three times and needs a token
// that can read it:
//
//	AGENTRY_LIVE_SOURCES=1 \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home> \
//	AGENTRY_LIVE_SOURCES_REPO=https://github.com/<owner>/<a private repository> \
//	AGENTRY_LIVE_SOURCES_TOKEN=<a token that reads it, from a file, never typed> \
//	AGENTRY_LIVE_SOURCES_OUT=<a folder to keep the runs' output in, optional> \
//	go test -run TestLiveSources -v -timeout 60m .
//
// It refuses to run when the system keychain already holds a key for the
// repository's host, so it never replaces or removes the person's own.
// With no model key on the machine a run stops after its inputs are
// decoded; the source is fetched before that, so the receipt and
// intake.json name the commit either way.
//
// What it holds: a token saved through SaveSourceKey is the CLI's entry and
// the run uses it (the receipt says the saved key, intake.json names the
// commit); a token typed for one run is used for that run alone and not
// saved; with neither the private repository is refused in the host's own
// words; and the token is in no file any run leaves.

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/modelkey"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/sources"
)

const liveSourcesManifest = `schemaVersion: "2"
name: source-probe
namespace: local
modes: [autonomous]
version: 0.1.0
summary: "Reads the repository it is given and names its commit (agentry-desktop#794 live check)"
entrypoints:
  instruction: SKILL.md
inputs:
  kinds: [code]
intake:
  - id: codebase
    prompt: "The repository to read"
    type: repository
    required: true
capabilities:
  filesystem:
    read: workspace
    write: workspace
publish:
  include: [agent.yaml, SKILL.md]
`

func TestLiveSources(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_SOURCES") != "1" {
		t.Skip("clones a private repository: set AGENTRY_LIVE_SOURCES=1 (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" {
		t.Fatalf("set %s and %s to scratch directories", configdir.EnvVar, policy.HomeEnv)
	}
	repo, token := os.Getenv("AGENTRY_LIVE_SOURCES_REPO"), strings.TrimSpace(os.Getenv("AGENTRY_LIVE_SOURCES_TOKEN"))
	if repo == "" || token == "" {
		t.Fatal("set AGENTRY_LIVE_SOURCES_REPO and AGENTRY_LIVE_SOURCES_TOKEN")
	}
	outRoot := cmp.Or(os.Getenv("AGENTRY_LIVE_SOURCES_OUT"), t.TempDir())
	if modelkey.Disabled() {
		t.Fatal("the system keychain is off (AGENTRY_NO_KEYRING): this test saves a key in it")
	}

	app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
	r := &RunAPI{app: app}
	b := &BuildAPI{app: app, run: r}

	st, err := r.SourceKeyStatus("repository", repo)
	if err != nil || st.Problem != "" {
		t.Fatalf("the repository's address: %+v, %v", st, err)
	}
	host := st.Host
	if st.Saved {
		t.Fatalf("the system keychain already holds a key for %s: this test would replace it", host)
	}
	t.Cleanup(func() { _ = keyring.Delete(modelkey.Service, sources.KeyAccount(host)) })

	dir := filepath.Join(outRoot, "probe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	app.markPicked(dir) // what Build → Open folder… records (D-030)
	for name, body := range map[string]string{
		"agent.yaml": liveSourcesManifest,
		"SKILL.md":   "# Source probe\n\nList /inputs/codebase/, read its README, and report the commit named in /inputs/intake.json. Finish in at most 6 tool calls.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Install(dir); err != nil {
		t.Fatalf("install the probe: %v", err)
	}
	const ref = "local/source-probe@0.1.0"

	// noToken fails when the token is in any file under out.
	noToken := func(out string) {
		t.Helper()
		_ = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			raw, rerr := os.ReadFile(p)
			if rerr == nil && strings.Contains(string(raw), token) {
				t.Errorf("the token is in %s", p)
			}
			return nil
		})
	}
	// said answers the receipt text and intake.json under out.
	said := func(out string) string {
		t.Helper()
		var all strings.Builder
		_ = filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && (strings.HasSuffix(p, ".json") || strings.HasSuffix(p, ".log")) {
				raw, _ := os.ReadFile(p)
				all.Write(raw)
				all.WriteString("\n")
			}
			return nil
		})
		return all.String()
	}
	commitRE := regexp.MustCompile(`"commit":\s*"([0-9a-f]{40})"`)
	runIt := func(name string, typed map[string]string) (string, error) {
		out := filepath.Join(outRoot, name)
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
		app.markPicked(out)
		res, err := r.RunAgent("https://secureagentry.net", ref, "", nil, out, nil, 0, -1, true, map[string]string{"codebase": repo}, false, false, nil, false, typed)
		if res != nil {
			t.Logf("%s: status %s, exit %d, %s", name, res.Status, res.ExitCode, res.Error)
		}
		return out, err
	}

	// 1. A key saved through the setup is the CLI's entry, and the run uses it.
	if v, err := r.SaveSourceKey("repository", repo, token); err != nil || !v.Saved {
		t.Fatalf("save: %+v, %v", v, err)
	}
	out, err := runIt("saved", nil)
	if err != nil {
		t.Logf("saved: the run ended with %v", err)
	}
	text := said(out)
	m := commitRE.FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("saved: no commit in the run's receipts or intake.json under %s", out)
	}
	t.Logf("saved: commit %s", m[1])
	if !strings.Contains(text, "with the access token saved in the system keychain for "+host) {
		t.Errorf("saved: the receipt does not say the saved key was used")
	}
	noToken(out)

	// 2. Removed, then typed for one run: used, and not saved.
	if v, err := r.ForgetSourceKey("repository", repo); err != nil || v.Saved {
		t.Fatalf("forget: %+v, %v", v, err)
	}
	out, err = runIt("typed", map[string]string{"codebase": token})
	if err != nil {
		t.Logf("typed: the run ended with %v", err)
	}
	text = said(out)
	if commitRE.FindStringSubmatch(text) == nil || !strings.Contains(text, "with an access token given at the prompt") {
		t.Errorf("typed: no commit, or the receipt does not say a token was given for the run")
	}
	noToken(out)
	if v, _ := r.SourceKeyStatus("repository", repo); v == nil || v.Saved {
		t.Errorf("typed: the token was saved: %+v", v)
	}

	// 3. Neither: the private repository is refused in the host's words.
	out, err = runIt("anonymous", nil)
	if err == nil && commitRE.FindStringSubmatch(said(out)) != nil {
		t.Fatalf("anonymous: a private repository was fetched with no token")
	}
	t.Logf("anonymous: %v", err)
	if err != nil && errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	noToken(out)
}
