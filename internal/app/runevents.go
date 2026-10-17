package app

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
)

// runEventQueue is how many of a run's events wait for the log before the
// next is dropped: a burst of tool calls fits, and a log that cannot keep up
// costs lines, never the run's time.
const runEventQueue = 1024

// resultLines and messageLines are the most lines of a tool's result and of
// one agent message the log shows. The result's bytes are already cut by
// the engine; a message is not, and the run's result shows the last one
// whole.
const (
	resultLines  = 20
	messageLines = 60
)

// logIndent sets what a tool or the model wrote under the line that names
// it, so their text cannot pass for one of the log's own lines.
const logIndent = "    "

// runEventCloseWait bounds how long a finished run waits for its last
// lines to reach the log; a test shortens it.
var runEventCloseWait = 5 * time.Second

// runEventLog writes a live run's harness events into its run log. The
// engine hands each event over on the goroutine that also enforces the
// run's turn and cost budgets and releases its teardown, so Receive never
// waits: it queues the event, or drops it and counts it when the queue is
// full, and a goroutine of its own writes the lines.
type runEventLog struct {
	write func(line string)
	queue chan adapter.Event

	mu      sync.Mutex
	closed  bool
	dropped int
	done    chan struct{}
}

// newRunEventLog starts the log; write takes one line at a time, from the
// log's own goroutine only.
func newRunEventLog(write func(line string)) *runEventLog {
	l := &runEventLog{write: write, queue: make(chan adapter.Event, runEventQueue), done: make(chan struct{})}
	go l.drain()
	return l
}

// Receive is the run's event hook. It never waits on the log: the send is
// non-blocking, and its lock is never held while a line is written.
func (l *runEventLog) Receive(ev adapter.Event) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	select {
	case l.queue <- ev:
	default:
		l.dropped++
	}
}

// Close ends the log once the run has returned, so nothing is received
// after it, and waits for the queued lines to be written, at most
// runEventCloseWait: a log that has stopped moving loses its last lines
// rather than keep the run's result from the page.
func (l *runEventLog) Close() {
	l.mu.Lock()
	if !l.closed {
		l.closed = true
		close(l.queue)
	}
	l.mu.Unlock()
	select {
	case <-l.done:
	case <-time.After(runEventCloseWait):
	}
}

// drain writes the queued events. A write that panics stops the log, not
// the app: the rest of the queue is let go.
func (l *runEventLog) drain() {
	defer close(l.done)
	defer func() {
		if recover() != nil {
			for range l.queue {
			}
		}
	}()
	for ev := range l.queue {
		l.sayDropped()
		for _, line := range eventLines(ev) {
			l.write(line)
		}
	}
	l.sayDropped()
}

// sayDropped writes how many events the log could not keep up with since
// it last said so. The run's events receipt keeps every one.
func (l *runEventLog) sayDropped() {
	l.mu.Lock()
	n := l.dropped
	l.dropped = 0
	l.mu.Unlock()
	if n == 0 {
		return
	}
	what := "events are"
	if n == 1 {
		what = "event is"
	}
	l.write(fmt.Sprintf("(%d of the agent's %s not shown here: the log could not keep up. The run's receipts keep every event.)", n, what))
}

// eventLines is how one event reads in the run log: a tool call by its
// tool, a tool's result with what it returned, the agent's message, and
// what ended the run when the agent's side reported it. Usage, reasoning
// and the harness's own bookkeeping are not lines: the progress above the
// log shows the usage, and reasoning is never output.
func eventLines(ev adapter.Event) []string {
	text := strings.TrimSpace(ev.Text)
	switch ev.Kind {
	case adapter.EventToolCall:
		return []string{"Tool call: " + logText(toolLabel(ev))}
	case adapter.EventToolResult:
		head := "Tool result (" + logText(toolLabel(ev)) + "):"
		if ev.Failed {
			head = "Tool failed (" + logText(toolLabel(ev)) + "):"
		}
		if text == "" {
			return []string{head + " nothing returned"}
		}
		return append([]string{head}, indent(bodyLines(text, resultLines))...)
	case adapter.EventAssistantText:
		if text == "" {
			return nil
		}
		body := bodyLines(text, messageLines)
		return append([]string{"Agent: " + body[0]}, indent(body[1:])...)
	}
	if text == "" {
		return nil
	}
	switch ev.Kind {
	case adapter.EventError:
		return []string{"Error: " + logText(reportLine(text))}
	case adapter.EventPlanLimit:
		return []string{"Plan limit reached: " + logText(reportLine(text))}
	case adapter.EventOutOfCredits:
		return []string{"Out of credits: " + logText(reportLine(text))}
	case adapter.EventOrgPolicy:
		return []string{"Refused by an organization's policy: " + logText(reportLine(text))}
	}
	return nil
}

// toolLabel names an event's tool: its name, else the title a harness gave
// the call, else "a tool".
func toolLabel(ev adapter.Event) string {
	switch {
	case ev.Tool != "":
		return ev.Tool
	case ev.Kind == adapter.EventToolCall && ev.Text != "":
		return ev.Text
	}
	return "a tool"
}

// bodyLines is text as the log's lines, at most limit of them and then how
// many more there were.
func bodyLines(text string, limit int) []string {
	src := strings.Split(text, "\n")
	var out []string
	for i, line := range src {
		if i == limit {
			out = append(out, fmt.Sprintf("(%d more lines)", len(src)-limit))
			break
		}
		out = append(out, logText(strings.TrimRight(line, "\r")))
	}
	return out
}

// indent sets lines under the line that names them.
func indent(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = logIndent + l
	}
	return out
}

// reportLine is text up to its first line break: a one-line report.
func reportLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimRight(line, "\r")
}

// logText is one line of what a tool or the model wrote, as the log shows
// it: control characters, which could rewrite or hide what the log shows
// (a terminal escape, a carriage return), become the replacement
// character. A tab stays. Invisible format characters are left for the
// log's own reveal to name.
func logText(s string) string {
	return strings.Map(func(r rune) rune {
		if r != '\t' && unicode.IsControl(r) {
			return '\uFFFD'
		}
		return r
	}, s)
}
