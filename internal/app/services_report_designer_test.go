package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry/pkg/reportdesign"

	"github.com/blaktron/agentry-cli/pkg/reportdesigner/build"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"

	"github.com/blaktron/agentry-desktop/internal/designer"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
)

// designerFake is a registry for Design with AI: the report design library
// (the library fake behind it) and the runs extension's status, inputs,
// create, list, read and cancel for a design_build.
type designerFake struct {
	mu      sync.Mutex
	lib     *fakeLibrary
	srv     *httptest.Server
	scopes  []string
	enabled bool
	// document says the status route offers designBuild.document.
	document bool
	inputs   []string
	creates  []string
	run      *hostedrun.Run
}

func newDesignerFake(t *testing.T) *designerFake {
	t.Helper()
	f := &designerFake{lib: newFakeLibrary(t, true), enabled: true,
		scopes: []string{hostedrun.ScopeRunRead, hostedrun.ScopeRunWrite, hostedrun.ScopeUserProfile}}
	target, _ := url.Parse(f.lib.srv.URL)
	library := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.SetURL(target)
		if r.In.Header.Get("Authorization") == "Bearer "+hostedTestToken {
			r.Out.Header.Set("Authorization", "Bearer agy_test")
		}
	}}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/user/report-designs", library)
	mux.Handle("/api/v1/user/report-designs/", library)
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"version": "1", "apiVersion": "v1", "features": map[string]bool{}, "extensions": []string{"runs", "billing"}})
	})
	mux.HandleFunc("GET /api/v1/user/tokens", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, []map[string]any{{"id": "00000000-0000-0000-0000-0000000000a2", "name": "Agentry Desktop",
			"prefix": hostedTestToken[:12], "scopes": f.scopes, "expiresAt": "2027-01-01T00:00:00Z", "createdAt": "2026-01-01T00:00:00Z"}})
	})
	mux.HandleFunc("GET /api/v1/x/billing/account", func(w http.ResponseWriter, r *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{
			"tier": "standard", "balance": 180, "allowance": 50, "submissionCost": 10,
			"periodEnd": "2026-12-01T00:00:00Z", "privateAgents": map[string]any{"max": 10, "used": 1},
			"upgrade": map[string]any{"available": true, "manage": false, "url": "http://" + r.Host + "/settings/subscription"},
		})
	})
	mux.HandleFunc("GET /api/v1/x/runs/status", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		terms := map[string]any{"enabled": f.enabled, "credits": 20, "caps": map[string]int{"timeoutSeconds": 1800}, "maxImages": 4,
			"maxBriefBytes": 4096, "ops": []string{"design", "cover"}, "urls": map[string]any{"enabled": true, "max": 2, "maxPages": 60}}
		if f.document {
			terms["document"] = map[string]any{"enabled": true, "maxBytes": 20 << 20, "types": []string{"pdf", "docx"}}
		}
		hostedJSON(w, http.StatusOK, map[string]any{"enabled": true, "hostsOnline": 1,
			"designBuild": terms,
			"caller":      map[string]any{"tier": "standard", "designBuild": map[string]int{"holdCredits": 20}}})
	})
	mux.HandleFunc("POST /api/v1/x/runs/inputs", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		id := "0000000" + string(rune('1'+len(f.inputs))) + "-0000-4000-8000-000000000000"
		kind := r.URL.Query().Get("kind")
		f.inputs = append(f.inputs, kind+":"+r.URL.Query().Get("name")+"="+string(body))
		out := map[string]any{"id": id, "name": r.URL.Query().Get("name"), "bytes": len(body)}
		if kind == "document" {
			out["document"] = "pdf"
		}
		hostedJSON(w, http.StatusCreated, out)
	})
	mux.HandleFunc("POST /api/v1/x/runs/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var cr hostedrun.CreateRequest
		_ = json.Unmarshal(body, &cr)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.creates = append(f.creates, string(body))
		f.run = &hostedrun.Run{ID: "01a1ad22-83e0-7f23-ae55-edb2187c4a3b", Kind: hostedrun.KindDesignBuild, Status: hostedrun.StatusQueued,
			HoldCredits: 20, CreatedAt: time.Now(), DesignBuild: &hostedrun.DesignBuild{Op: cr.Op, From: cr.From}}
		hostedJSON(w, http.StatusCreated, f.run)
	})
	mux.HandleFunc("GET /api/v1/x/runs/", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		runs := []*hostedrun.Run{}
		if f.run != nil {
			runs = append(runs, f.run)
		}
		hostedJSON(w, http.StatusOK, map[string]any{"items": runs})
	})
	mux.HandleFunc("GET /api/v1/x/runs/{id}", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, f.run)
	})
	mux.HandleFunc("POST /api/v1/x/runs/{id}/cancel", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.run.CancelRequested = true
		hostedJSON(w, http.StatusOK, f.run)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// finish ends the run as the registry would: saved as a new design, or
// with a note and refunded.
func (f *designerFake) finish(applied bool, designID, name, note string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	settled := int64(20)
	if !applied {
		settled = 0
	}
	f.run.Status, f.run.Outcome, f.run.FinishedAt, f.run.SettledCredits = hostedrun.StatusFinished, hostedrun.OutcomeSucceeded, &now, &settled
	f.run.DesignBuild.Applied, f.run.DesignBuild.DesignID, f.run.DesignBuild.Name, f.run.DesignBuild.Note = applied, designID, name, note
}

// designerEvents collects designer:hosted and designer:log.
type designerEvents struct {
	mu     sync.Mutex
	hosted []*DesignerHostedView
	log    []string
}

func newDesignerAPI(t *testing.T) (*DesignerAPI, *designerEvents) {
	t.Helper()
	ev := &designerEvents{}
	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		ev.mu.Lock()
		defer ev.mu.Unlock()
		switch name {
		case eventDesignerHosted:
			ev.hosted = append(ev.hosted, data[0].(*DesignerHostedView))
		case eventDesignerLog:
			ev.log = append(ev.log, data[0].(string))
		}
	}
	t.Cleanup(func() { emitEvent = old })
	a := &App{ctx: context.Background()}
	return &DesignerAPI{app: a, run: &RunAPI{app: a}}, ev
}

func (e *designerEvents) final(t *testing.T) *DesignerHostedView {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		for _, v := range e.hosted {
			if v.Final {
				e.mu.Unlock()
				return v
			}
		}
		e.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the run never reported a final state")
	return nil
}

// pickedImages writes images and marks them as a native picker's.
func pickedImages(t *testing.T, a *App, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	var out []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, testPNG(t, 40, 20), 0o600); err != nil {
			t.Fatal(err)
		}
		a.markPicked(p)
		out = append(out, p)
	}
	return out
}

// The dialog is told both places: on this computer with the model, and In
// Agentry with the registry's flat price, what this payer holds and the
// balance, the images and addresses it takes; and In Agentry is held, with
// a reason, where the library is full or the token lacks user:profile.
func TestDesignWithAIIsOfferedInBothPlaces(t *testing.T) {
	f := newDesignerFake(t)
	signIn(t, f.srv.URL)
	a, _ := newDesignerAPI(t)
	v, err := a.Availability(f.srv.URL, designer.JobDesign)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Offered || !v.Available || v.Credits != 20 || v.HoldCredits != 20 || !v.BalanceKnown || v.Balance != 180 ||
		v.HostedMaxImages != 4 || !v.HostedURLs || v.HostedMaxURLs != 2 || v.MaxReferences != 4 || v.MaxURLs != 2 || v.MaxBriefBytes != 4096 {
		t.Fatalf("ready: %+v", v)
	}
	if !v.DocumentHere || v.DocumentInAgentry || v.MaxDocumentBytes != 20<<20 {
		t.Fatalf("a document is offered on this computer alone: %+v", v)
	}
	f.mu.Lock()
	f.scopes = f.scopes[:2]
	f.mu.Unlock()
	if v, _ := a.Availability(f.srv.URL, designer.JobCover); !v.Offered || v.Available || !strings.Contains(v.Reason, "user:profile") {
		t.Fatalf("no user:profile: %+v", v)
	}
	f.mu.Lock()
	f.scopes = append(f.scopes, hostedrun.ScopeUserProfile)
	f.mu.Unlock()
	for i := 0; i < 10; i++ {
		f.lib.add(builtin(t, "plain"), false)
	}
	if v, _ := a.Availability(f.srv.URL, designer.JobDesign); v.Available || !strings.Contains(v.Reason, "nowhere to save") {
		t.Fatalf("full: %+v", v)
	}
	if _, err := a.Availability(f.srv.URL, "edit"); err == nil {
		t.Fatal("an unknown job")
	}
	// Signed out: on this computer only, and nothing asked of the registry.
	signIn(t, "https://elsewhere.example")
	if v, err := a.Availability(f.srv.URL, designer.JobDesign); err != nil || v.Offered || v.Model == "" {
		t.Fatalf("signed out: %+v %v", v, err)
	}
}

// On this computer the run takes only picked images and no document, hands
// the CLI's Report Designer the design to start from with its images, and
// answers the result without saving it; Save adds it to the library as a
// new design under a free name, never the default, its images with it.
func TestOnThisComputerTheResultIsSavedOnlyOnYes(t *testing.T) {
	f := newDesignerFake(t)
	home, _ := signIn(t, f.srv.URL)
	a, events := newDesignerAPI(t)
	house := builtin(t, "modern")
	house.Name = "House"
	houseID := f.lib.add(house, true)
	taken := builtin(t, "plain")
	taken.Name = "Northwind"
	f.lib.add(taken, false)

	imgs := pickedImages(t, a.app, "logo.png", "ref.png")
	stray := filepath.Join(t.TempDir(), "stray.png")
	_ = os.WriteFile(stray, testPNG(t, 4, 4), 0o600)
	strayDoc := filepath.Join(t.TempDir(), "stray.pdf")
	_ = os.WriteFile(strayDoc, []byte("%PDF-1.7 not picked"), 0o600)
	doc := filepath.Join(t.TempDir(), "Annual Report.pdf")
	_ = os.WriteFile(doc, []byte("%PDF-1.7 the brand"), 0o600)
	a.app.markPicked(doc)

	made := builtin(t, "boardroom")
	made.Name = "Northwind"
	logo, err := reportdesign.NormalizeLogo(testPNG(t, 80, 40))
	if err != nil {
		t.Fatal(err)
	}
	made.Logo = logo
	var seen build.Options
	a.build = func(_ context.Context, _ dockerx.Client, o build.Options) (*build.Result, error) {
		seen = o
		_, _ = io.WriteString(o.Stderr, "rendered the cover\n")
		return &build.Result{Design: made, Looked: true, Previews: 2, Dir: filepath.Join(o.OutDir, "design"), DocumentPages: 3, LogoFromDocument: false}, nil
	}
	req := DesignerRequest{Job: designer.JobDesign, Brief: "Reports for a harbour trust.", From: houseID, Logo: imgs[0], Images: imgs[1:], Document: doc}

	for name, bad := range map[string]DesignerRequest{
		"an image no picker returned":   {Job: designer.JobDesign, Brief: "b", Images: []string{stray}},
		"a logo no picker returned":     {Job: designer.JobDesign, Brief: "b", Logo: stray},
		"an image as the document":      {Job: designer.JobDesign, Brief: "b", Document: imgs[1]},
		"a document no picker returned": {Job: designer.JobDesign, Brief: "b", Document: strayDoc},
		"a cover with a document":       {Job: designer.JobCover, Brief: "b", From: houseID, Document: doc},
		"no brief":                      {Job: designer.JobDesign},
		"a cover for no design":         {Job: designer.JobCover, Brief: "b"},
	} {
		if _, err := a.RunHere(f.srv.URL, bad); err == nil {
			t.Errorf("%s ran", name)
		}
	}
	if seen.Job != "" {
		t.Fatalf("a refused request reached the run: %+v", seen)
	}

	res, err := a.RunHere(f.srv.URL, req)
	if err != nil {
		t.Fatal(err)
	}
	if seen.Document == nil || seen.Document.Name != "Annual Report.pdf" || string(seen.Document.Data) != "%PDF-1.7 the brand" || res.DocumentPages != 3 {
		t.Fatalf("the document: %+v, result %+v", seen.Document, res)
	}
	if seen.From == nil || seen.From.Name != "House" || len(seen.Logo) == 0 || len(seen.Images) != 1 || !strings.HasPrefix(seen.OutDir, filepath.Join(home, "Downloads", "agentry-out")) {
		t.Fatalf("handed %+v", seen)
	}
	if res.Token == "" || res.Name != "Northwind" || res.View == nil || res.View.ID != "" || res.View.Default || res.View.Logo == "" || !res.Looked || res.Previews != 2 {
		t.Fatalf("result %+v", res)
	}
	events.mu.Lock()
	logged := strings.Join(events.log, "\n")
	events.mu.Unlock()
	if !strings.Contains(logged, "rendered the cover") || !strings.Contains(logged, "costs no credits") {
		t.Fatalf("log: %s", logged)
	}
	lib, _ := reportDesignLibrary(context.Background(), f.lib.srv.URL, "agy_test")
	if len(lib.Designs) != 2 {
		t.Fatalf("a run saved before the yes: %+v", lib)
	}

	if _, err := a.SaveResult(f.srv.URL, "not-the-token"); err == nil {
		t.Fatal("a token that names nothing saved")
	}
	saved, err := a.SaveResult(f.srv.URL, res.Token)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Default || saved.Design.Name != "Northwind (2)" || saved.Logo == "" {
		t.Fatalf("saved %+v", saved)
	}
	if !strings.Contains(f.lib.lastPost, `"default":false`) {
		t.Fatalf("sent %s", f.lib.lastPost)
	}
	lib, _ = reportDesignLibrary(context.Background(), f.lib.srv.URL, "agy_test")
	if len(lib.Designs) != 3 || lib.Default != houseID {
		t.Fatalf("after the save: %+v", lib)
	}
	if _, err := a.SaveResult(f.srv.URL, res.Token); err == nil {
		t.Fatal("one result saved twice")
	}
}

// In Agentry the run uploads the logo and the images, names the logo by
// its input id, sends the design it starts from by id and the addresses,
// and is followed to its end: the design it saved, by id and name, never
// the default; or, when it saved nothing, why, with the refund.
func TestInAgentryTheRunIsFollowedToTheDesignItSaved(t *testing.T) {
	f := newDesignerFake(t)
	signIn(t, f.srv.URL)
	fastPoll(t)
	a, events := newDesignerAPI(t)
	houseID := f.lib.add(builtin(t, "modern"), true)
	imgs := pickedImages(t, a.app, "logo.png", "ref-a.png", "ref-b.png")

	if _, err := a.StartHosted(f.srv.URL, DesignerRequest{Job: designer.JobDesign, Brief: "b", From: "../x"}); err == nil {
		t.Fatal("a malformed design id")
	}
	if _, err := a.StartHosted(f.srv.URL, DesignerRequest{Job: designer.JobDesign, Brief: "b", URLs: []string{"https://a.example/", "https://b.example/", "https://c.example/"}}); err == nil {
		t.Fatal("three addresses")
	}
	pdf := filepath.Join(t.TempDir(), "brand.pdf")
	_ = os.WriteFile(pdf, []byte("%PDF-1.7 brand"), 0o600)
	a.app.markPicked(pdf)
	// A registry that does not offer designBuild.document is given none.
	if _, err := a.StartHosted(f.srv.URL, DesignerRequest{Job: designer.JobDesign, Brief: "b", Document: pdf}); err == nil || !strings.Contains(err.Error(), "run it on this computer") {
		t.Fatalf("a document In Agentry where the registry takes none: %v", err)
	}
	if len(f.creates) != 0 || len(f.inputs) != 0 {
		t.Fatalf("a refusal reached the registry: %v %v", f.creates, f.inputs)
	}

	start, err := a.StartHosted(f.srv.URL, DesignerRequest{Job: designer.JobDesign, Brief: "  Reports for a harbour trust. ", From: houseID,
		Logo: imgs[0], Images: imgs[1:], URLs: []string{"https://example.com/about"}})
	if err != nil || start.Run == nil || start.Refusal != "" {
		t.Fatalf("start: %+v %v", start, err)
	}
	f.mu.Lock()
	var cr hostedrun.CreateRequest
	_ = json.Unmarshal([]byte(f.creates[0]), &cr)
	inputs := append([]string(nil), f.inputs...)
	f.mu.Unlock()
	if cr.Kind != hostedrun.KindDesignBuild || cr.Op != "design" || cr.Brief != "Reports for a harbour trust." || cr.From != houseID ||
		len(cr.Inputs) != 3 || cr.Logo != cr.Inputs[0] || len(cr.URLs) != 1 || cr.Ref != "" || cr.Task != "" {
		t.Fatalf("created %s", f.creates[0])
	}
	if len(inputs) != 3 || !strings.HasPrefix(inputs[0], ":logo.png=") {
		t.Fatalf("uploaded %v", inputs)
	}
	if _, err := a.StartHosted(f.srv.URL, DesignerRequest{Job: designer.JobDesign, Brief: "b"}); err == nil {
		t.Fatal("a second run while one is followed")
	}

	saved := f.lib.add(builtin(t, "boardroom"), false)
	f.finish(true, saved, "Northwind Harbour Trust reports", "")
	end := events.final(t)
	if !end.Applied || end.DesignID != saved || end.Name != "Northwind Harbour Trust reports" || !strings.Contains(end.OutcomeLine, "not your default") || end.Settled && end.SettledCredits != 20 {
		t.Fatalf("final %+v", end)
	}
	// The card is the library's own read of that id.
	if v, err := openReportDesign(context.Background(), f.lib.srv.URL, "agy_test", end.DesignID); err != nil || v.Default {
		t.Fatalf("the saved design: %+v %v", v, err)
	}

	// A Cover that saved nothing says why, with the refund.
	events.mu.Lock()
	events.hosted = nil
	events.mu.Unlock()
	if _, err := a.StartHosted(f.srv.URL, DesignerRequest{Job: designer.JobCover, Brief: "A dawn band.", From: houseID, Images: imgs[1:2]}); err != nil {
		t.Fatal(err)
	}
	f.finish(false, "", "", "gone")
	end = events.final(t)
	if end.Applied || !strings.Contains(end.Note, "deleted before it started") || !strings.Contains(end.Note, "refunded") {
		t.Fatalf("nothing saved: %+v", end)
	}
}

// Every Design with AI binding holds the address to Settings' rule before a
// token is looked up: an http address of a registry signed in over https
// sends nothing.
func TestDesignWithAISendsNoTokenToAURLSettingsRefuses(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits++; w.WriteHeader(http.StatusTeapot) }))
	t.Cleanup(srv.Close)
	refused := strings.Replace(srv.URL, "127.0.0.1", "0.0.0.0", 1)
	signIn(t, refused)
	a, _ := newDesignerAPI(t)
	imgs := pickedImages(t, a.app, "ref.png")
	if _, err := a.StartHosted(refused, DesignerRequest{Job: designer.JobDesign, Brief: "b", Images: imgs}); err == nil {
		t.Fatal("started at a refused address")
	}
	if _, err := a.RunHere(refused, DesignerRequest{Job: designer.JobDesign, Brief: "b"}); err == nil {
		t.Fatal("ran for a refused address")
	}
	if v, err := a.Availability(refused, designer.JobDesign); err != nil || v.Offered {
		t.Fatalf("offered at a refused address: %+v %v", v, err)
	}
	if _, err := a.SaveResult(refused, "t"); err == nil {
		t.Fatal("saved to a refused address")
	}
	if r, err := a.AttachHosted(refused); err != nil || r.Run != nil {
		t.Fatalf("followed at a refused address: %+v %v", r, err)
	}
	if hits != 0 {
		t.Fatalf("%d requests reached a refused address", hits)
	}
}

// A taken name gets the first free number, cut to the core's bound.
func TestASavedResultTakesAFreeName(t *testing.T) {
	lib := &ReportDesignLibrary{Designs: []ReportDesignCard{{Name: "Northwind"}, {Name: "northwind (2)"}}}
	if n := freeDesignName(lib, "Harbour"); n != "Harbour" {
		t.Fatal(n)
	}
	if n := freeDesignName(lib, "Northwind"); n != "Northwind (3)" {
		t.Fatal(n)
	}
	long := strings.Repeat("x", reportdesign.MaxNameRunes)
	lib.Designs = append(lib.Designs, ReportDesignCard{Name: long})
	if n := freeDesignName(lib, long); len([]rune(n)) > reportdesign.MaxNameRunes || !strings.HasSuffix(n, " (2)") {
		t.Fatal(n)
	}
}

// The pickers mark only what they return: an image as it was picked, and a
// document once its bytes say it is a PDF or a Word document.
func TestThePickersMarkOnlyWhatTheyReturn(t *testing.T) {
	a, _ := newDesignerAPI(t)
	dir := t.TempDir()
	img, pdf, txt := filepath.Join(dir, "logo.png"), filepath.Join(dir, "brand.pdf"), filepath.Join(dir, "notes.pdf")
	_ = os.WriteFile(img, testPNG(t, 4, 4), 0o600)
	_ = os.WriteFile(pdf, []byte("%PDF-1.7 brand"), 0o600)
	_ = os.WriteFile(txt, []byte("not a pdf"), 0o600)
	oldImg, oldDoc := pickDesignerImage, pickDesignerDocument
	t.Cleanup(func() { pickDesignerImage, pickDesignerDocument = oldImg, oldDoc })
	answer := ""
	pickDesignerImage = func(context.Context, string) (string, error) { return answer, nil }
	pickDesignerDocument = func(context.Context, string) (string, error) { return answer, nil }

	if p, err := a.ChooseImage("Choose your logo"); err != nil || p != "" {
		t.Fatalf("cancelled: %q %v", p, err)
	}
	answer = img
	if p, err := a.ChooseImage("Choose your logo"); err != nil || p != img || a.app.requirePicked("the logo", img) != nil {
		t.Fatalf("picked: %q %v", p, err)
	}
	answer = txt
	if p, err := a.ChooseDocument("Choose a document"); err == nil || p != "" || a.app.requirePicked("the document", txt) == nil {
		t.Fatalf("a text file named .pdf: %q %v", p, err)
	}
	answer = pdf
	if p, err := a.ChooseDocument("Choose a document"); err != nil || p != pdf || a.app.requirePicked("the document", pdf) != nil {
		t.Fatalf("a PDF: %q %v", p, err)
	}
}

// Where the registry takes a document, a Design In Agentry uploads it with
// kind=document and names its input in the create's document, among the
// inputs; the availability says so; a Cover still takes none, and a file
// that is not a PDF or a Word document never reaches the registry.
func TestInAgentryADesignIsBuiltAroundAnUploadedDocument(t *testing.T) {
	f := newDesignerFake(t)
	f.document = true
	signIn(t, f.srv.URL)
	fastPoll(t)
	a, events := newDesignerAPI(t)
	if v, err := a.Availability(f.srv.URL, designer.JobDesign); err != nil || !v.DocumentInAgentry || v.MaxDocumentBytes != 20<<20 {
		t.Fatalf("availability: %+v %v", v, err)
	}
	dir := t.TempDir()
	pdf, txt := filepath.Join(dir, "Annual Report.pdf"), filepath.Join(dir, "notes.pdf")
	_ = os.WriteFile(pdf, []byte("%PDF-1.7 the brand"), 0o600)
	_ = os.WriteFile(txt, []byte("plain text"), 0o600)
	a.app.markPicked(pdf)
	a.app.markPicked(txt)
	houseID := f.lib.add(builtin(t, "modern"), true)
	if _, err := a.StartHosted(f.srv.URL, DesignerRequest{Job: designer.JobDesign, Brief: "b", Document: txt}); err == nil || !strings.Contains(err.Error(), "a PDF or a Word document") {
		t.Fatalf("a text file named .pdf: %v", err)
	}
	if _, err := a.StartHosted(f.srv.URL, DesignerRequest{Job: designer.JobCover, Brief: "b", From: houseID, Document: pdf}); err == nil {
		t.Fatal("a cover with a document")
	}
	f.mu.Lock()
	n := len(f.inputs) + len(f.creates)
	f.mu.Unlock()
	if n != 0 {
		t.Fatalf("a refusal reached the registry: %v %v", f.inputs, f.creates)
	}
	imgs := pickedImages(t, a.app, "logo.png")
	start, err := a.StartHosted(f.srv.URL, DesignerRequest{Job: designer.JobDesign, Brief: "Like our annual report.", Logo: imgs[0], Document: pdf})
	if err != nil || start.Run == nil {
		t.Fatalf("start: %+v %v", start, err)
	}
	f.mu.Lock()
	var cr hostedrun.CreateRequest
	_ = json.Unmarshal([]byte(f.creates[0]), &cr)
	inputs := append([]string(nil), f.inputs...)
	f.mu.Unlock()
	if len(inputs) != 2 || inputs[0] != "document:Annual Report.pdf=%PDF-1.7 the brand" || !strings.HasPrefix(inputs[1], ":logo.png=") {
		t.Fatalf("uploaded %q", inputs)
	}
	if cr.Document == "" || cr.Logo == "" || cr.Document == cr.Logo || len(cr.Inputs) != 2 || cr.Inputs[0] != cr.Logo || cr.Inputs[1] != cr.Document {
		t.Fatalf("created %s", f.creates[0])
	}
	f.mu.Lock()
	f.run.DesignBuild.Document = "Annual Report.pdf"
	f.mu.Unlock()
	saved := f.lib.add(builtin(t, "boardroom"), false)
	f.finish(true, saved, "Larkspur Co-operative", "")
	if end := events.final(t); !end.Applied || end.Document != "Annual Report.pdf" {
		t.Fatalf("final %+v", end)
	}
}
