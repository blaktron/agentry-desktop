package main

// Describe it In Agentry (agentry-desktop#281) through the bound surface,
// against a fake registry with the draft kind (agentry-enterprise
// schemas/x-runs.yaml): the offer and the hold in the site's words, the
// create's body, the follow, the sealed output unpacked into the run's own
// folder, draft.json decoded into the Model as data, and the refusals.

import (
	"archive/tar"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/drafter"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
)

const draftRunID = "66666666-6666-6666-6666-666666666666"

const draftFakeManifest = `{"schemaVersion": "2", "name": "meeting-actions", "namespace": "draft", "version": "0.1.0",
  "summary": "Turns meeting notes into decisions and action items.", "license": "Apache-2.0",
  "modes": ["autonomous"], "entrypoints": {"instruction": "AGENT.md"},
  "capabilities": {"network": {"egress": ["slack.com"]}, "filesystem": {"read": "workspace", "write": "none"}, "exec": "none"},
  "publish": {"include": ["agent.yaml", "AGENT.md", "README.md", "LICENSE"]}}`

// draftFake is the registry: the core routes the offer reads and the runs
// extension's account routes with the draft kind.
type draftFake struct {
	mu      sync.Mutex
	url     string
	scopes  []string
	enabled bool
	balance int64
	run     *hostedrun.Run
	created []map[string]any
	refuse  func(w http.ResponseWriter)
	// outputs are the files of the draft's output.tar.
	outputs map[string]string
	// agentsDown makes the person's QuickAgents unreadable.
	agentsDown bool
	// knowledge says the status route's drafts read packs
	// (agentry-enterprise#295); packs are the account's packs in Agentry.
	knowledge bool
	packs     []map[string]any
	// revise is the status route's caller.revise.allowed, absent when nil
	// (agentry-desktop#501); mux takes a test's extra routes.
	revise *bool
	mux    *http.ServeMux
}

func newDraftFake(t *testing.T) *draftFake {
	t.Helper()
	f := &draftFake{enabled: true, balance: 496, scopes: []string{"agent:read", "user:profile", "run:read", "run:write"},
		outputs: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		hostedJSON(w, http.StatusOK, map[string]any{"version": "1", "apiVersion": "v1", "features": map[string]bool{},
			"extensions": []string{"billing", "runs", "knowledge"}})
	})
	mux.HandleFunc("GET /api/v1/user", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+hostedTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		hostedJSON(w, http.StatusOK, map[string]any{"id": "00000000-0000-0000-0000-000000000002", "handle": "chris", "email": "c@example.test",
			"displayName": "Chris", "emailVerified": true, "totpEnabled": false, "createdAt": "2026-01-02T03:04:05Z"})
	})
	// The person's own QuickAgents: one already has the draft's name.
	mux.HandleFunc("GET /api/v1/agents", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		down := f.agentsDown
		f.mu.Unlock()
		if down || r.URL.Query().Get("namespace") != "chris" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		hostedJSON(w, http.StatusOK, []map[string]any{{"namespace": "chris", "name": "meeting-actions"}, {"namespace": "chris", "name": "other"}})
	})
	mux.HandleFunc("GET /api/v1/user/tokens", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		hostedJSON(w, http.StatusOK, []map[string]any{{"id": "00000000-0000-0000-0000-0000000000a2", "name": "Agentry Desktop",
			"prefix": hostedTestToken[:12], "scopes": f.scopes, "expiresAt": "2027-01-01T00:00:00Z", "createdAt": "2026-01-01T00:00:00Z"}})
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
		caps := map[string]any{"timeoutSeconds": 1200, "maxTurns": 60, "maxTokens": 1000000}
		draft := map[string]any{"enabled": f.enabled, "caps": caps, "holdCredits": 41, "tokenCredits": 30, "maxSearches": 10,
			"creditsPerSearch": 1, "maxPages": 40, "aiReviewCredits": 1}
		if f.knowledge {
			draft["knowledge"] = map[string]any{"enabled": true, "maxPacks": 5, "maxBytes": 10 << 20}
		}
		hostedJSON(w, http.StatusOK, map[string]any{
			"enabled": true, "workersOnline": 0, "hostsOnline": 1, "queued": 0,
			"inputTokensPerCredit": 50000, "outputTokensPerCredit": 25000,
			"knowledgeBuild": map[string]any{"enabled": true, "holdCredits": 100, "scanCredits": 1},
			"draft":          draft,
			"caller": map[string]any{
				"tier": "pro", "caps": map[string]any{"timeoutSeconds": 900, "maxTurns": 60, "maxTokens": 5000000},
				"holdCredits": 200, "inputTokensPerCredit": 50000, "outputTokensPerCredit": 25000,
				"build": map[string]any{"caps": caps, "holdCredits": 100, "allowed": true},
				"draft": map[string]any{"caps": caps, "holdCredits": 41},
				"revise": func() any {
					if f.revise == nil {
						return nil
					}
					return map[string]any{"allowed": *f.revise}
				}(),
			},
		})
	})
	mux.HandleFunc("GET /api/v1/knowledge/packs", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		packs := f.packs
		if packs == nil {
			packs = []map[string]any{}
		}
		hostedJSON(w, http.StatusOK, packs)
	})
	mux.HandleFunc("POST /api/v1/x/runs/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.refuse != nil {
			f.refuse(w)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.created = append(f.created, body)
		var hints []string
		if hs, ok := body["hints"].([]any); ok {
			for _, h := range hs {
				hints = append(hints, h.(string))
			}
		}
		desc, _ := body["description"].(string)
		f.run = &hostedrun.Run{
			ID: draftRunID, Kind: hostedrun.KindDraft, Ref: "agentry/drafter@1.0.0", Digest: "builtin", Mode: "autonomous",
			Task: desc, Status: hostedrun.StatusQueued, QueuePosition: 1, HoldCredits: 41,
			Caps: hostedrun.Caps{TimeoutSeconds: 1200, MaxTurns: 60}, CreatedAt: time.Now().UTC(),
			Draft: &hostedrun.Draft{Hints: hints, TokenCredits: 30, MaxSearches: 10, CreditsPerSearch: 1, MaxPages: 40},
		}
		if ref, _ := body["revises"].(string); ref != "" {
			f.run.Task = "Revise " + ref + "."
			f.run.Draft.Revises = &hostedrun.Revises{Ref: ref, Digest: editDigest}
			f.run.Draft.Instructions, _ = body["instructions"].(string)
			f.run.Draft.Problems, _ = body["problems"].(string)
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
	mux.HandleFunc("GET /api/v1/x/runs/{id}/outputs/{name...}", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		files := f.outputs
		f.mu.Unlock()
		if r.PathValue("name") != hostedrun.PushedOutput {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		names := make([]string, 0, len(files))
		for n := range files {
			names = append(names, n)
		}
		slices.Sort(names)
		tw := tar.NewWriter(w)
		for _, n := range names {
			_ = tw.WriteHeader(&tar.Header{Name: n, Mode: 0o600, Size: int64(len(files[n])), Typeflag: tar.TypeReg})
			_, _ = tw.Write([]byte(files[n]))
		}
		_ = tw.Close()
	})
	f.mux = mux
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.url = srv.URL
	return f
}

// finish ends the draft; with succeeded, its output holds draftJSON and the
// package the host rendered.
func (f *draftFake) finish(outcome, draftJSON string, settled int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	f.run.Status, f.run.Outcome, f.run.QueuePosition, f.run.FinishedAt = hostedrun.StatusFinished, outcome, 0, &now
	f.run.SettledCredits = &settled
	f.run.Usage = &hostedrun.Usage{Turns: 12}
	search, review := int64(2), int64(1)
	f.run.Draft.Searched, f.run.Draft.SearchCredits, f.run.Draft.AIReviewCredits = 2, &search, &review
	if outcome != hostedrun.OutcomeSucceeded {
		return
	}
	f.outputs = map[string]string{
		"draft.json":         draftJSON,
		"package/agent.yaml": "name: something-else\n",
		"package/AGENT.md":   "# Not the draft's\n",
		"receipt.json":       `{"exitClass":"ok"}`,
	}
	f.run.Outputs = []hostedrun.Artefact{{Name: hostedrun.PushedOutput, Bytes: 8 << 20}}
}

func (f *draftFake) set(status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.run.Status, f.run.QueuePosition = status, 0
}

func draftFakeJSON(t *testing.T, edit func(map[string]any)) string {
	t.Helper()
	d := map[string]any{
		"manifest": json.RawMessage(draftFakeManifest), "instructions": "# Meeting actions\n\nRead the notes.\n",
		"notes": "## Notes\n\nA first draft.\n", "matches": []any{}, "openQuestions": []any{"Which channel?"},
		"sources": []any{map[string]any{"url": "https://api.slack.com/methods", "used": "the methods"}},
		"drafter": map[string]any{"ref": "agentry/drafter@1.0.0", "model": "platform", "runId": draftRunID},
		"aiReview": map[string]any{"description": "pass", "scan": "registry", "read": 12, "passed": 11, "withheld": 1, "notScanned": 0,
			"withheldSources": []any{map[string]any{"tool": "web_read", "url": "https://evil.example/inject", "reason": "flagged (injection)"}},
			"check": map[string]any{
				"rubric": map[string]any{"verdict": "flagged", "findings": []any{map[string]any{"id": "C3_undeclared_capability", "score": 3, "path": "AGENT.md"},
					map[string]any{"id": "__proto__", "score": 2, "path": "README.md"}}, "rubricHash": "sha256:ab"},
				"draft": map[string]any{"verdict": "pass", "flagged": []any{}, "questionSet": "draft-1"},
				"model": "jev-1.13.0", "requests": 4, "inputTokens": 9000, "checks": 1}},
	}
	if edit != nil {
		edit(d)
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// draftEvents collects build:hosted-draft.
type draftEvents struct {
	mu    sync.Mutex
	views []*HostedDraftView
}

func (e *draftEvents) final(t *testing.T) *HostedDraftView {
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
	t.Fatal("the draft never reported its final word")
	return nil
}

func (e *draftEvents) statuses() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, v := range e.views {
		if len(out) == 0 || out[len(out)-1] != v.Status {
			out = append(out, v.Status)
		}
	}
	return out
}

// hostedDraftAPI is a BuildAPI signed in to the fake, with its
// build:hosted-draft events collected; it returns the temporary HOME.
func hostedDraftAPI(t *testing.T, f *draftFake) (*BuildAPI, *draftEvents, string) {
	t.Helper()
	home, _ := signIn(t, f.url)
	ev := &draftEvents{}
	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if name != eventDraftHosted || len(data) != 1 {
			return
		}
		if v, ok := data[0].(*HostedDraftView); ok {
			ev.mu.Lock()
			ev.views = append(ev.views, v)
			ev.mu.Unlock()
		}
	}
	t.Cleanup(func() { emitEvent = old })
	oldPoll := hostedPoll
	hostedPoll = 10 * time.Millisecond
	t.Cleanup(func() { hostedPoll = oldPoll })
	app := &App{ctx: context.Background()}
	b := &BuildAPI{app: app, run: &RunAPI{app: app}}
	t.Cleanup(func() {
		if cur := b.hostedCurrent(); cur != nil && cur.done != nil {
			cur.cancel()
			<-cur.done
		}
	})
	return b, ev, home
}

func TestHostedDraftAvailabilityStatesTheHoldInTheSitesWords(t *testing.T) {
	f := newDraftFake(t)
	b, _, _ := hostedDraftAPI(t, f)
	v, err := b.DescribeInAgentryStatus(f.url)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Offered || !v.Available || !v.SignedIn || v.HoldCredits != 41 || v.Balance != 496 || !v.BalanceKnown || v.Short {
		t.Fatalf("%+v", v)
	}
	if v.HoldSentence != "This draft holds 41 credits and settles from what it uses: its model's work, up to 10 web searches at 1 credit each, and 1 credit for AI Review, never more than it holds." {
		t.Fatalf("hold %q", v.HoldSentence)
	}
	if v.InputsSentence != hostedrun.DraftInputsSentence || v.TimeoutSeconds != 1200 || v.MaxTurns != 60 || v.MaxPages != 40 {
		t.Fatalf("%+v", v)
	}

	f.balance = 10
	if v, _ := b.DescribeInAgentryStatus(f.url); !v.Short || !v.Available {
		t.Fatalf("short of credits: %+v (the registry refuses, the page says so first)", v)
	}
	f.enabled = false
	if v, _ := b.DescribeInAgentryStatus(f.url); v.Available || !strings.Contains(v.Reason, "not taking drafts") {
		t.Fatalf("drafts off: %+v", v)
	}
	f.enabled, f.scopes = true, []string{"agent:read"}
	if v, _ := b.DescribeInAgentryStatus(f.url); v.Available || !strings.Contains(v.Reason, "run:write") {
		t.Fatalf("an old token: %+v", v)
	}

	// Signed out: offered, and it says a draft needs an account.
	if v, _ := b.DescribeInAgentryStatus("http://127.0.0.1:1"); v.SignedIn || !v.Offered || v.Available || !strings.Contains(v.Reason, "needs an account") {
		t.Fatalf("signed out: %+v", v)
	}
}

func TestDescribeInAgentryDraftsFollowsAndDecodesTheDraftAsData(t *testing.T) {
	f := newDraftFake(t)
	b, ev, home := hostedDraftAPI(t, f)
	start, err := b.DescribeInAgentry(f.url, "  Turn meeting notes into action items. ", []string{"The services or sites it works with: Slack", " ", "It runs on its own, from a task, with no conversation."}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if start.Draft == nil || start.Draft.ID != draftRunID || start.Draft.CreditsLine != "41 credits are held for this draft." {
		t.Fatalf("start %+v", start)
	}
	// The create is a draft's: its words, and nothing a run carries.
	body := f.created[0]
	if body["kind"] != "draft" || body["description"] != "Turn meeting notes into action items." {
		t.Fatalf("body %v", body)
	}
	if hints, _ := body["hints"].([]any); len(hints) != 2 {
		t.Fatalf("hints %v: the blank one is dropped", body["hints"])
	}
	for _, k := range []string{"ref", "task", "intake", "inputs", "knowledge", "name", "pack", "continue"} {
		if _, ok := body[k]; ok {
			t.Fatalf("the draft's create carries %s: %v", k, body)
		}
	}
	// A second is refused here, before any request.
	if _, err := b.DescribeInAgentry(f.url, "again", nil, nil); err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("second start: %v", err)
	}

	f.set(hostedrun.StatusRunning)
	time.Sleep(50 * time.Millisecond)
	f.finish(hostedrun.OutcomeSucceeded, draftFakeJSON(t, func(d map[string]any) { d["x-open-me"] = "package/agent.yaml" }), 28)
	final := ev.final(t)
	if final.Draft == nil || final.DraftError != "" {
		t.Fatalf("final %+v", final)
	}
	if !slices.Contains(ev.statuses(), hostedrun.StatusRunning) || final.OutcomeLine != "The Drafter finished the draft." || final.StatusLine != final.OutcomeLine ||
		final.CreditsLine != "41 credits were held; 28 settled (2 for 2 searches, 1 for AI Review) and the rest was released." {
		t.Fatalf("statuses %v, final %+v", ev.statuses(), final)
	}
	d := final.Draft
	if d.Model.Namespace != "chris" || d.Model.Instructions != "# Meeting actions\n\nRead the notes.\n" {
		t.Fatalf("model %+v: the draft is draft.json's, under the person's namespace, and never package/'s", d.Model)
	}
	// What the Builder sets (plan builder-simplify.md D10): a name none of
	// the person's QuickAgents has, and MIT, since the description names no
	// licence; each change named.
	if d.Model.Name != "meeting-actions-2" || d.Model.License != "MIT" || d.Model.Version != "0.1.0" {
		t.Fatalf("model %+v", d.Model)
	}
	notes := strings.Join(d.DecodeNotes, "\n")
	for _, want := range []string{`You already have a QuickAgent named "meeting-actions", so this draft is named "meeting-actions-2".`,
		`The draft's licence "Apache-2.0" is MIT here, because your description does not name it.`} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes %q do not say %q", d.DecodeNotes, want)
		}
	}
	// AI Review's check: one line for the summary, and its details, each
	// rubric question named for a person (an id that is not one names
	// nothing).
	c := d.Check
	if c == nil || c.Line == nil || c.Line.Tone != "review" || !strings.Contains(c.Line.Text, "flagged something the release check looks for") ||
		c.Rubric != "flagged" || c.Draft != "pass" || len(c.Findings) != 2 {
		t.Fatalf("check %+v", c)
	}
	if f := c.Findings[0]; f.ID != "C3_undeclared_capability" || f.Question != "reaching for something the contract does not declare" || f.Path != "AGENT.md" || f.Score != 3 {
		t.Errorf("finding %+v", f)
	}
	if f := c.Findings[1]; f.Question != "" {
		t.Errorf("an id that is not the rubric's is named: %+v", f)
	}
	if d.Where != "agentry" || d.Description != "Turn meeting notes into action items." || len(d.Hints) != 2 || d.AIReview.Scan != "registry" {
		t.Fatalf("view %+v", d)
	}
	if !strings.Contains(strings.Join(d.DecodeNotes, " "), "x-open-me") {
		t.Fatalf("notes %q: the unknown field is named", d.DecodeNotes)
	}
	// The withheld address is marked from AI Review's record, never a link.
	if i := slices.IndexFunc(d.Sources, func(s drafter.Source) bool { return s.URL == "https://evil.example/inject" }); i < 0 || !d.Sources[i].Withheld {
		t.Fatalf("sources %+v", d.Sources)
	}
	// The output landed in the run's own folder, and nothing else was
	// written in HOME.
	want := filepath.Join(home, "Downloads", "agentry-out", "agentry-drafter-"+draftRunID)
	if d.OutDir != want {
		t.Fatalf("out %q, want %q", d.OutDir, want)
	}
	var written []string
	_ = filepath.WalkDir(home, func(p string, e fs.DirEntry, _ error) error {
		if !e.IsDir() {
			rel, _ := filepath.Rel(home, p)
			written = append(written, rel)
		}
		return nil
	})
	for _, p := range written {
		if !strings.HasPrefix(p, filepath.Join("Downloads", "agentry-out", "agentry-drafter-"+draftRunID)+string(filepath.Separator)) {
			t.Errorf("wrote %s outside the run's folder", p)
		}
	}
	if len(written) != 4 {
		t.Fatalf("wrote %v", written)
	}
}

// When the person's QuickAgents cannot be read, the draft keeps its name
// and the notes say to check it.
func TestADraftWhoseNamesCannotBeReadSaysSo(t *testing.T) {
	f := newDraftFake(t)
	f.agentsDown = true
	b, ev, _ := hostedDraftAPI(t, f)
	if _, err := b.DescribeInAgentry(f.url, "Turn meeting notes into action items under the Apache 2.0 licence.", nil, nil); err != nil {
		t.Fatal(err)
	}
	f.finish(hostedrun.OutcomeSucceeded, draftFakeJSON(t, nil), 28)
	d := ev.final(t).Draft
	if d == nil || d.Model.Name != "meeting-actions" || d.Model.License != "Apache-2.0" {
		t.Fatalf("draft %+v", d)
	}
	if !strings.Contains(strings.Join(d.DecodeNotes, " "), "could not be read from the registry, so check that no QuickAgent of yours is already named meeting-actions.") {
		t.Errorf("notes %q", d.DecodeNotes)
	}
}

func TestAHostileHostedDraftOpensNothing(t *testing.T) {
	f := newDraftFake(t)
	b, ev, _ := hostedDraftAPI(t, f)
	if _, err := b.DescribeInAgentry(f.url, "Summarise notes.", nil, nil); err != nil {
		t.Fatal(err)
	}
	f.finish(hostedrun.OutcomeSucceeded, draftFakeJSON(t, func(d map[string]any) {
		d["notes"] = strings.Repeat("n", 1<<20+1)
	}), 20)
	final := ev.final(t)
	if final.Draft != nil || !strings.Contains(final.DraftError, "at most") {
		t.Fatalf("final %+v: an oversize draft is refused whole", final)
	}
}

func TestAHostedDraftThatDidNotFinishDownloadsNothing(t *testing.T) {
	f := newDraftFake(t)
	b, ev, home := hostedDraftAPI(t, f)
	if _, err := b.DescribeInAgentry(f.url, "Summarise notes.", nil, nil); err != nil {
		t.Fatal(err)
	}
	f.finish(hostedrun.OutcomeRefused, "", 0)
	final := ev.final(t)
	if final.Draft != nil || final.DraftError != "" || !strings.Contains(final.OutcomeLine, "No draft came of it") ||
		!strings.Contains(final.CreditsLine, "cost nothing") {
		t.Fatalf("final %+v", final)
	}
	if _, err := os.Stat(filepath.Join(home, "Downloads")); !os.IsNotExist(err) {
		t.Fatalf("a draft that did not finish made a folder: %v", err)
	}
}

func TestDescribeInAgentryRefusesInWordsAndBeforeAnyRequest(t *testing.T) {
	f := newDraftFake(t)
	b, _, _ := hostedDraftAPI(t, f)
	for name, c := range map[string]struct {
		d string
		h []string
	}{
		"empty":       {" ", nil},
		"too long":    {strings.Repeat("a", hostedrun.MaxDraftDescriptionBytes+1), nil},
		"a long hint": {"a", []string{strings.Repeat("h", hostedrun.MaxDraftHintBytes+1)}},
	} {
		if _, err := b.DescribeInAgentry(f.url, c.d, c.h, nil); err == nil {
			t.Errorf("%s: not refused", name)
		}
	}
	if len(f.created) != 0 {
		t.Fatalf("a refused draft reached the registry: %v", f.created)
	}
	f.refuse = func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(`{"type":"https://secureagentry.net/problems/credits-exhausted","title":"Not enough credits","detail":"This draft holds 41 credits and the account has 10.","balance":10,"hold":41,"resetsAt":"2026-10-11T00:00:00Z"}`))
	}
	v, err := b.DescribeInAgentry(f.url, "Summarise notes.", nil, nil)
	if err != nil || v.Refusal != "credits-exhausted" || v.Balance != 10 || v.Hold != 41 || !strings.Contains(v.Detail, "41 credits") {
		t.Fatalf("%+v %v", v, err)
	}
	// The refused start gave the slot back.
	f.refuse = nil
	if v, err := b.DescribeInAgentry(f.url, "Summarise notes.", nil, nil); err != nil || v.Draft == nil {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestAHostedDraftReattachesToTheBuildTabAndNeverToTheRunPanel(t *testing.T) {
	f := newDraftFake(t)
	b, _, _ := hostedDraftAPI(t, f)
	if _, err := b.DescribeInAgentry(f.url, "Summarise notes.", nil, nil); err != nil {
		t.Fatal(err)
	}
	// The run panel, reopened, has no run to follow: the draft is the
	// Build tab's.
	r := &RunAPI{app: &App{ctx: context.Background()}}
	dir := t.TempDir()
	r.app.markPicked(dir)
	if v, err := r.AttachHostedRun(f.url, dir); err != nil || v.Run != nil {
		t.Fatalf("the run panel attached to a draft: %+v %v", v, err)
	}
	// A new app session reattaches the Build tab to it.
	app := &App{ctx: context.Background()}
	b2 := &BuildAPI{app: app, run: &RunAPI{app: app}}
	t.Cleanup(func() {
		if cur := b2.hostedCurrent(); cur != nil && cur.done != nil {
			cur.cancel()
			<-cur.done
		}
	})
	v, err := b2.AttachHostedDraft(f.url)
	if err != nil || v.Draft == nil || v.Draft.ID != draftRunID {
		t.Fatalf("%+v %v", v, err)
	}
	if c, err := b2.CancelHostedDraft(); err != nil || !c.CancelRequested {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestDescribeAgainTextQuotesTheFormsDraftAsData(t *testing.T) {
	b := &BuildAPI{app: &App{ctx: context.Background()}}
	m := build.Default("chris")
	m.Name, m.Instructions = "meeting-actions", "# Meeting actions\n"
	first := "What the person first asked for:\nTurn notes into actions.\n\nWhat the person wants changed now:\nAdd Slack."
	got, err := b.DescribeAgainText(*m, "Post to #general too.", first)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "What the person first asked for:\nTurn notes into actions.\n\nWhat the person wants changed now:\nPost to #general too.\n\nThe current draft's agent.yaml:\n\n```yaml\n") ||
		!strings.Contains(got, "name: meeting-actions") || !strings.HasSuffix(got, "The current draft's AGENT.md:\n\n```markdown\n# Meeting actions\n```") {
		t.Fatalf("got %q", got)
	}
	if _, err := b.DescribeAgainText(*m, "  ", first); err == nil {
		t.Fatal("Describe again with nothing to change")
	}
}

// TestARegistryThatRenamesTheDraftGetsNothingUnpacked: the run's folder is
// named by the id the create answered, and a final poll that names the
// draft otherwise (here a path out of agentry-out) unpacks nothing.
func TestARegistryThatRenamesTheDraftGetsNothingUnpacked(t *testing.T) {
	f := newDraftFake(t)
	b, ev, home := hostedDraftAPI(t, f)
	if _, err := b.DescribeInAgentry(f.url, "Summarise notes.", nil, nil); err != nil {
		t.Fatal(err)
	}
	f.finish(hostedrun.OutcomeSucceeded, draftFakeJSON(t, nil), 20)
	f.mu.Lock()
	f.run.ID = "x/../../../.ssh"
	f.mu.Unlock()
	final := ev.final(t)
	if final.Draft != nil || !strings.Contains(final.DraftError, "was not downloaded") {
		t.Fatalf("final %+v", final)
	}
	if _, err := os.Stat(filepath.Join(home, "Downloads")); !os.IsNotExist(err) {
		t.Fatalf("a folder was made: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".ssh")); !os.IsNotExist(err) {
		t.Fatalf("the renamed draft wrote outside agentry-out: %v", err)
	}
}

// TestADraftSendsNoTokenOverPlainHTTPToARealHost: the token is kept by
// host, so the page's registry URL is held to Settings' rule first
// (agentry-desktop#307's rule, for the draft's bindings).
func TestADraftSendsNoTokenOverPlainHTTPToARealHost(t *testing.T) {
	f := newDraftFake(t)
	b, _, _ := hostedDraftAPI(t, f)
	if _, err := b.DescribeInAgentryStatus("http://dev.secureagentry.net"); err == nil {
		t.Fatal("the status was read over plain http")
	}
	if _, err := b.DescribeInAgentry("http://dev.secureagentry.net", "Summarise notes.", nil, nil); err == nil {
		t.Fatal("a draft was started over plain http")
	}
	if v, err := b.AttachHostedDraft("http://dev.secureagentry.net"); err != nil || v.Draft != nil {
		t.Fatalf("%+v %v", v, err)
	}
}

// A draft In Agentry reads the person's packs kept in Agentry
// (agentry-desktop#497; agentry-enterprise#295): the offer says so, with
// the registry's bounds, only where the status route does and the token
// reads knowledge; the create carries each pack's registry id once; a pack
// not kept in Agentry or not scanned there is refused here, with nothing
// created.
func TestADraftInAgentryReadsPacksKeptInAgentry(t *testing.T) {
	const ready, scanning = "e1f8fcda-25b8-44df-9ebd-d27298fa5f0a", "4cb3f36e-213f-4423-bccb-055d505845e0"
	f := newDraftFake(t)
	f.knowledge = true
	f.packs = []map[string]any{remotePack(ready, "runbook", "ready"), remotePack(scanning, "rescanned", "scanning")}
	b, ev, _ := hostedDraftAPI(t, f)

	v, err := b.DescribeInAgentryStatus(f.url)
	if err != nil || v.KnowledgeEnabled || !strings.Contains(v.KnowledgeNote, "knowledge:read") {
		t.Fatalf("a token without knowledge:read: %+v %v", v, err)
	}
	f.mu.Lock()
	f.scopes = append(f.scopes, "knowledge:read")
	f.mu.Unlock()
	if v, _ = b.DescribeInAgentryStatus(f.url); !v.KnowledgeEnabled || v.MaxPacks != 5 || v.MaxBytes != 10<<20 || v.KnowledgeNote != "" {
		t.Fatalf("the offer: %+v", v)
	}

	for name, want := range map[string]string{"elsewhere": "not kept in Agentry", "rescanned": "not ready in Agentry"} {
		if _, err := b.DescribeInAgentry(f.url, "Answer from the runbook.", nil, []string{name}); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("drafting with %s: %v, want %q", name, err, want)
		}
	}
	f.mu.Lock()
	created := len(f.created)
	f.mu.Unlock()
	if created != 0 {
		t.Fatal("a refused pack still created a draft")
	}
	if _, err := b.DescribeInAgentry(f.url, "Answer from the runbook.", nil, []string{"runbook", "runbook"}); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	body := f.created[0]
	f.mu.Unlock()
	if ids, _ := body["knowledge"].([]any); len(ids) != 1 || ids[0] != ready {
		t.Fatalf("the create's knowledge = %v, want the registry's id once", body["knowledge"])
	}
	f.finish(hostedrun.OutcomeSucceeded, draftFakeJSON(t, nil), 5)
	ev.final(t)
}
