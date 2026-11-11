package app

// A persistent agent's remote MCP servers that sign in by OAuth sign in
// from their card: Sign in opens the vendor's consent in the person's
// browser and keeps the connection for the agent; Remove removes it,
// revoked at the vendor where it offers that. A running agent is given the
// server at its next turn, since its supervisor reads the same store.

import (
	"context"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The seams the tests replace.
var (
	connectAgentServer    = run.ConnectAgentServer
	removeAgentConnection = run.RemoveAgentConnection
)

// SignInServer signs agent name in to its server at the address server:
// the vendor's consent in the person's browser, under way until the vendor
// answers, the person cancels (CancelConnect) or it times out. Its
// address and what it says reach the page as Connect tools' do, and a
// redirect pasted with PasteRedirect is taken. A connection the server had
// is replaced.
func (p *PersistentAPI) SignInServer(name, server string) (*ConnectView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err != nil {
		return nil, err
	}
	reg := ""
	if s, err := settings.Load(); err == nil {
		reg = s.DefaultRegistry
	}
	v, err := p.run.consent(server, func(ctx context.Context, ui run.ConsentUI) error {
		return connectAgentServer(ctx, home, reg, a, cfg, server, ui)
	})
	if v != nil && v.Done {
		p.stateChanged(a.Name)
	}
	return v, err
}

// RemoveServerSignIn removes agent name's connection to server, revoked at
// the vendor where it offers that, and returns the agent as it now stands.
func (p *PersistentAPI) RemoveServerSignIn(name, server string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	_, err = removeAgentConnection(p.run.ctx(), home, a.Name, server)
	// The record may be gone though the vendor's revocation failed: the
	// card reads the agent again either way.
	p.stateChanged(a.Name)
	if err != nil {
		return nil, err
	}
	return p.Get(a.Name)
}
