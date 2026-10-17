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
	if ep, ok := d.endpoints[os.Getenv("DOCKER_CONTEXT")]; ok && os.Getenv("DOCKER_HOST") == "" {
		return ep, true
	}
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
	e, err := SetEngineChoice(t.Context(), choicePodman)
	if err != nil {
		t.Fatal(err)
	}
	if !e.Found || e.Kind != dockerx.KindPodman || !e.Chosen || remembered != choicePodman {
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
	if read := DetectEngine(t.Context(), false); !read.Chosen || read.Choice != choicePodman {
		t.Errorf("a read: %+v", read)
	}
	// A second docker context, chosen by name, through DOCKER_CONTEXT.
	if e, err = SetEngineChoice(t.Context(), choiceContext+"colima"); err != nil || !strings.Contains(e.Endpoint, ".colima") || os.Getenv("DOCKER_CONTEXT") != "colima" || os.Getenv(dockerx.EnvEngine) != "" {
		t.Fatalf("colima: %+v %v DOCKER_CONTEXT=%q AGENTRY_ENGINE=%q", e, err, os.Getenv("DOCKER_CONTEXT"), os.Getenv(dockerx.EnvEngine))
	}
}

// A chosen engine that stops answering is said, and no other engine is
// adopted in its place, though one answers; the card says so and offers
// the way back.
func TestASilentChoiceIsSaidAndNotReplaced(t *testing.T) {
	f, _ := twoEngines(t, choiceContext+"colima")
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
	// The card says it in the chosen engine's words; which state it shows
	// depends on this machine's own facts (settleChosen, below).
	if p := DetectPrereqs(t.Context()); p.DockerOK || !strings.Contains(p.DockerErr, "your chosen engine") ||
		p.State == StateInstallable || p.State == StateManual || p.State == StateManagedOffer || p.State == StateWindowsOffer {
		t.Errorf("card: state %s ok %v err %q", p.State, p.DockerOK, p.DockerErr)
	}
}

// With a chosen engine silent, a state that would offer another engine
// becomes chosenSilent; a state that starts or repairs the chosen one
// stays; Agentry's engine chosen and since removed may be installed again.
func TestASilentChoiceSettlesTheCardsState(t *testing.T) {
	silent := Engine{Chosen: true, Choice: choiceContext + "colima"}
	managed := Engine{Chosen: true, Choice: choiceManaged}
	for _, tc := range []struct {
		state string
		e     Engine
		want  string
	}{
		{StateInstallable, silent, StateChosenSilent},
		{StateManual, silent, StateChosenSilent},
		{StateWindowsOffer, silent, StateChosenSilent},
		{StateManagedOffer, silent, StateChosenSilent},
		{StateManagedOffer, managed, StateManagedOffer},
		{StateManagedStopped, managed, StateManagedStopped},
		{StateSocketOff, Engine{Chosen: true, Choice: choicePodman}, StateSocketOff},
		{StateGroupMissing, silent, StateGroupMissing},
		{StatePendingRestart, silent, StatePendingRestart},
		{StateWindowsStopped, silent, StateWindowsStopped},
		{StateInstallable, Engine{}, StateInstallable},
		{StateReady, Engine{Found: true, Chosen: true}, StateReady},
	} {
		if got := settleChosen(tc.state, tc.e); got != tc.want {
			t.Errorf("settleChosen(%s, %+v) = %s, want %s", tc.state, tc.e, got, tc.want)
		}
	}
}

// Automatic forgets the choice and runs the whole order again.
func TestAutomaticForgetsTheChoice(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("podman is driven natively on Linux")
	}
	twoEngines(t, choicePodman)
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
	_, err := SetEngineChoice(t.Context(), choiceContext+"orbstack")
	if err == nil || !strings.HasPrefix(err.Error(), "the docker CLI's context orbstack cannot be chosen: ") {
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
	f, _ := twoEngines(t, choicePodman)
	f.named = true
	withEngines(t, f)
	if got := ListEngines(t.Context()); len(got.Candidates) != 0 || !strings.Contains(got.Locked, "named in its environment") {
		t.Errorf("listing: %+v", got)
	}
	if _, err := SetEngineChoice(t.Context(), choiceContext+"colima"); !errors.Is(err, errLaunchNamed) {
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
	twoEngines(t, choiceContext+"colima")
	t.Setenv(dockerx.EnvDocker, "")
	t.Setenv("DOCKER_CONFIG", "")
	saved := managedEngine
	managedEngine = func() *managedengine.Engine { return e }
	t.Cleanup(func() { managedEngine = saved })
	var log strings.Builder
	StartManagedEngineAtLaunch(t.Context(), &log)
	if !strings.Contains(log.String(), "left stopped: another engine is chosen in Settings (the docker CLI's context colima)") {
		t.Errorf("log %q", log.String())
	}
	// Installed and stopped, it is listed under the places asked, not as a
	// candidate.
	got := ListEngines(t.Context())
	for _, c := range got.Candidates {
		if c.Choice == choiceManaged {
			t.Errorf("a stopped managed engine was listed: %+v", c)
		}
	}
	if len(got.Tried) == 0 || !strings.HasPrefix(got.Tried[0], managedHow+": it is installed under ") {
		t.Errorf("tried %q", got.Tried)
	}
}

// A context whose engine is on another machine is not offered: a run
// refuses it (its docker filter cannot be put in front of a remote engine),
// so the listing says why and a choice of it is refused.
func TestARemoteContextIsNotOffered(t *testing.T) {
	f, _ := twoEngines(t, "")
	f.docker.(*contextsDocker).endpoints["build-box"] = "tcp://10.0.0.5:2376"
	f.contexts = append(f.contexts, "build-box")
	withEngines(t, f)
	got := ListEngines(t.Context())
	for _, c := range got.Candidates {
		if c.Choice == choiceContext+"build-box" {
			t.Fatalf("the remote context was offered: %+v", c)
		}
	}
	var said bool
	for _, tr := range got.Tried {
		said = said || strings.HasPrefix(tr, "the docker CLI's context build-box: it serves at tcp://10.0.0.5:2376, and runs refuse an engine on another machine")
	}
	if !said {
		t.Errorf("tried %q", got.Tried)
	}
	if _, err := SetEngineChoice(t.Context(), choiceContext+"build-box"); err == nil || remembered != "" {
		t.Errorf("chose the remote context: %v, remembered %q", err, remembered)
	}
	// Remembered from elsewhere, it is said and not driven.
	remembered = choiceContext + "build-box"
	if e := DetectEngine(t.Context(), true); e.Found || !strings.Contains(e.Err, "runs refuse an engine on another machine") || os.Getenv("DOCKER_CONTEXT") != "" {
		t.Errorf("detected %+v DOCKER_CONTEXT=%q", e, os.Getenv("DOCKER_CONTEXT"))
	}
}

// A chosen engine adopted earlier that stops answering is said as the
// choice on the next read, with the way back, not as an engine Re-detect
// would replace.
func TestAChosenEngineThatStopsAnsweringIsSaidOnARead(t *testing.T) {
	f, _ := twoEngines(t, "")
	if _, err := SetEngineChoice(t.Context(), choiceContext+"colima"); err != nil {
		t.Fatal(err)
	}
	delete(f.docker.(*contextsDocker).endpoints, "colima")
	got := DetectEngine(t.Context(), false)
	if got.Found || !got.Chosen || got.Choice != choiceContext+"colima" || !strings.HasPrefix(got.Err, "your chosen engine, the docker CLI's context colima, is not answering") {
		t.Fatalf("read: %+v", got)
	}
	for _, tr := range got.Tried {
		if strings.Contains(tr, "Re-detect looks for another engine") {
			t.Errorf("tried %q", got.Tried)
		}
	}
}

// A choice that cannot be read leaves no engine adopted in its place, and
// says so.
func TestAnUnreadableChoiceAdoptsNothing(t *testing.T) {
	twoEngines(t, "")
	was := loadEngineChoice
	loadEngineChoice = func() (string, error) { return "", errors.New("open desktop.yaml: permission denied") }
	t.Cleanup(func() { loadEngineChoice = was })
	e := DetectEngine(t.Context(), true)
	if e.Found || !e.Chosen || !strings.Contains(e.Err, "the container engine chosen in Settings could not be read: open desktop.yaml: permission denied") {
		t.Fatalf("detected %+v", e)
	}
	if AdoptedEngine() != nil {
		t.Errorf("adopted %+v", AdoptedEngine())
	}
}

// podman chosen with its API socket off is said as the order says it, so
// the card offers to turn the socket on rather than another engine.
func TestAChosenPodmanWithItsSocketOffOffersTheSocket(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("podman is driven natively on Linux")
	}
	f, _ := twoEngines(t, choicePodman)
	f.podman = &dockerx.Fake{EngineErr: errors.New("podman's API socket unix:///run/user/1000/podman/podman.sock does not answer (connection refused)")}
	withEngines(t, f)
	e := DetectEngine(t.Context(), true)
	if e.Found || !e.Chosen || !e.PodmanInstalled || !e.PodmanSocketOff || e.SocketCommand != "systemctl --user enable --now podman.socket" {
		t.Fatalf("detected %+v", e)
	}
}

// With Agentry's engine adopted, a listing names the contexts from the
// person's own docker configuration and puts every variable of the
// adoption back afterwards.
func TestAListingPutsTheManagedAdoptionBack(t *testing.T) {
	m := installedManagedEngine(t)
	f, _ := twoEngines(t, "")
	f.answering[m.Socket()] = true
	withEngines(t, f)
	t.Setenv(dockerx.EnvDocker, "")
	t.Setenv("DOCKER_CONFIG", "/home/me/.docker")
	saved := managedEngine
	managedEngine = func() *managedengine.Engine { return m }
	t.Cleanup(func() { managedEngine = saved })
	f.docker.(*contextsDocker).sockets[strings.TrimPrefix(m.Endpoint(), "unix://")] = true
	if e := DetectEngine(t.Context(), true); !e.IsManaged() {
		t.Fatalf("detected %+v", e)
	}
	var configWhileListing string
	dockerContexts = func(context.Context) ([]string, error) {
		configWhileListing = os.Getenv("DOCKER_CONFIG")
		return f.contexts, nil
	}
	got := ListEngines(t.Context())
	if configWhileListing != "/home/me/.docker" {
		t.Errorf("contexts listed under DOCKER_CONFIG=%q", configWhileListing)
	}
	if len(got.Candidates) == 0 || got.Candidates[0].Choice != choiceManaged || !got.Candidates[0].Current {
		t.Errorf("candidates %+v", got.Candidates)
	}
	if os.Getenv("DOCKER_HOST") != m.Endpoint() || os.Getenv(dockerx.EnvDocker) != m.Docker() || os.Getenv("DOCKER_CONFIG") != m.DockerConfigDir() {
		t.Errorf("after the listing DOCKER_HOST=%q AGENTRY_DOCKER=%q DOCKER_CONFIG=%q", os.Getenv("DOCKER_HOST"), os.Getenv(dockerx.EnvDocker), os.Getenv("DOCKER_CONFIG"))
	}
}
