package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
	// A stand-in command that prints a version line: a shell script, or a
	// batch file on Windows, which has no /bin/sh.
	bin, script := filepath.Join(t.TempDir(), "agentry"), "#!/bin/sh\necho 'agentry version 0.6.93 default registry: https://secureagentry.net'\n"
	if runtime.GOOS == "windows" {
		bin, script = bin+".cmd", "@echo agentry version 0.6.93 default registry: https://secureagentry.net\r\n"
	}
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
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

// TestInstallCLILinksTheCarriedCommand: where the bundle carries the
// command, the entry says Install until ~/.local/bin/agentry links to it,
// the dialog says what is linked, the install makes the link and the PATH
// line, and Settings' view then reports it linked, at the carried version,
// and what a terminal finds. An agentry the app did not put there is
// refused with what to do about it.
func TestInstallCLILinksTheCarriedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command is linked on macOS; Windows carries it in its installer")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("XDG_BIN_HOME", "")
	t.Setenv("AGENTRY_INSTALL_DIR", "")
	carried := filepath.Join(t.TempDir(), "Agentry Desktop.app", "Contents", "Resources", "bin", "agentry")
	if err := os.MkdirAll(filepath.Dir(carried), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(carried, []byte("#!/bin/sh\necho 'agentry version 0.6.120 default registry: https://secureagentry.net'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &scheduleFake{answer: true}
	s := newScheduleAPI(t, f, true)
	s.findCLI = func(context.Context) (*schedules.CLI, error) { return nil, schedules.ErrNoCLI }
	s.carriedCLI = func() string { return carried }
	// CI runs as root, where the app offers no install.
	s.installOffered = func() bool { return true }
	link := filepath.Join(home, ".local", "bin", "agentry")
	s.shellLookup = func() string {
		if _, err := os.Lstat(link); err == nil {
			return link
		}
		return ""
	}
	emitEvent = func(*App, string, ...any) {}

	before, err := s.CLICommand()
	if err != nil || !before.Offered || before.Label != installCLILabel || before.Status.Linked || before.Status.Resolves != "" || before.Version != "0.6.120" {
		t.Fatalf("before: %+v %v", before, err)
	}
	got, err := s.InstallCLI()
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != link || got.Version != "0.6.120" {
		t.Fatalf("InstallCLI = %+v", got)
	}
	if len(f.asked) != 1 || !strings.Contains(f.asked[0], "links "+link+" to "+carried) {
		t.Fatalf("asked %q", f.asked)
	}
	if dest, err := os.Readlink(link); err != nil || dest != carried {
		t.Fatalf("the link: %q %v", dest, err)
	}
	if b, err := os.ReadFile(filepath.Join(home, ".zshrc")); err != nil || !strings.Contains(string(b), "# Added by the agentry CLI installer: "+filepath.Dir(link)) {
		t.Fatalf(".zshrc: %q %v", b, err)
	}
	after, err := s.CLICommand()
	// On macOS the temporary folder is itself behind a link (/var).
	real, _ := filepath.EvalSymlinks(carried)
	if err != nil || after.Label != reinstallCLILabel || !after.Status.Linked || after.Status.Resolves != link || after.Status.ResolvesTo != real {
		t.Fatalf("after: %+v %v", after, err)
	}
	if v, _ := s.Availability(); v.InstallCLI != reinstallCLILabel {
		t.Fatalf("the Scheduled page's entry after the link: %+v", v)
	}

	// The install script's copy in the way: refused, left as it is.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("the script's copy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.CLICommand(); !v.Status.Other || v.Label != "" {
		t.Fatalf("with the script's copy: %+v", v)
	}
	if v, _ := s.Availability(); v.InstallCLI != "" {
		t.Fatalf("the Scheduled page offers an install over the script's copy: %+v", v)
	}
	if _, err := s.InstallCLI(); !errors.Is(err, cliinstall.ErrNotOurs) || !strings.Contains(err.Error(), "remove "+link) {
		t.Fatalf("over the script's copy: %v", err)
	}
	if b, _ := os.ReadFile(link); string(b) != "the script's copy" {
		t.Fatalf("the script's copy became %q", b)
	}

	// Anywhere the bundle carries nothing, Settings shows nothing.
	s.carriedCLI = func() string { return "" }
	if v, _ := s.CLICommand(); v.Offered {
		t.Fatalf("no carried command: %+v", v)
	}
}
