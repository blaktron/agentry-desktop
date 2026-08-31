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
}

// PrereqStatus reports what an "Install prerequisites" click would do.
func (r *RunAPI) PrereqStatus() (*PrereqView, error) {
	p := run.DetectPrereqs(r.app.ctx)
	return &PrereqView{
		DockerOK: p.DockerOK, DockerErr: p.DockerErr,
		OS: p.OS, Distro: p.Distro,
		CanInstall: p.CanInstall, Command: p.Command,
		DownloadURL: p.DownloadURL, Note: p.Note,
	}, nil
}

// InstallDocker installs the engine through pkexec (the OS shows its own
// elevation dialog; this process never sees a password). Output streams as
// `runner:setup-progress` events; on success the caller typically follows
// with ApplySetup(pullImages=true) to stand up the gateway plane.
func (r *RunAPI) InstallDocker() (*PrereqView, error) {
	if !r.mu.TryLock() {
		return nil, errBusy
	}
	defer r.mu.Unlock()
	logw := r.eventWriter(eventSetupProgress)
	defer logw.Flush()
	if err := run.InstallDocker(r.app.ctx, logw); err != nil {
		return nil, err
	}
	return r.PrereqStatus()
}
