package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blaktron/agentry-desktop/internal/deliverables"
	"github.com/blaktron/agentry-desktop/internal/destinations"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
)

// retryTimeout bounds a retried delivery: the step, with the runner image built
// first when this computer has none.
const retryTimeout = 20 * time.Minute

// redeliver is the CLI engine's delivery of a finished run's folder; a test
// substitutes it.
var redeliver = run.Redeliver

// DestinationsAPI is Settings → Destinations: the places a run's result may be
// sent, the CLI's own (its destinations.yaml and the system keychain), so a
// run, a schedule's fire and the command line all read what is saved here. A
// signing secret, an Authorization value, or a bucket's access key id and
// secret access key go in with Save and never come back: no view carries one.
type DestinationsAPI struct{ app *App }

// List is the saved destinations.
func (d *DestinationsAPI) List() (*destinations.Status, error) { return destinations.Load() }

// Check says why an address cannot be a destination, in the CLI's words, or ""
// when it can; the page asks as the person types.
func (d *DestinationsAPI) Check(url string, onMyNetwork bool) string {
	return destinations.Check(strings.TrimSpace(url), onMyNetwork)
}

// CheckBucket says why a bucket's fields cannot be a destination, in the CLI's
// words, or "" when they can; the page asks as the person types.
func (d *DestinationsAPI) CheckBucket(bucket, prefix, region, endpoint string, onMyNetwork bool) string {
	return destinations.CheckBucket(destinations.Input{Bucket: bucket, Prefix: prefix, Region: region, Endpoint: endpoint, OnMyNetwork: onMyNetwork})
}

// KeepsKeyPair says saving a bucket as the input names it would keep the key
// pair saved for it, so the page can say when another service needs the pair
// typed again. The input carries no credential the answer depends on.
func (d *DestinationsAPI) KeepsKeyPair(in destinations.Input) bool {
	in.Name, in.Bucket = strings.TrimSpace(in.Name), strings.TrimSpace(in.Bucket)
	return destinations.KeepsKeyPair(in)
}

// Save keeps a destination, refused where the CLI would refuse it, and seals
// the keys of schedules that run while signed out again, since one of them may
// name it.
func (d *DestinationsAPI) Save(in destinations.Input) (*destinations.Status, error) {
	in.Name, in.URL = strings.TrimSpace(in.Name), strings.TrimSpace(in.URL)
	if err := destinations.Save(in); err != nil {
		return nil, err
	}
	resealSchedules(d.app)
	return destinations.Load()
}

// Remove deletes a destination and its credentials.
func (d *DestinationsAPI) Remove(name string) (*destinations.Status, error) {
	if err := destinations.Remove(name); err != nil {
		return nil, err
	}
	resealSchedules(d.app)
	return destinations.Load()
}

// SetDefault sets the destinations the run setup preselects for a QuickAgent
// (namespace/name), none clearing it. It is the person's choice and the
// setup's to apply: a run is given only the names its setup sends.
func (d *DestinationsAPI) SetDefault(agent string, names []string) (*destinations.Status, error) {
	if err := destinations.SetDefault(agent, names); err != nil {
		return nil, err
	}
	return destinations.Load()
}

// DefaultFor is the default set of a QuickAgent (a release ref).
func (d *DestinationsAPI) DefaultFor(ref string) []string { return destinations.DefaultFor(ref) }

// RetryDelivery sends a finished run's result and deliverables again to one
// destination that failed it, as `agentry deliver` does, and answers the run's
// deliverables as they stand. Only a destination the run's own record names, as
// a send that failed, is tried: the page names a run and a destination, never
// an address.
func (r *RunAPI) RetryDelivery(runID, destination string) (*RunDeliverablesView, error) {
	dir, err := r.folderOf(runID)
	if err != nil {
		return nil, err
	}
	if !r.runIsLocal(runID) {
		return nil, errors.New("a run In Agentry is sent from Agentry's own runs, not from this computer")
	}
	rec := deliverables.ReadRecord(dir)
	failed := false
	for _, s := range rec.Sends {
		if s.Destination == destination && s.Status == "failed" {
			failed = true
		}
	}
	if !failed {
		return nil, fmt.Errorf("this run has no failed delivery to %q to try again", destination)
	}
	if !destinations.CanSend(destination) {
		return nil, errors.New("that destination is no longer saved with its credentials: add it again in Settings → Destinations")
	}
	// The run lock, as every job that builds an image or starts a container
	// holds it: no delivery beside a live run, and one at a time.
	if err := r.acquire(jobSetup, "the delivery of a run's result"); err != nil {
		return nil, err
	}
	defer r.release()
	ctx, cancel := context.WithTimeout(r.app.ctxOrBackground(), retryTimeout)
	defer cancel()
	if _, err := redeliver(ctx, dir, []string{destination}, r.eventWriter(eventRunLog)); err != nil {
		return nil, err
	}
	return r.RunDeliverables(runID)
}

// runIsLocal says this computer ran the run: its result came from the engine
// here (resultView), or its history entry, written by this app, says it ran
// here, or it is a fire of a schedule on this computer (the CLI's record).
// Everything in a folder a run In Agentry brought down was written by its
// sandbox, which an agent shared.
func (r *RunAPI) runIsLocal(runID string) bool {
	r.foldersMu.Lock()
	local := r.localRuns[runID]
	r.foldersMu.Unlock()
	if local {
		return true
	}
	if historyOn() {
		if e, ok, err := runhistory.Find(runID); err == nil && ok {
			return e.Where == runhistory.WhereHere
		}
	}
	_, _, ok := scheduledEntry(runID)
	return ok
}
