package build

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/draftproof"
)

// A revision the Drafter wrote reads into the form as Edit reads the
// release: the QuickAgent's own name at the version given, the unheld
// fields dropped, the release's licence unless the person's words name
// the draft's.
func TestFromRevisionKeepsTheQuickAgentAndTheReleasesLicence(t *testing.T) {
	drafted := `{"schemaVersion":"2","name":"meeting-actions-v2","namespace":"draft","version":"0.1.0","summary":"Turns notes into actions with dates.",
	  "license":"MIT","homepage":"https://example.test","modes":["autonomous"],"entrypoints":{"instruction":"AGENT.md"},
	  "capabilities":{"network":{"egress":["slack.com"]},"filesystem":{"read":"workspace","write":"none"},"exec":"none"}}`
	m, err := FromRevision([]byte(drafted), "# Actions\n", "chris", "meeting-actions", "1.3.0", "Apache-2.0", "Add dates written as 3 March.")
	if err != nil {
		t.Fatal(err)
	}
	if m.Namespace != "chris" || m.Name != "meeting-actions" || m.Version != "1.3.0" || m.License != "Apache-2.0" || m.Homepage != "" ||
		m.Summary != "Turns notes into actions with dates." || m.Instructions != "# Actions\n" {
		t.Fatalf("%+v", m)
	}
	// The person asked for the draft's licence by name: it is kept.
	m, err = FromRevision([]byte(drafted), "# Actions\n", "chris", "meeting-actions", "1.3.0", "Apache-2.0", "Relicense it under MIT, please.")
	if err != nil || m.License != "MIT" {
		t.Fatalf("named licence: %q %v", m.License, err)
	}
	// A word that only contains it is not naming it.
	m, _ = FromRevision([]byte(drafted), "x", "chris", "meeting-actions", "1.3.0", "Apache-2.0", "Make it submittable.")
	if m.License != "Apache-2.0" {
		t.Fatalf("submittable names MIT? %q", m.License)
	}
}

func TestLineDiffTrimsFoldsAndMarksChanges(t *testing.T) {
	var a, b []string
	for i := 0; i < 20; i++ {
		a = append(a, "line "+string(rune('a'+i)))
	}
	b = append(b, a...)
	b[10] = "line K changed"
	b = append(b, "a new last line")
	f := LineDiff("AGENT.md", strings.Join(a, "\n")+"\n", strings.Join(b, "\n"))
	if f.Same || f.Path != "AGENT.md" {
		t.Fatalf("%+v", f)
	}
	var ops strings.Builder
	for _, l := range f.Lines {
		ops.WriteString(l.Op)
	}
	// 7 folded, 3 context, the change, 3 context, a fold of 3, then the
	// last 3 and the addition.
	if got := ops.String(); got != "~   -+   ~   +" {
		t.Fatalf("ops %q:\n%+v", got, f.Lines)
	}
	if f.Lines[0].Text != "7 unchanged lines" || f.Lines[4].Text != "line k" || f.Lines[5].Text != "line K changed" {
		t.Fatalf("%+v", f.Lines)
	}
	if s := LineDiff("agent.yaml", "a\r\nb\n", "a\nb"); !s.Same || len(s.Lines) != 0 {
		t.Fatalf("line breaks alone are not a change: %+v", s)
	}
}

// A revision published as drafted proves as the draft: the form's own
// writer, saving what FromRevision read, gives the same draftproof as the
// package the sandbox host recorded. testdata/revision is a real hosted
// revision's draft.json and package/, whose guide.prepare items hold
// commas.
func TestASavedRevisionProvesAsTheDraftsPackage(t *testing.T) {
	raw, err := os.ReadFile("testdata/revision/draft.json")
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Manifest     json.RawMessage `json:"manifest"`
		Instructions string          `json:"instructions"`
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	m, err := FromRevision(d.Manifest, d.Instructions, "ses-t748-pro-727604", "talk-only-check", "0.7.0", "Apache-2.0", "")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	want, _, err := draftproof.OfDir("testdata/revision/package")
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := draftproof.OfDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("the saved revision proves as %+v, the draft's package as %+v", got, want)
	}
}
