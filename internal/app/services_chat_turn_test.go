package app

// A QuickAgent chat on this computer: a message sent while the agent is
// answering goes into the turn where the harness takes one, and Stop ends
// the turn while the chat goes on.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/session"
	"github.com/blaktron/agentry-desktop/internal/run"
)

// turnChat is a chat whose Send, Steer and Interrupt answer what a test
// gives them.
type turnChat struct {
	sendErr, steerErr, interruptErr error
	steered                         []string
	interrupts                      int
}

func (c *turnChat) Ref() string                  { return "acme/agent@1.0.0" }
func (c *turnChat) Events() <-chan session.Event { return nil }
func (c *turnChat) Send(context.Context, string) error {
	return c.sendErr
}
func (c *turnChat) Steer(_ context.Context, text string) error {
	if c.steerErr == nil {
		c.steered = append(c.steered, text)
	}
	return c.steerErr
}
func (c *turnChat) Interrupt(context.Context) error {
	c.interrupts++
	return c.interruptErr
}
func (c *turnChat) AddInput(context.Context, string) error         { return nil }
func (c *turnChat) End(context.Context, bool) (*run.Result, error) { return &run.Result{}, nil }

func chatAPI(c *turnChat) *RunAPI {
	return &RunAPI{app: &App{ctx: context.Background()}, session: c}
}

func TestSteerTurnSaysWhatBecameOfTheMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"the turn took it", nil, SteerSent},
		{"no turn in flight", session.ErrNoTurn, SteerNoTurn},
		{"a harness that takes one only between turns", fmt.Errorf("%w: docker-agent", session.ErrCannotSteer), SteerBetweenTurns},
	} {
		c := &turnChat{steerErr: tc.err}
		got, err := chatAPI(c).SteerTurn("also check the footnotes")
		if err != nil || got != tc.want {
			t.Fatalf("%s: SteerTurn = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	c := &turnChat{}
	if _, err := chatAPI(c).SteerTurn("also check the footnotes"); err != nil || len(c.steered) != 1 || c.steered[0] != "also check the footnotes" {
		t.Fatalf("the message did not reach the session: %v, %v", c.steered, err)
	}
	// Any other refusal is the page's error, which keeps the text.
	if _, err := chatAPI(&turnChat{steerErr: session.ErrEnded}).SteerTurn("x"); !errors.Is(err, session.ErrEnded) {
		t.Fatalf("an ended session's refusal came back as %v", err)
	}
	if _, err := (&RunAPI{app: &App{ctx: context.Background()}}).SteerTurn("x"); err == nil {
		t.Fatal("a message with no chat was taken")
	}
}

func TestInterruptTurnStopsTheTurnAndTheChatGoesOn(t *testing.T) {
	c := &turnChat{}
	if err := chatAPI(c).InterruptTurn(); err != nil || c.interrupts != 1 {
		t.Fatalf("InterruptTurn = %v after %d interrupts", err, c.interrupts)
	}
	// With no turn in flight there is nothing to stop.
	if err := chatAPI(&turnChat{interruptErr: session.ErrNoTurn}).InterruptTurn(); err != nil {
		t.Fatalf("no turn in flight came back as %v", err)
	}
	if err := chatAPI(&turnChat{interruptErr: errors.New("the codex harness cannot stop a turn")}).InterruptTurn(); err == nil {
		t.Fatal("a turn that did not stop was reported stopped")
	}
	// The turn's own Send ends with ErrInterrupted: the chat takes that as
	// the turn's end, not a failure.
	if err := chatAPI(&turnChat{sendErr: session.ErrInterrupted}).SendTurn("go on"); err != nil {
		t.Fatalf("a stopped turn's send came back as %v", err)
	}
	if err := chatAPI(&turnChat{sendErr: session.ErrEnded}).SendTurn("go on"); !errors.Is(err, session.ErrEnded) {
		t.Fatalf("an ended session's send came back as %v", err)
	}
}
