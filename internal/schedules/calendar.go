package schedules

import (
	"errors"
	"os"
	"sort"
	"time"

	"github.com/blaktron/agentry-cli/pkg/schedule"
)

// Outcomes of an occurrence on the calendar (plan invariant 8): what
// happened, never "done" when nothing ran.
const (
	// Due: still to come.
	Due = "due"
	// Ran, Failed and Missed are the fires log's own words.
	Ran    = string(schedule.Ran)
	Failed = string(schedule.Failed)
	Missed = string(schedule.Missed)
)

// Item is one schedule as the page lists it.
type Item struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Ref      string `json:"ref"`
	Version  string `json:"version"`
	Registry string `json:"registry,omitempty"`
	// Local: a QuickAgent directory, not a registry release.
	Local    bool   `json:"local,omitempty"`
	Describe string `json:"describe"`
	// When is the repeat as the edit form takes it.
	When      When   `json:"when"`
	OutDir    string `json:"outDir"`
	Paused    bool   `json:"paused,omitempty"`
	Ended     bool   `json:"ended,omitempty"`
	SignedOut bool   `json:"signedOut,omitempty"`
	// KeysSealed: a signed-out schedule's keys are sealed beside it. One
	// whose re-seal failed has none, and does not run until they are
	// sealed again.
	KeysSealed bool   `json:"keysSealed,omitempty"`
	Runs       int    `json:"runs,omitempty"`
	Next       string `json:"next,omitempty"`
	Last       *Fire  `json:"last,omitempty"`
}

// Fire is one occurrence, or a Run now, as the page shows it.
type Fire struct {
	ScheduleID string `json:"scheduleId"`
	Name       string `json:"name"`
	Ref        string `json:"ref"`
	// At is the occurrence (a Run now's start), RFC 3339.
	At      string `json:"at"`
	Outcome string `json:"outcome"`
	Manual  bool   `json:"manual,omitempty"`
	Started string `json:"started,omitempty"`
	Ended   string `json:"ended,omitempty"`
	RunID   string `json:"runId,omitempty"`
	// ExitCode and ExitClass say how a failed one failed.
	ExitCode  int    `json:"exitCode,omitempty"`
	ExitClass string `json:"exitClass,omitempty"`
	Message   string `json:"message,omitempty"`
	// OutDir is the run's own folder, for the run page; never sent for a
	// fire that made none.
	OutDir string `json:"-"`
}

// Store reads the CLI's schedules under a runner home.
type Store struct{ s schedule.Store }

// StoreIn is the store under the runner home.
func StoreIn(runnerHome string) Store { return Store{schedule.StoreIn(runnerHome)} }

func rfc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func whenOf(r schedule.Recurrence) When {
	return When{
		Kind: string(r.Kind), Date: r.Date, Time: r.Time, Days: r.Days, MonthDay: r.MonthDay,
		End: string(r.End.Kind), Until: r.End.On, Times: r.End.After, Zone: r.Zone,
	}
}

// List is every schedule, the next occurrence of each that is live.
// A store that does not exist yet is an empty list.
func (st Store) List(now time.Time) ([]Item, error) {
	list, _, err := st.s.List()
	if errors.Is(err, os.ErrNotExist) {
		return []Item{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(list))
	for _, sc := range list {
		state, _ := st.s.State(sc.ID)
		it := Item{
			ID: sc.ID, Name: sc.Name, Ref: sc.Ref, Version: sc.Version, Registry: sc.Registry, Local: sc.Local != "",
			Describe: sc.When.Describe(), When: whenOf(sc.When), OutDir: sc.Run.OutDir, Paused: sc.Paused,
			Ended: state.Ended || sc.When.Remaining(state.Runs) == 0, SignedOut: sc.SignedOut, Runs: state.Runs,
		}
		if sc.SignedOut {
			if p, err := st.s.CredentialPath(sc.ID); err == nil {
				_, serr := os.Stat(p)
				it.KeysSealed = serr == nil
			}
		}
		if !it.Ended && !it.Paused {
			if n, ok := sc.When.Next(now); ok {
				it.Next = rfc(n)
			}
		}
		if fires, _ := st.s.Fires(sc.ID, 1); len(fires) == 1 {
			f := fireOf(sc, fires[0])
			it.Last = &f
		}
		items = append(items, it)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func fireOf(sc schedule.Schedule, f schedule.Fire) Fire {
	at := f.Occurrence
	if f.Manual && !f.Started.IsZero() {
		at = f.Started
	}
	return Fire{
		ScheduleID: sc.ID, Name: sc.Name, Ref: sc.Ref + "@" + sc.Version, At: rfc(at), Outcome: string(f.Outcome),
		Manual: f.Manual, Started: rfc(f.Started), Ended: rfc(f.Ended), RunID: f.RunID,
		ExitCode: f.ExitCode, ExitClass: f.ExitClass, Message: f.Message, OutDir: f.OutDir,
	}
}

// maxFires is how many of each schedule's fires the page reads.
const maxFires = 200

// Fires is every schedule's recorded fires, newest first, at most limit
// (0: all read).
func (st Store) Fires(limit int) ([]Fire, error) {
	list, _, err := st.s.List()
	if errors.Is(err, os.ErrNotExist) {
		return []Fire{}, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Fire
	for _, sc := range list {
		fires, _ := st.s.Fires(sc.ID, maxFires)
		for _, f := range fires {
			out = append(out, fireOf(sc, f))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At > out[j].At })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []Fire{}
	}
	return out, nil
}

// FindRun is the fire that made run id, for the run page.
func (st Store) FindRun(runID string) (Fire, bool) {
	fires, err := st.Fires(0)
	if err != nil || runID == "" {
		return Fire{}, false
	}
	for _, f := range fires {
		if f.RunID == runID {
			return f, true
		}
	}
	return Fire{}, false
}

// maxPerSchedule caps the occurrences one schedule puts on a calendar
// range: at most one a day (D2), so a month needs 31.
const maxPerSchedule = 62

// Between is what happened and what is due in [from, to): the recorded
// fires (Run now included) and, from now on, each live schedule's
// occurrences to come, oldest first.
func (st Store) Between(from, to, now time.Time) ([]Fire, error) {
	list, _, err := st.s.List()
	if errors.Is(err, os.ErrNotExist) {
		return []Fire{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Fire{}
	in := func(t time.Time) bool { return !t.Before(from) && t.Before(to) }
	for _, sc := range list {
		fires, _ := st.s.Fires(sc.ID, maxFires)
		for _, f := range fires {
			v := fireOf(sc, f)
			if at, err := time.Parse(time.RFC3339, v.At); err == nil && in(at) {
				out = append(out, v)
			}
		}
		state, _ := st.s.State(sc.ID)
		if sc.Paused || state.Ended {
			continue
		}
		left := sc.When.Remaining(state.Runs)
		if left == 0 || !now.Before(to) {
			continue
		}
		// What is due is counted from now, so a repeat that ends after N
		// runs spends them in order whichever month is shown; then only
		// the range's are kept. Between is (after, until]; the range is
		// [from, to).
		limit := maxPerSchedule
		if from.After(now) {
			limit += int(from.Sub(now)/(24*time.Hour)) + 1
		}
		due := sc.When.Between(now, to.Add(-time.Nanosecond), limit)
		if left > 0 && len(due) > left {
			due = due[:left]
		}
		for _, t := range due {
			if t.Before(from) {
				continue
			}
			out = append(out, Fire{ScheduleID: sc.ID, Name: sc.Name, Ref: sc.Ref + "@" + sc.Version, At: rfc(t), Outcome: Due})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out, nil
}

// SignedOutLive reports whether any schedule that has not ended runs
// signed out: the only ones a key change must seal again.
func (st Store) SignedOutLive() bool {
	list, _, err := st.s.List()
	if err != nil {
		return false
	}
	for _, sc := range list {
		if !sc.SignedOut {
			continue
		}
		if state, _ := st.s.State(sc.ID); !state.Ended {
			return true
		}
	}
	return false
}

// Any reports whether the store holds a schedule.
func (st Store) Any() bool {
	list, _, err := st.s.List()
	return err == nil && len(list) > 0
}
