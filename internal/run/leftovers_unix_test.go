//go:build unix

package run

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// A link standing where a run's scratch directory would be is removed as a
// link: the sweep deletes nothing through it.
func TestTheSweepRemovesALinkAndNotWhatItPointsAt(t *testing.T) {
	home := isolateRunEnv(t)
	withFakeDocker(t, &dockerx.Fake{})
	target := filepath.Join(t.TempDir(), "documents")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "thesis.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	runs := filepath.Join(home, "agents", "acme", "reporter", "1.0.0-0123456789ab", "runs")
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(runs, "dead0001")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := Sweep(context.Background(), "dead0001"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Error("the link is still there")
	}
	if !exists(filepath.Join(target, "thesis.txt")) {
		t.Fatal("the sweep deleted through a link")
	}
}
