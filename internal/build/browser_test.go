package build

// A QuickAgent's browser line and target hosts belong to its contract: the
// form carries both whole from a draft or a loaded folder to the next save,
// and the list of fields a draft may carry follows the core's schema, so a
// capability the core gains cannot be left out of a draft unnoticed.

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

// browserDraft is a Drafter's manifest for a website test: a site question
// its browser opens, two optional keys the browser may type, and a target
// host beside it.
const browserDraft = `{"schemaVersion": "2", "name": "website-test-plan-runner", "namespace": "draft", "version": "0.1.0",
  "summary": "Executes a supplied website test plan step by step.", "license": "MIT",
  "modes": ["autonomous"], "entrypoints": {"instruction": "AGENT.md"},
  "intake": [{"id": "site", "prompt": "The website to test (https://…)", "type": "url", "required": true},
    {"id": "box", "prompt": "The staging server", "type": "host", "required": true}],
  "inputs": {"kinds": ["documents", "data"], "view": "text"},
  "capabilities": {"network": {"egress": "none"}, "filesystem": {"read": "none", "write": "none"}, "exec": "none",
    "secrets": ["SITE_USER", "SITE_PASSWORD"], "optionalSecrets": ["SITE_USER", "SITE_PASSWORD"],
    "dataAccess": ["files", "other"], "targetHosts": ["box"],
    "browser": {"sites": ["site"], "hosts": ["fonts.gstatic.com", "*.cdn.example.com"], "secrets": ["SITE_USER", "SITE_PASSWORD"]}},
  "publish": {"include": ["agent.yaml", "AGENT.md", "README.md", "LICENSE"]}}`

const browserInstructions = "# Website test\n\nOpen the site with `browser_navigate`, then `browser_snapshot`.\n"

func TestABrowserDraftKeepsItsBrowserAndTargetHosts(t *testing.T) {
	m, notes, err := FromDraft([]byte(browserDraft), browserInstructions, "blaktron")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if strings.Contains(n, "left out") {
			t.Fatalf("a browser draft lost fields: %s", n)
		}
	}
	want := &BrowserRow{Sites: []string{"site"}, Hosts: []string{"fonts.gstatic.com", "*.cdn.example.com"}, Secrets: []string{"SITE_USER", "SITE_PASSWORD"}}
	if !reflect.DeepEqual(m.Browser, want) {
		t.Fatalf("browser = %+v, want %+v", m.Browser, want)
	}
	if !slices.Equal(m.TargetHosts, []string{"box"}) {
		t.Fatalf("targetHosts = %v", m.TargetHosts)
	}
	if res := m.Check(); res.First != nil {
		t.Fatalf("the form's checks refuse a browser draft: %+v", res.Problems)
	}

	dir := t.TempDir()
	if err := Save(dir, m); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(dir + "/agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := manifest.Parse(first)
	if err != nil {
		t.Fatalf("the saved manifest does not parse: %v\n%s", err, first)
	}
	b := parsed.Capabilities.Browser
	if b == nil || !slices.Equal(b.Sites, want.Sites) || !slices.Equal(b.Hosts, want.Hosts) || !slices.Equal(b.Secrets, want.Secrets) || b.IsPublic() {
		t.Fatalf("saved browser = %+v\n%s", b, first)
	}
	if !slices.Equal(parsed.Capabilities.TargetHosts, []string{"box"}) {
		t.Fatalf("saved targetHosts = %v", parsed.Capabilities.TargetHosts)
	}

	// A load and a second save write the same file.
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Browser, want) || !slices.Equal(loaded.TargetHosts, []string{"box"}) {
		t.Fatalf("loaded browser = %+v, targetHosts = %v", loaded.Browser, loaded.TargetHosts)
	}
	again := t.TempDir()
	if err := Save(again, loaded); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(again + "/agent.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("a load and save changed the manifest:\n%s\n---\n%s", first, second)
	}
}

func TestTheSummaryNamesTheBrowserAndTheTargetHost(t *testing.T) {
	m, _, err := FromDraft([]byte(browserDraft), browserInstructions, "blaktron")
	if err != nil {
		t.Fatal(err)
	}
	grants := strings.Join(m.Summarize(nil).Grants, "\n")
	for _, want := range []string{
		"Opens these in a headless browser it drives: the site you name for site and its subdomains, and fonts.gstatic.com, *.cdn.example.com.",
		"Asks for a server (box) when it runs",
	} {
		if !strings.Contains(grants, want) {
			t.Fatalf("the summary's grants lack %q:\n%s", want, grants)
		}
	}
}

func TestAPublicBrowserKeepsItsScope(t *testing.T) {
	raw := strings.Replace(browserDraft, `"browser": {"sites": ["site"], "hosts": ["fonts.gstatic.com", "*.cdn.example.com"], "secrets": ["SITE_USER", "SITE_PASSWORD"]}`, `"browser": {"scope": "public"}`, 1)
	m, _, err := FromDraft([]byte(raw), browserInstructions, "blaktron")
	if err != nil {
		t.Fatal(err)
	}
	if m.Browser == nil || m.Browser.Scope != "public" {
		t.Fatalf("browser = %+v", m.Browser)
	}
	parsed, err := manifest.Parse([]byte(m.manifestYAML()))
	if err != nil {
		t.Fatalf("%v\n%s", err, m.manifestYAML())
	}
	if !parsed.Capabilities.Browser.IsPublic() {
		t.Fatalf("saved browser = %+v", parsed.Capabilities.Browser)
	}
}

func TestRemovingTheSiteQuestionIsAFormProblem(t *testing.T) {
	m, _, err := FromDraft([]byte(browserDraft), browserInstructions, "blaktron")
	if err != nil {
		t.Fatal(err)
	}
	m.Intake = slices.DeleteFunc(m.Intake, func(q IntakeRow) bool { return q.ID == "site" })
	res := m.Check()
	if res.First == nil {
		t.Fatal("a browser line naming a removed question passed the form's checks")
	}
	if res.First.Section != SectionAccess {
		t.Fatalf("the problem is in %s, not the access section: %+v", res.First.Section, res.Problems)
	}
}

func TestANoBrowserModelWritesNoBrowserLine(t *testing.T) {
	m := Default("alice")
	m.Name, m.Summary = "plain", "A plain QuickAgent."
	y := m.manifestYAML()
	if strings.Contains(y, "browser") || strings.Contains(y, "targetHosts") {
		t.Fatalf("a model with neither wrote one:\n%s", y)
	}
}

// leftOut are the core's fields the form does not carry, each on purpose.
var leftOut = map[string]bool{
	// The withdrawn v1 mcp-server kind's launch spec: decodeManifest
	// refuses that kind, and it is never valid for publish.
	"entrypoints.mcp": true,
	// A package server's arguments: the form's server row does not carry
	// them, so a manifest that declares them loses them in the form.
	"capabilities.mcpServers.args": true,
}

// TestDraftFieldsCoverTheCoresSchema walks the core's manifest types and
// requires draftFields to hold every key they read, so a field the core
// adds is either carried by the form or named here as one it leaves out.
func TestDraftFieldsCoverTheCoresSchema(t *testing.T) {
	var missing []string
	var walk func(t reflect.Type, known map[string]any, at string)
	walk = func(typ reflect.Type, known map[string]any, at string) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
			if name == "" || name == "-" || !f.IsExported() {
				continue
			}
			path := name
			if at != "" {
				path = at + "." + name
			}
			sch, ok := known[name]
			if !ok && !leftOut[path] {
				missing = append(missing, path)
				continue
			}
			switch s := sch.(type) {
			case map[string]any:
				walk(f.Type, s, path)
			case []any:
				if elem, _ := s[0].(map[string]any); elem != nil {
					walk(f.Type, elem, path)
				}
			}
		}
	}
	walk(reflect.TypeOf(manifest.Manifest{}), draftFields, "")
	if len(missing) > 0 {
		t.Fatalf("the core's manifest reads fields the Build form does not carry: %s", strings.Join(missing, ", "))
	}
}
