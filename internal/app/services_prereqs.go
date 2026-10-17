// Prerequisite bindings (Settings → Run defaults, the container engine
// card): detection of the engine this computer offers and, on Linux with
// polkit, a guided install of the distribution's own; on macOS the engine
// the app installs and runs itself. The
// gateway/harness/runner images stay ApplySetup's job once an engine
// answers; this surface only gets the engine there.
package app

import (
	"context"
	"errors"
	"strconv"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/windowsengine"
)

// EngineView is the container engine detection found, for the pages: what
// answers, where, how it was found, and the traps seen on the way.
type EngineView struct {
	Found    bool   `json:"found"`
	Kind     string `json:"kind,omitempty"`
	CLI      string `json:"cli,omitempty"`
	Version  string `json:"version,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	Rootless bool   `json:"rootless"`
	How      string `json:"how,omitempty"`
	// Summary is the one line a page shows for an engine found.
	Summary string `json:"summary,omitempty"`
	// Err is why nothing answered, in the installed engine's own words;
	// Tried each place asked and what it said.
	Err      string   `json:"err,omitempty"`
	Tried    []string `json:"tried"`
	Warnings []string `json:"warnings"`
	// Managed says the engine found is the app's own, which the engine
	// card offers to remove.
	Managed bool `json:"managed"`
}

func engineView(e run.Engine) EngineView {
	return EngineView{
		Found: e.Found, Kind: e.Kind, CLI: e.CLI, Version: e.Version, Endpoint: e.Endpoint, Rootless: e.Rootless,
		How: e.How, Summary: e.Summary(), Err: e.Err, Tried: emptyNotNil(e.Tried), Warnings: emptyNotNil(e.Warnings),
		Managed: e.IsManaged(),
	}
}

// PrereqView mirrors run.Prereqs for the frontend.
type PrereqView struct {
	DockerOK    bool       `json:"dockerOk"`
	DockerErr   string     `json:"dockerErr,omitempty"`
	Engine      EngineView `json:"engine"`
	OS          string     `json:"os"`
	Distro      string     `json:"distro,omitempty"`
	CanInstall  bool       `json:"canInstall"`
	Command     string     `json:"command,omitempty"`
	DownloadURL string     `json:"downloadUrl"`
	Note        string     `json:"note,omitempty"`
	// State is run.State*: what the card shows and offers.
	State           string `json:"state"`
	EngineInstalled bool   `json:"engineInstalled"`
	InDockerGroup   bool   `json:"inDockerGroup"`
	GroupCommand    string `json:"groupCommand,omitempty"`
	// Installs is the engine an install here would put (podman or docker);
	// SocketCommand what EnablePodmanSocket runs as the person.
	Installs      string   `json:"installs,omitempty"`
	SocketCommand string   `json:"socketCommand,omitempty"`
	Warnings      []string `json:"warnings"`
	// Managed is the engine the app manages itself (run.Managed).
	Managed run.Managed `json:"managed"`
	// Windows is the engine the app installs on Windows (run.WindowsEngine).
	Windows run.WindowsEngine `json:"windows"`
}

func prereqView(p *run.Prereqs) *PrereqView {
	return &PrereqView{
		DockerOK: p.DockerOK, DockerErr: p.DockerErr, Engine: engineView(p.Engine),
		OS: p.OS, Distro: p.Distro,
		CanInstall: p.CanInstall, Command: p.Command,
		DownloadURL: p.DownloadURL, Note: p.Note,
		State: p.State, EngineInstalled: p.EngineInstalled, InDockerGroup: p.InDockerGroup, GroupCommand: p.GroupCommand,
		Installs: p.Installs, SocketCommand: p.SocketCommand, Warnings: emptyNotNil(p.Warnings),
		Managed: p.Managed, Windows: p.Windows,
	}
}

// InstallManagedEngine installs the engine the app runs itself (Colima's,
// Lima's and the docker command's pinned release binaries under the app's
// own directory, no Homebrew, no administrator password), starts it and
// adopts it. Output streams as `runner:setup-progress` events.
func (r *RunAPI) InstallManagedEngine() (*PrereqView, error) {
	if err := r.app.askConsent("Install Agentry's engine?", "The app downloads "+run.ManagedVersions()+" ("+run.ManagedLicences()+", about "+strconv.Itoa(run.ManagedDownloadMB())+" MB) into its own directory, checks each file against the sum it pins, and starts a Linux virtual machine on this Mac that runs when the app runs. Nothing asks for your password. Continue?"); err != nil {
		return nil, err
	}
	if err := r.acquire(jobSetup, "the install of Agentry's engine"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.InstallManagedEngine(r.app.ctx, logw); err != nil {
		return nil, err
	}
	return r.PrereqStatus()
}

// InstallWindowsEngine installs Rancher Desktop for this user and starts it
// with its dockerd engine; where Windows Subsystem for Linux is missing it
// first asks Windows to install WSL (administrator consent), after which
// Windows must restart and the card offers the rest. Output streams as
// `runner:setup-progress` events.
func (r *RunAPI) InstallWindowsEngine() (*PrereqView, error) {
	if err := r.app.askConsent("Install Rancher Desktop?", "The app downloads Rancher Desktop "+windowsengine.RancherVersion+" (SUSE, "+windowsengine.Licence+", about "+strconv.Itoa(windowsengine.DownloadMB)+" MB) from its GitHub release, checks it against the SHA-512 the release publishes, and installs it for your account with its dockerd engine and Kubernetes off. If Windows Subsystem for Linux is not on yet, Windows first asks for administrator consent to install it and then needs a restart; after the restart, install again to finish. Rancher Desktop stays your own application, listed in Windows' installed apps. Continue?"); err != nil {
		return nil, err
	}
	if err := r.acquire(jobSetup, "the install of Rancher Desktop"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.InstallWindowsEngine(r.app.ctx, logw); err != nil && !errors.Is(err, run.ErrWindowsRestartNeeded) {
		return nil, err
	}
	return r.PrereqStatus()
}

// StartWindowsEngine starts an installed Rancher Desktop, for the card when
// its engine does not answer.
func (r *RunAPI) StartWindowsEngine() (*PrereqView, error) {
	if err := r.acquire(jobSetup, "the start of Rancher Desktop"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.StartWindowsEngine(r.app.ctx, logw); err != nil {
		return nil, err
	}
	return r.PrereqStatus()
}

// StartManagedEngine starts the app's installed engine, for the card when
// it is stopped.
func (r *RunAPI) StartManagedEngine() (*PrereqView, error) {
	if err := r.acquire(jobSetup, "the start of Agentry's engine"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.StartManagedEngine(r.app.ctx, logw); err != nil {
		return nil, err
	}
	return r.PrereqStatus()
}

// RemoveManagedEngine stops and deletes the app's engine with its data,
// leaving nothing of it but the app; detection then looks for another.
func (r *RunAPI) RemoveManagedEngine() (*PrereqView, error) {
	if err := r.app.askConsent("Remove Agentry's engine?", "The virtual machine, its images and the three binaries are deleted; runs on this computer need another engine afterwards. Continue?"); err != nil {
		return nil, err
	}
	if err := r.acquire(jobSetup, "the removal of Agentry's engine"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.RemoveManagedEngine(r.app.ctx, logw); err != nil {
		return nil, err
	}
	return r.PrereqStatus()
}

// PrereqStatus reports what the engine card should show and what a click
// would do.
func (r *RunAPI) PrereqStatus() (*PrereqView, error) {
	return prereqView(run.DetectPrereqs(r.app.ctx)), nil
}

// RedetectEngine runs the whole engine detection again, from nothing, and
// adopts what it finds: for a person who changed engines. It takes the run
// lock, since the engine every later run drives is what changes.
func (r *RunAPI) RedetectEngine() (*EngineView, error) {
	if err := r.acquire(jobSetup, "the container engine detection"); err != nil {
		return nil, err
	}
	defer r.release()
	e := engineView(run.DetectEngine(r.app.ctx, true))
	return &e, nil
}

// AddToDockerGroup runs only the usermod half of the install through pkexec,
// for a machine where the engine landed but the membership did not;
// detection afterwards says pendingRestart.
func (r *RunAPI) AddToDockerGroup() (*PrereqView, error) {
	if err := r.app.askConsent("Add your account to the docker group?", "This grants your account control of the Docker daemon, which is root-equivalent on this machine. An elevation prompt follows. Continue?"); err != nil {
		return nil, err
	}
	if err := r.acquire(jobSetup, "the change to your docker group membership"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.AddToDockerGroup(r.app.ctx, logw); err != nil {
		return nil, err
	}
	return r.PrereqStatus()
}

// EnablePodmanSocket turns podman's API socket on for this account, as the
// person: no elevation, no group, no restart. Detection afterwards finds
// podman.
func (r *RunAPI) EnablePodmanSocket() (*PrereqView, error) {
	if err := r.acquire(jobSetup, "turning podman's API socket on"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.EnablePodmanSocket(r.app.ctx, logw); err != nil {
		return nil, err
	}
	return r.PrereqStatus()
}

// RestartNow asks the session manager to reboot (systemctl reboot; pkexec
// if polkit refuses). The page confirms first; the app never runs as root.
func (r *RunAPI) RestartNow() error {
	// The page confirms first; so does the OS, in a dialog the page cannot
	// answer: a reboot is not a thing script in the webview gets.
	if err := r.app.askConsent("Restart this machine now?", "Anything unsaved in other applications will be lost. Restart?"); err != nil {
		return err
	}
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	return run.Restart(r.app.ctx, logw)
}

// InstallEngine installs the distribution's own engine through pkexec (the
// OS shows its own elevation dialog; this process never sees a password):
// podman on Fedora and the RHEL family, whose socket is then turned on as
// the person, Docker Engine elsewhere. Output streams as
// `runner:setup-progress` events; on success the caller typically follows
// with ApplySetup(pullImages=true) to stand up the gateway plane.
func (r *RunAPI) InstallEngine() (*PrereqView, error) {
	title, body := "Install Docker Engine?", "The engine is installed through the OS package manager; an elevation prompt follows. Continue?"
	if run.InstallTarget() == dockerx.KindPodman {
		title, body = "Install podman?", "podman is installed through the OS package manager; an elevation prompt follows. Its API socket is then turned on for your account, with no elevation. Continue?"
	}
	if err := r.app.askConsent(title, body); err != nil {
		return nil, err
	}
	if err := r.acquire(jobSetup, "the container engine install"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.InstallEngine(r.app.ctx, logw); err != nil {
		return nil, err
	}
	return r.PrereqStatus()
}

// startManagedEngine is the app's own start of the engine it manages, when
// it opens: nothing where none is installed. It holds the setup lock while
// the VM boots, so a click that would drive the engine waits for it; what
// it does streams as `runner:setup-progress` events.
func (r *RunAPI) startManagedEngine(ctx context.Context) {
	if err := r.acquire(jobSetup, "the start of Agentry's engine"); err != nil {
		return
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	run.StartManagedEngineAtLaunch(ctx, logw)
	run.StartWindowsEngineAtLaunch(ctx, logw)
}

// stopManagedEngine stops the engine the app manages when the app quits;
// by here the run in flight, if any, was ended (beforeClose).
func (r *RunAPI) stopManagedEngine(ctx context.Context) {
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	run.StopManagedEngine(ctx, logw)
}
