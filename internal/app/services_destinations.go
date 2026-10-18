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
// signing secret and an Authorization value go in with Save and never come
// back: no view carries one.
type DestinationsAPI struct{ app *App }

// List is the saved destinations.
func (d *DestinationsAPI) List() (*destinations.Status, error) { return destinations.Load() }

// Check says why an address cannot be a destination, in the CLI's words, or ""
// when it can; the page asks as the person types.
func (d *DestinationsAPI) Check(url string, onMyNetwork bool) string {
	return destinations.Check(strings.TrimSpace(url), onMyNetwork)
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
		return nil, errors.New("that destination is no longer saved with a signing secret: add it again in Settings → Destinations")
	}
	ctx, cancel := context.WithTimeout(r.app.ctxOrBackground(), retryTimeout)
	defer cancel()
	if _, err := redeliver(ctx, dir, []string{destination}, r.eventWriter(eventRunLog)); err != nil {
		return nil, err
	}
	return r.RunDeliverables(runID)
}
