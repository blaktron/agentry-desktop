// Prerequisites detection and guided installation (Settings → Run
// defaults, the container engine card): QuickAgent runs need a container
// engine; everything else (gateway/harness images, the runner image) is
// handled by ApplySetup once an engine answers. This file tells the page
// what is missing and — on Linux, where a package manager plus polkit make
// it tractable — can install the distribution's own engine itself: podman
// on Fedora and the RHEL family, Docker Engine elsewhere. An install runs
// through pkexec, so the elevation prompt is the OS's own dialog and this
// process never handles the user's password; podman's API socket is turned
// on as the person, with no elevation, no group and no restart.
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

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// osReleasePath is a var so tests can point detection at a fixture.
var osReleasePath = "/etc/os-release"

// The other seams detection reads, each a var so a test can hand it a
// fixture: the group database, the engine's socket, whether its unit is
// active, and the groups this process actually carries — which differ from
// /etc/group until the session restarts, and that difference is the
// pendingRestart state.
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
	// userShell is how a command runs as the person, unelevated: podman's
	// socket is a user unit. A var so tests run nothing.
	userShell = func(ctx context.Context, command string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", command) //nolint:gosec // a fixed command of this file's
	}
	// lookPath is a var so tests can decide whether pkexec exists.
	lookPath = exec.LookPath
)

// The engine states the page renders. Each names a fact about the
// machine, not an error string.
const (
	// StateReady: the engine answers.
	StateReady = "ready"
	// StateSocketOff: podman is installed and its API socket is not on,
	// which one click turns on for this account.
	StateSocketOff = "socketOff"
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

// Prereqs is the machine's container-engine story, as the settings page
// tells it.
type Prereqs struct {
	// DockerOK / DockerErr mirror SetupStatus (the engine answers or not).
	DockerOK  bool   `json:"dockerOk"`
	DockerErr string `json:"dockerErr,omitempty"`
	// Engine is what detection found, or what it saw on the way.
	Engine Engine `json:"engine"`
	// Installs is the engine an install here would put: podman or docker.
	Installs string `json:"installs,omitempty"`
	// SocketCommand is exactly what EnablePodmanSocket would run, as the
	// person, when podman is installed and its socket is off.
	SocketCommand string `json:"socketCommand,omitempty"`
	// Warnings are the traps detection saw (Engine.Warnings), for the page.
	Warnings []string `json:"warnings"`
	// OS is runtime.GOOS; Distro is /etc/os-release ID on Linux.
	OS     string `json:"os"`
	Distro string `json:"distro,omitempty"`
	// CanInstall reports whether InstallEngine can do the work here:
	// Linux, a recognized package manager, and pkexec present.
	CanInstall bool `json:"canInstall"`
	// Command is exactly what InstallEngine would run (shown before consent).
	Command string `json:"command,omitempty"`
	// DownloadURL is the hand-off for platforms the app cannot install on.
	DownloadURL string `json:"downloadUrl"`
	// Note carries the honest caveat (group membership needs a re-login).
	Note string `json:"note,omitempty"`
	// State is one of the State* constants: what the page shows and which
	// action it offers.
	State string `json:"state"`
	// EngineInstalled is true when the docker unit is active or its socket
	// exists, or podman is installed, whether or not this process may use it.
	EngineInstalled bool `json:"engineInstalled"`
	// InDockerGroup is true when /etc/group lists the user in docker.
	InDockerGroup bool `json:"inDockerGroup"`
	// GroupCommand is exactly what AddToDockerGroup would run.
	GroupCommand string `json:"groupCommand,omitempty"`
}

// distroInstall maps an os-release ID (or ID_LIKE token) to the
// distribution's own engine and its install command: podman where the
// distribution ships and maintains it as its engine (Fedora and the RHEL
// family), Docker Engine elsewhere. Distribution packages only — no piping
// remote scripts into a root shell.
type engineInstall struct {
	engine  string
	command string
}

var distroInstall = map[string]engineInstall{
	"fedora": {dockerx.KindPodman, "dnf install -y podman"},
	"rhel":   {dockerx.KindPodman, "dnf install -y podman"},
	"centos": {dockerx.KindPodman, "dnf install -y podman"},
	"debian": {dockerx.KindDocker, "apt-get update && apt-get install -y docker.io"},
	"ubuntu": {dockerx.KindDocker, "apt-get update && apt-get install -y docker.io"},
	"arch":   {dockerx.KindDocker, "pacman -S --noconfirm docker"},
	"suse":   {dockerx.KindDocker, "zypper --non-interactive install docker"},
}

// DetectPrereqs reports what the page should show and what a click would
// do. It runs the engine detection (an engine adopted earlier that still
// answers is kept), then, on Linux, reads what is installed.
func DetectPrereqs(ctx context.Context) *Prereqs {
	p := &Prereqs{OS: runtime.GOOS, DownloadURL: "https://docs.docker.com/engine/install/"}
	if runtime.GOOS != "linux" {
		p.DownloadURL = "https://www.docker.com/products/docker-desktop/"
	}
	p.Engine = DetectEngine(ctx, false)
	p.Warnings = append([]string{}, p.Engine.Warnings...)
	if p.Engine.Found {
		p.DockerOK = true
	} else {
		p.DockerErr = p.Engine.Err
		if p.DockerErr == "" {
			p.DockerErr = errNoEngine.Error()
		}
	}
	if runtime.GOOS != "linux" {
		p.State = StateManual
		if p.DockerOK {
			p.State = StateReady
		}
		return p
	}

	p.Distro = readOSReleaseID()
	inst, ok := distributionEngine()
	uname, safeName := currentUsername()
	_, pkErr := lookPath("pkexec")
	// Both elevated commands carry the username into a root shell, so a
	// name outside the whitelist gets neither: better no button than a
	// command that would expand to something else as root.
	elevate := pkErr == nil && safeName
	if ok {
		p.Installs = inst.engine
		switch inst.engine {
		case dockerx.KindPodman:
			p.Command = inst.command
			p.Note = "Afterwards the app turns podman's API socket on for your account (" + podmanSocketCommand() + "): no group to join and no restart."
			p.CanInstall = pkErr == nil
		default:
			p.Command = fmt.Sprintf("%s && systemctl enable --now docker && usermod -aG docker %s", inst.command, uname)
			p.Note = "After installation, log out and back in (or reboot) so your user joins the docker group."
			p.CanInstall = elevate
		}
	}
	if elevate {
		p.GroupCommand = "usermod -aG docker " + uname
	}
	p.SocketCommand = p.Engine.SocketCommand
	dockerInstalled := engineInstalled(ctx)
	p.EngineInstalled = dockerInstalled || p.Engine.PodmanInstalled
	gid, inFile := dockerGroup(uname)
	p.InDockerGroup = inFile
	p.State = classify(p, dockerInstalled, gid)
	return p
}

// distributionEngine is the engine this distribution's own packages
// provide and its install command, by os-release ID or ID_LIKE; false for
// a distribution the table does not know.
func distributionEngine() (engineInstall, bool) {
	if inst, ok := distroInstall[readOSReleaseID()]; ok {
		return inst, true
	}
	for _, like := range readOSReleaseIDLike() {
		if inst, ok := distroInstall[like]; ok {
			return inst, true
		}
	}
	return engineInstall{}, false
}

// InstallTarget names the engine an install on this computer would put,
// podman or docker, from the distribution alone: what to ask consent for
// before anything is detected. "" off Linux and on a distribution the
// table does not know.
func InstallTarget() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	inst, ok := distributionEngine()
	if !ok {
		return ""
	}
	return inst.engine
}

// classify turns the facts into the one state the page renders. The
// docker group states apply to Docker Engine alone: podman answers as the
// person, so its one missing piece is its socket.
func classify(p *Prereqs, dockerInstalled bool, dockerGID int) string {
	switch {
	case p.DockerOK:
		return StateReady
	case p.Engine.PodmanSocketOff:
		return StateSocketOff
	case !p.EngineInstalled && p.CanInstall:
		return StateInstallable
	case !p.EngineInstalled:
		return StateManual
	case !dockerInstalled:
		// podman is installed, its socket answers, and still nothing was
		// found: Engine.Err carries podman's own words.
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

// AddToDockerGroup runs only the group half of the install through pkexec:
// for a machine where the engine landed but the membership did not.
// Detection afterwards reports pendingRestart, since this session's groups
// do not change until it restarts.
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

// InstallEngine performs the detected install through pkexec — the polkit
// agent shows the OS's own elevation dialog. Output streams to logw. For
// podman the socket is then turned on as the person (EnablePodmanSocket);
// for Docker Engine the unit and the group step are part of the command.
func InstallEngine(ctx context.Context, logw io.Writer) error {
	p := DetectPrereqs(ctx)
	if p.DockerOK {
		_, _ = fmt.Fprintln(logw, "a container engine already answers — nothing to install")
		return nil
	}
	if !p.CanInstall {
		return fmt.Errorf("cannot install a container engine automatically on this system (%s/%s) — see %s", p.OS, p.Distro, p.DownloadURL)
	}
	_, _ = fmt.Fprintf(logw, "requesting elevation to run: %s\n", p.Command)
	cmd := elevatedShell(ctx, p.Command)
	cmd.Stdout = logw
	cmd.Stderr = logw
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install %s: %w", p.Installs, err)
	}
	if p.Installs == dockerx.KindPodman {
		_, _ = fmt.Fprintln(logw, "podman installed")
		return enablePodmanSocket(ctx, logw)
	}
	_, _ = fmt.Fprintln(logw, "docker engine installed and started")
	if p.Note != "" {
		_, _ = fmt.Fprintln(logw, p.Note)
	}
	return nil
}

// EnablePodmanSocket turns podman's API socket on for this account, as the
// person: a user unit, no elevation. It applies only where podman is
// installed and its socket is off; elsewhere it names the state.
func EnablePodmanSocket(ctx context.Context, logw io.Writer) error {
	p := DetectPrereqs(ctx)
	if p.State != StateSocketOff {
		return fmt.Errorf("the podman socket step does not apply: this machine's state is %s", p.State)
	}
	return enablePodmanSocket(ctx, logw)
}

func enablePodmanSocket(ctx context.Context, logw io.Writer) error {
	command := podmanSocketCommand()
	_, _ = fmt.Fprintf(logw, "running as you: %s\n", command)
	cmd := userShell(ctx, command)
	cmd.Stdout = logw
	cmd.Stderr = logw
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("turn podman's API socket on: %w", err)
	}
	_, _ = fmt.Fprintln(logw, "podman's API socket is on")
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
