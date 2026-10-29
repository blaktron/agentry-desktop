package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/session"
)

// A turn a persistent agent's heartbeat sent reaches the page with its
// task's mark, live and from the transcript, and the person's own turn
// carries none.
func TestATaskTurnReachesThePageMarked(t *testing.T) {
	at := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	task := session.Event{Kind: session.KindUserTurn, Time: at, Text: "Task: Stand up\n\nRemind the person.", Task: "t1"}
	person := session.Event{Kind: session.KindUserTurn, Time: at, Text: "thanks"}

	b, err := json.Marshal(eventView(task))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"task":"t1"`) {
		t.Errorf("the live view of a task's turn: %s", b)
	}
	if b, _ := json.Marshal(eventView(person)); strings.Contains(string(b), `"task"`) {
		t.Errorf("the person's turn carries a task: %s", b)
	}

	// The transcript keeps the mark, and the history the page draws from it
	// hands it on.
	var entries []persistent.Entry
	for _, line := range []string{
		`{"segment":1,"run":"r1","kind":"user_turn","time":"2026-10-24T09:00:00Z","text":"Task: Stand up\n\nRemind the person.","task":"t1"}`,
		`{"segment":1,"run":"r1","kind":"user_turn","time":"2026-10-24T09:01:00Z","text":"thanks"}`,
	} {
		var e persistent.Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, e)
	}
	history := persistent.HistoryEvents(entries)
	if len(history) != 2 || eventView(history[0]).Task != "t1" || eventView(history[1]).Task != "" {
		t.Errorf("the history's views: %+v", history)
	}
}

// An upkeep turn reaches the page with the runner's mark, live and from the
// transcript, so the chat never draws it as the person's.
func TestAnUpkeepTurnReachesThePageMarked(t *testing.T) {
	at := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	upkeep := session.Event{Kind: session.KindUserTurn, Time: at, Text: "Upkeep: compact this conversation (memory-compact).", Upkeep: true}
	b, err := json.Marshal(eventView(upkeep))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"upkeep":true`) {
		t.Errorf("the live view of an upkeep turn: %s", b)
	}
	if b, _ := json.Marshal(eventView(session.Event{Kind: session.KindUserTurn, Time: at, Text: "thanks"})); strings.Contains(string(b), `"upkeep"`) {
		t.Errorf("the person's turn carries the upkeep mark: %s", b)
	}

	var e persistent.Entry
	if err := json.Unmarshal([]byte(`{"segment":1,"run":"r1","kind":"user_turn","time":"2026-10-24T09:00:00Z","text":"Upkeep: compact this conversation (memory-compact).","upkeep":true}`), &e); err != nil {
		t.Fatal(err)
	}
	history := persistent.HistoryEvents([]persistent.Entry{e})
	if len(history) != 1 || !eventView(history[0]).Upkeep {
		t.Errorf("the history's view: %+v", history)
	}
}
