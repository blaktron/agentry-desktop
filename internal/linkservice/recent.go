package linkservice

// The runs that used the person's computer, as Plugins → Computer use
// lists them: which run, which applications through which launch, the calls
// by class and outcome, the exec questions and their answers, and how the
// run's use ended. Kept in a file of the app's own beside its settings,
// the newest first, a bounded number; never the calls' arguments or
// answers, which the run's own receipt keeps.

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MaxRecent bounds the runs kept.
const MaxRecent = 30

// RecentRun is one run that used the person's computer.
type RecentRun struct {
	Run     string         `json:"run"`
	Ref     string         `json:"ref,omitempty"`
	Agent   string         `json:"agent,omitempty"`
	Started time.Time      `json:"started"`
	Ended   *time.Time     `json:"ended,omitempty"`
	State   string         `json:"state"`
	Servers []RecentServer `json:"servers"`
	// ExecAsked counts the exec questions put to the person, ExecAllowed and
	// ExecDeclined their answers; ExecForRun says one allowed the rest of
	// the run.
	ExecAsked    int  `json:"execAsked"`
	ExecAllowed  int  `json:"execAllowed"`
	ExecDeclined int  `json:"execDeclined"`
	ExecForRun   bool `json:"execForRun"`
}

// RecentServer is one application server a run used.
type RecentServer struct {
	ID          string `json:"id"`
	Application string `json:"application"`
	Package     string `json:"package"`
	Launch      string `json:"launch"`
	Era         string `json:"era,omitempty"`
	// Calls counts the calls by class; Refused the refusals by reason;
	// Errors the calls the server did not answer.
	Calls   map[string]int `json:"calls"`
	Refused map[string]int `json:"refused"`
	Errors  int            `json:"errors"`
}

// Recent is the kept list. A nil Recent keeps nothing.
type Recent struct {
	path string
	mu   sync.Mutex
	runs []*RecentRun
	now  func() time.Time
}

// OpenRecent reads the list kept at path; a missing or unreadable file is
// an empty list.
func OpenRecent(path string) *Recent {
	r := &Recent{path: path, now: time.Now}
	if raw, err := os.ReadFile(path); err == nil { //nolint:gosec // the app's own file
		_ = json.Unmarshal(raw, &r.runs)
	}
	// A run the list holds as unended was using the computer when an
	// earlier Agentry Desktop ended: none of its calls can be answered now.
	for _, run := range r.runs {
		if run != nil && run.Ended == nil {
			run.State = StateInterrupted
		}
	}
	return r
}

// StateInterrupted is a recent run whose use of the computer ended with the
// Agentry Desktop that answered it.
const StateInterrupted = "interrupted"

// update changes a record under the list's lock, and writes the list after
// when save is set. On a nil list it does nothing: there is no record.
func (r *Recent) update(save bool, f func()) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	f()
	if save {
		r.saveLocked()
	}
}

// Begin records a run's binding.
func (r *Recent) Begin(run RecentRun) *RecentRun {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run.Started = r.now().UTC()
	rr := &run
	r.runs = append([]*RecentRun{rr}, r.runs...)
	if len(r.runs) > MaxRecent {
		r.runs = r.runs[:MaxRecent]
	}
	r.saveLocked()
	return rr
}

// List is the kept runs, the newest first.
func (r *Recent) List() []RecentRun {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RecentRun, 0, len(r.runs))
	for _, run := range r.runs {
		if run == nil {
			continue
		}
		c := *run
		c.Servers = make([]RecentServer, len(run.Servers))
		for i, s := range run.Servers {
			s.Calls, s.Refused = maps.Clone(s.Calls), maps.Clone(s.Refused)
			c.Servers[i] = s
		}
		out = append(out, c)
	}
	return out
}

func (r *Recent) saveLocked() {
	if r.path == "" {
		return
	}
	raw, err := json.MarshalIndent(r.runs, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, r.path)
}

// The record's updates, called under the list's lock (Recent.update); each
// is a no-op on a nil record.

func (rr *RecentRun) call(server, class, outcome string) {
	if rr == nil {
		return
	}
	for i := range rr.Servers {
		s := &rr.Servers[i]
		if s.ID != server {
			continue
		}
		s.Calls[class]++
		switch outcome {
		case "":
		case "error":
			s.Errors++
		default:
			s.Refused[outcome]++
		}
	}
}

func (rr *RecentRun) exec(allowed bool) {
	if rr == nil {
		return
	}
	rr.ExecAsked++
	if allowed {
		rr.ExecAllowed++
	} else {
		rr.ExecDeclined++
	}
}

func (rr *RecentRun) execForRun() {
	if rr == nil {
		return
	}
	rr.ExecAsked++
	rr.ExecAllowed++
	rr.ExecForRun = true
}

func (rr *RecentRun) setState(state string) {
	if rr != nil {
		rr.State = state
	}
}

func (rr *RecentRun) end(state string) {
	if rr == nil {
		return
	}
	now := time.Now().UTC()
	rr.Ended = &now
	if state == StateStopped {
		rr.State = StateStopped
	} else {
		rr.State = "ended"
	}
}
