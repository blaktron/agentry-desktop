package run

// A review's staged copy: the Examiner's pre-step copies the QuickAgent
// into the system's temp directory, outside the runner home, so the run's
// record names the copy and forgetting the record removes it; a copy no
// record names goes once it is a day old.

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// stageIn makes a staged copy in the system's temp directory, as the
// pre-step does, with a file in it.
func stageIn(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", StagePrefix+"*")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "package"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package", "AGENT.md"), []byte("the QuickAgent's text"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// isolateTemp gives the test a system temp directory of its own.
func isolateTemp(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	return tmp
}

// A review that returns forgets its record, and its staged copy with it,
// whether or not the review's own Cleanup ran: the copy never outlives the
// record.
func TestForgettingARecordRemovesTheStagedCopyItNames(t *testing.T) {
	isolateTemp(t)
	dir := filepath.Join(t.TempDir(), InFlightDirName)
	if err := RecordInFlight(dir, "5ea10001"); err != nil {
		t.Fatal(err)
	}
	stage := stageIn(t)
	if err := RecordStage(dir, "5ea10001", stage); err != nil {
		t.Fatal(err)
	}
	rec, err := readRecord(dir, "5ea10001")
	if err != nil || rec.Stage != stage || rec.PID != os.Getpid() {
		t.Fatalf("record %+v %v", rec, err)
	}
	if err := ForgetInFlight(dir, "5ea10001"); err != nil {
		t.Fatal(err)
	}
	if exists(stage) || exists(recordPath(dir, "5ea10001")) {
		t.Fatal("the staged copy or the record outlived the forget")
	}
}

// A record is never a way to delete something else: RecordStage refuses a
// path that is not a staged copy in the system's temp directory, and a
// record that names one anyway (or a link where the copy was) has the path
// left as it is when the record goes.
func TestARecordedStageOutsideTheTempDirectoryIsLeftAlone(t *testing.T) {
	tmp := isolateTemp(t)
	dir := filepath.Join(t.TempDir(), InFlightDirName)
	precious := t.TempDir()
	if err := os.WriteFile(filepath.Join(precious, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RecordInFlight(dir, "5ea10002"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{precious, filepath.Join(tmp, "other-dir"), filepath.Join(tmp, StagePrefix+"x", ".."), "relative/" + StagePrefix + "x"} {
		if err := RecordStage(dir, "5ea10002", p); err == nil {
			t.Fatalf("RecordStage took %q", p)
		}
	}
	// A record written by hand that names the folder.
	saveRecordForTest(t, dir, inFlight{RunID: "5ea10002", PID: os.Getpid(), Stage: precious})
	if err := ForgetInFlight(dir, "5ea10002"); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(precious, "keep")) {
		t.Fatal("a recorded path outside the temp directory was removed")
	}
	// A link standing where a staged copy was named is left too, and so is
	// what it points at.
	link := filepath.Join(tmp, StagePrefix+"link")
	if err := os.Symlink(precious, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if err := RecordInFlight(dir, "5ea10003"); err != nil {
		t.Fatal(err)
	}
	if err := RecordStage(dir, "5ea10003", link); err != nil {
		t.Fatal(err)
	}
	if err := ForgetInFlight(dir, "5ea10003"); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(precious, "keep")) {
		t.Fatal("a link named as a staged copy was followed")
	}
}

// A review the app died inside is swept at the next start by its run id,
// its staged copy with it; a live review's copy is left.
func TestTheStartSweepRemovesADeadReviewsStagedCopy(t *testing.T) {
	home := isolateRunEnv(t)
	isolateTemp(t)
	withFakeDocker(t, &dockerx.Fake{})
	dir := filepath.Join(home, InFlightDirName)
	pid, start := deadPID(t)
	dead, live := stageIn(t), stageIn(t)
	saveRecordForTest(t, dir, inFlight{RunID: "dead0005", PID: pid, Start: start, Stage: dead})
	if err := RecordInFlight(dir, "5e1f0006"); err != nil {
		t.Fatal(err)
	}
	if err := RecordStage(dir, "5e1f0006", live); err != nil {
		t.Fatal(err)
	}
	swept, err := SweepOrphaned(context.Background(), dir, Sweep)
	if err != nil {
		t.Fatal(err)
	}
	if len(swept) != 1 || swept[0] != "dead0005" || exists(dead) || !exists(live) {
		t.Fatalf("swept %v; dead copy kept %v, live copy kept %v", swept, exists(dead), exists(live))
	}
	// At quit this process's own record goes too, with its copy.
	if _, err := SweepAtQuit(context.Background(), dir, Sweep); err != nil {
		t.Fatal(err)
	}
	if exists(live) {
		t.Fatal("the quit sweep left this process's staged copy")
	}
}

// A copy no record names (a pre-step the app died inside) goes once it is
// a day old; a newer one, a link and anything not named as a staged copy
// stay.
func TestSweepStaleStagesRemovesOnlyOldStagedCopies(t *testing.T) {
	tmp := isolateTemp(t)
	old, fresh := stageIn(t), stageIn(t)
	other := filepath.Join(tmp, "not-a-stage")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	link := filepath.Join(tmp, StagePrefix+"link")
	_ = os.Symlink(target, link)
	then := time.Now().Add(-25 * time.Hour)
	for _, p := range []string{old, other, target} {
		if err := os.Chtimes(p, then, then); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := SweepStaleStages(StaleStageAge)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != filepath.Base(old) || exists(old) || !exists(fresh) || !exists(other) || !exists(target) {
		t.Fatalf("removed %v; fresh kept %v, other kept %v, link target kept %v", removed, exists(fresh), exists(other), exists(target))
	}
}

// saveRecordForTest writes a record as the app would, with any fields.
func saveRecordForTest(t *testing.T, dir string, rec inFlight) {
	t.Helper()
	if err := saveRecord(dir, rec); err != nil {
		t.Fatal(err)
	}
}
