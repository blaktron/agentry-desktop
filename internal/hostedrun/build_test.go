package hostedrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The build rule is one table: absent signed out or without the knowledge
// extension; offered and refused in words for each missing precondition;
// available only with every scope, builds taken and nothing in flight.
func TestDecideBuild(t *testing.T) {
	all := []string{ScopeRunRead, ScopeRunWrite, ScopeKnowledgeRead, ScopeKnowledgeWrite}
	on := &BuildPrice{Enabled: true, HoldCredits: 100, ScanCredits: 1}
	ok := BuildFacts{SignedIn: true, Knowledge: true, Runs: true, Scopes: all, ScopesKnown: true, Price: on, StatusKnown: true}
	cases := []struct {
		name   string
		edit   func(*BuildFacts)
		offer  bool
		avail  bool
		reason string
	}{
		{"available", func(*BuildFacts) {}, true, true, ""},
		{"signed out", func(f *BuildFacts) { f.SignedIn = false }, false, false, ""},
		{"no knowledge extension", func(f *BuildFacts) { f.Knowledge = false }, false, false, ""},
		{"no runs extension", func(f *BuildFacts) { f.Runs = false }, true, false, "does not run anything In Agentry"},
		{"scopes unreadable", func(f *BuildFacts) { f.ScopesKnown = false }, true, false, "did not say what the stored token may do"},
		{"no knowledge scopes", func(f *BuildFacts) { f.Scopes = []string{ScopeRunRead, ScopeRunWrite} }, true, false, "knowledge:read and knowledge:write"},
		{"no run scopes", func(f *BuildFacts) { f.Scopes = []string{ScopeKnowledgeRead, ScopeKnowledgeWrite} }, true, false, "run:read, run:write"},
		{"status unknown", func(f *BuildFacts) { f.StatusKnown = false }, true, false, "did not answer"},
		{"builds not taken", func(f *BuildFacts) { f.Price = &BuildPrice{} }, true, false, "not taking knowledge builds"},
		{"an older registry", func(f *BuildFacts) { f.Price = nil }, true, false, "not taking knowledge builds"},
		{"a run in flight", func(f *BuildFacts) { f.Active = &Run{Status: StatusRunning} }, true, false, "already has a run In Agentry"},
		{"a build in flight", func(f *BuildFacts) { f.Active = &Run{Status: StatusQueued, Kind: KindKnowledgeBuild} }, true, false, "already has a build In Agentry"},
	}
	for _, c := range cases {
		f := ok
		c.edit(&f)
		o := DecideBuild(f)
		if o.Offered != c.offer || o.Available != c.avail || !strings.Contains(o.Reason, c.reason) {
			t.Errorf("%s: %+v", c.name, o)
		}
	}
}

func TestCanContinue(t *testing.T) {
	b := func(status, outcome, note string) *Run {
		return &Run{Kind: KindKnowledgeBuild, Status: status, Outcome: outcome, Build: &KnowledgeBuild{Note: note}}
	}
	yes := []*Run{b(StatusFinished, "budget", ""), b(StatusFinished, "timeout", ""), b(StatusFinished, "interrupted", ""),
		b(StatusFinished, "succeeded", "changed"), b(StatusCancelled, "", "")}
	no := []*Run{b(StatusFinished, "succeeded", ""), b(StatusFinished, "failed", ""), b(StatusFinished, "error", "invalid"),
		b(StatusRunning, "", ""), b(StatusQueued, "", ""), {Status: StatusFinished, Outcome: "budget"}, nil}
	for _, r := range yes {
		if !CanContinue(r) {
			t.Errorf("not continued: %+v", r)
		}
	}
	for _, r := range no {
		if CanContinue(r) {
			t.Errorf("continued: %+v", r)
		}
	}
}

// The notes say why, never what it cost (the settlement says that), and a
// note this app does not know says nothing rather than something wrong.
func TestNoteSentence(t *testing.T) {
	for _, n := range []string{"nothing", "changed", "allocation", "deleted", "invalid"} {
		if NoteSentence(n) == "" {
			t.Errorf("no sentence for %q", n)
		}
	}
	for _, n := range []string{"nothing", "changed", "allocation", "deleted"} {
		if strings.Contains(NoteSentence(n), "charged") || strings.Contains(NoteSentence(n), "refund") {
			t.Errorf("%q speaks of the charge: %s", n, NoteSentence(n))
		}
	}
	if NoteSentence("something-new") != "" || NoteSentence("") != "" {
		t.Error("an unknown note was given a sentence")
	}
}

// A folder is its regular files, hidden ones aside; two documents with one
// name, too many files or too many bytes are refused with the limit named.
func TestStageDocuments(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, text string) string {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("a.md", "a")
	write("sub/b.txt", "b")
	write(".git/config", "x")
	write(".env", "SECRET=1")
	loose := write("../loose.pdf", "c")
	got, err := StageDocuments([]string{dir, loose})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range got {
		names = append(names, f.Name)
	}
	if strings.Join(names, ",") != "a.md,b.txt,loose.pdf" {
		t.Fatalf("staged %v", names)
	}
	write("sub/a.md", "dup")
	if _, err := StageDocuments([]string{dir}); err == nil || !strings.Contains(err.Error(), "both named a.md") {
		t.Fatalf("two a.md: %v", err)
	}
	many := t.TempDir()
	for i := 0; i <= MaxInputFiles; i++ {
		if err := os.WriteFile(filepath.Join(many, strings.Repeat("f", i+1)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := StageDocuments([]string{many}); err == nil || !strings.Contains(err.Error(), "at most 20 documents") {
		t.Fatalf("too many: %v", err)
	}
	empty := t.TempDir()
	if _, err := StageDocuments([]string{empty}); err == nil || !strings.Contains(err.Error(), "no files") {
		t.Fatalf("an empty folder: %v", err)
	}
}
