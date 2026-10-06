package build

import (
	"fmt"
	"strings"
)

// Summary is the Build tab's summary (plan builder-simplify.md §4.1, D7;
// agentry-desktop#344), the one short page a QuickAgent is submitted from,
// whether the Drafter wrote it or the person did in the form: how it works,
// then what it needs. The plumbing (the Drafter's notes, sources and AI
// Review's counts, the namespace, version, licence, tags, include list,
// file name and run mode) stays in the form views.
//
// The capability contract is shown in full before Submit (the plan's §3):
// every key, website, MCP server with its tools, hosts and configuration,
// and file, command or data grant the model declares is here, and nothing
// it does not declare. The run modes and the intake questions stay in the
// form: neither widens what a run may touch (a secret question is carried
// by a declared key, which is listed). It mirrors the site's
// lib/builder-summary.ts summarize (agentry-web#651), the knowledge lines
// included since agentry-desktop#497: the packs it ships and the one it asks
// a person for, one line each, only those declared.
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
	// Help is the guide's help for it (Markdown, DEV-167), or "".
	Help string `json:"help"`
	// UsedBy is who asks for it: "the QuickAgent" for its own, and each
	// server's reference.
	UsedBy []string `json:"usedBy"`
	// Optional says a run can start without it (`optionalSecrets`,
	// agentry-desktop#655).
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
	// Config is the names of its plain configuration (env); the values
	// are in the form.
	Config []string `json:"config"`
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
	for _, srv := range m.MCPServers {
		ref := srv.Ref()
		sv := SummaryServer{Ref: ref, Tools: compact(srv.Tools), Hosts: []SummaryHost{}, Keys: compact(srv.Secrets), Config: []string{}}
		for _, h := range compact(srv.Egress) {
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
	// The site's words (lib/builder-summary.ts).
	if ships := compact(m.KnowledgeShips); len(ships) > 0 {
		word, them := "packs", "them"
		if len(ships) == 1 {
			word, them = "pack", "it"
		}
		s.Knowledge = append(s.Knowledge, fmt.Sprintf("Ships the knowledge %s %s in the release, unencrypted: anyone who can download it can read %s.", word, strings.Join(ships, ", "), them))
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
// it: source:package@version.
func (r MCPServerRow) Ref() string {
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
