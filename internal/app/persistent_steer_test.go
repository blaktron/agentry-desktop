package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/session"
)

// A message during the person's turn goes to the running agent's turn; with
// no turn in flight the page is told to send it as a turn, and on the
// agent's own turn or a harness that takes a message only between turns the
// page keeps it.
func TestASteerGoesToTheRunningAgentsTurn(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.Steer("assistant", "and the second file"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("Steer of a stopped agent = %v", err)
	}
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	if got, err := rig.p.Steer("assistant", "and the second file"); err != nil || got != SteerSent {
		t.Fatalf("Steer = %q, %v", got, err)
	}
	if len(rig.agent.steered) != 1 || rig.agent.steered[0] != "and the second file" {
		t.Fatalf("the agent took %q", rig.agent.steered)
	}
	if _, err := rig.p.Steer("assistant", "  "); err == nil {
		t.Fatal("an empty message was steered")
	}
	for _, c := range []struct {
		err  error
		want string
	}{
		{session.ErrNoTurn, SteerNoTurn},
		{fmt.Errorf("%w: the turn in flight is the agent's own; send once it ends", session.ErrCannotSteer), SteerBetweenTurns},
		{fmt.Errorf("%w: this agent's harness takes a message only between turns", session.ErrCannotSteer), SteerBetweenTurns},
	} {
		rig.agent.steerErr = c.err
		if got, err := rig.p.Steer("assistant", "later"); err != nil || got != c.want {
			t.Errorf("Steer answering %v = %q, %v; want %q", c.err, got, err, c.want)
		}
	}
	rig.agent.steerErr = errors.New("the cap on messages is reached")
	if got, err := rig.p.Steer("assistant", "one more"); err == nil || got != "" {
		t.Fatalf("a refused steer = %q, %v", got, err)
	}
}

// An agent another client runs takes the message through its supervisor's
// socket, with the same answers.
func TestASteerElsewhereGoesThroughTheSocket(t *testing.T) {
	r := newElsewhereRig(t)
	r.hold(t, r.home, true)
	if got, err := r.p.Steer("assistant", "and the second file"); err != nil || got != SteerSent {
		t.Fatalf("Steer elsewhere = %q, %v", got, err)
	}
	r.mu.Lock()
	remote := r.remote
	r.mu.Unlock()
	if remote == nil || len(remote.steered) != 1 || remote.steered[0] != "and the second file" {
		t.Fatalf("the socket took %+v", remote)
	}
	remote.mu.Lock()
	remote.steerErr = session.ErrNoTurn
	remote.mu.Unlock()
	if got, err := r.p.Steer("assistant", "later"); err != nil || got != SteerNoTurn {
		t.Fatalf("Steer elsewhere with no turn = %q, %v", got, err)
	}
}
