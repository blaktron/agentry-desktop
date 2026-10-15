package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/mcpinstall"
	"github.com/blaktron/agentry-cli/pkg/runner/owner"
	"github.com/blaktron/agentry-cli/pkg/runner/profile"
)

// profileOf saves the fixture release's profile under home, holding a
// secret value, with its MCP server store beside it, and returns the
// profile's directory and the release's run root.
func saveFixtureProfile(t *testing.T, home string, keep bool) (profDir, runRoot string) {
	t.Helper()
	profDir = profile.Dir(home, "acme", "reporter", "1.0.0", "0123456789ab")
	if err := profile.New("acme/reporter@1.0.0", "sha256:0123456789ab", keep, map[string]string{"API_TOKEN": "sekret-value-1234"}).Save(profDir); err != nil {
		t.Fatal(err)
	}
	store := mcpinstall.StoreDir(profDir)
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, "server.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	runRoot = filepath.Join(home, "agents", "acme", "reporter", "1.0.0-0123456789ab")
	if err := os.MkdirAll(filepath.Join(runRoot, "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	return profDir, runRoot
}

// withRunsInUse makes the given runs, and no others, the ones whose run
// lock a live process holds in any release's run directories.
func withRunsInUse(t *testing.T, ids ...string) {
	t.Helper()
	old := runsInUse
	runsInUse = func(string) []string { return ids }
	t.Cleanup(func() { runsInUse = old })
}

// deadRecord records a run of a process that is gone, holding the
// fixture's profile with the run's keep decision.
func deadRecord(t *testing.T, dir, runID, profDir, runRoot string, keep bool) {
	t.Helper()
	pid, start := deadPID(t)
	writeRecord(t, dir, inFlight{RunID: runID, PID: pid, Start: start})
	recordProfile(t, dir, runID, profDir, runRoot, keep)
}

// recordProfile puts the fixture's profile, as it is now, on runID's record.
func recordProfile(t *testing.T, dir, runID, profDir, runRoot string, keep bool) {
	t.Helper()
	held, err := heldProfile(profDir, runRoot, keep)
	if err != nil || held == nil {
		t.Fatalf("held profile = %v, %v", held, err)
	}
	if err := RecordProfile(dir, runID, *held); err != nil {
		t.Fatal(err)
	}
}

// A killed app's run whose profile was not to be kept leaves it, secret
// values and MCP server store with it; the next start's sweep removes it
// as the run's completion would have, and the record with it.
func TestTheStartupSweepRemovesAKilledRunsUnkeptProfile(t *testing.T) {
	home := isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{})
	withRunsInUse(t)
	dir := filepath.Join(home, InFlightDirName)
	profDir, runRoot := saveFixtureProfile(t, home, false)
	deadRecord(t, dir, "dead0001", profDir, runRoot, false)

	if _, err := SweepOrphaned(context.Background(), dir, Sweep); err != nil {
		t.Fatal(err)
	}
	if exists(profDir) {
		t.Fatal("a killed run's unkept profile outlived the startup sweep")
	}
	if exists(filepath.Join(dir, "dead0001.json")) {
		t.Fatal("the record outlived the sweep")
	}
}

// The quit path: this app's own run, whose engine outlasted the quit's
// wait, is swept at quit, its unkept profile with it, by the sweep of its
// records and by the forget of the run the app still had live.
func TestTheQuitPathRemovesThisAppsUnkeptProfile(t *testing.T) {
	for _, viaSweep := range []bool{true, false} {
		home := isolateRunEnv(t)
		withFakeDocker(t, &dockerx.Fake{})
		// The engine still runs, holding its own run's lock.
		withRunsInUse(t, "5e1f0004")
		dir := filepath.Join(home, InFlightDirName)
		profDir, runRoot := saveFixtureProfile(t, home, false)
		if err := RecordInFlight(dir, "5e1f0004"); err != nil {
			t.Fatal(err)
		}
		recordProfile(t, dir, "5e1f0004", profDir, runRoot, false)
		if viaSweep {
			if _, err := SweepAtQuit(context.Background(), dir, Sweep); err != nil {
				t.Fatal(err)
			}
		} else if err := ForgetInFlight(dir, "5e1f0004"); err != nil {
			t.Fatal(err)
		}
		if exists(profDir) {
			t.Fatalf("via sweep %v: the run's unkept profile outlived the quit", viaSweep)
		}
	}
}

// A profile the run was to keep is untouched, MCP server store included,
// whether the stored flag kept it or the run setup's tick over a profile
// saved with Keep off did.
func TestAKeptProfileOutlivesAKilledRun(t *testing.T) {
	for _, stored := range []bool{true, false} {
		home := isolateRunEnv(t)
		withFakeDocker(t, &dockerx.Fake{})
		withRunsInUse(t)
		dir := filepath.Join(home, InFlightDirName)
		profDir, runRoot := saveFixtureProfile(t, home, stored)
		deadRecord(t, dir, "dead0001", profDir, runRoot, true)
		if _, err := SweepOrphaned(context.Background(), dir, Sweep); err != nil {
			t.Fatal(err)
		}
		if !exists(filepath.Join(profDir, "profile.json")) || !exists(filepath.Join(mcpinstall.StoreDir(profDir), "server.json")) {
			t.Fatalf("stored keep %v: the sweep removed a profile the run was to keep", stored)
		}
	}
}

// A record with no profile is swept as any record is, and the release's
// profile left alone: nothing says what the run decided.
func TestARecordWithoutAProfileLeavesTheProfileAlone(t *testing.T) {
	home := isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{})
	withRunsInUse(t)
	dir := filepath.Join(home, InFlightDirName)
	profDir, _ := saveFixtureProfile(t, home, false)
	pid, start := deadPID(t)
	writeRecord(t, dir, inFlight{RunID: "dead0001", PID: pid, Start: start})
	swept, err := SweepOrphaned(context.Background(), dir, Sweep)
	if err != nil || len(swept) != 1 {
		t.Fatalf("swept %v, %v", swept, err)
	}
	if !exists(filepath.Join(profDir, "profile.json")) {
		t.Fatal("a record without the profile removed it")
	}
}

// A profile a live run of the same release may be using is left to that
// run: one holding a run lock in the release's run directories (the CLI's
// or another desktop's), and another live app's record that names it. The
// dead run's record still goes.
func TestAProfileALiveRunOfTheReleaseUsesIsLeft(t *testing.T) {
	t.Run("a run lock", func(t *testing.T) {
		home := isolateRunEnv(t)
		withFakeDocker(t, &dockerx.Fake{})
		withRunsInUse(t, "c11c0003")
		dir := filepath.Join(home, InFlightDirName)
		profDir, runRoot := saveFixtureProfile(t, home, false)
		deadRecord(t, dir, "dead0001", profDir, runRoot, false)
		if _, err := SweepOrphaned(context.Background(), dir, Sweep); err != nil {
			t.Fatal(err)
		}
		if !exists(filepath.Join(profDir, "profile.json")) {
			t.Fatal("the sweep removed a profile a live run of the release holds")
		}
		if exists(filepath.Join(dir, "dead0001.json")) {
			t.Fatal("the dead run's record stayed")
		}
	})
	t.Run("another app's record", func(t *testing.T) {
		home := isolateRunEnv(t)
		withFakeDocker(t, &dockerx.Fake{})
		withRunsInUse(t)
		dir := filepath.Join(home, InFlightDirName)
		profDir, runRoot := saveFixtureProfile(t, home, false)
		parent := os.Getppid()
		pstart, _ := owner.StartTime(parent)
		writeRecord(t, dir, inFlight{RunID: "a11e0002", PID: parent, Start: pstart})
		recordProfile(t, dir, "a11e0002", profDir, runRoot, false)
		deadRecord(t, dir, "dead0001", profDir, runRoot, false)
		if _, err := SweepOrphaned(context.Background(), dir, Sweep); err != nil {
			t.Fatal(err)
		}
		if !exists(filepath.Join(profDir, "profile.json")) {
			t.Fatal("the sweep removed a profile another live app's run holds")
		}
	})
}

// A profile saved again after the run read it (the guided form, or a run
// that completed and kept it) is the newer writer's, and is left.
func TestAProfileSavedAgainSinceIsLeft(t *testing.T) {
	home := isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{})
	withRunsInUse(t)
	dir := filepath.Join(home, InFlightDirName)
	profDir, runRoot := saveFixtureProfile(t, home, false)
	deadRecord(t, dir, "dead0001", profDir, runRoot, false)
	if err := profile.New("acme/reporter@1.0.0", "sha256:0123456789ab", false, map[string]string{"API_TOKEN": "a-newer-value"}).Save(profDir); err != nil {
		t.Fatal(err)
	}
	if _, err := SweepOrphaned(context.Background(), dir, Sweep); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(profDir, "profile.json")) {
		t.Fatal("the sweep removed a profile saved after the run read it")
	}
}

// A record is never a way to delete something else: a profile directory
// that is not a release's under the runner home's profiles is left,
// however the record names it.
func TestARecordNamingAnotherDirectoryRemovesNothing(t *testing.T) {
	home := isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{})
	withRunsInUse(t)
	dir := filepath.Join(home, InFlightDirName)
	_, runRoot := saveFixtureProfile(t, home, false)
	for i, elsewhere := range []string{
		t.TempDir(),
		filepath.Join(home, "profiles", "acme"),
		filepath.Join(home, "profiles", "acme", "reporter", "1.0.0-0123456789ab", "servers"),
	} {
		if err := profile.New("x/y@1.0.0", "sha256:00", false, nil).Save(elsewhere); err != nil {
			t.Fatal(err)
		}
		id := []string{"dead0001", "dead0002", "dead0003"}[i]
		deadRecord(t, dir, id, elsewhere, runRoot, false)
		if _, err := SweepOrphaned(context.Background(), dir, Sweep); err != nil {
			t.Fatal(err)
		}
		if !exists(filepath.Join(elsewhere, "profile.json")) {
			t.Fatalf("a record naming %s removed it", elsewhere)
		}
	}
}

// A release with no profile gives a run none to hold, and none is recorded.
func TestARunWithNoProfileHoldsNone(t *testing.T) {
	held, err := heldProfile(filepath.Join(t.TempDir(), "profiles", "a", "b", "c"), "", false)
	if err != nil || held != nil {
		t.Fatalf("held = %+v, %v; want none", held, err)
	}
}

// A link standing for any level of a profile directory, or for the
// profiles directory itself, removes nothing through it.
func TestARecordRemovesNothingThroughALink(t *testing.T) {
	home := isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{})
	withRunsInUse(t)
	dir := filepath.Join(home, InFlightDirName)
	profDir, runRoot := saveFixtureProfile(t, home, false)
	elsewhere := filepath.Join(t.TempDir(), "1.0.0-0123456789ab")
	if err := os.Rename(profDir, elsewhere); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, profDir); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	deadRecord(t, dir, "dead0001", profDir, runRoot, false)
	if _, err := SweepOrphaned(context.Background(), dir, Sweep); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(elsewhere, "profile.json")) {
		t.Fatal("the sweep removed a profile through a link")
	}
}

// A run prepared with an unkept profile puts the profile, its release's
// run root and the run's own keep decision on its record before the engine
// starts. Had the app been killed mid-run, that record alone takes the
// profile at the next start: the run's own completion is undone here (the
// profile it removed is put back as the run found it, the record's owner
// made a dead process) and the startup sweep removes it.
func TestARunRecordsItsProfileForTheStartupSweep(t *testing.T) {
	fr, regURL, _, home := runFixture(t)
	withRunsInUse(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	sig := resolveFixture(t, regURL)
	if _, err := saveProfileForSig(sig, map[string]string{"RUN_TEST_TOKEN": "v"}, false); err != nil {
		t.Fatal(err)
	}
	profDir := profile.Dir(home, "acme", "reporter", "1.0.0", installdir.DigestSuffix(fr.digest))
	held, err := os.ReadFile(filepath.Join(profDir, "profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, InFlightDirName)
	if err := RecordInFlight(dir, "ab12cd34"); err != nil {
		t.Fatal(err)
	}
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	req.RunID, req.InFlightDir = "ab12cd34", dir
	if _, err := Run(context.Background(), req, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	rec, err := readRecord(dir, "ab12cd34")
	if err != nil {
		t.Fatal(err)
	}
	want := installdir.ReleaseDir(home, "acme", "reporter", "1.0.0", fr.digest)
	if rec.Profile == nil || rec.Profile.Dir != profDir || rec.Profile.Keep || rec.Profile.RunRoot != want {
		t.Fatalf("record's profile = %+v, want %s, run root %s, not kept", rec.Profile, profDir, want)
	}

	if err := os.MkdirAll(profDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "profile.json"), held, 0o600); err != nil {
		t.Fatal(err)
	}
	pid, start := deadPID(t)
	rec.PID, rec.Start = pid, start
	if err := saveRecord(dir, rec); err != nil {
		t.Fatal(err)
	}
	if _, err := SweepOrphaned(context.Background(), dir, Sweep); err != nil {
		t.Fatal(err)
	}
	if exists(profDir) {
		t.Fatal("the startup sweep left the killed run's unkept profile")
	}
}

// The record carries the run's own keep decision, the one its completion
// applies: the run setup's tick over the stored flag in either direction,
// and the stored flag when nothing was ticked.
func TestARunRecordsItsOwnKeepDecision(t *testing.T) {
	yes, no := true, false
	for _, c := range []struct {
		name   string
		stored bool
		tick   *bool
		want   bool
	}{
		{"kept by the stored flag", true, nil, true},
		{"kept by the tick over a profile saved with Keep off", false, &yes, true},
		{"removed by the tick over a profile saved with Keep on", true, &no, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			fr, regURL, _, home := runFixture(t)
			withRunsInUse(t)
			t.Setenv("RUN_TEST_TOKEN", "env-secret")
			sig := resolveFixture(t, regURL)
			if _, err := saveProfileForSig(sig, map[string]string{"RUN_TEST_TOKEN": "v"}, c.stored); err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(home, InFlightDirName)
			if err := RecordInFlight(dir, "ab12cd34"); err != nil {
				t.Fatal(err)
			}
			req := request(regURL, filepath.Join(t.TempDir(), "out"))
			req.RunID, req.InFlightDir, req.KeepProfile = "ab12cd34", dir, c.tick
			if _, err := Run(context.Background(), req, &strings.Builder{}); err != nil {
				t.Fatal(err)
			}
			rec, err := readRecord(dir, "ab12cd34")
			if err != nil || rec.Profile == nil {
				t.Fatalf("record = %+v, %v", rec, err)
			}
			if rec.Profile.Keep != c.want {
				t.Fatalf("recorded keep = %v, want %v", rec.Profile.Keep, c.want)
			}
			profDir := profile.Dir(home, "acme", "reporter", "1.0.0", installdir.DigestSuffix(fr.digest))
			if exists(filepath.Join(profDir, "profile.json")) != c.want {
				t.Fatalf("the completing run's choice disagrees with the record: profile present = %v", !c.want)
			}
		})
	}
}
