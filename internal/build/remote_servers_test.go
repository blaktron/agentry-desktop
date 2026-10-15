package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

// A remote MCP server and a package server's OAuth sign-in. The Build
// page's step check runs the core's own V5 over what the Model writes, so
// these tests pin the Model's half: every field is written and read back,
// only the fields of a row's own shape reach agent.yaml, and the summary
// says how each server signs in in the site's words.

func linearRow() MCPServerRow {
	return MCPServerRow{Source: "url", URL: "https://mcp.linear.app/mcp", Auth: "oauth", Scopes: []string{"read", "write"},
		Tools: []string{"list_issues"}, Secrets: []string{}, Egress: []string{}, Env: []EnvRow{}}
}

func workspaceRow() MCPServerRow {
	return MCPServerRow{Source: "pypi", Package: "workspace-mcp", Version: "2.0.1", Tools: []string{}, Secrets: []string{},
		Egress: []string{"gmail.googleapis.com"}, Env: []EnvRow{{Name: "MCP_SINGLE_USER_MODE", Value: "1"}},
		OAuth: &OAuthRow{
			Issuer: "https://accounts.google.com",
			Scopes: []string{"https://www.googleapis.com/auth/gmail.readonly"},
			Files:  []TokenFileRow{{Path: "me@agentry.invalid.json", Template: `{"token": "{access_token}", "expiry": "{expiry}", "client_id": "agentry", "client_secret": "agentry"}`}},
			Env:    []EnvRow{{Name: "WORKSPACE_MCP_CREDENTIALS_DIR", Value: "{dir}"}},
		}}
}

func withServers(rows ...MCPServerRow) *Model {
	m := model()
	m.MCPServers = rows
	return m
}

func TestRemoteServersAndSignInsRoundTripAndPassTheCore(t *testing.T) {
	gh := MCPServerRow{Source: "url", URL: "https://api.githubcopilot.com/mcp/", Auth: "secret", Scopes: []string{},
		Tools: []string{}, Secrets: []string{"GITHUB_TOKEN"}, Egress: []string{}, Env: []EnvRow{}}
	none := MCPServerRow{Source: "url", URL: "https://mcp.deepwiki.com/mcp", Auth: "none", Scopes: []string{},
		Tools: []string{}, Secrets: []string{}, Egress: []string{}, Env: []EnvRow{}}
	m := withServers(workspaceRow(), linearRow(), gh, none)
	if res := m.Check(); res.First != nil {
		t.Fatalf("the four shapes pass the form and the core: %+v\n%s", res.First, res.YAML)
	}
	dir := t.TempDir()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	parsed, err := manifest.Parse(raw)
	if err != nil {
		t.Fatalf("does not parse: %v\n%s", err, raw)
	}
	if errs := manifest.Blocking(manifest.Validate(parsed)); len(errs) > 0 {
		t.Fatalf("V-rules: %v\n%s", errs, raw)
	}
	srv := parsed.Capabilities.MCPServers
	if len(srv) != 4 || srv[0].OAuth == nil || srv[0].OAuth.Issuer != "https://accounts.google.com" || len(srv[0].OAuth.Files) != 1 ||
		srv[0].OAuth.Env["WORKSPACE_MCP_CREDENTIALS_DIR"] != "{dir}" || srv[1].URL != "https://mcp.linear.app/mcp" ||
		srv[1].Auth != manifest.AuthOAuth || !reflect.DeepEqual(srv[1].Scopes, []string{"read", "write"}) ||
		srv[2].Auth != manifest.AuthSecret || !reflect.DeepEqual(srv[2].Secrets, []string{"GITHUB_TOKEN"}) || srv[3].Auth != manifest.AuthNone {
		t.Fatalf("the core reads the servers as written:\n%s", raw)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.MCPServers, m.MCPServers) {
		t.Errorf("round trip:\nwant %+v\n got %+v", m.MCPServers, got.MCPServers)
	}
}

// The reader takes a remote server for a remote server, not an empty npm
// package: opening a workspace that declared one and saving it keeps it.
func TestLoadingAWorkspaceKeepsItsRemoteServer(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, withServers(linearRow())); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, got); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "agent.yaml"))
	if !strings.Contains(string(raw), "- url: https://mcp.linear.app/mcp") || strings.Contains(string(raw), `npm: ""`) {
		t.Errorf("a loaded remote server is saved as itself:\n%s", raw)
	}
}

// A row keeps what its other shape holds, so switching the source back
// loses nothing; only its own shape's fields are written, and a key or
// scopes its auth does not use are not either.
func TestOnlyARowsOwnShapeIsWritten(t *testing.T) {
	remote := linearRow()
	remote.Package, remote.Version, remote.Egress = "left-behind", "9.9.9", []string{"stale.example"}
	remote.Env = []EnvRow{{Name: "STALE", Value: "1"}}
	remote.OAuth = &OAuthRow{Issuer: "https://stale.example"}
	remote.Secrets = []string{"NOT_SENT"} // auth oauth sends no key
	pkg := workspaceRow()
	pkg.URL, pkg.Auth, pkg.Scopes = "https://stale.example/mcp", "secret", []string{"stale"}
	m := withServers(remote, pkg)
	res := m.Check()
	if res.First != nil {
		t.Fatalf("hidden fields hold nothing: %+v\n%s", res.First, res.YAML)
	}
	for _, stale := range []string{"left-behind", "9.9.9", "stale.example", "STALE", "NOT_SENT", "auth: secret"} {
		if strings.Contains(res.YAML, stale) {
			t.Errorf("agent.yaml carries %q, which the row's shape hides:\n%s", stale, res.YAML)
		}
	}
	for _, k := range m.declaredSecrets() {
		if k == "NOT_SENT" {
			t.Errorf("a key a remote server does not send is not a declared key")
		}
	}

	// The same key with auth secret is sent, and declared.
	gh := linearRow()
	gh.Auth, gh.Secrets = "secret", []string{"GITHUB_TOKEN"}
	m = withServers(gh)
	if res := m.Check(); res.First != nil || !strings.Contains(res.YAML, "secrets: [GITHUB_TOKEN]") || strings.Contains(res.YAML, "scopes:") {
		t.Errorf("auth secret writes its key and no scopes: %+v\n%s", res.First, res.YAML)
	}
	if got := m.OptionalSecrets(); !reflect.DeepEqual(got, []string{"EXAMPLE_TOKEN", "GITHUB_TOKEN"}) {
		t.Errorf("a remote server's key starts Optional like any attached key: %v", got)
	}
}

func TestTheStepCheckHoldsRemoteServersAndSignInsToTheCore(t *testing.T) {
	cases := map[string]struct {
		row  MCPServerRow
		want string
	}{
		"no address":         {MCPServerRow{Source: "url", Auth: "oauth"}, "give the remote server's https:// address"},
		"no auth":            {MCPServerRow{Source: "url", URL: "https://mcp.linear.app/mcp"}, "say how it signs in"},
		"http":               {MCPServerRow{Source: "url", URL: "http://mcp.linear.app/mcp", Auth: "oauth"}, "must be https://"},
		"an IP":              {MCPServerRow{Source: "url", URL: "https://127.0.0.1/mcp", Auth: "none"}, "not an IP address"},
		"a key with no name": {MCPServerRow{Source: "url", URL: "https://mcp.example.com/mcp", Auth: "secret"}, "exactly one secret"},
		"no issuer": {func() MCPServerRow {
			r := workspaceRow()
			r.OAuth.Issuer = ""
			return r
		}(), "issuer is required"},
		"no token reaches it": {func() MCPServerRow {
			r := workspaceRow()
			r.OAuth.Files = nil
			r.OAuth.Env = []EnvRow{{Name: "X", Value: "plain"}}
			return r
		}(), "{access_token} must appear"},
		"an unknown placeholder": {func() MCPServerRow {
			r := workspaceRow()
			r.OAuth.Files[0].Template = "{refresh_token}"
			return r
		}(), "unknown placeholder {refresh_token}"},
		"the unsupported list": {func() MCPServerRow {
			r := workspaceRow()
			r.Package = "mcp-gsuite"
			return r
		}(), "declare pypi:workspace-mcp instead"},
	}
	for name, c := range cases {
		res := withServers(c.row).Check()
		if !strings.Contains(res.Problems["tools"], c.want) || res.First == nil || res.First.Section != SectionTools {
			t.Errorf("%s: want a Tools problem naming %q, got %q (first %+v)", name, c.want, res.Problems["tools"], res.First)
		}
	}
	two := withServers(linearRow(), linearRow()).Check()
	if !strings.Contains(two.Problems["tools"], "declared twice") {
		t.Errorf("a remote server declared twice is refused: %q", two.Problems["tools"])
	}
}

// Every value is quoted, auth and a token setting's name too: the step
// check renders a model before its rules pass.
func TestHostileSignInValuesCannotChangeTheShape(t *testing.T) {
	r := workspaceRow()
	r.OAuth.Files[0].Template = "{access_token}\"\n  injected: true\n#"
	r.OAuth.Env = []EnvRow{{Name: "A: b\n  injected", Value: "{access_token}"}}
	r.OAuth.Issuer = "https://a.example\n  injected: true"
	r.OAuth.Scopes = []string{"a], injected: [x", "? k"}
	r.OAuth.Files = append(r.OAuth.Files, TokenFileRow{Path: "b.json\n  injected: true", Template: "{access_token}"})
	remote := linearRow()
	remote.Auth = "oauth\n  injected: true"
	remote.URL = "https://x.example/\"\n  injected: true"
	remote.Scopes = []string{"read]\n  injected: true"}
	res := withServers(r, remote).Check()
	for _, line := range strings.Split(res.YAML, "\n") {
		if strings.TrimSpace(line) == "injected: true" || strings.HasPrefix(strings.TrimSpace(line), "injected") {
			t.Fatalf("a typed value became YAML structure:\n%s", res.YAML)
		}
	}
	if res.First == nil {
		t.Errorf("the hostile values are refused")
	}
}

func TestTheSummarySaysHowEachServerSignsIn(t *testing.T) {
	gh := linearRow()
	gh.URL, gh.Auth, gh.Secrets = "https://api.githubcopilot.com/mcp/", "secret", []string{"GITHUB_TOKEN"}
	notion := linearRow()
	notion.URL, notion.Scopes = "https://mcp.notion.com/mcp", nil
	m := withServers(linearRow(), gh, notion, workspaceRow())
	m.Secrets = nil
	s := m.Summarize([]string{"mcp.linear.app"})
	if len(s.Servers) != 4 {
		t.Fatalf("four servers, got %+v", s.Servers)
	}
	lin, ghs, not, ws := s.Servers[0], s.Servers[1], s.Servers[2], s.Servers[3]
	vendor := "Runs at its vendor. Agentry checks its address and what it asks for, not the vendor's code."
	if lin.Ref != "https://mcp.linear.app/mcp" || !lin.Remote || len(lin.Hosts) != 1 || lin.Hosts[0] != (SummaryHost{Host: "mcp.linear.app", Unnamed: true}) ||
		!reflect.DeepEqual(lin.SignIn, []string{vendor, "Signs in with OAuth: the first run asks whoever runs it to sign in at the vendor and allow these scopes: read, write. The run gets an access token, nothing else."}) {
		t.Errorf("a remote server signed in to by OAuth: %+v", lin)
	}
	if !s.AnyUnnamed {
		t.Errorf("a remote server's host the description did not name is marked")
	}
	if !reflect.DeepEqual(ghs.SignIn, []string{vendor, "Signs in with the key GITHUB_TOKEN, sent to the vendor as a bearer token."}) ||
		!reflect.DeepEqual(ghs.Keys, []string{"GITHUB_TOKEN"}) || len(s.Keys) != 1 || s.Keys[0].Name != "GITHUB_TOKEN" ||
		!reflect.DeepEqual(s.Keys[0].UsedBy, []string{"https://api.githubcopilot.com/mcp/"}) {
		t.Errorf("a remote server's key is a key, listed as that server's: %+v %+v", ghs, s.Keys)
	}
	if !strings.Contains(not.SignIn[1], "allow the scopes the vendor asks for.") {
		t.Errorf("OAuth with no scopes: %+v", not.SignIn)
	}
	want := []string{
		"Signs in with OAuth at https://accounts.google.com: the first run asks whoever runs it to allow these scopes: https://www.googleapis.com/auth/gmail.readonly. The server gets an access token, never a refresh token.",
		`Token file me@agentry.invalid.json: {"token": "{access_token}", "expiry": "{expiry}", "client_id": "agentry", "client_secret": "agentry"}`,
		"Token settings: WORKSPACE_MCP_CREDENTIALS_DIR={dir}.",
	}
	if ws.Remote || !reflect.DeepEqual(ws.SignIn, want) || ws.Ref != "pypi:workspace-mcp@2.0.1" {
		t.Errorf("a package server's sign-in:\nwant %q\n got %q", want, ws.SignIn)
	}
	b, _ := json.Marshal(withServers(model().MCPServers...).Summarize(nil))
	if strings.Contains(string(b), `"signIn":null`) {
		t.Errorf("a server with no sign-in has an empty list, not null: %s", b)
	}
}

func TestADraftsRemoteServerAndSignInComeIntoTheForm(t *testing.T) {
	doc := map[string]any{
		"schemaVersion": "2", "name": "inbox", "namespace": "drafter", "version": "0.1.0", "summary": "Reads the inbox",
		"modes": []string{"autonomous"}, "entrypoints": map[string]any{"instruction": "AGENT.md"},
		"capabilities": map[string]any{
			"network": map[string]any{"egress": "none"},
			"mcpServers": []any{
				map[string]any{"url": "https://mcp.linear.app/mcp", "auth": "oauth", "scopes": []string{"read"}},
				map[string]any{"pypi": "workspace-mcp", "version": "2.0.1", "egress": []string{"gmail.googleapis.com"},
					"oauth": map[string]any{"issuer": "https://accounts.google.com", "files": []any{map[string]any{"path": "a.json", "template": "{access_token}"}}}},
			},
		},
	}
	raw, _ := json.Marshal(doc)
	m, notes, err := FromDraft(raw, "# inbox\n", "alice")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if strings.Contains(n, "cannot hold") {
			t.Errorf("the form holds a remote server and a sign-in: %q", n)
		}
	}
	if r := m.MCPServers[0]; !r.IsRemote() || r.URL != "https://mcp.linear.app/mcp" || r.Auth != "oauth" || !reflect.DeepEqual(r.Scopes, []string{"read"}) {
		t.Errorf("the remote server: %+v", r)
	}
	if o := m.MCPServers[1].OAuth; o == nil || o.Issuer != "https://accounts.google.com" || len(o.Files) != 1 || o.Files[0].Template != "{access_token}" {
		t.Errorf("the sign-in: %+v", o)
	}
}

func TestARemoteServerIsNotLookedUpAtTheRegistry(t *testing.T) {
	got := withServers(linearRow(), workspaceRow()).DeclaredServers()
	if len(got) != 1 || got[0].PyPI != "workspace-mcp" {
		t.Errorf("only the package is asked about: %+v", got)
	}
}

// The reader refuses a server entry the form could not hold whole, as the
// site's importer does, rather than letting the next save narrow it.
func TestLoadRefusesAServerTheFormCannotHoldWhole(t *testing.T) {
	head := "schemaVersion: \"2\"\nname: x\nnamespace: acme\nversion: 0.1.0\nsummary: s\nmodes: [autonomous]\ncapabilities:\n  mcpServers:\n"
	cases := map[string]string{
		"a remote server with a version": "    - url: https://mcp.linear.app/mcp\n      auth: oauth\n      version: 1.0.0\n",
		"a remote server with hosts":     "    - url: https://mcp.linear.app/mcp\n      auth: oauth\n      egress: [x.example]\n",
		"a package with auth":            "    - npm: x\n      version: 1.0.0\n      auth: oauth\n",
		"a key under OAuth":              "    - url: https://mcp.linear.app/mcp\n      auth: oauth\n      secrets: [TOKEN]\n",
		"scopes under a key":             "    - url: https://api.githubcopilot.com/mcp/\n      auth: secret\n      secrets: [TOKEN]\n      scopes: [repo]\n",
		"an address beside a package":    "    - npm: x\n      url: https://mcp.linear.app/mcp\n",
		"an auth outside the three":      "    - url: https://mcp.linear.app/mcp\n      auth: basic\n",
	}
	for name, entry := range cases {
		if _, err := decodeManifest([]byte(head + entry)); err == nil || !strings.Contains(err.Error(), "capabilities.mcpServers[0]") {
			t.Errorf("%s: want a refusal naming the entry, got %v", name, err)
		}
	}
	m, err := decodeManifest([]byte(head + "    - url: https://mcp.linear.app/mcp\n      auth: \" oauth \"\n"))
	if err != nil || m.MCPServers[0].Auth != "oauth" {
		t.Errorf("a hand-written auth is read trimmed, as the page's select compares it: %v %+v", err, m)
	}
}
