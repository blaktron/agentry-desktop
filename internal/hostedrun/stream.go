package hostedrun

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The log, streamed (plan §4.5). The registry serves the run's durable
// event rows as server-sent events — `id: <seq>`, `event: <log|status|
// usage|error>`, `data: {"seq","at","kind","line"}` — ending with an event
// named `done`, or `timeout` after ten minutes of quiet. A client resumes
// from the last id it saw, so a dropped connection loses no line; the seq
// it remembers also means a re-delivered row is dropped rather than shown
// twice.

// FollowOptions parameterise Follow.
type FollowOptions struct {
	// After is the last seq the caller already has; 0 starts at the top.
	After int64
	// OnEvent receives every event once, in seq order.
	OnEvent func(Event)
	// OnStatus receives the run whenever the registry's copy of it has
	// moved — a claim, a start, a new queue position, the finish.
	OnStatus func(*Run)
	// Poll is how often the run itself is re-read while the stream is
	// quiet, which is what keeps a queue position current. Zero is three
	// seconds.
	Poll time.Duration
	// Retries bounds the consecutive reconnection attempts after a
	// dropped connection before Follow gives up. Zero is ten.
	Retries int
	// RetryDelay is the pause between those attempts. Zero is a second.
	RetryDelay time.Duration
}

// Follow streams one run's log until it reaches a terminal state, and
// returns the run as the registry last reported it. It reconnects from the
// last event id on a dropped connection and after the stream's own
// timeout, and calls the callbacks one at a time.
func (c *Client) Follow(ctx context.Context, id string, opt FollowOptions) (*Run, error) {
	if opt.Poll <= 0 {
		opt.Poll = 3 * time.Second
	}
	if opt.Retries <= 0 {
		opt.Retries = 10
	}
	if opt.RetryDelay <= 0 {
		opt.RetryDelay = time.Second
	}
	ctx, cancel := context.WithCancel(ctx)
	// The poll below ends with the context, so the cancel has to run
	// before the wait — these two defers are in that order on purpose.
	var wg sync.WaitGroup
	defer wg.Wait()
	defer cancel()

	var mu sync.Mutex // the callbacks are called from the poll and the stream
	last := opt.After
	emit := func(ev Event) {
		mu.Lock()
		defer mu.Unlock()
		if ev.Seq <= last {
			return // already delivered: a resume overlapped
		}
		last = ev.Seq
		if opt.OnEvent != nil {
			opt.OnEvent(ev)
		}
	}
	var lastStatus string
	var lastPosition int
	status := func(run *Run) {
		mu.Lock()
		defer mu.Unlock()
		if run == nil || (run.Status == lastStatus && run.QueuePosition == lastPosition) {
			return
		}
		lastStatus, lastPosition = run.Status, run.QueuePosition
		if opt.OnStatus != nil {
			opt.OnStatus(run)
		}
	}
	seen := func() int64 {
		mu.Lock()
		defer mu.Unlock()
		return last
	}

	// The poll keeps the queue position current while the stream is quiet
	// — a queued run writes no events at all.
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(opt.Poll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if run, err := c.Get(ctx, id); err == nil {
					status(run)
				}
			}
		}
	}()

	// Where the run stands before a line of log arrives. A run that is
	// already over is still streamed: its events route serves the whole
	// log and ends with `done`, which is what a panel reopened after the
	// fact shows.
	if run, err := c.Get(ctx, id); err == nil {
		status(run)
	}

	fails := 0
	for {
		done, timedOut, err := c.streamOnce(ctx, id, seen(), emit, func() {
			if run, gerr := c.Get(ctx, id); gerr == nil {
				status(run)
			}
		})
		switch {
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case done:
			run, err := c.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			status(run)
			return c.finish(ctx, id, run)
		case timedOut:
			// The stream's own ten-minute life ended: the registry asked
			// for a reconnect, so this is not a failure. A long queue sees
			// several of these.
			fails = 0
		default:
			// A dropped connection: reopen from the last id, losing
			// nothing, until the retries run out.
			fails++
			if fails > opt.Retries {
				return nil, err
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(opt.RetryDelay):
			}
		}
	}
}

// finish re-reads a run that reports itself terminal but has no outcome
// yet: the finish writes the outcome, the usage, the settlement and the
// outputs in one transaction, and a stream that ended on the terminal
// status row can reach the read a moment before it lands.
func (c *Client) finish(ctx context.Context, id string, run *Run) (*Run, error) {
	if run.Status != StatusFinished || run.Outcome != "" {
		return run, nil
	}
	for i := 0; i < 3; i++ {
		select {
		case <-ctx.Done():
			return run, nil
		case <-time.After(500 * time.Millisecond):
		}
		fresh, err := c.Get(ctx, id)
		if err != nil {
			return run, nil
		}
		run = fresh
		if run.Outcome != "" {
			return run, nil
		}
	}
	return run, nil
}

// streamOnce reads one connection's worth of events. done is true when the
// registry said the run is over; timedOut is true when it ended the stream
// on its own ten-minute life and expects a reconnect; an error means the
// connection ended for some other reason and the caller should reopen.
func (c *Client) streamOnce(ctx context.Context, id string, after int64, emit func(Event), moved func()) (done, timedOut bool, err error) {
	body, err := c.events(ctx, id, after)
	if err != nil {
		return false, false, err
	}
	defer func() { _ = body.Close() }()

	sc := bufio.NewScanner(body)
	// A log line is capped at 16 KB by the API; the frame around it and a
	// generous margin fit in 256 KB.
	sc.Buffer(make([]byte, 0, 64<<10), 256<<10)
	var name, data string
	var frameID int64
	// dispatch handles one whole frame and reports whether the stream is
	// over (`done`) or the registry asked for a reconnect (`timeout`).
	dispatch := func() (bool, bool) {
		evName, payload, seq := name, data, frameID
		name, data, frameID = "", "", 0
		switch evName {
		case "":
			return false, false
		case "done":
			return true, false
		case "timeout":
			return false, true
		}
		var ev Event
		if json.Unmarshal([]byte(payload), &ev) != nil {
			return false, false
		}
		if ev.Seq == 0 {
			ev.Seq = seq // the frame's id is the row's seq
		}
		if ev.Seq == 0 {
			return false, false
		}
		if ev.Kind == "" {
			ev.Kind = evName
		}
		emit(ev)
		if ev.Kind == "status" {
			moved()
		}
		return false, false
	}
	for sc.Scan() {
		line := strings.TrimSuffix(sc.Text(), "\r")
		switch {
		case line == "":
			if over, timeout := dispatch(); over || timeout {
				return over, timeout, nil
			}
		case strings.HasPrefix(line, ":"):
			// A comment (a keep-alive); nothing to do.
		case strings.HasPrefix(line, "event:"):
			name = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data != "" {
				data += "\n"
			}
			data += strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
		case strings.HasPrefix(line, "id:"):
			if n, perr := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "id:")), 10, 64); perr == nil {
				frameID = n
			}
		}
	}
	if err := sc.Err(); err != nil {
		return false, false, err
	}
	if over, timeout := dispatch(); over || timeout {
		return over, timeout, nil
	}
	// The connection ended without `done` or `timeout`: the registry
	// restarted, the edge closed it, or the network dropped. Reopening
	// from the last id is the remedy in every case.
	return false, false, io.ErrUnexpectedEOF
}
