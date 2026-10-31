package runhistory

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
)

func useTemp(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "config")
	t.Setenv(configdir.EnvVar, dir)
	return filepath.Join(dir, FileName)
}

func entry(id string, ended time.Time) Entry {
	return Entry{ID: id, Kind: KindRun, Where: WhereHere, Ref: "acme/agent@1.0.0", StartedAt: ended.Add(-time.Minute), EndedAt: ended, Outcome: OutcomeOK, Folder: "/tmp/out-" + id}
}

// The history keeps the newest 50, newest first, and a run recorded again
// replaces its entry.
func TestTheHistoryKeepsTheNewestFiftyNewestFirst(t *testing.T) {
	useTemp(t)
	base := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	for i := range Max + 7 {
		if err := Add(entry(fmt.Sprintf("run%04d", i), base.Add(time.Duration(i)*time.Minute))); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != Max {
		t.Fatalf("kept %d entries, want %d", len(runs), Max)
	}
	if runs[0].ID != fmt.Sprintf("run%04d", Max+6) || runs[Max-1].ID != "run0007" {
		t.Fatalf("kept %s … %s, want the newest 50", runs[0].ID, runs[Max-1].ID)
	}
	again := entry("run0030", base.Add(time.Hour*5))
	again.Outcome = OutcomeError
	if err := Add(again); err != nil {
		t.Fatal(err)
	}
	runs, _ = List()
	if len(runs) != Max || runs[0].ID != "run0030" || runs[0].Outcome != OutcomeError {
		t.Fatalf("a run recorded again: first %+v of %d", runs[0], len(runs))
	}
}

// The file is 0600 and holds an entry's fields and nothing else: never the
// task, the inputs, the result, a secret or the log's lines, because the
// entry has no field for any of them. The log is a file of its own; the
// entry holds its path.
func TestTheFileIsPrivateAndHoldsNoTaskInputResultOrLog(t *testing.T) {
	path := useTemp(t)
	e := entry("ab12cd34", time.Now())
	e.Cause = "the harness could not reach its model"
	e.Log, _ = KeepLog(e.ID, []string{"a line of the log with TOKEN=sk-live-1234"})
	if err := Add(e); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("runs.yaml is %v, want 0600", info.Mode().Perm())
	}
	var fields []string
	for f := range reflect.TypeFor[Entry]().Fields() {
		fields = append(fields, f.Name)
	}
	want := "ID Kind Where Registry Ref Digest StartedAt EndedAt Outcome ExitCode ExitClass Cause Log Folder"
	if strings.Join(fields, " ") != want {
		t.Fatalf("an entry holds %v; a new field must not carry the task, inputs, result, a secret or the log, and this test is changed with it", fields)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "cause: the harness could not reach its model") || !strings.Contains(string(raw), "log: "+e.Log) {
		t.Errorf("runs.yaml lacks the cause or the log path:\n%s", raw)
	}
	// The log's path is under the test's own folder, whose name this test's
	// name is part of: the words are looked for in the rest of the file.
	rest := strings.ReplaceAll(string(raw), e.Log, "")
	for _, word := range []string{"task", "input", "result", "a line of the log", "token", "secret"} {
		if strings.Contains(strings.ToLower(rest), word) {
			t.Errorf("runs.yaml mentions %q:\n%s", word, raw)
		}
	}
}

// A history written before entries had a class, a cause or a log reads as
// it did, those fields empty, and keeps its entries beside new ones that
// have them.
func TestOlderRecordsReadBesideNewOnes(t *testing.T) {
	path := useTemp(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `version: 1
runs:
  - {id: old12345, kind: run, where: here, ref: acme/agent@1.0.0, folder: /abs/out, outcome: error, exitCode: 14, startedAt: 2026-10-05T11:59:00Z, endedAt: 2026-10-05T12:00:00Z}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	old, ok, err := Find("old12345")
	if err != nil || !ok {
		t.Fatalf("the older entry: %v, %v", ok, err)
	}
	if old.ExitClass != "" || old.Cause != "" || old.Log != "" || old.ExitCode != 14 || old.Outcome != OutcomeError {
		t.Fatalf("the older entry read as %+v", old)
	}
	e := entry("new12345", time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC))
	e.Outcome, e.ExitCode, e.ExitClass, e.Cause = OutcomeError, 14, "harness", "the harness could not reach its model"
	if e.Log, err = KeepLog(e.ID, []string{"one", "two"}); err != nil {
		t.Fatal(err)
	}
	if err := Add(e); err != nil {
		t.Fatal(err)
	}
	runs, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].ID != "new12345" || runs[1].ID != "old12345" {
		t.Fatalf("after Add: %+v", runs)
	}
	if runs[0].ExitClass != "harness" || runs[0].Cause != e.Cause || runs[0].Log != e.Log || runs[1].Cause != "" || runs[1].Log != "" {
		t.Fatalf("the two entries: %+v", runs)
	}
	raw, _ := os.ReadFile(path)
	if strings.Count(string(raw), "cause:") != 1 || strings.Count(string(raw), "log:") != 1 {
		t.Fatalf("the file names a cause or a log for an entry that has none:\n%s", raw)
	}
}

// A kept log is a private file named by its run, cut to the newest lines
// that fit, and it lives as long as its entry: pruned with it past Max,
// removed with it, gone with Clear. A log path an entry names elsewhere is
// not offered.
func TestAKeptLogLivesAsLongAsItsEntry(t *testing.T) {
	useTemp(t)
	base := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	path, err := KeepLog("ab12cd34", []string{"one", "two"})
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := LogsDir()
	if filepath.Dir(path) != dir || filepath.Base(path) != "ab12cd34.log" {
		t.Fatalf("the log is at %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("the log is %v, want 0600", info.Mode().Perm())
	}
	if raw, _ := os.ReadFile(path); string(raw) != "one\ntwo\n" {
		t.Fatalf("the log holds %q", raw)
	}
	e := entry("ab12cd34", base)
	e.Log = path
	if err := Add(e); err != nil {
		t.Fatal(err)
	}
	got, _, _ := Find("ab12cd34")
	if got.Log != path {
		t.Fatalf("the entry's log is %q, want %q", got.Log, path)
	}
	// A path elsewhere is dropped, the entry kept.
	other := entry("cd34ef56", base.Add(time.Minute))
	other.Log = filepath.Join(t.TempDir(), "cd34ef56.log")
	if err := Add(other); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := Find("cd34ef56"); got.Log != "" {
		t.Fatalf("an entry named a log elsewhere: %q", got.Log)
	}
	// Remove takes the log with the entry.
	if err := Remove("ab12cd34"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the log after Remove: %v", err)
	}
	// Past Max, the oldest entry's log goes with it.
	for i := range Max + 1 {
		id := fmt.Sprintf("run%05d", i)
		e := entry(id, base.Add(time.Duration(i+2)*time.Minute))
		if e.Log, err = KeepLog(id, []string{"line"}); err != nil {
			t.Fatal(err)
		}
		if err := Add(e); err != nil {
			t.Fatal(err)
		}
	}
	first, _ := LogPath("run00000")
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("the oldest entry's log after %d more: %v", Max+1, err)
	}
	last, _ := LogPath(fmt.Sprintf("run%05d", Max))
	if _, err := os.Stat(last); err != nil {
		t.Fatalf("the newest entry's log: %v", err)
	}
	if err := Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the logs folder after Clear: %v", err)
	}
}

// A long log keeps its newest lines, after a line that counts the rest; a
// long cause is cut.
func TestAKeptLogAndACauseAreBounded(t *testing.T) {
	useTemp(t)
	long := strings.Repeat("x", 1000)
	var lines []string
	for range MaxLogBytes/1000 + 10 {
		lines = append(lines, long)
	}
	lines = append(lines, "the last line")
	path, err := KeepLog("ab12cd34", lines)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if len(raw) > MaxLogBytes {
		t.Fatalf("the log is %d bytes", len(raw))
	}
	if !strings.HasPrefix(string(raw), "(") || !strings.Contains(string(raw), "earlier lines are not kept here.)\n") || !strings.HasSuffix(string(raw), "the last line\n") {
		t.Fatalf("the log starts %q and ends %q", raw[:60], raw[len(raw)-40:])
	}
	e := entry("ab12cd34", time.Now())
	e.Cause = strings.Repeat("c", MaxCause+50)
	if err := Add(e); err != nil {
		t.Fatal(err)
	}
	got, _, _ := Find("ab12cd34")
	if n := len([]rune(got.Cause)); n != MaxCause || !strings.HasSuffix(got.Cause, "…") {
		t.Fatalf("the cause is %d characters, ending %q", n, got.Cause[len(got.Cause)-3:])
	}
}

// An entry that is not one (a hand-edited file: no id, a relative folder, a
// kind this app does not know) is dropped on read, never shown or used.
func TestAnEntryThatIsNotOneIsDropped(t *testing.T) {
	path := useTemp(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := `version: 1
runs:
  - {id: good1234, kind: run, where: here, folder: /abs/out, outcome: ok, endedAt: 2026-10-05T12:00:00Z}
  - {id: "../etc", kind: run, where: here, outcome: ok}
  - {id: rel12345, kind: run, where: here, folder: out/here, outcome: ok}
  - {id: dirty123, kind: run, where: here, folder: /abs/../etc, outcome: ok}
  - {id: kind1234, kind: shell, where: here, outcome: ok}
  - {id: where123, kind: run, where: there, outcome: ok}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	runs, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != "good1234" {
		t.Fatalf("read %+v, want only good1234", runs)
	}
	if _, ok, _ := Find("../etc"); ok {
		t.Fatal("Find returned an entry with no id")
	}
	if err := Add(Entry{ID: "abcd1234", Kind: KindRun, Where: WhereHere, Folder: "relative"}); err == nil {
		t.Fatal("Add kept a relative folder")
	}
}

// Remove drops one entry; Clear removes the file. Neither touches a folder.
func TestRemoveAndClear(t *testing.T) {
	path := useTemp(t)
	folder := t.TempDir()
	e := entry("ab12cd34", time.Now())
	e.Folder = folder
	if err := Add(e); err != nil {
		t.Fatal(err)
	}
	if err := Add(entry("ef56ab78", time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := Remove("ab12cd34"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Find("ab12cd34"); ok {
		t.Fatal("a removed entry is still found")
	}
	if _, ok, _ := Find("ef56ab78"); !ok {
		t.Fatal("Remove dropped another entry")
	}
	if err := Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("runs.yaml after Clear: %v", err)
	}
	if err := Clear(); err != nil {
		t.Fatalf("a second Clear: %v", err)
	}
	if _, err := os.Stat(folder); err != nil {
		t.Fatalf("the run folder after Remove and Clear: %v", err)
	}
}

// A file that does not parse is said, not written over; a file of another
// version reads as empty.
func TestAnUnreadableFileIsSaid(t *testing.T) {
	path := useTemp(t)
	_ = os.MkdirAll(filepath.Dir(path), 0o700)
	_ = os.WriteFile(path, []byte("runs: [unclosed"), 0o600)
	if _, err := List(); err == nil {
		t.Fatal("an unreadable history read as empty")
	}
	if err := Add(entry("ab12cd34", time.Now())); err == nil {
		t.Fatal("Add wrote over an unreadable history")
	}
	_ = os.WriteFile(path, []byte("version: 9\nruns: []\n"), 0o600)
	if runs, err := List(); err != nil || len(runs) != 0 {
		t.Fatalf("a newer version: %v, %v", runs, err)
	}
}
