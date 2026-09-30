// Prerequisite bindings (Settings → "Install prerequisites"): detection of
// the docker engine story and, on Linux with polkit, a guided install. The
// gateway/harness/runner images stay ApplySetup's job once an engine
// answers; this surface only gets the engine there.
package main

import (
	"github.com/blaktron/agentry-desktop/internal/run"
)

// PrereqView mirrors run.Prereqs for the frontend.
type PrereqView struct {
	DockerOK    bool   `json:"dockerOk"`
	DockerErr   string `json:"dockerErr,omitempty"`
	OS          string `json:"os"`
	Distro      string `json:"distro,omitempty"`
	CanInstall  bool   `json:"canInstall"`
	Command     string `json:"command,omitempty"`
	DownloadURL string `json:"downloadUrl"`
	Note        string `json:"note,omitempty"`
	// State is run.State*: what the card shows and offers (agentry-desktop#64).
	State           string `json:"state"`
	EngineInstalled bool   `json:"engineInstalled"`
	InDockerGroup   bool   `json:"inDockerGroup"`
	GroupCommand    string `json:"groupCommand,omitempty"`
}

func prereqView(p *run.Prereqs) *PrereqView {
	return &PrereqView{
		DockerOK: p.DockerOK, DockerErr: p.DockerErr,
		OS: p.OS, Distro: p.Distro,
		CanInstall: p.CanInstall, Command: p.Command,
		DownloadURL: p.DownloadURL, Note: p.Note,
		State: p.State, EngineInstalled: p.EngineInstalled, InDockerGroup: p.InDockerGroup, GroupCommand: p.GroupCommand,
	}
}

// PrereqStatus reports what an "Install prerequisites" click would do.
func (r *RunAPI) PrereqStatus() (*PrereqView, error) {
	return prereqView(run.DetectPrereqs(r.app.ctx)), nil
}

// AddToDockerGroup runs only the usermod half of the install through
// pkexec, for a machine where the engine landed but the membership did
// not; detection afterwards says pendingRestart (agentry-desktop#64).
func (r *RunAPI) AddToDockerGroup() (*PrereqView, error) {
	if err := r.app.askConsent("Add your account to the docker group?", "This grants your account control of the Docker daemon, which is root-equivalent on this machine. An elevation prompt follows. Continue?"); err != nil {
		return nil, err
	}
	if err := r.acquire(jobSetup, "adding your account to the docker group"); err != nil {
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

// RestartNow asks the session manager to reboot (systemctl reboot; pkexec
// if polkit refuses). The page confirms first; the app never runs as root.
func (r *RunAPI) RestartNow() error {
	// The page confirms first; so does the OS, in a dialog the page cannot
	// answer (D-030): a reboot is not a thing script in the webview gets.
	if err := r.app.askConsent("Restart this machine now?", "Anything unsaved in other applications will be lost. Restart?"); err != nil {
		return err
	}
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	return run.Restart(r.app.ctx, logw)
}

// InstallDocker installs the engine through pkexec (the OS shows its own
// elevation dialog; this process never sees a password). Output streams as
// `runner:setup-progress` events; on success the caller typically follows
// with ApplySetup(pullImages=true) to stand up the gateway plane.
func (r *RunAPI) InstallDocker() (*PrereqView, error) {
	if err := r.app.askConsent("Install Docker Engine?", "The engine is installed through the OS package manager; an elevation prompt follows. Continue?"); err != nil {
		return nil, err
	}
	if err := r.acquire(jobSetup, "the Docker Engine install"); err != nil {
		return nil, err
	}
	defer r.release()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.InstallDocker(r.app.ctx, logw); err != nil {
		return nil, err
	}
	return r.PrereqStatus()
}
