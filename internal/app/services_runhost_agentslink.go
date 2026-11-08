package app

// A runhost's agents reach this computer. While the app runs it holds a
// link session (`link agents`) open to each paired runhost, over that
// runhost's run key: the runhost's persistent agents bind their segments
// through it and their calls to an application here come back on it,
// answered by this app's own link service with the agent's name in the
// dialogs. The runhost opens nothing towards this computer. With the app
// closed, or the runhost unreachable, an agent's calls are refused there as
// the person's computer away, and the agent goes on.

import (
	"context"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	"github.com/blaktron/agentry-cli/pkg/runhost/remote"
	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
)

// agentsLinkRetry is how long the app waits before linking a runhost's
// agents again after a session ended or could not open.
var agentsLinkRetry = 30 * time.Second

// agentsLinks are the runhosts whose agents are linked to this computer
// now, by name, and the keepers running.
type agentsLinks struct {
	mu      sync.Mutex
	linked  map[string]bool
	keeping map[string]context.CancelFunc
}

// startAgentsLinks keeps a link session to every paired runhost while ctx
// lives, and to each one paired later at its next look (every minute).
func (r *RunhostsAPI) startAgentsLinks(ctx context.Context) {
	r.links.mu.Lock()
	if r.links.linked == nil {
		r.links.linked, r.links.keeping = map[string]bool{}, map[string]context.CancelFunc{}
	}
	r.links.mu.Unlock()
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			r.keepAgentsLinks(ctx)
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// keepAgentsLinks starts a keeper for each paired runhost without one, and
// ends the keeper of one no longer paired.
func (r *RunhostsAPI) keepAgentsLinks(ctx context.Context) {
	home, err := r.home()
	if err != nil {
		return
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		return
	}
	paired := map[string]runhost.Record{}
	for _, rec := range recs.Runhosts {
		paired[rec.Name] = rec
	}
	r.links.mu.Lock()
	defer r.links.mu.Unlock()
	for name, stop := range r.links.keeping {
		if _, ok := paired[name]; !ok {
			stop()
			delete(r.links.keeping, name)
			delete(r.links.linked, name)
		}
	}
	for name, rec := range paired {
		if _, ok := r.links.keeping[name]; ok {
			continue
		}
		kctx, stop := context.WithCancel(ctx)
		r.links.keeping[name] = stop
		go r.keepAgentsLink(kctx, rec)
	}
}

// keepAgentsLink holds runhost rec's agents link while ctx lives, opening
// it again after it ends.
func (r *RunhostsAPI) keepAgentsLink(ctx context.Context, rec runhost.Record) {
	bind := func(ctx context.Context, req desktop.BindRequest) (*engine.DesktopLink, error) {
		req.Agent += " on " + rec.Name
		return r.run.bindDesktop(ctx, req)
	}
	for ctx.Err() == nil {
		keys, err := runhost.LoadKeys(rec.Name)
		if err == nil {
			forward, closeAll := remote.AgentsForward(bind)
			_ = remote.KeepLink(ctx, remote.LinkDialerFrom(nil, &rec, keys), remote.AgentsLinkID, forward, func() {
				r.setAgentsLinked(rec.Name, true)
			})
			closeAll()
		}
		r.setAgentsLinked(rec.Name, false)
		select {
		case <-ctx.Done():
			return
		case <-time.After(agentsLinkRetry):
		}
	}
}

func (r *RunhostsAPI) setAgentsLinked(name string, on bool) {
	r.links.mu.Lock()
	if r.links.linked != nil {
		r.links.linked[name] = on
	}
	r.links.mu.Unlock()
}

// agentsLinked says runhost name's agents are linked to this computer now.
func (r *RunhostsAPI) agentsLinked(name string) bool {
	r.links.mu.Lock()
	defer r.links.mu.Unlock()
	return r.links.linked[name]
}
