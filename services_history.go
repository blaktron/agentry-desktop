package main

// The run history and the run page. Go keeps the last 50 runs on this
// computer, the built-ins' jobs included, in runs.yaml
// (internal/runhistory), and Home's Recent runs merges them with the
// account's runs In Agentry, read live.
//
// The page names a run by its id and never a path. An id is read through
// this process's memory of the runs it produced, or through the history,
// which only Go writes, from the engine's own result, a built-in's own
// return, or the registry's record of a run whose output this app
// downloaded. A folder read through the history is checked again on every
// read (Lstat: still there, a directory, not a link), and it is never
// searched for when it has gone.
//
// Nothing a run wrote is believed: how a run ended comes from the history's
// entry or the registry's record, never from result.json read back off
// disk. The one thing read from that file is its result, which the page
// renders under the hostile rules it renders every run's result with.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/blaktron/agentry-cli/pkg/examiner"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/review"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/runhistory"
	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// maxResultBytes caps what is read of a recorded run's result.json.
const maxResultBytes = 1 << 20

// historyOn says whether Go keeps the history: Run defaults' Remember
// recent runs, on unless turned off. A settings file that cannot be read
// keeps nothing, rather than guessing the person's answer.
func historyOn() bool {
	st, err := settings.Load()
	return err == nil && st.RememberRunsEnabled()
}

// record writes e into the history while the switch is on. Best effort: a
// run's result is never refused because the history could not be written.
func record(e runhistory.Entry) {
	if !historyOn() {
		return
	}
	if err := runhistory.Add(e); err != nil {
		log.Printf("agentry-desktop: run history: %v", err)
	}
}

// refOrEmpty is ref when it names a release, else "": the history keeps a
// ref, never other words.
func refOrEmpty(ref string) string {
	if _, err := manifest.ParseRef(ref); err != nil {
		return ""
	}
	return ref
}

// localOutcome is how a run on this computer ended: stopped when its
// context was cancelled (Stop, or quitting), else the engine's status.
func localOutcome(ctx context.Context, res *run.Result) (string, int) {
	switch {
	case ctx.Err() != nil:
		code := 0
		if res != nil {
			code = res.ExitCode
		}
		return runhistory.OutcomeStopped, code
	case res == nil:
		return runhistory.OutcomeError, 0
	case res.Status == "ok":
		return runhistory.OutcomeOK, 0
	case res.ExitClass == "blocked":
		return runhistory.OutcomeBlocked, res.ExitCode
	default:
		return runhistory.OutcomeError, res.ExitCode
	}
}

// recordLocal records a QuickAgent's run or chat on this computer from the
// engine's result as Go read it (res is nil when the engine never wrote an
// envelope).
func recordLocal(ctx context.Context, kind string, l *liveRun, ref string, res *run.Result) {
	if l == nil {
		return
	}
	outcome, code := localOutcome(ctx, res)
	e := runhistory.Entry{
		ID: l.id, Kind: kind, Where: runhistory.WhereHere, Registry: settings.Normalize(l.registry), Ref: refOrEmpty(ref),
		StartedAt: l.started, EndedAt: time.Now().UTC(), Outcome: outcome, ExitCode: code,
	}
	if res != nil {
		if r := refOrEmpty(res.Ref); r != "" {
			e.Ref = r
		}
		e.Digest = res.Digest
		e.Folder = cleanAbs(res.OutDir)
	}
	record(e)
}

// builtinKind is the history's kind for a built-in's job, by the words it
// holds the run lock under.
func builtinKind(ref string) string {
	switch ref {
	case draftRunRef:
		return runhistory.KindDraft
	case reviewRunRef:
		return runhistory.KindReview
	case knowledgeRunRef:
		return runhistory.KindKnowledge
	case voiceRunRef:
		return runhistory.KindVoice
	}
	return ""
}

// recordBuiltin records a built-in's job on this computer as asRun ends it:
// stopped when its context was cancelled, failed when it returned an error,
// else ok; its folder is the one its flow kept (keepFolder).
func recordBuiltin(ctx context.Context, l *liveRun, err error) {
	kind := builtinKind(l.ref)
	if kind == "" {
		return
	}
	outcome := runhistory.OutcomeOK
	switch {
	case ctx.Err() != nil:
		outcome = runhistory.OutcomeStopped
	case err != nil:
		outcome = runhistory.OutcomeError
	}
	record(runhistory.Entry{
		ID: l.id, Kind: kind, Where: runhistory.WhereHere,
		StartedAt: l.started, EndedAt: time.Now().UTC(), Outcome: outcome, Folder: cleanAbs(l.folder),
	})
}

// keepFolder is a built-in's flow naming the folder its job wrote, from
// its own return: the result view lists it by the job's id, and the
// history keeps it.
func (r *RunAPI) keepFolder(runID, dir string) {
	if dir == "" {
		return
	}
	r.liveMu.Lock()
	if r.live != nil && r.live.id == runID {
		r.live.folder = dir
	}
	r.liveMu.Unlock()
	r.rememberFolder(runID, dir)
}

// hostedKind is the history's kind for a run In Agentry.
func hostedKind(h *hostedrun.Run) string {
	switch {
	case h.IsBuild():
		return runhistory.KindKnowledge
	case h.IsDraft():
		return runhistory.KindDraft
	case h.IsVoiceBuild():
		return runhistory.KindVoice
	case h.IsSession():
		return runhistory.KindChat
	}
	return runhistory.KindRun
}

// recordHosted records a run In Agentry whose output this app downloaded
// into dir, from the registry's own record of it.
func recordHosted(reg string, h *hostedrun.Run, dir string) {
	if h == nil || !validRunID(h.ID) {
		return
	}
	started, ended := h.CreatedAt, time.Now().UTC()
	if h.StartedAt != nil {
		started = *h.StartedAt
	}
	if h.FinishedAt != nil {
		ended = *h.FinishedAt
	}
	outcome := h.Outcome
	if outcome == "" {
		outcome = h.Status
	}
	record(runhistory.Entry{
		ID: h.ID, Kind: hostedKind(h), Where: runhistory.WhereAgentry, Registry: settings.Normalize(reg),
		Ref: refOrEmpty(h.Ref), Digest: h.Digest, StartedAt: started.UTC(), EndedAt: ended.UTC(),
		Outcome: outcome, ExitClass: h.ExitClass, Folder: cleanAbs(dir),
	})
}

// cleanAbs is dir made absolute and clean, as the history keeps a folder;
// "" for none, or for one that cannot be made absolute.
func cleanAbs(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	return abs
}

var errRunFolderGone = errors.New("the run's folder is no longer there: it was deleted or moved")

// recordedFolder is a history entry's folder, checked again now
// (checkRunFolder).
func recordedFolder(e runhistory.Entry) (string, error) {
	if e.Folder == "" {
		return "", errUnknownRun
	}
	return checkRunFolder(e.Folder)
}

// checkRunFolder is dir when it is still there, a directory, and not a link:
// a folder replaced by a link since the run is not followed, and one that
// has gone is never looked for.
func checkRunFolder(dir string) (string, error) {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", errRunFolderGone
	case err != nil:
		return "", fmt.Errorf("the run's folder cannot be read: %w", err)
	case info.Mode()&fs.ModeSymlink != 0:
		return "", fmt.Errorf("the run's folder %s is now a link, so it is not read", dir)
	case !info.IsDir():
		return "", fmt.Errorf("the run's folder %s is now a file, so it is not read", dir)
	}
	return dir, nil
}

// Where the run page reads a run, and what its folder is now.
const (
	sourceLocal  = "local"
	sourceHosted = "hosted"

	folderPresent = "present"
	folderGone    = "gone"
	folderNone    = "none"
)

// historyFolder is the folder the history keeps for runID, checked, or
// the one a schedule's fire recorded for it.
func historyFolder(runID string) (string, error) {
	if historyOn() {
		if e, ok, err := runhistory.Find(runID); err == nil && ok {
			return recordedFolder(e)
		}
	}
	if e, _, ok := scheduledEntry(runID); ok {
		return recordedFolder(e)
	}
	return "", errUnknownRun
}

// scheduledEntry is a run a schedule on this computer fired, as a history
// entry, read from the CLI's record of the schedule's fires (the agentry
// command wrote it, never the page), with the schedule's name. A fire
// that made no run has no run id and is never found.
func scheduledEntry(runID string) (runhistory.Entry, string, bool) {
	if !validRunID(runID) {
		return runhistory.Entry{}, "", false
	}
	home, err := run.Home()
	if err != nil {
		return runhistory.Entry{}, "", false
	}
	f, ok := schedules.StoreIn(home).FindRun(runID)
	if !ok {
		return runhistory.Entry{}, "", false
	}
	return fireEntry(f), f.Name, true
}

// fireEntry is a schedule's fire as a history entry.
func fireEntry(f schedules.Fire) runhistory.Entry {
	e := runhistory.Entry{
		ID: f.RunID, Kind: runhistory.KindRun, Where: runhistory.WhereHere, Ref: f.Ref,
		Outcome: runhistory.OutcomeError, ExitCode: f.ExitCode, ExitClass: f.ExitClass,
	}
	if f.Outcome == schedules.Ran {
		e.Outcome = runhistory.OutcomeOK
	}
	e.StartedAt, _ = time.Parse(time.RFC3339, f.Started)
	e.EndedAt, _ = time.Parse(time.RFC3339, f.Ended)
	// The folder is the CLI's record's; one that is not absolute and clean
	// is not offered.
	if filepath.IsAbs(f.OutDir) && filepath.Clean(f.OutDir) == f.OutDir {
		e.Folder = f.OutDir
	}
	return e
}

// scheduledRows is the runs schedules on this computer fired, as Recent
// runs rows: a missed occurrence ran nothing and is not one (the
// Scheduled page's History shows it).
func scheduledRows(limit int) []RecentRunView {
	home, err := run.Home()
	if err != nil {
		return nil
	}
	fires, err := schedules.StoreIn(home).Fires(0)
	if err != nil {
		return nil
	}
	var rows []RecentRunView
	for _, f := range fires {
		if f.RunID == "" || f.Outcome == schedules.Missed || !validRunID(f.RunID) {
			continue
		}
		row := entryView(fireEntry(f))
		row.Scheduled, row.Schedule = true, f.Name
		rows = append(rows, row)
		if len(rows) >= limit {
			break
		}
	}
	return rows
}

// RecentRunView is one row of Recent runs: a run, a chat or a built-in's
// job, on this computer or In Agentry.
type RecentRunView struct {
	ID string `json:"id"`
	// Kind is run, chat, draft, review, knowledge or voice.
	Kind string `json:"kind"`
	// Where is "here" or "agentry".
	Where     string `json:"where"`
	Registry  string `json:"registry,omitempty"`
	Ref       string `json:"ref,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
	// EndedAt is "" while a run In Agentry is still under way.
	EndedAt string `json:"endedAt,omitempty"`
	// Outcome is ok, error or stopped for a run here; the registry's
	// outcome, or its status while the run moves, for one In Agentry.
	Outcome   string `json:"outcome"`
	ExitCode  int    `json:"exitCode,omitempty"`
	ExitClass string `json:"exitClass,omitempty"`
	// State is a run In Agentry's place in the site's words; Tone is ok,
	// bad or warn.
	State string `json:"state,omitempty"`
	Tone  string `json:"tone"`
	// Kept says the history keeps a folder for it, so the run page reads
	// its files by its id.
	Kept bool `json:"kept"`
	// Source is where the run page reads it: "local" (the history) or
	// "hosted" (the registry).
	Source string `json:"source"`
	// Scheduled: a schedule on this computer fired it, through the agentry
	// command, so the CLI's record holds it rather than the history.
	// Schedule is that schedule's name.
	Scheduled bool   `json:"scheduled,omitempty"`
	Schedule  string `json:"schedule,omitempty"`
}

// RecentRunsView is Home's Recent runs.
type RecentRunsView struct {
	Runs []RecentRunView `json:"runs"`
	// Remember is Run defaults' switch: off, nothing here is kept.
	Remember bool `json:"remember"`
	// SignedIn says the registry was asked for the account's runs.
	SignedIn     bool   `json:"signedIn"`
	HostedError  string `json:"hostedError,omitempty"`
	HistoryError string `json:"historyError,omitempty"`
}

// localTone is a run on this computer's outcome as the page's tone.
func localTone(outcome string) string {
	switch outcome {
	case runhistory.OutcomeOK:
		return "ok"
	case runhistory.OutcomeStopped:
		return "warn"
	}
	return "bad"
}

// entryView is a history entry as a row. A run In Agentry's entry keeps the
// registry's outcome word, so its state and tone are said as the
// registry's record of a finished (or cancelled) run would say them.
func entryView(e runhistory.Entry) RecentRunView {
	v := RecentRunView{
		ID: e.ID, Kind: e.Kind, Where: e.Where, Registry: e.Registry, Ref: e.Ref,
		StartedAt: rfc3339(e.StartedAt), EndedAt: rfc3339(e.EndedAt),
		Outcome: e.Outcome, ExitCode: e.ExitCode, ExitClass: e.ExitClass, Kept: e.Folder != "", Source: sourceLocal,
	}
	if e.Where == runhistory.WhereAgentry {
		h := &hostedrun.Run{Status: hostedrun.StatusFinished, Outcome: e.Outcome}
		if e.Outcome == hostedrun.StatusCancelled {
			h.Status, h.Outcome = hostedrun.StatusCancelled, ""
		}
		v.State, v.Tone = hostedrun.State(h), hostedrun.Read(h).Tone
	} else {
		v.Tone = localTone(e.Outcome)
	}
	return v
}

// hostedRowView is a run In Agentry, read live from reg, as a row.
func hostedRowView(reg string, h *hostedrun.Run) RecentRunView {
	v := RecentRunView{
		ID: h.ID, Kind: hostedKind(h), Where: runhistory.WhereAgentry, Registry: reg, Ref: refOrEmpty(h.Ref),
		StartedAt: rfc3339(h.CreatedAt), Outcome: h.Outcome, ExitClass: h.ExitClass,
		State: hostedrun.State(h), Tone: hostedrun.Read(h).Tone, Source: sourceHosted,
	}
	if h.StartedAt != nil {
		v.StartedAt = rfc3339(*h.StartedAt)
	}
	if h.FinishedAt != nil {
		v.EndedAt = rfc3339(*h.FinishedAt)
	}
	if v.Outcome == "" {
		v.Outcome = h.Status
	}
	return v
}

// RecentRuns is Home's Recent runs: this computer's history and, signed in
// to reg, the account's runs In Agentry read live, merged newest first, at
// most limit (1 to 50). A run In Agentry this app downloaded is one row,
// the registry's, marked kept. It starts nothing and spends nothing.
func (r *RunAPI) RecentRuns(reg string, limit int) (*RecentRunsView, error) {
	limit = min(max(limit, 1), runhistory.Max)
	v := &RecentRunsView{Runs: []RecentRunView{}, Remember: historyOn()}
	kept := map[string]runhistory.Entry{}
	if v.Remember {
		entries, err := runhistory.List()
		if err != nil {
			v.HistoryError = err.Error()
		}
		for _, e := range entries {
			kept[e.ID] = e
		}
	}
	reg = settings.Normalize(reg)
	seen := map[string]bool{}
	if token := settings.TokenFor(reg); reg != "" && token != "" && settings.ValidateURL(reg) == nil {
		v.SignedIn = true
		ctx, cancel := context.WithTimeout(r.ctx(), 15*time.Second)
		runs, err := hostedrun.New(reg, token).List(ctx)
		cancel()
		if err != nil {
			v.HostedError = err.Error()
		}
		for i := range runs {
			h := &runs[i]
			if !validRunID(h.ID) || seen[h.ID] {
				continue
			}
			seen[h.ID] = true
			row := hostedRowView(reg, h)
			if e, ok := kept[h.ID]; ok && e.Where == runhistory.WhereAgentry {
				row.Kept = e.Folder != ""
			}
			v.Runs = append(v.Runs, row)
		}
	}
	for _, e := range kept {
		if !seen[e.ID] {
			seen[e.ID] = true
			v.Runs = append(v.Runs, entryView(e))
		}
	}
	// Runs a schedule fired, also while the app was closed.
	for _, row := range scheduledRows(limit) {
		if !seen[row.ID] {
			seen[row.ID] = true
			v.Runs = append(v.Runs, row)
		}
	}
	sort.SliceStable(v.Runs, func(i, j int) bool { return rowTime(v.Runs[i]) > rowTime(v.Runs[j]) })
	if len(v.Runs) > limit {
		v.Runs = v.Runs[:limit]
	}
	return v, nil
}

// rowTime orders rows by when they ended, a run still moving by when it
// started (so it sits among the newest): RFC 3339 in UTC sorts as text.
func rowTime(v RecentRunView) string {
	if v.EndedAt != "" {
		return v.EndedAt
	}
	return v.StartedAt
}

// RecordedRunView is a run the history keeps, for the run page: the row,
// the folder and whether it is still there, and the result its folder
// holds.
type RecordedRunView struct {
	Run    RecentRunView `json:"run"`
	Folder string        `json:"folder,omitempty"`
	// FolderState is "present", "gone" (deleted, moved, or now a link or a
	// file) or "none" (the job kept no folder: a knowledge or voice build).
	FolderState string `json:"folderState"`
	FolderError string `json:"folderError,omitempty"`
	// ResultJSON is the result in the folder's result.json, its result
	// alone: how the run ended is the entry's, never the file's.
	ResultJSON string `json:"resultJson,omitempty"`
}

// RecordedRun is a run the history keeps, by its id: the run page's local
// source. An id the history does not hold is refused.
func (r *RunAPI) RecordedRun(runID string) (*RecordedRunView, error) {
	if !validRunID(runID) {
		return nil, errUnknownRun
	}
	var (
		e        runhistory.Entry
		ok       bool
		err      error
		schedule string
	)
	if historyOn() {
		if e, ok, err = runhistory.Find(runID); err != nil {
			return nil, err
		}
	}
	if !ok {
		// A run a schedule fired, from the CLI's record.
		if e, schedule, ok = scheduledEntry(runID); !ok {
			return nil, errUnknownRun
		}
	}
	v := &RecordedRunView{Run: entryView(e), Folder: e.Folder, FolderState: folderNone}
	if schedule != "" {
		v.Run.Scheduled, v.Run.Schedule = true, schedule
	}
	if e.Folder == "" {
		return v, nil
	}
	dir, err := recordedFolder(e)
	if err != nil {
		v.FolderState, v.FolderError = folderGone, err.Error()
		return v, nil
	}
	v.FolderState = folderPresent
	v.ResultJSON = recordedResult(dir)
	return v, nil
}

// recordedResult is the result in dir/result.json, read as a run's output
// file is (no link, a regular file, capped) and cut to its result: the
// envelope's status and exit code are not passed on, so nothing the agent
// could have written there says how the run ended. "" when there is none.
func recordedResult(dir string) string {
	b, ok := readRegular(filepath.Join(dir, "result.json"), maxResultBytes)
	if !ok {
		return ""
	}
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(b, &env) != nil || len(env.Result) == 0 {
		return ""
	}
	return string(env.Result)
}

// readRegular reads p as a run's output file is read: a regular file, of at
// most limit bytes, opened without following a link and checked to be the
// file the Lstat saw.
func readRegular(p string, limit int64) ([]byte, bool) {
	before, err := os.Lstat(p)
	if err != nil || !before.Mode().IsRegular() || before.Size() > limit {
		return nil, false
	}
	f, err := openNoFollow(p)
	if err != nil {
		return nil, false
	}
	defer func() { _ = f.Close() }()
	if after, err := f.Stat(); err != nil || !os.SameFile(before, after) {
		return nil, false
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, false
	}
	return b, true
}

// RecordedReview is an Examiner review the history keeps, for its run page:
// its report as the review wrote it once the Examiner's run was over
// (review.Run writes report.json then, into a folder it refuses if a report
// is already there), read as readRegular reads and drawn through the view
// the review's own dialog uses, which holds every verdict word to the
// report's vocabulary.
func (r *RunAPI) RecordedReview(runID string) (*ReviewView, error) {
	if !historyOn() || !validRunID(runID) {
		return nil, errUnknownRun
	}
	e, ok, err := runhistory.Find(runID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errUnknownRun
	}
	if e.Kind != runhistory.KindReview {
		return nil, errors.New("that run is not a review by the Examiner")
	}
	dir, err := recordedFolder(e)
	if err != nil {
		return nil, err
	}
	b, ok := readRegular(filepath.Join(dir, examiner.ReportFile), maxResultBytes)
	if !ok {
		return nil, errors.New("the review's report is not in its folder")
	}
	var rep examiner.Report
	if err := json.Unmarshal(b, &rep); err != nil {
		return nil, fmt.Errorf("the review's report cannot be read: %w", err)
	}
	return reviewView(runID, &review.Result{Report: &rep, OutDir: dir}), nil
}

// RemoveRecordedRun drops a run from the history, as the run page offers
// when its folder has gone; the folder, wherever it is, is not touched or
// looked for.
func (r *RunAPI) RemoveRecordedRun(runID string) error {
	if !validRunID(runID) {
		return errUnknownRun
	}
	r.foldersMu.Lock()
	delete(r.folders, runID)
	r.foldersMu.Unlock()
	return runhistory.Remove(runID)
}

// HostedRecordView is a run In Agentry for the run page: the registry's
// record read now, and whether its output is on this computer.
type HostedRecordView struct {
	Run  *HostedRunView `json:"run"`
	Kind string         `json:"kind"`
	// Kept says its output is in a folder this app keeps, read by its id;
	// FolderState is "present", "gone" or "none".
	Kept        bool   `json:"kept"`
	FolderState string `json:"folderState"`
	FolderError string `json:"folderError,omitempty"`
	// Downloadable says Download output is offered: a run or a chat that is
	// over, with output the registry still holds, not on this computer.
	Downloadable bool `json:"downloadable"`
	// Followed says this app is following the run now: its card has it.
	Followed bool `json:"followed"`
	// NoOutput says why a run that is over offers no download: the
	// registry holds no output for it.
	NoOutput string `json:"noOutput,omitempty"`
}

// hostedClient is reg's runs client for the run page, under Settings' URL
// rule, with the token kept for reg.
func hostedClient(reg string) (*hostedrun.Client, error) {
	return hostedClientFor(reg, "see its runs In Agentry")
}

// hostedClientFor is reg's client under Settings' URL rule, with the token
// kept for reg; signed out, the error says what signing in is for.
func hostedClientFor(reg, forWhat string) (*hostedrun.Client, error) {
	reg = settings.Normalize(reg)
	if err := settings.ValidateURL(reg); err != nil {
		return nil, err
	}
	token := settings.TokenFor(reg)
	if token == "" {
		return nil, fmt.Errorf("sign in to %s to %s", reg, forWhat)
	}
	return hostedrun.New(reg, token), nil
}

// HostedRecord is a run In Agentry by its id, read live: the run page's
// hosted source.
func (r *RunAPI) HostedRecord(reg, runID string) (*HostedRecordView, error) {
	if !validRunID(runID) {
		return nil, errUnknownRun
	}
	client, err := hostedClient(reg)
	if err != nil {
		return nil, err
	}
	h, err := client.Get(r.ctx(), runID)
	if err != nil {
		return nil, err
	}
	if h.ID != runID {
		return nil, fmt.Errorf("the registry answered the run %s as %q", runID, h.ID)
	}
	return r.hostedRecordView(h), nil
}

// hostedRecordView is a run In Agentry for the run page: the registry's
// record, the folder its output is in when the history (or this process)
// keeps one that is still there, and whether Download output is offered.
func (r *RunAPI) hostedRecordView(h *hostedrun.Run) *HostedRecordView {
	v := &HostedRecordView{Kind: hostedKind(h), FolderState: folderNone}
	dir := ""
	result := ""
	if historyOn() {
		if e, ok, err := runhistory.Find(h.ID); err == nil && ok && e.Where == runhistory.WhereAgentry && e.Folder != "" {
			if d, err := recordedFolder(e); err != nil {
				v.FolderState, v.FolderError = folderGone, err.Error()
			} else {
				dir, v.Kept, v.FolderState = d, true, folderPresent
				result = recordedResult(d)
			}
		}
	}
	// A run downloaded in this process and not kept in the history (the
	// switch off) is read from memory, checked as a history folder is; a
	// history that says its folder has gone is not overruled by memory.
	if !v.Kept && v.FolderState != folderGone {
		r.foldersMu.Lock()
		d, ok := r.folders[h.ID]
		r.foldersMu.Unlock()
		if ok {
			if d, err := checkRunFolder(d); err != nil {
				v.FolderState, v.FolderError = folderGone, err.Error()
			} else {
				dir, v.Kept, v.FolderState = d, true, folderPresent
				result = recordedResult(d)
			}
		}
	}
	if cur := r.hostedCurrent(); cur != nil && cur.id == h.ID {
		v.Followed = true
	}
	v.Run = hostedRunView(h, dir, nil, result, "", h.Terminal())
	if !v.Kept && !v.Followed && h.Terminal() && (v.Kind == runhistory.KindRun || v.Kind == runhistory.KindChat) {
		if len(h.Outputs) > 0 {
			v.Downloadable = true
		} else {
			v.NoOutput = errNoHostedOutput.Error()
		}
	}
	return v
}

var errNoHostedOutput = errors.New("the registry holds no output for this run: it wrote none, or the 30 days it is kept have passed")

// DownloadHostedOutput brings a finished run In Agentry's output down to
// this computer: the existing download, which spends no credits, into the
// folder Go chooses as it does for a run attached at start (Run defaults'
// saved folder, a folder of the run's own inside it, else the default under
// Downloads; never one the page names). The history keeps the folder against
// the registry's run id.
func (r *RunAPI) DownloadHostedOutput(reg, runID string) (*HostedRecordView, error) {
	if !validRunID(runID) {
		return nil, errUnknownRun
	}
	if _, busy := r.downloading.LoadOrStore(runID, true); busy {
		return nil, errors.New("this run's output is already being downloaded")
	}
	defer r.downloading.Delete(runID)
	client, err := hostedClient(reg)
	if err != nil {
		return nil, err
	}
	ctx := r.ctx()
	h, err := client.Get(ctx, runID)
	if err != nil {
		return nil, err
	}
	if h.ID != runID {
		return nil, fmt.Errorf("the registry answered the run %s as %q", runID, h.ID)
	}
	if cur := r.hostedCurrent(); cur != nil && cur.id == runID {
		return nil, errors.New("this run is being followed in its card, which downloads its output when it ends")
	}
	if !h.Terminal() {
		return nil, errors.New("this run is not over yet")
	}
	if k := hostedKind(h); k != runhistory.KindRun && k != runhistory.KindChat {
		return nil, errors.New("a built-in's job In Agentry is opened in its own place, not downloaded here")
	}
	if len(h.Outputs) == 0 {
		return nil, errNoHostedOutput
	}
	saved := ""
	if st, err := settings.Load(); err == nil {
		saved = st.DefaultOutputDir
	}
	dir, err := hostedOutDir(saved, h)
	if err != nil {
		return nil, err
	}
	// The run's folder name is fixed by its id, so an earlier download's
	// folder may have been replaced since: nothing is written through a
	// link or into a file.
	if _, err := checkRunFolder(dir); err != nil {
		return nil, err
	}
	if _, err := client.DownloadOutputs(ctx, h, dir); err != nil {
		return nil, fmt.Errorf("the output could not be downloaded: %w", err)
	}
	r.rememberFolder(h.ID, dir)
	recordHosted(reg, h, dir)
	return r.hostedRecordView(h), nil
}

// SetRememberRuns turns Remember recent runs on or off. Off clears the
// history and stops its writes; no run folder is touched either way.
func (s *SettingsAPI) SetRememberRuns(on bool) (*RunDefaultsView, error) {
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	st.RememberRuns = &on
	if err := settings.Save(st); err != nil {
		return nil, err
	}
	if !on {
		if err := runhistory.Clear(); err != nil {
			return nil, err
		}
	}
	return runDefaultsView()
}

// ClearRunHistory empties the history. No run folder is touched.
func (s *SettingsAPI) ClearRunHistory() (*RunDefaultsView, error) {
	if err := runhistory.Clear(); err != nil {
		return nil, err
	}
	return runDefaultsView()
}

// historyCount is how many runs the history keeps, -1 when it cannot be
// read.
func historyCount() int {
	runs, err := runhistory.List()
	if err != nil {
		return -1
	}
	return len(runs)
}
