package build

import (
	"context"
	"encoding/json"
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
)

func shipOpt(name, kind, org string, bytes int64, ready, admin bool) ShipOption {
	return ShipOption{ShipSource: ShipSource{Name: name, Kind: kind, Org: org, Bytes: bytes}, Ready: ready, OrgAdmin: admin}
}

func TestPackSizeIsInTheUnitsTheKnowledgePagesCountIn(t *testing.T) {
	for n, want := range map[int64]string{
		0: "0 B", 512: "512 B", 1024: "1 KB", 1536: "1.5 KB", 10 << 20: "10 MB", 3<<20 + 512<<10: "3.5 MB",
		1<<20 - 1: "1 MB", 5 << 30: "5 GB",
	} {
		if got := PackSize(n); got != want {
			t.Errorf("PackSize(%d) = %q, want %q", n, got, want)
		}
	}
}

// The confirmation names the pack and its size, and says who can read it, in
// the words every client uses; a private QuickAgent says who can see it, and
// Make public names the versions that already ship the pack.
func TestShipConfirmationWordsCoverPublicPrivateAndVersions(t *testing.T) {
	const check = "Check that it holds no private data: names, emails, customer records, internal hostnames, keys."
	pub := ShipConfirmation("aws-docs", 2<<20, false, nil)
	if pub.Lead != "aws-docs (2 MB) becomes public." ||
		pub.Rest != "Anyone can download it with this release, and it stays in this version for good: retiring or deleting the QuickAgent later does not recall copies already downloaded. "+check {
		t.Errorf("public: %+v", pub)
	}
	priv := ShipConfirmation("aws-docs", 2<<20, true, nil)
	if priv.Lead != "Everyone who can see this QuickAgent can read aws-docs (2 MB)." ||
		!strings.HasPrefix(priv.Rest, "They can download it with this release, and it stays in this version for good") || !strings.HasSuffix(priv.Rest, check) {
		t.Errorf("private: %+v", priv)
	}
	if got := ShipConfirmation("notes", -1, false, []string{"0.2.0"}); got.Lead != "notes becomes public." ||
		!strings.HasPrefix(got.Rest, "Anyone can download it with version 0.2.0, and it stays in that version for good") {
		t.Errorf("one version, no size: %+v", got)
	}
	if got := ShipConfirmation("notes", 0, false, []string{"0.2.0", "0.3.0"}); got.Lead != "notes (0 B) becomes public." ||
		!strings.HasPrefix(got.Rest, "Anyone can download it with versions 0.2.0, 0.3.0, and it stays in those versions for good") {
		t.Errorf("two versions: %+v", got)
	}
	if got := pub.Text(); got != pub.Lead+" "+pub.Rest {
		t.Errorf("text %q", got)
	}
}

// An organization's pack ships only from its administrator into a QuickAgent
// in its own namespace; any pack only once AI Review has read it and within
// the cap of one pack.
func TestAPackShipsOnlyWhereTheRegistryWouldTakeIt(t *testing.T) {
	for _, tc := range []struct {
		name string
		opt  ShipOption
		ns   string
		why  string
	}{
		{"own and ready", shipOpt("a", ShipOwn, "", 1<<20, true, false), "chris", ""},
		{"own, not read yet", shipOpt("a", ShipOwn, "", 1<<20, false, false), "chris", "wait until AI Review has read it, then it can ship"},
		{"own, past the cap", shipOpt("a", ShipOwn, "", 11<<20, true, false), "chris", "it holds 11 MB, and a release ships at most 10 MB of one pack"},
		{"org admin, its namespace", shipOpt("a", ShipOrg, "acme", 1<<20, true, true), "acme", ""},
		{"org member, not admin", shipOpt("a", ShipOrg, "acme", 1<<20, true, false), "acme", "only an administrator of acme can ship its pack"},
		{"org admin, another namespace", shipOpt("a", ShipOrg, "acme", 1<<20, true, true), "chris", "ships only into a QuickAgent in the acme namespace, and this one is in chris"},
		{"org admin, no namespace", shipOpt("a", ShipOrg, "acme", 1<<20, true, true), "", "ships only into a QuickAgent in the acme namespace, and this one is in no namespace yet"},
	} {
		if got := tc.opt.Reason(tc.ns); got != tc.why {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.why)
		}
	}
}

func TestTickAndUntickKeepTheNamesTheSourcesAndTheIncludeList(t *testing.T) {
	m := Default("chris")
	m.Name, m.Summary = "ship-check", "Ships packs."
	if got := m.defaultInclude(); slices.Contains(got, "knowledge/**") {
		t.Fatalf("a QuickAgent that ships nothing packs knowledge/**: %v", got)
	}
	for _, n := range []string{"one", "two", "three", "four", "five"} {
		if err := m.TickShip(ShipSource{Name: n, Kind: ShipOwn, Bytes: 10}); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.TickShip(ShipSource{Name: "six", Kind: ShipOwn}); err == nil || !strings.Contains(err.Error(), "at most 5") {
		t.Fatalf("a sixth pack: %v", err)
	}
	// Ticking one again refreshes its source and adds no name.
	if err := m.TickShip(ShipSource{Name: "two", Kind: ShipOwn, Bytes: 99}); err != nil {
		t.Fatal(err)
	}
	if len(m.KnowledgeShips) != 5 || len(m.Ships) != 5 {
		t.Fatalf("ships %v, sources %v", m.KnowledgeShips, m.Ships)
	}
	if s, _ := m.shipSource("two"); s.Bytes != 99 {
		t.Fatalf("source not refreshed: %+v", s)
	}
	if got := m.defaultInclude(); !slices.Contains(got, "knowledge/**") {
		t.Fatalf("include %v", got)
	}
	m.UntickShip("one")
	if !slices.Equal(m.KnowledgeShips, []string{"two", "three", "four", "five"}) || len(m.Ships) != 4 {
		t.Fatalf("after untick: %v %v", m.KnowledgeShips, m.Ships)
	}

	// A list that came with the workspace gets the glob when a pack is
	// ticked, and loses it when the last one is unticked.
	custom := Default("chris")
	custom.PublishInclude = []string{"agent.yaml", "AGENT.md", "extra/**"}
	if err := custom.TickShip(ShipSource{Name: "glossary", Kind: ShipOwn}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(custom.PublishInclude, []string{"agent.yaml", "AGENT.md", "extra/**", "knowledge/**"}) {
		t.Fatalf("custom include %v", custom.PublishInclude)
	}
	custom.UntickShip("glossary")
	if !slices.Equal(custom.PublishInclude, []string{"agent.yaml", "AGENT.md", "extra/**"}) {
		t.Fatalf("custom include after untick %v", custom.PublishInclude)
	}
}

// The form writes the packs it ships, and the file it writes passes the
// registry's own rules for a manifest (the shipped names and the include
// list that packs them).
func TestAFormThatShipsPacksWritesAManifestThatParses(t *testing.T) {
	m := model()
	m.KnowledgeShips, m.PublishInclude = nil, nil
	for _, n := range []string{"glossary", "runbook"} {
		if err := m.TickShip(ShipSource{Name: n, Kind: ShipOwn, Bytes: 1 << 10}); err != nil {
			t.Fatal(err)
		}
	}
	if c := m.Check(); c.First != nil {
		t.Fatalf("check: %+v", c.First)
	}
	dir := t.TempDir()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	got, err := ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.ShippedPacks(), []string{"glossary", "runbook"}) || !slices.Contains(got.Publish.Include, "knowledge/**") {
		t.Fatalf("manifest ships %v, include %v", got.ShippedPacks(), got.Publish.Include)
	}
	names, err := ShippedNames(dir)
	if err != nil || !slices.Equal(names, []string{"glossary", "runbook"}) {
		t.Fatalf("ShippedNames %v, %v", names, err)
	}
}

func TestTheFormRefusesASixthPackADuplicateAndABadName(t *testing.T) {
	m := model()
	m.KnowledgeShips = []string{"a", "b", "c", "d", "e", "f"}
	if c := m.Check(); c.First == nil || !strings.Contains(c.First.Problem, "at most 5 knowledge packs") {
		t.Fatalf("six packs: %+v", c.First)
	}
	m.KnowledgeShips = []string{"a", "a"}
	if c := m.Check(); c.First == nil || !strings.Contains(c.First.Problem, "shipped twice") {
		t.Fatalf("a duplicate: %+v", c.First)
	}
	m.KnowledgeShips = []string{"../etc"}
	if c := m.Check(); c.First == nil || !strings.Contains(c.First.Problem, "not a knowledge pack's name") {
		t.Fatalf("a bad name: %+v", c.First)
	}
}

// Edit ties the previous version's packs to the person's packs now: one the
// person can still ship stays ticked, and any other is unticked and named.
func TestReconcileShipsKeepsWhatCanShipAndNamesWhatCannot(t *testing.T) {
	m := Default("acme")
	m.KnowledgeShips = []string{"kept", "gone", "unread", "org-pack", "elsewhere"}
	opts := []ShipOption{
		shipOpt("kept", ShipOwn, "", 5, true, false),
		shipOpt("unread", ShipOwn, "", 5, false, false),
		shipOpt("org-pack", ShipOrg, "acme", 5, true, true),
		shipOpt("elsewhere", ShipOrg, "other", 5, true, true),
	}
	notes := m.ReconcileShips(opts)
	if !slices.Equal(m.KnowledgeShips, []string{"kept", "org-pack"}) {
		t.Fatalf("ticked %v", m.KnowledgeShips)
	}
	if len(m.Ships) != 2 || m.Ships[1].Org != "acme" {
		t.Fatalf("sources %v", m.Ships)
	}
	want := []string{
		"The previous version shipped the knowledge pack gone. It is not among your packs now, so it is unticked and this version will not ship it.",
		"The previous version shipped the knowledge pack unread. It cannot ship now (wait until AI Review has read it, then it can ship), so it is unticked.",
		"The previous version shipped the knowledge pack elsewhere. It cannot ship now (ships only into a QuickAgent in the other namespace, and this one is in acme), so it is unticked.",
	}
	if !slices.Equal(notes, want) {
		t.Fatalf("notes:\n%q\nwant\n%q", notes, want)
	}
}

// A pack of the same name that the person may ship wins over one they may
// not: the organization's name may match the person's own.
func TestReconcileShipsPrefersThePackThePersonMayShip(t *testing.T) {
	m := Default("chris")
	m.KnowledgeShips = []string{"runbook"}
	notes := m.ReconcileShips([]ShipOption{
		shipOpt("runbook", ShipOrg, "acme", 5, true, false),
		shipOpt("runbook", ShipOwn, "", 5, true, false),
	})
	if len(notes) != 0 || len(m.Ships) != 1 || m.Ships[0].Kind != ShipOwn {
		t.Fatalf("notes %q, sources %v", notes, m.Ships)
	}
}

// A draft ships exactly the packs its person chose.
func TestAFormFromADraftShipsExactlyWhatWasChosen(t *testing.T) {
	m := Default("alice")
	m.KnowledgeShips = []string{"from-draft", "chosen"}
	chosen := []ShipSource{{Name: "chosen", Kind: ShipOwn, Bytes: 7}, {Name: "missed", Kind: ShipOwn, Bytes: 9}}
	notes := m.KeepChosenShips(chosen)
	if !slices.Equal(m.KnowledgeShips, []string{"chosen", "missed"}) || len(m.Ships) != 2 {
		t.Fatalf("ships %v %v", m.KnowledgeShips, m.Ships)
	}
	if len(notes) != 2 ||
		notes[0] != "The draft listed the knowledge pack from-draft to ship, which you did not choose to ship, so it was left out." ||
		!strings.HasPrefix(notes[1], "The draft did not list the knowledge pack missed, which you chose to ship, so it was added.") {
		t.Fatalf("notes %q", notes)
	}
	// Choosing nothing ships nothing.
	m = Default("alice")
	m.KnowledgeShips = []string{"from-draft"}
	if notes := m.KeepChosenShips(nil); len(m.KnowledgeShips) != 0 || len(notes) != 1 {
		t.Fatalf("nothing chosen: %v %q", m.KnowledgeShips, notes)
	}
}

func TestFromDraftKeepsTheChosenShipsAndNamesTheRest(t *testing.T) {
	withShips := strings.Replace(draftManifest, `"publish":`,
		`"knowledge": {"ships": [{"name": "glossary"}, {"name": "stray"}]}, "publish":`, 1)
	m, notes, err := FromDraft([]byte(withShips), "# Runbook answers\n", "alice", ShipSource{Name: "glossary", Kind: ShipOwn, Bytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.KnowledgeShips, []string{"glossary"}) || len(m.Ships) != 1 || m.Ships[0].Bytes != 4096 {
		t.Fatalf("ships %v %v", m.KnowledgeShips, m.Ships)
	}
	if !slices.Contains(notes, "The draft listed the knowledge pack stray to ship, which you did not choose to ship, so it was left out.") {
		t.Fatalf("notes %q", notes)
	}
	if !slices.Contains(m.defaultInclude(), "knowledge/**") || !strings.Contains(m.manifestYAML(), "- name: glossary") {
		t.Fatalf("manifest:\n%s", m.manifestYAML())
	}
	// With none chosen, a draft that lists one ships none.
	m, _, err = FromDraft([]byte(withShips), "# Runbook answers\n", "alice")
	if err != nil || len(m.KnowledgeShips) != 0 || strings.Contains(m.manifestYAML(), "ships") {
		t.Fatalf("unchosen: %v %v\n%s", err, m.KnowledgeShips, m.manifestYAML())
	}
}

func TestTheSummaryNamesEachShippedPackWithItsSize(t *testing.T) {
	m := bare()
	if err := m.TickShip(ShipSource{Name: "glossary", Kind: ShipOwn, Bytes: 1536}); err != nil {
		t.Fatal(err)
	}
	if err := m.TickShip(ShipSource{Name: "runbook", Kind: ShipOwn, Bytes: 5 << 20}); err != nil {
		t.Fatal(err)
	}
	m.KnowledgeShips = append(m.KnowledgeShips, "by-hand") // its files are the folder's own: no size
	got := m.Summarize(nil).Knowledge
	want := []string{
		"Ships the knowledge packs glossary (1.5 KB), runbook (5 MB), by-hand in the release, unencrypted: anyone who can download it can read them.",
		"Submit asks you to confirm, for each pack, that it holds no private data.",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("summary:\n%q\nwant\n%q", got, want)
	}
}

// A fake registry for the upload: it records what the version create carried.
type shipRegistry struct {
	mu       sync.Mutex
	requests []string
	shipping string
	hasPart  bool
	partType string
	refuse   func(w http.ResponseWriter)
}

func (s *shipRegistry) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/agents/acme/hello-build", func(w http.ResponseWriter, _ *http.Request) {
		s.record("GET agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"namespace":"acme","name":"hello-build","visibility":"public","modes":["autonomous"],"summary":"s","quarantineState":"none","createdAt":"2026-08-31T00:00:00Z"}`))
	})
	mux.HandleFunc("POST /api/v1/agents/acme/hello-build/versions", func(w http.ResponseWriter, r *http.Request) {
		s.record("POST version")
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Errorf("content type: %v", err)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("part: %v", err)
				break
			}
			if p.FormName() == "knowledgeShipping" {
				b, _ := io.ReadAll(p)
				s.mu.Lock()
				s.hasPart, s.shipping, s.partType = true, string(b), p.Header.Get("Content-Type")
				s.mu.Unlock()
			}
		}
		if s.refuse != nil {
			s.refuse(w)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"version":"0.1.0","status":"published","channel":"stable","createdAt":"2026-08-31T00:00:00Z"}`))
	})
	mux.HandleFunc("GET /api/v1/agents/acme/hello-build/versions/0.1.0", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"0.1.0","status":"published","channel":"stable","createdAt":"2026-08-31T00:00:00Z"}`))
	})
	mux.HandleFunc("GET /api/v1/agents/acme/hello-build/versions/0.1.0/report", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"versionStatus":"published","findings":[],"findingCounts":{}}`))
	})
	return mux
}

func (s *shipRegistry) record(r string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, r)
}

// shippingWorkspace is a saved workspace whose manifest ships the named
// packs, each with a file under knowledge/<name>/.
func shippingWorkspace(t *testing.T, names ...string) string {
	t.Helper()
	m := model()
	m.KnowledgeShips, m.PublishInclude = nil, nil
	for _, n := range names {
		if err := m.TickShip(ShipSource{Name: n, Kind: ShipOwn, Bytes: 11}); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		d := filepath.Join(dir, "knowledge", n)
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "manifest.json"), []byte(`{"schema":"knowledge/v1","name":"`+n+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The upload carries the confirmation as a JSON part, one entry for each
// shipped pack, the origin where the pack came from a stored one.
func TestPublishSendsTheConfirmationOfEachShippedPack(t *testing.T) {
	dir := shippingWorkspace(t, "glossary", "runbook")
	reg := &shipRegistry{}
	srv := httptest.NewServer(reg.handler(t))
	defer srv.Close()
	ack := AcknowledgementFor([]string{"glossary", "runbook"}, map[string]*PackOrigin{
		"runbook": {Kind: "org", Org: "acme", PackID: "6f1c0000-0000-4000-8000-000000000001"},
	})
	res, err := PublishWith(t.Context(), srv.URL, "agy_test", dir, false, PublishOptions{Shipping: ack}, io.Discard)
	if err != nil || res.Status != "published" {
		t.Fatalf("%+v, %v", res, err)
	}
	if !reg.hasPart || !strings.HasPrefix(reg.partType, "application/json") {
		t.Fatalf("part present %v, type %q", reg.hasPart, reg.partType)
	}
	var got KnowledgeShipping
	if err := json.Unmarshal([]byte(reg.shipping), &got); err != nil {
		t.Fatalf("%v: %s", err, reg.shipping)
	}
	if len(got.Packs) != 2 || got.Packs[0].Name != "glossary" || got.Packs[0].Origin != nil ||
		got.Packs[1].Origin == nil || got.Packs[1].Origin.Kind != "org" || got.Packs[1].Origin.Org != "acme" {
		t.Fatalf("sent %s", reg.shipping)
	}
	if strings.Contains(reg.shipping, `"origin":null`) {
		t.Fatalf("an empty origin was sent as null: %s", reg.shipping)
	}
}

// A release that ships a pack is never sent without the confirmation, and
// nothing is sent at all, not even the request that creates the agent.
func TestPublishRefusesAShippingReleaseWithoutItsConfirmation(t *testing.T) {
	dir := shippingWorkspace(t, "glossary")
	reg := &shipRegistry{}
	srv := httptest.NewServer(reg.handler(t))
	defer srv.Close()
	for what, opts := range map[string]PublishOptions{
		"none":          {},
		"another pack":  {Shipping: AcknowledgementFor([]string{"runbook"}, nil)},
		"a missing one": {Shipping: AcknowledgementFor(nil, nil)},
		"an extra one":  {Shipping: AcknowledgementFor([]string{"glossary", "runbook"}, nil)},
	} {
		res, err := PublishWith(t.Context(), srv.URL, "agy_test", dir, false, opts, io.Discard)
		if err == nil || res != nil {
			t.Fatalf("%s: sent %+v, %v", what, res, err)
		}
	}
	if len(reg.requests) != 0 {
		t.Fatalf("requests were made: %v", reg.requests)
	}
	// And the other way: a release that ships nothing sends no part, and a
	// confirmation for a pack it does not ship is not sent.
	plain := t.TempDir()
	if err := Save(plain, model()); err != nil {
		t.Fatal(err)
	}
	m, _ := ReadManifest(plain)
	if len(m.ShippedPacks()) != 0 {
		t.Fatal("the fixture ships a pack")
	}
	if _, err := PublishWith(t.Context(), srv.URL, "agy_test", plain, false, PublishOptions{Shipping: AcknowledgementFor([]string{"glossary"}, nil)}, io.Discard); err == nil {
		t.Fatal("a confirmation for a pack the release does not ship was sent")
	}
	if _, err := PublishWith(t.Context(), srv.URL, "agy_test", plain, false, PublishOptions{}, io.Discard); err != nil {
		t.Fatal(err)
	}
	if reg.hasPart {
		t.Fatal("a release that ships nothing sent the part")
	}
}

// The registry's refusals of the confirmation come back as a refused result
// that names the pack, not as an error.
func TestPublishSaysWhichPackTheRegistryRefused(t *testing.T) {
	dir := shippingWorkspace(t, "glossary")
	reg := &shipRegistry{refuse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"type":"https://agentry.dev/problems/knowledge-shipping-invalid","title":"The confirmation is not true","detail":"glossary is not one of your packs","status":422,"packs":["glossary"]}`))
	}}
	srv := httptest.NewServer(reg.handler(t))
	defer srv.Close()
	var log strings.Builder
	res, err := PublishWith(context.Background(), srv.URL, "agy_test", dir, false,
		PublishOptions{Shipping: AcknowledgementFor([]string{"glossary"}, map[string]*PackOrigin{"glossary": {Kind: "own", PackID: "6f1c0000-0000-4000-8000-000000000002"}})}, &log)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != StatusRefused || res.Refusal != "knowledge-shipping-invalid" || !strings.Contains(res.Detail, "glossary is not one of your packs") {
		t.Fatalf("%+v", res)
	}
}
