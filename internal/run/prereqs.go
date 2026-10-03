// Prerequisites detection and guided installation (Settings → "Install
// prerequisites"): QuickAgent runs need a docker engine; everything else
// (gateway/harness images, the runner image) is handled by ApplySetup once
// an engine answers. This file tells the settings page what is missing and
// — on Linux, where a package manager plus polkit make it tractable — can
// install the engine itself through pkexec, so the elevation prompt is the
// OS's own dialog and this process never handles the user's password.
package run

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
)

// osReleasePath is a var so tests can point detection at a fixture.
var osReleasePath = "/etc/os-release"

// The other seams detection reads, each a var so a test can hand it a
// fixture: the group database, the engine's socket, whether its unit is
// active, and the groups this process actually carries — which differ from
// /etc/group until the session restarts, and that difference is the
// pendingRestart state (agentry-desktop#64).
var (
	groupPath        = "/etc/group"
	dockerSocketPath = "/var/run/docker.sock"
	dockerUnitActive = func(ctx context.Context) bool {
		out, err := exec.CommandContext(ctx, "systemctl", "is-active", "docker").Output()
		return err == nil && strings.TrimSpace(string(out)) == "active"
	}
	processGroups = func() []int {
		gs, err := os.Getgroups()
		if err != nil {
			return nil
		}
		return gs
	}
	// restartCommand is what Restart runs; a var so tests never reboot.
	restartCommand = func(ctx context.Context, elevated bool) *exec.Cmd {
		if elevated {
			return exec.CommandContext(ctx, "pkexec", "systemctl", "reboot")
		}
		return exec.CommandContext(ctx, "systemctl", "reboot")
	}
	// elevatedShell is how the install and the group step run a command as
	// root: pkexec, so the OS shows its own dialog and this process never
	// handles a password. A var so tests run nothing.
	elevatedShell = func(ctx context.Context, command string) *exec.Cmd {
		return exec.CommandContext(ctx, "pkexec", "sh", "-c", command) //nolint:gosec // static distro table + whitelisted username
	}
	// lookPath is a var so tests can decide whether pkexec exists.
	lookPath = exec.LookPath
)

// The docker-engine states the page renders (plan desktop-linux-first-run
// D4). Each names a fact about the machine, not an error string.
const (
	// StateReady: the engine answers.
	StateReady = "ready"
	// StatePendingRestart: the engine is installed and the user is in the
	// docker group in /etc/group, but this session's groups do not include
	// it yet, so the socket refuses until the session restarts.
	StatePendingRestart = "pendingRestart"
	// StateGroupMissing: the engine is installed and the user is not in the
	// docker group — the install stopped after the package step.
	StateGroupMissing = "groupMissing"
	// StateInstallable: no engine, and this app can install one here.
	StateInstallable = "installable"
	// StateManual: no engine and no automatic install, or an engine that is
	// installed and refuses for a reason group membership does not explain
	// (DockerErr says what).
	StateManual = "manual"
)

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
	// State is one of the State* constants: what the page shows and which
	// action it offers (agentry-desktop#64).
	State string `json:"state"`
	// EngineInstalled is true when the docker unit is active or its socket
	// exists, whether or not this process may use it.
	EngineInstalled bool `json:"engineInstalled"`
	// InDockerGroup is true when /etc/group lists the user in docker.
	InDockerGroup bool `json:"inDockerGroup"`
	// GroupCommand is exactly what AddToDockerGroup would run.
	GroupCommand string `json:"groupCommand,omitempty"`
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
	if err := dockerAvailable(ctx, docker); err != nil {
		p.DockerErr = err.Error()
	} else {
		p.DockerOK = true
	}
	if runtime.GOOS != "linux" {
		p.State = StateManual
		if p.DockerOK {
			p.State = StateReady
		}
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
	uname, safeName := currentUsername()
	_, pkErr := lookPath("pkexec")
	// Both elevated commands carry the username into a root shell, so a
	// name outside the whitelist gets neither: better no button than a
	// command that would expand to something else as root.
	elevate := pkErr == nil && safeName
	if ok {
		p.Command = fmt.Sprintf("%s && systemctl enable --now docker && usermod -aG docker %s", pkg, uname)
		p.Note = "After installation, log out and back in (or reboot) so your user joins the docker group."
		p.CanInstall = elevate
	}
	if elevate {
		p.GroupCommand = "usermod -aG docker " + uname
	}
	p.EngineInstalled = engineInstalled(ctx)
	gid, inFile := dockerGroup(uname)
	p.InDockerGroup = inFile
	p.State = classify(p, gid)
	return p
}

// classify turns the facts into the one state the page renders.
func classify(p *Prereqs, dockerGID int) string {
	switch {
	case p.DockerOK:
		return StateReady
	case !p.EngineInstalled && p.CanInstall:
		return StateInstallable
	case !p.EngineInstalled:
		return StateManual
	case !p.InDockerGroup:
		return StateGroupMissing
	case !hasGroup(processGroups(), dockerGID):
		return StatePendingRestart
	default:
		// Installed, in the group here and now, and still refused: not a
		// group problem; DockerErr carries the engine's own words.
		return StateManual
	}
}

// engineInstalled: the unit answers "active", or the socket is there.
func engineInstalled(ctx context.Context) bool {
	if dockerUnitActive(ctx) {
		return true
	}
	_, err := os.Stat(dockerSocketPath)
	return err == nil
}

// dockerGroup reads the docker line of the group database: its gid and
// whether user is among its members. (-1, false) when there is no such
// group — an engine installed without one is its packaging's problem.
func dockerGroup(user string) (int, bool) {
	f, err := os.Open(groupPath)
	if err != nil {
		return -1, false
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) < 4 || fields[0] != "docker" {
			continue
		}
		gid, err := strconv.Atoi(fields[2])
		if err != nil {
			return -1, false
		}
		for _, m := range strings.Split(fields[3], ",") {
			if m == user {
				return gid, true
			}
		}
		return gid, false
	}
	return -1, false
}

func hasGroup(groups []int, gid int) bool {
	for _, g := range groups {
		if g == gid {
			return true
		}
	}
	return false
}

// AddToDockerGroup runs only the group half of the install through pkexec
// (agentry-desktop#64): for a machine where the engine landed but the
// membership did not. Detection afterwards reports pendingRestart, since
// this session's groups do not change until it restarts.
func AddToDockerGroup(ctx context.Context, logw io.Writer) error {
	p := DetectPrereqs(ctx)
	if p.State != StateGroupMissing {
		return fmt.Errorf("the docker group step does not apply: this machine's state is %s", p.State)
	}
	if p.GroupCommand == "" {
		return errors.New("the app cannot ask for elevation here (no pkexec, or a username it will not place in a root shell); run as root: usermod -aG docker <you>")
	}
	_, _ = fmt.Fprintf(logw, "requesting elevation to run: %s\n", p.GroupCommand)
	cmd := elevatedShell(ctx, p.GroupCommand)
	cmd.Stdout = logw
	cmd.Stderr = logw
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("add to docker group: %w", err)
	}
	_, _ = fmt.Fprintln(logw, "added to the docker group; the session has to restart before it counts")
	return nil
}

// Restart asks the session manager to reboot the machine: systemctl reboot
// as this user first (polkit grants an active local session that without a
// password on every distribution the app installs on), and through pkexec
// if polkit refuses. The app never runs as root; the confirmation that
// names what will happen is the page's, before this is called.
func Restart(ctx context.Context, logw io.Writer) error {
	_, _ = fmt.Fprintln(logw, "asking the session manager to reboot")
	plain := restartCommand(ctx, false)
	plain.Stdout, plain.Stderr = logw, logw
	if err := plain.Run(); err == nil {
		return nil
	}
	_, _ = fmt.Fprintln(logw, "the plain request was refused; asking with elevation")
	elevated := restartCommand(ctx, true)
	elevated.Stdout, elevated.Stderr = logw, logw
	if err := elevated.Run(); err != nil {
		return fmt.Errorf("reboot: %w", err)
	}
	return nil
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
	cmd := elevatedShell(ctx, p.Command)
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

// currentUsername returns the username and whether it is safe to place in
// a root shell: only [A-Za-z0-9._-]. A name outside that set (a directory
// account like DOM\user) still detects, but gets no elevated command.
func currentUsername() (string, bool) {
	u, err := user.Current()
	if err != nil {
		return "", false
	}
	for _, r := range u.Username {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.' {
			return u.Username, false
		}
	}
	return u.Username, true
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
