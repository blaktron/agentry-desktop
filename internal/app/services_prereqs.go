// Prerequisite bindings (Settings → Run defaults, the container engine
// card): detection of the engine this computer offers and, on Linux with
// polkit, a guided install of the distribution's own. The
// gateway/harness/runner images stay ApplySetup's job once an engine
// answers; this surface only gets the engine there.
package app

import (
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"

	"github.com/blaktron/agentry-desktop/internal/run"
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
}

func engineView(e run.Engine) EngineView {
	return EngineView{
		Found: e.Found, Kind: e.Kind, CLI: e.CLI, Version: e.Version, Endpoint: e.Endpoint, Rootless: e.Rootless,
		How: e.How, Summary: e.Summary(), Err: e.Err, Tried: emptyNotNil(e.Tried), Warnings: emptyNotNil(e.Warnings),
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
}

func prereqView(p *run.Prereqs) *PrereqView {
	return &PrereqView{
		DockerOK: p.DockerOK, DockerErr: p.DockerErr, Engine: engineView(p.Engine),
		OS: p.OS, Distro: p.Distro,
		CanInstall: p.CanInstall, Command: p.Command,
		DownloadURL: p.DownloadURL, Note: p.Note,
		State: p.State, EngineInstalled: p.EngineInstalled, InDockerGroup: p.InDockerGroup, GroupCommand: p.GroupCommand,
		Installs: p.Installs, SocketCommand: p.SocketCommand, Warnings: emptyNotNil(p.Warnings),
	}
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
