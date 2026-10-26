package run

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/mcpauth"
	"github.com/blaktron/agentry-cli/pkg/mcpauth/mcpauthtest"
)

func noAgentryClient(context.Context, manifest.MCPServer, string) string { return "" }

// Agentry's own public client comes first for a server the connector
// catalogue vets, where the registry serves its id: before the person's own
// client and a registration, with no secret, and the connection refreshes
// with it.
func TestAgentrysOwnAppComesFirstForAVettedServer(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR = true
	v.AddClient("agentry-public-id", "")
	keys := &memKeys{m: map[string]string{}}
	home := fakeVendorSeams(t, v, keys)
	m := remoteOAuthManifest(v.ServerURL())
	server := v.ServerURL()
	if err := SetOwnClient(v.URL, "mine", "my-secret"); err != nil {
		t.Fatal(err)
	}
	v.AddClient("mine", "my-secret")

	var asked []string
	agentryClients = func(gotHome, regURL string) agentryClientFor {
		asked = append(asked, gotHome+" "+regURL)
		return func(_ context.Context, srv manifest.MCPServer, issuer string) string {
			if srv.URL == server && issuer == v.URL {
				return "agentry-public-id"
			}
			return ""
		}
	}
	sc, err := connect(context.Background(), home, "https://registry.example", refA, digestA, m, server, browserUI(v))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if sc.State != ConnConnected || sc.Client != mcpauth.ClientAgentry {
		t.Fatalf("after consent: %+v", sc)
	}
	if len(asked) != 1 || asked[0] != home+" https://registry.example" {
		t.Fatalf("Agentry's clients were opened for %q", asked)
	}
	if len(v.Registrations) != 0 {
		t.Fatal("a client was registered where Agentry's own app comes first")
	}
	last := v.Requests[len(v.Requests)-1]
	if last.Get("client_id") != "agentry-public-id" || last.Get("client_secret") != "" {
		t.Fatalf("the exchange: %v", last)
	}
	rc, err := openConnections(context.Background(), home, refA, digestA, m)
	if err != nil || rc.tokens[server].AccessToken == "" {
		t.Fatalf("a start with Agentry's client: %+v, %v", rc, err)
	}
	if last := v.Requests[len(v.Requests)-1]; last.Get("grant_type") != "refresh_token" || last.Get("client_id") != "agentry-public-id" || last.Get("client_secret") != "" {
		t.Fatalf("the refresh: %v", last)
	}
}

// With no id served for the server, the order goes on as before: the
// person's own client where they keep one, else a registration.
func TestWithNoAgentryAppTheOrderIsAsBefore(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR = true
	keys := &memKeys{m: map[string]string{}}
	home := fakeVendorSeams(t, v, keys)
	m := remoteOAuthManifest(v.ServerURL())
	sc, err := connect(context.Background(), home, "https://registry.example", refA, digestA, m, v.ServerURL(), browserUI(v))
	if err != nil || sc.Client != mcpauth.ClientDCR {
		t.Fatalf("with none served: %+v, %v", sc, err)
	}
	if err := SetOwnClient(v.URL, "mine", "my-secret"); err != nil {
		t.Fatal(err)
	}
	v.AddClient("mine", "my-secret")
	sc, err = connect(context.Background(), home, "https://registry.example", refA, digestA, m, v.ServerURL(), browserUI(v))
	if err != nil || sc.Client != mcpauth.ClientOwn {
		t.Fatalf("with the person's own client: %+v, %v", sc, err)
	}
}

// A server the catalogue does not vet for a public client's connector gets
// none of the registry's ids, and the registry is not even asked: GitHub's
// app is confidential, and an address no connector lists is not vetted.
func TestAServerTheCatalogueDoesNotVetForAPublicClientGetsNone(t *testing.T) {
	asked := 0
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		_ = json.NewEncoder(w).Encode(map[string]any{"connectors": []map[string]string{
			{"id": "microsoft-365", "publicClientId": "00000000-0000-4000-8000-000000001262"},
		}})
	}))
	t.Cleanup(reg.Close)
	home := t.TempDir()
	forServer := openAgentryClients(home, reg.URL)
	for _, c := range []struct {
		srv    manifest.MCPServer
		issuer string
	}{
		{manifest.MCPServer{URL: "https://api.githubcopilot.com/mcp", Auth: manifest.AuthOAuth}, "https://github.com/login/oauth"},
		{manifest.MCPServer{URL: "https://graph.example/mcp", Auth: manifest.AuthOAuth}, "https://login.microsoftonline.com/common/v2.0"},
	} {
		if id := forServer(context.Background(), c.srv, c.issuer); id != "" {
			t.Errorf("%s at %s got %q", c.srv.URL, c.issuer, id)
		}
	}
	if asked != 0 {
		t.Fatalf("the registry was asked %d times for servers nothing vets", asked)
	}
	if _, err := os.Stat(filepath.Join(home, "connectors")); !os.IsNotExist(err) {
		t.Fatalf("something was kept for servers nothing vets: %v", err)
	}
}
