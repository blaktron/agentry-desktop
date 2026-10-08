package main

// A run In Agentry whose release signs in to its MCP servers (OAuth
// connections M9, agentry-desktop#667; plan mcp-oauth-connections.md D5,
// D19). Agentry's vault keeps the connection, one per account, version and
// server, apart from the one this computer keeps for a local run; no
// refresh token is ever copied between them. A hosted consent starts only
// from a browser signed in to the site (DEV-222), so the app makes none:
// run create's 409 connection-missing names the servers still to connect,
// the app opens the site's Connect tools for the version it runs, and makes
// the same create again until the vault holds every connection, when the
// run starts. The app holds a token that cannot list, read or make a
// connection, and the vault returns no connection's token to anyone: the
// worker gives a run its access token at the hand-off, on the sandbox host.

import (
	"errors"
	"fmt"
	"time"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// connectPoll is how often a start waiting for its connections is made
// again, and connectWait how long it waits: a consent address lapses after
// ten minutes (DEV-222), so the wait outlasts one, with time to sign in to
// the site first. Tests shorten both.
var (
	connectPoll = 3 * time.Second
	connectWait = 15 * time.Minute
)

// awaitingStart is a start refused connection-missing: the create as it
// was made, its inputs already uploaded and any secrets it set held for it,
// and where its outputs go.
type awaitingStart struct {
	registry string
	ref      string
	req      hostedrun.CreateRequest
	outDir   string
	// stop is open while a wait on this start runs, and closed to end it;
	// the wait itself clears it as it returns, so a second wait cannot
	// begin while the first is still inside a create.
	stop    chan struct{}
	stopped bool
}

func (r *RunAPI) holdAwaiting(a *awaitingStart) {
	r.awaitMu.Lock()
	defer r.awaitMu.Unlock()
	r.stopLocked()
	r.awaiting = a
}

// dropAwaiting forgets the start held for its connections: a, or whatever
// is held when a is nil. A wait under way on it stops. It returns what it
// dropped.
func (r *RunAPI) dropAwaiting(a *awaitingStart) *awaitingStart {
	r.awaitMu.Lock()
	defer r.awaitMu.Unlock()
	if a != nil && r.awaiting != a {
		return nil
	}
	held := r.awaiting
	r.stopLocked()
	r.awaiting = nil
	return held
}

func (r *RunAPI) stopLocked() {
	if a := r.awaiting; a != nil && a.stop != nil && !a.stopped {
		close(a.stop)
		a.stopped = true
	}
}

func (r *RunAPI) holds(a *awaitingStart) bool {
	r.awaitMu.Lock()
	defer r.awaitMu.Unlock()
	return r.awaiting == a
}

// OpenHostedConnect opens, in the system browser, the site's Connect tools
// for ref's exact version on reg: where the person, signed in to the site
// as the account this app uses, connects the servers a run In Agentry
// needs. The caller names a registry and a release, never an address; the
// page is on the registry's own origin, held to Settings' URL rule.
func (r *RunAPI) OpenHostedConnect(reg, ref string) error {
	reg = settings.Normalize(reg)
	if err := settings.ValidateURL(reg); err != nil {
		return err
	}
	page, err := hostedrun.ConnectPage(reg, ref)
	if err != nil {
		return err
	}
	return r.openURL(page)
}

// AwaitHostedConnections makes again the start that was refused 409
// connection-missing for ref on reg, every few seconds, until the vault
// holds every connection it needs and the run is created, or the create is
// refused for another reason (a secret still to give, the credits), which
// it answers as a start does. A wait that runs out, or that
// StopAwaitingConnections ends, answers connection-missing again and keeps
// the start, so the next wait makes the same create with the same inputs.
// A start discarded or replaced while it waits answers nothing.
//
// The create is never cut off mid-request: an answer that never came back
// may have created the run, so a create that fails without the registry's
// refusal ends the wait with that error and forgets the start, as a start
// does; the app's attach finds a run that was created.
func (r *RunAPI) AwaitHostedConnections(reg, ref string) (*HostedStartView, error) {
	reg, token, err := storedCredential(reg)
	if err != nil {
		return nil, err
	}
	r.awaitMu.Lock()
	a := r.awaiting
	if a == nil || a.registry != reg || a.ref != ref {
		r.awaitMu.Unlock()
		return nil, fmt.Errorf("no start of %s is waiting for its connections; start the run again", ref)
	}
	if a.stop != nil {
		r.awaitMu.Unlock()
		return nil, errors.New("this start is already waiting for its connections")
	}
	stop := make(chan struct{})
	a.stop, a.stopped = stop, false
	r.awaitMu.Unlock()
	defer func() {
		r.awaitMu.Lock()
		a.stop = nil
		r.awaitMu.Unlock()
	}()

	client := hostedrun.New(reg, token)
	deadline := time.Now().Add(connectWait)
	for {
		if err := r.hostedIdle(); err != nil {
			return nil, err
		}
		created, err := client.Create(r.ctx(), a.req)
		if err == nil {
			r.dropAwaiting(a)
			r.forgetHeld(reg)
			return r.startedView(reg, token, a.outDir, created)
		}
		refusal := hostedrun.RefusalOf(err)
		if refusal == nil {
			// The run may exist and hold the secrets: they are left to it.
			r.dropAwaiting(a)
			r.forgetHeld(reg)
			return nil, err
		}
		if refusal.Slug != hostedrun.SlugConnectionMissing {
			r.dropAwaiting(a)
			r.DiscardHostedSecrets(reg)
			return r.refusalView(refusal), nil
		}
		still := r.refusalView(refusal)
		if !time.Now().Before(deadline) {
			still.Detail = fmt.Sprintf("Agentry still keeps no connection to %s for this version after %s. Connect %s on the page that opened in your browser, then wait again.",
				joinServers(refusal.Missing), connectWait.Round(time.Minute), itOrThem(refusal.Missing))
			return still, nil
		}
		select {
		case <-stop:
			if !r.holds(a) {
				return &HostedStartView{}, nil
			}
			still.Detail = "Stopped waiting. Agentry keeps no connection to " + joinServers(refusal.Missing) + " for this version yet."
			return still, nil
		case <-r.ctx().Done():
			return nil, r.ctx().Err()
		case <-time.After(connectPoll):
		}
	}
}

// StopAwaitingConnections ends a wait under way, which answers
// connection-missing; the start is kept for the next wait.
func (r *RunAPI) StopAwaitingConnections() {
	r.awaitMu.Lock()
	defer r.awaitMu.Unlock()
	r.stopLocked()
}

// DiscardHostedConnect forgets the start held for its connections, and the
// secrets this app set for it and did not keep: the panel closed, the
// person cancelled, or changed where or how it runs. Its uploaded inputs
// stay unclaimed until the registry sweeps them.
func (r *RunAPI) DiscardHostedConnect() {
	if a := r.dropAwaiting(nil); a != nil {
		r.DiscardHostedSecrets(a.registry)
	}
}

func joinServers(servers []string) string {
	switch len(servers) {
	case 0:
		return "a server it signs in to"
	case 1:
		return servers[0]
	}
	out := servers[0]
	for i, s := range servers[1:] {
		if i == len(servers)-2 {
			out += " and " + s
		} else {
			out += ", " + s
		}
	}
	return out
}

func itOrThem(servers []string) string {
	if len(servers) > 1 {
		return "them"
	}
	return "it"
}
