package hostedrun

// A fake runs extension for this package's tests: the routes of
// agentry-enterprise/schemas/x-runs.yaml an account calls, with the same
// shapes and the same problem documents. Kept in this package's own style
// rather than shared with internal/run's fake, which serves releases and
// archives and nothing hosted.

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
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
	// steps move the run on as it is read: each read of the run applies
	// the next one first, so a watch sees the run advance.
	steps []func(*Run)
	// outputs are the artefact bodies by name. pushed serves them the way
	// a pushed run does: one output.tar holding them all.
	outputs map[string][]byte
	pushed  bool
	// bundle, when set, is the output.tar served as it is, for a test
	// that needs one no host would write.
	bundle []byte

	// createRefusal, when set, is the problem a create answers with.
	createRefusal func(w http.ResponseWriter)

	// eventsRead counts the requests to the run's events route, which a
	// client of a pushed run must never make.
	eventsRead int
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
			Mode: "autonomous", Status: StatusQueued, Caps: Caps{TimeoutSeconds: 300, MaxTurns: 20},
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
		if len(f.steps) > 0 {
			f.steps[0](f.run)
			f.steps = f.steps[1:]
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
	// The outputs route reads the artefact name from the raw path rather than
	// through the mux, which would clean "../x" away and answer 404 before any
	// test of the client's own containment could reach it: this fake serves
	// whatever name it holds, escaping ones included, the way a hostile registry
	// would.
	outputs := func(w http.ResponseWriter, r *http.Request, name string) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		body, ok := f.outputs[name]
		if f.pushed {
			body, ok = f.bundle, name == PushedOutput
			if body == nil {
				body = tarOf(f.outputs)
			}
		}
		f.mu.Unlock()
		if !ok {
			problemJSON(w, http.StatusNotFound, "not-found", "Not found", "", nil)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}
	mux.HandleFunc("GET /api/v1/x/runs/{id}/events", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		f.eventsRead++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
	})
	// The core routes the availability rule reads.
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"version": "1", "apiVersion": "v1", "features": map[string]bool{}, "extensions": []string{"billing", "runs"}})
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/x/runs/") {
			if _, name, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/v1/x/runs/"), "/outputs/"); ok {
				outputs(w, r, name)
				return
			}
		}
		mux.ServeHTTP(w, r)
	}))
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
	f.run.Usage = &Usage{Turns: 4, WallMS: 12000}
	f.run.Receipt = Receipt{CapabilitiesEnforced: map[string]string{
		"network.egress": "llm-proxy allowlist: api.example.com",
		"filesystem":     "workspace bind mount, read-only /inputs",
	}}
	names := make([]Artefact, 0, len(f.outputs))
	for name, body := range f.outputs {
		names = append(names, Artefact{Name: name, Bytes: int64(len(body))})
	}
	if f.pushed {
		// The sealed object's size: the bundle plus the seal's overhead.
		names = []Artefact{{Name: PushedOutput, Bytes: int64(len(tarOf(f.outputs))) + 1024}}
	}
	f.run.Outputs = names
}

// step moves the run to a status and a queue position on its next read.
func step(s string, position int) func(*Run) {
	return func(r *Run) { r.Status, r.QueuePosition = s, position }
}

// tarOf is the bundle a host writes: every file, regular, named relative
// to the run's out folder, in name order.
func tarOf(files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, n := range names {
		if err := tw.WriteHeader(&tar.Header{Name: n, Mode: 0o600, Size: int64(len(files[n])), Typeflag: tar.TypeReg}); err != nil {
			panic(err) // a bytes.Buffer does not fail
		}
		if _, err := tw.Write(files[n]); err != nil {
			panic(err) // a bytes.Buffer does not fail
		}
	}
	if err := tw.Close(); err != nil {
		panic(err) // a bytes.Buffer does not fail
	}
	return buf.Bytes()
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

// sentRequest is everything one request carried: a credential can travel
// in any of these, so a test of "only the account's token travels" reads
// them all.
type sentRequest struct {
	Method string
	URL    string
	Header http.Header
	Body   string
}

// recordingTransport records every request the client sends — method, the
// whole URL with its query, every header, the body — then sends it on.
type recordingTransport struct {
	mu   *sync.Mutex
	seen *[]sentRequest
}

func (rt recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		if body, err = io.ReadAll(req.Body); err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	rt.mu.Lock()
	*rt.seen = append(*rt.seen, sentRequest{Method: req.Method, URL: req.URL.String(), Header: req.Header.Clone(), Body: string(body)})
	rt.mu.Unlock()
	return http.DefaultTransport.RoundTrip(req)
}

func recorder(seen *[]sentRequest) http.RoundTripper {
	return recordingTransport{mu: &sync.Mutex{}, seen: seen}
}

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("%q does not contain %q", got, want)
	}
}
