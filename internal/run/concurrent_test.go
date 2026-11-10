package run

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// A second run's preparation waits for the first's, says so in its log, and
// takes the slot once the first lets it go; a run stopped while it waits
// gives up without it. Releasing twice gives the slot back once.
func TestRunsPrepareOneAtATime(t *testing.T) {
	first, err := holdPrepare(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	got := make(chan func(), 1)
	go func() {
		r, err := holdPrepare(context.Background(), &log)
		if err != nil {
			t.Error(err)
		}
		got <- r
	}()
	select {
	case <-got:
		t.Fatal("the second run prepared beside the first")
	case <-time.After(50 * time.Millisecond):
	}
	first()
	first()
	var second func()
	select {
	case second = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the second run never got to prepare")
	}
	if !strings.Contains(log.String(), "waiting for another run") {
		t.Errorf("the waiting run's log: %q", log.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := holdPrepare(ctx, io.Discard); !errors.Is(err, context.Canceled) {
		t.Errorf("a stopped run waiting to prepare: %v", err)
	}
	second()
	third, err := holdPrepare(context.Background(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	third()
}

// An unkept MCP profile held by two runs of one release is removed only
// when the last of them lets it go, and a hold let go twice counts once.
func TestTheLastRunOfAReleaseLetsItsProfileGo(t *testing.T) {
	dir := t.TempDir()
	a, b := useProfile(dir), useProfile(dir)
	if a.leave() {
		t.Fatal("the first run to end was taken for the last")
	}
	if a.leave() {
		t.Fatal("a second leave of the same hold counted again")
	}
	if !b.leave() {
		t.Fatal("the last run to end was not taken for the last")
	}
	if c := useProfile(dir); !c.leave() {
		t.Fatal("a run alone with the profile was not the last")
	}
	var none *profileHold
	if !none.leave() {
		t.Fatal("a run with no hold is not the last")
	}
}
