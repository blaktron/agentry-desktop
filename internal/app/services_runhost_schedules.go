package app

// A runhost's schedules from the app: Settings → Runs → Runhosts lists what each
// runhost's store keeps (never a value) and removes a schedule there; and
// CollectSchedules, which the app calls at its start and then while it runs,
// opens each finished run of a runhost schedule into the output folder and
// the run history, so Home shows it as any run.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	"github.com/blaktron/agentry-cli/pkg/runhost/remote"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// RunhostScheduleView is one schedule a runhost runs, as Settings lists it.
type RunhostScheduleView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Ref   string `json:"ref"`
	Next  string `json:"next,omitempty"`
	Ended bool   `json:"ended,omitempty"`
	// Keeps says what the schedule keeps in the runhost's store.
	Keeps string `json:"keeps"`
	// Runs is how many of its runs the runhost still holds.
	Runs int `json:"runs"`
	// Model is the runhost's saved model each run is on, by its id, or
	// "default" for the runhost's default at each run; "" for a schedule
	// that carries the model of the device that made it.
	Model string `json:"model,omitempty"`
}

// RunhostStoreView is what a runhost's store keeps: never a value.
type RunhostStoreView struct {
	Releases  int                   `json:"releases"`
	Credits   string                `json:"credits,omitempty"`
	SignIns   []string              `json:"signIns"`
	Schedules []RunhostScheduleView `json:"schedules"`
	// Sealed says whether the store's key is sealed by the server's
	// systemd, which a schedule needs.
	Sealed bool `json:"sealed"`
}

func entriesSaid(es []runhost.ScheduleEntry) string {
	if len(es) == 0 {
		return "nothing but its release"
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Kind+" "+e.Name)
	}
	return joinList(out)
}

func joinList(s []string) string {
	switch len(s) {
	case 0:
		return ""
	case 1:
		return s[0]
	}
	out := s[0]
	for _, x := range s[1 : len(s)-1] {
		out += ", " + x
	}
	return out + " and " + s[len(s)-1]
}

// Store lists what runhost name's store keeps, over its run key.
func (r *RunhostsAPI) Store(name string) (*RunhostStoreView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), 30*time.Second)
	defer cancel()
	_, _, c, err := r.dial(ctx, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	list, err := remote.ListStore(ctx, c)
	if err != nil {
		return nil, err
	}
	v := &RunhostStoreView{Releases: list.Releases, SignIns: []string{}, Schedules: []RunhostScheduleView{}, Sealed: list.Sealed}
	if list.Credits != nil {
		v.Credits = list.Credits.Name
	}
	for _, k := range list.SignIns {
		v.SignIns = append(v.SignIns, k.Vendor)
	}
	for _, s := range list.Schedules {
		sv := RunhostScheduleView{ID: s.ID, Name: s.Name, Ref: s.Ref, Ended: s.Ended, Keeps: entriesSaid(s.Entries), Model: s.Model}
		if s.Next != nil {
			sv.Next = s.Next.UTC().Format(time.RFC3339)
		}
		for _, f := range s.Fires {
			if f.Job != "" {
				sv.Runs++
			}
		}
		v.Schedules = append(v.Schedules, sv)
	}
	return v, nil
}

// RemoveSchedule takes schedule id off runhost name, with everything it
// kept in the store, after the native confirmation.
func (r *RunhostsAPI) RemoveSchedule(name, id string) (*RunhostStoreView, error) {
	if err := r.app.askConsent("Remove this schedule from runhost "+name+"?", "The schedule stops, and the runhost's store forgets everything it kept for it. Its runs collected here stay."); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(r.ctx(), 30*time.Second)
	_, _, c, err := r.dial(ctx, name)
	if err != nil {
		cancel()
		return nil, err
	}
	_, err = remote.RemoveSchedule(ctx, c, id)
	_ = c.Close()
	cancel()
	if err != nil {
		return nil, err
	}
	emitEvent(r.app, eventSchedulesChanged)
	return r.Store(name)
}

// runhostCollectedFile keeps the runhost schedule runs this app has
// collected, beside its settings, so none is opened twice.
const runhostCollectedFile = "runhost-collected.json"

var runhostCollectedPath = func() (string, error) {
	p, err := settings.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(p), runhostCollectedFile), nil
}

// runhostCollectScheduled is run.CollectScheduledRun; a test replaces it.
var runhostCollectScheduled = run.CollectScheduledRun

// CollectSchedules opens every finished run of a runhost schedule this app
// has not collected yet: each runhost that takes schedules is asked over its
// run key, and each run is collected into the output folder Settings names
// and recorded in the run history. A runhost that cannot be reached waits
// for the next call. It returns how many runs it opened.
func (r *RunhostsAPI) CollectSchedules() (int, error) {
	home, err := r.home()
	if err != nil {
		return 0, err
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil || len(recs.Runhosts) == 0 {
		return 0, err
	}
	p, err := runhostCollectedPath()
	if err != nil {
		return 0, err
	}
	var done []string
	if b, err := os.ReadFile(p); err == nil { //nolint:gosec // the app's own file
		_ = json.Unmarshal(b, &done)
	}
	s, _ := settings.Load()
	n := 0
	for _, rec := range recs.Runhosts {
		if !rec.Capabilities["schedules"] {
			continue
		}
		ctx, cancel := context.WithTimeout(r.ctx(), 10*time.Minute)
		_, _, c, err := r.dial(ctx, rec.Name)
		if err != nil {
			cancel()
			continue
		}
		list, err := remote.Schedules(ctx, c)
		_ = c.Close()
		if err != nil {
			cancel()
			continue
		}
		for _, sc := range list {
			for _, f := range sc.Fires {
				if f.Job == "" || f.Outcome == "running" || slices.Contains(done, f.Job) {
					continue
				}
				started := time.Now().UTC()
				res, err := runhostCollectScheduled(ctx, rec.Name, f.Job, sc.Ref, f.ExitCode, s.DefaultOutputDir, r.say(rec.Name))
				if err != nil {
					log.Printf("agentry-desktop: runhost %s: collect schedule %s's run %s: %v", rec.Name, sc.ID, f.Job, err)
					continue
				}
				done = append(done, f.Job)
				n++
				recordScheduledRun(r.app, rec.Name, sc.Ref, f, started, res)
			}
		}
		cancel()
	}
	if n > 0 {
		if len(done) > 500 {
			done = done[len(done)-500:]
		}
		b, _ := json.Marshal(done)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			return n, err
		}
	}
	return n, nil
}

// recordScheduledRun puts a collected runhost schedule run in the run
// history, as a run of this computer's is.
func recordScheduledRun(app *App, name, ref string, f runhost.ScheduleFire, started time.Time, res *run.Result) {
	e := runhistory.Entry{ID: res.RunID, Kind: runhistory.KindRun, Where: runhistory.WhereHere, Ref: refOrEmpty(ref),
		StartedAt: f.Occurrence, EndedAt: started, Outcome: runhistory.OutcomeOK, ExitCode: res.ExitCode,
		Digest: res.Digest, Folder: cleanAbs(res.OutDir), Runhost: name}
	if !runhistory.ValidID(e.ID) {
		e.ID = f.Job
	}
	if res.SavedModel != nil {
		e.Model = res.SavedModel.Name
	}
	if res.Status != "ok" {
		e.Outcome, e.ExitClass, e.Cause = runhistory.OutcomeError, res.ExitClass, fmt.Sprintf("on runhost %s: %s", name, res.Error)
	}
	app.noticed(localEntry(e, ref), true)
	record(e)
}
