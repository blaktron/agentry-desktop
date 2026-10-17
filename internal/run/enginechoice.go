// The engine a person chose, for a computer that offers more than one.
// Without a choice, detection adopts the first engine that answers in its
// order. Settings → Run defaults → Container engine lists every engine that
// answers on this computer and remembers the one chosen, by the place it
// answers at (Agentry's engine, a docker context, a host socket, podman on
// this computer), and from then on detection asks that place alone, at
// launch and on Re-detect. A chosen engine that stops answering is said and
// never replaced by another: runs wait for the person rather than going to
// whichever engine happens to answer. Automatic forgets the choice and goes
// back to the order.
package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/procx"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The places a choice names, as desktop.yaml spells them: "managed",
// "podman", "context:<name>", "socket:<path>"; "" is automatic.
const (
	choiceManaged = "managed"
	choicePodman  = "podman"
	choiceContext = "context:"
	choiceSocket  = "socket:"
)

// How detection names each place, shared with the order (engine.go).
const (
	howPodman         = "podman on this computer"
	howCurrentContext = "the docker CLI's current context"
	howContext        = "the docker CLI's context "
	howSocketOf       = "the socket of "
	howSocketAt       = "the socket at "
)

// choiceTimeout bounds the asking of one place, so a context whose engine
// is far away or gone cannot hold the listing (and the run lock) for long.
const choiceTimeout = 20 * time.Second

// errLaunchNamed is why the choice cannot be changed in an app started
// with its engine named in its environment.
var errLaunchNamed = errors.New("the app was started with an engine named in its environment (AGENTRY_ENGINE, DOCKER_HOST or DOCKER_CONTEXT), which it drives as named; start the app without it to choose here")

// place is a choice read once: its kind (one of the choice* constants, the
// prefixes without their colon) and its argument, a context's name or a
// socket's path.
type place struct {
	kind string
	arg  string
}

// parseChoice reads a choice, refusing one that names no place this app
// knows how to ask: a context name the docker CLI could read as anything
// but a name, a socket path that is not absolute and clean or that a URL
// would read differently from the file it names.
func parseChoice(choice string) (place, error) {
	switch {
	case choice == choiceManaged || choice == choicePodman:
		return place{kind: choice}, nil
	case strings.HasPrefix(choice, choiceContext):
		n := strings.TrimPrefix(choice, choiceContext)
		if n == "" || strings.HasPrefix(n, "-") || strings.ContainsAny(n, "/\\") || hasControl(n) {
			return place{}, fmt.Errorf("%q is not a docker context name", n)
		}
		return place{kind: choiceContext, arg: n}, nil
	case strings.HasPrefix(choice, choiceSocket):
		p := strings.TrimPrefix(choice, choiceSocket)
		if !filepath.IsAbs(p) || filepath.Clean(p) != p || strings.ContainsAny(p, "?#%") || hasControl(p) {
			return place{}, fmt.Errorf("%q is not an absolute socket path", p)
		}
		return place{kind: choiceSocket, arg: p}, nil
	}
	return place{}, fmt.Errorf("%q names no container engine this app can ask", choice)
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// label names the place for the page, without asking it anything.
func (p place) label() string {
	switch p.kind {
	case choiceManaged:
		return managedHow
	case choicePodman:
		return howPodman
	case choiceContext:
		return howContext + p.arg
	case choiceSocket:
		return howSocketAt + p.arg
	}
	return "automatic"
}

// ChoiceLabel names a choice for the page: "automatic" for none.
func ChoiceLabel(choice string) string {
	if choice == "" {
		return "automatic"
	}
	p, err := parseChoice(choice)
	if err != nil {
		return choice
	}
	return p.label()
}

// EngineCandidate is one engine that answers, with the choice that names it.
type EngineCandidate struct {
	Choice string
	Label  string
	Engine Engine
	// Current says this is the engine adopted now.
	Current bool
}

// EngineChoices is the picker's listing: the remembered choice ("" for
// automatic), every engine that answers on this computer, each place asked
// that did not (or that serves an engine a run refuses), and why the
// choice cannot be changed, if it cannot.
type EngineChoices struct {
	Choice      string
	ChoiceLabel string
	Candidates  []EngineCandidate
	Tried       []string
	Locked      string
}

// The seams the choice reads: the remembered choice in the app's settings
// and the docker CLI's contexts. Each is a var so a test hands it a
// stand-in.
var (
	loadEngineChoice = func() (string, error) {
		st, err := settings.Load()
		if err != nil {
			return "", err
		}
		return st.EngineChoice, nil
	}
	saveEngineChoice = func(choice string) error {
		st, err := settings.Load()
		if err != nil {
			return err
		}
		st.EngineChoice = choice
		return settings.Save(st)
	}
	dockerContexts = func(ctx context.Context) ([]string, error) {
		cmd := exec.CommandContext(ctx, engineBin(dockerx.KindDocker), "context", "ls", "--format", "{{.Name}}") //nolint:gosec // the docker CLI, fixed args
		procx.NoWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("could not be listed: %w", err)
		}
		var names []string
		for _, l := range strings.Split(string(out), "\n") {
			if n := strings.TrimSpace(l); n != "" {
				names = append(names, n)
			}
		}
		return names, nil
	}
)

// sameAs says two engines are one: the same kind, through the same CLI, at
// the same endpoint.
func (e Engine) sameAs(o Engine) bool {
	return e.Kind == o.Kind && e.CLI == o.CLI && e.Endpoint == o.Endpoint
}

// onThisComputer says the engine answers at a socket or a named pipe on
// this computer: a run refuses any other endpoint (tcp://, ssh://), since
// its docker filter cannot be put in front of an engine on another machine.
func (e Engine) onThisComputer() bool {
	return strings.HasPrefix(e.Endpoint, dockerx.SchemeUnix+"://") || strings.HasPrefix(e.Endpoint, "npipe://")
}

// errNotOnThisComputer is why an engine on another machine is not offered.
func errNotOnThisComputer(e Engine) error {
	return fmt.Errorf("it serves at %s, and runs refuse an engine on another machine: the run's docker filter cannot be put in front of it", e.Endpoint)
}

// tryPlace adopts the engine at a place and describes it; adoptMu is held.
// On failure nothing stays adopted and the error says what the place said.
func tryPlace(ctx context.Context, p place) (Engine, error) {
	ctx, cancel := context.WithTimeout(ctx, choiceTimeout)
	defer cancel()
	describe := func(d dockerx.Client, how string) (Engine, error) {
		e, err := describeEngine(ctx, d)
		if err != nil {
			clearAdoption()
			return Engine{}, err
		}
		e.How = how
		return e, nil
	}
	switch p.kind {
	case choiceManaged:
		m := managedEngine()
		if m == nil || !m.Installed() {
			return Engine{}, errors.New("the app's engine is not installed on this computer")
		}
		if err := pingSocket(ctx, m.Socket()); err != nil {
			return Engine{}, fmt.Errorf("it is installed under %s and not running (%v)", m.Dir, err)
		}
		adoptManaged(m)
		return describe(newDockerClient(), managedHow)
	case choicePodman:
		if runtime.GOOS != "linux" {
			return Engine{}, errors.New("podman is driven on this computer only on Linux; elsewhere its machine answers at a socket")
		}
		c := newEngineClient(dockerx.KindPodman)
		if err := c.Available(ctx); err != nil {
			return Engine{}, err
		}
		adoptVar(dockerx.EnvEngine, dockerx.KindPodman)
		e, err := describe(c, howPodman)
		e.PodmanInstalled = true
		return e, err
	case choiceContext:
		if _, err := lookPath(engineBin(dockerx.KindDocker)); err != nil {
			return Engine{}, errors.New("the docker CLI is not installed")
		}
		adoptVar("DOCKER_CONTEXT", p.arg)
		return describe(newEngineClient(dockerx.KindDocker), howContext+p.arg)
	default:
		if _, err := os.Lstat(p.arg); err != nil {
			return Engine{}, fmt.Errorf("nothing is at %s", p.arg)
		}
		if err := pingSocket(ctx, p.arg); err != nil {
			return Engine{}, fmt.Errorf("nothing answers at %s (%v)", p.arg, err)
		}
		adoptVar("DOCKER_HOST", dockerx.SchemeUnix+"://"+p.arg)
		how := howSocketAt + p.arg
		for _, s := range knownSockets(ctx) {
			if s.path == p.arg {
				how = howSocketOf + s.name
				break
			}
		}
		return describe(newEngineClient(dockerx.KindDocker), how)
	}
}

// detectChosen is detection with a choice remembered: that place alone is
// asked; adoptMu is held. One that does not answer is said with the way
// back to the order, and podman installed with its API socket off is said
// as the order says it, so the card can turn the socket on.
func detectChosen(ctx context.Context, choice string) Engine {
	refreshPath()
	p, err := parseChoice(choice)
	var e Engine
	if err == nil {
		e, err = tryPlace(ctx, p)
	}
	if err == nil && !e.onThisComputer() {
		clearAdoption()
		err = errNotOnThisComputer(e)
	}
	if err != nil {
		out := chosenSilent(choice, err)
		if p.kind == choicePodman && e.PodmanInstalled {
			out.PodmanInstalled = true
			if strings.Contains(err.Error(), "API socket") {
				out.PodmanSocketOff, out.SocketCommand = true, podmanSocketCommand()
			}
		}
		return out
	}
	e.Chosen, e.Choice = true, choice
	return e
}

// chosenSilent is what detection says of a chosen engine that does not
// answer.
func chosenSilent(choice string, err error) Engine {
	label := ChoiceLabel(choice)
	return Engine{Chosen: true, Choice: choice,
		Err:   fmt.Sprintf("your chosen engine, %s, is not answering: %v", label, err),
		Tried: []string{label + " (your choice under Settings → Run defaults → Container engine): " + err.Error()}}
}

// adoption is what detection had adopted and the variables it had set, for
// a listing that tries each engine in turn and then puts things back.
type adoption struct {
	engine *Engine
	names  []string
	values map[string]string
}

func saveAdoption() adoption {
	a := adoption{engine: adopted, values: map[string]string{}}
	for _, v := range adoptedVars {
		a.names = append(a.names, v)
		a.values[v] = os.Getenv(v)
	}
	return a
}

func restoreAdoption(a adoption) {
	clearAdoption()
	for _, v := range a.names {
		adoptVar(v, a.values[v])
	}
	adopted = a.engine
}

// ListEngines asks every place an engine may answer at, in detection's
// order, and lists each engine on this computer that answers once (a
// context and the socket it reaches are one engine, listed by the first).
// It adopts each in turn to describe it, so the app holds the run lock
// while it runs, and it puts back the adoption it found.
func ListEngines(ctx context.Context) EngineChoices {
	adoptMu.Lock()
	defer adoptMu.Unlock()
	choice, err := loadEngineChoice()
	out := EngineChoices{Choice: choice, ChoiceLabel: ChoiceLabel(choice)}
	if err != nil {
		out.Tried = append(out.Tried, "the engine chosen in Settings: "+err.Error())
	}
	if launchNamed() {
		out.Locked = errLaunchNamed.Error()
		return out
	}
	refreshPath()
	saved := saveAdoption()
	defer restoreAdoption(saved)
	// The person's own docker configuration, not an adopted engine's, names
	// the contexts.
	clearAdoption()

	var places []place
	if m := managedEngine(); m != nil && m.Installed() {
		places = append(places, place{kind: choiceManaged})
	}
	if _, err := lookPath(engineBin(dockerx.KindDocker)); err == nil {
		names, err := dockerContexts(ctx)
		if err != nil {
			out.Tried = append(out.Tried, "the docker CLI's contexts "+err.Error())
		}
		for _, n := range names {
			if p, err := parseChoice(choiceContext + n); err == nil {
				places = append(places, p)
			}
		}
	}
	for _, s := range knownSockets(ctx) {
		if s.path == "" {
			continue
		}
		if _, err := os.Lstat(s.path); err != nil {
			continue
		}
		if p, err := parseChoice(choiceSocket + s.path); err == nil {
			places = append(places, p)
		}
	}
	if runtime.GOOS == "linux" {
		places = append(places, place{kind: choicePodman})
	}

	var listed []Engine
	for _, p := range places {
		clearAdoption()
		e, err := tryPlace(ctx, p)
		clearAdoption()
		if err == nil && !e.onThisComputer() {
			err = errNotOnThisComputer(e)
		}
		if err != nil {
			out.Tried = append(out.Tried, p.label()+": "+err.Error())
			continue
		}
		dup := false
		for _, l := range listed {
			dup = dup || l.sameAs(e)
		}
		if dup {
			continue
		}
		listed = append(listed, e)
		c := EngineCandidate{Choice: p.choice(), Label: p.label(), Engine: e, Current: saved.engine != nil && saved.engine.sameAs(e)}
		out.Candidates = append(out.Candidates, c)
	}
	return out
}

// choice is the place as desktop.yaml spells it.
func (p place) choice() string {
	switch p.kind {
	case choiceContext, choiceSocket:
		return p.kind + p.arg
	}
	return p.kind
}

// SetEngineChoice remembers the engine to drive and adopts it; "" is
// automatic, which forgets the choice and runs the whole order again. A
// choice that does not answer now, or that serves an engine a run refuses,
// is refused, and the engine adopted before stays. It changes the engine
// every later run drives, so the app holds the run lock while it runs.
func SetEngineChoice(ctx context.Context, choice string) (Engine, error) {
	adoptMu.Lock()
	defer adoptMu.Unlock()
	if launchNamed() {
		return Engine{}, errLaunchNamed
	}
	if choice == "" {
		if err := saveEngineChoice(""); err != nil {
			return Engine{}, err
		}
		return detectFresh(ctx), nil
	}
	p, err := parseChoice(choice)
	if err != nil {
		return Engine{}, err
	}
	saved := saveAdoption()
	clearAdoption()
	refreshPath()
	e, err := tryPlace(ctx, p)
	if err == nil && !e.onThisComputer() {
		clearAdoption()
		err = errNotOnThisComputer(e)
	}
	if err != nil {
		restoreAdoption(saved)
		return Engine{}, fmt.Errorf("%s cannot be chosen: %w", p.label(), err)
	}
	if err := saveEngineChoice(choice); err != nil {
		restoreAdoption(saved)
		return Engine{}, err
	}
	e.Chosen, e.Choice = true, choice
	finishDetection(&e)
	adopted = &e
	return e, nil
}
