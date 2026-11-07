package linkservice

// Launching an installed server. What runs on the person's computer is the
// catalogue entry's launch line and environment, nothing else: the command,
// its arguments and its variables come from the entry the app's own
// catalogue holds for the bound id, never from the release, the bind, a
// call or the agent. The process runs as the person, the app's own user,
// with the app's environment and the entry's variables over it, and is
// ended with its children when the run unbinds.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry/pkg/appserver"

	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"
)

// LaunchSpec is what the link service runs for a stdio entry: the
// command line and the variables over the app's environment.
type LaunchSpec struct {
	Command []string
	Env     map[string]string
}

// Line is the launch as the receipt and the page say it: the variables
// set, then the command.
func (l LaunchSpec) Line() string {
	keys := make([]string, 0, len(l.Env))
	for k := range l.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)+len(l.Command))
	for _, k := range keys {
		parts = append(parts, k+"="+l.Env[k])
	}
	return strings.Join(append(parts, l.Command...), " ")
}

// launchSpec is the entry's launch, with its screening variable dropped
// when the person turned the application's script screening off in
// Settings. Nothing
// else changes it.
func launchSpec(e appserver.Server, screeningOff bool) (LaunchSpec, error) {
	if e.Attachment != appserver.AttachmentStdio || e.Launch == nil || len(e.Launch.Command) == 0 {
		return LaunchSpec{}, fmt.Errorf("the catalogue gives %s no launch", e.Application)
	}
	spec := LaunchSpec{Command: append([]string(nil), e.Launch.Command...), Env: map[string]string{}}
	for k, v := range e.Launch.Env {
		spec.Env[k] = v
	}
	if v := ScreeningVariable(e.ID); v != "" && screeningOff {
		delete(spec.Env, v)
	}
	return spec, nil
}

// ScreeningVariable is the launch variable that turns an application
// server's own script screening on, "" for one with none: the switch Settings →
// Your computer offers per application.
func ScreeningVariable(id string) string {
	switch id {
	case "blender":
		return "BLENDER_MCP_SAFE_MODE"
	}
	return ""
}

// Launcher starts an installed server from its spec and opens an MCP
// session with it. The link service's is startStdio; a test substitutes
// its own.
type Launcher func(ctx context.Context, spec LaunchSpec) (Session, error)

// stderrKeep bounds what the link service keeps of a server's own log.
const stderrKeep = 8 << 10

// tail keeps the last bytes written to it.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > stderrKeep {
		t.buf = t.buf[len(t.buf)-stderrKeep:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}

// stdioSession is an MCP session with a process the link service started.
type stdioSession struct {
	m      *mcp
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.Closer
	stderr *tail
	done   chan struct{}
	once   sync.Once
}

// startStdio starts spec's command as the person and negotiates MCP over
// its standard input and output.
func startStdio(ctx context.Context, spec LaunchSpec) (Session, error) {
	path, err := exec.LookPath(spec.Command[0])
	if err != nil {
		return nil, fmt.Errorf("%s is not installed on this computer: %w", spec.Command[0], err)
	}
	// The process outlives the bind's context: it is ended at unbind.
	cmd := exec.Command(path, spec.Command[1:]...) //nolint:gosec // the catalogue entry's launch, never an argument a run gave
	cmd.Env = os.Environ()
	keys := make([]string, 0, len(spec.Env))
	for k := range spec.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		cmd.Env = append(cmd.Env, k+"="+spec.Env[k])
	}
	ownProcessGroup(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	// The server's output is read from a pipe of the link service's own,
	// which Wait does not close, so nothing the server wrote before it
	// ended is lost to the read.
	stdout, stdoutW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = stdoutW
	s := &stdioSession{cmd: cmd, stdin: stdin, stdout: stdout, stderr: &tail{}, done: make(chan struct{})}
	cmd.Stderr = s.stderr
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stdoutW.Close()
		return nil, fmt.Errorf("start %s: %w", spec.Command[0], err)
	}
	_ = stdoutW.Close()
	go func() {
		_ = cmd.Wait()
		close(s.done)
	}()
	conn := newRPCConn(stdout, stdin)
	s.m = &mcp{t: conn}
	if err := s.m.negotiate(ctx, stdioProbe(conn)); err != nil {
		_ = s.Close()
		if log := s.stderr.String(); log != "" {
			return nil, fmt.Errorf("%w; its last words: %s", err, clip(lastLines(log, 3), 600))
		}
		return nil, err
	}
	return s, nil
}

// lastLines is the last n lines of s.
func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " / ")
}

func (s *stdioSession) Tools(ctx context.Context) ([]desktoplink.Tool, error) {
	return s.m.tools(ctx)
}

func (s *stdioSession) Call(ctx context.Context, name string, args json.RawMessage) (desktoplink.Answer, error) {
	return s.m.callTool(ctx, name, args)
}

func (s *stdioSession) Era() string { return s.m.era }

// Close ends the process and its children: its input closed first, as the
// stdio transport's shutdown says, then a signal, then a kill. The kill
// goes to the whole group even when the server ended by itself, since a
// child a launcher started may outlive it.
func (s *stdioSession) Close() error {
	s.once.Do(func() {
		_ = s.stdin.Close()
		select {
		case <-s.done:
		case <-time.After(2 * time.Second):
			terminate(s.cmd)
			select {
			case <-s.done:
			case <-time.After(3 * time.Second):
			}
		}
		kill(s.cmd)
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
		}
		_ = s.stdout.Close()
	})
	return nil
}
