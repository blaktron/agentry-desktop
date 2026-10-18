package deliverables

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A copy this app collects carries the Mark-of-the-Web, the Zone.Identifier
// stream Windows keeps beside a file that came from outside the computer, so
// Explorer and Office treat what an agent wrote as the download it is.
func TestACollectedCopyCarriesTheMarkOfTheWeb(t *testing.T) {
	r := newRun(t)
	r.put("reports/summary.md", []byte("# Summary\n"))
	r.envelope(nil)
	root := filepath.Join(t.TempDir(), "Agentry")
	got, err := Collect(r.dir, root)
	if err != nil || len(got) != 1 || got[0].Status != "delivered" {
		t.Fatalf("Collect = %+v, %v", got, err)
	}
	b, err := os.ReadFile(got[0].Object + ":Zone.Identifier")
	if err != nil {
		t.Fatalf("the copy %s has no Zone.Identifier stream: %v", got[0].Object, err)
	}
	if !strings.Contains(string(b), "ZoneId=3") {
		t.Fatalf("Zone.Identifier = %q", b)
	}
}
