package hostedrun

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSchedules serves x-schedules.yaml's routes for one account, in the
// extension's shapes, and records what each call sent.
type fakeSchedules struct {
	mu    sync.Mutex
	items []Schedule
	// sent is "METHOD path?query body" for each call, in order.
	sent []string
	// refuse, when set, answers a create or an edit.
	refuse func(w http.ResponseWriter)
}

func (f *fakeSchedules) start(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	record := func(r *http.Request) []byte {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		line := r.Method + " " + r.URL.Path
		if r.URL.RawQuery != "" {
			line += "?" + r.URL.RawQuery
		}
		if len(b) > 0 {
			line += " " + string(b)
		}
		f.sent = append(f.sent, line)
		return b
	}
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			problemJSON(w, http.StatusUnauthorized, "unauthorized", "Sign in required", "", nil)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/v1/x/schedules/", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		writeJSON(w, http.StatusOK, map[string]any{"items": f.items, "limit": 25})
	})
	mux.HandleFunc("POST /api/v1/x/schedules/", func(w http.ResponseWriter, r *http.Request) {
		b := record(r)
		if f.refuse != nil {
			f.refuse(w)
			return
		}
		var req ScheduleRequest
		_ = json.Unmarshal(b, &req)
		s := Schedule{ID: "0b6f7a52-5c3c-4b8e-9a43-3f1f2b9d7e10", Name: req.Ref, Ref: req.Ref + "@1.0.0", State: "active", When: *req.When, Repeats: "every day at 09:00 (Europe/London)"}
		writeJSON(w, http.StatusCreated, s)
	})
	mux.HandleFunc("GET /api/v1/x/schedules/occurrences", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		writeJSON(w, http.StatusOK, map[string]any{"from": r.URL.Query().Get("from"), "to": r.URL.Query().Get("to"), "items": []Occurrence{
			{ScheduleID: "s1", Name: "daily", At: "2026-10-10T08:00:00Z", Zone: "Europe/London", State: OccurrenceRan, RunID: "r1"},
			{ScheduleID: "s1", Name: "daily", At: "2026-10-11T08:00:00Z", Zone: "Europe/London", State: OccurrenceUpcoming},
		}})
	})
	mux.HandleFunc("PATCH /api/v1/x/schedules/{id}", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		if f.refuse != nil {
			f.refuse(w)
			return
		}
		writeJSON(w, http.StatusOK, Schedule{ID: r.PathValue("id"), State: "active"})
	})
	mux.HandleFunc("DELETE /api/v1/x/schedules/{id}", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/v1/x/schedules/{id}/pause", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		writeJSON(w, http.StatusOK, Schedule{ID: r.PathValue("id"), State: "paused", PausedReason: "owner"})
	})
	mux.HandleFunc("POST /api/v1/x/schedules/{id}/resume", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		writeJSON(w, http.StatusOK, Schedule{ID: r.PathValue("id"), State: "active"})
	})
	mux.HandleFunc("POST /api/v1/x/schedules/{id}/run", func(w http.ResponseWriter, r *http.Request) {
		record(r)
		writeJSON(w, http.StatusCreated, map[string]string{"runId": "run-1", "scheduleId": r.PathValue("id")})
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestScheduleClientCallsTheExtensionsRoutes(t *testing.T) {
	f := &fakeSchedules{items: []Schedule{{ID: "s1", Name: "daily", State: "active"}}}
	c := New(f.start(t), testToken)
	ctx := context.Background()

	items, limit, err := c.Schedules(ctx)
	if err != nil || len(items) != 1 || limit != 25 {
		t.Fatalf("Schedules = %v, %d, %v", items, limit, err)
	}
	voice := false
	made, err := c.CreateSchedule(ctx, ScheduleRequest{Ref: "acme/hello@1.0.0", Task: "t", Voice: &voice, Inputs: []string{"u1"},
		When: &Recurrence{Kind: "daily", Time: "09:00", Zone: "Europe/London", End: &End{Kind: "after", After: 3}}})
	if err != nil || made.ID == "" {
		t.Fatalf("CreateSchedule = %v, %v", made, err)
	}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	occ, err := c.Occurrences(ctx, from, from.Add(31*24*time.Hour))
	if err != nil || len(occ) != 2 || occ[0].State != OccurrenceRan {
		t.Fatalf("Occurrences = %v, %v", occ, err)
	}
	if _, err := c.UpdateSchedule(ctx, "s1", ScheduleRequest{Name: "renamed"}); err != nil {
		t.Fatal(err)
	}
	if s, err := c.PauseSchedule(ctx, "s1"); err != nil || s.State != "paused" {
		t.Fatalf("PauseSchedule = %v, %v", s, err)
	}
	if _, err := c.ResumeSchedule(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if id, err := c.RunScheduleNow(ctx, "s1"); err != nil || id != "run-1" {
		t.Fatalf("RunScheduleNow = %q, %v", id, err)
	}
	if err := c.DeleteSchedule(ctx, "s1"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"GET /api/v1/x/schedules/",
		`POST /api/v1/x/schedules/ {"ref":"acme/hello@1.0.0","task":"t","voice":false,"inputs":["u1"],"when":{"kind":"daily","time":"09:00","zone":"Europe/London","end":{"kind":"after","after":3}}}`,
		"GET /api/v1/x/schedules/occurrences?from=2026-10-01T00%3A00%3A00Z&to=2026-11-01T00%3A00%3A00Z",
		`PATCH /api/v1/x/schedules/s1 {"name":"renamed"}`,
		"POST /api/v1/x/schedules/s1/pause",
		"POST /api/v1/x/schedules/s1/resume",
		"POST /api/v1/x/schedules/s1/run",
		"DELETE /api/v1/x/schedules/s1",
	}
	if strings.Join(f.sent, "\n") != strings.Join(want, "\n") {
		t.Fatalf("sent:\n%s\nwant:\n%s", strings.Join(f.sent, "\n"), strings.Join(want, "\n"))
	}
}

// A secret held for one run is refused by name (409 secret-not-kept,
// `names`), and the refusal carries the names where a secret-missing one
// carries `missing`, so the app asks for the same names either way.
func TestASecretNotKeptRefusalNamesTheSecrets(t *testing.T) {
	f := &fakeSchedules{refuse: func(w http.ResponseWriter) {
		problemJSON(w, http.StatusConflict, "secret-not-kept", "A secret is set for one run only",
			"A schedule uses secrets kept under Secrets, and API_KEY is set for one run only. Keep it, then make the schedule.",
			map[string]any{"names": []string{"API_KEY"}})
	}}
	c := New(f.start(t), testToken)
	_, err := c.CreateSchedule(context.Background(), ScheduleRequest{Ref: "acme/hello@1.0.0", When: &Recurrence{Kind: "daily", Time: "09:00", Zone: "UTC"}})
	r := RefusalOf(err)
	if r == nil || r.Slug != SlugSecretNotKept || len(r.Missing) != 1 || r.Missing[0] != "API_KEY" {
		t.Fatalf("refusal = %+v (err %v)", r, err)
	}
}

// An id is one path segment: a slash in one cannot reach another route.
func TestAScheduleIDIsOnePathSegment(t *testing.T) {
	c := New("https://registry.example", testToken)
	if got := c.schedulesPath("a/../b", "pause"); got != "https://registry.example/api/v1/x/schedules/a%2F..%2Fb/pause" {
		t.Fatalf("path = %s", got)
	}
}

func TestScheduleOfferMatrix(t *testing.T) {
	yes, no := true, false
	runScopes := []string{ScopeRunRead, ScopeRunWrite}
	cases := []struct {
		name      string
		f         Facts
		offered   bool
		available bool
		upgrade   bool
		reason    string
	}{
		{name: "signed out", f: Facts{Schedules: true}},
		{name: "no schedules extension", f: Facts{SignedIn: true, Scopes: runScopes, ScopesKnown: true}},
		{name: "standard is told the plan, with the way up", f: Facts{SignedIn: true, Schedules: true, ScheduleAllowed: &no, Scopes: runScopes, ScopesKnown: true},
			offered: true, upgrade: true, reason: ScheduleTierReason},
		{name: "the plan before the token", f: Facts{SignedIn: true, Schedules: true, ScheduleAllowed: &no, ScopesKnown: true},
			offered: true, upgrade: true, reason: ScheduleTierReason},
		{name: "scopes unknown", f: Facts{SignedIn: true, Schedules: true, ScheduleAllowed: &yes},
			offered: true, reason: "did not say what the stored token may do"},
		{name: "a token without run:write", f: Facts{SignedIn: true, Schedules: true, ScheduleAllowed: &yes, Scopes: []string{ScopeRunRead}, ScopesKnown: true},
			offered: true, reason: "does not carry run:read and run:write"},
		{name: "pro", f: Facts{SignedIn: true, Schedules: true, ScheduleAllowed: &yes, Scopes: runScopes, ScopesKnown: true},
			offered: true, available: true},
		{name: "the registry did not say the plan: the create decides", f: Facts{SignedIn: true, Schedules: true, Scopes: runScopes, ScopesKnown: true},
			offered: true, available: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := DecideSchedule(c.f)
			if o.Offered != c.offered || o.Available != c.available || o.Upgrade != c.upgrade || !strings.Contains(o.Reason, c.reason) {
				t.Fatalf("DecideSchedule = %+v", o)
			}
			if c.offered && !c.available && o.Remedy == "" {
				t.Fatalf("an offer that cannot be chosen says no way out: %+v", o)
			}
		})
	}
}
