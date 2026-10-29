package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/session"
)

// Stop on a turn stops the turn the running agent is on, and does nothing
// when it is on none; a turn the person stopped is no error of their Send.
func TestAStoppedTurnIsStoppedAndNoErrorOfItsSend(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	if err := rig.p.Interrupt("assistant"); err != nil || rig.agent.interrupts != 1 {
		t.Fatalf("Interrupt = %v after %d interrupts", err, rig.agent.interrupts)
	}
	rig.agent.interruptErr = session.ErrNoTurn
	if err := rig.p.Interrupt("assistant"); err != nil {
		t.Fatalf("Interrupt with no turn = %v", err)
	}
	rig.agent.interruptErr = errors.New("the harness did not stop the turn")
	if err := rig.p.Interrupt("assistant"); err == nil || !strings.Contains(err.Error(), "did not stop its turn") {
		t.Fatalf("Interrupt that failed = %v", err)
	}
	rig.agent.sendErr = session.ErrInterrupted
	if err := rig.p.Send("assistant", "Write me an essay"); err != nil {
		t.Fatalf("Send of a stopped turn = %v", err)
	}
	rig.agent.sendErr = errors.New("the harness failed the turn")
	if err := rig.p.Send("assistant", "Again"); err == nil {
		t.Fatal("a failed turn's Send said nothing")
	}
}

// An agent another client runs is asked to stop its turn through its
// supervisor's socket; one that is not running is told so.
func TestAStopOfATurnElsewhereGoesThroughTheSocket(t *testing.T) {
	r := newElsewhereRig(t)
	if err := r.p.Interrupt("assistant"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("Interrupt of a stopped agent = %v", err)
	}
	r.hold(t, r.home, true)
	asked := 0
	r.p.requestInterrupt = func(_ context.Context, home, name string) (bool, error) {
		if home != r.home || name != "assistant" {
			t.Errorf("asked %s in %s", name, home)
		}
		asked++
		return true, nil
	}
	if err := r.p.Interrupt("assistant"); err != nil || asked != 1 {
		t.Fatalf("Interrupt elsewhere = %v after %d asks", err, asked)
	}
	r.p.requestInterrupt = func(context.Context, string, string) (bool, error) {
		return false, errors.New("the harness did not stop the turn")
	}
	if err := r.p.Interrupt("assistant"); err == nil || !strings.Contains(err.Error(), "did not stop its turn") {
		t.Fatalf("Interrupt elsewhere that failed = %v", err)
	}
}
