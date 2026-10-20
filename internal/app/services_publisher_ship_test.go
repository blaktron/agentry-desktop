package app

// Make public for a QuickAgent whose versions ship knowledge packs: the
// registry refuses until the publisher has confirmed each pack, and the
// desktop asks that in a native dialog per pack, then sends the confirmation
// with the same request.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// shipFake is a registry that holds a QuickAgent private with versions that
// ship packs, and makes it public only once the request carries the
// confirmation, as the core does.
type shipFake struct {
	mu       sync.Mutex
	patches  []string
	versions []map[string]any
	// files is the listing of each version's files.
	files map[string][]map[string]any
}

func (f *shipFake) start(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/agents/{ns}/{name}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.patches = append(f.patches, strings.TrimSpace(string(body)))
		f.mu.Unlock()
		var req struct {
			Visibility string `json:"visibility"`
			Ack        bool   `json:"acknowledgeShippedKnowledge"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Visibility == "public" && !req.Ack && len(f.versions) > 0 {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type": "https://agentry.dev/problems/shipped-knowledge-unacknowledged", "title": "Shipped knowledge is not confirmed",
				"status": 409, "detail": "Once this QuickAgent is public, everyone can download what its versions ship.", "versions": f.versions,
			})
			return
		}
		hostedJSON(w, http.StatusOK, publisherAgentDoc(r.PathValue("ns"), r.PathValue("name"), req.Visibility, nil))
	})
	mux.HandleFunc("GET /api/v1/agents/{ns}/{name}/versions/{version}/files", func(w http.ResponseWriter, r *http.Request) {
		files, ok := f.files[r.PathValue("version")]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		hostedJSON(w, http.StatusOK, map[string]any{"files": files})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func (f *shipFake) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.patches...)
}

func newShipFake() *shipFake {
	return &shipFake{
		versions: []map[string]any{
			{"version": "0.2.0", "packs": []string{"glossary", "runbook"}},
			{"version": "0.3.0", "packs": []string{"glossary"}},
		},
		files: map[string][]map[string]any{
			"0.2.0": {
				{"path": "agent.yaml", "size": 400},
				{"path": "knowledge/glossary/manifest.json", "size": 90},
				{"path": "knowledge/glossary/wiki/index.md", "size": 1024},
				{"path": "knowledge/glossary/sources/a.txt", "size": 512},
				{"path": "knowledge/runbook/wiki/index.md", "size": 2 << 20},
			},
			"0.3.0": {
				{"path": "knowledge/glossary/wiki/index.md", "size": 1536},
			},
		},
	}
}

// Each pack is confirmed on its own in a native dialog, in the words every
// client uses, naming the pack, its size and the versions that ship it; the
// confirmation goes back with the same request and the QuickAgent is public.
func TestMakePublicAsksTheConfirmationOfEachShippedPack(t *testing.T) {
	f := newShipFake()
	reg := f.start(t)
	signIn(t, reg)
	p, asked := newPublisherAPI(true)

	done, err := p.SetVisibility(reg, "pat", "kept", "public")
	if err != nil || done.Cancelled || !strings.Contains(done.Sentence, "pat/kept is public now") {
		t.Fatalf("%+v, %v", done, err)
	}
	// The first request carries no confirmation: the page cannot make one.
	want := []string{`{"visibility":"public"}`, `{"acknowledgeShippedKnowledge":true,"visibility":"public"}`}
	if got := f.sent(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(*asked) != 3 {
		t.Fatalf("dialogs: %q", *asked)
	}
	for i, w := range []string{
		"Make pat/kept public? | ",
		"Make the knowledge pack glossary public? | glossary (1.5 KB) becomes public. Anyone can download it with versions 0.2.0, 0.3.0, and it stays in those versions for good: retiring or deleting the QuickAgent later does not recall copies already downloaded. Check that it holds no private data: names, emails, customer records, internal hostnames, keys.\n\nMake pat/kept public, with it?",
		"Make the knowledge pack runbook public? | runbook (2 MB) becomes public. Anyone can download it with version 0.2.0, and it stays in that version for good",
	} {
		if !strings.HasPrefix((*asked)[i], w) {
			t.Errorf("dialog %d:\n%s\nwant it to start\n%s", i, (*asked)[i], w)
		}
	}
}

// A No on any pack changes nothing: the confirmation is never sent.
func TestMakePublicSendsNothingMoreAfterANoOnAPack(t *testing.T) {
	f := newShipFake()
	reg := f.start(t)
	signIn(t, reg)
	p, asked := newPublisherAPI(true)
	n := 0
	p.app.confirm = func(_ context.Context, title, message string) (bool, error) {
		*asked = append(*asked, title+" | "+message)
		n++
		return n < 3, nil // yes to Make public and the first pack, no to the second
	}
	done, err := p.SetVisibility(reg, "pat", "kept", "public")
	if err != nil || done == nil || !done.Cancelled {
		t.Fatalf("%+v, %v", done, err)
	}
	for _, s := range f.sent() {
		if strings.Contains(s, "acknowledgeShippedKnowledge") {
			t.Fatalf("a confirmation was sent after a No: %v", f.sent())
		}
	}
	if len(f.sent()) != 1 {
		t.Fatalf("requests: %v", f.sent())
	}
}

// A size the registry's listing does not give is left out of the sentence,
// not guessed.
func TestMakePublicNamesAPackWhoseSizeIsNotKnown(t *testing.T) {
	f := newShipFake()
	f.files = nil
	reg := f.start(t)
	signIn(t, reg)
	p, asked := newPublisherAPI(true)
	if _, err := p.SetVisibility(reg, "pat", "kept", "public"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains((*asked)[1], "glossary becomes public.") || strings.Contains((*asked)[1], "glossary (") {
		t.Fatalf("dialog: %s", (*asked)[1])
	}
}

// A QuickAgent that ships nothing is made public in one request with no
// confirmation, and making one private never carries it.
func TestMakePublicAsksNothingExtraWhenNothingIsShipped(t *testing.T) {
	f := newShipFake()
	f.versions = nil
	reg := f.start(t)
	signIn(t, reg)
	p, asked := newPublisherAPI(true)
	if _, err := p.SetVisibility(reg, "pat", "kept", "public"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SetVisibility(reg, "pat", "kept", "private"); err != nil {
		t.Fatal(err)
	}
	if got := f.sent(); len(got) != 2 || got[0] != `{"visibility":"public"}` || got[1] != `{"visibility":"private"}` || len(*asked) != 2 {
		t.Fatalf("requests %v, dialogs %q", got, *asked)
	}
}

// A refusal that names no pack is not one the person can confirm.
func TestMakePublicRefusesAConfirmationThatNamesNoPack(t *testing.T) {
	f := newShipFake()
	f.versions = []map[string]any{{"version": "0.2.0", "packs": []string{}}}
	reg := f.start(t)
	signIn(t, reg)
	p, _ := newPublisherAPI(true)
	if _, err := p.SetVisibility(reg, "pat", "kept", "public"); err == nil || !strings.Contains(err.Error(), "did not say which") {
		t.Fatalf("%v", err)
	}
	if len(f.sent()) != 1 {
		t.Fatalf("requests: %v", f.sent())
	}
}
