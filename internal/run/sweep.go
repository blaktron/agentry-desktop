package run

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/blaktron/agentry-cli/pkg/removal"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
)

// Sweep removes what a run left behind on this computer outside its output
// folder: its containers (agent, gateway, proxy, tool containers the
// gateway started), then its volumes, then its networks, then its scratch
// directory under the runner home and the socket of its docker filter
// (SweepFiles). The engine's own teardown does exactly this when its Run or
// session End returns ("teardown on every path"); Sweep is the desktop's
// second pass for the paths the engine cannot see: the app quitting while a
// run is still in flight, when the bounded wait for the engine's teardown
// ran out, and the app killed mid-run, which runs no teardown at all and
// leaves the run's secrets file, staged inputs and knowledge pages in the
// clear. Each step is best-effort and absent objects are not errors, so
// sweeping a run the engine already tore down is a no-op.
//
// The files go whatever the docker steps answered: a daemon that is not
// running must not keep a secrets file on disk. Nothing else is touched:
// the run's output folder with its receipts, the QuickAgent's install
// record and cached release, its profile and every docker image are the
// person's or the QuickAgent's, not the run's. (A profile the run was to
// remove when it completed goes when its record is forgotten, by what the
// record says: settleProfile.)
//
// Not reached, because nothing names them by the run's id: a container the
// gateway started where there is no docker filter to label it (macOS and
// Windows), the gateway's anonymous volume, the opened copy of an attached
// knowledge pack (the
// store's own sweep, an hour on), a Knowledge Creator build's work
// directory, and the filter's fallback socket directory under the system's
// temp directory.
func Sweep(ctx context.Context, runID string) error {
	// The id becomes a docker label, object names and a path element. An
	// empty one would match every run on the machine, the CLI's included.
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("sweep: %q is not a run id", runID)
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
	// is not an error. A list that fails is: the fixed names still go, and
	// the error keeps the run's record for the next start to ask again.
	nets, err := runNetworks(ctx, d, runID, label)
	if err != nil {
		errs = append(errs, fmt.Errorf("list networks: %w", err))
	}
	for _, n := range nets {
		if err := d.NetworkRemove(ctx, n); err != nil {
			errs = append(errs, fmt.Errorf("remove network %s: %w", n, err))
		}
	}
	home, err := Home()
	if err != nil {
		errs = append(errs, fmt.Errorf("runner home: %w", err))
	} else if err := SweepFiles(home, runID); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// runNetworks are the networks a run can make: the engine's names for them
// (agentry-cli pkg/runner/engine: networkName, proxyEgressNetwork, webNetwork,
// relayNetworks, browserNetwork), which are the run network, the model proxy's
// egress network under a sandboxed runtime, the web tools' network, the relays'
// pair and the browser's own; then every other network carrying the run's
// label, which finds those a step names itself, such as an input source's. The
// names are the engine's own and not exported, so they are restated here as
// engine.SweepAbandoned restates them, held to the engine by
// TestSweepNamesEveryNetworkTheEngineMakes, and are returned with the error
// when docker cannot list.
func runNetworks(ctx context.Context, d dockerx.Client, runID, label string) ([]string, error) {
	p := "agentry-" + runID
	names := []string{p + "-net", p + "-egress", p + "-web", p + "-relays", p + "-relays-out", p + "-browser"}
	labelled, err := d.NetworksByLabel(ctx, label)
	if err != nil {
		return names, err
	}
	for _, n := range labelled {
		if !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	return names, nil
}

// SweepFiles is Sweep's file half, which needs no docker: it removes what a
// run keeps under the runner home while it is live. That is its scratch
// directory, <release directory>/runs/<run id>, which holds the workspace
// copy, the staged inputs, the copies of the attached knowledge packs and
// the rendered configuration with secrets.env; the podman API service a
// run under a policy runtime started on podman, which the directory's
// record names and which outlives a killed run (stopLeftPodmanService,
// before the record goes with the directory); and the sockets of its
// docker filter and of that service, <runner home>/s/<run id>.sock and
// <run id>-podman.sock. The app also calls it when a run's engine has
// returned, for what the engine's own teardown could not delete.
//
// The scratch directory is found by the run id under every release
// directory, not from a path the record carries: the record is written
// before the release is resolved, and a kill between the two must still
// leave the next start enough to find it. A run id belongs only to the
// process that minted it, so a directory of that name is that run's under
// whichever release it ran.
func SweepFiles(home, runID string) error {
	if !runIDPattern.MatchString(runID) {
		return fmt.Errorf("sweep: %q is not a run id", runID)
	}
	var errs []error
	for _, dir := range scratchDirs(home, runID) {
		stopLeftPodmanService(dir, home, runID)
		if err := removeTree(dir); err != nil {
			errs = append(errs, fmt.Errorf("remove the run's scratch directory: %w", err))
		}
	}
	if err := removal.Remove(filepath.Join(home, "s", runID+".sock"), false); err != nil {
		errs = append(errs, fmt.Errorf("remove the run's docker filter socket: %w", err))
	}
	if err := removal.Remove(filepath.Join(home, "s", runID+"-podman.sock"), false); err != nil {
		errs = append(errs, fmt.Errorf("remove the run's podman service socket: %w", err))
	}
	return errors.Join(errs...)
}

// removeTree deletes a run's scratch directory. The run's tools write into
// the workspace in it and can leave a directory there that its owner may
// not write; when the delete is refused, every directory under the scratch
// directory is made writable and the delete tried once more, rather than
// the record of the run standing for ever. What another user owns (a tools
// container killed before it handed its files back) is beyond that, and
// stays with the record.
//
// Nothing outside the runs directory is changed. The delete removes a link
// rather than what it points at. The second try is made only for a real
// directory, and walks through an os.Root opened on the runs directory
// above it, which a link swapped in afterwards cannot lead out of (it could
// lead into another run's directory beside this one, whose directories
// would be made writable to their owner); no test holds it to that last
// part, since the swap is a race. A variable so a
// test can make the delete fail on any platform.
var removeTree = func(dir string) error {
	err := removal.Remove(dir, true)
	if err == nil {
		return nil
	}
	root, rerr := os.OpenRoot(filepath.Dir(dir))
	if rerr != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	base := filepath.Base(dir)
	// Not a link standing in the scratch directory's place: the walk would
	// follow one that stays inside runs, into another run's directory.
	if fi, lerr := root.Lstat(base); lerr != nil || !fi.IsDir() {
		return err
	}
	_ = fs.WalkDir(root.FS(), base, func(p string, d fs.DirEntry, werr error) error {
		if werr == nil && d.IsDir() {
			_ = root.Chmod(p, 0o700)
		}
		return nil
	})
	return removal.Remove(dir, true)
}

// SetRemoveTreeForTest replaces the delete of a run's scratch directory for
// the test's life.
func SetRemoveTreeForTest(t interface{ Cleanup(func()) }, f func(dir string) error) {
	prev := removeTree
	removeTree = f
	t.Cleanup(func() { removeTree = prev })
}

// scratchDirs lists runs/<run id> under every release directory of the
// runner home: <home>/agents/<namespace>/<name>/<release>, the one layout
// the engine's RunRoot has for a release from a registry, a local install
// and a built-in alike (installdir.ReleaseDir). Only real directories are
// descended: a link standing for a namespace, a QuickAgent, a release or
// its runs directory is left alone, so nothing outside the runner home is
// reached through one.
func scratchDirs(home, runID string) []string {
	sub := func(dir string) []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var dirs []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(dir, e.Name()))
			}
		}
		return dirs
	}
	var out []string
	for _, ns := range sub(installdir.AgentsRoot(home)) {
		for _, agent := range sub(ns) {
			for _, release := range sub(agent) {
				runs := filepath.Join(release, "runs")
				if fi, err := os.Lstat(runs); err != nil || !fi.IsDir() {
					continue
				}
				scratch := filepath.Join(runs, runID)
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
