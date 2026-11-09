package codingtools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// stepTimeout bounds each install command: a marketplace clone over a slow
// network takes seconds, and a command waiting on a prompt it was not meant
// to show must not hold the page.
const stepTimeout = 3 * time.Minute

// outputLimit is how much of a command's output is kept for the page: the
// end, where a failure says what went wrong.
const outputLimit = 32 << 10

// Plan is an install the person can be asked about: the tool, the
// executable that does it, and the commands in order.
type Plan struct {
	Tool Tool
	// Via is the tool whose command runs, when it is not Tool itself.
	Via  *Tool
	Exe  string
	Argv [][]string
}

// ErrNoInstall is a tool the app cannot install the plugin into itself:
// its plugins are added in its own window, or it loads another tool's.
var ErrNoInstall = errors.New("no install command")

// Installer is the executable that installs the plugin into t, and the
// tool it belongs to when that is another entry: the Installer tool's own
// command when it is found, else a command-line build the app carries.
func Installer(t Tool) (exe string, via *Tool, err error) {
	if len(t.Steps) == 0 {
		return "", nil, ErrNoInstall
	}
	owner := t
	if t.Installer != "" {
		o, ok := ByID(t.Installer)
		if !ok {
			return "", nil, fmt.Errorf("%s: unknown installer %q", t.Name, t.Installer)
		}
		owner, via = o, &o
	}
	if f, ok := Find(owner); ok && owner.Kind == KindCLI {
		return f.Path, via, nil
	}
	if p, ok := firstExisting(t.Bundled.For(goos), true); ok {
		return p, nil, nil
	}
	return "", via, fmt.Errorf("%s is not on this computer: the plugin is installed with its %s command", owner.Name, strings.Join(owner.Commands, " or "))
}

// PlanInstall is the install for the tool id, refused when the tool is
// unknown, has no install command, or is not on this computer.
func PlanInstall(id string) (Plan, error) {
	t, ok := ByID(id)
	if !ok {
		return Plan{}, fmt.Errorf("unknown coding tool %q", id)
	}
	exe, via, err := Installer(t)
	if err != nil {
		return Plan{}, err
	}
	if !t.Repeatable && Plugin(t) == PluginInstalled {
		return Plan{}, fmt.Errorf("the Agentry plugin is already installed in %s", t.Name)
	}
	return Plan{Tool: t, Via: via, Exe: exe, Argv: t.Steps}, nil
}

// Commands are the plan's commands as a person would type them.
func (p Plan) Commands() []string {
	out := make([]string, len(p.Argv))
	for i, argv := range p.Argv {
		out[i] = CommandLine(p.Exe, argv)
	}
	return out
}

// CommandLine renders exe and its arguments for reading, quoting what has
// a space or a quote in it. It is never handed to a shell.
func CommandLine(exe string, argv []string) string {
	parts := make([]string, 0, len(argv)+1)
	for _, a := range append([]string{exe}, argv...) {
		if a == "" || strings.ContainsAny(a, " \t\"'") {
			a = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// StepResult is one command as it ran.
type StepResult struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exitCode"`
}

// runCommand is the seam the tests set.
var runCommand = func(ctx context.Context, exe string, argv []string) ([]byte, int, error) {
	cmd := exec.CommandContext(ctx, exe, argv...)
	cmd.Stdin = nil // the null device: a prompt reads end of input and gives up
	cmd.Env = commandEnv(exe)
	hideWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	}
	return out.Bytes(), code, err
}

// commandEnv is this process's environment with the executable's own
// directory first on PATH, where an npm-installed tool finds the runtime
// installed beside it, and colour turned off for the page.
func commandEnv(exe string) []string {
	env := os.Environ()
	dir := filepath.Dir(exe)
	for i, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, "PATH") {
			env[i] = k + "=" + dir + string(os.PathListSeparator) + v
			return append(env, "NO_COLOR=1")
		}
	}
	return append(env, "PATH="+dir, "NO_COLOR=1")
}

// Run runs the plan's commands in order and stops at the first that fails.
func Run(ctx context.Context, p Plan) ([]StepResult, error) {
	var results []StepResult
	for _, argv := range p.Argv {
		sctx, cancel := context.WithTimeout(ctx, stepTimeout)
		out, code, err := runCommand(sctx, p.Exe, argv)
		timedOut := errors.Is(sctx.Err(), context.DeadlineExceeded)
		cancel()
		r := StepResult{Command: CommandLine(p.Exe, argv), Output: tail(out), ExitCode: code}
		results = append(results, r)
		switch {
		case timedOut:
			return results, fmt.Errorf("%s did not finish within %s", r.Command, stepTimeout)
		case err != nil:
			return results, fmt.Errorf("%s failed: %w", r.Command, err)
		}
	}
	return results, nil
}

func tail(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > outputLimit {
		s = "…" + s[len(s)-outputLimit:]
	}
	return s
}
