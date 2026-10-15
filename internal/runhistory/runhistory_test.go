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
// task, the inputs, the result, a secret or the log, because the entry has
// no field for any of them.
func TestTheFileIsPrivateAndHoldsNoTaskInputResultOrLog(t *testing.T) {
	path := useTemp(t)
	if err := Add(entry("ab12cd34", time.Now())); err != nil {
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
	want := "ID Kind Where Registry Ref Digest StartedAt EndedAt Outcome ExitCode ExitClass Folder"
	if strings.Join(fields, " ") != want {
		t.Fatalf("an entry holds %v; a new field must not carry the task, inputs, result, a secret or the log, and this test is changed with it", fields)
	}
	raw, _ := os.ReadFile(path)
	for _, word := range []string{"task", "input", "result", "log", "token", "secret"} {
		if strings.Contains(strings.ToLower(string(raw)), word) {
			t.Errorf("runs.yaml mentions %q:\n%s", word, raw)
		}
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
