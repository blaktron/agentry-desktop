package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
)

// contextDocker stands in for the docker CLI: it answers only when the
// process's DOCKER_HOST names the socket it serves ("" for its own default
// context), as the CLI follows the endpoint the environment names.
type contextDocker struct {
	*dockerx.Fake
	answersAt string
	kind      string
}

func (d *contextDocker) Available(context.Context) error {
	if os.Getenv("DOCKER_HOST") != d.answersAt {
		return errors.New("Cannot connect to the Docker daemon. Is the docker daemon running?")
	}
	return nil
}

func (d *contextDocker) Engine(context.Context) (dockerx.Engine, error) {
	kind := d.kind
	if kind == "" {
		kind = dockerx.KindDocker
	}
	ep := os.Getenv("DOCKER_HOST")
	if ep == "" {
		ep = "unix:///var/run/docker.sock"
	}
	return dockerx.Engine{Kind: kind, CLI: "docker", Version: "29.7.2", Endpoint: ep}, nil
}

// engineFixture points every detection seam at stand-ins: the docker CLI,
// podman's, the known sockets and their pings, and a PATH that holds the
// programs it names.
type engineFixture struct {
	docker  dockerx.Client
	podman  dockerx.Client
	sockets []knownSocket
	// answering are the socket paths that answer a ping.
	answering map[string]bool
	onPath    []string
	named     bool
}

func withEngines(t *testing.T, f engineFixture) {
	t.Helper()
	t.Setenv("DOCKER_HOST", "")
	t.Setenv(dockerx.EnvEngine, "")
	_ = os.Unsetenv("DOCKER_HOST")
	_ = os.Unsetenv(dockerx.EnvEngine)
	adoptMu.Lock()
	clearAdoption()
	adoptMu.Unlock()
	if f.docker == nil {
		f.docker = &dockerx.Fake{AvailableErr: errors.New("docker CLI not found")}
	}
	if f.podman == nil {
		f.podman = &dockerx.Fake{AvailableErr: errors.New("podman not found")}
	}
	oldClient, oldEngine, oldSockets, oldPing, oldNamed, oldLook, oldInterval, oldCfg, oldUID :=
		newDockerClient, newEngineClient, knownSockets, pingSocket, launchNamed, lookPath, dockerRetryInterval, dockerConfigFile, effectiveUID
	dockerRetryInterval = 0
	// A person's account, wherever the test runs (CI runs as root).
	effectiveUID = func() int { return 1000 }
	newEngineClient = func(kind string) dockerx.Client {
		if kind == dockerx.KindPodman {
			return f.podman
		}
		return f.docker
	}
	newDockerClient = func() dockerx.Client {
		if os.Getenv(dockerx.EnvEngine) == dockerx.KindPodman {
			return f.podman
		}
		return f.docker
	}
	knownSockets = func(context.Context) []knownSocket { return f.sockets }
	pingSocket = func(_ context.Context, path string) error {
		if f.answering[path] {
			return nil
		}
		return errors.New("connection refused")
	}
	launchNamed = func() bool { return f.named }
	onPath := map[string]bool{}
	for _, p := range f.onPath {
		onPath[p] = true
	}
	lookPath = func(file string) (string, error) {
		if onPath[file] {
			return "/usr/bin/" + file, nil
		}
		return "", errors.New("not found")
	}
	dockerConfigFile = func() string { return "" }
	t.Cleanup(func() {
		adoptMu.Lock()
		clearAdoption()
		adoptMu.Unlock()
		newDockerClient, newEngineClient, knownSockets, pingSocket, launchNamed, lookPath, dockerRetryInterval, dockerConfigFile, effectiveUID =
			oldClient, oldEngine, oldSockets, oldPing, oldNamed, oldLook, oldInterval, oldCfg, oldUID
	})
}

// socketFile makes a file standing where an engine's socket would be: a
// stopped engine leaves exactly that.
func socketFile(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDetectionTakesTheDockerCLIsOwnContextFirst(t *testing.T) {
	sock := socketFile(t, "colima.sock")
	withEngines(t, engineFixture{
		docker:    &contextDocker{Fake: &dockerx.Fake{}},
		sockets:   []knownSocket{{"Colima", sock}},
		answering: map[string]bool{sock: true},
		onPath:    []string{"docker"},
	})
	e := DetectEngine(t.Context(), false)
	if !e.Found || e.How != "the docker CLI's current context" || e.Kind != dockerx.KindDocker {
		t.Fatalf("found = %+v", e)
	}
	if os.Getenv("DOCKER_HOST") != "" {
		t.Fatalf("the CLI's own context needs no DOCKER_HOST, got %q", os.Getenv("DOCKER_HOST"))
	}
	if got := e.Summary(); !strings.Contains(got, "docker 29.7.2, rootful, at unix:///var/run/docker.sock, found as the docker CLI's current context") {
		t.Fatalf("summary = %q", got)
	}
}

// An engine installed while the app is open puts its CLI in a directory
// this process's PATH lacks until it is read again: detection reads it
// before it looks the docker CLI up.
func TestDetectionFindsADockerCLIInstalledSinceTheAppStarted(t *testing.T) {
	withEngines(t, engineFixture{docker: &contextDocker{Fake: &dockerx.Fake{}}})
	look := lookPath
	refreshed := false
	lookPath = func(file string) (string, error) {
		if refreshed && file == "docker" {
			return "/usr/bin/docker", nil
		}
		return look(file)
	}
	was := refreshPath
	refreshPath = func() { refreshed = true }
	t.Cleanup(func() { refreshPath = was })
	if e := DetectEngine(t.Context(), true); !e.Found || e.How != "the docker CLI's current context" {
		t.Fatalf("found = %+v", e)
	}
}

func TestDetectionAsksEachKnownSocketAndBelievesOnlyAnAnswer(t *testing.T) {
	dead := socketFile(t, "rancher.sock")
	live := socketFile(t, "colima.sock")
	withEngines(t, engineFixture{
		docker:    &contextDocker{Fake: &dockerx.Fake{}, answersAt: "unix://" + live},
		sockets:   []knownSocket{{"Rancher Desktop", dead}, {"Colima", live}, {"OrbStack", filepath.Join(t.TempDir(), "absent.sock")}},
		answering: map[string]bool{live: true},
		onPath:    []string{"docker"},
	})
	e := DetectEngine(t.Context(), false)
	if !e.Found || e.How != "the socket of Colima" || e.Endpoint != "unix://"+live {
		t.Fatalf("found = %+v", e)
	}
	if got := os.Getenv("DOCKER_HOST"); got != "unix://"+live {
		t.Fatalf("DOCKER_HOST adopted = %q", got)
	}
	// The socket file with nothing behind it is named, not believed.
	if len(e.Warnings) != 1 || !strings.Contains(e.Warnings[0], "Rancher Desktop left a socket at "+dead+" with nothing answering behind it") {
		t.Fatalf("warnings = %v", e.Warnings)
	}
	if a := AdoptedEngine(); a == nil || a.How != e.How {
		t.Fatalf("adopted = %+v", a)
	}
	// A later read says the socket trap again.
	if again := DetectEngine(t.Context(), false); len(again.Warnings) != 1 || again.Warnings[0] != e.Warnings[0] {
		t.Fatalf("a read lost the warning: %v", again.Warnings)
	}
}

func TestDetectionFindsPodmanOnThisComputerLast(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("podman is driven natively on Linux")
	}
	withEngines(t, engineFixture{
		podman: &dockerx.Fake{EngineInfo: &dockerx.Engine{Kind: dockerx.KindPodman, CLI: "podman", Version: "5.8.4", Rootless: true, Endpoint: "unix:///run/user/1000/podman/podman.sock"}},
		onPath: []string{"podman"},
	})
	e := DetectEngine(t.Context(), false)
	if !e.Found || e.How != "podman on this computer" || !e.PodmanInstalled {
		t.Fatalf("found = %+v", e)
	}
	if os.Getenv(dockerx.EnvEngine) != dockerx.KindPodman || os.Getenv("DOCKER_HOST") != "" {
		t.Fatalf("adopted env: AGENTRY_ENGINE=%q DOCKER_HOST=%q", os.Getenv(dockerx.EnvEngine), os.Getenv("DOCKER_HOST"))
	}
	if got := e.Summary(); got != "podman 5.8.4, rootless, at unix:///run/user/1000/podman/podman.sock, found as podman on this computer" {
		t.Fatalf("summary = %q", got)
	}
}

func TestPodmanWithItsSocketOffIsSaidWithTheCommandThatTurnsItOn(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("podman is driven natively on Linux")
	}
	withEngines(t, engineFixture{
		podman: &dockerx.Fake{EngineErr: errors.New("podman's API socket unix:///run/user/1000/podman/podman.sock does not answer (connection refused): start it with `systemctl --user enable --now podman.socket`")},
		// (the engine's own words name the socket; a service on another
		// machine would not, and gets no socket click: below)
		onPath: []string{"podman"},
	})
	e := DetectEngine(t.Context(), false)
	if e.Found || !e.PodmanInstalled || !e.PodmanSocketOff {
		t.Fatalf("engine = %+v", e)
	}
	if e.SocketCommand != "systemctl --user enable --now podman.socket" {
		t.Fatalf("socket command = %q", e.SocketCommand)
	}
	if !strings.HasPrefix(e.Err, "podman's API socket unix:///run/user/1000/podman/podman.sock does not answer") {
		t.Fatalf("err = %q", e.Err)
	}
	if len(e.Tried) != 2 || e.Tried[0] != "the docker CLI: not installed" || !strings.HasPrefix(e.Tried[1], "podman on this computer: podman's API socket") {
		t.Fatalf("tried = %q", e.Tried)
	}
	remote := &dockerx.Fake{EngineErr: errors.New("podman drives a service on another machine (CONTAINER_HOST, a connection, or a podman machine), and the runner drives podman only on this machine")}
	newEngineClient = func(string) dockerx.Client { return remote }
	if r := DetectEngine(t.Context(), true); r.Found || r.PodmanSocketOff || r.SocketCommand != "" {
		t.Fatalf("a remote podman got the socket click: %+v", r)
	}
	if os.Getenv(dockerx.EnvEngine) != "" {
		t.Fatal("nothing found, yet an engine was adopted")
	}
	if AdoptedEngine() != nil {
		t.Fatal("nothing found, yet an engine is recorded as adopted")
	}
}

func TestAnAdoptedEngineThatStillAnswersIsKeptUntilRedetect(t *testing.T) {
	live := socketFile(t, "colima.sock")
	orders := 0
	withEngines(t, engineFixture{
		docker:    &contextDocker{Fake: &dockerx.Fake{}, answersAt: "unix://" + live},
		sockets:   []knownSocket{{"Colima", live}},
		answering: map[string]bool{live: true},
		onPath:    []string{"docker"},
	})
	inner := knownSockets
	knownSockets = func(ctx context.Context) []knownSocket { orders++; return inner(ctx) }
	if e := DetectEngine(t.Context(), false); e.How != "the socket of Colima" {
		t.Fatalf("first: %+v", e)
	}
	// A page's read keeps what answers: the order does not run again.
	if e := DetectEngine(t.Context(), false); e.How != "the socket of Colima" || orders != 1 {
		t.Fatalf("second: %+v, orders = %d", e, orders)
	}
	// Colima stops answering: a read says so and keeps the adoption (a run
	// may be using it), with Re-detect named as the way to another engine.
	docker := newEngineClient(dockerx.KindDocker).(*contextDocker)
	docker.answersAt = "never"
	gone := DetectEngine(t.Context(), false)
	if gone.Found || gone.How != "the socket of Colima" || os.Getenv("DOCKER_HOST") != "unix://"+live || orders != 1 {
		t.Fatalf("a read switched or dropped the adopted engine: %+v, DOCKER_HOST=%q, orders=%d", gone, os.Getenv("DOCKER_HOST"), orders)
	}
	if len(gone.Tried) != 1 || !strings.Contains(gone.Tried[0], "adopted earlier") || !strings.Contains(gone.Tried[0], "Re-detect") {
		t.Fatalf("tried = %q", gone.Tried)
	}
	docker.answersAt = "unix://" + live
	// Re-detect clears the adoption and runs the order from nothing; the
	// same engine is found again, by the same route.
	if e := DetectEngine(t.Context(), true); e.How != "the socket of Colima" || orders != 2 {
		t.Fatalf("redetect: %+v, orders = %d", e, orders)
	}
}

func TestRedetectAdoptsTheEngineThatAnswersNow(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("podman is driven natively on Linux")
	}
	docker := &contextDocker{Fake: &dockerx.Fake{}, answersAt: "never"}
	withEngines(t, engineFixture{
		docker: docker,
		podman: &dockerx.Fake{EngineInfo: &dockerx.Engine{Kind: dockerx.KindPodman, CLI: "podman", Version: "5.8.4", Rootless: true, Endpoint: "unix:///run/user/1000/podman/podman.sock"}},
		onPath: []string{"docker", "podman"},
	})
	if e := DetectEngine(t.Context(), false); e.Kind != dockerx.KindPodman {
		t.Fatalf("first: %+v", e)
	}
	// Docker Engine's socket is made current; a page's read keeps podman,
	// which still answers, and Re-detect switches.
	docker.answersAt = ""
	if e := DetectEngine(t.Context(), false); e.Kind != dockerx.KindPodman {
		t.Fatalf("a read switched engines: %+v", e)
	}
	e := DetectEngine(t.Context(), true)
	if e.Kind != dockerx.KindDocker || e.How != "the docker CLI's current context" {
		t.Fatalf("redetect: %+v", e)
	}
	if os.Getenv(dockerx.EnvEngine) != "" {
		t.Fatalf("podman's adoption was kept: AGENTRY_ENGINE=%q", os.Getenv(dockerx.EnvEngine))
	}
	// And back, once Docker Engine stops answering.
	docker.answersAt = "never"
	if e := DetectEngine(t.Context(), true); e.Kind != dockerx.KindPodman || os.Getenv(dockerx.EnvEngine) != dockerx.KindPodman {
		t.Fatalf("redetect back: %+v", e)
	}
}

func TestAnEngineNamedInTheEnvironmentEndsTheSearch(t *testing.T) {
	live := socketFile(t, "colima.sock")
	withEngines(t, engineFixture{
		docker:    &contextDocker{Fake: &dockerx.Fake{}, answersAt: "never"},
		podman:    &dockerx.Fake{EngineInfo: &dockerx.Engine{Kind: dockerx.KindPodman, CLI: "podman"}},
		sockets:   []knownSocket{{"Colima", live}},
		answering: map[string]bool{live: true},
		onPath:    []string{"docker", "podman"},
		named:     true,
	})
	e := DetectEngine(t.Context(), false)
	if e.Found {
		t.Fatalf("the environment named the engine, yet another was adopted: %+v", e)
	}
	if e.Err != "Cannot connect to the Docker daemon. Is the docker daemon running?" || os.Getenv("DOCKER_HOST") != "" {
		t.Fatalf("err = %q, DOCKER_HOST = %q", e.Err, os.Getenv("DOCKER_HOST"))
	}
	if len(e.Tried) != 1 || !strings.HasPrefix(e.Tried[0], "the engine named in the app's environment: ") || !strings.HasSuffix(e.Tried[0], "nothing else is tried") {
		t.Fatalf("tried = %q", e.Tried)
	}
	// The named engine is driven as named: with AGENTRY_ENGINE=podman the
	// client newDockerClient builds is podman's, and it is what is found.
	t.Setenv(dockerx.EnvEngine, dockerx.KindPodman)
	named := DetectEngine(t.Context(), true)
	if !named.Found || named.Kind != dockerx.KindPodman || named.How != "the engine named in the app's environment" {
		t.Fatalf("named podman: %+v", named)
	}
}

func TestNothingInstalledIsSaidAsSuch(t *testing.T) {
	withEngines(t, engineFixture{})
	e := DetectEngine(t.Context(), false)
	if e.Found || e.Err != "no container engine answers on this computer: neither the docker CLI nor podman is installed" {
		t.Fatalf("engine = %+v", e)
	}
}

func TestACredentialHelperOffPathIsAWarning(t *testing.T) {
	withEngines(t, engineFixture{docker: &contextDocker{Fake: &dockerx.Fake{}}, onPath: []string{"docker", "docker-credential-osxkeychain"}})
	cfg := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfg, []byte(`{"credsStore":"desktop","credHelpers":{"ghcr.io":"osxkeychain","example.com":"gone"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dockerConfigFile = func() string { return cfg }
	e := DetectEngine(t.Context(), false)
	if !e.Found || len(e.Warnings) != 2 {
		t.Fatalf("engine = %+v", e)
	}
	joined := strings.Join(e.Warnings, "\n")
	for _, want := range []string{`names the credential helper "desktop" in credsStore`, "docker-credential-desktop is not on PATH", `"gone" in credHelpers for example.com`, "a leftover of Docker Desktop"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings miss %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "osxkeychain") {
		t.Errorf("a helper on PATH was named:\n%s", joined)
	}
	// podman driven natively reads its own authentication file, not docker's.
	if got := credentialHelperTrap(Engine{Found: true, CLI: dockerx.KindPodman}); got != nil {
		t.Fatalf("podman natively: %v", got)
	}
}

func TestTheKnownSocketsAreTriedInAFixedOrder(t *testing.T) {
	home := t.TempDir()
	for _, d := range []string{".colima/default", ".colima/work", ".lima/docker"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	got := hostSocketsFor("darwin", home, "/tmp/podman/podman-machine-default-api.sock")
	want := []string{
		"Docker Desktop " + filepath.Join(home, ".docker/run/docker.sock"),
		"Docker Desktop /var/run/docker.sock",
		"Colima " + filepath.Join(home, ".colima/default/docker.sock"),
		"Colima " + filepath.Join(home, ".colima/work/docker.sock"),
		"OrbStack " + filepath.Join(home, ".orbstack/run/docker.sock"),
		"Rancher Desktop " + filepath.Join(home, ".rd/docker.sock"),
		"Lima " + filepath.Join(home, ".lima/docker/sock/docker.sock"),
		"the podman machine /tmp/podman/podman-machine-default-api.sock",
	}
	var names []string
	for _, s := range got {
		names = append(names, s.name+" "+s.path)
	}
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		t.Fatalf("darwin order:\n%s\nwant:\n%s", strings.Join(names, "\n"), strings.Join(want, "\n"))
	}
	linux := hostSocketsFor("linux", home, "")
	if len(linux) != 2 || linux[0].name != "Docker Desktop" || linux[1].path != "/var/run/docker.sock" {
		t.Fatalf("linux: %+v", linux)
	}
	if w := hostSocketsFor("windows", home, ""); w != nil {
		t.Fatalf("windows serves named pipes, which the docker CLI's context reaches: %+v", w)
	}
}
