package app

// The agentry command from the Scheduled page's Options menu: on macOS and
// Linux the page offers to install it, or reinstall it, by running the
// site's own install script, so that scheduling on this computer has the
// command it needs.

import (
	"context"
	"errors"
	"os"
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

// cliInstallOffered reports whether this app installs the command here: on
// macOS and Linux, not as root, in a build whose update channel names a
// site.
func (s *ScheduleAPI) cliInstallOffered() bool {
	if s.installOffered != nil {
		return s.installOffered()
	}
	if !cliinstall.Offered(runtime.GOOS, os.Geteuid()) {
		return false
	}
	_, err := cliinstall.ScriptURL(selfupdate.ManifestURL)
	return err == nil
}

// installCLILabelFor is the Options menu's entry, "" where it offers none.
func (s *ScheduleAPI) installCLILabelFor() string {
	if !s.cliInstallOffered() {
		return ""
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

// installCLI runs the site's install script; a field in tests.
func (s *ScheduleAPI) installCLIRun(ctx context.Context) (cliinstall.Result, error) {
	if s.installRun != nil {
		return s.installRun(ctx)
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
// runs, from where, and every file it touches.
func installCLIQuestion() string {
	from := "this app's site"
	if u, err := cliinstall.ScriptURL(selfupdate.ManifestURL); err == nil {
		from = u
	}
	dir := cliinstall.Dir(os.Getenv)
	if dir == "" {
		dir = "~/.local/bin"
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
	if err := s.app.askConsent(installCLILabel+"?", installCLIQuestion()); err != nil {
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
