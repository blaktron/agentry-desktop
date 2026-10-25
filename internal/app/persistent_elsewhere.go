package app

// An agent another client runs: the supervisor `agentry agent start`
// launched from a terminal. The app watches each agent's owner lock and says
// when one starts or stops there, attaches to that supervisor's socket as a
// viewer so the page shows its chat and sends to it, and stops it through
// the socket. The lock stays the other client's throughout: a viewer never
// owns the agent, and leaving it leaves the agent running.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/session"
)

// elsewhereWatch is how often the owner locks are looked at.
const elsewhereWatch = 2 * time.Second

// elsewhereStopWait bounds a stop of an agent another client runs: its
// supervisor writes the segment's envelope before it answers.
const elsewhereStopWait = 3 * time.Minute

// viewer is the app's chat with an agent another client runs.
type viewer struct{ s session.Session }

// heldElsewhere reports whether another process holds the agent's owner
// lock, and who. The lock is probed only when a holder's record exists, so
// the watch never contends with a start; a hold by this process is a
// set-up of its own (a sign-in), not a run.
func (p *PersistentAPI) heldElsewhere(home, name string) (bool, *persistent.Holder) {
	h, err := persistent.ReadHolder(home, name)
	if err == nil && h == nil {
		return false, nil
	}
	if h != nil && h.PID == os.Getpid() {
		return false, nil
	}
	held := persistent.Held
	if p.held != nil {
		held = p.held
	}
	return held(home, name)
}

// holderWords names the client holding an agent, as its view reads it.
func holderWords(h *persistent.Holder) string {
	switch {
	case h == nil:
		return ""
	case h.Client == persistent.ClientCLI:
		return "a terminal"
	}
	return h.Where()
}

// watchElsewhere looks at the agents' owner locks until ctx ends, then
// leaves every viewer.
func (p *PersistentAPI) watchElsewhere(ctx context.Context) {
	t := time.NewTicker(elsewhereWatch)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			p.leaveViewers()
			return
		case <-t.C:
			p.checkElsewhere()
		}
	}
}

// checkElsewhere tells the page of each agent that started or stopped in
// another client since the last look, and keeps a viewer on each agent
// another client runs.
func (p *PersistentAPI) checkElsewhere() {
	home, err := p.home()
	if err != nil {
		return
	}
	for _, name := range shownAgents {
		p.mu.Lock()
		_, ours := p.sups[name]
		starting := p.starting[name] != nil
		p.mu.Unlock()
		now := false
		if !ours && !starting {
			now, _ = p.heldElsewhere(home, name)
		}
		p.mu.Lock()
		if p.elsewhere == nil {
			p.elsewhere = map[string]bool{}
		}
		was := p.elsewhere[name]
		p.elsewhere[name] = now
		p.mu.Unlock()
		if now {
			p.viewerOf(home, name)
		} else {
			p.leaveViewer(name)
		}
		if now != was {
			p.stateChanged(name)
		}
	}
}

// viewerOf is the app's chat with an agent another client runs, attached
// now if it is not yet; nil when its supervisor does not answer (another
// client's set-up holds the lock, or it is still starting).
func (p *PersistentAPI) viewerOf(home, name string) session.Session {
	p.mu.Lock()
	if v := p.viewers[name]; v != nil {
		p.mu.Unlock()
		return v.s
	}
	p.mu.Unlock()
	attach := persistent.Attach
	if p.attach != nil {
		attach = p.attach
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	s, err := attach(ctx, home, name, nil)
	cancel()
	if err != nil {
		return nil
	}
	v := &viewer{s: s}
	p.mu.Lock()
	if have := p.viewers[name]; have != nil {
		// Another call attached first: this one leaves.
		p.mu.Unlock()
		_, _ = s.End(context.Background(), false)
		return have.s
	}
	if p.viewers == nil {
		p.viewers = map[string]*viewer{}
	}
	p.viewers[name] = v
	p.mu.Unlock()
	p.followers.Add(1)
	go func() {
		defer p.followers.Done()
		for ev := range s.Events() {
			emitEvent(p.app, eventAgentEvent, PersistentEventView{Agent: name, Event: eventView(ev)})
			p.noticeTask(name, ev, nil)
		}
		p.mu.Lock()
		if p.viewers[name] == v {
			delete(p.viewers, name)
		}
		p.mu.Unlock()
	}()
	return s
}

// leaveViewer ends the app's chat with an agent another client runs; the
// agent keeps running.
func (p *PersistentAPI) leaveViewer(name string) {
	p.mu.Lock()
	v := p.viewers[name]
	delete(p.viewers, name)
	p.mu.Unlock()
	if v == nil {
		return
	}
	_, _ = v.s.End(context.Background(), false)
}

// leaveViewers leaves every viewer, as the app quits.
func (p *PersistentAPI) leaveViewers() {
	for _, name := range shownAgents {
		p.leaveViewer(name)
	}
}

// sendElsewhere sends the person's message to an agent another client runs.
func (p *PersistentAPI) sendElsewhere(home string, a persistent.Agent, text string) error {
	s := p.viewerOf(home, a.Name)
	if s == nil {
		return fmt.Errorf("the %s is running in another client and does not answer yet: try again in a moment", a.Title)
	}
	err := s.Send(context.Background(), text)
	if errors.Is(err, persistent.ErrBusy) {
		return fmt.Errorf("the %s is answering a message sent from another client: send yours when it has answered", a.Title)
	}
	return err
}

// stopElsewhere stops an agent another client runs, through its
// supervisor's socket, and waits for its segment's envelope. A supervisor
// that closes the socket without answering but lets the lock go stopped
// all the same.
func (p *PersistentAPI) stopElsewhere(home string, a persistent.Agent) error {
	stop := persistent.RequestStop
	if p.requestStop != nil {
		stop = p.requestStop
	}
	ctx, cancel := context.WithTimeout(context.Background(), elsewhereStopWait)
	defer cancel()
	_, err := stop(ctx, home, a.Name)
	p.leaveViewer(a.Name)
	defer p.checkElsewhere()
	if err != nil {
		if held, _ := p.heldElsewhere(home, a.Name); held {
			return fmt.Errorf("the %s did not stop: %w", a.Title, err)
		}
	}
	return nil
}
