package app

// Shipping knowledge packs from the Build tab through the bound surface: Save
// lays the ticked packs and nothing else, Submit confirms each pack in a
// native dialog before anything is sent and sends the confirmation with the
// upload, and Edit ties the previous version's packs to the person's.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
)

// fakeShipper lays a pack of the size the source says, with the files the
// registry's rules read, and remembers what it laid and where from.
type fakeShipper struct {
	laid    map[string]build.ShipSource
	origins map[string]*build.PackOrigin
	// failOn makes Lay refuse the pack.
	failOn string
	pruned [][]string
}

func newFakeShipper() *fakeShipper {
	return &fakeShipper{laid: map[string]build.ShipSource{}, origins: map[string]*build.PackOrigin{}}
}

func (f *fakeShipper) Lay(_ context.Context, dir string, src build.ShipSource) (*build.Laid, error) {
	if src.Name == f.failOn {
		return nil, errors.New("the store no longer holds it")
	}
	root := filepath.Join(dir, "knowledge", src.Name)
	if err := os.RemoveAll(root); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "wiki"), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"schema":"knowledge/v1"}`), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(root, "wiki", "index.md"), make([]byte, src.Bytes), 0o644); err != nil {
		return nil, err
	}
	f.laid[src.Name] = src
	var origin *build.PackOrigin
	if src.AgentryID != "" {
		origin = &build.PackOrigin{Kind: src.Kind, Org: src.Org, PackID: src.AgentryID}
	}
	f.origins[src.Name] = origin
	return &build.Laid{Bytes: src.Bytes, Files: 2, Origin: origin}, nil
}

func (f *fakeShipper) Prune(dir string, keep []string) error {
	f.pruned = append(f.pruned, slices.Clone(keep))
	for name := range f.laid {
		if !slices.Contains(keep, name) {
			delete(f.laid, name)
			delete(f.origins, name)
			if err := os.RemoveAll(filepath.Join(dir, "knowledge", name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *fakeShipper) Origins(string) (map[string]*build.PackOrigin, error) {
	out := map[string]*build.PackOrigin{}
	for k, v := range f.origins {
		if v != nil {
			out[k] = v
		}
	}
	return out, nil
}

func shipModel(t *testing.T, srcs ...build.ShipSource) build.Model {
	t.Helper()
	m := build.Default("acme")
	m.Name, m.Summary, m.FSRead = "reporter", "Reports.", "none"
	for _, s := range srcs {
		if err := m.TickShip(s); err != nil {
			t.Fatal(err)
		}
	}
	return *m
}

// Save lays each pack the form ships from its source and takes out a pack
// laid earlier that the form no longer ships; a name with no source is the
// folder's own files and is left alone.
func TestSaveLaysTheShippedPacksAndTakesOutThoseNoLongerShipped(t *testing.T) {
	b, _, dir, _ := localBuild(t, true)
	sh := newFakeShipper()
	b.shipper = sh
	m := shipModel(t,
		build.ShipSource{Name: "glossary", Kind: build.ShipOwn, Bytes: 2048, AgentryID: "6f1c0000-0000-4000-8000-000000000001"},
		build.ShipSource{Name: "runbook", Kind: build.ShipOwn, Bytes: 100})
	m.KnowledgeShips = append(m.KnowledgeShips, "by-hand")
	if err := os.MkdirAll(filepath.Join(dir, "knowledge", "by-hand"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(dir, m); err != nil {
		t.Fatal(err)
	}
	if sh.laid["glossary"].Bytes != 2048 || sh.laid["runbook"].Bytes != 100 || len(sh.laid) != 2 {
		t.Fatalf("laid %+v", sh.laid)
	}
	if !slices.Equal(sh.pruned[0], []string{"glossary", "runbook", "by-hand"}) {
		t.Fatalf("kept %v", sh.pruned)
	}
	// The form drops runbook: its folder goes, the hand-made one stays.
	m.UntickShip("runbook")
	if err := b.Save(dir, m); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]bool{"glossary": true, "runbook": false, "by-hand": true} {
		if _, err := os.Stat(filepath.Join(dir, "knowledge", name)); (err == nil) != want {
			t.Errorf("knowledge/%s present = %v, want %v", name, err == nil, want)
		}
	}
	// Ships nothing and no shipper needed: a save is the plain one.
	b.shipper = nil
	plain := shipModel(t)
	if err := b.Save(dir, plain); err != nil {
		t.Fatalf("a QuickAgent that ships nothing needs no shipper: %v", err)
	}
}

func TestSaveNamesThePackThatCouldNotBeLaid(t *testing.T) {
	b, _, dir, _ := localBuild(t, true)
	sh := newFakeShipper()
	sh.failOn = "runbook"
	b.shipper = sh
	err := b.Save(dir, shipModel(t, build.ShipSource{Name: "glossary", Kind: build.ShipOwn, Bytes: 1}, build.ShipSource{Name: "runbook", Kind: build.ShipOwn, Bytes: 1}))
	if err == nil || !strings.Contains(err.Error(), "the knowledge pack runbook could not be put in the folder") {
		t.Fatalf("%v", err)
	}
	// With no way to ship, a form that ships a pack cannot be saved.
	b.shipper = nil
	if err := b.Save(dir, shipModel(t, build.ShipSource{Name: "glossary", Kind: build.ShipOwn, Bytes: 1})); !errors.Is(err, build.ErrNoShipper) {
		t.Fatalf("%v", err)
	}
}

// shipRegistryFor is a registry that takes one version of acme/reporter and
// records what the upload carried.
type shipRegistryFor struct {
	mu       sync.Mutex
	private  bool
	writes   []string
	shipping string
	shipPart bool
	srvURL   string
}

func startShipRegistry(t *testing.T, private bool) *shipRegistryFor {
	t.Helper()
	r := &shipRegistryFor{private: private}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agents/acme/reporter", func(w http.ResponseWriter, _ *http.Request) {
		vis := "public"
		if r.private {
			vis = "private"
		}
		hostedJSON(w, http.StatusOK, publisherAgentDoc("acme", "reporter", vis, nil))
	})
	mux.HandleFunc("POST /api/v1/agents", func(w http.ResponseWriter, _ *http.Request) {
		r.mu.Lock()
		r.writes = append(r.writes, "POST agent")
		r.mu.Unlock()
		hostedJSON(w, http.StatusCreated, publisherAgentDoc("acme", "reporter", "public", nil))
	})
	mux.HandleFunc("POST /api/v1/agents/acme/reporter/versions", func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		r.writes = append(r.writes, "POST version")
		r.mu.Unlock()
		_, params, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
		mr := multipart.NewReader(req.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			if p.FormName() == "knowledgeShipping" {
				b, _ := io.ReadAll(p)
				r.mu.Lock()
				r.shipPart, r.shipping = true, string(b)
				r.mu.Unlock()
			}
		}
		hostedJSON(w, http.StatusAccepted, map[string]any{"version": "1.0.0", "status": "published", "channel": "stable", "createdAt": "2026-10-20T00:00:00Z"})
	})
	mux.HandleFunc("GET /api/v1/agents/acme/reporter/versions/1.0.0/report", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"versionStatus": "published", "findings": []any{}, "findingCounts": map[string]int{}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	r.srvURL = srv.URL
	return r
}

func (r *shipRegistryFor) sentWrites() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.writes)
}

// shipFolder saves a QuickAgent that ships two packs into dir, as the page
// does before it submits.
func shipFolder(t *testing.T, b *BuildAPI, dir string) {
	t.Helper()
	m := shipModel(t,
		build.ShipSource{Name: "glossary", Kind: build.ShipOwn, Bytes: 1536, AgentryID: "6f1c0000-0000-4000-8000-000000000001"},
		build.ShipSource{Name: "runbook", Kind: build.ShipOwn, Bytes: 3 << 20})
	if err := os.Remove(filepath.Join(dir, "agent.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(dir, m); err != nil {
		t.Fatal(err)
	}
}

// Submit asks each pack in its own dialog, in the public words, after the
// submission is agreed, and sends the confirmation with the origins the
// shipper recorded.
func TestSubmitConfirmsEachShippedPackAndSendsTheConfirmation(t *testing.T) {
	b, _, dir, said := localBuild(t, true)
	b.shipper = newFakeShipper()
	shipFolder(t, b, dir)
	reg := startShipRegistry(t, false)
	storeTestToken(t, reg.srvURL)

	res, err := b.Submit(reg.srvURL, dir)
	if err != nil || res == nil || res.Status != "published" {
		t.Fatalf("%+v, %v", res, err)
	}
	if len(*said) != 3 {
		t.Fatalf("dialogs %q", *said)
	}
	for i, want := range [][]string{
		{"Submit acme/reporter publicly?", "It ships the knowledge packs glossary and runbook, readable by everyone who can download the release; you confirm each pack next."},
		{"Ship the knowledge pack glossary?", "glossary (1.5 KB) becomes public. Anyone can download it with this release, and it stays in this version for good", "Check that it holds no private data: names, emails, customer records, internal hostnames, keys.", "Ship it?"},
		{"Ship the knowledge pack runbook?", "runbook (3 MB) becomes public."},
	} {
		for _, w := range want {
			if !strings.Contains((*said)[i], w) {
				t.Errorf("dialog %d does not say %q:\n%s", i, w, (*said)[i])
			}
		}
	}
	var got build.KnowledgeShipping
	if err := json.Unmarshal([]byte(reg.shipping), &got); err != nil {
		t.Fatalf("%v: %q", err, reg.shipping)
	}
	if len(got.Packs) != 2 || got.Packs[0].Name != "glossary" || got.Packs[0].Origin == nil || got.Packs[0].Origin.PackID != "6f1c0000-0000-4000-8000-000000000001" ||
		got.Packs[1].Name != "runbook" || got.Packs[1].Origin != nil {
		t.Fatalf("sent %s", reg.shipping)
	}
}

// A No on a pack sends nothing: no upload, and not even the agent's creation.
func TestSubmitSendsNothingAfterANoOnAPack(t *testing.T) {
	b, _, dir, said := localBuild(t, true)
	b.shipper = newFakeShipper()
	shipFolder(t, b, dir)
	n := 0
	b.app.confirm = func(_ context.Context, title, msg string) (bool, error) {
		*said = append(*said, title+"\n"+msg)
		n++
		return n != 3, nil // the submission, glossary: yes; runbook: no
	}
	reg := startShipRegistry(t, false)
	storeTestToken(t, reg.srvURL)
	res, err := b.Submit(reg.srvURL, dir)
	if err != nil || res != nil {
		t.Fatalf("%+v, %v", res, err)
	}
	if w := reg.sentWrites(); len(w) != 0 {
		t.Fatalf("writes after a No: %v", w)
	}
}

// A private QuickAgent says who can see the pack, since it stays private.
func TestSubmitToAPrivateQuickAgentAsksThePrivateWording(t *testing.T) {
	b, _, dir, said := localBuild(t, true)
	b.shipper = newFakeShipper()
	shipFolder(t, b, dir)
	reg := startShipRegistry(t, true)
	storeTestToken(t, reg.srvURL)
	if _, err := b.Submit(reg.srvURL, dir); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains((*said)[1], "Everyone who can see this QuickAgent can read glossary (1.5 KB).") || strings.Contains((*said)[1], "becomes public") {
		t.Fatalf("dialog: %s", (*said)[1])
	}
}

// A folder whose manifest ships a pack with no files is not sent.
func TestSubmitRefusesAPackThatIsNotInTheFolder(t *testing.T) {
	b, _, dir, _ := localBuild(t, true)
	b.shipper = newFakeShipper()
	shipFolder(t, b, dir)
	if err := os.RemoveAll(filepath.Join(dir, "knowledge", "runbook")); err != nil {
		t.Fatal(err)
	}
	reg := startShipRegistry(t, false)
	storeTestToken(t, reg.srvURL)
	if _, err := b.Submit(reg.srvURL, dir); err == nil || !strings.Contains(err.Error(), "holds no knowledge/runbook") {
		t.Fatalf("%v", err)
	}
	if w := reg.sentWrites(); len(w) != 0 {
		t.Fatalf("writes: %v", w)
	}
}

// A QuickAgent that ships nothing is asked nothing more and sends no part.
func TestSubmitOfAQuickAgentThatShipsNothingAsksNoPackQuestion(t *testing.T) {
	b, _, dir, said := localBuild(t, true)
	reg := startShipRegistry(t, false)
	storeTestToken(t, reg.srvURL)
	m := shipModel(t)
	if err := os.Remove(filepath.Join(dir, "agent.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := b.Save(dir, m); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Submit(reg.srvURL, dir); err != nil {
		t.Fatal(err)
	}
	if len(*said) != 1 || strings.Contains((*said)[0], "knowledge") || reg.shipPart {
		t.Fatalf("dialogs %q, part %v", *said, reg.shipPart)
	}
}

func listOf(packs ...knowledge.Pack) func(context.Context, string) (*knowledge.List, error) {
	return func(context.Context, string) (*knowledge.List, error) { return &knowledge.List{Packs: packs}, nil }
}

// The Knowledge section offers the packs the person keeps here or in Agentry
// and their organizations' packs, each with what decides whether it ships.
func TestShipOptionsReadTheKnowledgeList(t *testing.T) {
	b := &BuildAPI{app: &App{ctx: context.Background()}}
	b.lister = listOf(
		knowledge.Pack{Name: "scanned-here", ID: "kp_1", Local: true, ScanState: knowledge.ScanScanned, Bytes: 10, Pages: 2},
		knowledge.Pack{Name: "unscanned-here", Local: true, ScanState: knowledge.ScanNotScanned},
		knowledge.Pack{Name: "ready-in-agentry", InAgentry: true, AgentryID: "6f1c0000-0000-4000-8000-000000000003", AgentryReady: true, Bytes: 5},
		knowledge.Pack{Name: "scanning-in-agentry", InAgentry: true, ScanState: knowledge.ScanScanning},
		knowledge.Pack{Name: "org-pack", Shared: true, Org: "acme", OrgAdmin: true, InAgentry: true, AgentryReady: true, AgentryID: "6f1c0000-0000-4000-8000-000000000004"},
		knowledge.Pack{Name: "org-copy-only", Shared: true, Org: "acme", OrgAdmin: true, Local: true, ScanState: knowledge.ScanScanned},
		knowledge.Pack{Name: "gone"},
	)
	got, err := b.ShipOptions("https://r.example")
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		name, kind, org string
		ready, admin    bool
	}
	var rows []row
	for _, o := range got {
		rows = append(rows, row{o.Name, o.Kind, o.Org, o.Ready, o.OrgAdmin})
	}
	want := []row{
		{"scanned-here", "own", "", true, false},
		{"unscanned-here", "own", "", false, false},
		{"ready-in-agentry", "own", "", true, false},
		{"scanning-in-agentry", "own", "", false, false},
		{"org-pack", "org", "acme", true, true},
		{"org-copy-only", "org", "acme", false, true},
	}
	if !slices.Equal(rows, want) {
		t.Fatalf("options:\n%+v\nwant\n%+v", rows, want)
	}
	if got[0].ID != "kp_1" || got[0].Bytes != 10 || got[0].Pages != 2 || got[2].AgentryID != "6f1c0000-0000-4000-8000-000000000003" {
		t.Fatalf("sources: %+v %+v", got[0], got[2])
	}
}

// Edit opens with the previous version's packs ticked, tied to the person's
// packs, and names a pack that is gone or cannot ship now.
func TestEditKeepsThePreviousVersionsPacksAndNamesThoseGone(t *testing.T) {
	f := newEditFake(t)
	f.manifest = strings.Replace(editManifest, `"publish":`, `"knowledge": {"ships": [{"name": "glossary"}, {"name": "retired-pack"}, {"name": "unread"}]}, "publish":`, 1)
	b := editAPI(t, f)
	b.lister = listOf(
		knowledge.Pack{Name: "glossary", ID: "kp_1", Local: true, ScanState: knowledge.ScanScanned, Bytes: 2048},
		knowledge.Pack{Name: "unread", Local: true, ScanState: knowledge.ScanScanning},
	)
	v, err := b.EditRelease(f.url, "chris", "meeting-actions")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(v.Model.KnowledgeShips, []string{"glossary"}) || len(v.Model.Ships) != 1 || v.Model.Ships[0].ID != "kp_1" {
		t.Fatalf("ships %v %v", v.Model.KnowledgeShips, v.Model.Ships)
	}
	if len(v.Notes) != 2 || !strings.Contains(v.Notes[0], "retired-pack") || !strings.Contains(v.Notes[1], "unread") {
		t.Fatalf("notes %q", v.Notes)
	}
	// The manifest the form writes ships glossary and packs it.
	b.shipper = newFakeShipper()
	dir := t.TempDir()
	b.app.markPicked(dir)
	if err := b.Save(dir, *v.Model); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	if !strings.Contains(string(raw), "- name: glossary") || !strings.Contains(string(raw), "knowledge/**") || strings.Contains(string(raw), "retired-pack") {
		t.Fatalf("agent.yaml:\n%s", raw)
	}
	if _, err := os.Stat(filepath.Join(dir, "knowledge", "glossary", "wiki", "index.md")); err != nil {
		t.Fatalf("the pack was not read again: %v", err)
	}
}

// A list of packs that cannot be read unticks the previous version's packs
// and says why, rather than carrying them unread.
func TestEditUntickssPacksItCannotReadAndSaysSo(t *testing.T) {
	f := newEditFake(t)
	f.manifest = strings.Replace(editManifest, `"publish":`, `"knowledge": {"ships": [{"name": "glossary"}]}, "publish":`, 1)
	b := editAPI(t, f)
	b.lister = func(context.Context, string) (*knowledge.List, error) { return nil, errors.New("the store is locked") }
	v, err := b.EditRelease(f.url, "chris", "meeting-actions")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Model.KnowledgeShips) != 0 || len(v.Notes) != 1 || !strings.Contains(v.Notes[0], "the store is locked") || !strings.Contains(v.Notes[0], "glossary") {
		t.Fatalf("ships %v, notes %q", v.Model.KnowledgeShips, v.Notes)
	}
}

// A draft ships only packs the person chose, each of them one the draft
// reads and one they may ship in their own namespace.
func TestShipChoicesHoldADraftToPacksThePersonMayShip(t *testing.T) {
	b := &BuildAPI{app: &App{ctx: context.Background()}}
	b.lister = listOf(
		knowledge.Pack{Name: "glossary", ID: "kp_1", Local: true, ScanState: knowledge.ScanScanned, Bytes: 10},
		knowledge.Pack{Name: "runbook", Local: true, ScanState: knowledge.ScanScanning},
		knowledge.Pack{Name: "team", Shared: true, Org: "acme", OrgAdmin: true, InAgentry: true, AgentryReady: true},
		knowledge.Pack{Name: "team", ID: "kp_2", Local: true, ScanState: knowledge.ScanScanned},
	)
	ctx := context.Background()
	got, err := b.shipChoices(ctx, "https://r.example", "chris", []string{"glossary", "team"}, []string{"glossary", "team", "glossary"})
	if err != nil || len(got) != 2 || got[0].Name != "glossary" || got[0].ID != "kp_1" || got[1].Kind != build.ShipOwn {
		t.Fatalf("%+v, %v (an own pack wins over an organization's of the same name)", got, err)
	}
	if got, err := b.shipChoices(ctx, "https://r.example", "acme", []string{"team"}, nil); err != nil || got != nil {
		t.Fatalf("no ships: %+v, %v", got, err)
	}
	for _, tc := range []struct {
		packs, ships []string
		want         string
	}{
		{[]string{"glossary"}, []string{"runbook"}, "runbook, which is not among the packs it reads"},
		{[]string{"runbook"}, []string{"runbook"}, "runbook cannot ship with this QuickAgent: wait until AI Review has read it"},
		{[]string{"nothing"}, []string{"nothing"}, "nothing is not among your knowledge packs"},
		{[]string{"a", "b", "c", "d", "e", "f"}, []string{"a", "b", "c", "d", "e", "f"}, "at most 5 knowledge packs"},
	} {
		if _, err := b.shipChoices(ctx, "https://r.example", "chris", tc.packs, tc.ships); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("ships %v: %v, want %q", tc.ships, err, tc.want)
		}
	}
	// An organization's pack ships into its own namespace only.
	b.lister = listOf(knowledge.Pack{Name: "team", Shared: true, Org: "acme", OrgAdmin: true, InAgentry: true, AgentryReady: true})
	if _, err := b.shipChoices(ctx, "https://r.example", "acme", []string{"team"}, []string{"team"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.shipChoices(ctx, "https://r.example", "chris", []string{"team"}, []string{"team"}); err == nil || !strings.Contains(err.Error(), "ships only into a QuickAgent in the acme namespace") {
		t.Fatalf("%v", err)
	}
	b.lister = func(context.Context, string) (*knowledge.List, error) { return nil, errors.New("locked") }
	if _, err := b.shipChoices(ctx, "https://r.example", "chris", []string{"a"}, []string{"a"}); err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Fatalf("%v", err)
	}
}

// The draft's dialog names a pack it ships, and a ship the draft does not
// read is refused before any dialog is asked.
func TestDescribeOnThisComputerRefusesAShipItDoesNotRead(t *testing.T) {
	f := newDescribeFixture(t)
	f.b.lister = listOf(knowledge.Pack{Name: "glossary", Local: true, ScanState: knowledge.ScanScanned})
	_, err := f.b.DescribeOnThisComputer("https://registry.example", "Summarize my meeting notes.", nil, nil, []string{"glossary"})
	if err == nil || !strings.Contains(err.Error(), "not among the packs it reads") {
		t.Fatalf("%v", err)
	}
	if len(f.asked) != 0 || len(f.runs) != 0 {
		t.Fatalf("dialogs %q, runs %d", f.asked, len(f.runs))
	}
}

func TestShipConsentNamesTheChosenPacks(t *testing.T) {
	if got := shipConsent(nil); got != "" {
		t.Fatalf("%q", got)
	}
	got := shipConsent([]build.ShipSource{{Name: "a"}, {Name: "b"}, {Name: "c"}})
	if !strings.HasPrefix(got, "You chose to ship a, b and c with the QuickAgent as well") || !strings.Contains(got, "Submit asks you to confirm each pack then") {
		t.Fatalf("%q", got)
	}
}
