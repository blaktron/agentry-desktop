package hostedrun

// A fake runs extension for this package's tests: the routes of
// agentry-enterprise/schemas/x-runs.yaml an account calls, with the same
// shapes, the same problem documents and the same SSE frames. Kept in this
// package's own style rather than shared with internal/run's fake, which
// serves releases and archives and nothing hosted.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const testToken = "agy_testtoken00000000"

// fakeRuns serves one account's runs.
type fakeRuns struct {
	mu sync.Mutex

	// enabled, workersOnline and queued are the status route's answer;
	// caller is its signed-in block (nil = not reported).
	enabled       bool
	workersOnline int
	queued        int
	caller        *Caller

	// run is the one run this registry holds, if any.
	run *Run
	// events are its log rows.
	events []Event
	// outputs are the artefact bodies by name.
	outputs map[string][]byte

	// createRefusal, when set, is the problem a create answers with.
	createRefusal func(w http.ResponseWriter)

	// cutAfter closes the stream without `done` after this many events on
	// the first connection, so a resume can be proved; replayAll makes the
	// second connection ignore ?after= and send everything again, so the
	// de-duplication can be proved too.
	cutAfter  int
	replayAll bool
	// streams counts the event-stream connections served.
	streams int
	// afterSeen records the ?after= of each connection.
	afterSeen []int64
	// uploads records what the input route stored.
	uploads []Input
}

func (f *fakeRuns) start(t *testing.T) string {
	t.Helper()
	if f.outputs == nil {
		f.outputs = map[string][]byte{}
	}
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			problemJSON(w, http.StatusUnauthorized, "unauthorized", "Sign in required", "This request carried no valid session or token.", nil)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/v1/x/runs/status", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		out := map[string]any{"enabled": f.enabled, "workersOnline": f.workersOnline, "queued": f.queued}
		if f.caller != nil {
			out["caller"] = f.caller
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("POST /api/v1/x/runs/", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.createRefusal != nil {
			f.createRefusal(w)
			return
		}
		var req CreateRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil {
			problemJSON(w, http.StatusBadRequest, "bad-request", "Malformed body", "", nil)
			return
		}
		f.run = &Run{
			ID: "11111111-1111-1111-1111-111111111111", Ref: req.Ref, Digest: "sha256:abc",
			Mode: "autonomous", Status: StatusQueued, Caps: Caps{TimeoutSeconds: 300, MaxTurns: 20, MaxTokens: 20000},
			HoldCredits: 20, QueuePosition: 2, CreatedAt: time.Now().UTC(), Outputs: []Artefact{},
		}
		writeJSON(w, http.StatusCreated, f.run)
	})
	mux.HandleFunc("GET /api/v1/x/runs/", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		items := []*Run{}
		if f.run != nil {
			items = append(items, f.run)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /api/v1/x/runs/inputs", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		name := r.URL.Query().Get("name")
		body := make([]byte, 0, 1024)
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			body = append(body, buf[:n]...)
			if err != nil {
				break
			}
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		in := Input{ID: fmt.Sprintf("input-%d", len(f.uploads)+1), Name: name, Bytes: int64(len(body))}
		f.uploads = append(f.uploads, in)
		writeJSON(w, http.StatusCreated, in)
	})
	mux.HandleFunc("GET /api/v1/x/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.run == nil || f.run.ID != r.PathValue("id") {
			problemJSON(w, http.StatusNotFound, "not-found", "Not found", "", nil)
			return
		}
		writeJSON(w, http.StatusOK, f.run)
	})
	mux.HandleFunc("POST /api/v1/x/runs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.run == nil || f.run.ID != r.PathValue("id") {
			problemJSON(w, http.StatusNotFound, "not-found", "Not found", "", nil)
			return
		}
		if f.run.Status == StatusQueued {
			f.run.Status = StatusCancelled
		} else {
			f.run.CancelRequested = true
		}
		writeJSON(w, http.StatusOK, f.run)
	})
	mux.HandleFunc("GET /api/v1/x/runs/{id}/outputs/{name...}", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		body, ok := f.outputs[r.PathValue("name")]
		f.mu.Unlock()
		if !ok {
			problemJSON(w, http.StatusNotFound, "not-found", "Not found", "", nil)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("GET /api/v1/x/runs/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		if last := r.Header.Get("Last-Event-ID"); last != "" {
			if n, err := strconv.ParseInt(last, 10, 64); err == nil {
				after = n
			}
		}
		f.mu.Lock()
		f.streams++
		conn := f.streams
		f.afterSeen = append(f.afterSeen, after)
		evs := append([]Event(nil), f.events...)
		cut, replay := f.cutAfter, f.replayAll
		terminal := f.run != nil && !Active(f.run.Status)
		f.mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		sent := 0
		for _, ev := range evs {
			if ev.Seq <= after && (!replay || conn <= 1) {
				continue
			}
			payload, _ := json.Marshal(ev)
			_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Kind, payload)
			sent++
			if conn == 1 && cut > 0 && sent >= cut {
				return // the connection drops mid-stream, with no `done`
			}
		}
		if terminal {
			_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
		}
	})
	// The core routes the availability rule reads.
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"version": "1", "apiVersion": "v1", "features": map[string]bool{}, "extensions": []string{"billing", "runs"}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// finish moves the fake's run to a finished state with a receipt.
func (f *fakeRuns) finish(outcome, exitClass string, settled int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	f.run.Status = StatusFinished
	f.run.Outcome = outcome
	f.run.ExitClass = exitClass
	f.run.FinishedAt = &now
	f.run.SettledCredits = &settled
	f.run.Usage = &Usage{InputTokens: 900, OutputTokens: 300, Turns: 4, WallMS: 12000, CostUSD: 0.02}
	f.run.Receipt = Receipt{CapabilitiesEnforced: map[string]string{
		"network.egress": "llm-proxy allowlist: api.example.com",
		"filesystem":     "workspace bind mount, read-only /inputs",
	}}
	names := make([]Artefact, 0, len(f.outputs))
	for name, body := range f.outputs {
		names = append(names, Artefact{Name: name, Bytes: int64(len(body))})
	}
	f.run.Outputs = names
}

func (f *fakeRuns) addEvents(evs ...Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, evs...)
}

func logEvents(lines ...string) []Event {
	out := make([]Event, 0, len(lines))
	for i, l := range lines {
		out = append(out, Event{Seq: int64(i + 1), At: time.Now().UTC(), Kind: "log", Line: l})
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// problemJSON writes the extension's RFC 9457 shape.
func problemJSON(w http.ResponseWriter, status int, slug, title, detail string, fields map[string]any) {
	body := map[string]any{"type": "https://agentry.dev/problems/" + slug, "title": title, "status": status}
	if detail != "" {
		body["detail"] = detail
	}
	for k, v := range fields {
		body[k] = v
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// creditsExhausted is the 402 the create answers when the balance is short
// of the hold (x-billing.yaml's shape, as the runs create returns it).
func creditsExhausted(w http.ResponseWriter) {
	problemJSON(w, http.StatusPaymentRequired, "credits-exhausted", "Not enough credits for this run",
		"a run holds 20 credits; this account has 4, and its allowance resets on 11 October 2026",
		map[string]any{"balance": 4, "hold": 20, "resetsAt": "2026-10-11T00:00:00Z", "tier": "standard"})
}

// runActive is the 409 a second create meets.
func runActive(w http.ResponseWriter) {
	problemJSON(w, http.StatusConflict, "run-active", "A run is already active",
		"One hosted run at a time per account: wait for it to finish, or cancel it.", nil)
}

// recorder is a round tripper that records every request's headers, so a
// test can prove what credentials the app sends — and what it does not.
type recordingTransport struct {
	seen *[]string
}

func (rt recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var b strings.Builder
	for k, vs := range req.Header {
		for _, v := range vs {
			b.WriteString(k + ": " + v + "\n")
		}
	}
	*rt.seen = append(*rt.seen, b.String())
	return http.DefaultTransport.RoundTrip(req)
}

func recorder(seen *[]string) http.RoundTripper { return recordingTransport{seen: seen} }

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("%q does not contain %q", got, want)
	}
}
