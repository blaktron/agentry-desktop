package run

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-desktop/internal/managedengine"
)

// contextsDocker stands in for the docker CLI on a computer with several
// contexts: it answers when DOCKER_CONTEXT names one of its live contexts
// (its current one when unset) or DOCKER_HOST names a live socket, and
// reports the endpoint that context or socket reaches.
type contextsDocker struct {
	*dockerx.Fake
	current string
	// endpoints maps a live context to the endpoint it reaches.
	endpoints map[string]string
	sockets   map[string]bool
}

func (d *contextsDocker) endpoint() (string, bool) {
	if h := os.Getenv("DOCKER_HOST"); h != "" {
		return h, d.sockets[strings.TrimPrefix(h, "unix://")]
	}
	c := os.Getenv("DOCKER_CONTEXT")
	if c == "" {
		c = d.current
	}
	ep, ok := d.endpoints[c]
	return ep, ok
}

func (d *contextsDocker) Available(context.Context) error {
	if _, ok := d.endpoint(); !ok {
		return errors.New("Cannot connect to the Docker daemon. Is the docker daemon running?")
	}
	return nil
}

func (d *contextsDocker) Engine(context.Context) (dockerx.Engine, error) {
	ep, _ := d.endpoint()
	return dockerx.Engine{Kind: dockerx.KindDocker, CLI: "docker", Version: "29.7.2", Endpoint: ep}, nil
}

const podmanEndpoint = "unix:///run/user/1000/podman/podman.sock"

func livePodman() *dockerx.Fake {
	return &dockerx.Fake{EngineInfo: &dockerx.Engine{Kind: dockerx.KindPodman, CLI: "podman", Version: "5.8.4", Endpoint: podmanEndpoint, Rootless: true}}
}

// twoEngines is a Linux computer with Docker Engine on the docker CLI's
// default context (its socket also among the known ones), a second context
// to Colima, and rootless podman.
func twoEngines(t *testing.T, choice string) (engineFixture, string) {
	t.Helper()
	sock := socketFile(t, "docker.sock")
	f := engineFixture{
		docker: &contextsDocker{current: "default",
			endpoints: map[string]string{"default": "unix://" + sock, "colima": "unix:///home/p/.colima/default/docker.sock"},
			sockets:   map[string]bool{sock: true}},
		podman:    livePodman(),
		sockets:   []knownSocket{{"Docker Engine", sock}},
		answering: map[string]bool{sock: true},
		onPath:    []string{"docker", "podman"},
		contexts:  []string{"default", "colima"},
		choice:    choice,
	}
	withEngines(t, f)
	return f, sock
}

func choices(c EngineChoices) []string {
	var out []string
	for _, k := range c.Candidates {
		out = append(out, k.Choice)
	}
	return out
}

// The picker lists every engine that answers, once each (the default
// context and the socket it reaches are one engine), marks the one adopted
// now, and leaves that adoption as it was.
func TestThePickerListsEveryEngineThatAnswersOnce(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("podman is driven natively on Linux")
	}
	twoEngines(t, "")
	before := DetectEngine(t.Context(), true)
	if !before.Found || before.How != "the docker CLI's current context" {
		t.Fatalf("detected %+v", before)
	}
	got := ListEngines(t.Context())
	want := []string{"context:default", "context:colima", "podman"}
	if strings.Join(choices(got), ",") != strings.Join(want, ",") {
		t.Fatalf("listed %q, want %q; tried %q", choices(got), want, got.Tried)
	}
	if !got.Candidates[0].Current || got.Candidates[1].Current || got.Candidates[2].Current {
		t.Errorf("current marks: %+v", got.Candidates)
	}
	if got.Candidates[2].Engine.Kind != dockerx.KindPodman || got.Candidates[2].Label != "podman on this computer" {
		t.Errorf("podman row: %+v", got.Candidates[2])
	}
	if got.Choice != "" || got.ChoiceLabel != "automatic" {
		t.Errorf("choice %q (%q)", got.Choice, got.ChoiceLabel)
	}
	// The listing adopted each in turn and put the first adoption back.
	if a := AdoptedEngine(); a == nil || a.Endpoint != before.Endpoint {
		t.Errorf("adopted after the listing: %+v", a)
	}
	for _, v := range []string{"DOCKER_CONTEXT", "DOCKER_HOST", dockerx.EnvEngine} {
		if os.Getenv(v) != "" {
			t.Errorf("%s=%q left by the listing", v, os.Getenv(v))
		}
	}
}

// A choice is adopted at once and remembered, and from then on detection,
// at launch or on Re-detect, asks that engine alone even though another
// answers first in the order.
func TestAChoiceIsAdoptedRememberedAndAskedAlone(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("podman is driven natively on Linux")
	}
	twoEngines(t, "")
	DetectEngine(t.Context(), true)
	e, err := SetEngineChoice(t.Context(), ChoicePodman)
	if err != nil {
		t.Fatal(err)
	}
	if !e.Found || e.Kind != dockerx.KindPodman || !e.Chosen || remembered != ChoicePodman {
		t.Fatalf("chose %+v, remembered %q", e, remembered)
	}
	if os.Getenv(dockerx.EnvEngine) != dockerx.KindPodman {
		t.Errorf("AGENTRY_ENGINE=%q", os.Getenv(dockerx.EnvEngine))
	}
	if !strings.HasSuffix(e.Summary(), "found as podman on this computer, your choice") {
		t.Errorf("summary %q", e.Summary())
	}
	// The next launch: nothing adopted, the docker CLI's context answers
	// first in the order, and the choice is what is adopted.
	adoptMu.Lock()
	clearAdoption()
	adoptMu.Unlock()
	again := DetectEngine(t.Context(), true)
	if !again.Found || again.Kind != dockerx.KindPodman || !again.Chosen {
		t.Fatalf("at launch: %+v", again)
	}
	// A page's read keeps it, still marked as the choice.
	if read := DetectEngine(t.Context(), false); !read.Chosen || read.Choice != ChoicePodman {
		t.Errorf("a read: %+v", read)
	}
	// A second docker context, chosen by name, through DOCKER_CONTEXT.
	if e, err = SetEngineChoice(t.Context(), ChoiceForContext("colima")); err != nil || !strings.Contains(e.Endpoint, ".colima") || os.Getenv("DOCKER_CONTEXT") != "colima" || os.Getenv(dockerx.EnvEngine) != "" {
		t.Fatalf("colima: %+v %v DOCKER_CONTEXT=%q AGENTRY_ENGINE=%q", e, err, os.Getenv("DOCKER_CONTEXT"), os.Getenv(dockerx.EnvEngine))
	}
}

// A chosen engine that stops answering is said, and no other engine is
// adopted in its place, though one answers; the card says so and offers
// the way back.
func TestASilentChoiceIsSaidAndNotReplaced(t *testing.T) {
	f, _ := twoEngines(t, ChoiceForContext("colima"))
	f.docker.(*contextsDocker).endpoints = map[string]string{"default": f.docker.(*contextsDocker).endpoints["default"]}
	got := DetectEngine(t.Context(), true)
	if got.Found {
		t.Fatalf("adopted %q in place of the silent choice", got.Summary())
	}
	if !got.Chosen || got.Choice != "context:colima" || !strings.HasPrefix(got.Err, "your chosen engine, the docker CLI's context colima, is not answering: ") {
		t.Errorf("err %q chosen %v", got.Err, got.Chosen)
	}
	if AdoptedEngine() != nil || os.Getenv("DOCKER_CONTEXT") != "" {
		t.Errorf("an adoption was left: %+v DOCKER_CONTEXT=%q", AdoptedEngine(), os.Getenv("DOCKER_CONTEXT"))
	}
	if p := DetectPrereqs(t.Context()); p.State != StateChosenSilent || p.DockerOK || !strings.Contains(p.DockerErr, "your chosen engine") {
		t.Errorf("card: state %s ok %v err %q", p.State, p.DockerOK, p.DockerErr)
	}
}

// Automatic forgets the choice and runs the whole order again.
func TestAutomaticForgetsTheChoice(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("podman is driven natively on Linux")
	}
	twoEngines(t, ChoicePodman)
	if e := DetectEngine(t.Context(), true); e.Kind != dockerx.KindPodman {
		t.Fatalf("the choice was not asked: %+v", e)
	}
	e, err := SetEngineChoice(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if remembered != "" || e.Chosen || e.How != "the docker CLI's current context" || os.Getenv(dockerx.EnvEngine) != "" {
		t.Errorf("automatic: remembered %q, %+v, AGENTRY_ENGINE=%q", remembered, e, os.Getenv(dockerx.EnvEngine))
	}
}

// A choice that does not answer now is refused, nothing is remembered, and
// the engine adopted before stays adopted.
func TestAChoiceThatDoesNotAnswerIsRefused(t *testing.T) {
	_, sock := twoEngines(t, "")
	before := DetectEngine(t.Context(), true)
	_, err := SetEngineChoice(t.Context(), ChoiceForContext("orbstack"))
	if err == nil || !strings.HasPrefix(err.Error(), "the docker CLI's context orbstack does not answer: ") {
		t.Fatalf("err = %v", err)
	}
	if remembered != "" {
		t.Errorf("remembered %q", remembered)
	}
	if a := AdoptedEngine(); a == nil || a.Endpoint != before.Endpoint || a.Endpoint != "unix://"+sock || os.Getenv("DOCKER_CONTEXT") != "" {
		t.Errorf("adopted after the refusal: %+v DOCKER_CONTEXT=%q", a, os.Getenv("DOCKER_CONTEXT"))
	}
}

// A choice naming no place the app can ask is refused before anything is
// tried: a context name the docker CLI would read as a flag or a path, a
// socket path that is not absolute, a word it does not know.
func TestAChoiceNamingNoPlaceIsRefused(t *testing.T) {
	twoEngines(t, "")
	for _, c := range []string{"context:", "context:--host=tcp://x", "context:../x", "socket:docker.sock", "socket:/a/../b", "docker"} {
		if _, err := SetEngineChoice(t.Context(), c); err == nil {
			t.Errorf("%q accepted", c)
		}
	}
	if remembered != "" {
		t.Errorf("remembered %q", remembered)
	}
}

// An app started with its engine named in its environment drives that
// engine as named: the picker lists nothing and says why, a choice is
// refused, and a remembered one is not asked.
func TestAnEngineNamedAtLaunchLocksThePicker(t *testing.T) {
	f, _ := twoEngines(t, ChoicePodman)
	f.named = true
	withEngines(t, f)
	if got := ListEngines(t.Context()); len(got.Candidates) != 0 || !strings.Contains(got.Locked, "named in its environment") {
		t.Errorf("listing: %+v", got)
	}
	if _, err := SetEngineChoice(t.Context(), ChoiceForContext("colima")); !errors.Is(err, errLaunchNamed) {
		t.Errorf("err = %v", err)
	}
	if e := DetectEngine(t.Context(), true); e.Chosen || e.How != "the engine named in the app's environment" {
		t.Errorf("detected %+v", e)
	}
}

// The app's own engine is a choice like any other where it is installed,
// and with another engine chosen it is left stopped at launch.
func TestAnotherChoiceLeavesTheManagedEngineStoppedAtLaunch(t *testing.T) {
	e := installedManagedEngine(t)
	twoEngines(t, ChoiceForContext("colima"))
	t.Setenv(dockerx.EnvDocker, "")
	t.Setenv("DOCKER_CONFIG", "")
	saved := managedEngine
	managedEngine = func() *managedengine.Engine { return e }
	t.Cleanup(func() { managedEngine = saved })
	var log strings.Builder
	StartManagedEngineAtLaunch(t.Context(), &log)
	if !strings.Contains(log.String(), "left stopped: the engine chosen in Settings is the docker CLI's context colima") {
		t.Errorf("log %q", log.String())
	}
	// Installed and stopped, it is listed under the places asked, not as a
	// candidate.
	got := ListEngines(t.Context())
	for _, c := range got.Candidates {
		if c.Choice == ChoiceManaged {
			t.Errorf("a stopped managed engine was listed: %+v", c)
		}
	}
	if len(got.Tried) == 0 || !strings.HasPrefix(got.Tried[0], managedHow+": it is installed under ") {
		t.Errorf("tried %q", got.Tried)
	}
}
