package build

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// bare is a QuickAgent that declares no capability at all: no web access,
// no file, command or data grant, no key and no server.
func bare() *Model {
	m := Default("acme")
	m.Name, m.Summary = "bare", "Declares nothing."
	m.FSRead = "none"
	return m
}

// kinds says which of the summary's capability lists hold anything.
func kinds(s *Summary) []string {
	var out []string
	if len(s.Keys) > 0 {
		out = append(out, "keys")
	}
	if len(s.Websites) > 0 {
		out = append(out, "websites")
	}
	if len(s.Servers) > 0 {
		out = append(out, "servers")
	}
	if len(s.Grants) > 0 {
		out = append(out, "grants")
	}
	return out
}

// The capability contract is shown in full before Submit (plan
// builder-simplify.md §3): every kind the form can declare is listed when
// it is declared, on its own, and nothing is listed that is not.
func TestTheSummaryListsEveryDeclaredCapabilityAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		name  string
		set   func(*Model)
		kinds []string
		want  func(*testing.T, *Summary)
	}{
		{name: "nothing declared", set: func(*Model) {}},
		{name: "its own key", set: func(m *Model) { m.Secrets = []string{"SLACK_TOKEN"} }, kinds: []string{"keys"},
			want: func(t *testing.T, s *Summary) {
				if s.Keys[0].Name != "SLACK_TOKEN" || !reflect.DeepEqual(s.Keys[0].UsedBy, []string{"the QuickAgent"}) {
					t.Errorf("keys = %+v", s.Keys)
				}
			}},
		{name: "websites", set: func(m *Model) { m.EgressMode, m.EgressHosts = "hosts", []string{"api.example.com", "docs.example.com"} },
			kinds: []string{"websites"},
			want: func(t *testing.T, s *Summary) {
				if len(s.Websites) != 2 || s.Websites[1].Host != "docs.example.com" {
					t.Errorf("websites = %+v", s.Websites)
				}
			}},
		{name: "any host", set: func(m *Model) { m.EgressMode = "all" }, kinds: []string{"websites"},
			want: func(t *testing.T, s *Summary) {
				if len(s.Websites) != 1 || s.Websites[0].Host != "*" {
					t.Errorf("websites = %+v", s.Websites)
				}
			}},
		// A list of hosts left behind by switching web access off is not
		// declared: the manifest writes no egress then.
		{name: "hosts typed, then web access off", set: func(m *Model) { m.EgressMode, m.EgressHosts = "none", []string{"api.example.com"} }},
		{name: "a server with its tools", set: func(m *Model) {
			m.MCPServers = []MCPServerRow{{Source: "npm", Package: "@scope/server-x", Version: "1.2.3", Tools: []string{"list_issues", "get_issue"}}}
		}, kinds: []string{"servers"},
			want: func(t *testing.T, s *Summary) {
				sv := s.Servers[0]
				if sv.Ref != "npm:@scope/server-x@1.2.3" || !reflect.DeepEqual(sv.Tools, []string{"list_issues", "get_issue"}) ||
					len(sv.Hosts) != 0 || len(sv.Keys) != 0 || len(sv.Config) != 0 {
					t.Errorf("server = %+v", sv)
				}
			}},
		// A server's key is a key the QuickAgent needs, and its hosts are
		// under the server, never the QuickAgent's own websites.
		{name: "a server's key, hosts and configuration", set: func(m *Model) {
			m.MCPServers = []MCPServerRow{{Source: "pypi", Package: "mcp-server-y", Version: "0.4.0", Secrets: []string{"GITHUB_PAT"},
				Egress: []string{"api.github.com"}, Env: []EnvRow{{Name: "GITHUB_API_URL", Value: "https://api.github.com"}}}}
		}, kinds: []string{"keys", "servers"},
			want: func(t *testing.T, s *Summary) {
				if s.Keys[0].Name != "GITHUB_PAT" || !reflect.DeepEqual(s.Keys[0].UsedBy, []string{"pypi:mcp-server-y@0.4.0"}) {
					t.Errorf("keys = %+v", s.Keys)
				}
				sv := s.Servers[0]
				if len(sv.Tools) != 0 || sv.Hosts[0].Host != "api.github.com" || !reflect.DeepEqual(sv.Keys, []string{"GITHUB_PAT"}) ||
					!reflect.DeepEqual(sv.Config, []string{"GITHUB_API_URL"}) {
					t.Errorf("server = %+v", sv)
				}
			}},
		{name: "reads the workspace", set: func(m *Model) { m.FSRead = "workspace" }, kinds: []string{"grants"},
			want: func(t *testing.T, s *Summary) { wantGrants(t, s, "Reads the files in its workspace.") }},
		{name: "writes paths", set: func(m *Model) { m.FSWrite, m.FSWritePaths = "paths", []string{"out", "report.md"} }, kinds: []string{"grants"},
			want: func(t *testing.T, s *Summary) { wantGrants(t, s, "Writes out, report.md in its workspace.") }},
		{name: "paths mode with no path", set: func(m *Model) { m.FSRead = "paths" }},
		{name: "a command", set: func(m *Model) { m.ExecCommands = []string{"jq"} }, kinds: []string{"grants"},
			want: func(t *testing.T, s *Summary) { wantGrants(t, s, "Runs the command jq.") }},
		{name: "commands", set: func(m *Model) { m.ExecCommands = []string{"git", "jq"} }, kinds: []string{"grants"},
			want: func(t *testing.T, s *Summary) { wantGrants(t, s, "Runs the commands git, jq.") }},
		{name: "data", set: func(m *Model) { m.DataAccess = []string{"email", "calendar"} }, kinds: []string{"grants"},
			want: func(t *testing.T, s *Summary) { wantGrants(t, s, "Touches these kinds of data: email, calendar.") }},
		// Help written for a key nothing declares names no key, and a
		// secret question with no declared key (which the form's rules
		// refuse) adds none either.
		{name: "help and a question, no key", set: func(m *Model) {
			m.Guide.Connect = []ConnectRow{{Secret: "STRAY_TOKEN", Help: "Make one."}}
			m.Intake = []IntakeRow{{ID: "api_key", Prompt: "Your key", Type: "secret"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := bare()
			tc.set(m)
			s := m.Summarize(nil)
			if got := kinds(s); !reflect.DeepEqual(got, tc.kinds) {
				t.Fatalf("listed %v, want %v: %+v", got, tc.kinds, s)
			}
			if tc.want != nil {
				tc.want(t, s)
			}
		})
	}
}

func wantGrants(t *testing.T, s *Summary, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(s.Grants, want) {
		t.Errorf("grants = %q, want %q", s.Grants, want)
	}
}

// Every key is listed once, with every place that asks for it and the
// help the guide writes for it; the summary's own text is the model's.
func TestTheSummarysKeysCarryTheirHelpAndWhoAsks(t *testing.T) {
	m := bare()
	m.Guide.Overview = "Reads the channel.\n\n- one\n- two"
	m.Secrets = []string{"SLACK_TOKEN", "GITHUB_PAT"}
	m.MCPServers = []MCPServerRow{
		{Source: "npm", Package: "gh", Version: "1.0.0", Secrets: []string{"GITHUB_PAT"}},
		{Source: "npm", Package: "gh-extra", Version: "2.0.0", Secrets: []string{"GITHUB_PAT", "EXTRA_KEY"}},
	}
	m.Guide.Connect = []ConnectRow{{Secret: "GITHUB_PAT", Help: "1. Open **Settings**."}, {Secret: "GITHUB_PAT", Help: "a second row"}}
	s := m.Summarize(nil)
	if s.Name != "bare" || s.Summary != "Declares nothing." || s.Overview != m.Guide.Overview {
		t.Errorf("how it works = %q %q %q", s.Name, s.Summary, s.Overview)
	}
	want := []SummaryKey{
		{Name: "SLACK_TOKEN", Help: "", UsedBy: []string{"the QuickAgent"}},
		{Name: "GITHUB_PAT", Help: "1. Open **Settings**.", UsedBy: []string{"the QuickAgent", "npm:gh@1.0.0", "npm:gh-extra@2.0.0"}},
		{Name: "EXTRA_KEY", Help: "", UsedBy: []string{"npm:gh-extra@2.0.0"}},
	}
	if !reflect.DeepEqual(s.Keys, want) {
		t.Errorf("keys = %+v\nwant %+v", s.Keys, want)
	}
}

// The hosts a draft's description did not name are marked wherever they
// appear, case folded, "*" included; nothing is marked for a QuickAgent
// written in the form.
func TestTheSummaryMarksTheHostsTheDescriptionDidNotName(t *testing.T) {
	m := bare()
	m.EgressMode, m.EgressHosts = "hosts", []string{"api.example.com", "Evil.Example"}
	m.MCPServers = []MCPServerRow{{Source: "npm", Package: "x", Version: "1.0.0", Egress: []string{"hooks.slack.com"}}}
	if s := m.Summarize(nil); s.AnyUnnamed || s.Websites[1].Unnamed {
		t.Errorf("a form-written QuickAgent is marked: %+v", s)
	}
	s := m.Summarize([]string{"evil.example", "hooks.slack.com"})
	if !s.AnyUnnamed || s.Websites[0].Unnamed || !s.Websites[1].Unnamed || !s.Servers[0].Hosts[0].Unnamed {
		t.Errorf("marks = %+v %+v", s.Websites, s.Servers)
	}
	m.EgressMode = "all"
	if s := m.Summarize([]string{"*"}); !s.Websites[0].Unnamed {
		t.Errorf("any host is not marked: %+v", s.Websites)
	}
}

// The page reads every list as a list: an empty one is [], never null (a
// null list blanked the desktop once; wiki lessons/nil-list-blanked-the-desktop).
func TestTheSummaryHasNoNullLists(t *testing.T) {
	m := bare()
	m.MCPServers = []MCPServerRow{{Source: "npm", Package: "x", Version: "1.0.0"}}
	m.Tags, m.Guide.Connect = nil, nil
	raw, err := json.Marshal(m.Summarize(nil))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "null") {
		t.Errorf("the summary carries a null: %s", raw)
	}
}
