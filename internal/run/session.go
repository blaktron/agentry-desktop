package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/session"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
)

// Session is an interactive run driven from the app (plan §7): the CLI's
// session API behind the desktop's one-session-at-a-time rule (D-017).
// Everything the chat can do is a method here; the engine enforces the
// plan and ends the session on the operator's caps regardless.
type Session struct {
	// mu serializes End: the chat's End and the app's quit can race
	// (agentry-desktop#125), and the engine's End must run once.
	mu     sync.Mutex
	inner  session.Session
	ref    string
	prof   *prepared
	logw   io.Writer
	ended  bool
	result *Result
	endErr error
}

// ErrNotInteractive says the release does not declare the interactive mode.
var ErrNotInteractive = errors.New("this release does not declare the interactive run mode")

// StartSession prepares the run exactly as Run does and starts the
// interactive plan's harness. The task text, if any, is the opening turn.
func StartSession(ctx context.Context, req Request, logw io.Writer) (*Session, error) {
	sig, err := resolveForRun(ctx, req.Registry, req.Ref, true)
	if err != nil {
		return nil, err
	}
	if !sig.Manifest.Supports(manifest.ModeInteractive) {
		return nil, ErrNotInteractive
	}
	if req.ChatIdleTimeoutSeconds < 0 {
		return nil, fmt.Errorf("a chat's idle timeout is 0 (none) or more seconds, not %d", req.ChatIdleTimeoutSeconds)
	}
	p, err := prepare(ctx, req, logw, manifest.ModeInteractive)
	if err != nil {
		return nil, err
	}
	// The compile took the session caps from the runner policy, which the
	// CLI's chats read too, tightened by the release's idle hint. A chat
	// here has its own (D21, D22): the engine reads them from this plan in
	// memory, and the receipt's run-plan.json records them. The run timeout
	// is no limit on a session, so the plan and meta.json say none rather
	// than the policy's. The policy file and the installed plan.json are not
	// changed.
	if p.plan.Harness.Session == nil {
		finishProfile(logw, p.prof, p.profDir, p.keep)
		return nil, fmt.Errorf("the interactive plan for %s has no session block", p.sig.Ref)
	}
	p.plan.Harness.Session.Budgets = spec.SessionBudgets{IdleTimeoutSeconds: req.ChatIdleTimeoutSeconds}
	p.plan.Budgets.TimeoutSeconds, p.params.Budgets.TimeoutSeconds = 0, 0
	inner, err := engine.StartSession(ctx, p.docker, p.params)
	if err != nil {
		finishProfile(logw, p.prof, p.profDir, p.keep)
		return nil, err
	}
	return &Session{inner: inner, ref: p.sig.Ref, prof: p, logw: logw}, nil
}

// Ref is namespace/name@version.
func (s *Session) Ref() string { return s.ref }

// Events streams the session's events; closed after End.
func (s *Session) Events() <-chan session.Event { return s.inner.Events() }

// Send delivers one user turn and returns when the harness ends it.
func (s *Session) Send(ctx context.Context, text string) error { return s.inner.Send(ctx, text) }

// AddInput stages one host file under /inputs and tells the agent.
func (s *Session) AddInput(ctx context.Context, path string) error {
	return s.inner.AddInput(ctx, path)
}

// Ended reports whether the engine has ended the session and why.
func (s *Session) Ended() (bool, session.EndReason) { return s.inner.Ended() }

// End finishes the session (asking for the final result first when finish
// is set and the release declares an output schema), applies the profile
// keep choice like Run, and returns the flattened envelope. Idempotent.
func (s *Session) End(ctx context.Context, finish bool) (*Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return s.result, s.endErr
	}
	s.ended = true
	env, err := s.inner.End(ctx, finish)
	finishProfile(s.logw, s.prof.prof, s.prof.profDir, s.prof.keep)
	if env != nil {
		s.result = resultFromEnvelope(env, s.prof.plan)
		s.endErr = nil
		return s.result, nil
	}
	s.endErr = err
	return nil, err
}
