package build

import (
	"fmt"
	"strings"

	"github.com/blaktron/agentry-desktop/internal/mcpview"
	"github.com/blaktron/agentry/pkg/manifest"
)

// Summary is the Build tab's summary, the one short page a QuickAgent is
// submitted from, whether the Drafter wrote it or the person did in the
// form: how it works, then what it needs. The plumbing (the Drafter's
// notes, sources and AI Review's counts, the namespace, version, licence,
// tags, include list, file name and run mode) stays in the form views.
//
// The capability contract is shown in full before Submit: every key,
// website, MCP server with its tools, hosts and configuration, and file,
// command or data grant the model declares is here, and nothing it does not
// declare. The run modes and the intake questions stay in the form: neither
// widens what a run may touch (a secret question is carried by a declared
// key, which is listed). It mirrors the site's summary,
// the knowledge lines included: the packs it ships and the one it
// asks a person for, one line each, only those declared.
type Summary struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	// Overview is the guide's overview (Markdown), or "".
	Overview string       `json:"overview"`
	Keys     []SummaryKey `json:"keys"`
	// Websites is the QuickAgent's own web access; "*" is any host.
	Websites []SummaryHost   `json:"websites"`
	Servers  []SummaryServer `json:"servers"`
	// Grants is each file, command or data grant, one line each, only
	// those declared.
	Grants []string `json:"grants"`
	// Knowledge is the packs it ships and the one it asks a person for, one
	// line each; only those declared.
	Knowledge []string `json:"knowledge"`
	// AnyUnnamed says whether anything it reaches is a host the draft's
	// description did not name.
	AnyUnnamed bool `json:"anyUnnamed"`
}

// SummaryKey is one key the QuickAgent or a server of it needs.
type SummaryKey struct {
	Name string `json:"name"`
	// Help is the guide's help for it (Markdown), or "".
	Help string `json:"help"`
	// UsedBy is who asks for it: "the QuickAgent" for its own, and each
	// server's reference.
	UsedBy []string `json:"usedBy"`
	// Optional says a run can start without it (`optionalSecrets`).
	Optional bool `json:"optional"`
}

// SummaryHost is a host the QuickAgent or a server reaches.
type SummaryHost struct {
	Host string `json:"host"`
	// Unnamed says the description and hints of the draft it came from do
	// not name it.
	Unnamed bool `json:"unnamed"`
}

// SummaryServer is one declared MCP server.
type SummaryServer struct {
	Ref string `json:"ref"`
	// Tools are the tools it may call; empty is all of them, as the
	// manifest reads it.
	Tools []string      `json:"tools"`
	Hosts []SummaryHost `json:"hosts"`
	Keys  []string      `json:"keys"`
	// Args are the arguments a package server's command starts with, in
	// order; empty for none.
	Args []string `json:"args"`
	// Config is the names of its plain configuration (env); the values
	// are in the form.
	Config []string `json:"config"`
	// Remote is a server run at its vendor, declared by its
	// address.
	Remote bool `json:"remote"`
	// SignIn says how it signs in, in the site's sentences: a remote server's
	// auth and scopes; a package's issuer, scopes, each token file with its
	// template, and its token settings. Empty for none.
	SignIn []string `json:"signIn"`
}

// issuerNotGiven stands in for a sign-in's issuer before it is given.
const issuerNotGiven = "an issuer not given yet"

// SignInLines says how a server signs in: what the first run asks of whoever
// runs it, and that a run gets an access token and nothing else. It never
// vouches for a vendor: of a remote server it says what Agentry checks and
// what it cannot.
// A token file's template and a token setting's value are shown whole,
// unlike configuration's names: what is written beside the token is
// contract. The words are the site's.
func (r MCPServerRow) SignInLines() []string {
	d := r.Declared()
	allow := func(scopes []string) string {
		if len(scopes) == 0 {
			return "the scopes the vendor asks for"
		}
		return "these scopes: " + strings.Join(scopes, ", ")
	}
	if d.IsRemote() {
		out := []string{"Runs at its vendor. Agentry checks its address and what it asks for, not the vendor's code."}
		switch manifest.MCPAuth(d.Auth) {
		case manifest.AuthOAuth:
			out = append(out, "Signs in with OAuth: the first run asks whoever runs it to sign in at the vendor and allow "+allow(d.Scopes)+". The run gets an access token, nothing else.")
		case manifest.AuthSecret:
			out = append(out, "Signs in with the key "+strings.Join(d.Secrets, ", ")+", sent to the vendor as a bearer token.")
		case manifest.AuthNone:
			out = append(out, "Signs in with nothing.")
		}
		return out
	}
	o := d.OAuth
	if o == nil {
		return []string{}
	}
	issuer := strings.TrimSpace(o.Issuer)
	if issuer == "" {
		issuer = issuerNotGiven
	}
	out := []string{"Signs in with OAuth at " + issuer + ": the first run asks whoever runs it to allow " + allow(compact(o.Scopes)) + ". The server gets an access token, never a refresh token."}
	for _, f := range o.Files {
		out = append(out, "Token file "+strings.TrimSpace(f.Path)+": "+f.Template)
	}
	var settings []string
	for _, e := range o.Env {
		settings = append(settings, strings.TrimSpace(e.Name)+"="+e.Value)
	}
	if len(settings) > 0 {
		out = append(out, "Token settings: "+strings.Join(settings, ", ")+".")
	}
	return out
}

// Summarize reads the model, and the hosts a draft's description did not
// name (draft.UnmentionedHosts; none for a QuickAgent written in the form),
// into its summary. It reads and writes nothing.
func (m *Model) Summarize(unmentioned []string) *Summary {
	marked := map[string]bool{}
	for _, h := range unmentioned {
		marked[strings.ToLower(strings.TrimSpace(h))] = true
	}
	s := &Summary{Name: m.Name, Summary: m.Summary, Overview: m.Guide.Overview,
		Keys: []SummaryKey{}, Websites: []SummaryHost{}, Servers: []SummaryServer{}, Grants: []string{}, Knowledge: []string{}}
	host := func(h string) SummaryHost {
		sh := SummaryHost{Host: h, Unnamed: marked[strings.ToLower(h)]}
		s.AnyUnnamed = s.AnyUnnamed || sh.Unnamed
		return sh
	}

	switch m.EgressMode {
	case "all":
		s.Websites = append(s.Websites, host("*"))
	case "hosts":
		for _, h := range compact(m.EgressHosts) {
			s.Websites = append(s.Websites, host(h))
		}
	}

	// Each key once, in the order the manifest declares them: the
	// QuickAgent's own, then each server's.
	keyAt := map[string]int{}
	optional := map[string]bool{}
	for _, k := range m.OptionalSecrets() {
		optional[k] = true
	}
	need := func(name, by string) {
		i, ok := keyAt[name]
		if !ok {
			i = len(s.Keys)
			keyAt[name] = i
			s.Keys = append(s.Keys, SummaryKey{Name: name, Help: m.keyHelp(name), UsedBy: []string{}, Optional: optional[name]})
		}
		if u := s.Keys[i].UsedBy; len(u) == 0 || u[len(u)-1] != by {
			s.Keys[i].UsedBy = append(u, by)
		}
	}
	for _, k := range compact(m.Secrets) {
		need(k, "the QuickAgent")
	}
	for _, row := range m.MCPServers {
		srv := row.Declared()
		ref := srv.Ref()
		sv := SummaryServer{Ref: ref, Tools: compact(srv.Tools), Hosts: []SummaryHost{}, Keys: compact(srv.Secrets), Args: orEmpty(srv.Args), Config: []string{},
			Remote: srv.IsRemote(), SignIn: srv.SignInLines()}
		for _, h := range srv.Hosts() {
			sv.Hosts = append(sv.Hosts, host(h))
		}
		for _, e := range srv.Env {
			if n := strings.TrimSpace(e.Name); n != "" {
				sv.Config = append(sv.Config, n)
			}
		}
		for _, k := range sv.Keys {
			need(k, ref)
		}
		s.Servers = append(s.Servers, sv)
	}

	fs := func(mode string, paths []string, verb string) {
		switch p := compact(paths); {
		case mode == "workspace":
			s.Grants = append(s.Grants, verb+" the files in its workspace.")
		case mode == "paths" && len(p) > 0:
			s.Grants = append(s.Grants, verb+" "+strings.Join(p, ", ")+" in its workspace.")
		}
	}
	fs(m.FSRead, m.FSReadPaths, "Reads")
	fs(m.FSWrite, m.FSWritePaths, "Writes")
	if cmds := compact(m.ExecCommands); len(cmds) > 0 {
		word := "commands"
		if len(cmds) == 1 {
			word = "command"
		}
		s.Grants = append(s.Grants, fmt.Sprintf("Runs the %s %s.", word, strings.Join(cmds, ", ")))
	}
	if data := compact(m.DataAccess); len(data) > 0 {
		s.Grants = append(s.Grants, "Touches these kinds of data: "+strings.Join(data, ", ")+".")
	}
	// The site's words for a target host, and the
	// install review's for where a browser may go.
	if ids := compact(m.TargetHosts); len(ids) > 0 {
		which := "a server (" + ids[0] + ")"
		if len(ids) > 1 {
			which = fmt.Sprintf("%d servers (%s)", len(ids), strings.Join(ids, ", "))
		}
		s.Grants = append(s.Grants, "Asks for "+which+" when it runs, and can run any command there as the account the person signs in with, on their own computer only.")
	}
	if b := m.Browser; b != nil {
		s.Grants = append(s.Grants, "Opens these in a headless browser it drives: "+mcpview.BrowserReach(b.manifest())+".")
	}
	// The site's words, with each pack's size where the form knows it, and
	// the confirmation Submit will ask.
	if ships := compact(m.KnowledgeShips); len(ships) > 0 {
		word, them := "packs", "them"
		if len(ships) == 1 {
			word, them = "pack", "it"
		}
		named := make([]string, 0, len(ships))
		for _, n := range ships {
			if src, ok := m.shipSource(n); ok && src.Bytes > 0 {
				n += " (" + PackSize(src.Bytes) + ")"
			}
			named = append(named, n)
		}
		s.Knowledge = append(s.Knowledge, fmt.Sprintf("Ships the knowledge %s %s in the release, unencrypted: anyone who can download it can read %s.", word, strings.Join(named, ", "), them))
		s.Knowledge = append(s.Knowledge, "Submit asks you to confirm, for each pack, that it holds no private data.")
	}
	if strings.TrimSpace(m.KnowledgePrompt) != "" {
		line := "Asks a person to attach a knowledge pack of their own"
		if m.KnowledgeRequired {
			line += ", and cannot run without one"
		}
		s.Knowledge = append(s.Knowledge, line+".")
	}
	return s
}

// Ref is a server's reference as the page and the registry's check name
// it: source:package@version, or a remote server's address.
func (r MCPServerRow) Ref() string {
	if r.IsRemote() {
		return strings.TrimSpace(r.URL)
	}
	return r.Source + ":" + strings.TrimSpace(r.Package) + "@" + strings.TrimSpace(r.Version)
}

// keyHelp is the guide's help for a key: the first row that names it with
// help, as the run setup reads guide.connect.
func (m *Model) keyHelp(name string) string {
	for _, c := range m.Guide.Connect {
		if strings.TrimSpace(c.Secret) == name {
			if h := strings.TrimSpace(c.Help); h != "" {
				return h
			}
		}
	}
	return ""
}
