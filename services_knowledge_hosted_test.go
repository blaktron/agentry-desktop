package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
)

// Create knowledge In Agentry (agentry-desktop#274) against a fake
// registry that is both halves the build touches: the core's knowledge
// routes (the CLI's knowledgetest fake, which wraps, unwraps, keeps and
// serves sealed packs as the core does) and the runs extension's account
// routes with the build kind (agentry-enterprise schemas/x-runs.yaml). The
// fake's "sandbox host" is the test: finishing a build uploads the pack a
// Creator would have written, through a second store as the same account,
// so the app's download is the real unwrap flow into the real local store.

// buildFake is the registry.
type buildFake struct {
	mu        sync.Mutex
	reg       *knowledgetest.Registry
	url       string
	token     string
	scopes    []string
	enabled   bool
	balance   int64
	run       *hostedrun.Run
	created   []hostedrun.CreateRequest
	uploads   []string
	refuse    func(w http.ResponseWriter)
	helperEnv knowledge.Env
	// scanning counts the app's reads of the pack list while a pack is
	// being scanned, which is the wait before a download.
	scanning int
	// hold keeps a create waiting until it is closed, to race a second.
	hold chan struct{}
}

const buildID = "33333333-3333-3333-3333-333333333333"

func newBuildFake(t *testing.T) *buildFake {
	t.Helper()
	f := &buildFake{reg: knowledgetest.NewRegistry(t), enabled: true, balance: 496,
		scopes: []string{"agent:read", "user:profile", "run:read", "run:write", "knowledge:read", "knowledge:write"}}
	f.token = f.reg.Token("ana")
	core := f.reg.Config.Handler
	mux := http.NewServeMux()
	mux.Handle("/api/v1/knowledge/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/knowledge/packs" && r.Header.Get("Authorization") == "Bearer "+f.token {
			rec := httptest.NewRecorder()
			core.ServeHTTP(rec, r)
			if strings.Contains(rec.Body.String(), `"state":"scanning"`) {
				f.mu.Lock()
				f.scanning++
				f.mu.Unlock()
			}
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		core.ServeHTTP(w, r)
	}))
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"version": "1", "apiVersion": "v1", "features": map[string]bool{},
			"extensions": []string{"billing", "runs", "knowledge"}})
	})
	mux.HandleFunc("GET /api/v1/user/tokens", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, []map[string]any{{"id": "00000000-0000-0000-0000-0000000000b1", "name": "Agentry Desktop",
			"prefix": f.token[:12], "scopes": f.scopes, "expiresAt": "2027-01-01T00:00:00Z", "createdAt": "2026-01-01T00:00:00Z"}})
	})
	mux.HandleFunc("GET /api/v1/x/billing/account", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, map[string]any{"tier": "pro", "balance": f.balance, "allowance": 1000, "submissionCost": 1,
			"periodEnd": "2026-10-11T00:00:00Z", "privateAgents": map[string]any{"max": 0, "used": 1},
			"upgrade": map[string]any{"available": false, "manage": false}})
	})
	mux.HandleFunc("GET /api/v1/x/runs/status", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, map[string]any{
			"enabled": true, "workersOnline": 0, "hostsOnline": 1, "queued": 0,
			"inputTokensPerCredit": 50000, "outputTokensPerCredit": 25000,
			"knowledgeBuild": map[string]any{"enabled": f.enabled, "holdCredits": 100, "scanCredits": 1},
			"caller": map[string]any{
				"tier": "pro", "caps": map[string]any{"timeoutSeconds": 900, "maxTurns": 60, "maxTokens": 5000000},
				"holdCredits": 200, "inputTokensPerCredit": 50000, "outputTokensPerCredit": 25000,
				"build": map[string]any{"caps": map[string]any{"timeoutSeconds": 3600, "maxTurns": 300, "maxTokens": 5000000}, "holdCredits": 100},
			},
		})
	})
	mux.HandleFunc("POST /api/v1/x/runs/inputs", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		name := r.URL.Query().Get("name")
		f.uploads = append(f.uploads, name)
		hostedJSON(w, http.StatusCreated, map[string]any{"id": "44444444-4444-4444-4444-00000000000" + string(rune('0'+len(f.uploads))), "name": name, "bytes": r.ContentLength})
	})
	mux.HandleFunc("POST /api/v1/x/runs/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		hold := f.hold
		f.mu.Unlock()
		if hold != nil {
			<-hold
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.refuse != nil {
			f.refuse(w)
			return
		}
		var req hostedrun.CreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		f.created = append(f.created, req)
		name := req.Name
		if req.Continue != "" && f.run != nil && f.run.Build != nil {
			name = f.run.Build.Name
		}
		if req.Pack != "" {
			name = "handbook"
		}
		inputs := []hostedrun.Artefact{}
		for _, n := range f.uploads {
			inputs = append(inputs, hostedrun.Artefact{Name: n, Bytes: 10})
		}
		f.run = &hostedrun.Run{
			ID: buildID, Kind: hostedrun.KindKnowledgeBuild, Ref: "agentry/knowledge-creator@1.0.0", Digest: "builtin",
			Mode: "autonomous", Status: hostedrun.StatusQueued, QueuePosition: 1, HoldCredits: 100, Inputs: inputs,
			Caps: hostedrun.Caps{TimeoutSeconds: 3600, MaxTurns: 300, MaxTokens: 5000000}, CreatedAt: time.Now().UTC(),
			Build: &hostedrun.KnowledgeBuild{Pack: "55555555-5555-5555-5555-555555555555", Name: name},
		}
		hostedJSON(w, http.StatusCreated, f.run)
	})
	mux.HandleFunc("GET /api/v1/x/runs/", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		items := []*hostedrun.Run{}
		if f.run != nil {
			items = append(items, f.run)
		}
		hostedJSON(w, http.StatusOK, map[string]any{"items": items})
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
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	helperHome := t.TempDir()
	f.helperEnv = knowledge.Env{
		Home:     func() (string, error) { return helperHome, nil },
		Keys:     &knowledgeKeys{m: map[string]string{}},
		TokenFor: func(u string) string { return map[bool]string{true: f.token}[u == f.url] },
		HTTP:     srv.Client(),
	}
	return f
}

// apply is the sandbox host and the registry's collect: the Creator's pack
// named name lands in Agentry for the account, as a Replace would put it.
func (f *buildFake) apply(t *testing.T, name string) {
	t.Helper()
	s, err := f.helperEnv.Store(f.url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Keep(context.Background(), knowledgetest.Make(t, name, nil).Dir); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Upload(context.Background(), name); err != nil {
		t.Fatal(err)
	}
}

// finish ends the build: with applied, the pack took the tree and the
// settlement carries the scan's credit.
func (f *buildFake) finish(outcome string, applied bool, note string, settled int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.run.Status, f.run.Outcome, f.run.QueuePosition = hostedrun.StatusFinished, outcome, 0
	f.run.SettledCredits = &settled
	f.run.Usage = &hostedrun.Usage{InputTokens: 23056, OutputTokens: 2501, Turns: 10}
	f.run.Build.Applied, f.run.Build.Note = applied, note
	if applied {
		f.run.Build.Pages, f.run.Build.Bytes, f.run.Build.ScanCredits = 2, 1912, 1
	}
}

func (f *buildFake) set(status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.run.Status = status
}

// hostedBuildAPI is a KnowledgeAPI over its own runner home, signed in to
// the fake, with its knowledge:hosted-build events collected.
func hostedBuildAPI(t *testing.T, f *buildFake) (*KnowledgeAPI, *buildEvents) {
	t.Helper()
	home := t.TempDir()
	t.Setenv(policy.HomeEnv, home)
	ev := &buildEvents{}
	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if name != eventKnowledgeHosted || len(data) != 1 {
			return
		}
		if v, ok := data[0].(*KnowledgeHostedBuildView); ok {
			ev.mu.Lock()
			ev.views = append(ev.views, v)
			ev.mu.Unlock()
		}
	}
	t.Cleanup(func() { emitEvent = old })
	oldPoll, oldWait := hostedPoll, hostedScanWait
	hostedPoll, hostedScanWait = 10*time.Millisecond, 5*time.Second
	t.Cleanup(func() { hostedPoll, hostedScanWait = oldPoll, oldWait })
	env := knowledge.Env{
		Home:     func() (string, error) { return home, nil },
		Keys:     &knowledgeKeys{m: map[string]string{}},
		TokenFor: func(u string) string { return map[bool]string{true: f.token}[u == f.url] },
		HTTP:     http.DefaultClient,
	}
	app := &App{ctx: context.Background()}
	r := &RunAPI{app: app, knowledgeEnv: &env, inFlightDir: t.TempDir()}
	k := &KnowledgeAPI{app: app, run: r}
	// Registered last, so it runs first: a follow still going when the test
	// ends is stopped and waited for before the globals it reads are put
	// back.
	t.Cleanup(func() {
		if cur := k.hostedCurrent(); cur != nil && cur.done != nil {
			cur.cancel()
			<-cur.done
		}
	})
	return k, ev
}

type buildEvents struct {
	mu    sync.Mutex
	views []*KnowledgeHostedBuildView
}

// final waits for the build's last word.
func (e *buildEvents) final(t *testing.T) *KnowledgeHostedBuildView {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		for _, v := range e.views {
			if v.Final {
				e.mu.Unlock()
				return v
			}
		}
		e.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the build never reported its final word")
	return nil
}

func docs(t *testing.T, k *KnowledgeAPI) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{"backups.md": "# Backups\nNightly.\n", "oncall.md": "# On-call\nWeekly.\n", ".hidden": "x"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	k.app.markPicked(dir)
	return dir
}

// The availability rule, as the panel reads it: offered and available
// with the caller's own hold, the scan's credit inside it, the build's caps
// and the balance; refused in words when the token lacks the knowledge
// scopes or a run is in flight.
func TestHostedBuildAvailabilityStatesTheHoldAndRefusesInWords(t *testing.T) {
	f := newBuildFake(t)
	k, _ := hostedBuildAPI(t, f)
	v, err := k.HostedAvailability(f.url)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Offered || !v.Available || v.HoldCredits != 100 || v.ScanCredits != 1 || v.TimeoutSeconds != 3600 || v.MaxTurns != 300 ||
		!v.BalanceKnown || v.Balance != 496 {
		t.Fatalf("availability = %+v", v)
	}
	f.mu.Lock()
	f.scopes = []string{"run:read", "run:write"}
	f.mu.Unlock()
	if v, _ = k.HostedAvailability(f.url); v.Available || !strings.Contains(v.Reason, "knowledge:read and knowledge:write") || v.Remedy == "" {
		t.Fatalf("without the knowledge scopes: %+v", v)
	}
	f.mu.Lock()
	f.scopes = append(f.scopes, "knowledge:read", "knowledge:write")
	f.run = &hostedrun.Run{ID: "x", Status: hostedrun.StatusRunning}
	f.mu.Unlock()
	if v, _ = k.HostedAvailability(f.url); v.Available || !strings.Contains(v.Reason, "already has a run In Agentry in flight") {
		t.Fatalf("with a run in flight: %+v", v)
	}
	// Signed out: the choice is absent, and Create knowledge is local.
	k.run.knowledgeEnv.TokenFor = func(string) string { return "" }
	if v, _ = k.HostedAvailability(f.url); v.Offered {
		t.Fatalf("signed out: %+v", v)
	}
}

// Create: the folder's files go up through the run's input upload (its
// hidden file aside), the build is created with the pack's name and no
// ref, it is followed until done, Agentry's scan is waited for, and the
// sealed pack is downloaded into the local store, where the panel lists it
// with its scan.
func TestHostedBuildCreatesPollsWaitsForTheScanAndDownloads(t *testing.T) {
	f := newBuildFake(t)
	f.reg.Scanning = true
	k, ev := hostedBuildAPI(t, f)
	dir := docs(t, k)

	start, err := k.StartHostedBuild(f.url, "runbooks", "", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if start.Build == nil || start.Build.Name != "runbooks" || start.Build.State != "queued" {
		t.Fatalf("start = %+v", start)
	}
	f.mu.Lock()
	created, uploads := f.created, f.uploads
	f.mu.Unlock()
	if len(created) != 1 || created[0].Kind != hostedrun.KindKnowledgeBuild || created[0].Name != "runbooks" || created[0].Ref != "" ||
		created[0].Pack != "" || created[0].Continue != "" || len(created[0].Inputs) != 2 {
		t.Fatalf("created = %+v", created)
	}
	if strings.Join(uploads, ",") != "backups.md,oncall.md" {
		t.Fatalf("uploaded %v; a hidden file is not a document", uploads)
	}
	if _, err := k.StartHostedBuild(f.url, "other", "", []string{dir}); err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("a second build while one is followed: %v", err)
	}

	f.set(hostedrun.StatusRunning)
	time.Sleep(50 * time.Millisecond)
	f.apply(t, "runbooks")
	f.finish("succeeded", true, "", 2)
	// The pack is scanning in Agentry: the app waits, then downloads.
	time.Sleep(100 * time.Millisecond)
	f.reg.Finish()

	v := ev.final(t)
	f.mu.Lock()
	waited := f.scanning
	f.mu.Unlock()
	if waited == 0 {
		t.Fatal("the download never met the pack being scanned: the wait was not exercised")
	}
	if v.Status != "finished" || !v.Applied || v.Pages != 2 || v.SettledCredits != 2 || v.ScanCredits != 1 || !v.Downloaded || v.DownloadError != "" {
		t.Fatalf("final = %+v", v)
	}
	if v.ScanState != knowledge.ScanScanned || v.CanContinue {
		t.Fatalf("the kept pack: scan %q %q, continue %v", v.ScanState, v.Scan, v.CanContinue)
	}
	list, err := k.List(f.url)
	if err != nil {
		t.Fatal(err)
	}
	var kept *knowledge.Pack
	for i := range list.Packs {
		if list.Packs[i].Name == "runbooks" && list.Packs[i].Local {
			kept = &list.Packs[i]
		}
	}
	if kept == nil || !kept.InAgentry || kept.ScanState != knowledge.ScanScanned {
		t.Fatalf("the panel lists %+v", list.Packs)
	}
	if k.hostedCurrent() != nil {
		t.Fatal("still following a build that is over")
	}
}

// Continue: a build that stopped at its cap offers it; continuing posts
// the build's id alone (the registry copies its documents, so nothing is
// uploaded) and lands the pack as a create does.
func TestHostedBuildContinuesOneThatStoppedShort(t *testing.T) {
	f := newBuildFake(t)
	k, ev := hostedBuildAPI(t, f)
	dir := docs(t, k)
	if _, err := k.StartHostedBuild(f.url, "runbooks", "", []string{dir}); err != nil {
		t.Fatal(err)
	}
	f.finish("budget", false, "nothing", 1)
	first := ev.final(t)
	if !first.CanContinue || first.Applied || first.Downloaded || first.Note == "" || first.SettledCredits != 1 {
		t.Fatalf("a build stopped at its budget: %+v", first)
	}

	ev.mu.Lock()
	ev.views = nil
	ev.mu.Unlock()
	f.mu.Lock()
	f.uploads = nil
	f.mu.Unlock()
	start, err := k.ContinueHostedBuild(f.url, buildID)
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	last, uploads := f.created[len(f.created)-1], f.uploads
	f.mu.Unlock()
	if start.Build == nil || last.Continue != buildID || last.Name != "" || last.Pack != "" || len(last.Inputs) != 0 || len(uploads) != 0 {
		t.Fatalf("continue sent %+v and uploaded %v", last, uploads)
	}
	f.apply(t, "runbooks")
	f.finish("succeeded", true, "", 2)
	if v := ev.final(t); !v.Downloaded || v.Name != "runbooks" {
		t.Fatalf("the continued build: %+v", v)
	}
	// A build that finished its work is not continued.
	if _, err := k.ContinueHostedBuild(f.url, buildID); err == nil || !strings.Contains(err.Error(), "did not stop short") {
		t.Fatalf("continue a finished build: %v", err)
	}
}

// What a build writes must come back here: a new pack's name that is kept
// already, a pack to add to that is not in Agentry, or one changed here
// since its upload, is refused before anything is uploaded; the registry's
// refusal is returned as its slug and sentence.
func TestHostedBuildRefusesWhatCouldNotComeBack(t *testing.T) {
	f := newBuildFake(t)
	k, _ := hostedBuildAPI(t, f)
	dir := docs(t, k)
	s, err := k.env().Store(f.url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Keep(context.Background(), knowledgetest.Make(t, "handbook", nil).Dir); err != nil {
		t.Fatal(err)
	}
	if _, err := k.StartHostedBuild(f.url, "handbook", "", []string{dir}); err == nil || !strings.Contains(err.Error(), "already kept") {
		t.Fatalf("a name kept here: %v", err)
	}
	if _, err := k.StartHostedBuild(f.url, "Bad Name", "", []string{dir}); err == nil || !strings.Contains(err.Error(), "lowercase") {
		t.Fatalf("a bad name: %v", err)
	}
	if _, err := k.StartHostedBuild(f.url, "", "handbook", []string{dir}); err == nil || !strings.Contains(err.Error(), "Upload to Agentry first") {
		t.Fatalf("add to a pack not in Agentry: %v", err)
	}
	if _, _, err := s.Upload(context.Background(), "handbook"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.StartHostedBuild(f.url, "", "handbook", []string{dir}); err != nil {
		t.Fatalf("add to a pack kept in Agentry as it is here: %v", err)
	}
	f.mu.Lock()
	last := f.created[len(f.created)-1]
	f.mu.Unlock()
	if last.Pack == "" || last.Name != "" || len(last.Inputs) != 2 {
		t.Fatalf("add sent %+v", last)
	}
	f.finish("failed", false, "", 1)
	k.hostedCurrent()
	deadline := time.Now().Add(5 * time.Second)
	for k.hostedCurrent() != nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	f.mu.Lock()
	f.refuse = func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "https://agentry.dev/problems/credits-exhausted", "title": "Not enough credits",
			"detail": "This account has 40 credits; a build holds 100.", "balance": 40, "hold": 100, "resetsAt": "2026-10-11T00:00:00Z", "tier": "standard"})
	}
	f.mu.Unlock()
	v, err := k.StartHostedBuild(f.url, "notes", "", []string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if v.Refusal != "credits-exhausted" || v.Balance != 40 || v.Hold != 100 || v.Build != nil {
		t.Fatalf("a 402: %+v", v)
	}
}

// Every document is one a native picker returned (D-030), and the run
// panel's reattach leaves a build in flight to the Knowledge panel.
func TestHostedBuildDocumentsComeFromThePickerAndARunDoesNotAttachToABuild(t *testing.T) {
	f := newBuildFake(t)
	k, _ := hostedBuildAPI(t, f)
	typed := filepath.Join(t.TempDir(), "typed.md")
	if err := os.WriteFile(typed, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := k.StartHostedBuild(f.url, "notes", "", []string{typed}); err == nil || !strings.Contains(err.Error(), "not chosen through the file dialog") {
		t.Fatalf("a typed path: %v", err)
	}
	f.mu.Lock()
	uploaded := len(f.uploads)
	f.mu.Unlock()
	if uploaded != 0 {
		t.Fatal("a typed path was uploaded")
	}

	f.mu.Lock()
	f.run = &hostedrun.Run{ID: buildID, Kind: hostedrun.KindKnowledgeBuild, Status: hostedrun.StatusRunning, Build: &hostedrun.KnowledgeBuild{Name: "notes"}}
	f.mu.Unlock()
	signIn(t, f.url)
	outDir := t.TempDir()
	k.app.markPicked(outDir)
	got, err := k.run.AttachHostedRun(f.url, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Run != nil {
		t.Fatalf("the run panel attached to a build: %+v", got.Run)
	}
}

// A build's status line is a run's with the word changed, and only the
// word: "running" stays "running" (the live check on dev once read
// "buildning").
func TestHostedBuildStatusLineChangesOnlyTheWord(t *testing.T) {
	for _, status := range []string{hostedrun.StatusQueued, hostedrun.StatusRunning, hostedrun.StatusFinished, hostedrun.StatusCancelled} {
		r := &hostedrun.Run{ID: buildID, Kind: hostedrun.KindKnowledgeBuild, Status: status, Outcome: map[bool]string{true: "succeeded"}[status == hostedrun.StatusFinished]}
		line := hostedBuildView(r, false).StatusLine
		if strings.Contains(line, "buildn") || strings.Contains(line, " run ") || strings.HasSuffix(line, " run") {
			t.Errorf("%s: %q", status, line)
		}
	}
}

// A second Start while the first is still uploading and creating is
// refused before it uploads anything: the follow slot is claimed first.
func TestHostedBuildASecondStartIsRefusedBeforeItUploads(t *testing.T) {
	f := newBuildFake(t)
	k, ev := hostedBuildAPI(t, f)
	dir := docs(t, k)
	f.mu.Lock()
	f.hold = make(chan struct{})
	f.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := k.StartHostedBuild(f.url, "runbooks", "", []string{dir})
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		n := len(f.uploads)
		f.mu.Unlock()
		if n == 2 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := k.StartHostedBuild(f.url, "other", "", []string{dir}); err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("a second start: %v", err)
	}
	f.mu.Lock()
	uploaded := len(f.uploads)
	close(f.hold)
	f.hold = nil
	f.mu.Unlock()
	if uploaded != 2 {
		t.Fatalf("the second start uploaded: %d uploads", uploaded)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	f.finish("failed", false, "", 1)
	ev.final(t)
}

// A link inside a picked folder that leads out of it is refused, though
// the path as written lies under the folder.
func TestHostedBuildRefusesALinkOutOfAPickedFolder(t *testing.T) {
	f := newBuildFake(t)
	k, _ := hostedBuildAPI(t, f)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "id_rsa"), []byte("not for upload"), 0o600); err != nil {
		t.Fatal(err)
	}
	picked := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(picked, "link")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	k.app.markPicked(picked)
	through := filepath.Join(picked, "link", "id_rsa")
	if _, err := k.StartHostedBuild(f.url, "notes", "", []string{through}); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("a file reached through a link: %v", err)
	}
	f.mu.Lock()
	uploaded := len(f.uploads)
	f.mu.Unlock()
	if uploaded != 0 {
		t.Fatal("a file outside the picked folder was uploaded")
	}
	// The picked folder itself is fine, and its walk does not follow the link.
	if err := os.WriteFile(filepath.Join(picked, "notes.md"), []byte("# Notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := k.StartHostedBuild(f.url, "notes", "", []string{picked}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	names := strings.Join(f.uploads, ",")
	f.mu.Unlock()
	if names != "notes.md" {
		t.Fatalf("uploaded %q", names)
	}
}

// TestARunInAgentryRefusesANameTwoCopiesHold (Knowledge M8): a person's
// own pack and an organization's shared one of the same name, both kept in
// Agentry, are two copies; a run In Agentry attaching the name is refused
// rather than given whichever sorts first. A pack kept in both places is
// one copy.
func TestARunInAgentryRefusesANameTwoCopiesHold(t *testing.T) {
	packs := []knowledge.Pack{
		{Name: "handbook", InAgentry: true, AgentryID: "a", Local: true},
		{Name: "handbook", InAgentry: true, AgentryID: "a"},
		{Name: "runbook", InAgentry: true, AgentryID: "r"},
	}
	if n := inAgentryNamed(packs, "handbook"); n != 1 {
		t.Fatalf("one copy kept in both places counts %d", n)
	}
	packs = append(packs, knowledge.Pack{Name: "handbook", InAgentry: true, AgentryID: "b", Shared: true, Org: "acme"})
	if n := inAgentryNamed(packs, "handbook"); n != 2 {
		t.Fatalf("a personal and a shared copy count %d", n)
	}
}
