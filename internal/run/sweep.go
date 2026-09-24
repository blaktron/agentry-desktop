package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
)

// Sweep removes whatever a run left behind under its label: its containers
// (agent, gateway, proxy, tool containers the gateway started), then its
// volumes, then its network. The engine's own teardown does exactly this
// when its Run or session End returns (§R11 invariant 6, "teardown on every
// path"); Sweep is the desktop's second pass for the one path the engine
// cannot see, the app quitting while a run is still in flight
// (agentry-desktop#125), when the bounded wait for the engine's teardown
// ran out. Each step is best-effort and absent objects are not errors, so
// sweeping a run the engine already tore down is a no-op.
func Sweep(ctx context.Context, runID string) error {
	if runID == "" {
		return errors.New("sweep: no run id")
	}
	d := newDockerClient()
	label := "agentry.run=" + runID
	var errs []error
	names, err := d.PSByLabel(ctx, label)
	if err != nil {
		errs = append(errs, fmt.Errorf("list containers: %w", err))
	}
	for _, n := range names {
		if err := d.RemoveForce(ctx, n); err != nil {
			errs = append(errs, fmt.Errorf("remove container %s: %w", n, err))
		}
	}
	// Volumes after the containers: a volume in use cannot go.
	vols, err := d.VolumesByLabel(ctx, label)
	if err != nil {
		errs = append(errs, fmt.Errorf("list volumes: %w", err))
	}
	for _, v := range vols {
		if err := d.VolumeRemove(ctx, v); err != nil {
			errs = append(errs, fmt.Errorf("remove volume %s: %w", v, err))
		}
	}
	// The network is found by the engine's name for it
	// (agentry-cli pkg/runner/engine networkName); the client has no
	// list-by-label for networks. Absent is not an error.
	if err := d.NetworkRemove(ctx, "agentry-"+runID+"-net"); err != nil {
		errs = append(errs, fmt.Errorf("remove network: %w", err))
	}
	return errors.Join(errs...)
}

// NewRunID mints a run id the engine's way.
func NewRunID() (string, error) { return engine.NewRunID() }
