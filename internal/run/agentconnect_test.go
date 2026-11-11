package run

import (
	"context"
	"errors"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/mcpauth/mcpauthtest"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// The Assistant with a remote server the person added that signs in by
// OAuth: it has no connection until Sign in makes one under the agent's
// key, the agent's supervisor then finds it, and Remove takes it away,
// revoked at the vendor.
func TestAnAgentsServerSignsInFromTheAppAndIsRemoved(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR = true
	home := fakeVendorSeams(t, v, &memKeys{m: map[string]string{}})
	a, err := persistent.Lookup("assistant")
	if err != nil {
		t.Fatal(err)
	}
	srv := manifest.MCPServer{URL: v.ServerURL(), Auth: manifest.AuthOAuth, Scopes: []string{"read"}, Tools: []string{"list_teams"}}
	cfg := persistent.Config{Servers: []manifest.MCPServer{srv}}
	id := persistent.ServerEntryID(srv)

	if got := AgentSignIns(home, a, cfg); got[id].Server != v.ServerURL() || got[id].Connected {
		t.Fatalf("before a sign-in: %+v", got)
	}
	if err := ConnectAgentServer(context.Background(), home, "", a, cfg, "https://elsewhere.example/mcp", browserUI(v)); err == nil {
		t.Fatal("a server the agent does not have was signed in to")
	}
	if err := ConnectAgentServer(context.Background(), home, "", a, cfg, v.ServerURL(), browserUI(v)); err != nil {
		t.Fatalf("sign in: %v", err)
	}
	if got := AgentSignIns(home, a, cfg); !got[id].Connected {
		t.Fatalf("after the sign-in: %+v", got)
	}
	if !persistent.StoreConnections(home).Connected(persistent.ConnectionKey(a.Name, srv)) {
		t.Fatal("the agent's supervisor does not find the connection under the agent's key")
	}

	gone, err := RemoveAgentConnection(context.Background(), home, a.Name, v.ServerURL())
	if err != nil || !gone {
		t.Fatalf("remove: %v %v", gone, err)
	}
	if len(v.Revoked) == 0 {
		t.Error("the connection was not revoked at the vendor")
	}
	if got := AgentSignIns(home, a, cfg); got[id].Connected {
		t.Fatalf("after Remove: %+v", got)
	}
	if gone, err := RemoveAgentConnection(context.Background(), home, a.Name, v.ServerURL()); err != nil || gone {
		t.Fatalf("a second Remove: %v %v", gone, err)
	}
}

// A vendor that needs the person's own OAuth client answers the agent's
// sign-in with Agentry's steps for making one, as Connect tools does.
func TestAnAgentsSignInAsksForAnOwnClientWhereTheVendorNeedsOne(t *testing.T) {
	v := mcpauthtest.New(t)
	home := fakeVendorSeams(t, v, &memKeys{m: map[string]string{}})
	a, _ := persistent.Lookup("assistant")
	srv := manifest.MCPServer{URL: v.ServerURL(), Auth: manifest.AuthOAuth}
	err := ConnectAgentServer(context.Background(), home, "", a, persistent.Config{Servers: []manifest.MCPServer{srv}}, v.ServerURL(), browserUI(v))
	var own *NeedsOwnClient
	if !errors.As(err, &own) || own.Issuer == "" {
		t.Fatalf("got %v, want the own-client steps", err)
	}
}
