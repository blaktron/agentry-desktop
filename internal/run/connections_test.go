package run

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/mcpauth"
	"github.com/blaktron/agentry-cli/pkg/mcpauth/mcpauthtest"
	"github.com/blaktron/agentry-cli/pkg/runner/connection"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
)

// memKeys is an OS keychain held in memory.
type memKeys struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKeys) Get(a string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[a]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (k *memKeys) Set(a, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[a] = v
	return nil
}

func (k *memKeys) Delete(a string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, a)
	return nil
}

// fakeVendorSeams points the vendor client at v and the keychain at keys,
// with no CIMD and no Agentry public client, and gives the runner home a
// folder of its own.
func fakeVendorSeams(t *testing.T, v *mcpauthtest.Vendor, keys connection.Keychain) string {
	t.Helper()
	oldHTTP, oldKeys, oldCIMD, oldAgentry := vendorHTTP, connectionKeys, nativeCIMD, agentryClients
	vendorHTTP = func() *http.Client { return v.Guard().Client() }
	connectionKeys = func() connection.Keychain { return keys }
	nativeCIMD = ""
	agentryClients = func(string, string) agentryClientFor { return noAgentryClient }
	t.Cleanup(func() { vendorHTTP, connectionKeys, nativeCIMD, agentryClients = oldHTTP, oldKeys, oldCIMD, oldAgentry })
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	return home
}

func remoteOAuthManifest(serverURL string) *manifest.Manifest {
	m := &manifest.Manifest{}
	m.Capabilities.MCPServers = []manifest.MCPServer{{URL: serverURL, Auth: manifest.AuthOAuth, Scopes: []string{"read"}, Tools: []string{"list_teams"}}}
	return m
}

const (
	refA    = "acme/inbox@1.0.0"
	digestA = "sha256:aaaa"
)

// browserUI approves the consent in the fake vendor's browser.
func browserUI(v *mcpauthtest.Vendor) ConsentUI { return ConsentUI{Present: v.Browser()} }

func TestAConnectionIsMadeOnceAndEveryStartAfterReusesIt(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR = true
	keys := &memKeys{m: map[string]string{}}
	home := fakeVendorSeams(t, v, keys)
	m := remoteOAuthManifest(v.ServerURL())
	server := v.ServerURL()

	before, err := connectionsOf(connection.Open(home, nil), refA, digestA, m)
	if err != nil || len(before.Servers) != 1 || before.Servers[0].State != ConnNone {
		t.Fatalf("before any consent: %+v, %v", before, err)
	}
	sc, err := connect(context.Background(), home, "", refA, digestA, m, server, browserUI(v))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if sc.State != ConnConnected || sc.Client != mcpauth.ClientDCR || sc.Issuer != v.URL {
		t.Fatalf("after consent: %+v", sc)
	}

	// Two starts: each refreshes, and neither asks for consent (the UI
	// is never given to openConnections at all).
	var tokens []string
	for i := 0; i < 2; i++ {
		rc, err := openConnections(context.Background(), home, refA, digestA, m)
		if err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		tok := rc.tokens[server]
		if tok.AccessToken == "" {
			t.Fatalf("start %d gave no access token: %+v", i, rc.tokens)
		}
		tokens = append(tokens, tok.AccessToken, rc.line)
		if !strings.Contains(rc.line, server) || !strings.Contains(rc.line, "access token alone") {
			t.Fatalf("receipt line: %q", rc.line)
		}
		renewed, err := rc.renew(context.Background(), server)
		if err != nil || renewed.AccessToken == "" {
			t.Fatalf("renew: %+v, %v", renewed, err)
		}
		tokens = append(tokens, renewed.AccessToken)
	}

	// No refresh token the vendor issued is in anything a run was given, nor in
	// any file of the runner home but the store's own secret, which the
	// keychain holds here instead.
	if len(v.Issued) < 2 {
		t.Fatalf("the vendor issued %d refresh tokens; rotation is not exercised", len(v.Issued))
	}
	for _, rt := range v.Issued {
		for _, got := range tokens {
			if strings.Contains(got, rt) || strings.Contains(got, base64.StdEncoding.EncodeToString([]byte(rt))) {
				t.Fatalf("a refresh token reached what a run is given: %q", got)
			}
		}
		assertNotUnder(t, home, rt)
	}
}

func assertNotUnder(t *testing.T, dir, secret string) {
	t.Helper()
	b64 := base64.StdEncoding.EncodeToString([]byte(secret))
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		raw, rerr := os.ReadFile(p) //nolint:gosec // the test's own temp dir
		if rerr == nil && (strings.Contains(string(raw), secret) || strings.Contains(string(raw), b64)) {
			t.Fatalf("%s holds a secret it must not", p)
		}
		return nil
	})
}

func TestAConnectionOpensForItsReleaseAloneAndAMissingOneRefusesTheRunByName(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR = true
	home := fakeVendorSeams(t, v, &memKeys{m: map[string]string{}})
	m := remoteOAuthManifest(v.ServerURL())
	if _, err := connect(context.Background(), home, "", refA, digestA, m, v.ServerURL(), browserUI(v)); err != nil {
		t.Fatal(err)
	}
	for _, other := range []struct{ ref, digest string }{
		{"acme/inbox@1.0.1", digestA}, // another version
		{refA, "sha256:bbbb"},         // another build of the same version
		{"acme/other@1.0.0", digestA}, // another QuickAgent
	} {
		_, err := openConnections(context.Background(), home, other.ref, other.digest, m)
		var ce *engine.ClassifiedError
		if !errors.As(err, &ce) || ce.Code != engine.ExitPolicy {
			t.Fatalf("%s %s: want exit 11, got %v", other.ref, other.digest, err)
		}
		if !strings.Contains(err.Error(), v.ServerURL()) || !strings.Contains(err.Error(), "Connect tools") {
			t.Fatalf("the refusal names neither the server nor where to connect it: %v", err)
		}
	}
}

func TestTheRedirectCanBePastedBack(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR = true
	home := fakeVendorSeams(t, v, &memKeys{m: map[string]string{}})
	m := remoteOAuthManifest(v.ServerURL())
	pasted := make(chan string, 2)
	var said []string
	ui := ConsentUI{
		Pasted: pasted,
		Say:    func(s string) { said = append(said, s) },
		// The browser is on another computer: the loopback is never
		// reached, and the person pastes the address it ended on, after
		// first pasting one that is not this sign-in's.
		Present: func(authURL string) {
			to, err := v.Redirect(authURL)
			if err != nil {
				t.Errorf("redirect: %v", err)
				return
			}
			pasted <- strings.Replace(to, "state=", "state=x", 1)
			pasted <- to
		},
	}
	sc, err := connect(context.Background(), home, "", refA, digestA, m, v.ServerURL(), ui)
	if err != nil || sc.State != ConnConnected {
		t.Fatalf("pasted back: %+v, %v", sc, err)
	}
	if !strings.Contains(strings.Join(said, "\n"), "state") {
		t.Fatalf("a pasted address of another sign-in was not said: %q", said)
	}
}

func TestAVendorWithNoRegistrationAsksForTheirOwnClientThenUsesIt(t *testing.T) {
	v := mcpauthtest.New(t)
	keys := &memKeys{m: map[string]string{}}
	home := fakeVendorSeams(t, v, keys)
	m := remoteOAuthManifest(v.ServerURL())

	_, err := connect(context.Background(), home, "", refA, digestA, m, v.ServerURL(), browserUI(v))
	var own *NeedsOwnClient
	if !errors.As(err, &own) || own.Issuer != v.URL || len(own.Guide.Steps) == 0 || own.Guide.Known {
		t.Fatalf("want the own-client ask with the general steps, got %v (%+v)", err, own)
	}

	const secret = "PROBE-CLIENT-SECRET-own"
	v.AddClient("my-client", secret)
	if err := SetOwnClient(v.URL, "my-client", secret); err != nil {
		t.Fatal(err)
	}
	sc, err := connect(context.Background(), home, "", refA, digestA, m, v.ServerURL(), browserUI(v))
	if err != nil || sc.Client != mcpauth.ClientOwn {
		t.Fatalf("with their own client: %+v, %v", sc, err)
	}
	rc, err := openConnections(context.Background(), home, refA, digestA, m)
	if err != nil {
		t.Fatal(err)
	}
	conns, clients, err := KeptConnections()
	if err != nil || len(conns) != 1 || len(clients) != 1 || clients[0].ClientID != "my-client" || clients[0].SecretIn != "keychain" {
		t.Fatalf("kept: %+v %+v %v", conns, clients, err)
	}
	// The secret is in no view, no line and no file: the keychain alone.
	views, _ := json.Marshal([]any{sc, rc.tokens, rc.line, conns, clients})
	if strings.Contains(string(views), secret) {
		t.Fatalf("the client secret is in a view: %s", views)
	}
	assertNotUnder(t, home, secret)
	if ok, err := ForgetOwnClient(v.URL); !ok || err != nil {
		t.Fatalf("forget: %v %v", ok, err)
	}
	if _, err := openConnections(context.Background(), home, refA, digestA, m); err == nil || !strings.Contains(err.Error(), "Connect it again") {
		t.Fatalf("a connection whose client is forgotten must ask again: %v", err)
	}
}

func TestDisconnectRevokesAtTheVendorAndRemoves(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR = true
	home := fakeVendorSeams(t, v, &memKeys{m: map[string]string{}})
	m := remoteOAuthManifest(v.ServerURL())
	if _, err := connect(context.Background(), home, "", refA, digestA, m, v.ServerURL(), browserUI(v)); err != nil {
		t.Fatal(err)
	}
	outcome, err := Disconnect(context.Background(), refA, digestA, v.ServerURL())
	if err != nil || outcome != connection.RevokeDone || len(v.Revoked) != 1 {
		t.Fatalf("disconnect: %q %v, revoked %v", outcome, err, v.Revoked)
	}
	rc, err := connectionsOf(connection.Open(home, nil), refA, digestA, m)
	if err != nil || rc.Servers[0].State != ConnNone {
		t.Fatalf("after disconnect: %+v %v", rc, err)
	}
}

func TestARefusedRefreshShowsBrokenAndConnectReplacesIt(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR = true
	home := fakeVendorSeams(t, v, &memKeys{m: map[string]string{}})
	m := remoteOAuthManifest(v.ServerURL())
	if _, err := connect(context.Background(), home, "", refA, digestA, m, v.ServerURL(), browserUI(v)); err != nil {
		t.Fatal(err)
	}
	v.Refused = true
	if _, err := openConnections(context.Background(), home, refA, digestA, m); err == nil || !strings.Contains(err.Error(), "no longer works") {
		t.Fatalf("a refused refresh: %v", err)
	}
	rc, _ := connectionsOf(connection.Open(home, nil), refA, digestA, m)
	if rc.Servers[0].State != ConnBroken || rc.Servers[0].Broken == "" {
		t.Fatalf("not shown broken: %+v", rc.Servers[0])
	}
	v.Refused = false
	sc, err := connect(context.Background(), home, "", refA, digestA, m, v.ServerURL(), browserUI(v))
	if err != nil || sc.State != ConnConnected {
		t.Fatalf("connect again: %+v %v", sc, err)
	}
	if _, err := openConnections(context.Background(), home, refA, digestA, m); err != nil {
		t.Fatalf("a start after connecting again: %v", err)
	}
}

func TestAPackageOnTheUnsupportedListIsRefusedBeforeConsent(t *testing.T) {
	v := mcpauthtest.New(t)
	home := fakeVendorSeams(t, v, nil)
	m := &manifest.Manifest{}
	m.Capabilities.MCPServers = []manifest.MCPServer{{PyPI: "mcp-gsuite", Version: "0.4.1", OAuth: &manifest.MCPOAuth{Issuer: v.URL}}}
	rc, err := connectionsOf(connection.Open(home, nil), refA, digestA, m)
	if err != nil || rc.Servers[0].Unsupported == "" {
		t.Fatalf("the list's alternative is not shown: %+v %v", rc, err)
	}
	presented := false
	_, err = connect(context.Background(), home, "", refA, digestA, m, "pypi:mcp-gsuite", ConsentUI{Present: func(string) { presented = true }})
	if err == nil || presented || !strings.Contains(err.Error(), rc.Servers[0].Unsupported) {
		t.Fatalf("want a refusal naming the alternative before any consent: %v (presented %v)", err, presented)
	}
}

func TestAPackageServerSignsInAtItsIssuer(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR, v.IssuerOnly = true, true
	home := fakeVendorSeams(t, v, &memKeys{m: map[string]string{}})
	m := &manifest.Manifest{}
	m.Capabilities.MCPServers = []manifest.MCPServer{{PyPI: "workspace-mcp", Version: "2.0.1",
		OAuth: &manifest.MCPOAuth{Issuer: v.URL, Scopes: []string{"read"}}}}
	sc, err := connect(context.Background(), home, "", refA, digestA, m, "pypi:workspace-mcp", browserUI(v))
	if err != nil || sc.Issuer != v.URL || sc.What != "pypi:workspace-mcp (signing in at "+v.URL+")" {
		t.Fatalf("package server: %+v %v", sc, err)
	}
	rc, err := openConnections(context.Background(), home, refA, digestA, m)
	if err != nil || rc.tokens["pypi:workspace-mcp"].AccessToken == "" {
		t.Fatalf("start: %+v %v", rc, err)
	}
}

func TestRemoveConnectionsOfTakesEveryVersionOfThatQuickAgentAlone(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR = true
	home := fakeVendorSeams(t, v, &memKeys{m: map[string]string{}})
	m := remoteOAuthManifest(v.ServerURL())
	for _, ref := range []string{"acme/inbox@1.0.0", "acme/inbox@1.1.0", "acme/inbox-two@1.0.0"} {
		if _, err := connect(context.Background(), home, "", ref, digestA, m, v.ServerURL(), browserUI(v)); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := RemoveConnectionsOf(context.Background(), "acme", "inbox")
	if err != nil || len(removed) != 2 {
		t.Fatalf("removed %v, %v", removed, err)
	}
	conns, _, _ := KeptConnections()
	if len(conns) != 1 || conns[0].Ref != "acme/inbox-two@1.0.0" {
		t.Fatalf("left: %+v", conns)
	}
}

func TestTheOwnClientAskCarriesThePublishersHelpForThatServer(t *testing.T) {
	v := mcpauthtest.New(t)
	keys := &memKeys{m: map[string]string{}}
	home := fakeVendorSeams(t, v, keys)

	m := remoteOAuthManifest(v.ServerURL())
	m.Guide = &manifest.Guide{Connect: map[string]string{
		"url:" + v.ServerURL(): "Make a **native** client under Developer apps.\nThen copy its id.\u202e\u0007\t",
		"LINEAR_TOKEN":         "a secret's help is not this server's",
	}}
	_, err := connect(context.Background(), home, "", refA, digestA, m, v.ServerURL(), browserUI(v))
	var own *NeedsOwnClient
	if !errors.As(err, &own) {
		t.Fatalf("want the own-client ask, got %v", err)
	}
	if want := "Make a **native** client under Developer apps.\nThen copy its id."; own.Guide.Help != want {
		t.Fatalf("the publisher's help = %q, want %q (control and format characters removed)", own.Guide.Help, want)
	}
	if len(own.Guide.Steps) == 0 {
		t.Fatal("Agentry's own steps are gone")
	}

	// A release with no help for this server, or help under its bare
	// address rather than its source id, carries none.
	for _, g := range []*manifest.Guide{nil, {Connect: map[string]string{v.ServerURL(): "bare address"}}} {
		m.Guide = g
		_, err := connect(context.Background(), home, "", refA, digestA, m, v.ServerURL(), browserUI(v))
		if !errors.As(err, &own) || own.Guide.Help != "" {
			t.Fatalf("guide %+v: want the ask with no help, got %v (%+v)", g, err, own)
		}
	}
}

func TestGooglesStepsAreAgentrysAndCarryTheTestUsersStep(t *testing.T) {
	gmail := manifest.MCPServer{URL: "https://gmailmcp.googleapis.com/mcp/v1", Auth: manifest.AuthOAuth,
		Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"}}
	g := OwnClientSteps("https://accounts.google.com/", gmail)
	all := strings.Join(g.Steps, "\n")
	for _, want := range []string{"Test users", "Desktop app", "the Gmail API and the Gmail MCP API", "gmail.readonly", "7 days", "Internal", "Publish app", "Developer Preview", "workspace-mcp"} {
		if !strings.Contains(all, want) {
			t.Errorf("Google's steps lack %q:\n%s", want, all)
		}
	}
	if !g.Known || g.Vendor != "Google" || g.Docs != OwnClientDocs {
		t.Fatalf("not Agentry's own steps: %+v", g)
	}
	pkg := manifest.MCPServer{PyPI: "workspace-mcp", Egress: []string{"gmail.googleapis.com"},
		OAuth: &manifest.MCPOAuth{Issuer: GoogleIssuer}}
	if s := strings.Join(OwnClientSteps(GoogleIssuer, pkg).Steps, "\n"); !strings.Contains(s, "gmail.googleapis.com") || strings.Contains(s, "Developer Preview") {
		t.Fatalf("a package server's steps must name the hosts it reaches and ask for no preview enrolment:\n%s", s)
	}
	if other := OwnClientSteps("https://auth.example.com", gmail); other.Known || other.Vendor != "" {
		t.Fatalf("another vendor's steps claim to be Agentry's: %+v", other)
	}
}

// oauthRunManifest is the lifecycle fixture with a remote server that
// signs in by OAuth.
const oauthRunManifest = `schemaVersion: "2"
name: reporter
namespace: acme
modes: [autonomous]
version: 1.0.0
summary: "desktop run lifecycle fixture, with a remote server that signs in by OAuth"
entrypoints:
  instruction: SKILL.md
capabilities:
  filesystem:
    read: workspace
    write: workspace
  mcpServers:
    - url: https://mcp.example.com/mcp
      auth: oauth
      scopes: [read]
      tools: [list_teams]
publish:
  include: [agent.yaml, SKILL.md]
`

func oauthRunFixture(t *testing.T) (*fakeRunRegistry, string, string) {
	t.Helper()
	home := isolateRunEnv(t)
	saveCataloguedPolicy(t)
	fr := &fakeRunRegistry{
		manifestDoc: manifestDocFromYAML(t, oauthRunManifest),
		archive: buildRunArchive(t, map[string]string{
			"agent.yaml": oauthRunManifest,
			"SKILL.md":   "# Reporter\n\nList the teams.\n",
		}),
	}
	regURL := startRunRegistry(t, fr)
	fake := readyImagesFake()
	scriptHappyAgent(fake)
	withFakeDocker(t, fake)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	return fr, regURL, home
}

func TestARunWithNoConnectionIsRefusedByNameBeforeItStarts(t *testing.T) {
	_, regURL, _ := oauthRunFixture(t)
	_, err := Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &strings.Builder{})
	var ce *engine.ClassifiedError
	if !errors.As(err, &ce) || ce.Code != engine.ExitPolicy {
		t.Fatalf("want exit 11 before the engine, got %v", err)
	}
	if !strings.Contains(err.Error(), "https://mcp.example.com/mcp") || !strings.Contains(err.Error(), "Connect tools") {
		t.Fatalf("the refusal must name the server and where to connect it: %v", err)
	}
}

func TestARunIsGivenItsConnectionsAccessTokenAndTheReceiptLine(t *testing.T) {
	fr, regURL, home := oauthRunFixture(t)
	v := mcpauthtest.New(t)
	v.DCR = true
	oldHTTP, oldKeys, oldCIMD := vendorHTTP, connectionKeys, nativeCIMD
	vendorHTTP = func() *http.Client { return v.Guard().Client() }
	mem := &memKeys{m: map[string]string{}}
	connectionKeys = func() connection.Keychain { return mem }
	nativeCIMD = ""
	t.Cleanup(func() { vendorHTTP, connectionKeys, nativeCIMD = oldHTTP, oldKeys, oldCIMD })

	// The grant is made at the fake vendor and kept under this release's
	// key for the manifest's server.
	g, err := mcpauth.Consent{HTTP: v.Guard().Client(), ServerURL: v.ServerURL(), Present: v.Browser()}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	keys := connectionKeys()
	if _, err := connection.Open(home, keys).Put(connection.Key{Ref: "acme/reporter@1.0.0", Digest: fr.digest, Server: "https://mcp.example.com/mcp"}, g, ""); err != nil {
		t.Fatal(err)
	}
	p, err := prepare(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &strings.Builder{}, manifest.ModeAutonomous)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	tok := p.params.Connections["https://mcp.example.com/mcp"]
	if tok.AccessToken == "" || p.params.RenewConnection == nil {
		t.Fatalf("the engine is not given the connection: %+v", p.params.Connections)
	}
	line := p.plan.Enforcement["mcpServers.connections"]
	if !strings.Contains(line, "https://mcp.example.com/mcp") || !strings.Contains(line, "access token alone") {
		t.Fatalf("receipt line: %q", line)
	}
	for _, rt := range v.Issued {
		if strings.Contains(line, rt) || tok.AccessToken == rt {
			t.Fatal("a refresh token reached the run")
		}
	}
}

func TestAStartRefusesAnUnsupportedPackageAndConnectAnUndeclaredServer(t *testing.T) {
	v := mcpauthtest.New(t)
	home := fakeVendorSeams(t, v, nil)
	m := &manifest.Manifest{}
	m.Capabilities.MCPServers = []manifest.MCPServer{{PyPI: "mcp-gsuite", Version: "0.4.1", OAuth: &manifest.MCPOAuth{Issuer: v.URL}}}
	_, err := openConnections(context.Background(), home, refA, digestA, m)
	var ce *engine.ClassifiedError
	if !errors.As(err, &ce) || ce.Code != engine.ExitPolicy || !strings.Contains(err.Error(), "access token alone") {
		t.Fatalf("a start with an unsupported package: %v", err)
	}
	presented := false
	_, err = connect(context.Background(), home, "", refA, digestA, remoteOAuthManifest(v.ServerURL()), "https://elsewhere.example/mcp",
		ConsentUI{Present: func(string) { presented = true }})
	if err == nil || presented || !strings.Contains(err.Error(), "declares no MCP server") {
		t.Fatalf("a server the release does not declare must be refused before consent: %v (presented %v)", err, presented)
	}
}

// Google revokes an account's whole grant to a client, not one refresh
// token. Connecting again with the same client must therefore not revoke
// the old token: that would end the new grant, and every other
// connection made with the client.
func TestConnectingAgainWithTheSameClientRevokesNothing(t *testing.T) {
	v := mcpauthtest.New(t)
	home := fakeVendorSeams(t, v, &memKeys{m: map[string]string{}})
	v.AddClient("my-client", "PROBE-CLIENT-SECRET-again")
	if err := SetOwnClient(v.URL, "my-client", "PROBE-CLIENT-SECRET-again"); err != nil {
		t.Fatal(err)
	}
	m := remoteOAuthManifest(v.ServerURL())
	for i := 0; i < 2; i++ {
		var said []string
		ui := browserUI(v)
		ui.Say = func(s string) { said = append(said, s) }
		if _, err := connect(context.Background(), home, "", refA, digestA, m, v.ServerURL(), ui); err != nil {
			t.Fatal(err)
		}
		if i == 1 && !strings.Contains(strings.Join(said, "\n"), "not revoked") {
			t.Fatalf("the replacement does not say it revoked nothing: %q", said)
		}
	}
	if len(v.Revoked) != 0 {
		t.Fatalf("connecting again revoked %v", v.Revoked)
	}
	if _, err := openConnections(context.Background(), home, refA, digestA, m); err != nil {
		t.Fatalf("the new connection does not work: %v", err)
	}
}

func TestConnectionsSayWhichOthersShareTheirGrant(t *testing.T) {
	v := mcpauthtest.New(t)
	v.DCR = true
	home := fakeVendorSeams(t, v, &memKeys{m: map[string]string{}})
	v.AddClient("my-client", "s")
	if err := SetOwnClient(v.URL, "my-client", "s"); err != nil {
		t.Fatal(err)
	}
	m := remoteOAuthManifest(v.ServerURL())
	for _, ref := range []string{"acme/inbox@1.0.0", "acme/digest@2.0.0"} {
		if _, err := connect(context.Background(), home, "", ref, digestA, m, v.ServerURL(), browserUI(v)); err != nil {
			t.Fatal(err)
		}
	}
	conns, _, err := KeptConnections()
	if err != nil || len(conns) != 2 {
		t.Fatalf("%+v %v", conns, err)
	}
	for _, c := range conns {
		if len(c.SharesGrantWith) != 1 || strings.HasPrefix(c.SharesGrantWith[0], c.Ref) {
			t.Fatalf("%s shares with %v", c.Ref, c.SharesGrantWith)
		}
	}
	rc, _ := connectionsOf(connection.Open(home, nil), "acme/inbox@1.0.0", digestA, m)
	if got := rc.Servers[0].SharesGrantWith; len(got) != 1 || got[0] != "acme/digest@2.0.0 → "+v.ServerURL() {
		t.Fatalf("the run setup's view: %v", got)
	}
	// A client registered per consent shares nothing.
	if _, err := ForgetOwnClient(v.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := connect(context.Background(), home, "", "acme/other@1.0.0", digestA, m, v.ServerURL(), browserUI(v)); err != nil {
		t.Fatal(err)
	}
	rc, _ = connectionsOf(connection.Open(home, nil), "acme/other@1.0.0", digestA, m)
	if len(rc.Servers[0].SharesGrantWith) != 0 {
		t.Fatalf("a registered client shares: %v", rc.Servers[0].SharesGrantWith)
	}
}
