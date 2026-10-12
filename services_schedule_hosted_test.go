package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/schedules"
)

// scheduleRoutes is the schedules extension's side of the fake registry
// (agentry-enterprise schemas/x-schedules.yaml): the account's schedules,
// its occurrences, and what each call sent.
type scheduleRoutes struct {
	mu          sync.Mutex
	items       []hostedrun.Schedule
	occurrences []hostedrun.Occurrence
	sent        []string
	// refuse, when set, answers a create.
	refuse func(w http.ResponseWriter)
}

func (sr *scheduleRoutes) add(mux *http.ServeMux) {
	rec := func(r *http.Request) []byte {
		b, _ := io.ReadAll(r.Body)
		sr.mu.Lock()
		defer sr.mu.Unlock()
		line := r.Method + " " + r.URL.Path
		if len(b) > 0 {
			line += " " + string(b)
		}
		sr.sent = append(sr.sent, line)
		return b
	}
	authed := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+hostedTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/v1/x/schedules/", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		rec(r)
		sr.mu.Lock()
		defer sr.mu.Unlock()
		hostedJSON(w, http.StatusOK, map[string]any{"items": sr.items, "limit": 25})
	})
	mux.HandleFunc("POST /api/v1/x/schedules/", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		b := rec(r)
		if sr.refuse != nil {
			sr.refuse(w)
			return
		}
		var req hostedrun.ScheduleRequest
		_ = json.Unmarshal(b, &req)
		s := hostedrun.Schedule{ID: "6f1c2d3e-4b5a-4c6d-8e7f-8091a2b3c4d5", Name: req.Ref, Ref: req.Ref, State: "active", When: *req.When, NextAt: "2026-10-10T08:00:00Z"}
		hostedJSON(w, http.StatusCreated, s)
	})
	mux.HandleFunc("GET /api/v1/x/schedules/occurrences", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		rec(r)
		from, _ := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		to, _ := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		sr.mu.Lock()
		defer sr.mu.Unlock()
		out := []hostedrun.Occurrence{}
		for _, o := range sr.occurrences {
			at, _ := time.Parse(time.RFC3339, o.At)
			if !at.Before(from) && at.Before(to) {
				out = append(out, o)
			}
		}
		hostedJSON(w, http.StatusOK, map[string]any{"from": from, "to": to, "items": out})
	})
	mux.HandleFunc("PATCH /api/v1/x/schedules/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		rec(r)
		hostedJSON(w, http.StatusOK, hostedrun.Schedule{ID: r.PathValue("id"), State: "active"})
	})
	mux.HandleFunc("DELETE /api/v1/x/schedules/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		rec(r)
		w.WriteHeader(http.StatusNoContent)
	})
	for _, verb := range []string{"pause", "resume"} {
		mux.HandleFunc("POST /api/v1/x/schedules/{id}/"+verb, func(w http.ResponseWriter, r *http.Request) {
			if !authed(w, r) {
				return
			}
			rec(r)
			hostedJSON(w, http.StatusOK, hostedrun.Schedule{ID: r.PathValue("id"), State: "active"})
		})
	}
	mux.HandleFunc("POST /api/v1/x/schedules/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		if !authed(w, r) {
			return
		}
		rec(r)
		hostedJSON(w, http.StatusConflict, map[string]any{"type": "https://agentry.dev/problems/run-active", "title": "A run is active", "detail": "Wait for it to end."})
	})
}

func (sr *scheduleRoutes) calls() []string {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	return append([]string(nil), sr.sent...)
}

const hostedScheduleID = "6f1c2d3e-4b5a-4c6d-8e7f-8091a2b3c4d5"

// hostedScheduleAPI is a ScheduleAPI signed in to a fake registry whose
// /meta lists schedules, with an empty store on this computer.
func hostedScheduleAPI(t *testing.T, sr *scheduleRoutes, set func(*hostedFake)) (*ScheduleAPI, string, *hostedFake) {
	t.Helper()
	allowed := true
	f := &hostedFake{enabled: true, extensions: []string{"billing", "runs", "vault", "schedules"}, sessionAllowed: &allowed, routes: sr.add}
	if set != nil {
		set(f)
	}
	reg := f.start(t)
	signIn(t, reg)
	emit := emitEvent
	emitEvent = func(*App, string, ...any) {}
	t.Cleanup(func() { emitEvent = emit })
	home := t.TempDir()
	app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
	return &ScheduleAPI{app: app, run: &RunAPI{app: app}, home: func() (string, error) { return home, nil },
		locateCLI: func() (string, error) { return "/usr/bin/agentry", nil }}, reg, f
}

// The run setup's offer (D7): Pro is offered Later and Repeat In Agentry,
// Standard is told the plan with the way up, and a registry without the
// schedules extension offers nothing.
func TestTheRunSetupOffersSchedulesInAgentryByPlan(t *testing.T) {
	for _, c := range []struct {
		name      string
		set       func(*hostedFake)
		offered   bool
		available bool
		upgrade   bool
	}{
		{name: "pro", offered: true, available: true},
		{name: "standard", set: func(f *hostedFake) { no := false; f.sessionAllowed = &no }, offered: true, upgrade: true},
		{name: "no schedules extension", set: func(f *hostedFake) { f.extensions = []string{"billing", "runs"} }},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, reg, _ := hostedScheduleAPI(t, &scheduleRoutes{}, c.set)
			r, _ := newRunAPI(t)
			v, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			got := v.Schedule
			if !c.offered {
				if got != nil {
					t.Fatalf("offered where the registry schedules nothing: %+v", got)
				}
				return
			}
			if got == nil || got.Available != c.available || got.Upgrade != c.upgrade {
				t.Fatalf("schedule offer = %+v", got)
			}
		})
	}
}

// A schedule In Agentry is made with the run's own setup: the picked file
// uploaded, the task, the answers, the voice, and When in the registry's
// shape, its zone named.
func TestHostedAddMakesTheScheduleWithTheRunsSetup(t *testing.T) {
	sr := &scheduleRoutes{}
	s, reg, f := hostedScheduleAPI(t, sr, nil)
	in := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(in, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := HostedScheduleRequest{Registry: reg, Ref: "acme/reporter@1.0.0", Task: "sum up", Files: []string{in}, Intake: map[string]string{"topic": "x"}, Voice: true,
		When: schedules.When{Kind: "weekly", Days: []string{"tue"}, Time: "22:00", End: "after", Times: 4, Zone: "Europe/London"}}

	// A file no picker returned is refused before anything is sent (D-030).
	if _, err := s.HostedAdd(req); err == nil || len(sr.calls()) != 0 {
		t.Fatalf("an unpicked file: %v, sent %v", err, sr.calls())
	}
	s.app.markPicked(in)
	made, err := s.HostedAdd(req)
	if err != nil || made.ID != hostedScheduleID || made.Refusal != "" {
		t.Fatalf("HostedAdd = %+v, %v", made, err)
	}
	f.mu.Lock()
	uploads := f.uploads
	f.mu.Unlock()
	if uploads != 1 {
		t.Fatalf("uploads = %d, want the one file", uploads)
	}
	calls := sr.calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %v", calls)
	}
	for _, want := range []string{`"ref":"acme/reporter@1.0.0"`, `"task":"sum up"`, `"intake":{"topic":"x"}`, `"voice":true`, `"inputs":["`,
		`"when":{"kind":"weekly","time":"22:00","days":["tue"],"zone":"Europe/London","end":{"kind":"after","after":4}}`} {
		if !strings.Contains(calls[0], want) {
			t.Fatalf("create %s\nlacks %s", calls[0], want)
		}
	}
}

// What a fire would lack is the create's refusal, given back to the When
// step to fix in place, never an error (D8): a secret held for one run is
// named so the step asks for it kept.
func TestHostedAddGivesTheGapBack(t *testing.T) {
	sr := &scheduleRoutes{refuse: func(w http.ResponseWriter) {
		hostedJSON(w, http.StatusConflict, map[string]any{"type": "https://agentry.dev/problems/secret-not-kept", "title": "A secret is set for one run only",
			"detail": "A schedule uses secrets kept under Secrets, and API_KEY is set for one run only.", "names": []string{"API_KEY"}})
	}}
	s, reg, _ := hostedScheduleAPI(t, sr, nil)
	made, err := s.HostedAdd(HostedScheduleRequest{Registry: reg, Ref: "acme/reporter@1.0.0", When: schedules.When{Kind: "daily", Time: "09:00", Zone: "UTC"}})
	if err != nil || made.Refusal != hostedrun.SlugSecretNotKept || len(made.Missing) != 1 || made.Missing[0] != "API_KEY" || made.ID != "" {
		t.Fatalf("HostedAdd = %+v, %v", made, err)
	}
	// A repeat out of shape is refused here, before the registry is asked.
	if _, err := s.HostedAdd(HostedScheduleRequest{Registry: reg, Ref: "acme/reporter@1.0.0", When: schedules.When{Kind: "daily", Time: "9am"}}); err == nil {
		t.Fatal("a time out of shape was sent")
	}
}

// agentry-desktop#795: a schedule asked to ask for the optional keys sends
// ask_optional_secrets to the registry's check, one not asked sends nothing
// of it, and the check's secret-missing comes back with its optional names.
func TestHostedAddAsksForTheOptionalKeysOnlyWhenTold(t *testing.T) {
	sr := &scheduleRoutes{refuse: func(w http.ResponseWriter) {
		hostedJSON(w, http.StatusConflict, map[string]any{"type": "https://agentry.dev/problems/secret-missing", "title": "A secret is missing",
			"missing": []string{"OPT_KEY"}, "secrets": []string{"OPT_KEY"}, "optional": []string{"OPT_KEY"}})
	}}
	s, reg, _ := hostedScheduleAPI(t, sr, nil)
	req := HostedScheduleRequest{Registry: reg, Ref: "acme/reporter@1.0.0", When: schedules.When{Kind: "daily", Time: "09:00", Zone: "UTC"}, AskOptionalSecrets: true}
	made, err := s.HostedAdd(req)
	if err != nil || made.Refusal != "secret-missing" || strings.Join(made.Missing, ",") != "OPT_KEY" || strings.Join(made.Optional, ",") != "OPT_KEY" {
		t.Fatalf("HostedAdd = %+v, %v", made, err)
	}
	req.AskOptionalSecrets = false
	if _, err := s.HostedAdd(req); err != nil {
		t.Fatal(err)
	}
	calls := sr.calls()
	if len(calls) != 2 || !strings.Contains(calls[0], `"ask_optional_secrets":true`) || strings.Contains(calls[1], "ask_optional_secrets") {
		t.Fatalf("calls = %v", calls)
	}
}

// The page's list: each schedule In Agentry in the local list's shape,
// marked hosted; a registry that cannot be reached is a sentence, never
// an error, so the schedules on this computer still show.
func TestHostedSchedulesListAndAnUnreachableRegistry(t *testing.T) {
	sr := &scheduleRoutes{items: []hostedrun.Schedule{{ID: hostedScheduleID, Name: "Morning digest", Ref: "acme/reporter@1.0.0", State: "paused", PausedReason: "refusals",
		Repeats: "every day at 09:00 (Europe/London)", When: hostedrun.Recurrence{Kind: "daily", Time: "09:00", Zone: "Europe/London", End: &hostedrun.End{Kind: "never"}},
		Inputs: []hostedrun.ScheduleInput{{Name: "notes.txt", Bytes: 5}}, Refusals: 3}}}
	s, reg, _ := hostedScheduleAPI(t, sr, nil)
	v, err := s.HostedSchedules(reg)
	if err != nil || v.Unavailable != "" || v.Limit != 25 || len(v.Schedules) != 1 {
		t.Fatalf("HostedSchedules = %+v, %v", v, err)
	}
	// The list is one read of the registry; the offer is read apart.
	if calls := sr.calls(); len(calls) != 1 || calls[0] != "GET /api/v1/x/schedules/" {
		t.Fatalf("the list read %v", calls)
	}
	if o, err := s.HostedOffer(reg); err != nil || o == nil || !o.Available {
		t.Fatalf("HostedOffer = %+v, %v", o, err)
	}
	it := v.Schedules[0]
	if !it.Hosted || it.Ref != "acme/reporter" || it.Version != "1.0.0" || !it.Paused || it.PausedReason != "refusals" || it.Describe == "" ||
		it.When.Kind != "daily" || it.When.Zone != "Europe/London" || len(it.Files) != 1 {
		t.Fatalf("item = %+v", it)
	}

	// Nobody answers at that address now.
	gone := "http://127.0.0.1:1"
	signIn(t, gone)
	v, err = s.HostedSchedules(gone)
	if err != nil || v.Unavailable == "" || len(v.Schedules) != 0 {
		t.Fatalf("an unreachable registry: %+v, %v", v, err)
	}
	cal, err := s.HostedCalendar(gone, "2026-09-28T00:00:00Z", "2026-11-02T00:00:00Z")
	if err != nil || cal.Unavailable == "" || len(cal.Fires) != 0 {
		t.Fatalf("an unreachable calendar: %+v, %v", cal, err)
	}
}

// The calendar's words: a coming occurrence is due, as a local one is, one
// being tried is pending, and a fire that started no run keeps its reason.
func TestHostedCalendarStates(t *testing.T) {
	sr := &scheduleRoutes{occurrences: []hostedrun.Occurrence{
		{ScheduleID: hostedScheduleID, Name: "d", At: "2026-10-05T08:00:00Z", State: "ran", RunID: "r1"},
		{ScheduleID: hostedScheduleID, Name: "d", At: "2026-10-06T08:00:00Z", State: "skipped", Reason: "credits"},
		{ScheduleID: hostedScheduleID, Name: "d", At: "2026-10-07T08:00:00Z", State: "firing"},
		{ScheduleID: hostedScheduleID, Name: "d", At: "2026-10-08T08:00:00Z", State: "upcoming"},
	}}
	s, reg, _ := hostedScheduleAPI(t, sr, nil)
	cal, err := s.HostedCalendar(reg, "2026-09-28T00:00:00Z", "2026-11-02T00:00:00Z")
	if err != nil || cal.Unavailable != "" {
		t.Fatalf("HostedCalendar = %+v, %v", cal, err)
	}
	var got []string
	for _, f := range cal.Fires {
		got = append(got, f.Outcome+":"+f.Reason+":"+f.RunID)
	}
	if strings.Join(got, ",") != "ran::r1,skipped:credits:,pending::,due::" {
		t.Fatalf("outcomes = %v", got)
	}
	s.now = func() time.Time { return time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC) }
	h, err := s.HostedHistory(reg, 20)
	if err != nil || len(h.Fires) != 3 || h.Fires[0].At != "2026-10-07T08:00:00Z" {
		t.Fatalf("history = %+v, %v", h, err)
	}
}

// A registry without the schedules extension answers its route 404: the
// calendar has nothing In Agentry, and says nothing is wrong.
func TestHostedCalendarWithoutTheExtension(t *testing.T) {
	s, reg, _ := hostedScheduleAPI(t, &scheduleRoutes{}, func(f *hostedFake) {
		f.extensions = []string{"billing", "runs"}
		f.routes = nil
	})
	cal, err := s.HostedCalendar(reg, "2026-09-28T00:00:00Z", "2026-11-02T00:00:00Z")
	if err != nil || cal.Unavailable != "" || len(cal.Fires) != 0 {
		t.Fatalf("HostedCalendar = %+v, %v", cal, err)
	}
	if v, err := s.HostedSchedules(reg); err != nil || v.Unavailable != "" || len(v.Schedules) != 0 {
		t.Fatalf("HostedSchedules = %+v, %v", v, err)
	}
	if o, err := s.HostedOffer(reg); o != nil || err != nil {
		t.Fatalf("an offer without the extension: %+v, %v", o, err)
	}
}

// Each action calls its route with an id checked first; Run now's refusal
// is a sentence, not an error.
func TestHostedActionsCallTheirRoutes(t *testing.T) {
	sr := &scheduleRoutes{}
	s, reg, _ := hostedScheduleAPI(t, sr, nil)
	if err := s.HostedPause(reg, "../x"); err == nil || len(sr.calls()) != 0 {
		t.Fatalf("an id out of shape reached the registry: %v %v", err, sr.calls())
	}
	if err := s.HostedEdit(reg, hostedScheduleID, schedules.Edit{Name: " Renamed ", When: &schedules.When{Kind: "monthly", MonthDay: 31, Time: "07:30", Zone: "UTC"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.HostedPause(reg, hostedScheduleID); err != nil {
		t.Fatal(err)
	}
	if err := s.HostedResume(reg, hostedScheduleID); err != nil {
		t.Fatal(err)
	}
	now, err := s.HostedRunNow(reg, hostedScheduleID)
	if err != nil || now.Refusal != "run-active" || now.Detail == "" {
		t.Fatalf("HostedRunNow = %+v, %v", now, err)
	}
	// A No in the native dialog deletes nothing (D-070).
	s.app.confirm = func(context.Context, string, string) (bool, error) { return false, nil }
	if ok, err := s.HostedRemove(reg, hostedScheduleID, "Renamed"); ok || err != nil {
		t.Fatalf("a declined Delete: %v, %v", ok, err)
	}
	s.app.confirm = func(context.Context, string, string) (bool, error) { return true, nil }
	if ok, err := s.HostedRemove(reg, hostedScheduleID, "Renamed"); !ok || err != nil {
		t.Fatalf("HostedRemove = %v, %v", ok, err)
	}
	want := []string{
		"PATCH /api/v1/x/schedules/" + hostedScheduleID + ` {"name":"Renamed","when":{"kind":"monthly","time":"07:30","monthDay":31,"zone":"UTC"}}`,
		"POST /api/v1/x/schedules/" + hostedScheduleID + "/pause",
		"POST /api/v1/x/schedules/" + hostedScheduleID + "/resume",
		"POST /api/v1/x/schedules/" + hostedScheduleID + "/run",
		"DELETE /api/v1/x/schedules/" + hostedScheduleID,
	}
	if got := sr.calls(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Home's card holds both: the month In Agentry beside this computer's, and
// the next three from both, soonest first; a registry that does not answer
// is a sentence on the card, and the local half is as it was.
func TestHomeCardMergesSchedulesInAgentry(t *testing.T) {
	sr := &scheduleRoutes{
		items: []hostedrun.Schedule{{ID: hostedScheduleID, Name: "hosted", State: "active"}},
		occurrences: []hostedrun.Occurrence{
			{ScheduleID: hostedScheduleID, Name: "hosted", At: "2026-10-05T08:00:00Z", State: "ran", RunID: "r1"},
			{ScheduleID: hostedScheduleID, Name: "hosted", At: "2026-10-31T08:00:00Z", State: "upcoming"},
			{ScheduleID: hostedScheduleID, Name: "hosted", At: "2026-11-30T08:00:00Z", State: "upcoming"},
		},
	}
	s, reg, _ := hostedScheduleAPI(t, sr, nil)
	s.now = func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }
	v, err := s.HomeCard("2026-09-28T00:00:00Z", "2026-11-02T00:00:00Z", reg)
	if err != nil || v.HostedReason != "" || v.HostedSchedules != 1 || len(v.HostedMonth) != 2 {
		t.Fatalf("card = %+v, %v", v, err)
	}
	var next []string
	for _, n := range v.Next {
		next = append(next, n.At)
	}
	if strings.Join(next, ",") != "2026-10-31T08:00:00Z,2026-11-30T08:00:00Z" || !v.Next[0].Hosted {
		t.Fatalf("next = %v", v.Next)
	}

	gone := "http://127.0.0.1:1"
	signIn(t, gone)
	v, err = s.HomeCard("2026-09-28T00:00:00Z", "2026-11-02T00:00:00Z", gone)
	if err != nil || v.HostedReason == "" || len(v.HostedMonth) != 0 || v.Next == nil {
		t.Fatalf("an unreachable registry: %+v, %v", v, err)
	}
}

// Signed out of the registry: nothing In Agentry is asked for, and the
// page is told signing in shows them.
func TestHostedSchedulesSignedOut(t *testing.T) {
	s, _, _ := hostedScheduleAPI(t, &scheduleRoutes{}, nil)
	v, err := s.HostedSchedules("https://elsewhere.example")
	if err != nil || !v.SignedOut || v.Unavailable != "" || len(v.Schedules) != 0 {
		t.Fatalf("signed out: %+v, %v", v, err)
	}
	if o, err := s.HostedOffer("https://elsewhere.example"); o != nil || err != nil {
		t.Fatalf("an offer signed out: %+v, %v", o, err)
	}
}

// A schedule In Agentry given a source with a token (typed, or the key
// saved on this computer for its host) saves it in Agentry as the host's
// key before the schedule is made, with the person's leave, since its
// fires use the key Agentry keeps (agentry-enterprise#532); without the
// leave it is held, nothing sent. A public source with no token is
// scheduled as given, and no key is saved (agentry-desktop#833).
func TestHostedAddSavesASourcesKeyInAgentryWithLeave(t *testing.T) {
	isolateHostedKeychain(t)
	sr := &scheduleRoutes{}
	s, reg, f := hostedScheduleAPI(t, sr, func(f *hostedFake) {
		f.intake = "intake:\n  - id: codebase\n    prompt: \"The repository\"\n    type: repository\n"
	})
	const saved = "ghp-saved-for-the-schedule-4d2b"
	if err := run.SaveSourceKey("repository", "https://github.com/acme/app", saved); err != nil {
		t.Fatal(err)
	}
	req := HostedScheduleRequest{Registry: reg, Ref: "acme/reporter@1.0.0", Intake: map[string]string{"codebase": "https://github.com/acme/app"},
		When: schedules.When{Kind: "daily", Time: "07:00", Zone: "UTC"}}

	_, err := s.HostedAdd(req)
	if err == nil || !strings.Contains(err.Error(), "github.com") || strings.Contains(err.Error(), saved) {
		t.Fatalf("without leave: %v", err)
	}
	f.mu.Lock()
	sets := len(f.vaultSets)
	f.mu.Unlock()
	if sets != 0 || len(sr.calls()) != 0 {
		t.Fatalf("without leave, %d keys saved and calls %v", sets, sr.calls())
	}

	req.SaveSourceKeys = true
	made, err := s.HostedAdd(req)
	if err != nil || made.ID != hostedScheduleID {
		t.Fatalf("with leave: %+v %v", made, err)
	}
	f.mu.Lock()
	got := slices.Clone(f.vaultSets)
	f.mu.Unlock()
	if len(got) != 1 || got[0].Name != "source-key:github.com" || !strings.Contains(got[0].Body, saved) {
		t.Fatalf("keys saved: %+v", got)
	}
	for _, c := range sr.calls() {
		if strings.Contains(c, saved) {
			t.Fatalf("the schedule's create carries the key: %s", c)
		}
	}

	// A public source with no token: scheduled, no key saved.
	keyring.MockInit()
	pub := HostedScheduleRequest{Registry: reg, Ref: "acme/reporter@1.0.0", Intake: map[string]string{"codebase": "https://gitlab.com/acme/public"},
		When: schedules.When{Kind: "daily", Time: "07:00", Zone: "UTC"}}
	if made, err := s.HostedAdd(pub); err != nil || made.ID != hostedScheduleID {
		t.Fatalf("a public source: %+v %v", made, err)
	}
	f.mu.Lock()
	sets = len(f.vaultSets)
	f.mu.Unlock()
	if sets != 1 {
		t.Fatalf("a public source saved a key: %d", sets)
	}
}

// A key the vault refuses once the schedule is made (a key saved from the
// site, which a token may not replace: 409 secret-kept) takes the schedule
// back, so it never fires without the key it was given, and the refusal
// is the When step's to show.
func TestHostedAddTakesTheScheduleBackWhenItsKeyIsNotSaved(t *testing.T) {
	isolateHostedKeychain(t)
	sr := &scheduleRoutes{}
	s, reg, _ := hostedScheduleAPI(t, sr, func(f *hostedFake) {
		f.intake = "intake:\n  - id: codebase\n    prompt: \"The repository\"\n    type: repository\n"
		f.vaultAnswer = func(w http.ResponseWriter, _ []byte) {
			hostedJSON(w, http.StatusConflict, map[string]any{"type": "https://agentry.dev/problems/secret-kept", "title": "The key is kept", "detail": "Change it on the Secrets page."})
		}
	})
	made, err := s.HostedAdd(HostedScheduleRequest{Registry: reg, Ref: "acme/reporter@1.0.0", Intake: map[string]string{"codebase": "https://github.com/acme/app"},
		SourceTokens: map[string]string{"codebase": "ghp-typed-for-the-schedule"}, SaveSourceKeys: true, When: schedules.When{Kind: "daily", Time: "07:00", Zone: "UTC"}})
	if err != nil || made.ID != "" || made.Refusal != "secret-kept" {
		t.Fatalf("HostedAdd = %+v, %v", made, err)
	}
	calls := sr.calls()
	if len(calls) != 2 || !strings.HasPrefix(calls[0], "POST /api/v1/x/schedules/") || calls[1] != "DELETE /api/v1/x/schedules/"+hostedScheduleID {
		t.Fatalf("calls %v", calls)
	}
}
