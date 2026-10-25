package app

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/session"
	"github.com/blaktron/agentry-desktop/internal/notify"
)

func TestTaskNoticeReadsTheRunnersNoticeAndNothingElse(t *testing.T) {
	cases := []struct {
		text, task, line string
		ok               bool
	}{
		{`Task "Morning summary" ran: Three meetings today.`, "Morning summary", "Three meetings today.", true},
		{`Task "say \"hi\"" ran: hi`, `say "hi"`, "hi", true},
		{`Task "Empty" ran: `, "Empty", "", true},
		{`Task "Ran: twice" ran: ran: twice`, "Ran: twice", "ran: twice", true},
		{`Task "x" ran`, "", "", false},
		{`Task "" ran: y`, "", "", false},
		{`Task x ran: y`, "", "", false},
		{`Task "unterminated ran: y`, "", "", false},
		{"The agent stopped by itself: its segment ended.", "", "", false},
		{`Installed "weather".`, "", "", false},
	}
	for _, c := range cases {
		task, line, ok := taskNotice(c.text)
		if task != c.task || line != c.line || ok != c.ok {
			t.Errorf("%q: got %q %q %v", c.text, task, line, ok)
		}
	}
}

// A running agent's task notice raises a notification with the agent's and
// the task's name and the answer's first line, keyed to the agent's page;
// none while its page is in front, none while the person has them off, and
// none for any other notice.
func TestATasksAnswerRaisesADesktopNotification(t *testing.T) {
	rig := newPersistentRig(t)
	got := make(chan notify.Notice, 8)
	rig.p.showNotice = func(n notify.Notice) error { got <- n; return nil }
	var minimised atomic.Bool
	// Read slowly: a notice decided after the page has its event would see
	// what the test set next, and raise or hold the wrong notification.
	rig.p.minimised = func() bool {
		time.Sleep(20 * time.Millisecond)
		return minimised.Load()
	}
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	a, err := persistent.Lookup("assistant")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := persistent.LoadConfig(rig.home, "assistant")
	if err != nil {
		t.Fatal(err)
	}
	// queued counts the events sent; settle waits until the follower has
	// handed every one of them on, so whatever a notice raises is decided.
	rig.mu.Lock()
	queued := len(rig.emitted)
	rig.mu.Unlock()
	push := func(ev session.Event) {
		queued++
		rig.agent.events <- ev
	}
	send := func(text string) {
		push(session.Event{Kind: session.KindNotice, Time: time.Now(), Text: text})
	}
	settle := func() {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			rig.mu.Lock()
			n := len(rig.emitted)
			rig.mu.Unlock()
			if n >= queued {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("the follower did not hand the events on")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	next := func() notify.Notice {
		t.Helper()
		select {
		case n := <-got:
			return n
		case <-time.After(5 * time.Second):
			t.Fatal("no notification")
		}
		return notify.Notice{}
	}

	send(`Task "Morning summary" ran: Three meetings today.`)
	settle()
	if n := next(); n.Title != a.DisplayName(cfg)+": Morning summary" || n.Body != "Three meetings today." || n.Key != "assistant" {
		t.Errorf("notification %+v", n)
	}

	send("The agent stopped by itself: its segment ended.")
	push(session.Event{Kind: session.KindAssistantText, Time: time.Now(), Text: `Task "x" ran: y`})
	settle()
	rig.p.SetViewing("assistant")
	send(`Task "While looking" ran: seen already`)
	settle()
	// Minimised, the page in front is not in front of the person.
	minimised.Store(true)
	send(`Task "Minimised" ran: shown`)
	if n := next(); n.Body != "shown" {
		t.Errorf("a minimised window's agent: %+v", n)
	}
	minimised.Store(false)
	rig.p.SetViewing("coder")
	send(`Task "Empty" ran: `)
	if n := next(); n.Body != emptyAnswer {
		t.Errorf("an empty answer: %+v", n)
	}

	if on, err := rig.p.SetTaskNotifications(false); err != nil || on || rig.p.TaskNotifications() {
		t.Fatalf("switched off: %v %v", on, err)
	}
	send(`Task "While off" ran: nobody told`)
	settle()
	if on, err := rig.p.SetTaskNotifications(true); err != nil || !on || !rig.p.TaskNotifications() {
		t.Fatalf("switched on: %v %v", on, err)
	}
	send(`Task "Last" ran: done`)
	if n := next(); n.Body != "done" {
		t.Errorf("a notification that should not have been raised came first: %+v", n)
	}
	select {
	case n := <-got:
		t.Errorf("one too many: %+v", n)
	case <-time.After(300 * time.Millisecond):
	}
}
