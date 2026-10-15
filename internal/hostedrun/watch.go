package hostedrun

import (
	"context"
	"time"
)

// Where a run stands, read. A pushed run keeps no log: the registry
// writes status rows only, and what a run produced comes back as its
// outputs and its receipt when it is over. So the app does not stream
// anything; it re-reads the run until it stops moving, which also keeps a
// queue position current.

// WatchOptions parameterise Watch.
type WatchOptions struct {
	// OnStatus receives the run whenever the registry's copy of it has
	// moved: a new status, a new queue position, a cancellation asked
	// for, the finish.
	OnStatus func(*Run)
	// Poll is how often the run is re-read. Zero is three seconds.
	Poll time.Duration
	// Retries bounds the consecutive failed reads before Watch gives up.
	// Zero is ten.
	Retries int
}

// Watch re-reads one run until it reaches a terminal state, and returns
// the run as the registry last reported it. A failed read is retried at
// the next poll, so a registry that restarts mid-run loses the app
// nothing.
func (c *Client) Watch(ctx context.Context, id string, opt WatchOptions) (*Run, error) {
	if opt.Poll <= 0 {
		opt.Poll = 3 * time.Second
	}
	if opt.Retries <= 0 {
		opt.Retries = 10
	}
	var last *Run
	fails := 0
	for {
		run, err := c.Get(ctx, id)
		switch {
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case err != nil:
			fails++
			if fails > opt.Retries {
				return nil, err
			}
		default:
			fails = 0
			if moved(last, run) && opt.OnStatus != nil {
				opt.OnStatus(run)
			}
			last = run
			if run.Terminal() {
				return c.finish(ctx, id, run)
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(opt.Poll):
		}
	}
}

// moved reports whether a re-read says something the last one did not.
func moved(last, run *Run) bool {
	return last == nil || run.Status != last.Status || run.QueuePosition != last.QueuePosition ||
		run.CancelRequested != last.CancelRequested
}

// finish re-reads a run that reports itself terminal but has no outcome
// yet: the finish writes the outcome, the usage, the settlement and the
// outputs in one transaction, and a read can land a moment before it.
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
