// Connections on the desktop (plan mcp-oauth-connections.md §6, M5, D4–D6,
// D10, D16, D17, D20–D23; agentry-desktop#666): a person's OAuth grant to a
// declared MCP server that signs in by OAuth, for one QuickAgent release
// (ref plus archive digest), kept in the runner home's connection store
// with its refresh token in the OS keychain, the store the CLI keeps
// (agentry-cli#664, #665). A connection made by `agentry connect` is this
// app's too, and one made here is the CLI's.
//
// The run setup's Connect tools step makes them (Connect); a run never
// asks: prepare opens each one and refreshes it before the sandbox starts,
// refusing the run by name when one is missing, and the engine renews it
// while the run lasts. A run is given an access token and nothing else.
package run

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/mcpauth"
	"github.com/blaktron/agentry-cli/pkg/runner/connection"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/profile"
)

// Seams for tests: the vendor client (the egress guard in production), the
// keychain (the OS's) and Agentry's Client ID Metadata Document.
var (
	vendorHTTP     = func() *http.Client { return mcpauth.Guard{}.Client() }
	connectionKeys = func() connection.Keychain { return connection.OSKeychain{} }
	nativeCIMD     = mcpauth.NativeClientMetadataURL
)

// revokeTimeout bounds one removal's revocation at the vendor, as the
// CLI's does: a vendor that does not answer in time is recorded as a
// failed revocation and the connection is removed all the same.
const revokeTimeout = 30 * time.Second

// Connection states, as the run setup and Settings show them.
const (
	ConnConnected = "connected"
	ConnNone      = "none"
	ConnBroken    = "broken"
)

// ServerConnection is one declared server that signs in by OAuth, and the
// connection this release keeps for it. Never a token or a client secret.
type ServerConnection struct {
	// Server is the connection's key: a remote server's address, or a
	// package server's ecosystem-qualified id (pypi:workspace-mcp).
	Server string `json:"server"`
	// What names it in the person's words.
	What string `json:"what"`
	// Issuer is where it signs in: a package server's declared issuer, or
	// the one a kept connection was made at; "" for a remote server not
	// yet connected (its own metadata names it at Connect).
	Issuer string `json:"issuer,omitempty"`
	// Scopes are what the release asks for; none is the vendor's default.
	Scopes []string `json:"scopes"`
	// State is ConnConnected, ConnNone or ConnBroken; Broken says why the
	// vendor no longer honours it.
	State  string `json:"state"`
	Broken string `json:"broken,omitempty"`
	// Label is the account the vendor named, where it said one.
	Label string `json:"label,omitempty"`
	// Client is the kind of client it signs in as: own, cimd, dcr.
	Client string `json:"client,omitempty"`
	// Granted are the scopes the vendor granted, where it said.
	Granted    []string `json:"granted"`
	CreatedAt  string   `json:"createdAt,omitempty"`
	LastUsedAt string   `json:"lastUsedAt,omitempty"`
	// Unsupported is the alternative for a package that cannot run on an
	// access token alone (plan D25): Connect refuses it, as its run would.
	Unsupported string `json:"unsupported,omitempty"`
}

// ReleaseConnections is a release's servers that sign in by OAuth.
type ReleaseConnections struct {
	Ref     string             `json:"ref"`
	Digest  string             `json:"digest"`
	Servers []ServerConnection `json:"servers"`
}

// oauthServers are the release's servers that sign in by OAuth: remote
// servers declared auth: oauth, and package servers with an oauth block.
func oauthServers(m *manifest.Manifest) []manifest.MCPServer {
	var out []manifest.MCPServer
	if m == nil {
		return out
	}
	for _, s := range m.Capabilities.MCPServers {
		if s.NeedsConnection() {
			out = append(out, s)
		}
	}
	return out
}

// connectionServer is how a connection names its server, the third part
// of its key and engine.Params.Connections' key, the CLI's own rule
// (spec.MCPServerSpec.ConnectionKey).
func connectionServer(srv manifest.MCPServer) string {
	if srv.IsRemote() {
		return srv.URL
	}
	return srv.SourceID()
}

// connectionScopes are the scopes a server's consent asks for.
func connectionScopes(srv manifest.MCPServer) []string {
	if srv.OAuth != nil {
		return srv.OAuth.Scopes
	}
	return srv.Scopes
}

// connectionWhat names a server: a remote server's address, or a package
// server and where it signs in.
func connectionWhat(srv manifest.MCPServer) string {
	if srv.IsRemote() {
		return srv.URL
	}
	return srv.SourceID() + " (signing in at " + srv.OAuth.Issuer + ")"
}

// unsupportedRefusal is the refusal of a package on the unsupported list
// (D25), before any consent and at a run's start alike.
func unsupportedRefusal(srv manifest.MCPServer, alt string) error {
	return engine.Classify(engine.ExitPolicy, fmt.Errorf("declared MCP server %s cannot run on an access token alone, so it cannot take a connection: run a version of the QuickAgent that declares %s instead", srv.SourceID(), alt))
}

// stores are the connection store and the own-client store of a runner
// home, with the client that reaches vendors: what every consent, start
// and removal opens.
type stores struct {
	conns   *connection.Store
	clients *connection.Clients
	httpc   *http.Client
}

func openStores(home string) stores {
	keys := connectionKeys()
	return stores{conns: connection.Open(home, keys), clients: connection.OpenClients(home, keys), httpc: vendorHTTP()}
}

// stateOf is a kept connection's state and why it is broken.
func stateOf(c *connection.Connection) (string, string) {
	if c.Broken != "" {
		return ConnBroken, c.Broken
	}
	return ConnConnected, ""
}

func unsupportedAlternative(srv manifest.MCPServer) string {
	if srv.IsRemote() {
		return ""
	}
	if alt, bad := manifest.UnsupportedOAuthPackage(srv.SourceID()); bad {
		return alt
	}
	return ""
}

// ConnectionsFor is the connection state of each server of ref's release
// that signs in by OAuth: the release a run of ref would run (a local
// install first, then the registry's).
func ConnectionsFor(ctx context.Context, regURL, ref string) (*ReleaseConnections, error) {
	sig, err := resolveForRun(ctx, regURL, ref, false)
	if err != nil {
		return nil, err
	}
	home, err := Home()
	if err != nil {
		return nil, err
	}
	return connectionsOf(connection.Open(home, nil), sig.Ref, sig.Digest, sig.Manifest)
}

func connectionsOf(store *connection.Store, ref, digest string, m *manifest.Manifest) (*ReleaseConnections, error) {
	out := &ReleaseConnections{Ref: ref, Digest: digest, Servers: []ServerConnection{}}
	for _, srv := range oauthServers(m) {
		sc := ServerConnection{
			Server: connectionServer(srv), What: connectionWhat(srv),
			Scopes: append([]string{}, connectionScopes(srv)...), State: ConnNone, Granted: []string{},
			Unsupported: unsupportedAlternative(srv),
		}
		if srv.OAuth != nil {
			sc.Issuer = srv.OAuth.Issuer
		}
		c, err := store.Get(connection.Key{Ref: ref, Digest: digest, Server: sc.Server})
		switch {
		case errors.Is(err, connection.ErrNone):
		case err != nil:
			return nil, err
		default:
			sc.State, sc.Broken = stateOf(c)
			sc.Issuer, sc.Label, sc.Client = c.Issuer, c.Label, c.Client.Kind
			sc.Granted = append([]string{}, c.Scopes...)
			sc.CreatedAt, sc.LastUsedAt = c.CreatedAt, c.LastUsedAt
		}
		out.Servers = append(out.Servers, sc)
	}
	return out, nil
}

// ConsentUI is how one consent reaches the person in the app.
type ConsentUI struct {
	// Present is given the vendor's consent address once it is known: the
	// app opens it in the browser and shows it, so it can be opened
	// elsewhere.
	Present func(authURL string)
	// Pasted carries the addresses the person pasted back, where the
	// browser was on another computer and the page it ended on did not
	// load (D21).
	Pasted <-chan string
	// Say tells the person what the consent is doing.
	Say func(string)
}

// NeedsOwnClient is a consent that cannot start until the person keeps an
// OAuth client of their own for the issuer (D16, D23): the vendor lets no
// app register itself (Google). Guide is Agentry's steps for making one.
type NeedsOwnClient struct {
	Issuer string
	Guide  OwnClientGuide
}

func (e *NeedsOwnClient) Error() string {
	return fmt.Sprintf("%s lets no app register itself, so signing in there needs an OAuth client of your own", e.Issuer)
}

// Connect makes the connection of server (a ServerConnection.Server) for
// ref's release: discovery, the client (the person's own where they keep
// one for the issuer, Agentry's CIMD where the vendor takes it, else a
// dynamic registration), the vendor's consent screen, and the code
// exchange. The grant is kept for that release alone, replacing a broken
// or an earlier one (whose refresh token is revoked where the vendor
// offers that). A vendor that needs the person's own client and none is
// kept answers *NeedsOwnClient, with Agentry's steps.
func Connect(ctx context.Context, regURL, ref, server string, ui ConsentUI) (*ServerConnection, error) {
	sig, err := resolveForRun(ctx, regURL, ref, false)
	if err != nil {
		return nil, err
	}
	home, err := Home()
	if err != nil {
		return nil, err
	}
	return connect(ctx, home, sig.Ref, sig.Digest, sig.Manifest, server, ui)
}

func connect(ctx context.Context, home, ref, digest string, m *manifest.Manifest, server string, ui ConsentUI) (*ServerConnection, error) {
	var srv *manifest.MCPServer
	for _, s := range oauthServers(m) {
		if connectionServer(s) == server {
			s := s
			srv = &s
			break
		}
	}
	if srv == nil {
		return nil, fmt.Errorf("%s declares no MCP server %s that signs in by OAuth", ref, server)
	}
	if alt := unsupportedAlternative(*srv); alt != "" {
		return nil, unsupportedRefusal(*srv, alt)
	}
	st := openStores(home)
	store, clients, httpc := st.conns, st.clients, st.httpc
	c := mcpauth.Consent{
		HTTP: httpc, ServerURL: srv.URL, Scopes: connectionScopes(*srv),
		OwnClient: clients.Get, CIMDURL: nativeCIMD,
		Pasted: ui.Pasted, Say: ui.Say, Present: ui.Present,
	}
	if !srv.IsRemote() {
		c.Issuer = srv.OAuth.Issuer
	}
	g, err := c.Run(ctx)
	var own *mcpauth.NeedsOwnClientError
	if errors.As(err, &own) {
		return nil, &NeedsOwnClient{Issuer: own.Issuer, Guide: OwnClientSteps(own.Issuer, *srv)}
	}
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", server, err)
	}
	k := connection.Key{Ref: ref, Digest: digest, Server: server}
	if old, gerr := store.Get(k); gerr == nil && old != nil {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), revokeTimeout)
		outcome, rerr := store.Remove(rctx, k, httpc, clients, "replaced by a new consent")
		cancel()
		if ui.Say != nil {
			if rerr != nil {
				ui.Say("The previous connection could not be removed cleanly (" + rerr.Error() + "); the new one replaces it (revocation: " + outcome + ").")
			} else {
				ui.Say("The previous connection was removed (revocation: " + outcome + ").")
			}
		}
	}
	if _, err := store.Put(k, g, ""); err != nil {
		return nil, err
	}
	rc, err := connectionsOf(store, ref, digest, m)
	if err != nil {
		return nil, err
	}
	for _, s := range rc.Servers {
		if s.Server == server {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("the connection to %s was kept but cannot be read back", server)
}

// runConnections is what a run is given of its connections: each server's
// access token, the receipt's line naming each one used (never a token),
// and the renewal the engine asks for before a token expires (D24).
type runConnections struct {
	tokens map[string]engine.ConnectionToken
	line   string
	renew  func(ctx context.Context, server string) (engine.ConnectionToken, error)
}

// openConnections opens the connection of every server of the release that
// signs in by OAuth and refreshes each under its lock (connection.Store.Use)
// before the sandbox starts. The desktop never asks for consent here: the
// run setup's Connect tools step does, and a server with no working
// connection refuses the run with exit 11, naming it, as the CLI does with
// no terminal. A vendor that cannot be reached is exit 1 and the
// connection is kept.
func openConnections(ctx context.Context, home, ref, digest string, m *manifest.Manifest) (*runConnections, error) {
	servers := oauthServers(m)
	if len(servers) == 0 {
		return &runConnections{}, nil
	}
	st := openStores(home)
	store, clients, httpc := st.conns, st.clients, st.httpc
	out := &runConnections{
		tokens: map[string]engine.ConnectionToken{},
		renew: func(ctx context.Context, server string) (engine.ConnectionToken, error) {
			a, _, err := store.Use(ctx, connection.Key{Ref: ref, Digest: digest, Server: server}, httpc, clients)
			return engine.ConnectionToken{AccessToken: a.Token, ExpiresAt: a.ExpiresAt}, err
		},
	}
	var used []string
	for _, srv := range servers {
		if alt := unsupportedAlternative(srv); alt != "" {
			return nil, unsupportedRefusal(srv, alt)
		}
		k := connection.Key{Ref: ref, Digest: digest, Server: connectionServer(srv)}
		access, c, err := store.Use(ctx, k, httpc, clients)
		var broken *connection.BrokenError
		switch {
		case err == nil:
		case errors.Is(err, connection.ErrNone):
			return nil, engine.Classify(engine.ExitPolicy, fmt.Errorf(
				"declared MCP server %s signs in by OAuth and has no connection for this version: Connect it on the run setup's Connect tools step, then start the run again", k.Server))
		case errors.As(err, &broken):
			return nil, engine.Classify(engine.ExitPolicy, fmt.Errorf(
				"declared MCP server %s signs in by OAuth and its connection no longer works (%s): Connect it again on the run setup's Connect tools step", k.Server, broken.Reason))
		default:
			return nil, engine.Classify(engine.ExitCLI, err)
		}
		out.tokens[k.Server] = engine.ConnectionToken{AccessToken: access.Token, ExpiresAt: access.ExpiresAt}
		used = append(used, fmt.Sprintf("%s (issuer %s, scopes %s, client %s)", k.Server, c.Issuer, scopeList(c.Scopes), c.Client.Kind))
	}
	out.line = "servers signed in to by the person's connection for this release, refreshed by the runner before the run, and renewed before a token expires during the run where the server reads it from a file (a renewal that fails is said on the run's stderr); the run was given an access token alone: " + strings.Join(used, "; ")
	return out, nil
}

func scopeList(s []string) string {
	if len(s) == 0 {
		return "the vendor's default"
	}
	return strings.Join(s, " ")
}

// KeptConnection is one kept connection, as Settings → Connections lists
// it: never a token.
type KeptConnection struct {
	Ref        string   `json:"ref"`
	Digest     string   `json:"digest"`
	Server     string   `json:"server"`
	Issuer     string   `json:"issuer"`
	Client     string   `json:"client"`
	Scopes     []string `json:"scopes"`
	State      string   `json:"state"`
	Broken     string   `json:"broken,omitempty"`
	Label      string   `json:"label,omitempty"`
	CreatedAt  string   `json:"createdAt"`
	LastUsedAt string   `json:"lastUsedAt,omitempty"`
	// KeptIn is where its secret half is: "keychain" or "file".
	KeptIn string `json:"keptIn"`
}

// KeptClient is one of the person's own OAuth clients: never its secret.
type KeptClient struct {
	Issuer   string `json:"issuer"`
	ClientID string `json:"clientId"`
	// SecretIn is where its secret is ("keychain", "file"), "" for none.
	SecretIn string `json:"secretIn,omitempty"`
}

// KeptConnections lists every kept connection and own client of this
// machine's runner home, the CLI's included.
func KeptConnections() ([]KeptConnection, []KeptClient, error) {
	home, err := Home()
	if err != nil {
		return nil, nil, err
	}
	list, err := connection.Open(home, nil).List()
	if err != nil {
		return nil, nil, err
	}
	conns := []KeptConnection{}
	for _, c := range list {
		k := KeptConnection{
			Ref: c.Ref, Digest: c.Digest, Server: c.Server, Issuer: c.Issuer, Client: c.Client.Kind,
			Scopes: append([]string{}, c.Scopes...), Label: c.Label,
			CreatedAt: c.CreatedAt, LastUsedAt: c.LastUsedAt, KeptIn: c.SecretIn,
		}
		k.State, k.Broken = stateOf(c)
		conns = append(conns, k)
	}
	cs, err := connection.OpenClients(home, nil).List()
	if err != nil {
		return nil, nil, err
	}
	clients := []KeptClient{}
	for _, c := range cs {
		clients = append(clients, KeptClient{Issuer: c.Issuer, ClientID: c.ClientID, SecretIn: c.SecretIn})
	}
	return conns, clients, nil
}

// Disconnect removes one kept connection, first revoking its refresh token
// at the vendor where it offers that (D17, D20), and returns the
// revocation's outcome, which the store's audit log also records.
func Disconnect(ctx context.Context, ref, digest, server string) (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	st := openStores(home)
	rctx, cancel := context.WithTimeout(ctx, revokeTimeout)
	defer cancel()
	return st.conns.Remove(rctx, connection.Key{Ref: ref, Digest: digest, Server: server}, st.httpc, st.clients, "removed by the person")
}

// RemoveConnectionsOf removes the connections of every version of ns/name,
// revoking each where the vendor offers that: an uninstalled QuickAgent's
// connections go with it (D17). It returns one line per removal.
func RemoveConnectionsOf(ctx context.Context, ns, name string) ([]string, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	st := openStores(home)
	store, clients, httpc := st.conns, st.clients, st.httpc
	list, err := store.List()
	if err != nil {
		return nil, err
	}
	var out []string
	var errs []error
	for _, c := range list {
		if !profile.RefMatches(c.Ref, ns+"/"+name) {
			continue
		}
		rctx, cancel := context.WithTimeout(ctx, revokeTimeout)
		outcome, rerr := store.Remove(rctx, c.Key(), httpc, clients, "the QuickAgent was uninstalled")
		cancel()
		if rerr != nil {
			errs = append(errs, rerr)
			continue
		}
		out = append(out, fmt.Sprintf("removed the connection of %s to %s (revocation: %s)", c.Ref, c.Server, outcome))
	}
	return out, errors.Join(errs...)
}

// SetOwnClient keeps the person's own OAuth client for issuer, used by
// every later consent there (D23); its secret goes to the OS keychain, a
// 0600 file where none answers, and is never read back to the page.
func SetOwnClient(issuer, clientID, secret string) error {
	home, err := Home()
	if err != nil {
		return err
	}
	return connection.OpenClients(home, connectionKeys()).Set(issuer, clientID, strings.TrimSpace(secret))
}

// ForgetOwnClient forgets the person's OAuth client for issuer; the
// connections made with it then ask again. False when none was kept.
func ForgetOwnClient(issuer string) (bool, error) {
	home, err := Home()
	if err != nil {
		return false, err
	}
	return connection.OpenClients(home, connectionKeys()).Remove(issuer)
}

// GoogleIssuer is Google's authorization server, the first vendor Agentry
// writes its own steps for (D22).
const GoogleIssuer = "https://accounts.google.com"

// OwnClientDocs is the docs page that carries Agentry's Google steps.
const OwnClientDocs = "https://docs.secureagentry.net/agentry/google-client/"

// OwnClientGuide is how to make one's own OAuth client at a vendor (D22):
// Agentry's steps where it has written them (Known), else the general
// shape of one, which a publisher's help may follow.
type OwnClientGuide struct {
	Issuer string `json:"issuer"`
	// Vendor names the vendor where Agentry knows it ("Google").
	Vendor string `json:"vendor,omitempty"`
	// Known says the steps are Agentry's own for this vendor.
	Known bool     `json:"known"`
	Steps []string `json:"steps"`
	// Note follows the steps; Docs is the page that carries them.
	Note string `json:"note,omitempty"`
	Docs string `json:"docs,omitempty"`
}

// googleProducts names, for each of Google's remote MCP servers, the API
// and the MCP API its project enables.
var googleProducts = map[string]string{
	"gmailmcp.googleapis.com":    "the Gmail API and the Gmail MCP API (gmailmcp.googleapis.com)",
	"drivemcp.googleapis.com":    "the Google Drive API and the Drive MCP API (drivemcp.googleapis.com)",
	"calendarmcp.googleapis.com": "the Google Calendar API and the Calendar MCP API (calendarmcp.googleapis.com)",
}

// OwnClientSteps are Agentry's steps for making one's own client for a
// run on this computer: a Desktop-type client, whose loopback redirect
// needs no address entered. Google's are Agentry's (D22), the Test users
// step included (agentry-cli#665's live check); any other vendor gets the
// general shape.
func OwnClientSteps(issuer string, srv manifest.MCPServer) OwnClientGuide {
	if strings.TrimRight(issuer, "/") == GoogleIssuer {
		steps := []string{
			"In the Google Cloud console (console.cloud.google.com), create a project or pick one of yours.",
			"APIs & Services → Library: enable " + googleAPIs(srv) + ".",
		}
		if googleRemote(srv) {
			// Google's remote MCP servers answer a tool call only for a
			// project enrolled in its Workspace Developer Preview (the
			// operator's decision of 2026-10-06: ship the steps and say it).
			steps = append(steps, "Google's remote MCP servers are in Google's Workspace Developer Preview: enrol this project at developers.google.com/workspace/preview. Google asks for a Google Workspace account and can take a couple of days. Until the project is enrolled, every tool call answers \"The caller does not have permission\". A QuickAgent that uses the workspace-mcp server instead reaches Gmail through the Gmail API and needs no enrolment.")
		}
		return OwnClientGuide{
			Issuer: GoogleIssuer, Vendor: "Google", Known: true,
			Steps: append(steps,
				"Google Auth Platform → Branding: give the app a name and your email address.",
				"Audience: choose Internal if your account is in a Google Workspace organization; otherwise External, then Publish app so it is In production. A client left in Testing gets sign-ins that stop working after 7 days.",
				"If the app stays in Testing: Audience → Test users → Add users, and add the Google account you will sign in with. Google refuses the sign-in of any account a Testing app does not list.",
				"Data Access → Add or remove scopes: add the scopes this QuickAgent asks for ("+scopeList(connectionScopes(srv))+").",
				"Clients → Create client → Application type: Desktop app. Copy its client ID and client secret into the fields below.",
			),
			Note: "Google may show an \"unverified app\" screen when you sign in with a client of your own: the app is yours, and Google says so because it has not reviewed it. Every QuickAgent you connect to Google from this computer uses the client you keep here; each version still asks for its own consent.",
			Docs: OwnClientDocs,
		}
	}
	return OwnClientGuide{
		Issuer: strings.TrimRight(issuer, "/"),
		Steps: []string{
			"In the vendor's developer settings, register an OAuth app or client of the native or desktop kind.",
			"Give it the redirect address http://127.0.0.1/callback (any port), the authorization code grant and refresh tokens.",
			"Give it the scopes this QuickAgent asks for (" + scopeList(connectionScopes(srv)) + ").",
			"Copy its client ID and, if it has one, its client secret into the fields below.",
		},
		Note: "Agentry has not written steps for this vendor. Every QuickAgent you connect there from this computer uses the client you keep here.",
	}
}

// googleRemote says srv is one of Google's remote MCP servers.
func googleRemote(srv manifest.MCPServer) bool {
	if !srv.IsRemote() {
		return false
	}
	u, err := url.Parse(srv.URL)
	return err == nil && strings.HasSuffix(u.Hostname(), "mcp.googleapis.com")
}

// googleAPIs is what a Google project enables for srv: the product API and
// MCP API of one of Google's remote servers, else the APIs of the hosts a
// package server reaches.
func googleAPIs(srv manifest.MCPServer) string {
	if srv.IsRemote() {
		if u, err := url.Parse(srv.URL); err == nil {
			if p, ok := googleProducts[u.Hostname()]; ok {
				return p
			}
			return "the API of the product " + u.Hostname() + " serves, and its MCP API"
		}
	}
	var hosts []string
	for _, h := range srv.Egress {
		if strings.HasSuffix(h, ".googleapis.com") {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return "the API of each Google product this QuickAgent reaches"
	}
	return "the API of each Google product this QuickAgent reaches (" + strings.Join(hosts, ", ") + "; for gmail.googleapis.com, the Gmail API)"
}
