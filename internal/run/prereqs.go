// Prerequisites detection and guided installation (Settings → "Install
// prerequisites"): BriefAgent runs need a docker engine; everything else
// (gateway/harness images, the runner image) is handled by ApplySetup once
// an engine answers. This file tells the settings page what is missing and
// — on Linux, where a package manager plus polkit make it tractable — can
// install the engine itself through pkexec, so the elevation prompt is the
// OS's own dialog and this process never handles the user's password.
package run

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

// osReleasePath is a var so tests can point detection at a fixture.
var osReleasePath = "/etc/os-release"

// Prereqs is the machine's docker-engine story, as the settings page tells it.
type Prereqs struct {
	// DockerOK / DockerErr mirror SetupStatus (the engine answers or not).
	DockerOK  bool   `json:"dockerOk"`
	DockerErr string `json:"dockerErr,omitempty"`
	// OS is runtime.GOOS; Distro is /etc/os-release ID on Linux.
	OS     string `json:"os"`
	Distro string `json:"distro,omitempty"`
	// CanInstall reports whether InstallDocker can do the work here:
	// Linux, a recognized package manager, and pkexec present.
	CanInstall bool `json:"canInstall"`
	// Command is exactly what InstallDocker would run (shown before consent).
	Command string `json:"command,omitempty"`
	// DownloadURL is the hand-off for platforms the app cannot install on.
	DownloadURL string `json:"downloadUrl"`
	// Note carries the honest caveat (group membership needs a re-login).
	Note string `json:"note,omitempty"`
}

// distroInstall maps an os-release ID (or ID_LIKE token) to the engine
// install command. Distribution packages only — no piping remote scripts
// into a root shell.
var distroInstall = map[string]string{
	"fedora": "dnf install -y moby-engine",
	"rhel":   "dnf install -y moby-engine",
	"centos": "dnf install -y moby-engine",
	"debian": "apt-get update && apt-get install -y docker.io",
	"ubuntu": "apt-get update && apt-get install -y docker.io",
	"arch":   "pacman -S --noconfirm docker",
	"suse":   "zypper --non-interactive install docker",
}

// DetectPrereqs reports what a "make this machine runnable" click would do.
func DetectPrereqs(ctx context.Context) *Prereqs {
	p := &Prereqs{OS: runtime.GOOS, DownloadURL: "https://docs.docker.com/engine/install/"}
	if runtime.GOOS != "linux" {
		p.DownloadURL = "https://www.docker.com/products/docker-desktop/"
	}
	docker := newDockerClient()
	if err := docker.Available(ctx); err != nil {
		p.DockerErr = err.Error()
	} else {
		p.DockerOK = true
	}
	if runtime.GOOS != "linux" {
		return p
	}

	p.Distro = readOSReleaseID()
	pkg, ok := distroInstall[p.Distro]
	if !ok {
		for _, like := range readOSReleaseIDLike() {
			if c, hit := distroInstall[like]; hit {
				pkg, ok = c, true
				break
			}
		}
	}
	if !ok {
		return p
	}
	uname := currentUsername()
	p.Command = fmt.Sprintf("%s && systemctl enable --now docker && usermod -aG docker %s", pkg, uname)
	p.Note = "After installation, log out and back in (or reboot) so your user joins the docker group."
	if _, err := exec.LookPath("pkexec"); err == nil {
		p.CanInstall = true
	}
	return p
}

// InstallDocker performs the detected install through pkexec — the polkit
// agent shows the OS's own elevation dialog. Output streams to logw.
func InstallDocker(ctx context.Context, logw io.Writer) error {
	p := DetectPrereqs(ctx)
	if p.DockerOK {
		_, _ = fmt.Fprintln(logw, "docker already answers — nothing to install")
		return nil
	}
	if !p.CanInstall {
		return fmt.Errorf("cannot install the docker engine automatically on this system (%s/%s) — see %s", p.OS, p.Distro, p.DownloadURL)
	}
	_, _ = fmt.Fprintf(logw, "requesting elevation to run: %s\n", p.Command)
	cmd := exec.CommandContext(ctx, "pkexec", "sh", "-c", p.Command) //nolint:gosec // command is assembled from the static distro table + validated username
	cmd.Stdout = logw
	cmd.Stderr = logw
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install docker: %w", err)
	}
	_, _ = fmt.Fprintln(logw, "docker engine installed and started")
	if p.Note != "" {
		_, _ = fmt.Fprintln(logw, p.Note)
	}
	return nil
}

// currentUsername returns a shell-safe username for the usermod call; a
// name outside the conservative set falls back to skipping the group step.
func currentUsername() string {
	u, err := user.Current()
	if err != nil {
		return "$(id -un)"
	}
	for _, r := range u.Username {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.' {
			return "$(id -un)"
		}
	}
	return u.Username
}

func readOSReleaseID() string { return osReleaseField("ID") }
func readOSReleaseIDLike() []string {
	return strings.Fields(osReleaseField("ID_LIKE"))
}

func osReleaseField(key string) string {
	f, err := os.Open(osReleasePath)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if v, ok := strings.CutPrefix(line, key+"="); ok {
			return strings.ToLower(strings.Trim(v, `"'`))
		}
	}
	return ""
}
