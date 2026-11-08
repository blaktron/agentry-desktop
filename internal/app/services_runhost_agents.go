package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	"github.com/blaktron/agentry-cli/pkg/runhost/remote"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/session"
)

// A runhost's persistent agents, as Settings → Runs → Runhosts shows them: they run
// on the runhost under its service, on the runhost's own model, and this app
// drives them over the run key (remote.AgentCommand) and talks to them
// through the agent's owner socket there, relayed to a socket of this
// computer's own (remote.RelayAgentSocket). Agentry Helper stays on this
// computer: it drives this app through its app link.

// RunhostAgentView is one persistent agent on a runhost.
type RunhostAgentView struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	State       string `json:"state"`
	DesktopOnly bool   `json:"desktopOnly,omitempty"`
}

// agentTurnTimeout bounds one message to an agent on a runhost.
var agentTurnTimeout = 10 * time.Minute

// agentCommand runs `agentry agent sub args…` on runhost name and answers
// with its exit code, its stdout and its stderr.
func (r *RunhostsAPI) agentCommand(ctx context.Context, name, sub string, args ...string) (int, []byte, string, error) {
	_, _, c, err := r.dial(ctx, name)
	if err != nil {
		return 0, nil, "", err
	}
	defer func() { _ = c.Close() }()
	v, err := runhostVersion(ctx, c)
	if err != nil {
		return 0, nil, "", err
	}
	if carried := carriedCLIVersion(); carried != "" && v.Version != carried {
		return 0, nil, "", fmt.Errorf("runhost %s runs CLI %s and this app carries %s: update it from its card first", name, v.Version, carried)
	}
	var out bytes.Buffer
	code, stderr, err := remote.AgentCommand(ctx, c, sub, args, remote.AgentIO{Stdout: &out})
	return code, out.Bytes(), stderr, err
}

// failed is a command that did not succeed, in its own words.
func failed(sub string, code int, stderr string) error {
	msg := strings.TrimSpace(strings.TrimPrefix(lastLine(stderr), "agentry: "))
	if msg == "" {
		msg = fmt.Sprintf("agentry agent %s exited %d", sub, code)
	}
	return errors.New(msg)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}

// Agents lists the persistent agents on runhost name.
func (r *RunhostsAPI) Agents(name string) ([]RunhostAgentView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), 60*time.Second)
	defer cancel()
	code, out, stderr, err := r.agentCommand(ctx, name, "list", "--json")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, failed("list", code, stderr)
	}
	var list struct {
		Agents []struct {
			Agent       string `json:"agent"`
			DisplayName string `json:"displayName"`
			State       string `json:"state"`
			DesktopOnly bool   `json:"desktopOnly"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("runhost %s listed its agents in a form this app does not read: %w", name, err)
	}
	views := make([]RunhostAgentView, 0, len(list.Agents))
	for _, a := range list.Agents {
		views = append(views, RunhostAgentView{Name: a.Agent, Title: a.DisplayName, State: a.State, DesktopOnly: a.DesktopOnly})
	}
	return views, nil
}

// StartAgent starts agent on runhost name, enabling it first when it is
// not. What it would be given for the first time there is put to the
// person in a native dialog; it starts only once they agree.
func (r *RunhostsAPI) StartAgent(name, agent string) ([]RunhostAgentView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), 30*time.Minute)
	defer cancel()
	list, err := r.Agents(name)
	if err != nil {
		return nil, err
	}
	for _, a := range list {
		if a.Name == agent && a.State == "disabled" {
			if code, _, stderr, err := r.agentCommand(ctx, name, "enable", agent); err != nil {
				return nil, err
			} else if code != 0 {
				return nil, failed("enable", code, stderr)
			}
		}
	}
	code, out, stderr, err := r.agentCommand(ctx, name, "start", agent, "--json")
	if err != nil {
		return nil, err
	}
	if code == engine.ExitPolicy {
		var need struct {
			NeedsAgreement []struct {
				Line string `json:"line"`
			} `json:"needsAgreement"`
		}
		if json.Unmarshal(out, &need) == nil && len(need.NeedsAgreement) > 0 {
			lines := make([]string, 0, len(need.NeedsAgreement))
			for _, n := range need.NeedsAgreement {
				lines = append(lines, "• "+n.Line)
			}
			if err := r.app.askConsent("Start "+agent+" on runhost "+name+"?",
				"It would be given, for the first time on "+name+":\n\n"+strings.Join(lines, "\n")); err != nil {
				return nil, err
			}
			code, _, stderr, err = r.agentCommand(ctx, name, "start", agent, "--json", "--yes")
			if err != nil {
				return nil, err
			}
		}
	}
	if code != 0 {
		return nil, failed("start", code, stderr)
	}
	return r.Agents(name)
}

// StopAgent stops agent on runhost name; its conversation and files stay
// there for its next start.
func (r *RunhostsAPI) StopAgent(name, agent string) ([]RunhostAgentView, error) {
	ctx, cancel := context.WithTimeout(r.ctx(), 5*time.Minute)
	defer cancel()
	code, _, stderr, err := r.agentCommand(ctx, name, "stop", agent, "--json")
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, failed("stop", code, stderr)
	}
	return r.Agents(name)
}

// SendAgent sends text to agent on runhost name as one turn, through the
// agent's owner socket relayed here, and answers with its reply. Leaving
// leaves the agent running there.
func (r *RunhostsAPI) SendAgent(name, agent, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("the message is empty")
	}
	ctx, cancel := context.WithTimeout(r.ctx(), agentTurnTimeout)
	defer cancel()
	_, _, c, err := r.dial(ctx, name)
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	return sendThroughRelay(ctx, c, name, agent, text)
}

// sendThroughRelay is SendAgent over a runhost connection already made.
func sendThroughRelay(ctx context.Context, c remote.Caller, runhostName, agent, text string) (string, error) {
	home, key, stop, err := remote.RelayAgentSocket(ctx, c, agent)
	if err != nil {
		return "", err
	}
	defer stop()
	if _, err := persistent.QueryStatus(ctx, home, key); err != nil {
		return "", fmt.Errorf("the %s agent is not running on runhost %s: start it first", agent, runhostName)
	}
	s, err := persistent.Attach(ctx, home, key, nil)
	if err != nil {
		return "", err
	}
	defer func() { _, _ = s.End(context.WithoutCancel(ctx), false) }()
	replies := make(chan string, 1)
	go func() {
		var mine bool
		for ev := range s.Events() {
			switch ev.Kind {
			case session.KindUserTurn:
				mine = ev.Text == text
			case session.KindAssistantTurn:
				if mine {
					replies <- ev.Text
					return
				}
			}
		}
		close(replies)
	}()
	if err := s.Send(ctx, text); err != nil {
		if errors.Is(err, session.ErrBusy) {
			return "", fmt.Errorf("the %s agent is answering another chat on runhost %s: try again in a moment", agent, runhostName)
		}
		return "", err
	}
	select {
	case reply, ok := <-replies:
		if !ok {
			return "", fmt.Errorf("the %s agent's segment ended before it answered", agent)
		}
		return reply, nil
	case <-ctx.Done():
		return "", fmt.Errorf("no answer from the %s agent within %s; it finishes the turn on runhost %s by itself", agent, agentTurnTimeout, runhostName)
	}
}

// The runhost connection SendAgent goes over; a *runhost.Client.
var _ remote.Caller = (*runhost.Client)(nil)
