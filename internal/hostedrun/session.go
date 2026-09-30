package hostedrun

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// A hosted session (plan hosted-sessions.md §5, §6; agentry-enterprise#276;
// agentry-desktop#403): a run of a release's interactive mode on a sandbox
// host, which its runner talks to while it runs. The registry relays the
// session's display events as server-sent events on GET {id}/session,
// opening each frame with the runner's key inside the runner's own request;
// the runner's turns, files and end are posts beside it. Nothing here keeps
// a frame: each is handed to the caller as it arrives, and a reconnect is
// replayed by the host from its memory, not from this app's (D8).

// ModeInteractive is the run mode of a hosted session.
const ModeInteractive = "interactive"

// SessionTerms is the status route's sessions line: whether this
// deployment runs hosted sessions, what one costs, and the idle close.
type SessionTerms struct {
	Enabled          bool  `json:"enabled"`
	FreeMinutes      int   `json:"freeMinutes"`
	CreditsPerMinute int64 `json:"creditsPerMinute"`
	MinCredits       int64 `json:"minCredits"`
	IdleSeconds      int   `json:"idleSeconds"`
	// Allowed is, per tier, whether a payer on it may start one.
	Allowed map[string]bool `json:"allowed,omitempty"`
}

// CallerSession is the signed-in caller's session line: whether their
// payer's tier may start one (agentry-enterprise#286).
type CallerSession struct {
	Allowed bool `json:"allowed"`
}

// Session is a hosted session's content-free record, as the registry keeps
// it on the run: the terms it was created on, the minutes settled, what its
// time and its tokens cost, and the idle close its host holds it to.
type Session struct {
	FreeMinutes      int   `json:"freeMinutes"`
	CreditsPerMinute int64 `json:"creditsPerMinute"`
	// PaidThrough is the last minute settled, counting the free ones.
	PaidThrough  int   `json:"paidThrough"`
	TimeCredits  int64 `json:"timeCredits"`
	TokenCredits int64 `json:"tokenCredits"`
	TokensUnpaid int64 `json:"tokensUnpaid,omitempty"`
	// OutOfCredits says a minute the balance could not cover stopped it.
	OutOfCredits bool  `json:"outOfCredits,omitempty"`
	FilesAdded   int   `json:"filesAdded,omitempty"`
	BytesAdded   int64 `json:"bytesAdded,omitempty"`
	IdleSeconds  int   `json:"idleSeconds"`
}

// FrameEvent is one display event of a session, as the registry writes it
// on the stream's `frame` events (agentry-enterprise hostapi.FrameEvent):
// the CLI's session event filtered to what a chat shows. An input_added
// names the file by its basename, and an error by a fixed class, never by
// text the harness wrote.
type FrameEvent struct {
	Kind         string    `json:"kind"`
	Time         time.Time `json:"time"`
	Text         string    `json:"text,omitempty"`
	Synthetic    bool      `json:"synthetic,omitempty"`
	StopReason   string    `json:"stopReason,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Class        string    `json:"class,omitempty"`
	InputTokens  int64     `json:"inputTokens,omitempty"`
	OutputTokens int64     `json:"outputTokens,omitempty"`
	TotalTokens  int64     `json:"totalTokens,omitempty"`
}

// SessionState is the stream's `state` event: the run's content-free state
// and its session record, whenever either changes.
type SessionState struct {
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Outcome    string     `json:"outcome,omitempty"`
	Error      string     `json:"error,omitempty"`
	Session    *Session   `json:"session,omitempty"`
}

// SessionHandlers receive a stream's events as they arrive. Any may be nil.
type SessionHandlers struct {
	State func(SessionState)
	Frame func(seq uint64, ev FrameEvent)
	// Dropped says the host let go of the frames before `before`: they are
	// only in the transcript.
	Dropped func(before uint64)
}

// SessionStreamError is the registry's `error` event: this session's
// frames cannot be shown (Class "key": the runner's key did not open;
// "frame": a frame did not open as the session's next), or an event came
// past the size a frame can have (Class "size", this client's own). The
// session goes on; its transcript comes back in its output.
type SessionStreamError struct {
	Class string
}

func (e *SessionStreamError) Error() string {
	return "the registry cannot show this session's conversation (" + e.Class + ")"
}

// streamEnd is how one stream stopped.
type streamEnd int

const (
	// streamClosed is a connection that closed with no end event.
	streamClosed streamEnd = iota
	// streamDone follows the session's end: nothing more will come.
	streamDone
	// streamTimeout is the registry's 30-minute bound on one stream: the
	// client asks again from its last frame.
	streamTimeout
)

// streamLifeFloor is how long a stream must have lasted for its `timeout`
// to be the registry's bound (30 minutes) and answered at once.
const streamLifeFloor = time.Minute

// maxEventBytes bounds one server-sent event's line: a frame carries at
// most a 16 KiB turn, re-encoded as JSON.
const maxEventBytes = 1 << 20

// sessionStream reads one GET {id}/session from after, handing each event
// to h, and answers the last frame's sequence number, how the stream
// stopped, and why when it failed. A frame at or before after is one the
// caller was already given and is skipped.
func (c *Client) sessionStream(ctx context.Context, id string, after uint64, h SessionHandlers) (uint64, streamEnd, bool, error) {
	req, err := c.request(ctx, http.MethodGet, c.path(id, "session")+"?after="+strconv.FormatUint(after, 10), nil, "")
	if err != nil {
		return after, streamClosed, false, err
	}
	req.Header.Set("Accept", "text/event-stream")
	res, err := c.Download.Do(req)
	if err != nil {
		return after, streamClosed, false, fmt.Errorf("follow the session: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
		return after, streamClosed, false, refusalFrom("follow the session", res.StatusCode, body)
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 0, 64<<10), maxEventBytes)
	var event, data, eventID string
	got := false
	for sc.Scan() {
		line := sc.Text()
		if line != "" {
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				event = value
			case "data":
				// One event is bounded as one line is: a registry that
				// never ends an event cannot grow this without limit.
				if len(data)+len(value)+1 > maxEventBytes {
					return after, streamClosed, got, &SessionStreamError{Class: "size"}
				}
				if data != "" {
					data += "\n"
				}
				data += value
			case "id":
				eventID = value
			}
			// A line starting with a colon is a comment: the heartbeat.
			continue
		}
		ev, body, seqText := event, data, eventID
		event, data, eventID = "", "", ""
		switch ev {
		case "state":
			var st SessionState
			if json.Unmarshal([]byte(body), &st) == nil && h.State != nil {
				got = true
				h.State(st)
			}
		case "frame":
			seq, err := strconv.ParseUint(seqText, 10, 64)
			if err != nil || seq <= after {
				continue
			}
			var f FrameEvent
			if json.Unmarshal([]byte(body), &f) != nil {
				continue
			}
			after, got = seq, true
			if h.Frame != nil {
				h.Frame(seq, f)
			}
		case "dropped":
			var d struct {
				Before uint64 `json:"before"`
			}
			_ = json.Unmarshal([]byte(body), &d)
			got = true
			if h.Dropped != nil {
				h.Dropped(d.Before)
			}
		case "done":
			return after, streamDone, true, nil
		case "timeout":
			return after, streamTimeout, got, nil
		case "error":
			var e struct {
				Class string `json:"class"`
			}
			_ = json.Unmarshal([]byte(body), &e)
			return after, streamClosed, got, &SessionStreamError{Class: e.Class}
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return after, streamClosed, got, fmt.Errorf("follow the session: %w", err)
	}
	return after, streamClosed, got, nil
}

// FollowSessionOptions parameterise FollowSession.
type FollowSessionOptions struct {
	// After is the last frame the caller has; 0 asks for every frame the
	// host still holds.
	After    uint64
	Handlers SessionHandlers
	// Retry is the pause before asking again after a failed stream; zero
	// is five seconds. Retries bounds the failures in a row before
	// FollowSession gives up; zero is twelve.
	Retry   time.Duration
	Retries int
}

// FollowSession follows a session until it ends: each stream from the last
// frame given, again at once after the registry's timeout, and after a
// pause when a stream failed, up to Retries failures in a row. It returns
// nil after `done`; the registry's `error` event (a *SessionStreamError)
// and a refusal (a *Refusal: 404 is a run that is not this account's
// session) end it at once, since asking again would get the same.
func (c *Client) FollowSession(ctx context.Context, id string, opt FollowSessionOptions) error {
	if opt.Retry <= 0 {
		opt.Retry = 5 * time.Second
	}
	if opt.Retries <= 0 {
		opt.Retries = 12
	}
	after, fails := opt.After, 0
	for {
		began := time.Now()
		last, end, got, err := c.sessionStream(ctx, id, after, opt.Handlers)
		after = last
		var se *SessionStreamError
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.As(err, &se):
			return err
		case err != nil && RefusalOf(err) != nil && RefusalOf(err).Status < 500:
			return err
		case end == streamDone:
			return nil
		case end == streamTimeout && time.Since(began) >= streamLifeFloor:
			// The registry's 30-minute bound: ask again at once.
			fails = 0
			continue
		}
		// Anything else is asked for again after the pause — a `timeout`
		// that came early included, so a registry answering it at once is
		// never asked in a tight loop.
		// A stream that delivered something and then closed starts the
		// count again; one that delivered nothing is a failure.
		if got {
			fails = 0
		} else {
			fails++
		}
		if fails > opt.Retries {
			if err == nil {
				err = errors.New("follow the session: the registry kept closing the stream")
			}
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(opt.Retry):
		}
	}
}

// SessionTurn sends the agent one message. The registry answers 202 once
// the host has it; 409 session-busy while the agent is still answering,
// session-starting before the host runs it, session-ended after.
func (c *Client) SessionTurn(ctx context.Context, id, text string) error {
	return c.sessionPost(ctx, id, "turns", "send the message", map[string]any{"text": text})
}

// SessionFile adds one of the account's uploads (UploadInput's id) to the
// running session, under the run's input caps.
func (c *Client) SessionFile(ctx context.Context, id, input string) error {
	return c.sessionPost(ctx, id, "files", "add the file", map[string]any{"input": input})
}

// SessionEnd ends the session: finish asks the agent for its final result
// first; without it the session stops now, with none.
func (c *Client) SessionEnd(ctx context.Context, id string, finish bool) error {
	op := "stop the session"
	if finish {
		op = "finish the session"
	}
	return c.sessionPost(ctx, id, "end", op, map[string]any{"finish": finish})
}

func (c *Client) sessionPost(ctx context.Context, id, route, op string, body map[string]any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := c.request(ctx, http.MethodPost, c.path(id, "session", route), strings.NewReader(string(b)), "application/json")
	if err != nil {
		return err
	}
	return c.do(op, req, http.StatusAccepted, nil)
}

// ErrorWords says an error frame's class, a fixed vocabulary, in words: the
// frame never carries text the harness wrote (plan §5).
func ErrorWords(class string) string {
	switch class {
	case "harness":
		return "the harness reported an error"
	case "malformed":
		return "the agent's runner could not read a command"
	case "unknown_command":
		return "the agent's runner did not know a command"
	case "busy":
		return "a message arrived while the agent was answering"
	case "input_refused":
		return "the file was not taken"
	default:
		return "an error this app does not know"
	}
}
