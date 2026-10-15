package install

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"
)

// The install consent's shipped knowledge: each pack's size is read from the
// verified archive's headers, and the archive may not ship a pack the
// consented contract did not name.
func TestShippedPackSizesFromTheArchive(t *testing.T) {
	dir := t.TempDir()
	pack := knowledgetest.Make(t, "aws-docs", nil)
	if err := os.MkdirAll(filepath.Join(dir, "knowledge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := knowledge.CopyPack(pack.Dir, filepath.Join(dir, "knowledge", "aws-docs")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("Answer from the pack.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := pkgarchive.WalkWorkspace(dir)
	if err != nil {
		t.Fatal(err)
	}
	var arch bytes.Buffer
	if _, err := pkgarchive.BuildArchive(dir, manifest.ResolveInclude([]string{"AGENT.md", "knowledge/**"}, files), &arch); err != nil {
		t.Fatal(err)
	}
	sizes, err := shippedPackSizes(arch.Bytes(), []string{"aws-docs"})
	if err != nil {
		t.Fatal(err)
	}
	if sizes["aws-docs"] != pack.Manifest.Bytes {
		t.Fatalf("size %d, pack bytes %d", sizes["aws-docs"], pack.Manifest.Bytes)
	}
	if sizes, err := shippedPackSizes([]byte("not an archive"), []string{"aws-docs"}); err == nil {
		t.Fatalf("a broken archive read as %v", sizes)
	}
}

func TestTheArchiveMayNotShipMoreThanConsented(t *testing.T) {
	ship := func(names ...string) *manifest.Manifest {
		m := &manifest.Manifest{SchemaVersion: "2", Namespace: "acme", Name: "demo", Version: "1.0.0", Knowledge: &manifest.Knowledge{}}
		for _, n := range names {
			m.Knowledge.Ships = append(m.Knowledge.Ships, manifest.ShippedKnowledge{Name: n})
		}
		return m
	}
	plan := Plan{Namespace: "acme", Name: "demo", Version: "1.0.0"}
	if err := holdToConsented(ship("aws-docs"), ship("aws-docs", "extra"), plan); err == nil {
		t.Fatal("an archive shipping a pack the resolve did not name was installed")
	}
	if err := holdToConsented(ship("aws-docs", "extra"), ship("aws-docs"), plan); err != nil {
		t.Fatalf("a narrower archive was refused: %v", err)
	}
	if got := installed.Snapshot(ship("aws-docs").Contract()); !slices.Equal(got.Knowledge, []string{"aws-docs"}) {
		t.Fatalf("the recorded contract = %+v", got)
	}
}
