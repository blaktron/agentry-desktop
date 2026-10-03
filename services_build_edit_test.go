package main

// Edit on the QuickAgent page (agentry-desktop#500) through the bound
// surface, against a fake registry: the page's condition is the registry's
// canPublish, and the current release reaches the form under the same name,
// at the next minor above every version, with what the Builder cannot hold
// dropped.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const editManifest = `{"schemaVersion": "2", "name": "meeting-actions", "namespace": "chris", "version": "1.2.0",
  "summary": "Turns meeting notes into decisions and action items.", "license": "Apache-2.0",
  "homepage": "https://example.test", "authors": [{"name": "Ada"}],
  "modes": ["autonomous"], "entrypoints": {"instruction": "PROMPT.md"},
  "capabilities": {"network": {"egress": ["slack.com"]}, "filesystem": {"read": "workspace", "write": "none"}, "exec": "none"},
  "publish": {"include": ["agent.yaml", "PROMPT.md", "README.md", "LICENSE"], "channel": "beta"}}`

type editFake struct {
	mu         sync.Mutex
	url        string
	canPublish *bool
	versions   []map[string]any
	file       map[string]any
	// asked are the paths the fake served, for what was and was not read.
	asked []string
}

func newEditFake(t *testing.T) *editFake {
	t.Helper()
	yes := true
	f := &editFake{
		canPublish: &yes,
		versions: []map[string]any{
			// Newest first, as the registry lists them: one in review,
			// one yanked, then the current release.
			{"version": "1.4.0-rc.1", "status": "needs_review", "channel": "stable", "createdAt": "2026-10-02T00:00:00Z"},
			{"version": "1.3.0", "status": "published", "channel": "stable", "createdAt": "2026-10-01T00:00:00Z", "yankedAt": "2026-10-01T01:00:00Z"},
			{"version": "1.2.0", "status": "published", "channel": "stable", "createdAt": "2026-09-30T00:00:00Z"},
		},
		file: map[string]any{"path": "PROMPT.md", "size": 30, "binary": false, "truncated": false, "content": "# Meeting actions\n\nRead the notes.\n"},
	}
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		f.mu.Lock()
		f.asked = append(f.asked, r.URL.Path+"?"+r.URL.RawQuery)
		f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+hostedTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/v1/agents/chris/meeting-actions", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		a := map[string]any{"id": "11111111-1111-1111-1111-111111111111", "namespace": "chris", "name": "meeting-actions",
			"modes": []string{"autonomous"}, "visibility": "public", "summary": "s", "defaultBranch": "main",
			"gitHttpUrl": "x", "quarantineState": "none", "createdAt": "2026-09-30T00:00:00Z"}
		if f.canPublish != nil {
			a["canPublish"] = *f.canPublish
		}
		hostedJSON(w, http.StatusOK, a)
	})
	mux.HandleFunc("GET /api/v1/agents/chris/meeting-actions/versions", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, f.versions)
	})
	mux.HandleFunc("GET /api/v1/agents/chris/meeting-actions/versions/{v}/manifest", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(editManifest))
	})
	mux.HandleFunc("GET /api/v1/agents/chris/meeting-actions/versions/{v}/file", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, f.file)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f
}

func editAPI(t *testing.T, f *editFake) *BuildAPI {
	t.Helper()
	signIn(t, f.url)
	app := &App{ctx: context.Background()}
	return &BuildAPI{app: app, run: &RunAPI{app: app}}
}

func TestEditReadsTheCurrentReleaseAtTheNextMinorUnderTheSameName(t *testing.T) {
	f := newEditFake(t)
	b := editAPI(t, f)
	v, err := b.EditRelease(f.url, "chris", "meeting-actions")
	if err != nil {
		t.Fatal(err)
	}
	// The current release is the newest published, unyanked one; the
	// version is the next minor above every version in any state.
	if v.From != "1.2.0" || v.Version != "1.5.0" || v.Namespace != "chris" || v.Name != "meeting-actions" {
		t.Fatalf("view %+v", v)
	}
	m := v.Model
	if m.Namespace != "chris" || m.Name != "meeting-actions" || m.Version != "1.5.0" || m.License != "Apache-2.0" ||
		m.Instructions != "# Meeting actions\n\nRead the notes.\n" || m.InstructionFile != "AGENT.md" {
		t.Fatalf("model %+v", m)
	}
	if m.Homepage != "" || len(m.Authors) != 0 || m.PublishChannel != "" || len(m.PublishInclude) != 0 {
		t.Fatalf("unheld fields reached the form: %+v", m)
	}
	// It read the release's own instruction file, of the current release.
	f.mu.Lock()
	asked := strings.Join(f.asked, "\n")
	f.mu.Unlock()
	if !strings.Contains(asked, "/versions/1.2.0/file?path=PROMPT.md") || !strings.Contains(asked, "/versions/1.2.0/manifest") {
		t.Fatalf("asked:\n%s", asked)
	}
}

func TestEditRefusesWhatTheRegistryDoesNotOffer(t *testing.T) {
	no := false
	for what, c := range map[string]struct {
		edit func(f *editFake)
		want string
	}{
		"canPublish false": {func(f *editFake) { f.canPublish = &no }, "cannot add a version"},
		// A registry that predates the field, or a viewer who may not
		// publish: the field is absent, and Edit is not offered.
		"canPublish absent": {func(f *editFake) { f.canPublish = nil }, "cannot add a version"},
		"nothing published": {func(f *editFake) { f.versions = f.versions[:2] }, "no published version"},
		"a truncated file":  {func(f *editFake) { f.file["truncated"] = true }, "too long"},
		"a binary file":     {func(f *editFake) { f.file["binary"] = true; delete(f.file, "content") }, "could not be read"},
	} {
		t.Run(what, func(t *testing.T) {
			f := newEditFake(t)
			c.edit(f)
			b := editAPI(t, f)
			if _, err := b.EditRelease(f.url, "chris", "meeting-actions"); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err %v, want %q", err, c.want)
			}
		})
	}
	// A QuickAgent the registry does not show is the same refusal.
	f := newEditFake(t)
	b := editAPI(t, f)
	if _, err := b.EditRelease(f.url, "chris", "nothing-here"); err == nil || !strings.Contains(err.Error(), "cannot add a version") {
		t.Fatalf("a missing QuickAgent: %v", err)
	}
}

func TestTheAgentPageReadsCanPublish(t *testing.T) {
	f := newEditFake(t)
	signIn(t, f.url)
	b := &BrowseAPI{app: &App{ctx: context.Background()}}
	d, err := b.Agent(f.url, "chris", "meeting-actions")
	if err != nil || !d.CanPublish {
		t.Fatalf("canPublish true: %+v %v", d, err)
	}
	f.mu.Lock()
	f.canPublish = nil
	f.mu.Unlock()
	if d, err = b.Agent(f.url, "chris", "meeting-actions"); err != nil || d.CanPublish {
		t.Fatalf("canPublish absent: %+v %v", d, err)
	}
}
