package app

// Install the agentry command on a Mac whose Agentry Desktop carries it,
// live: the bound methods driven as Settings, the first-run wizard and the
// Scheduled page drive them, against an installed bundle's
// Contents/Resources/bin/agentry, with the native dialog answered Yes by the
// test. Skipped unless AGENTRY_LIVE_LINK_CLI names that CLI, because it
// writes a home's ~/.local/bin and shell startup file:
//
//	AGENTRY_LIVE_LINK_CLI="/Applications/Agentry Desktop.app/Contents/Resources/bin/agentry" \
//	HOME=<a scratch home> PATH=/usr/bin:/bin SHELL=/bin/zsh \
//	go test -run TestLiveLinkCLI -v ./internal/app
//
// It refuses to run in the account's own home.
//
// What it holds: before, the command is not installed and a new terminal
// finds none; Install links ~/.local/bin/agentry to the carried CLI and adds
// one marked PATH line to the shell's startup file; a new login shell of
// SHELL then runs it as `agentry` and it answers the carried version;
// FindCLI finds the link; a second Install changes nothing.

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/schedules"
)

func TestLiveLinkCLI(t *testing.T) {
	carried := os.Getenv("AGENTRY_LIVE_LINK_CLI")
	if carried == "" {
		t.Skip("AGENTRY_LIVE_LINK_CLI names an installed bundle's Contents/Resources/bin/agentry")
	}
	home := os.Getenv("HOME")
	if u, err := user.Current(); err != nil || home == "" || filepath.Clean(home) == filepath.Clean(u.HomeDir) {
		t.Fatalf("HOME must name a scratch home, not the account's own (%q)", home)
	}
	shell := os.Getenv("SHELL")
	f := &scheduleFake{answer: true}
	s := newScheduleAPI(t, f, false)
	s.findCLI = nil
	s.carriedCLI = func() string { return carried }

	before, err := s.CLICommand()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before: %+v", before)
	if !before.Offered || before.Status.Linked || before.Status.Resolves != "" || before.Label != installCLILabel {
		t.Fatalf("before: %+v", before)
	}

	got, err := s.InstallCLI()
	if err != nil {
		t.Fatalf("InstallCLI: %v", err)
	}
	t.Logf("installed %s at %s\nnotes: %q\n%s", got.Version, got.Path, got.Notes, got.Log)
	link := filepath.Join(home, ".local", "bin", "agentry")
	if dest, err := os.Readlink(link); err != nil || dest != carried {
		t.Fatalf("the link: %q %v", dest, err)
	}
	if rc, n := startupFileMarked(t, home); n != 1 {
		t.Fatalf("%s carries the marker %d times", rc, n)
	}

	// A new terminal: an interactive login shell, which reads the startup
	// file the install wrote.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-i", "-l", "-c", "command -v agentry; agentry --version").CombinedOutput()
	t.Logf("a new %s:\n%s", shell, out)
	if err != nil || !strings.Contains(string(out), link) || !strings.Contains(string(out), "agentry version "+got.Version) {
		t.Fatalf("a new terminal does not run the carried agentry %s at %s: %v", got.Version, link, err)
	}

	after, err := s.CLICommand()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after: %+v", after)
	if !after.Status.Linked || after.Status.Resolves != link || after.Status.ResolvesTo == "" || after.Label != reinstallCLILabel {
		t.Fatalf("after: %+v", after)
	}
	if p, err := schedules.FindCLI(); err != nil || p != link {
		t.Fatalf("FindCLI = %q, %v", p, err)
	}

	if _, err := s.InstallCLI(); err != nil {
		t.Fatalf("Reinstall: %v", err)
	}
	if _, n := startupFileMarked(t, home); n != 1 {
		t.Errorf("after a Reinstall the marker is there %d times", n)
	}
}
