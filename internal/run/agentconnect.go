package run

// A persistent agent's remote MCP servers that sign in by OAuth, whether
// the person added them or their organization's managed settings put them
// on the agent, sign in with the person's own connection, kept for the
// agent under its own key: the vendor's consent in the browser, as a
// QuickAgent's Connect tools has it. The agent's supervisor reads the same
// store, so a running agent is given the server at its next turn.

import (
	"context"
	"fmt"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// AgentSignIn is how one of an agent's servers that sign in by OAuth
// stands: the address its connection is for, and whether a connection the
// vendor has not refused is kept.
type AgentSignIn struct {
	Server    string
	Connected bool
}

// AgentSignIns are the servers of agent a under cfg that sign in by OAuth,
// by roster entry id, on or off.
func AgentSignIns(home string, a persistent.Agent, cfg persistent.Config) map[string]AgentSignIn {
	conns := persistent.StoreConnections(home)
	out := map[string]AgentSignIn{}
	for _, srv := range persistent.SignInServers(a, cfg) {
		out[persistent.ServerEntryID(srv)] = AgentSignIn{Server: srv.URL, Connected: conns.Connected(persistent.ConnectionKey(a.Name, srv))}
	}
	return out
}

// agentSignInServer is a's server at the address server that signs in by
// OAuth under cfg.
func agentSignInServer(a persistent.Agent, cfg persistent.Config, server string) (manifest.MCPServer, error) {
	for _, srv := range persistent.SignInServers(a, cfg) {
		if srv.URL == server {
			return srv, nil
		}
	}
	return manifest.MCPServer{}, fmt.Errorf("the %s agent has no remote server %s that signs in by OAuth", a.Name, server)
}

// ConnectAgentServer signs agent a in to its server at the address server:
// the vendor's consent, the grant kept for the agent. A connection it had
// is replaced.
func ConnectAgentServer(ctx context.Context, home, regURL string, a persistent.Agent, cfg persistent.Config, server string, ui ConsentUI) error {
	srv, err := agentSignInServer(a, cfg, server)
	if err != nil {
		return err
	}
	return consentAndKeep(ctx, home, regURL, persistent.ConnectionKey(a.Name, srv), srv, ui, "")
}

// RemoveAgentConnection removes agent name's connection to server,
// revoked at the vendor where it offers that, and says whether there was
// one.
func RemoveAgentConnection(ctx context.Context, home, name, server string) (bool, error) {
	st := openStores(home)
	gone, err := persistent.RemoveAgentConnections(ctx, persistent.Removal{Store: st.conns, Clients: st.clients, HTTP: st.httpc}, name, server, "removed by the person")
	return len(gone) > 0, err
}
