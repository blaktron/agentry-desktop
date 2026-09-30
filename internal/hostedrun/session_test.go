package hostedrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSession is the registry's four session routes for one run, as
// agentry-enterprise internal/runs/session_routes.go answers them: each
// stream request is answered by the next scripted body, and every post is
// recorded whole.
type fakeSession struct {
	mu sync.Mutex
	// streams are the successive answers to GET {id}/session: a status and
	// the server-sent events written, verbatim.
	streams []fakeStream
	// afters are the `after` each stream request carried; lastEventIDs its
	// Last-Event-ID header, which this client never sends.
	afters       []string
	lastEventIDs []string
	accepts      []string
	// posts are the posted bodies by route (turns, files, end).
	posts map[string][]string
	// refuse, when set, answers a post instead of 202.
	refuse func(w http.ResponseWriter)
}

type fakeStream struct {
	status int
	body   string
}

func (s *fakeSession) start(t *testing.T) string {
	t.Helper()
	s.posts = map[string][]string{}
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			problemJSON(w, http.StatusUnauthorized, "unauthorized", "Sign in required", "", nil)
			return false
		}
		if r.PathValue("id") != sessionRun {
			problemJSON(w, http.StatusNotFound, "not-found", "Not found", "", nil)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/v1/x/runs/{id}/session", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		s.mu.Lock()
		s.afters = append(s.afters, r.URL.Query().Get("after"))
		s.lastEventIDs = append(s.lastEventIDs, r.Header.Get("Last-Event-ID"))
		s.accepts = append(s.accepts, r.Header.Get("Accept"))
		var next fakeStream
		if len(s.streams) > 0 {
			next, s.streams = s.streams[0], s.streams[1:]
		} else {
			next = fakeStream{status: http.StatusBadGateway}
		}
		s.mu.Unlock()
		if next.status != http.StatusOK {
			problemJSON(w, next.status, "not-found", "Not found", "", nil)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, next.body)
	})
	for _, route := range []string{"turns", "files", "end"} {
		mux.HandleFunc("POST /api/v1/x/runs/{id}/session/"+route, func(w http.ResponseWriter, r *http.Request) {
			if !auth(w, r) {
				return
			}
			b, _ := io.ReadAll(r.Body)
			s.mu.Lock()
			s.posts[route] = append(s.posts[route], string(b))
			refuse := s.refuse
			s.mu.Unlock()
			if refuse != nil {
				refuse(w)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

const sessionRun = "22222222-2222-2222-2222-222222222222"

// sse writes one event the way the registry does.
func sse(event string, id int, data string) string {
	if id > 0 {
		return fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", id, event, data)
	}
	return fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
}

// seen is what the handlers were given, in order.
type seen struct {
	mu     sync.Mutex
	log    []string
	frames []FrameEvent
	states []SessionState
}

func (s *seen) handlers() SessionHandlers {
	return SessionHandlers{
		State: func(st SessionState) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.log = append(s.log, "state "+st.Status)
			s.states = append(s.states, st)
		},
		Frame: func(seq uint64, ev FrameEvent) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.log = append(s.log, fmt.Sprintf("frame %d %s", seq, ev.Kind))
			s.frames = append(s.frames, ev)
		},
		Dropped: func(uint64) {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.log = append(s.log, "dropped")
		},
	}
}

// A session followed from its first event to its end: the state and each
// frame in order, the registry's 30-minute `timeout` answered by a new
// stream from the last frame's sequence number (plan D8), a frame the
// client has already been given taken once only, the host's `dropped`
// said, and `done` the end of it. The comments that keep a quiet stream
// open are nothing.
func TestFollowSessionResumesAfterTimeout(t *testing.T) {
	running := `{"status":"running","startedAt":"2026-09-29T10:00:00Z","session":{"freeMinutes":10,"creditsPerMinute":50,"paidThrough":10,"timeCredits":0,"tokenCredits":0,"idleSeconds":300}}`
	f := &fakeSession{streams: []fakeStream{
		{status: http.StatusOK, body: sse("state", 0, running) +
			sse("frame", 1, `{"kind":"user_turn","time":"2026-09-29T10:00:01Z","text":"check this csv"}`) +
			": keepalive\n\n" +
			sse("frame", 2, `{"kind":"assistant_text","time":"2026-09-29T10:00:02Z","text":"Look"}`) +
			sse("timeout", 0, "{}")},
		{status: http.StatusOK, body: sse("state", 0, running) +
			sse("frame", 2, `{"kind":"assistant_text","time":"2026-09-29T10:00:02Z","text":"Look"}`) +
			sse("frame", 3, `{"kind":"assistant_turn","time":"2026-09-29T10:00:03Z","text":"Looking at it."}`) +
			sse("dropped", 0, `{"before":7}`) +
			sse("frame", 7, `{"kind":"session_end","time":"2026-09-29T10:05:00Z","reason":"finished"}`) +
			sse("state", 0, `{"status":"finished","startedAt":"2026-09-29T10:00:00Z","outcome":"succeeded","session":{"freeMinutes":10,"creditsPerMinute":50,"paidThrough":10,"timeCredits":0,"tokenCredits":3,"idleSeconds":300}}`) +
			sse("done", 0, "{}")},
	}}
	c := New(f.start(t), testToken)
	var got seen
	if err := c.FollowSession(t.Context(), sessionRun, FollowSessionOptions{Handlers: got.handlers(), Retry: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"state running", "frame 1 user_turn", "frame 2 assistant_text",
		"state running", "frame 3 assistant_turn", "dropped", "frame 7 session_end", "state finished",
	}
	if strings.Join(got.log, "\n") != strings.Join(want, "\n") {
		t.Fatalf("handlers saw\n%s\nwant\n%s", strings.Join(got.log, "\n"), strings.Join(want, "\n"))
	}
	if strings.Join(f.afters, ",") != "0,2" {
		t.Fatalf("streams asked for after %q, want 0 then 2", f.afters)
	}
	for i := range f.accepts {
		if f.accepts[i] != "text/event-stream" || f.lastEventIDs[i] != "" {
			t.Fatalf("stream %d: Accept %q, Last-Event-ID %q", i, f.accepts[i], f.lastEventIDs[i])
		}
	}
	if got.frames[0].Text != "check this csv" || got.frames[3].Reason != "finished" {
		t.Fatalf("frames = %+v", got.frames)
	}
	last := got.states[len(got.states)-1]
	if last.Session == nil || last.Session.TokenCredits != 3 || last.StartedAt == nil || last.Outcome != "succeeded" {
		t.Fatalf("final state = %+v (session %+v)", last, last.Session)
	}
}

// A stream the registry ends with `error` cannot be shown here — the
// runner's key did not open, or a frame did not open as the session's next
// — and is not asked for again: the session goes on, its transcript in its
// output. A 404 is a run that is not this account's session, final too.
func TestFollowSessionStopsOnTheRegistrysError(t *testing.T) {
	f := &fakeSession{streams: []fakeStream{{status: http.StatusOK, body: sse("error", 0, `{"class":"key"}`)}}}
	c := New(f.start(t), testToken)
	var got seen
	err := c.FollowSession(t.Context(), sessionRun, FollowSessionOptions{Handlers: got.handlers(), Retry: time.Millisecond})
	var se *SessionStreamError
	if !errors.As(err, &se) || se.Class != "key" {
		t.Fatalf("err = %v, want the stream's error class key", err)
	}
	if len(f.afters) != 1 {
		t.Fatalf("asked %d times after an error event, want once", len(f.afters))
	}

	g := &fakeSession{streams: []fakeStream{{status: http.StatusNotFound}}}
	c = New(g.start(t), testToken)
	err = c.FollowSession(t.Context(), sessionRun, FollowSessionOptions{Handlers: got.handlers(), Retry: time.Millisecond})
	if r := RefusalOf(err); r == nil || r.Status != http.StatusNotFound || len(g.afters) != 1 {
		t.Fatalf("err = %v after %d asks, want the 404 refusal at once", err, len(g.afters))
	}
}

// A registry that keeps failing (502, a stream that closes with no end) is
// asked again from the last frame, a bounded number of times in a row;
// every stream that delivered something starts the count again.
func TestFollowSessionRetriesABoundedNumberOfTimes(t *testing.T) {
	f := &fakeSession{streams: []fakeStream{
		{status: http.StatusBadGateway},
		{status: http.StatusOK, body: sse("frame", 1, `{"kind":"user_turn","text":"hi"}`)},
		{status: http.StatusBadGateway},
		{status: http.StatusBadGateway},
		{status: http.StatusBadGateway},
	}}
	c := New(f.start(t), testToken)
	var got seen
	err := c.FollowSession(t.Context(), sessionRun, FollowSessionOptions{Handlers: got.handlers(), Retry: time.Millisecond, Retries: 2})
	if err == nil {
		t.Fatal("a registry that never recovers was followed forever")
	}
	if strings.Join(f.afters, ",") != "0,0,1,1,1" {
		t.Fatalf("afters = %v, want 0,0 then 1 three times", f.afters)
	}
	if len(got.frames) != 1 {
		t.Fatalf("frames = %+v", got.frames)
	}
}

// A turn, a file and the end go to their own routes as the registry reads
// them, and its refusals come back with their slugs.
func TestSessionTurnFileAndEnd(t *testing.T) {
	f := &fakeSession{}
	c := New(f.start(t), testToken)
	ctx := t.Context()
	if err := c.SessionTurn(ctx, sessionRun, "and the second column?"); err != nil {
		t.Fatal(err)
	}
	if err := c.SessionFile(ctx, sessionRun, "33333333-3333-3333-3333-333333333333"); err != nil {
		t.Fatal(err)
	}
	if err := c.SessionEnd(ctx, sessionRun, true); err != nil {
		t.Fatal(err)
	}
	if err := c.SessionEnd(ctx, sessionRun, false); err != nil {
		t.Fatal(err)
	}
	decode := func(s string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		return m
	}
	if m := decode(f.posts["turns"][0]); len(m) != 1 || m["text"] != "and the second column?" {
		t.Fatalf("turn body = %s", f.posts["turns"][0])
	}
	if m := decode(f.posts["files"][0]); len(m) != 1 || m["input"] != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("file body = %s", f.posts["files"][0])
	}
	if m := decode(f.posts["end"][0]); len(m) != 1 || m["finish"] != true {
		t.Fatalf("finish body = %s", f.posts["end"][0])
	}
	if m := decode(f.posts["end"][1]); len(m) != 1 || m["finish"] != false {
		t.Fatalf("stop body = %s", f.posts["end"][1])
	}

	f.refuse = func(w http.ResponseWriter) {
		problemJSON(w, http.StatusConflict, "session-busy", "A turn is in flight", "The agent is still answering. Send this once its reply has come.", nil)
	}
	err := c.SessionTurn(ctx, sessionRun, "again")
	if r := RefusalOf(err); r == nil || r.Slug != "session-busy" || !strings.Contains(r.Sentence(), "still answering") {
		t.Fatalf("err = %v, want the session-busy refusal", err)
	}
}

// A registry that answers `timeout` at once, or never ends an event, is not
// followed in a tight loop or into unbounded memory: the early timeout is a
// failure with its pause, the endless event is refused.
func TestFollowSessionBoundsAMisbehavingRegistry(t *testing.T) {
	early := make([]fakeStream, 4)
	for i := range early {
		early[i] = fakeStream{status: http.StatusOK, body: sse("timeout", 0, "{}")}
	}
	f := &fakeSession{streams: early}
	c := New(f.start(t), testToken)
	start := time.Now()
	err := c.FollowSession(t.Context(), sessionRun, FollowSessionOptions{Retry: 20 * time.Millisecond, Retries: 2})
	if err == nil || len(f.afters) != 3 {
		t.Fatalf("err = %v after %d streams, want a failure after 3", err, len(f.afters))
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("early timeouts were asked for again without the pause")
	}

	endless := "event: frame\nid: 1\n" + strings.Repeat("data: "+strings.Repeat("x", 60000)+"\n", 20)
	g := &fakeSession{streams: []fakeStream{{status: http.StatusOK, body: endless}}}
	c = New(g.start(t), testToken)
	var got seen
	err = c.FollowSession(t.Context(), sessionRun, FollowSessionOptions{Handlers: got.handlers(), Retry: time.Millisecond, Retries: 1})
	var se *SessionStreamError
	if !errors.As(err, &se) || se.Class != "size" || len(got.frames) != 0 || len(g.afters) != 1 {
		t.Fatalf("an endless event: err %v, frames %d", err, len(got.frames))
	}
}
