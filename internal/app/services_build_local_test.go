package app

// The Build tab's Install and Submit: each asks in a native dialog the page
// cannot answer, and a No does nothing.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

const localAgentYAML = `schemaVersion: "2"
name: reporter
namespace: acme
modes: [autonomous]
version: 1.0.0
summary: "a QuickAgent never published"
entrypoints:
  instruction: SKILL.md
capabilities:
  network:
    egress: [api.example.com]
  secrets: [RUN_TEST_TOKEN]
publish:
  include: [agent.yaml, SKILL.md]
`

// localBuild isolates every state location and returns a BuildAPI whose
// dialogs answer with answer and record what they said, and a picked
// folder holding the QuickAgent.
func localBuild(t *testing.T, answer bool) (*BuildAPI, string, string, *[]string) {
	t.Helper()
	setUserHome(t, t.TempDir())
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	var said []string
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, msg string) (bool, error) {
		said = append(said, title+"\n"+msg)
		return answer, nil
	}}
	dir := t.TempDir()
	for name, body := range map[string]string{"agent.yaml": localAgentYAML, "SKILL.md": "# Reporter\n\nWrite the report.\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app.markPicked(dir)
	return &BuildAPI{app: app}, home, dir, &said
}

func TestInstallLaysAFolderDownAsALocalInstall(t *testing.T) {
	b, home, dir, said := localBuild(t, true)
	v, err := b.Install(dir)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if v.Ref != "acme/reporter@1.0.0" || !strings.HasPrefix(v.Digest, "sha256:") || v.Repaired {
		t.Fatalf("view = %+v", v)
	}
	if len(*said) != 1 {
		t.Fatalf("dialogs = %v", *said)
	}
	for _, want := range []string{"Install acme/reporter on this computer?", "local and unsigned", "nothing is submitted", "network egress to api.example.com", "secrets: RUN_TEST_TOKEN"} {
		if !strings.Contains((*said)[0], want) {
			t.Errorf("the dialog does not say %q:\n%s", want, (*said)[0])
		}
	}
	st, err := installed.Load()
	if err != nil {
		t.Fatal(err)
	}
	rec := st.Find(installdir.ConsentKey(home, "acme", "reporter"))
	if rec == nil || !rec.Local() || rec.Digest != v.Digest {
		t.Fatalf("record = %+v", rec)
	}

	// A copy whose files changed is laid down again only on a second Yes.
	rel := installdir.ReleaseDir(home, "acme", "reporter", "1.0.0", v.Digest)
	if err := os.WriteFile(filepath.Join(rel, "workspace", "SKILL.md"), []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err = b.Install(dir)
	if err != nil || !v.Repaired {
		t.Fatalf("reinstall over a changed copy = %+v, %v", v, err)
	}
	if last := (*said)[len(*said)-1]; !strings.Contains(last, "Replace the changed copy?") || !strings.Contains(last, "changed SKILL.md") {
		t.Errorf("the repair dialog = %s", last)
	}
	if b, _ := os.ReadFile(filepath.Join(rel, "workspace", "SKILL.md")); string(b) != "# Reporter\n\nWrite the report.\n" {
		t.Errorf("the repaired copy holds %q", b)
	}
}

func TestInstallDeclinedInstallsNothing(t *testing.T) {
	b, home, dir, _ := localBuild(t, false)
	if v, err := b.Install(dir); err != nil || v != nil {
		t.Fatalf("Install answered No = %+v, %v; want nothing and no error", v, err)
	}
	if _, err := os.Stat(installdir.AgentDir(home, "acme", "reporter")); !os.IsNotExist(err) {
		t.Errorf("something was laid down: %v", err)
	}
}

func TestSubmitDeclinedSendsNothing(t *testing.T) {
	b, _, dir, said := localBuild(t, false)
	var uploads atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agents/acme/reporter", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			uploads.Add(1)
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	storeTestToken(t, srv.URL)

	res, err := b.Submit(srv.URL, dir)
	if err != nil || res != nil {
		t.Fatalf("Submit answered No = %+v, %v; want nothing and no error", res, err)
	}
	if n := uploads.Load(); n != 0 {
		t.Errorf("%d writes reached the registry after a No", n)
	}
	if len(*said) != 1 || !strings.Contains((*said)[0], "Submit acme/reporter publicly?") || !strings.Contains((*said)[0], "makes it public") {
		t.Fatalf("dialogs = %v", *said)
	}
	// This registry answers no metadata: what a submission costs is not
	// known, and the warning must not say it is free.
	if !strings.Contains((*said)[0], "could not be read") || strings.Contains((*said)[0], "charges no credits") {
		t.Errorf("a registry whose extensions could not be read: %s", (*said)[0])
	}
}

func TestSubmitWarningNamesTheCost(t *testing.T) {
	for _, c := range []struct {
		name    string
		private bool
		sub     *SubscriptionView
		subErr  error
		want    []string
	}{
		{"no billing", false, nil, nil, []string{"charges no credits"}},
		{"one credit", false, &SubscriptionView{SubmissionCost: 1, Balance: 40}, nil, []string{"It spends 1 credit from this account's balance of 40."}},
		{"organization, private", true, &SubscriptionView{SubmissionCost: 3, Balance: 9, MemberOf: "acme"}, nil, []string{"3 credits from the acme organization's balance of 9", "this version stays private with it", "Make public in the Publisher section"}},
		{"unmetered", false, &SubscriptionView{Unmetered: true}, nil, []string{"spends no credits"}},
		{"unreadable", false, nil, errors.New("HTTP 500"), []string{"could not be read (HTTP 500)"}},
	} {
		got := submitConsentText("acme/reporter@1.0.0", "https://dev.example", c.private, c.sub, c.subErr)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: %q missing from\n%s", c.name, w, got)
			}
		}
		if strings.Contains(got, "makes it public") == c.private {
			t.Errorf("%s: the warning says public for an agent that is private, or not for one that is not:\n%s", c.name, got)
		}
	}
}

// storeTestToken signs the test in to reg, in a keychain that lives only
// in this process.
func storeTestToken(t *testing.T, reg string) {
	t.Helper()
	keyring.MockInit()
	// The app signs in to one self-hosted registry: the test's server
	// takes the place of any an earlier test added to the shared config.
	cfg, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	kept := cfg.Registries[:0]
	for _, r := range cfg.Registries {
		if settings.IsSecureAgentry(r.URL) {
			kept = append(kept, r)
		}
	}
	cfg.Registries = append(kept, settings.Registry{URL: reg})
	if err := settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.StoreToken(reg, "agy_test"); err != nil {
		t.Fatal(err)
	}
}

// The site's settings page for an agent that stays private is on the
// registry's own origin, whatever path the registry URL carries, and the
// agent's name stays a path segment.
func TestSiteSettingsURL(t *testing.T) {
	for _, c := range []struct{ reg, ns, name, want string }{
		{"https://secureagentry.net", "acme", "demo", "https://secureagentry.net/acme/demo/settings"},
		{"https://dev.secureagentry.net/api/v1", "acme", "demo", "https://dev.secureagentry.net/acme/demo/settings"},
		{"https://secureagentry.net", "a/../b", "x?y", "https://secureagentry.net/a%2F..%2Fb/x%3Fy/settings"},
	} {
		if got := siteSettingsURL(c.reg, c.ns, c.name); got != c.want {
			t.Errorf("siteSettingsURL(%q, %q, %q) = %q, want %q", c.reg, c.ns, c.name, got, c.want)
		}
	}
}
