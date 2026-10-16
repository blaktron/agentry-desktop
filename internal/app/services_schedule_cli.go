package app

// The agentry command from the Scheduled page's Options menu: on macOS and
// Linux the page offers to install it, or reinstall it, so that scheduling
// on this computer has the command it needs. A macOS bundle that carries the
// CLI links ~/.local/bin/agentry to it (cliinstall.Link), the same entry
// Settings and the first-run wizard offer; elsewhere the app runs the site's
// own install script.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"

	"github.com/blaktron/agentry-desktop/internal/cliinstall"
	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/selfupdate"
)

// The Options menu's two labels.
const (
	installCLILabel   = "Install the agentry command"
	reinstallCLILabel = "Reinstall the agentry command"
)

// cliVersion reads the version out of `agentry version 0.6.93 default
// registry: …`.
var cliVersion = regexp.MustCompile(`^agentry version (\S+)`)

// installingCLI makes a second press wait for the first install's answer
// rather than run the script twice at once.
var installingCLI sync.Mutex

// carried is the agentry command this macOS bundle carries, "" anywhere
// else or when the bundle carries none (a build from a PR).
func (s *ScheduleAPI) carried() string {
	if s.carriedCLI != nil {
		return s.carriedCLI()
	}
	if runtime.GOOS != "darwin" {
		return ""
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	c := cliinstall.Carried(exe)
	if fi, err := os.Stat(c); c == "" || err != nil || !fi.Mode().IsRegular() {
		return ""
	}
	return c
}

// cliInstallOffered reports whether this app installs the command here: on
// macOS and Linux, not as root, in a bundle that carries the command or a
// build whose update channel names a site.
func (s *ScheduleAPI) cliInstallOffered() bool {
	if s.installOffered != nil {
		return s.installOffered()
	}
	if !cliinstall.Offered(runtime.GOOS, os.Geteuid()) {
		return false
	}
	if s.carried() != "" {
		return true
	}
	_, err := cliinstall.ScriptURL(selfupdate.ManifestURL)
	return err == nil
}

// installCLILabelFor is the Options menu's entry, "" where it offers none.
// Where the bundle carries the command it says Install until the link is
// made and Reinstall after, and offers nothing while an agentry the app did
// not put there holds the place: that one is the command, and the app keeps
// a copy the install script put there up to date.
func (s *ScheduleAPI) installCLILabelFor() string {
	if !s.cliInstallOffered() {
		return ""
	}
	if c := s.carried(); c != "" {
		return linkLabel(cliinstall.Status(c, os.Getenv, nil))
	}
	locate := schedules.FindCLI
	if s.locateCLI != nil {
		locate = s.locateCLI
	}
	if _, err := locate(); err != nil {
		return installCLILabel
	}
	return reinstallCLILabel
}

// CLIInstallView is what an install did.
type CLIInstallView struct {
	// Path is the agentry command now installed.
	Path string `json:"path"`
	// Version is the version its `agentry --version` names, "" when it did
	// not answer.
	Version string `json:"version,omitempty"`
	// Notes are the script's lines worth reading: the PATH line it added and
	// what an open terminal needs.
	Notes []string `json:"notes,omitempty"`
	// Log is all the script printed.
	Log string `json:"log"`
}

// installCLIRun links the command the bundle carries, or runs the site's
// install script; a field in tests.
func (s *ScheduleAPI) installCLIRun(ctx context.Context) (cliinstall.Result, error) {
	if s.installRun != nil {
		return s.installRun(ctx)
	}
	if c := s.carried(); c != "" {
		r, err := cliinstall.Link(c, os.Getenv, cliinstall.ShellPath(ctx, os.Getenv("SHELL")))
		if errors.Is(err, cliinstall.ErrNotOurs) {
			return r, fmt.Errorf("%w. The app leaves it as it is and keeps an older copy the install script put there up to date; to use the command the app carries instead, remove %s and choose %s again", err, r.Path, installCLILabel)
		}
		return r, err
	}
	u, err := cliinstall.ScriptURL(selfupdate.ManifestURL)
	if err != nil {
		return cliinstall.Result{}, err
	}
	script, err := cliinstall.Fetch(ctx, nil, u)
	if err != nil {
		return cliinstall.Result{}, err
	}
	return cliinstall.Run(ctx, script, os.Environ(), runtime.GOOS, cliinstall.RunSh)
}

// installCLIQuestion is what the native dialog asks before an install: what
// runs, from where, and every file it touches. carried is the command the
// bundle carries, "" where the app runs the site's install script.
func installCLIQuestion(carried string) string {
	dir := cliinstall.Dir(os.Getenv)
	if dir == "" {
		dir = "~/.local/bin"
	}
	if carried != "" {
		return "Agentry Desktop puts the agentry command it carries on your PATH: it links " + filepath.Join(dir, cliinstall.Name) + " to " + carried +
			", so the command is the version this app was built with and updates when the app does. " +
			"If that folder is not on your PATH, it adds one marked line to your shell's startup file so that new terminals find it.\n\nInstall it?"
	}
	from := "this app's site"
	if u, err := cliinstall.ScriptURL(selfupdate.ManifestURL); err == nil {
		from = u
	}
	return "Agentry Desktop runs " + from + ", the install script that site serves. It downloads the agentry command the site lists for this computer, " +
		"refuses it unless its SHA-256 is the one the site lists, and puts it in " + dir + ", replacing the agentry there. " +
		"If that folder is not on your PATH, it adds one marked line to your shell's startup file so that new terminals find it.\n\nInstall it?"
}

// InstallCLI installs the agentry command from this build's site, or
// reinstalls it at the version the site serves now, and says what it did;
// nil when the person said No in the dialog.
func (s *ScheduleAPI) InstallCLI() (*CLIInstallView, error) {
	if !s.cliInstallOffered() {
		return nil, errors.New("this app does not install the agentry command here")
	}
	installingCLI.Lock()
	defer installingCLI.Unlock()
	if err := s.app.askConsent(installCLILabel+"?", installCLIQuestion(s.carried())); err != nil {
		if errors.Is(err, errNotConfirmed) {
			return nil, nil
		}
		return nil, err
	}
	ctx, cancel := context.WithTimeout(s.ctx(), cliinstall.Timeout)
	defer cancel()
	r, err := s.installCLIRun(ctx)
	if err != nil {
		return nil, err
	}
	v := &CLIInstallView{Path: r.Path, Notes: r.Notes, Log: r.Log}
	vctx, vcancel := context.WithTimeout(s.ctx(), cliQuickTimeout)
	defer vcancel()
	if line, err := schedules.New(r.Path).Version(vctx); err == nil {
		if m := cliVersion.FindStringSubmatch(line); m != nil {
			v.Version = m[1]
		}
	}
	emitEvent(s.app, eventSchedulesChanged)
	return v, nil
}

// linkLabel is the entry for a carried command in state st.
func linkLabel(st cliinstall.LinkStatus) string {
	switch {
	case st.Other || !st.Installable:
		return ""
	case st.Linked:
		return reinstallCLILabel
	}
	return installCLILabel
}

// CLICommandView is the agentry command as Settings and the first-run
// wizard show it on macOS.
type CLICommandView struct {
	// Offered: this bundle carries the command and installs it.
	Offered bool `json:"offered"`
	// Label is the button: Install, or Reinstall once the link is made.
	Label string `json:"label,omitempty"`
	// Version is the version the carried command answers with.
	Version string `json:"version,omitempty"`
	// Status is where the command is and what a terminal finds.
	Status cliinstall.LinkStatus `json:"status"`
}

// CLICommand says whether the command this bundle carries is on PATH and
// what a new terminal runs as `agentry`; Offered is false everywhere but a
// macOS bundle that carries it.
func (s *ScheduleAPI) CLICommand() (*CLICommandView, error) {
	c := s.carried()
	if c == "" || !s.cliInstallOffered() {
		return &CLICommandView{}, nil
	}
	ctx, cancel := context.WithTimeout(s.ctx(), cliQuickTimeout)
	defer cancel()
	lookup := s.shellLookup
	if lookup == nil {
		lookup = func() string { return cliinstall.ShellLookup(ctx, os.Getenv("SHELL")) }
	}
	st := cliinstall.Status(c, os.Getenv, lookup)
	v := &CLICommandView{Offered: true, Label: linkLabel(st), Status: st}
	if line, err := schedules.New(c).Version(ctx); err == nil {
		if m := cliVersion.FindStringSubmatch(line); m != nil {
			v.Version = m[1]
		}
	}
	return v, nil
}
