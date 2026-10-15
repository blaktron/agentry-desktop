package main

// Edit with AI through the bound surface, against the draft fake: the In
// Agentry offer gated on the payer, the hosted revision's create, its
// landing as a revision of the release (the QuickAgent's name, the next
// minor, the release's licence, the diff), and Publish this version naming
// the run as draftRun only for a revision drafted In Agentry. The
// on-this-computer half: the release staged from the registry's archive,
// held to its digest.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

const editDigest = "sha256:" + "ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12"

// serveRelease adds chris/meeting-actions to the draft fake: an agent this
// sign-in may publish to, 1.2.0 its current release (as editManifest), and
// what a submit of the next version needs. It returns the draftRun field
// of every upload, "" for none.
func serveRelease(t *testing.T, f *draftFake) *[]string {
	t.Helper()
	return serveReleaseAt(t, f, editDigest)
}

// serveReleaseAt is serveRelease with the archive digest the registry
// records for 1.2.0.
func serveReleaseAt(t *testing.T, f *draftFake, digest string) *[]string {
	t.Helper()
	uploads := &[]string{}
	mux := f.mux
	mux.HandleFunc("GET /api/v1/agents/chris/meeting-actions", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"id": "11111111-1111-1111-1111-111111111111", "namespace": "chris", "name": "meeting-actions",
			"modes": []string{"autonomous"}, "visibility": "public", "summary": "s", "defaultBranch": "main",
			"gitHttpUrl": "x", "quarantineState": "none", "createdAt": "2026-09-30T00:00:00Z", "canPublish": true})
	})
	mux.HandleFunc("GET /api/v1/agents/chris/meeting-actions/versions", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, []map[string]any{
			{"version": "1.4.0", "status": "needs_review", "channel": "stable", "createdAt": "2026-10-02T00:00:00Z"},
			{"version": "1.2.0", "status": "published", "channel": "stable", "createdAt": "2026-09-30T00:00:00Z", "archiveDigest": digest},
		})
	})
	mux.HandleFunc("GET /api/v1/agents/chris/meeting-actions/versions/1.2.0/manifest", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(editManifest))
	})
	mux.HandleFunc("GET /api/v1/agents/chris/meeting-actions/versions/1.2.0/file", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"path": "PROMPT.md", "size": 30, "binary": false, "truncated": false, "content": "# Meeting actions\n\nRead the notes.\n"})
	})
	mux.HandleFunc("POST /api/v1/agents/chris/meeting-actions/versions", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Errorf("multipart: %v", err)
		}
		*uploads = append(*uploads, r.FormValue("draftRun"))
		hostedJSON(w, http.StatusAccepted, map[string]any{"version": "1.5.0", "status": "published", "channel": "stable", "createdAt": "2026-10-03T00:00:00Z"})
	})
	mux.HandleFunc("GET /api/v1/agents/chris/meeting-actions/versions/1.5.0", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"version": "1.5.0", "status": "published", "channel": "stable", "createdAt": "2026-10-03T00:00:00Z"})
	})
	mux.HandleFunc("GET /api/v1/agents/chris/meeting-actions/versions/1.5.0/report", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"versionStatus": "published", "findings": []any{}, "findingCounts": map[string]int{}})
	})
	return uploads
}

// revisedDraftJSON is the Drafter's revision of 1.2.0 as draft.json carries
// it: a name of its own choosing, MIT, a new host, and the release named.
func revisedDraftJSON(t *testing.T) string {
	t.Helper()
	d := map[string]any{
		"manifest": json.RawMessage(`{"schemaVersion": "2", "name": "meeting-actions-v2", "namespace": "draft", "version": "0.1.0",
		  "summary": "Turns meeting notes into decisions and dated action items.", "license": "MIT", "homepage": "https://example.test",
		  "modes": ["autonomous"], "entrypoints": {"instruction": "AGENT.md"},
		  "capabilities": {"network": {"egress": ["slack.com", "calendar.example"]}, "filesystem": {"read": "workspace", "write": "none"}, "exec": "none"}}`),
		"instructions": "# Meeting actions\n\nRead the notes. Give every action a date.\n",
		"notes":        "## Notes\n\nDates added.\n", "matches": []any{}, "openQuestions": []any{}, "sources": []any{},
		"drafter":  map[string]any{"ref": "agentry/drafter@1.0.0", "model": "platform", "runId": draftRunID},
		"aiReview": map[string]any{"description": "pass", "scan": "registry", "read": 1, "passed": 1, "withheld": 0, "notScanned": 0, "withheldSources": []any{}},
		"revises":  map[string]any{"ref": "chris/meeting-actions@1.2.0", "digest": editDigest},
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestEditWithAIInAgentryIsOfferedOnThePayersTier(t *testing.T) {
	f := newDraftFake(t)
	b, _, _ := hostedDraftAPI(t, f)
	v, err := b.DescribeInAgentryStatus(f.url)
	if err != nil || v.ReviseOffered {
		t.Fatalf("a registry without caller.revise: %+v %v", v, err)
	}
	no := false
	f.mu.Lock()
	f.revise = &no
	f.mu.Unlock()
	if v, _ = b.DescribeInAgentryStatus(f.url); !v.ReviseOffered || v.ReviseAvailable || !v.ReviseUpgrade || !strings.Contains(v.ReviseReason, "Pro and above") {
		t.Fatalf("Standard: %+v", v)
	}
	yes := true
	f.mu.Lock()
	f.revise = &yes
	f.mu.Unlock()
	if v, _ = b.DescribeInAgentryStatus(f.url); !v.ReviseAvailable {
		t.Fatalf("Pro: %+v", v)
	}
}

func TestARevisionInAgentryLandsUnderTheQuickAgentsNameAndPublishesWithItsRun(t *testing.T) {
	f := newDraftFake(t)
	uploads := serveRelease(t, f)
	b, ev, _ := hostedDraftAPI(t, f)
	start, err := b.ReviseInAgentry(f.url, "chris", "meeting-actions", "  Give every action a date.\r\n", "")
	if err != nil || start.Draft == nil || start.Draft.ID != draftRunID {
		t.Fatalf("start %+v %v", start, err)
	}
	body := f.created[0]
	if body["kind"] != "draft" || body["revises"] != "chris/meeting-actions@1.2.0" || body["instructions"] != "Give every action a date." {
		t.Fatalf("body %v", body)
	}
	for _, k := range []string{"description", "hints", "knowledge", "problems", "ref"} {
		if _, ok := body[k]; ok {
			t.Fatalf("a revision's create carries %s: %v", k, body)
		}
	}
	f.finish("succeeded", revisedDraftJSON(t), 12)
	final := ev.final(t)
	if final.Draft == nil || final.DraftError != "" {
		t.Fatalf("final %+v", final)
	}
	d := final.Draft
	r := d.Revision
	if r == nil || r.Namespace != "chris" || r.Name != "meeting-actions" || r.From != "1.2.0" || r.Version != "1.5.0" || !r.Hosted {
		t.Fatalf("revision %+v", r)
	}
	// The QuickAgent's own name, the next minor, the release's licence
	// (the texts do not ask for MIT), and no homepage.
	m := d.Model
	if m.Namespace != "chris" || m.Name != "meeting-actions" || m.Version != "1.5.0" || m.License != "Apache-2.0" || m.Homepage != "" {
		t.Fatalf("model %+v", m)
	}
	if len(d.DecodeNotes) != 0 {
		t.Fatalf("a revision drops without a word: %v", d.DecodeNotes)
	}
	// The diff: both files changed, the instruction file named as the
	// release had it.
	if len(r.Diff) != 2 || r.Diff[0].Path != "agent.yaml" || r.Diff[0].Same || r.Diff[1].Path != "PROMPT.md, written as AGENT.md" || r.Diff[1].Same {
		t.Fatalf("diff %+v", r.Diff)
	}
	if !slices.Contains(d.Unmentioned, "calendar.example") || slices.Contains(d.Unmentioned, "slack.com") {
		t.Fatalf("unmentioned %v: only the host the release did not reach", d.Unmentioned)
	}

	// Publish this version: into a folder the picker gave, after the
	// native warning, naming the run.
	dir := filepath.Join(t.TempDir(), "rev")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := b.PublishThisVersion(f.url, dir, r.Key); err == nil || !strings.Contains(err.Error(), "file dialog") {
		t.Fatalf("a folder the picker did not give: %v", err)
	}
	b.app.markPicked(dir)
	b.app.confirm = func(context.Context, string, string) (bool, error) { return true, nil }
	res, err := b.PublishThisVersion(f.url, dir, r.Key)
	if err != nil || res == nil || res.Status != "published" {
		t.Fatalf("publish %+v %v", res, err)
	}
	if !slices.Equal(*uploads, []string{draftRunID}) {
		t.Fatalf("uploads %v, want the run named once", *uploads)
	}
	// What was saved is the revision as drafted, whatever the page holds.
	yml, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	if !strings.Contains(string(yml), "version: \"1.5.0\"") && !strings.Contains(string(yml), "version: 1.5.0") || !strings.Contains(string(yml), "calendar.example") {
		t.Fatalf("saved agent.yaml:\n%s", yml)
	}
	// A folder that holds a QuickAgent is not written over.
	if _, err := b.PublishThisVersion(f.url, dir, r.Key); err == nil || !strings.Contains(err.Error(), "already holds") {
		t.Fatalf("a second publish into the same folder: %v", err)
	}
}

func TestARevisionOnThisComputerPublishesWithNoRun(t *testing.T) {
	f := newDraftFake(t)
	uploads := serveRelease(t, f)
	b, _, _ := hostedDraftAPI(t, f)
	var d drafter.Draft
	raw := revisedDraftJSON(t)
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	v := &DraftView{Description: "Give every action a date."}
	if err := b.landRevision(context.Background(), f.url, hostedTestToken, v, &d, "chris/meeting-actions@1.2.0", "", false); err != nil {
		t.Fatal(err)
	}
	if v.Revision == nil || v.Revision.Hosted || v.Model.Version != "1.5.0" {
		t.Fatalf("%+v", v.Revision)
	}
	dir := t.TempDir()
	b.app.markPicked(dir)
	b.app.confirm = func(context.Context, string, string) (bool, error) { return true, nil }
	first := v.Revision.Key
	// A second revision lands: the first one's key no longer publishes
	// anything, and never the second revision in its place.
	v2 := &DraftView{Description: "Give every action a date."}
	if err := b.landRevision(context.Background(), f.url, hostedTestToken, v2, &d, "chris/meeting-actions@1.2.0", "", false); err != nil {
		t.Fatal(err)
	}
	if v2.Revision.Key == "" || v2.Revision.Key == first {
		t.Fatalf("keys %q %q", first, v2.Revision.Key)
	}
	if _, err := b.PublishThisVersion(f.url, dir, first); err == nil || !strings.Contains(err.Error(), "landed last") {
		t.Fatalf("an earlier revision's key: %v", err)
	}
	if _, err := b.PublishThisVersion(f.url, dir, v2.Revision.Key); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(*uploads, []string{""}) {
		t.Fatalf("uploads %v, want no run named", *uploads)
	}
	// A draft that does not say it revises the release is not landed as one.
	other := d
	other.Revises = &drafter.Revises{Ref: "chris/meeting-actions@1.1.0"}
	if err := b.landRevision(context.Background(), f.url, hostedTestToken, &DraftView{}, &other, "chris/meeting-actions@1.2.0", "", false); err == nil {
		t.Fatal("a draft of another release landed")
	}
}

// releaseArchive packs chris/meeting-actions@1.2.0 as the registry holds
// it, with PROMPT.md its instruction file, as editManifest names it.
func releaseArchive(t *testing.T) []byte {
	t.Helper()
	pkg := t.TempDir()
	for name, body := range map[string]string{
		"agent.yaml": "schemaVersion: \"2\"\nname: meeting-actions\nnamespace: chris\nversion: 1.2.0\nsummary: Turns notes into actions.\nlicense: MIT\nmodes: [autonomous]\n" +
			"entrypoints:\n  instruction: PROMPT.md\ncapabilities:\n  network:\n    egress: none\n  filesystem:\n    read: workspace\n    write: none\n  exec: none\n",
		"PROMPT.md": "# Meeting actions\n",
		"README.md": "# readme\n",
	} {
		if err := os.WriteFile(filepath.Join(pkg, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := pkgarchive.WalkWorkspace(pkg)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if _, err := pkgarchive.BuildArchive(pkg, files, &archive); err != nil {
		t.Fatalf("pack: %v", err)
	}
	return archive.Bytes()
}

// serveArchive adds resolve and the archive's download to the fake;
// *served is the digest resolve answers.
func serveArchive(f *draftFake, archive []byte, served *string) {
	f.mux.HandleFunc("GET /api/v1/resolve", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"namespace": "chris", "name": "meeting-actions", "version": "1.2.0",
			"digest": *served, "archiveUrl": "/archive/meeting-actions-1.2.0.tar.zst", "manifest": map[string]any{}})
	})
	f.mux.HandleFunc("GET /archive/meeting-actions-1.2.0.tar.zst", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
}

func TestTheReleaseIsStagedFromItsArchiveHeldToItsDigest(t *testing.T) {
	archive := releaseArchive(t)
	digest := registry.Digest(archive)
	f := newDraftFake(t)
	served := digest
	serveArchive(f, archive, &served)
	signIn(t, f.url)
	dest := filepath.Join(t.TempDir(), drafter.ReviseInput)
	rev, err := stageRelease(context.Background(), f.url, hostedTestToken, "chris/meeting-actions@1.2.0", digest, dest)
	if err != nil {
		t.Fatal(err)
	}
	if rev.Name != "meeting-actions" || rev.InstructionFile != "PROMPT.md" || rev.Dir != dest || rev.Digest != digest {
		t.Fatalf("%+v", rev)
	}
	// The Drafter is given the manifest and the instruction file alone.
	entries, _ := os.ReadDir(dest)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"PROMPT.md", "agent.yaml"}) {
		t.Fatalf("staged %v", names)
	}
	// Its task names the staged files and fences the person's texts.
	if err := rev.WithTexts("Give every action a date.", ""); err != nil {
		t.Fatal(err)
	}
	task, err := drafter.RevisionTask(rev)
	if err != nil || !strings.Contains(task, "/inputs/revise/PROMPT.md") || !strings.Contains(task, "Give every action a date.") {
		t.Fatalf("task %q %v", task, err)
	}
	// A release whose archive is not the one the registry described is
	// refused before anything is staged.
	if _, err := stageRelease(context.Background(), f.url, hostedTestToken, "chris/meeting-actions@1.2.0", editDigest, filepath.Join(t.TempDir(), "revise")); err == nil {
		t.Fatal("a release that changed since it was read was staged")
	}
	served = editDigest
	if _, err := stageRelease(context.Background(), f.url, hostedTestToken, "chris/meeting-actions@1.2.0", editDigest, filepath.Join(t.TempDir(), "revise")); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("an archive that is not the digest: %v", err)
	}
}

// Edit with AI on this computer, through the binding: asked in a native
// dialog, then under the run lock the release is downloaded, held to its
// digest and staged under the run's revise/, AI Review reads the texts and
// both of its files before the Drafter runs, the Drafter is given them under
// /inputs/revise with a task naming them and the person's texts, and the
// draft lands as a revision drafted here.
func TestARevisionOnThisComputerStagesTheReleaseForTheDrafter(t *testing.T) {
	f := newDescribeFixture(t)
	saveDescribeKey(t)
	archive := releaseArchive(t)
	digest := registry.Digest(archive)
	reg := newDraftFake(t)
	serveReleaseAt(t, reg, digest)
	serveArchive(reg, archive, &digest)
	s, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Upsert(settings.Registry{URL: settings.Normalize(reg.url), Token: hostedTestToken})
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	f.result = `{"manifest": {"schemaVersion": "2", "name": "meeting-actions", "namespace": "draft", "version": "0.1.0",
		"summary": "Turns meeting notes into dated actions.", "license": "Apache-2.0", "modes": ["autonomous"], "entrypoints": {"instruction": "AGENT.md"},
		"capabilities": {"network": {"egress": "none"}, "filesystem": {"read": "none", "write": "none"}, "exec": "none"},
		"publish": {"include": ["agent.yaml", "AGENT.md", "README.md", "LICENSE"]}},
		"matches": [], "sources": [], "openQuestions": []}`
	v, err := f.b.ReviseOnThisComputer(reg.url, "chris", "meeting-actions", "Give every action a date.", "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.asked, []string{"Edit chris/meeting-actions with AI on this computer?"}) {
		t.Fatalf("asked %v", f.asked)
	}
	if len(f.runs) == 0 {
		t.Fatal("the Drafter did not run")
	}
	first := f.runs[0]
	revise := filepath.Join(first.OutDir, drafter.ReviseInput)
	if !slices.Equal(first.Inputs, []string{revise}) {
		t.Fatalf("inputs %v, want the staged release %s", first.Inputs, revise)
	}
	for _, name := range []string{"agent.yaml", "PROMPT.md"} {
		if _, err := os.Stat(filepath.Join(revise, name)); err != nil {
			t.Fatalf("the release's %s was not staged: %v", name, err)
		}
	}
	task, _ := io.ReadAll(first.Task)
	if !strings.Contains(string(task), "/inputs/revise/PROMPT.md") || !strings.Contains(string(task), "Give every action a date.") {
		t.Fatalf("task:\n%s", task)
	}
	// AI Review read the task, then the release's files (in the pack scan's
	// chunks: two small files are one), before the Drafter ran.
	if f.scan.read < 2 {
		t.Fatalf("AI Review read %d texts before the Drafter, want the task and the release", f.scan.read)
	}
	r := v.Revision
	if r == nil || r.Hosted || r.From != "1.2.0" || r.Version != "1.5.0" || r.Key == "" || v.Model.Name != "meeting-actions" || v.Model.Namespace != "chris" {
		t.Fatalf("revision %+v, model %s/%s", r, v.Model.Namespace, v.Model.Name)
	}
}
