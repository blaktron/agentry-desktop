package run

// A run is given the folder to copy its deliverables into, or none.

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// filesUnder is every regular file below dir, by slash path.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

func TestARunCopiesItsDeliverablesToTheFolderItIsGiven(t *testing.T) {
	_, regURL, fd, _ := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	req.DeliverTo = filepath.Join(t.TempDir(), "Agentry")
	toolsServerStarts(fd, req.OutDir)

	var log strings.Builder
	res, err := Run(context.Background(), req, &log)
	if err != nil || res.Status != "ok" {
		t.Fatalf("Run: %+v, %v\n%s", res, err, log.String())
	}
	// The fixture's final message is text, so the runner writes it down as
	// the run's deliverable and the run copies it.
	got := filesUnder(t, req.DeliverTo)
	if len(got) != 1 || !strings.HasPrefix(got[0], "reporter/") || !strings.HasSuffix(got[0], "/deliverable.md") {
		t.Fatalf("copies = %v, want reporter/<date> <title>/deliverable.md", got)
	}
	b, err := os.ReadFile(filepath.Join(req.DeliverTo, filepath.FromSlash(got[0])))
	if err != nil || !strings.Contains(string(b), "final answer") {
		t.Fatalf("the copy holds %q, %v", b, err)
	}
	// The run folder is still the whole record, and says what was copied.
	if _, err := os.Stat(filepath.Join(req.OutDir, "reports", "deliverable.md")); err != nil {
		t.Fatalf("the run folder lost the deliverable: %v", err)
	}
	if _, err := os.Stat(filepath.Join(req.OutDir, "receipts", "deliveries.ndjson")); err != nil {
		t.Fatalf("the run folder does not record the copy: %v", err)
	}
}

func TestARunGivenNoFolderCopiesNothing(t *testing.T) {
	_, regURL, fd, _ := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	toolsServerStarts(fd, req.OutDir)

	var log strings.Builder
	if res, err := Run(context.Background(), req, &log); err != nil || res.Status != "ok" {
		t.Fatalf("Run: %+v, %v\n%s", res, err, log.String())
	}
	if _, err := os.Stat(filepath.Join(req.OutDir, "receipts", "deliveries.ndjson")); err == nil {
		t.Fatal("a run given no folder recorded a delivery")
	}
}
