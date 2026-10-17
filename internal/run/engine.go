// Which container engine this computer offers. Detection asks, in a fixed
// order and by an API call never by a socket file's existence, the docker
// CLI as its own configuration has it, then the known host sockets of the
// engines that serve the Docker API from a VM (Docker Desktop, Colima,
// OrbStack, Rancher Desktop, Lima, a podman machine), then podman on this
// computer. The first that answers is adopted: its environment (DOCKER_HOST
// for a socket, AGENTRY_ENGINE for podman) is set in this process, which
// every engine client built after that inherits, so the runs, the setup
// and the images all drive the engine the page names. Re-detect runs the
// whole order again from nothing and adopts what it finds.
package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/procx"
)

// Engine is what detection found: the kind and version the pages show,
// where it answers, how it was found, and the traps seen on the way.
type Engine struct {
	Found bool
	// Kind is what answers (docker or podman); CLI the client binary family
	// driven, which differs from Kind for a podman socket the docker CLI
	// reaches.
	Kind     string
	CLI      string
	Version  string
	Endpoint string
	Rootless bool
	// How names what found it: the docker CLI's current context, the socket
	// of a named engine, or podman on this computer.
	How string
	// Err is why nothing answered, in the words of the engine that is
	// installed (the docker CLI's when it is, else podman's), and Tried
	// lists each place asked and what it said.
	Err   string
	Tried []string
	// PodmanInstalled says podman's CLI answers on this computer;
	// PodmanSocketOff that it does and its API socket does not, which
	// SocketCommand turns on for the account the app runs as.
	PodmanInstalled bool
	PodmanSocketOff bool
	SocketCommand   string
	// Warnings are the traps seen on the way: a credential helper the docker
	// configuration names and PATH lacks, a socket file with nothing behind
	// it. socketWarnings are the latter alone, kept with the adoption so a
	// later read still says them.
	Warnings       []string
	socketWarnings []string
	// Chosen says detection asked the engine remembered under Settings →
	// Run defaults → Container engine, Choice names it (enginechoice.go);
	// a chosen engine that does not answer is not replaced by another.
	Chosen bool
	Choice string
}

// Summary is the one line a page shows for the engine found.
func (e Engine) Summary() string {
	if !e.Found {
		return ""
	}
	who := e.Kind
	if e.CLI != "" && e.CLI != e.Kind {
		who += " through the " + e.CLI + " CLI"
	}
	root := "rootful"
	if e.Rootless {
		root = "rootless"
	}
	s := fmt.Sprintf("%s %s, %s, at %s, found as %s", who, e.Version, root, e.Endpoint, e.How)
	if e.Chosen {
		s += ", your choice"
	}
	return s
}

// The adopted engine, and what detection changed in the environment to
// adopt it. Only detection writes these; the environment is read by every
// engine client built afterwards.
var (
	adoptMu sync.Mutex
	adopted *Engine
	// adoptedVars are the variables detection set; clearing adoption unsets
	// exactly these, never one the app was started with. adoptedWere holds
	// the value one of them had before detection set it (the person's own
	// DOCKER_CONFIG under Agentry's engine), which clearing puts back.
	adoptedVars []string
	adoptedWere = map[string]string{}
)

// The seams detection reads, each a var so a test hands it a stand-in:
// the client for each CLI family, the socket ping, the known sockets, and
// whether the app was started with the engine named in its environment, in
// which case nothing but that engine is tried.
var (
	newEngineClient = func(kind string) dockerx.Client {
		return &dockerx.CLI{Bin: engineBin(kind), Podman: kind == dockerx.KindPodman}
	}
	pingSocket   = dockerx.PingSocket
	knownSockets = knownHostSockets
	launchNamed  = func() bool {
		for _, v := range []string{dockerx.EnvEngine, "DOCKER_HOST", "DOCKER_CONTEXT"} {
			if os.Getenv(v) != "" && !isAdoptedVar(v) {
				return true
			}
		}
		return false
	}
	// dockerConfigFile is the docker CLI's configuration, whose credential
	// helpers the pull reads.
	dockerConfigFile = func() string {
		if d := os.Getenv("DOCKER_CONFIG"); d != "" {
			return filepath.Join(d, "config.json")
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, ".docker", "config.json")
	}
)

func isAdoptedVar(name string) bool {
	for _, v := range adoptedVars {
		if v == name {
			return true
		}
	}
	return false
}

// DetectEngine finds the engine this computer offers and adopts it. With
// fresh false, a page's read, an engine adopted earlier is kept whether or
// not it answers now: a read never switches the engine a run may be
// using, and one that stopped answering is reported as such, with
// Re-detect the way to another. With fresh true (Re-detect), the adoption
// is cleared and the whole order runs again, or, with an engine chosen in
// Settings, that engine alone is asked. Nothing found leaves nothing
// adopted, and Err says what the installed engine said.
func DetectEngine(ctx context.Context, fresh bool) Engine {
	adoptMu.Lock()
	defer adoptMu.Unlock()
	if !fresh && adopted != nil {
		e, err := describeEngine(ctx, newDockerClient())
		if err != nil {
			if adopted.Chosen {
				return chosenSilent(adopted.Choice, err)
			}
			return Engine{How: adopted.How, Err: err.Error(),
				Tried: []string{adopted.How + ", adopted earlier: " + err.Error() + " (Re-detect looks for another engine)"}}
		}
		e.How = adopted.How
		e.Chosen, e.Choice = adopted.Chosen, adopted.Choice
		e.socketWarnings = adopted.socketWarnings
		e.Warnings = append(append([]string{}, e.socketWarnings...), credentialHelperTrap(e)...)
		adopted = &e
		return e
	}
	return detectFresh(ctx)
}

// detectFresh clears the adoption and detects again: the chosen engine
// alone where one is remembered, else the whole order; adoptMu is held.
func detectFresh(ctx context.Context) Engine {
	clearAdoption()
	var e Engine
	c, err := loadEngineChoice()
	switch {
	case launchNamed():
		e = detectInOrder(ctx)
	case err != nil:
		// The choice cannot be read: no engine is adopted in its place.
		e = Engine{Chosen: true, Err: "the container engine chosen in Settings could not be read: " + err.Error(),
			Tried: []string{"the engine chosen in Settings: " + err.Error()}}
	case c != "":
		e = detectChosen(ctx, c)
	default:
		e = detectInOrder(ctx)
	}
	finishDetection(&e)
	if e.Found {
		adopted = &e
	}
	return e
}

// finishDetection keeps the socket warnings detection saw with the engine
// and adds the credential helper trap.
func finishDetection(e *Engine) {
	e.socketWarnings = append([]string{}, e.Warnings...)
	e.Warnings = append(e.Warnings, credentialHelperTrap(*e)...)
}

// AdoptedEngine is the engine detection last adopted, if any.
func AdoptedEngine() *Engine {
	adoptMu.Lock()
	defer adoptMu.Unlock()
	if adopted == nil {
		return nil
	}
	e := *adopted
	return &e
}

func clearAdoption() {
	for _, v := range adoptedVars {
		if was, ok := adoptedWere[v]; ok {
			_ = os.Setenv(v, was)
		} else {
			_ = os.Unsetenv(v)
		}
	}
	adoptedVars = nil
	adoptedWere = map[string]string{}
	adopted = nil
}

func adoptVar(name, value string) {
	if !isAdoptedVar(name) {
		if was, ok := os.LookupEnv(name); ok {
			adoptedWere[name] = was
		}
		adoptedVars = append(adoptedVars, name)
	}
	_ = os.Setenv(name, value)
}

// detectInOrder is the order itself; adoptMu is held. Each place asked
// goes into Tried with what it said; Err is the installed engine's own
// words (the docker CLI's when it is installed, else podman's), else that
// neither is installed.
func detectInOrder(ctx context.Context) Engine {
	var out Engine
	said := func(place, what string) { out.Tried = append(out.Tried, place+": "+what) }
	// The PATH a terminal opened now would have, before any program is
	// looked up: an engine installed while the app is open puts its CLI
	// in a directory this process has not seen yet.
	refreshPath()
	// The app started with the engine named in its environment: that one
	// is driven as named, and nothing else is tried.
	if launchNamed() {
		e, err := describeEngine(ctx, newDockerClient())
		if err == nil {
			e.How = "the engine named in the app's environment"
			return e
		}
		out.Err = err.Error()
		said("the engine named in the app's environment", err.Error()+"; nothing else is tried")
		return out
	}
	// The engine the app manages itself, where it is installed: the app
	// put it there, so it is asked first, by a ping of its socket like
	// every other; one that does not answer is said, and the card offers
	// to start it.
	if m := managedEngine(); m != nil && m.Installed() {
		if err := pingSocket(ctx, m.Socket()); err == nil {
			adoptManaged(m)
			e, err := describeEngine(ctx, newDockerClient())
			if err == nil {
				e.How = managedHow
				return e
			}
			clearAdoption()
			said(managedHow, fmt.Sprintf("answers at %s, and its docker command cannot drive it: %v", m.Socket(), err))
		} else {
			said(managedHow, fmt.Sprintf("installed under %s and not running (%v)", m.Dir, err))
		}
	}
	// The docker CLI as its own configuration has it: DOCKER_HOST,
	// DOCKER_CONTEXT or `docker context use`, else its default endpoint. A
	// docker CLI that is not installed is said once and not retried.
	dockerInstalled := false
	var dockerWords string
	if _, err := lookPath(engineBin(dockerx.KindDocker)); err == nil {
		dockerInstalled = true
		e, err := describeEngine(ctx, newEngineClient(dockerx.KindDocker))
		if err == nil {
			e.How = howCurrentContext
			return e
		}
		dockerWords = err.Error()
		said(howCurrentContext, dockerWords)
	} else {
		said("the docker CLI", "not installed")
	}
	// The known host sockets, each by a ping of the Docker API: a socket
	// file a stopped engine left behind answers nothing and is said so.
	for _, s := range knownSockets(ctx) {
		if s.path == "" {
			continue
		}
		if _, err := os.Lstat(s.path); err != nil {
			continue
		}
		if err := pingSocket(ctx, s.path); err != nil {
			left := dockerx.LeftSocket{HostSocket: dockerx.HostSocket{Name: s.name, Path: s.path}, Err: err}
			out.Warnings = append(out.Warnings, left.Message())
			continue
		}
		adoptVar("DOCKER_HOST", dockerx.SchemeUnix+"://"+s.path)
		e, err := describeEngine(ctx, newEngineClient(dockerx.KindDocker))
		if err == nil {
			e.How = howSocketOf + s.name
			e.Warnings = out.Warnings
			return e
		}
		clearAdoption()
		said(howSocketOf+s.name, fmt.Sprintf("answers at %s, and the docker CLI cannot drive it: %v", s.path, err))
	}
	// podman on this computer, driven natively: its own CLI answers for the
	// engine, and its API socket must answer too.
	var podmanWords string
	if runtime.GOOS == "linux" {
		p := newEngineClient(dockerx.KindPodman)
		if err := p.Available(ctx); err == nil {
			out.PodmanInstalled = true
			adoptVar(dockerx.EnvEngine, dockerx.KindPodman)
			e, err := describeEngine(ctx, p)
			if err == nil {
				e.How = howPodman
				e.PodmanInstalled = true
				e.Warnings = out.Warnings
				return e
			}
			clearAdoption()
			podmanWords = err.Error()
			said(howPodman, podmanWords)
			// The engine's own words name the socket when that is what is
			// missing; a service on another machine, or any other refusal,
			// is not cured by turning the socket on.
			if strings.Contains(podmanWords, "API socket") {
				out.PodmanSocketOff = true
				out.SocketCommand = podmanSocketCommand()
			}
		} else {
			said("podman", err.Error())
		}
	}
	switch {
	case dockerInstalled:
		out.Err = dockerWords
	case out.PodmanInstalled:
		out.Err = podmanWords
	default:
		out.Err = errNoEngine.Error() + ": neither the docker CLI nor podman is installed"
	}
	return out
}

// describeEngine asks the client for its engine, waiting for one that is
// waking as the readiness check does, and reads the answer into Engine.
func describeEngine(ctx context.Context, d dockerx.Client) (Engine, error) {
	if err := dockerAvailable(ctx, d); err != nil {
		return Engine{}, err
	}
	de, err := dockerx.DescribeEngine(ctx, d)
	if err != nil {
		return Engine{}, err
	}
	return Engine{Found: true, Kind: de.Kind, CLI: de.CLI, Version: de.Version, Endpoint: de.Endpoint, Rootless: de.Rootless}, nil
}

// engineBin is the client binary for a CLI family: the one its variable
// names, else the family's own name on PATH.
func engineBin(kind string) string {
	if kind == dockerx.KindPodman {
		if b := os.Getenv(dockerx.EnvPodman); b != "" {
			return b
		}
		return "podman"
	}
	if b := os.Getenv(dockerx.EnvDocker); b != "" {
		return b
	}
	return "docker"
}

// effectiveUID is the account the app runs as; a var so a test is a
// person's account wherever it runs.
var effectiveUID = os.Geteuid

// podmanSocketCommand turns podman's API socket on for the account the app
// runs as: a user unit for a person, the system's for root. No elevation,
// no group, no restart.
func podmanSocketCommand() string {
	if effectiveUID() == 0 {
		return "systemctl enable --now podman.socket"
	}
	return "systemctl --user enable --now podman.socket"
}

// knownSocket is one place an engine may serve the Docker API on this
// computer, named for the page.
type knownSocket struct {
	name string
	path string
}

// knownHostSockets lists, in the order tried, where each engine that runs
// a daemon in a VM puts its host socket, and Docker Engine's own: Docker
// Desktop's, then Colima's, OrbStack's, Rancher Desktop's, Lima's, and a
// podman machine's, read from podman itself rather than guessed. A path is
// listed whether or not it exists; detection skips the absent ones. On
// Windows the engines serve a named pipe, which the docker CLI's own
// context reaches, so nothing is listed.
func knownHostSockets(ctx context.Context) []knownSocket {
	home, _ := os.UserHomeDir()
	machine := ""
	if runtime.GOOS == "darwin" {
		machine = podmanMachineSocket(ctx)
	}
	return hostSocketsFor(runtime.GOOS, home, machine)
}

// hostSocketsFor is knownHostSockets with its inputs passed in: the
// platform, the home directory and the podman machine's socket, "" for
// none.
func hostSocketsFor(goos, home, machine string) []knownSocket {
	under := func(rel string) string {
		if home == "" {
			return ""
		}
		return filepath.Join(home, rel)
	}
	switch goos {
	case "darwin":
		out := []knownSocket{
			{"Docker Desktop", under(".docker/run/docker.sock")},
			{"Docker Desktop", "/var/run/docker.sock"},
		}
		out = append(out, globSockets("Colima", under(".colima"), "docker.sock")...)
		out = append(out,
			knownSocket{"OrbStack", under(".orbstack/run/docker.sock")},
			knownSocket{"Rancher Desktop", under(".rd/docker.sock")},
		)
		out = append(out, globSockets("Lima", under(".lima"), "sock/docker.sock")...)
		if machine != "" {
			out = append(out, knownSocket{"the podman machine", machine})
		}
		return out
	case "linux":
		return []knownSocket{
			{"Docker Desktop", under(".docker/desktop/docker.sock")},
			{"Docker Engine", "/var/run/docker.sock"},
		}
	}
	return nil
}

// globSockets lists <dir>/<instance>/<rel> for every instance directory,
// sorted: Colima's profiles and Lima's instances each keep a socket of
// their own.
func globSockets(name, dir, rel string) []knownSocket {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []knownSocket
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, knownSocket{name, filepath.Join(dir, e.Name(), rel)})
		}
	}
	return out
}

// podmanMachineSocket is the host socket of the default podman machine, as
// `podman machine inspect` reports it; "" without podman or a machine.
func podmanMachineSocket(ctx context.Context) string {
	bin := engineBin(dockerx.KindPodman)
	if _, err := lookPath(bin); err != nil {
		return ""
	}
	cmd := exec.CommandContext(ctx, bin, "machine", "inspect", "--format", "{{.ConnectionInfo.PodmanSocket.Path}}") //nolint:gosec // the engine's own binary, fixed args
	procx.NoWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		return ""
	}
	return p
}

// credentialHelperTrap reads the docker configuration for a credential
// helper PATH lacks: `credsStore` and each `credHelpers` entry name a
// docker-credential-<name> program the CLI runs for every pull, and one
// Docker Desktop installed and a later engine did not (its `desktop`) fails
// every pull with "executable file not found". Only the docker CLI reads
// that file; podman driven natively keeps its own.
func credentialHelperTrap(e Engine) []string {
	if e.Found && e.CLI == dockerx.KindPodman {
		return nil
	}
	path := dockerConfigFile()
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path) //nolint:gosec // the docker CLI's own configuration file
	if err != nil {
		return nil
	}
	var cfg struct {
		CredsStore  string            `json:"credsStore"`
		CredHelpers map[string]string `json:"credHelpers"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	check := func(helper, where string) {
		if helper == "" || seen[helper] {
			return
		}
		seen[helper] = true
		prog := "docker-credential-" + helper
		if _, err := lookPath(prog); err != nil {
			out = append(out, fmt.Sprintf("%s names the credential helper %q in %s, and %s is not on PATH: every image pull fails until the entry is removed or the helper installed (a leftover of Docker Desktop, when it is \"desktop\")", path, helper, where, prog))
		}
	}
	check(cfg.CredsStore, "credsStore")
	for registry, helper := range cfg.CredHelpers {
		check(helper, "credHelpers for "+registry)
	}
	return out
}

// errNoEngine is the readiness error when detection found nothing.
var errNoEngine = errors.New("no container engine answers on this computer")
