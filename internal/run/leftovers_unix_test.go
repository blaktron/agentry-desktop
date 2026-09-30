//go:build unix

package run

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// The sweep deletes nothing through a link. One standing where a run's
// scratch directory would be is removed as a link; one standing for the
// runs directory, the release, the QuickAgent or the namespace is not
// descended at all.
func TestTheSweepDeletesNothingThroughALink(t *testing.T) {
	const runID = "dead0001"
	// <home>/agents/acme/reporter/1.0.0-0123456789ab/runs/<id>: real down
	// to the level under test, a link from there to a tree elsewhere that
	// holds the rest of the path and a file that is not the run's.
	parts := []string{"agents", "acme", "reporter", "1.0.0-0123456789ab", "runs", runID}
	for level, depth := range map[string]int{"namespace": 1, "agent": 2, "release": 3, "runs": 4, "scratch": 5} {
		t.Run(level, func(t *testing.T) {
			home := isolateRunEnv(t)
			withFakeDocker(t, &dockerx.Fake{})
			elsewhere := filepath.Join(t.TempDir(), "documents")
			mine := filepath.Join(append([]string{elsewhere}, parts[depth+1:]...)...)
			if err := os.MkdirAll(mine, 0o755); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(mine, "thesis.txt")
			if err := os.WriteFile(file, []byte("mine"), 0o600); err != nil {
				t.Fatal(err)
			}
			real := filepath.Join(append([]string{home}, parts[:depth]...)...)
			if err := os.MkdirAll(real, 0o755); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(real, parts[depth])
			if err := os.Symlink(elsewhere, link); err != nil {
				t.Fatal(err)
			}
			through := filepath.Join(append([]string{home}, append(parts, "thesis.txt")...)...)
			if !exists(through) {
				t.Fatalf("the fixture's file is not reachable through the link: %s", through)
			}

			if err := Sweep(context.Background(), runID); err != nil {
				t.Fatal(err)
			}
			if !exists(file) {
				t.Fatal("the sweep deleted through a link")
			}
			_, err := os.Lstat(link)
			if gone := err != nil; gone != (level == "scratch") {
				t.Fatalf("the link at the %s level: gone is %v", level, gone)
			}
		})
	}
}

// The workspace in a run's scratch directory is a copy of the release with
// its own mode bits, so it can hold a directory its owner may not write. It
// goes all the same, and the secrets file with it: a release cannot make a
// run's record and scratch directory permanent.
func TestTheSweepRemovesADirectoryTheReleaseShippedReadOnly(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root deletes from a read-only directory without the second try")
	}
	home := isolateRunEnv(t)
	scratch := scratchOf(t, home, "dead0001")
	locked := filepath.Join(scratch, "workspace", "vendor", "locked")
	if err := os.MkdirAll(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, "data"), []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{locked, filepath.Dir(locked)} {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(locked), 0o755); _ = os.Chmod(locked, 0o755) })
	if err := os.RemoveAll(scratch); err == nil {
		t.Fatal("the fixture's read-only directory did not refuse a plain delete")
	}
	scratchOf(t, home, "dead0001") // the refused delete took the secrets file; put the run back whole

	if err := SweepFiles(home, "dead0001"); err != nil {
		t.Fatal(err)
	}
	if exists(scratch) {
		t.Fatal("the scratch directory is still there")
	}
}
