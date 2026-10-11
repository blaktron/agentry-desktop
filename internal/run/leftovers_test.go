package run

// The leftovers sweep (agentry-desktop#463): what a run an app was killed in
// the middle of left on this computer, swept by its record at the next start
// and at quit. The run here is the real engine's, on the CLI's dockerx.Fake
// and the fake registry, stopped where a kill would stop it: every directory,
// file and docker object the sweep must reach is one the engine itself made.

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/profile"
)

// abandoned is a run stopped mid-flight, as a killed app leaves one.
type abandoned struct {
	runID                             string
	home, outDir, installDir, scratch string
	fake                              *dockerx.Fake
	// calls is how many docker calls the run had made when it was
	// abandoned: what the sweep did is everything after.
	calls int
}

// abandonRun starts the fixture release under runID with a kept profile and
// blocks its agent, so the run stands exactly as a SIGKILL would leave it:
// its scratch directory with the secrets file, its networks and containers.
// The engine is released, and its own teardown runs, when the test ends.
func abandonRun(t *testing.T, runID string) abandoned {
	t.Helper()
	_, regURL, fake, home := runFixture(t)
	if _, err := SaveProfile(context.Background(), regURL, "acme/reporter@1.0.0", map[string]string{"RUN_TEST_TOKEN": "the-secret-value"}, true); err != nil {
		t.Fatal(err)
	}
	agentUp := make(chan struct{})
	fake.Foreground["-agent"] = &dockerx.FakeForeground{Block: true}
	fake.OnForeground = func(r dockerx.RunSpec) {
		if strings.HasSuffix(r.Name, "-agent") {
			close(agentUp)
		}
	}
	outDir := filepath.Join(t.TempDir(), "out")
	req := request(regURL, outDir)
	req.RunID = runID
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = Run(ctx, req, &strings.Builder{})
	}()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-agentUp:
	case <-done:
		t.Fatal("the run ended before its agent started")
	case <-time.After(30 * time.Second):
		t.Fatal("the run never started its agent")
	}
	return abandonedAt(t, home, fake, "reporter", runID, outDir)
}

// abandonedAt records a live run's state as the sweep will find it: where
// its scratch directory is, and what the daemon would answer a sweep with,
// the run's containers and volumes by the label the engine gave them.
func abandonedAt(t *testing.T, home string, fake *dockerx.Fake, name, runID, outDir string) abandoned {
	t.Helper()
	releases, err := filepath.Glob(filepath.Join(home, "agents", "acme", name, "*"))
	if err != nil || len(releases) != 1 {
		t.Fatalf("the release directory of acme/%s: %v %v", name, releases, err)
	}
	a := abandoned{runID: runID, home: home, fake: fake, outDir: outDir, installDir: releases[0], scratch: filepath.Join(releases[0], "runs", runID)}
	var containers, volumes []string
	for _, c := range fake.Calls {
		switch {
		case (c.Op == "RunDetached" || c.Op == "RunForeground") && c.Run.Labels["agentry.run"] == runID:
			containers = append(containers, c.Arg)
		case c.Op == "VolumeCreate":
			volumes = append(volumes, c.Arg)
		}
	}
	fake.ByLabel = map[string][]string{"agentry.run=" + runID: containers}
	fake.LeftoverVolumes = volumes
	a.calls = len(fake.Calls)
	return a
}

// made lists what the run created through op before it was abandoned.
func (a abandoned) made(op string) []string {
	var out []string
	for _, c := range a.fake.Calls[:a.calls] {
		if c.Op == op {
			name, _, _ := strings.Cut(c.Arg, "(")
			out = append(out, name)
		}
	}
	return out
}

// removed lists what the daemon was asked to remove through op since the
// run was abandoned (or since the mark was moved, see sweepsFromHere).
func (a abandoned) removed(op string) []string {
	var out []string
	for _, c := range a.fake.Calls[a.calls:] {
		if c.Op == op {
			out = append(out, c.Arg)
		}
	}
	return out
}

// sweepsFromHere moves the mark, so that removed counts only what follows:
// for a run the engine has torn down itself, whose own removals must not
// be read as the sweep's.
func (a *abandoned) sweepsFromHere() { a.calls = len(a.fake.Calls) }

// secretsLeft walks the runner home for anything holding the run's secret.
func secretsLeft(t *testing.T, home string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(filepath.Join(home, "agents"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), "the-secret-value") {
			found = append(found, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// assertNothingLeft is plan §5.4's rule for a run that is over: nothing of
// it on this computer but its output folder, and everything that is the
// QuickAgent's rather than the run's (its install record and cached
// release, its kept profile) untouched.
func (a abandoned) assertNothingLeft(t *testing.T) {
	t.Helper()
	runID := a.runID
	if exists(a.scratch) {
		t.Errorf("the run's scratch directory is still there: %s", a.scratch)
	}
	if left := secretsLeft(t, a.home); len(left) != 0 {
		t.Errorf("the run's secret is still on disk in %v", left)
	}
	if sock := filepath.Join(a.home, "s", runID+".sock"); exists(sock) {
		t.Errorf("the run's docker filter socket is still there: %s", sock)
	}
	for _, n := range a.made("NetworkCreate") {
		if !slices.Contains(a.removed("NetworkRemove"), n) {
			t.Errorf("network %s, which the run made, was not removed", n)
		}
	}
	// Every network a run can make, whether this one did: the engine makes
	// the last four only for a sandboxed proxy, a relay or the web tools.
	for _, suffix := range []string{"-net", "-egress", "-web", "-relays", "-relays-out"} {
		if n := "agentry-" + runID + suffix; !slices.Contains(a.removed("NetworkRemove"), n) {
			t.Errorf("network %s was not asked for", n)
		}
	}
	for _, c := range a.fake.ByLabel["agentry.run="+runID] {
		if !slices.Contains(a.removed("RemoveForce"), c) {
			t.Errorf("container %s was not removed", c)
		}
	}
	for _, v := range a.fake.LeftoverVolumes {
		if !slices.Contains(a.removed("VolumeRemove"), v) {
			t.Errorf("volume %s was not removed", v)
		}
	}
	for _, keep := range []string{
		filepath.Join(a.installDir, "install.json"),
		filepath.Join(a.installDir, "workspace", "agent.yaml"),
		filepath.Join(a.installDir, "plan"),
		filepath.Join(a.outDir, "receipts"),
	} {
		if !exists(keep) {
			t.Errorf("the sweep removed %s, which is not the run's to lose", keep)
		}
	}
}

// keptProfile is the fixture release's kept profile, saved by abandonRun.
func (a abandoned) keptProfile() string {
	return filepath.Join(profile.Dir(a.home, "acme", "reporter", "1.0.0", strings.TrimPrefix(filepath.Base(a.installDir), "1.0.0-")), "profile.json")
}

// The premise, and the fix: a run abandoned as a kill leaves it has its
// secrets file in the clear under the runner home, and the start-up sweep
// leaves nothing of the run but its output folder.
func TestTheStartupSweepLeavesNothingOfAKilledAppsRun(t *testing.T) {
	const runID = "dead0001"
	a := abandonRun(t, runID)
	if len(a.fake.ByLabel["agentry.run="+runID]) == 0 || len(a.made("NetworkCreate")) == 0 {
		t.Fatalf("the abandoned run made no containers or networks: %v", a.fake.Ops())
	}
	if left := secretsLeft(t, a.home); len(left) != 1 || left[0] != filepath.Join(a.scratch, "cfg", "gateway", "secrets.env") {
		t.Fatalf("the abandoned run's secrets file is not where the sweep looks: %v", left)
	}

	dir := filepath.Join(a.home, InFlightDirName)
	pid, start := deadPID(t)
	writeRecord(t, dir, inFlight{RunID: runID, PID: pid, Start: start})

	swept, err := SweepOrphaned(context.Background(), dir, Sweep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(swept, ",") != runID {
		t.Fatalf("swept %v, want [%s]", swept, runID)
	}
	a.assertNothingLeft(t)
	if exists(filepath.Join(dir, runID+".json")) {
		t.Error("the swept run's record was kept")
	}
	if !exists(a.keptProfile()) {
		t.Error("the sweep removed the QuickAgent's kept profile")
	}
}

// The same run, left by this app's own teardown rather than by a kill (an
// engine that hung past the quit's wait): the quit sweep takes this
// process's own record, and leaves as little.
func TestTheQuitSweepLeavesNothingOfThisAppsRun(t *testing.T) {
	const runID = "5e1f0004"
	a := abandonRun(t, runID)
	dir := filepath.Join(a.home, InFlightDirName)
	if err := RecordInFlight(dir, runID); err != nil {
		t.Fatal(err)
	}
	if swept, err := SweepOrphaned(context.Background(), dir, Sweep); err != nil || len(swept) != 0 {
		t.Fatalf("the start-up sweep took a live process's run: %v %v", swept, err)
	}
	if !exists(a.scratch) {
		t.Fatal("the start-up sweep removed a live run's scratch directory")
	}

	swept, err := SweepAtQuit(context.Background(), dir, Sweep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(swept, ",") != runID {
		t.Fatalf("swept %v, want [%s]", swept, runID)
	}
	a.assertNothingLeft(t)
	if exists(filepath.Join(dir, runID+".json")) {
		t.Error("the swept run's record was kept")
	}
	if !exists(a.keptProfile()) {
		t.Error("the sweep removed the QuickAgent's kept profile")
	}
}

// downDocker is a daemon that is not running: every call the sweep makes
// fails.
type downDocker struct{ *dockerx.Fake }

var errDockerDown = errors.New("cannot connect to the Docker daemon")

func (downDocker) PSByLabel(context.Context, string) ([]string, error) { return nil, errDockerDown }
func (downDocker) VolumesByLabel(context.Context, string) ([]string, error) {
	return nil, errDockerDown
}
func (downDocker) NetworksByLabel(context.Context, string) ([]string, error) {
	return nil, errDockerDown
}
func (downDocker) NetworkRemove(context.Context, string) error { return errDockerDown }

// A secrets file does not wait for Docker: with the daemon down at start the
// run's files go all the same, and the record stays so that the next start
// asks the daemon again about the containers and the networks.
func TestTheSweepRemovesTheRunsFilesWithDockerDown(t *testing.T) {
	const runID = "dead0001"
	a := abandonRun(t, runID)
	dir := filepath.Join(a.home, InFlightDirName)
	pid, start := deadPID(t)
	writeRecord(t, dir, inFlight{RunID: runID, PID: pid, Start: start})

	withFakeDocker(t, downDocker{a.fake})
	swept, err := SweepOrphaned(context.Background(), dir, Sweep)
	if !errors.Is(err, errDockerDown) || len(swept) != 0 {
		t.Fatalf("swept %v, err %v; want the daemon's failure reported", swept, err)
	}
	if exists(a.scratch) || len(secretsLeft(t, a.home)) != 0 {
		t.Error("the run's files waited for Docker")
	}
	if !exists(filepath.Join(dir, runID+".json")) {
		t.Error("the record went though the containers and networks were never asked about")
	}

	// The daemon is back at the next start: the rest goes, and the record.
	withFakeDocker(t, a.fake)
	swept, err = SweepOrphaned(context.Background(), dir, Sweep)
	if err != nil || strings.Join(swept, ",") != runID {
		t.Fatalf("second sweep: %v %v", swept, err)
	}
	a.assertNothingLeft(t)
	if exists(filepath.Join(dir, runID+".json")) {
		t.Error("the swept run's record was kept")
	}
}

// startChat starts a chat of the session fixture under runID and sends it
// one turn; the caller ends it. It returns the session, the fake docker,
// the runner home and the output folder.
func startChat(t *testing.T, runID string) (*Session, *dockerx.Fake, string, string) {
	t.Helper()
	regURL, fake := sessionFixture(t)
	outDir := filepath.Join(t.TempDir(), "out")
	s, err := StartSession(context.Background(), Request{
		RunID: runID, AcceptUnsigned: true, Registry: regURL, Ref: "acme/chatty@1.0.0", OutDir: outDir, MaxTurns: -1,
		Intake: map[string]string{"region": "apac"},
	}, &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for range s.Events() {
		}
	}()
	if err := s.Send(context.Background(), "hello"); err != nil {
		t.Fatal(err)
	}
	return s, fake, os.Getenv("AGENTRY_RUNNER_HOME"), outDir
}

// A chat the app was killed in the middle of: the same sweep, and its
// session store's volume with it.
func TestTheStartupSweepLeavesNothingOfAKilledAppsChat(t *testing.T) {
	const runID = "dead0002"
	s, fake, home, outDir := startChat(t, runID)
	t.Cleanup(func() { _, _ = s.End(context.Background(), false) })
	a := abandonedAt(t, home, fake, "chatty", runID, outDir)
	if !exists(a.scratch) || !slices.Contains(fake.LeftoverVolumes, "agentry-"+runID+"-session") {
		t.Fatalf("the abandoned chat has no scratch directory or no session volume: %v", fake.Ops())
	}

	dir := filepath.Join(home, InFlightDirName)
	pid, start := deadPID(t)
	writeRecord(t, dir, inFlight{RunID: runID, PID: pid, Start: start})
	swept, err := SweepOrphaned(context.Background(), dir, Sweep)
	if err != nil || strings.Join(swept, ",") != runID {
		t.Fatalf("swept %v, err %v", swept, err)
	}
	a.assertNothingLeft(t)
}

// Quitting with a chat started ends it without asking for a final result
// (RunAPI.shutdown calls this End, and TestQuittingWithAChatStartedEndsItThenSweeps
// holds it to that), which writes its receipts as far as it got; the quit
// sweep that follows then asks for everything a run can leave, finds
// nothing of this one but the output folder, and leaves that whole.
func TestAChatEndedByQuittingHasItsReceiptsAndLeavesNothing(t *testing.T) {
	const runID = "5e1f0005"
	s, fake, home, outDir := startChat(t, runID)
	dir := filepath.Join(home, InFlightDirName)
	if err := RecordInFlight(dir, runID); err != nil {
		t.Fatal(err)
	}
	a := abandonedAt(t, home, fake, "chatty", runID, outDir)

	res, err := s.End(context.Background(), false)
	if err != nil {
		t.Fatalf("End: %v", err)
	}
	tr, err := os.ReadFile(filepath.Join(res.ReceiptsDir, "transcript.md"))
	if err != nil || !strings.Contains(string(tr), "## User\n\nhello") {
		t.Fatalf("the chat's transcript was not written as far as it got: %v\n%s", err, tr)
	}
	if !exists(filepath.Join(outDir, "result.json")) {
		t.Error("the ended chat has no envelope")
	}
	if exists(a.scratch) {
		t.Error("the engine's End left the chat's scratch directory")
	}

	// From here on the removals are the sweep's, not the engine's teardown.
	a.sweepsFromHere()
	swept, err := SweepAtQuit(context.Background(), dir, Sweep)
	if err != nil || strings.Join(swept, ",") != runID {
		t.Fatalf("swept %v, err %v", swept, err)
	}
	a.assertNothingLeft(t)
	if exists(filepath.Join(dir, runID+".json")) {
		t.Error("the ended chat's record was kept")
	}
	for _, rel := range []string{"result.json", filepath.Join("receipts", "transcript.md")} {
		if !exists(filepath.Join(outDir, rel)) {
			t.Errorf("the sweep removed %s from the output folder", rel)
		}
	}
}
