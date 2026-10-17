// The engine a person chose, for a computer that offers more than one.
// Without a choice, detection adopts the first engine that answers in its
// order. Settings → Run defaults → Container engine lists every engine that
// answers and remembers the one chosen, by the place it answers at (Agentry's
// engine, a docker context, a host socket, podman on this computer), and from
// then on detection asks that place alone. A chosen engine that stops
// answering is said and never replaced by another: runs wait for the person
// rather than going to whichever engine happens to answer. Automatic forgets
// the choice and goes back to the order.
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

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/procx"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The choices: Agentry's engine, podman on this computer, a docker context
// by name, a host socket by path. "" is automatic.
const (
	ChoiceManaged = "managed"
	ChoicePodman  = "podman"
	choiceContext = "context:"
	choiceSocket  = "socket:"
)

// ChoiceForContext and ChoiceForSocket name a docker context and a socket
// as choices.
func ChoiceForContext(name string) string { return choiceContext + name }
func ChoiceForSocket(path string) string  { return choiceSocket + path }

// errLaunchNamed is why the choice cannot be changed in an app started
// with its engine named in its environment.
var errLaunchNamed = errors.New("the app was started with an engine named in its environment (AGENTRY_ENGINE, DOCKER_HOST or DOCKER_CONTEXT), which it drives as named; start the app without it to choose here")

// EngineCandidate is one engine that answers, with the choice that names it.
type EngineCandidate struct {
	Choice string
	Label  string
	Engine Engine
	// Current says this is the engine adopted now.
	Current bool
}

// EngineChoices is the picker's listing: the remembered choice ("" for
// automatic), every engine that answers, each place asked that did not,
// and why the choice cannot be changed, if it cannot.
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
	loadEngineChoice = func() string {
		st, err := settings.Load()
		if err != nil {
			return ""
		}
		return st.EngineChoice
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
			return nil, fmt.Errorf("docker context ls: %w", err)
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

// ChoiceLabel names a choice for the page.
func ChoiceLabel(choice string) string {
	switch {
	case choice == "":
		return "automatic"
	case choice == ChoiceManaged:
		return managedHow
	case choice == ChoicePodman:
		return "podman on this computer"
	case strings.HasPrefix(choice, choiceContext):
		return "the docker CLI's context " + strings.TrimPrefix(choice, choiceContext)
	case strings.HasPrefix(choice, choiceSocket):
		p := strings.TrimPrefix(choice, choiceSocket)
		for _, s := range knownSockets(context.Background()) {
			if s.path == p {
				return "the socket of " + s.name + " (" + p + ")"
			}
		}
		return "the socket at " + p
	}
	return choice
}

// validChoice refuses a choice that names no place this app knows how to
// ask: a context name the docker CLI would read as a flag or a path, a
// socket path that is not absolute.
func validChoice(choice string) error {
	switch {
	case choice == ChoiceManaged, choice == ChoicePodman:
		return nil
	case strings.HasPrefix(choice, choiceContext):
		n := strings.TrimPrefix(choice, choiceContext)
		if n == "" || strings.HasPrefix(n, "-") || strings.ContainsAny(n, "/\\\n") {
			return fmt.Errorf("%q is not a docker context name", n)
		}
		return nil
	case strings.HasPrefix(choice, choiceSocket):
		p := strings.TrimPrefix(choice, choiceSocket)
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return fmt.Errorf("%q is not an absolute socket path", p)
		}
		return nil
	}
	return fmt.Errorf("%q names no container engine this app can ask", choice)
}

// tryChoice adopts the engine the choice names and describes it; adoptMu is
// held. On failure nothing stays adopted and the error says what the place
// said.
func tryChoice(ctx context.Context, choice string) (Engine, error) {
	if err := validChoice(choice); err != nil {
		return Engine{}, err
	}
	adoptAndDescribe := func(d dockerx.Client, how string) (Engine, error) {
		e, err := describeEngine(ctx, d)
		if err != nil {
			clearAdoption()
			return Engine{}, err
		}
		e.How = how
		return e, nil
	}
	switch {
	case choice == ChoiceManaged:
		m := managedEngine()
		if m == nil || !m.Installed() {
			return Engine{}, errors.New("the app's engine is not installed on this computer")
		}
		if err := pingSocket(ctx, m.Socket()); err != nil {
			return Engine{}, fmt.Errorf("it is installed under %s and not running (%v)", m.Dir, err)
		}
		adoptManaged(m)
		return adoptAndDescribe(newDockerClient(), managedHow)
	case choice == ChoicePodman:
		if runtime.GOOS != "linux" {
			return Engine{}, errors.New("podman is driven on this computer only on Linux; elsewhere its machine answers at a socket")
		}
		p := newEngineClient(dockerx.KindPodman)
		if err := p.Available(ctx); err != nil {
			return Engine{}, err
		}
		adoptVar(dockerx.EnvEngine, dockerx.KindPodman)
		e, err := adoptAndDescribe(p, "podman on this computer")
		e.PodmanInstalled = err == nil
		return e, err
	case strings.HasPrefix(choice, choiceContext):
		if _, err := lookPath(engineBin(dockerx.KindDocker)); err != nil {
			return Engine{}, errors.New("the docker CLI is not installed")
		}
		name := strings.TrimPrefix(choice, choiceContext)
		adoptVar("DOCKER_CONTEXT", name)
		return adoptAndDescribe(newEngineClient(dockerx.KindDocker), "the docker CLI's context "+name)
	default:
		path := strings.TrimPrefix(choice, choiceSocket)
		if _, err := os.Lstat(path); err != nil {
			return Engine{}, fmt.Errorf("nothing is at %s", path)
		}
		if err := pingSocket(ctx, path); err != nil {
			return Engine{}, fmt.Errorf("nothing answers at %s (%v)", path, err)
		}
		adoptVar("DOCKER_HOST", dockerx.SchemeUnix+"://"+path)
		label := ChoiceLabel(choice)
		return adoptAndDescribe(newEngineClient(dockerx.KindDocker), strings.TrimSuffix(label, " ("+path+")"))
	}
}

// detectChosen is detection with a choice remembered: that place alone is
// asked; adoptMu is held. One that does not answer is said, with the way
// back to the order.
func detectChosen(ctx context.Context, choice string) Engine {
	refreshPath()
	e, err := tryChoice(ctx, choice)
	if err != nil {
		label := ChoiceLabel(choice)
		return Engine{Chosen: true, Choice: choice,
			Err:   fmt.Sprintf("your chosen engine, %s, is not answering: %v", label, err),
			Tried: []string{label + " (your choice under Settings → Run defaults → Container engine): " + err.Error()}}
	}
	e.Chosen, e.Choice = true, choice
	return e
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
// order, and lists each that answers once (two places that reach one
// engine, a context and its socket, are listed by the first). It adopts
// each in turn to describe it, so it takes the run lock in the app, and it
// leaves the adoption it found.
func ListEngines(ctx context.Context) EngineChoices {
	adoptMu.Lock()
	defer adoptMu.Unlock()
	out := EngineChoices{Choice: loadEngineChoice()}
	out.ChoiceLabel = ChoiceLabel(out.Choice)
	if launchNamed() {
		out.Locked = errLaunchNamed.Error()
		return out
	}
	refreshPath()
	saved := saveAdoption()
	defer restoreAdoption(saved)

	var places []string
	if m := managedEngine(); m != nil && m.Installed() {
		places = append(places, ChoiceManaged)
	}
	if _, err := lookPath(engineBin(dockerx.KindDocker)); err == nil {
		names, err := dockerContexts(ctx)
		if err != nil {
			out.Tried = append(out.Tried, "the docker CLI's contexts: "+err.Error())
		}
		for _, n := range names {
			if validChoice(ChoiceForContext(n)) == nil {
				places = append(places, ChoiceForContext(n))
			}
		}
	}
	for _, s := range knownSockets(ctx) {
		if s.path == "" {
			continue
		}
		if _, err := os.Lstat(s.path); err == nil {
			places = append(places, ChoiceForSocket(s.path))
		}
	}
	if runtime.GOOS == "linux" {
		places = append(places, ChoicePodman)
	}

	seen := map[string]bool{}
	for _, c := range places {
		clearAdoption()
		e, err := tryChoice(ctx, c)
		clearAdoption()
		if err != nil {
			out.Tried = append(out.Tried, ChoiceLabel(c)+": "+err.Error())
			continue
		}
		key := e.Kind + " " + e.CLI + " " + e.Endpoint
		if seen[key] {
			continue
		}
		seen[key] = true
		cur := saved.engine != nil && saved.engine.Kind == e.Kind && saved.engine.CLI == e.CLI && saved.engine.Endpoint == e.Endpoint
		out.Candidates = append(out.Candidates, EngineCandidate{Choice: c, Label: ChoiceLabel(c), Engine: e, Current: cur})
	}
	return out
}

// SetEngineChoice remembers the engine to drive and adopts it; "" is
// automatic, which forgets the choice and runs the whole order again. A
// choice that does not answer now is refused, and the engine adopted
// before stays. It changes the engine every later run drives, so it takes
// the run lock in the app.
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
	if err := validChoice(choice); err != nil {
		return Engine{}, err
	}
	saved := saveAdoption()
	clearAdoption()
	refreshPath()
	e, err := tryChoice(ctx, choice)
	if err != nil {
		restoreAdoption(saved)
		return Engine{}, fmt.Errorf("%s does not answer: %w", ChoiceLabel(choice), err)
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
