package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/cliinstall"
	"github.com/blaktron/agentry-desktop/internal/schedules"
)

// TestInstallCLIOffersAndRuns: the Options menu's entry says Install where
// no command is found and Reinstall where one is, the install reports the
// command's path, version and notes and refreshes the page, and nothing is
// offered or run where the app does not install it.
func TestInstallCLIOffersAndRuns(t *testing.T) {
	f := &scheduleFake{answer: true}
	s := newScheduleAPI(t, f, true)
	found := false
	s.locateCLI = func() (string, error) {
		if found {
			return "/home/ada/.local/bin/agentry", nil
		}
		return "", schedules.ErrNoCLI
	}
	s.findCLI = func(context.Context) (*schedules.CLI, error) { return nil, schedules.ErrNoCLI }

	offered := false
	s.installOffered = func() bool { return offered }
	ran := 0
	bin := filepath.Join(t.TempDir(), "agentry")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'agentry version 0.6.93 default registry: https://secureagentry.net'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.installRun = func(context.Context) (cliinstall.Result, error) {
		ran++
		return cliinstall.Result{Path: bin, Notes: []string{"Added /home/ada/.local/bin to PATH in /home/ada/.bashrc"}, Log: "Installed agentry 0.6.93"}, nil
	}
	var emitted []string
	emitEvent = func(_ *App, name string, _ ...any) { emitted = append(emitted, name) }

	v, err := s.Availability()
	if err != nil || v.InstallCLI != "" || v.Available {
		t.Fatalf("not offered: %+v %v", v, err)
	}
	if _, err := s.InstallCLI(); err == nil || ran != 0 {
		t.Fatalf("an install where none is offered ran: %v (%d)", err, ran)
	}

	offered = true
	if v, _ := s.Availability(); v.InstallCLI != installCLILabel || v.Reason != schedules.ErrNoCLI.Error() {
		t.Fatalf("no command: %+v", v)
	}
	got, err := s.InstallCLI()
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != bin || got.Version != "0.6.93" || len(got.Notes) != 1 || got.Log == "" {
		t.Errorf("InstallCLI = %+v", got)
	}
	if len(emitted) != 1 || emitted[0] != eventSchedulesChanged {
		t.Errorf("emitted %v", emitted)
	}
	if len(f.asked) != 1 || !strings.Contains(f.asked[0], "SHA-256") || !strings.Contains(f.asked[0], "startup file") {
		t.Errorf("asked %q", f.asked)
	}
	// No in the dialog runs nothing.
	f.answer = false
	if got, err := s.InstallCLI(); got != nil || err != nil || ran != 1 {
		t.Fatalf("a No: %+v %v (%d runs)", got, err, ran)
	}
	f.answer = true
	found = true
	if v, _ := s.Availability(); v.InstallCLI != reinstallCLILabel {
		t.Fatalf("a command found: %+v", v)
	}

	s.installRun = func(context.Context) (cliinstall.Result, error) {
		return cliinstall.Result{}, errors.New("the download's SHA-256 is aa, but https://x lists bb; nothing was installed")
	}
	if _, err := s.InstallCLI(); err == nil || !strings.Contains(err.Error(), "nothing was installed") {
		t.Fatalf("a refused install: %v", err)
	}
}
