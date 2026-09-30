package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/blaktron/agentry-cli/pkg/removal"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
)

// Sweep removes whatever a run left behind on this computer outside its
// output folder: its containers (agent, gateway, proxy, tool containers the
// gateway started), then its volumes, then its networks, then its scratch
// directory under the runner home and the socket of its docker filter. The
// engine's own teardown does exactly this when its Run or session End
// returns (§R11 invariant 6, "teardown on every path"); Sweep is the
// desktop's second pass for the paths the engine cannot see: the app
// quitting while a run is still in flight (agentry-desktop#125), when the
// bounded wait for the engine's teardown ran out, and the app killed
// mid-run (agentry-desktop#198), which runs no teardown at all and leaves
// the run's secrets file, staged inputs and knowledge pages in the clear
// (agentry-desktop#463). Each step is best-effort and absent objects are
// not errors, so sweeping a run the engine already tore down is a no-op.
//
// The files go whatever the docker steps answered: a daemon that is not
// running must not keep a secrets file on disk. Nothing else is touched:
// the run's output folder with its receipts, the QuickAgent's install
// record and cached release, its kept profile and every docker image are
// the person's or the QuickAgent's, not the run's (plan
// desktop-review-key-and-run-cards.md §5.4).
func Sweep(ctx context.Context, runID string) error {
	// The id becomes a docker label, object names and a path element. An
	// empty one would match every run on the machine, the CLI's included.
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("sweep: bad run id %q", runID)
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
	// Networks last, since one with a container attached cannot go. Absent
	// is not an error.
	for _, n := range runNetworks(runID) {
		if err := d.NetworkRemove(ctx, n); err != nil {
			errs = append(errs, fmt.Errorf("remove network %s: %w", n, err))
		}
	}
	errs = append(errs, sweepFiles(runID)...)
	return errors.Join(errs...)
}

// runNetworks are the networks a run can make, found by the engine's names
// for them (agentry-cli pkg/runner/engine: networkName, proxyEgressNetwork,
// webNetwork, relayNetworks), since the client has no list-by-label for
// networks: the run network; the model proxy's egress network under a
// sandboxed runtime; the web tools' network; and the relays' pair. The
// names are the engine's own and not exported, so they are restated here as
// engine.SweepAbandoned restates them, and held to the engine by
// TestSweepNamesEveryNetworkTheEngineMakes.
func runNetworks(runID string) []string {
	p := "agentry-" + runID
	return []string{p + "-net", p + "-egress", p + "-web", p + "-relays", p + "-relays-out"}
}

// sweepFiles removes what a run keeps under the runner home while it is
// live: its scratch directory, <release directory>/runs/<run id>, which
// holds the workspace copy, the staged inputs, the copies of the attached
// knowledge packs and the rendered configuration with secrets.env; and the
// socket of its docker filter, <runner home>/s/<run id>.sock.
//
// The scratch directory is found by the run id under every release
// directory, not from a path the record carries: the record is written
// before the release is resolved, and a kill between the two must still
// leave the next start enough to find it. A run id belongs only to the
// process that minted it (D-034), so a directory of that name is that
// run's under whichever release it ran.
func sweepFiles(runID string) []error {
	home, err := Home()
	if err != nil {
		return []error{fmt.Errorf("runner home: %w", err)}
	}
	var errs []error
	for _, dir := range scratchDirs(home, runID) {
		if err := removal.Remove(dir, true); err != nil {
			errs = append(errs, fmt.Errorf("remove the run's directory: %w", err))
		}
	}
	if err := removal.Remove(filepath.Join(home, "s", runID+".sock"), false); err != nil {
		errs = append(errs, fmt.Errorf("remove the run's docker filter socket: %w", err))
	}
	return errs
}

// scratchDirs lists runs/<run id> under every release directory of the
// runner home: <home>/agents/<namespace>/<name>/<release>, the one layout
// the engine's RunRoot has for a release from a registry, a local install
// and a built-in alike (installdir.ReleaseDir).
func scratchDirs(home, runID string) []string {
	var out []string
	sub := func(dir string) []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var dirs []string
		for _, e := range entries {
			dirs = append(dirs, filepath.Join(dir, e.Name()))
		}
		return dirs
	}
	for _, ns := range sub(installdir.AgentsRoot(home)) {
		for _, agent := range sub(ns) {
			for _, release := range sub(agent) {
				scratch := filepath.Join(release, "runs", runID)
				if _, err := os.Lstat(scratch); err == nil {
					out = append(out, scratch)
				}
			}
		}
	}
	return out
}

// NewRunID mints a run id the engine's way.
func NewRunID() (string, error) { return engine.NewRunID() }
