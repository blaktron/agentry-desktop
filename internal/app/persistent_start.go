package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
)

// startingAgent is a start under way: its cancel, closed done once the
// start has returned and its outcome is recorded, the step it is on in a
// person's words, and whether a Stop cancelled it.
type startingAgent struct {
	cancel  func()
	done    chan struct{}
	stage   string
	stopped bool
}

// PersistentFailureView is how the agent's last start failed: the error,
// the step it stopped in (in the words the start showed), the reason in the
// runner's fixed words when the step named one, and the log that has the
// rest.
type PersistentFailureView struct {
	Error  string                `json:"error"`
	Stage  string                `json:"stage,omitempty"`
	Reason *PersistentReasonView `json:"reason,omitempty"`
	Log    string                `json:"log"`
	At     string                `json:"at"`
}

// PersistentReasonView is a run's reason: a code of the runner's closed
// list, its detail, and the declared names it concerns (a server, a
// secret), never text a run wrote.
type PersistentReasonView struct {
	Code   string   `json:"code"`
	Detail string   `json:"detail,omitempty"`
	Names  []string `json:"names"`
}

func reasonView(r *receipt.Reason) *PersistentReasonView {
	r = r.Clean()
	if r == nil {
		return nil
	}
	return &PersistentReasonView{Code: string(r.Code), Detail: r.Detail, Names: append([]string{}, r.Names...)}
}

// supervisorLog is the agent's supervisor.log as this app writes it: what
// a segment's start prints (image builds, server installs) and each step it
// reaches, as the CLI's supervisor writes its own. It is opened, and an
// earlier start's lines dropped, at the first write, which comes only once
// the start holds the agent's owner lock, so a start refused because
// another client runs the agent leaves that client's log as it is. A write
// never fails the segment: a log that cannot be opened is reported once on
// the app's own log and its lines dropped.
type supervisorLog struct {
	path string

	mu     sync.Mutex
	f      *os.File
	broken bool
	closed bool
}

func newSupervisorLog(home, name string) *supervisorLog {
	return &supervisorLog{path: persistent.LogPath(home, name)}
}

func (l *supervisorLog) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.broken {
		return len(b), nil
	}
	if l.f == nil {
		f, err := openLog(l.path)
		if err != nil {
			l.broken = true
			log.Printf("agentry-desktop: %s: %v", l.path, err)
			return len(b), nil
		}
		l.f = f
	}
	_, _ = l.f.Write(b)
	return len(b), nil
}

func openLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) //nolint:gosec // the runner home's own file
	if err != nil {
		return nil, err
	}
	_ = f.Chmod(0o600) // a log an earlier start left keeps its mode otherwise
	return f, nil
}

// Close ends the log; later writes are dropped.
func (l *supervisorLog) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.f != nil {
		_ = l.f.Close()
		l.f = nil
	}
}

// stageHook is the supervisor's OnStage: each step a segment's start
// reaches is a "stage:" line of the log, and, while the agent's own start is
// under way, the step its view shows under its name, pushed with a state
// event. A restart for a change, once the agent runs, is logged alone. It is
// called on the starting goroutine, so it does nothing slow.
func (p *PersistentAPI) stageHook(name string, w io.Writer) func(persistent.Stage) {
	return func(st persistent.Stage) {
		words := st.Words()
		_, _ = fmt.Fprintf(w, "stage: %s\n", words)
		p.mu.Lock()
		s := p.starting[name]
		if s != nil {
			s.stage = words
		}
		p.mu.Unlock()
		if s != nil {
			emitEvent(p.app, eventAgentState, name)
		}
	}
}

// startFailure is the failure of a start that began at since: its error and
// reason, or, when the segment got far enough to write its envelope, the
// envelope's.
func startFailure(home, name, stage string, since time.Time, err error) *PersistentFailureView {
	f := &PersistentFailureView{
		Error: err.Error(), Stage: stage, Log: persistent.LogPath(home, name),
		At: time.Now().UTC().Format(time.RFC3339),
	}
	f.Reason = reasonView(receipt.ReasonOf(err))
	if env := segmentEnvelope(home, name, since); env != nil && env.Status == "error" {
		if env.Error != "" {
			f.Error = env.Error
		}
		if r := reasonView(env.Reason); r != nil {
			f.Reason = r
		}
	}
	return f
}

// segmentEnvelope is the envelope of the newest segment folder written
// since the start began, if one wrote it. The folders are the CLI's, under
// the agent's state folder.
func segmentEnvelope(home, name string, since time.Time) *receipt.Envelope {
	dir := filepath.Join(persistent.Dir(home, name), "segments")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var newest *receipt.Envelope
	var at time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "result.json")
		fi, err := os.Stat(path)
		if err != nil || fi.ModTime().Before(since.Add(-time.Second)) || fi.ModTime().Before(at) {
			continue
		}
		b, err := os.ReadFile(path) //nolint:gosec // the runner home's own file
		if err != nil {
			continue
		}
		var env receipt.Envelope
		if json.Unmarshal(b, &env) != nil {
			continue
		}
		newest, at = &env, fi.ModTime()
	}
	return newest
}
