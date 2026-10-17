package draft

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/drafter"

	"github.com/blaktron/agentry-desktop/internal/build"
)

// Review marks every host the person's words do not name, as the site's
// import does.
func TestUnmentionedHostsAreTheOnesThePersonNeverNamed(t *testing.T) {
	m := build.Default("chris")
	m.EgressMode, m.EgressHosts = "hosts", []string{"api.github.com", "Slack.com", "evil.example"}
	m.MCPServers = []build.MCPServerRow{{Egress: []string{"hooks.slack.com", "api.osv.dev"}}}
	words := PersonsWords("Summarise the week's closed issues on github.com and post them to slack.com.", []string{"The services or sites it works with: OSV"})
	got := UnmentionedHosts(m, words)
	if !slices.Equal(got, []string{"evil.example", "api.osv.dev"}) {
		t.Fatalf("unmentioned %q", got)
	}
	m.EgressMode = "all"
	if got := UnmentionedHosts(m, "any host at all: *"); !slices.Contains(got, "*") {
		t.Fatalf("%q: any host is always marked", got)
	}
	// A Describe again's words are the person's: what they first asked and
	// their note, never the earlier draft it quoted, whose hosts are what
	// the marks are for.
	again := DescribeAgain("Post it to Slack too.", "Summarise closed issues on github.com.", "egress: [evil.example]\n", "")
	m.EgressMode, m.EgressHosts, m.MCPServers = "hosts", []string{"evil.example", "github.com"}, nil
	if got := UnmentionedHosts(m, PersonsWords(again, nil)); !slices.Equal(got, []string{"evil.example"}) {
		t.Fatalf("after Describe again %q", got)
	}
	// A name inside a longer one is not a mention.
	m.EgressHosts = []string{"hub.com"}
	if got := UnmentionedHosts(m, "github.com"); !slices.Equal(got, []string{"hub.com"}) {
		t.Fatalf("%q", got)
	}
}

func TestProblemsListWhatTheDraftStillFailsInTheSitesWords(t *testing.T) {
	v := map[string]any{
		"issues": []any{
			map[string]any{"rule": "V5", "path": "capabilities.network.egress[0]", "message": "not a host", "severity": "error"},
			map[string]any{"rule": "V9", "message": "a warning", "severity": "warning"},
		},
		"inventory": []any{map[string]any{"scanner": "S1", "filePath": "AGENT.md", "title": "a file"}},
		"findings":  []any{map[string]any{"scanner": "S4", "ruleId": "S4-001", "severity": "high", "filePath": "AGENT.md", "lineStart": 3, "title": "override"}},
	}
	raw, _ := json.Marshal(v)
	d := &drafter.Draft{Validation: raw, ManifestError: "it does not parse"}
	got, more := Problems(d)
	want := []string{
		"The draft could not be checked: it does not parse",
		"V5 capabilities.network.egress[0]: not a host",
		"S1 AGENT.md: a file",
		"S4 S4-001 (high) in AGENT.md line 3: override",
	}
	if !slices.Equal(got, want) || more != 0 {
		t.Fatalf("%q %d", got, more)
	}
	var many []any
	for i := range MaxProblems + 7 {
		many = append(many, map[string]any{"rule": "V1", "message": fmt.Sprint(i), "severity": "error"})
	}
	raw, _ = json.Marshal(map[string]any{"issues": many})
	if got, more := Problems(&drafter.Draft{Validation: raw}); len(got) != MaxProblems || more != 7 {
		t.Fatalf("%d listed, %d more", len(got), more)
	}
	if got, _ := Problems(&drafter.Draft{Validation: json.RawMessage(`"not a report"`)}); len(got) != 1 || !strings.Contains(got[0], "could not be read") {
		t.Fatalf("%q", got)
	}
}

// A remote server's one host is its address's, so a draft that reaches a
// vendor the person never named has it marked.
func TestARemoteServersHostIsMarkedWhenThePersonNeverNamedIt(t *testing.T) {
	m := build.Default("chris")
	m.MCPServers = []build.MCPServerRow{{Source: "url", URL: "https://mcp.linear.app/mcp", Auth: "oauth", Egress: []string{"hidden.example"}}}
	if got := UnmentionedHosts(m, PersonsWords("Triage my Linear issues.", nil)); !slices.Equal(got, []string{"mcp.linear.app"}) {
		t.Fatalf("unmentioned %q: the remote server's host, and never a package field its shape hides", got)
	}
	if got := UnmentionedHosts(m, PersonsWords("Triage my issues on mcp.linear.app.", nil)); len(got) != 0 {
		t.Fatalf("named, so not marked: %q", got)
	}
}
