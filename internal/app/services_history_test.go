package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/examiner"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The run history.

// freshConfig gives the test a config directory and a runner home of its
// own.
func freshConfig(t *testing.T) {
	t.Helper()
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	t.Setenv(policy.HomeEnv, t.TempDir())
	isolateDeliverables(t)
}

// setRemember turns Remember recent runs on or off in the test's settings.
func setRemember(t *testing.T, on bool) {
	t.Helper()
	if _, err := (&SettingsAPI{}).SetRememberRuns(on); err != nil {
		t.Fatal(err)
	}
}

func onlyEntry(t *testing.T) runhistory.Entry {
	t.Helper()
	runs, err := runhistory.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("the history holds %d entries, want 1: %+v", len(runs), runs)
	}
	return runs[0]
}

// A run on this computer is recorded from the engine's result as Go read it:
// its id, release, digest, times, how it ended and its folder, and nothing of
// its task, inputs, result or log.
func TestARunIsRecordedFromTheEnginesResult(t *testing.T) {
	freshConfig(t)
	out := t.TempDir()
	r := flowAPI(t, func(_ context.Context, req run.Request, logw io.Writer) (*run.Result, error) {
		_, _ = io.WriteString(logw, "a line of the log\n")
		return &run.Result{Status: "error", ExitCode: 3, RunID: req.RunID, Ref: "acme/agent@1.0.0", Digest: "sha256:d1",
			OutDir: out, ResultJSON: `"the agent's answer"`}, nil
	})
	before := time.Now().UTC().Add(-time.Second)
	if _, err := r.RunAgent("https://r.example/", "acme/agent@1.0.0", "the secret task", nil, "", nil, 0, -1, false,
		map[string]string{"since": "intake answer"}, false, false, nil, false, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	e := onlyEntry(t)
	if e.ID != "ab12cd34" || e.Kind != runhistory.KindRun || e.Where != runhistory.WhereHere || e.Registry != "https://r.example" ||
		e.Ref != "acme/agent@1.0.0" || e.Digest != "sha256:d1" || e.Outcome != runhistory.OutcomeError || e.ExitCode != 3 || e.Folder != out {
		t.Fatalf("entry %+v", e)
	}
	if e.StartedAt.Before(before) || e.EndedAt.Before(e.StartedAt) {
		t.Fatalf("times %v → %v", e.StartedAt, e.EndedAt)
	}
	path, _ := runhistory.Path()
	raw, _ := os.ReadFile(path)
	for _, s := range []string{"the secret task", "intake answer", "the agent's answer", "a line of the log"} {
		if strings.Contains(string(raw), s) {
			t.Errorf("runs.yaml holds %q", s)
		}
	}
}

// A run that is stopped is recorded as stopped, whatever its envelope says.
func TestAStoppedRunIsRecordedStopped(t *testing.T) {
	freshConfig(t)
	r := flowAPI(t, func(ctx context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		<-ctx.Done()
		return &run.Result{Status: "error", ExitCode: 7, RunID: req.RunID, OutDir: t.TempDir()}, nil
	})
	done := make(chan error, 1)
	go func() { _, err := runAgent(r); done <- err }()
	for r.StopRun() != nil {
		time.Sleep(5 * time.Millisecond)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if e := onlyEntry(t); e.Outcome != runhistory.OutcomeStopped {
		t.Fatalf("a stopped run's entry %+v", e)
	}
}

// Dismiss removes a run's card and does not forget a run that has ended: the
// history keeps it, and the result view reads its folder by its id.
func TestDismissKeepsAnEndedRunInTheHistory(t *testing.T) {
	freshConfig(t)
	out := t.TempDir()
	writeRunFile(t, filepath.Join(out, "outputs", "a.md"), "# A\n")
	r := flowAPI(t, func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		return &run.Result{Status: "ok", RunID: req.RunID, OutDir: out}, nil
	})
	v, err := runAgent(r)
	if err != nil {
		t.Fatal(err)
	}
	r.ForgetRun(v.RunID)
	got, err := r.RunOutputs(v.RunID)
	if err != nil || len(got.Files) != 1 {
		t.Fatalf("RunOutputs after Dismiss: %+v %v", got, err)
	}
	// A new process (the app started again) reads it through the history.
	again := &RunAPI{app: &App{ctx: context.Background()}}
	if got, err := again.RunOutputs(v.RunID); err != nil || len(got.Files) != 1 {
		t.Fatalf("RunOutputs in a new process: %+v %v", got, err)
	}
}

// The result-view bindings and the run page read an id this process produced
// or the history holds, and nothing else; a history entry's folder is
// checked again on every read, and one that is now a link, a file or gone is
// refused, never searched for.
func TestAHistoryIDIsReadOnlyThroughItsCheckedFolder(t *testing.T) {
	freshConfig(t)
	r := &RunAPI{app: &App{ctx: context.Background()}}
	opened := ""
	r.openFolder = func(dir string) error { opened = dir; return nil }
	for _, id := range []string{"", "nothere1", "../etc", "/etc", os.TempDir()} {
		if _, err := r.RunOutputs(id); err == nil {
			t.Errorf("RunOutputs(%q) read a folder", id)
		}
		if err := r.ShowRunFolder(id); err == nil {
			t.Errorf("ShowRunFolder(%q) opened a folder", id)
		}
		if _, err := r.RecordedRun(id); err == nil {
			t.Errorf("RecordedRun(%q) answered", id)
		}
	}
	base := t.TempDir()
	add := func(id, folder string) {
		t.Helper()
		if err := runhistory.Add(runhistory.Entry{ID: id, Kind: runhistory.KindRun, Where: runhistory.WhereHere, Outcome: "ok", EndedAt: time.Now(), Folder: folder}); err != nil {
			t.Fatal(err)
		}
	}
	good := filepath.Join(base, "good")
	writeRunFile(t, filepath.Join(good, "outputs", "x.txt"), "x")
	add("good0001", good)
	if _, err := r.RunOutputs("good0001"); err != nil {
		t.Fatalf("a recorded folder: %v", err)
	}
	if err := r.ShowRunFolder("good0001"); err != nil || opened != good {
		t.Fatalf("ShowRunFolder: %v, opened %q", err, opened)
	}

	gone := filepath.Join(base, "gone")
	add("gone0001", gone)
	if _, err := r.RunOutputs("gone0001"); !errors.Is(err, errRunFolderGone) {
		t.Fatalf("a folder that has gone: %v", err)
	}
	if v, err := r.RecordedRun("gone0001"); err != nil || v.FolderState != "gone" || v.ResultJSON != "" {
		t.Fatalf("RecordedRun of a gone folder: %+v %v", v, err)
	}

	file := filepath.Join(base, "file")
	writeRunFile(t, file, "not a folder")
	add("file0001", file)
	if _, err := r.RunOutputs("file0001"); err == nil {
		t.Fatal("a folder that is now a file was read")
	}

	if runtime.GOOS != "windows" {
		elsewhere := filepath.Join(base, "elsewhere")
		writeRunFile(t, filepath.Join(elsewhere, "outputs", "secret.txt"), "not the run's")
		link := filepath.Join(base, "link")
		if err := os.Symlink(elsewhere, link); err != nil {
			t.Fatal(err)
		}
		add("link0001", link)
		if _, err := r.RunOutputs("link0001"); err == nil {
			t.Fatal("a folder that is now a link was read")
		}
		opened = ""
		if err := r.ShowRunFolder("link0001"); err == nil || opened != "" {
			t.Fatalf("a folder that is now a link was opened: %v %q", err, opened)
		}
		if v, _ := r.RecordedRun("link0001"); v == nil || v.FolderState != "gone" {
			t.Fatalf("RecordedRun of a link: %+v", v)
		}
	}

	// With Remember recent runs off, the history answers for nothing.
	setRemember(t, false)
	add("good0002", good)
	if _, err := r.RunOutputs("good0002"); err == nil {
		t.Fatal("with the switch off, a history entry was read")
	}
}

// How a recorded run ended is the entry's, from the engine's result as Go
// read it; the folder's result.json, which the agent's tools can write,
// gives only its result, under the hostile rules the page renders every
// result with, and never through a link.
func TestHowARecordedRunEndedIsNeverReadFromResultJSON(t *testing.T) {
	freshConfig(t)
	out := t.TempDir()
	writeRunFile(t, filepath.Join(out, "result.json"), `{"agentry":"run/v1","status":"ok","exitCode":0,"result":"# All fine\n"}`)
	if err := runhistory.Add(runhistory.Entry{ID: "ab12cd34", Kind: runhistory.KindRun, Where: runhistory.WhereHere,
		Outcome: runhistory.OutcomeError, ExitCode: 4, EndedAt: time.Now(), Folder: out}); err != nil {
		t.Fatal(err)
	}
	r := &RunAPI{app: &App{ctx: context.Background()}}
	v, err := r.RecordedRun("ab12cd34")
	if err != nil {
		t.Fatal(err)
	}
	if v.Run.Outcome != runhistory.OutcomeError || v.Run.ExitCode != 4 || v.Run.Tone != "bad" {
		t.Fatalf("how it ended: %+v", v.Run)
	}
	if v.ResultJSON != `"# All fine\n"` || v.FolderState != "present" {
		t.Fatalf("the result: %q (%s)", v.ResultJSON, v.FolderState)
	}
	if runtime.GOOS != "windows" {
		elsewhere := filepath.Join(t.TempDir(), "x.json")
		writeRunFile(t, elsewhere, `{"result":"read through a link"}`)
		_ = os.Remove(filepath.Join(out, "result.json"))
		if err := os.Symlink(elsewhere, filepath.Join(out, "result.json")); err != nil {
			t.Fatal(err)
		}
		if v, _ := r.RecordedRun("ab12cd34"); v.ResultJSON != "" {
			t.Fatalf("a result.json that is a link was read: %q", v.ResultJSON)
		}
	}
}

// Remember recent runs off clears the history and stops its writes; Clear
// empties it; neither touches a run's folder.
func TestRememberOffClearsAndStopsWrites(t *testing.T) {
	freshConfig(t)
	out := t.TempDir()
	r := flowAPI(t, func(_ context.Context, req run.Request, _ io.Writer) (*run.Result, error) {
		return &run.Result{Status: "ok", RunID: req.RunID, OutDir: out}, nil
	})
	if _, err := runAgent(r); err != nil {
		t.Fatal(err)
	}
	s := &SettingsAPI{}
	if v, err := s.RunDefaults(); err != nil || !v.RememberRuns || v.RunHistoryCount != 1 {
		t.Fatalf("Run defaults with one run: %+v %v", v, err)
	}
	v, err := s.SetRememberRuns(false)
	if err != nil || v.RememberRuns || v.RunHistoryCount != 0 {
		t.Fatalf("turned off: %+v %v", v, err)
	}
	path, _ := runhistory.Path()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("runs.yaml with the switch off: %v", err)
	}
	if _, err := runAgent(r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a run wrote the history with the switch off: %v", err)
	}
	if rr, _ := r.RecentRuns("", 5); rr.Remember || len(rr.Runs) != 0 {
		t.Fatalf("Recent runs with the switch off: %+v", rr)
	}
	if _, err := s.SetRememberRuns(true); err != nil {
		t.Fatal(err)
	}
	if _, err := runAgent(r); err != nil {
		t.Fatal(err)
	}
	if v, err := s.ClearRunHistory(); err != nil || v.RunHistoryCount != 0 || !v.RememberRuns {
		t.Fatalf("Clear: %+v %v", v, err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("the run folder after Clear: %v", err)
	}
	st, _ := settings.Load()
	if !st.RememberRunsEnabled() {
		t.Fatal("Clear turned the switch off")
	}
}

// A built-in's job is recorded with its kind: ok, failed or stopped, and the
// folder its flow kept, or none.
func TestABuiltinsJobIsRecordedWithItsKind(t *testing.T) {
	freshConfig(t)
	r := flowAPI(t, nil)
	draftDir := t.TempDir()
	if err := r.asRun(jobDraft, "a draft by the Drafter", draftRunRef, "e", func(_ context.Context, id string, _ io.Writer) error {
		r.keepFolder(id, draftDir)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	e := onlyEntry(t)
	if e.Kind != runhistory.KindDraft || e.Outcome != runhistory.OutcomeOK || e.Folder != draftDir || e.Ref != "" {
		t.Fatalf("a draft's entry %+v", e)
	}
	_ = runhistory.Clear()
	_ = r.asRun(jobBuild, "a knowledge build", knowledgeRunRef, "e", func(context.Context, string, io.Writer) error {
		return errors.New("the build failed")
	})
	if e := onlyEntry(t); e.Kind != runhistory.KindKnowledge || e.Outcome != runhistory.OutcomeError || e.Folder != "" {
		t.Fatalf("a failed build's entry %+v", e)
	}
	_ = runhistory.Clear()
	_ = r.asRun(jobBuild, "a run of the Voice Writer", voiceRunRef, "e", func(ctx context.Context, _ string, _ io.Writer) error {
		r.live.cancel()
		<-ctx.Done()
		return ctx.Err()
	})
	if e := onlyEntry(t); e.Kind != runhistory.KindVoice || e.Outcome != runhistory.OutcomeStopped {
		t.Fatalf("a stopped voice build's entry %+v", e)
	}
	// A job that is not a built-in's (a setup) is not a run.
	_ = runhistory.Clear()
	_ = r.asRun(jobSetup, "a setup", "setup", "e", func(context.Context, string, io.Writer) error { return nil })
	if runs, _ := runhistory.List(); len(runs) != 0 {
		t.Fatalf("a setup was recorded: %+v", runs)
	}
}

// Recent runs merges this computer's history and the account's runs In
// Agentry by time, newest first; a run In Agentry this app downloaded is one
// row, the registry's, marked kept. Signed out it shows the history alone.
func TestRecentRunsMergesHereAndInAgentry(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	at := func(d time.Duration) *time.Time { x := now.Add(d); return &x }
	f := &hostedFake{enabled: true}
	f.run = &hostedrun.Run{ID: "22222222-2222-2222-2222-222222222222", Ref: "acme/reporter@1.0.0", Mode: "autonomous",
		Status: hostedrun.StatusFinished, Outcome: hostedrun.OutcomeSucceeded, CreatedAt: now.Add(-50 * time.Minute), FinishedAt: at(-40 * time.Minute)}
	f.others = []*hostedrun.Run{{ID: "33333333-3333-3333-3333-333333333333", Kind: hostedrun.KindKnowledgeBuild, Status: hostedrun.StatusRunning,
		CreatedAt: now.Add(-time.Minute)}}
	reg := f.start(t)
	signIn(t, reg)
	t.Setenv(policy.HomeEnv, t.TempDir())
	for _, e := range []runhistory.Entry{
		{ID: "aaaa0001", Kind: runhistory.KindRun, Where: runhistory.WhereHere, Ref: "acme/a@1.0.0", Outcome: "ok", EndedAt: now.Add(-10 * time.Minute), Folder: t.TempDir()},
		{ID: "aaaa0002", Kind: runhistory.KindDraft, Where: runhistory.WhereHere, Outcome: "error", EndedAt: now.Add(-60 * time.Minute)},
		{ID: f.run.ID, Kind: runhistory.KindRun, Where: runhistory.WhereAgentry, Outcome: "succeeded", EndedAt: now.Add(-40 * time.Minute), Folder: t.TempDir()},
	} {
		if err := runhistory.Add(e); err != nil {
			t.Fatal(err)
		}
	}
	r, _ := newRunAPI(t)
	v, err := r.RecentRuns(reg, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !v.SignedIn || v.HostedError != "" || !v.Remember {
		t.Fatalf("view %+v", v)
	}
	var got []string
	for _, row := range v.Runs {
		got = append(got, row.ID[:8]+"/"+row.Where+"/"+row.Kind+"/"+row.Source)
	}
	want := "33333333/agentry/knowledge/hosted aaaa0001/here/run/local 22222222/agentry/run/hosted aaaa0002/here/draft/local"
	if strings.Join(got, " ") != want {
		t.Fatalf("rows %v, want %v", got, want)
	}
	if !v.Runs[2].Kept || v.Runs[2].State != "done — pass" || v.Runs[2].Tone != "ok" || v.Runs[0].EndedAt != "" {
		t.Fatalf("the hosted rows %+v / %+v", v.Runs[2], v.Runs[0])
	}
	if v, _ := r.RecentRuns(reg, 2); len(v.Runs) != 2 {
		t.Fatalf("limit 2 gave %d", len(v.Runs))
	}

	// Signed out: the history alone, the downloaded run In Agentry included.
	v, err = r.RecentRuns("", 5)
	if err != nil || v.SignedIn || len(v.Runs) != 3 {
		t.Fatalf("signed out: %+v %v", v, err)
	}
	if row := v.Runs[1]; row.Where != runhistory.WhereAgentry || row.Source != "local" || row.State != "done — pass" {
		t.Fatalf("a downloaded run In Agentry, signed out: %+v", row)
	}
}

// A run In Agentry whose output is not on this computer offers Download
// output; the download goes into the folder Go chooses, the history keeps it
// by the registry's id, and the result view reads it by that id.
func TestDownloadOutputBringsAHostedRunHere(t *testing.T) {
	f := &hostedFake{enabled: true, outputs: map[string]string{"result.json": hostedEnvelope, "reports/report.md": "# Report\n"}}
	f.run = &hostedrun.Run{ID: "22222222-2222-2222-2222-222222222222", Ref: "acme/reporter@1.0.0", Mode: "autonomous",
		Status: hostedrun.StatusRunning, CreatedAt: time.Now().UTC()}
	reg := f.start(t)
	home, _ := signIn(t, reg)
	t.Setenv(policy.HomeEnv, t.TempDir())
	r, _ := newRunAPI(t)

	v, err := r.HostedRecord(reg, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Downloadable || v.Kept {
		t.Fatalf("a run still under way: %+v", v)
	}
	if _, err := r.DownloadHostedOutput(reg, f.run.ID); err == nil {
		t.Fatal("a run still under way was downloaded")
	}
	f.finish(hostedrun.OutcomeSucceeded, "ok", 2)
	if v, _ = r.HostedRecord(reg, f.run.ID); !v.Downloadable || v.Kept {
		t.Fatalf("a finished run not here: %+v", v)
	}
	if _, err := r.RunOutputs(f.run.ID); err == nil {
		t.Fatal("RunOutputs read a run not downloaded")
	}
	v, err = r.DownloadHostedOutput(reg, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Kept || v.Downloadable || v.Run.ResultJSON != `"the summary"` {
		t.Fatalf("after Download output: %+v (run %+v)", v, v.Run)
	}
	if !strings.HasPrefix(v.Run.OutDir, filepath.Join(home, "Downloads", "agentry-out")) {
		t.Fatalf("downloaded into %q, not a folder Go chose", v.Run.OutDir)
	}
	e, ok, _ := runhistory.Find(f.run.ID)
	if !ok || e.Where != runhistory.WhereAgentry || e.Outcome != hostedrun.OutcomeSucceeded || e.Folder != v.Run.OutDir || e.Registry != reg {
		t.Fatalf("the history's entry %+v", e)
	}
	again := &RunAPI{app: &App{ctx: context.Background()}}
	if got, err := again.RunOutputs(f.run.ID); err != nil || len(got.Files) != 1 {
		t.Fatalf("RunOutputs by the registry's id in a new process: %+v %v", got, err)
	}
	if _, err := r.HostedRecord(reg, "../x"); err == nil {
		t.Fatal("HostedRecord took an id that is not one")
	}
	if _, err := r.HostedRecord("http://192.0.2.1:1", f.run.ID); err == nil {
		t.Fatal("HostedRecord asked a registry it holds no token for")
	}
}

// A review's run page is its report, read back from its folder: a regular
// file, never through a link, drawn through the review's own view; any other
// kind of run has none.
func TestARecordedReviewIsItsReport(t *testing.T) {
	freshConfig(t)
	dir := t.TempDir()
	rep := &examiner.Report{Package: examiner.Package{Ref: "me/notes@0.1.0", Files: 4}}
	rep.Expected.Outcome = examiner.OutcomeUndetermined
	if _, err := examiner.Write(dir, rep); err != nil {
		t.Fatal(err)
	}
	add := func(id, kind string) {
		t.Helper()
		if err := runhistory.Add(runhistory.Entry{ID: id, Kind: kind, Where: runhistory.WhereHere, Outcome: "ok", EndedAt: time.Now(), Folder: dir}); err != nil {
			t.Fatal(err)
		}
	}
	add("rev00001", runhistory.KindReview)
	add("run00001", runhistory.KindRun)
	r := &RunAPI{app: &App{ctx: context.Background()}}
	v, err := r.RecordedReview("rev00001")
	if err != nil {
		t.Fatal(err)
	}
	if v.Ref != "me/notes@0.1.0" || v.Files != 4 || v.Outcome != examiner.OutcomeUndetermined || v.OutDir != dir {
		t.Fatalf("the report %+v", v)
	}
	if _, err := r.RecordedReview("run00001"); err == nil {
		t.Fatal("a run that is not a review gave a report")
	}
	if _, err := r.RecordedReview("nothere1"); err == nil {
		t.Fatal("an id the history does not hold gave a report")
	}
	if runtime.GOOS != "windows" {
		elsewhere := filepath.Join(t.TempDir(), "report.json")
		raw, _ := os.ReadFile(filepath.Join(dir, examiner.ReportFile))
		writeRunFile(t, elsewhere, string(raw))
		_ = os.Remove(filepath.Join(dir, examiner.ReportFile))
		if err := os.Symlink(elsewhere, filepath.Join(dir, examiner.ReportFile)); err != nil {
			t.Fatal(err)
		}
		if _, err := r.RecordedReview("rev00001"); err == nil {
			t.Fatal("a report that is a link was read")
		}
	}
}

// The run page's reads of a run In Agentry send the token only to an address
// Settings' URL rule accepts, as Recent runs does.
func TestTheRunPagesHostedReadsHoldToTheURLRule(t *testing.T) {
	keyring.MockInit()
	freshConfig(t)
	reg := "http://registry.example.test"
	s, _ := settings.Load()
	s.Upsert(settings.Registry{URL: reg, Token: hostedTestToken})
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	if s, _ := settings.Load(); s.Find(reg) == nil || s.Find(reg).Token == "" {
		t.Fatal("the test's token was not kept")
	}
	if settings.TokenFor(reg) != "" {
		t.Fatal("a token was handed out for an http registry off this machine")
	}
	r := &RunAPI{app: &App{ctx: context.Background()}}
	id := "22222222-2222-2222-2222-222222222222"
	for name, call := range map[string]func() error{
		"HostedRecord":         func() error { _, err := r.HostedRecord(reg, id); return err },
		"DownloadHostedOutput": func() error { _, err := r.DownloadHostedOutput(reg, id); return err },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "must use https") {
			t.Errorf("%s with an http registry off this machine: %v", name, err)
		}
	}
	if v, _ := r.RecentRuns(reg, 5); v.SignedIn || v.HostedError != "" {
		t.Fatalf("Recent runs asked an http registry off this machine: %+v", v)
	}
}

// Download output writes nothing through a run folder that is now a link,
// and a history that says a folder has gone is not overruled by memory.
func TestDownloadOutputWritesNothingThroughALink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	f := &hostedFake{enabled: true, outputs: map[string]string{"result.json": hostedEnvelope}}
	f.run = &hostedrun.Run{ID: "22222222-2222-2222-2222-222222222222", Ref: "acme/reporter@1.0.0", Mode: "autonomous", CreatedAt: time.Now().UTC()}
	reg := f.start(t)
	home, _ := signIn(t, reg)
	t.Setenv(policy.HomeEnv, t.TempDir())
	f.finish(hostedrun.OutcomeSucceeded, "ok", 2)
	r, _ := newRunAPI(t)
	want, err := hostedOutDir("", f.run)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(want)
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, want); err != nil {
		t.Fatal(err)
	}
	if _, err := r.DownloadHostedOutput(reg, f.run.ID); err == nil {
		t.Fatal("Download output wrote through a link")
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("the link's target got %v", entries)
	}
	// Memory and the history both name the folder; the history says it has
	// gone (a link), and the page is told so, not shown it.
	r.rememberFolder(f.run.ID, want)
	if err := runhistory.Add(runhistory.Entry{ID: f.run.ID, Kind: runhistory.KindRun, Where: runhistory.WhereAgentry, Outcome: "succeeded", EndedAt: time.Now(), Folder: want}); err != nil {
		t.Fatal(err)
	}
	v, err := r.HostedRecord(reg, f.run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Kept || v.FolderState != folderGone || v.Run.ResultJSON != "" {
		t.Fatalf("a folder that is now a link: %+v", v)
	}
	_ = home
}

// A run on this computer that ended blocked (the CLI's exit 18) is recorded
// as blocked, never as an error, so the history says so; any other failure
// is still the error it was.
func TestALocalBlockedRunIsRecordedAsBlocked(t *testing.T) {
	ctx := context.Background()
	if o, code := localOutcome(ctx, &run.Result{Status: "error", ExitCode: 18, ExitClass: "blocked"}); o != runhistory.OutcomeBlocked || code != 18 {
		t.Fatalf("blocked: %s %d", o, code)
	}
	if o, code := localOutcome(ctx, &run.Result{Status: "error", ExitCode: 14, ExitClass: "harness"}); o != runhistory.OutcomeError || code != 14 {
		t.Fatalf("harness: %s %d", o, code)
	}
	if o, _ := localOutcome(ctx, &run.Result{Status: "ok", ExitClass: "ok"}); o != runhistory.OutcomeOK {
		t.Fatalf("ok: %s", o)
	}
}

// A run that fails is recorded with its exit class, the runner's last error
// line and its log, kept in a file of its own; the run page reads all three
// by the run's id, with the class's meaning and retry hint from the CLI's
// table, after Dismiss and in a new process alike.
func TestAFailedRunIsRecordedWithItsClassCauseAndLog(t *testing.T) {
	freshConfig(t)
	out := t.TempDir()
	r := flowAPI(t, func(_ context.Context, req run.Request, logw io.Writer) (*run.Result, error) {
		_, _ = io.WriteString(logw, "pulling the image\nthe harness exited 1\n")
		return &run.Result{Status: "error", ExitCode: 14, ExitClass: "harness", Error: "harness: the model answered 503", RunID: req.RunID,
			Ref: "acme/agent@1.0.0", OutDir: out}, nil
	})
	v, err := runAgent(r)
	if err != nil {
		t.Fatal(err)
	}
	if v.ExitClassMeaning != "the agent's harness failed or could not reach its model" || !v.RetryLikelyHelps {
		t.Fatalf("the result view's class words: %q, retry %v", v.ExitClassMeaning, v.RetryLikelyHelps)
	}
	e := onlyEntry(t)
	if e.ExitClass != "harness" || e.Cause != "harness: the model answered 503" || e.Log == "" {
		t.Fatalf("entry %+v", e)
	}
	raw, err := os.ReadFile(e.Log)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "pulling the image\nthe harness exited 1\n" {
		t.Fatalf("the kept log holds %q", raw)
	}
	r.ForgetRun(v.RunID)
	again := &RunAPI{app: &App{ctx: context.Background()}}
	rec, err := again.RecordedRun(v.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Run.ExitClass != "harness" || rec.Run.Cause != e.Cause || !rec.Run.LogKept || rec.Log != string(raw) ||
		rec.Run.ExitClassMeaning != v.ExitClassMeaning || !rec.Run.RetryLikelyHelps {
		t.Fatalf("the run page's view after Dismiss: %+v, log %q", rec.Run, rec.Log)
	}
	// A run that ended ok records no class, cause or meaning, and its log.
	ok := flowAPI(t, func(_ context.Context, req run.Request, logw io.Writer) (*run.Result, error) {
		_, _ = io.WriteString(logw, "done\n")
		return &run.Result{Status: "ok", RunID: req.RunID, OutDir: out}, nil
	})
	ok.newRunID = func() (string, error) { return "ab12cd35", nil }
	if _, err := runAgent(ok); err != nil {
		t.Fatal(err)
	}
	rec, err = again.RecordedRun("ab12cd35")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Run.ExitClass != "" || rec.Run.Cause != "" || rec.Run.ExitClassMeaning != "" || rec.Log != "done\n" {
		t.Fatalf("a run that ended ok: %+v, log %q", rec.Run, rec.Log)
	}
	// With the history off, no log is kept either.
	setRemember(t, false)
	off := flowAPI(t, func(_ context.Context, req run.Request, logw io.Writer) (*run.Result, error) {
		_, _ = io.WriteString(logw, "quiet\n")
		return &run.Result{Status: "error", ExitCode: 13, ExitClass: "gateway", RunID: req.RunID, OutDir: out}, nil
	})
	off.newRunID = func() (string, error) { return "ab12cd36", nil }
	if _, err := runAgent(off); err != nil {
		t.Fatal(err)
	}
	if p, _ := runhistory.LogPath("ab12cd36"); p != "" {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("a log was kept with the history off: %v", err)
		}
	}
}

// A run In Agentry that failed records the registry's error line as its
// cause, and its row carries the class's words.
func TestAFailedHostedRunIsRecordedWithItsCause(t *testing.T) {
	freshConfig(t)
	now := time.Now().UTC()
	h := &hostedrun.Run{ID: "11111111-2222-3333-4444-555555555555", Ref: "acme/agent@1.0.0", Status: hostedrun.StatusFinished,
		Outcome: hostedrun.OutcomeFailed, ExitClass: "timeout", Error: "the run's time budget ran out", CreatedAt: now, FinishedAt: &now}
	recordHosted("https://r.example", h, t.TempDir())
	e := onlyEntry(t)
	if e.ExitClass != "timeout" || e.Cause != "the run's time budget ran out" {
		t.Fatalf("entry %+v", e)
	}
	row := entryView(e)
	if row.ExitClassMeaning != "the run's time budget ran out" || row.RetryLikelyHelps || row.Cause != e.Cause {
		t.Fatalf("row %+v", row)
	}
}
