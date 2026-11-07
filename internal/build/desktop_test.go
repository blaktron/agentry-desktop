package build

// capabilities.desktop belongs to the contract: the form offers the
// applications of the catalogue of application servers built into the app,
// carries the release's narrowing and computer use whole from a draft or a
// loaded folder to the next save, and says what the install consent will
// ask in its own words.

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/appserver"
	"github.com/blaktron/agentry/pkg/manifest"
)

// desktopDraft is a Drafter's manifest for a Blender helper: Blender's
// server narrowed to two tools, and computer use of its windows.
const desktopDraft = `{"schemaVersion": "2", "name": "scene-helper", "namespace": "draft", "version": "0.1.0",
  "summary": "Tidies a Blender scene.", "license": "MIT",
  "modes": ["autonomous"], "entrypoints": {"instruction": "AGENT.md"},
  "capabilities": {"network": {"egress": "none"}, "filesystem": {"read": "none", "write": "none"}, "exec": "none",
    "desktop": {"servers": [{"id": "blender", "tools": ["get_scene_info", "execute_blender_code"]}], "computerUse": "app"}},
  "publish": {"include": ["agent.yaml", "AGENT.md", "README.md", "LICENSE"]}}`

// A release that declares the line goes through the form unchanged: the
// draft keeps it with no note of a field left out, the save writes it, a load
// reads it back, and a second save writes the same file.
func TestADesktopLineRoundTripsThroughTheForm(t *testing.T) {
	m, notes, err := FromDraft([]byte(desktopDraft), "# Scene helper\n\nTidy the scene.\n", "blaktron")
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if strings.Contains(n, "left out") || strings.Contains(n, "desktop") {
			t.Fatalf("a desktop draft lost its line: %s", n)
		}
	}
	want := &DesktopRow{Servers: []DesktopServerRow{{ID: "blender", Tools: []string{"get_scene_info", "execute_blender_code"}}}, ComputerUse: "app"}
	if !reflect.DeepEqual(m.Desktop, want) {
		t.Fatalf("desktop = %+v, want %+v", m.Desktop, want)
	}
	if res := m.Check(); res.First != nil {
		t.Fatalf("the form's checks refuse a desktop draft: %+v", res.Problems)
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
	d := parsed.Capabilities.Desktop
	if d == nil || len(d.Servers) != 1 || d.Servers[0].ID != "blender" || !slices.Equal(d.Servers[0].Tools, want.Servers[0].Tools) || d.ComputerUse != manifest.ComputerUseApp {
		t.Fatalf("saved desktop = %+v\n%s", d, first)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Desktop, want) {
		t.Fatalf("loaded desktop = %+v", loaded.Desktop)
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

// A server without a narrowing is written with no tools, every tool the
// catalogue gives, and read back the same; a line with nothing in it writes
// nothing, and a model without the line writes no desktop key.
func TestADesktopLineWithoutNarrowingOrNothingInIt(t *testing.T) {
	m, _, err := FromDraft([]byte(strings.Replace(desktopDraft, `, "tools": ["get_scene_info", "execute_blender_code"]}], "computerUse": "app"`, `}]`, 1)), "x\n", "blaktron")
	if err != nil {
		t.Fatal(err)
	}
	y := m.manifestYAML()
	if !strings.Contains(y, "  desktop:\n    servers:\n      - id: blender\n") || strings.Contains(y, "tools:") || strings.Contains(y, "computerUse") {
		t.Fatalf("an unnarrowed server:\n%s", y)
	}
	m.Desktop = &DesktopRow{Servers: []DesktopServerRow{}}
	if strings.Contains(m.manifestYAML(), "desktop") {
		t.Fatalf("an empty line was written:\n%s", m.manifestYAML())
	}
	m.Desktop = nil
	if strings.Contains(m.manifestYAML(), "desktop") {
		t.Fatalf("no line was written as one:\n%s", m.manifestYAML())
	}
}

// The words the form shows are the install consent's, built from the same
// core descriptions the CLI and the install dialog use, and the summary
// carries them.
func TestTheDesktopConsentIsTheInstallsWords(t *testing.T) {
	m := &Model{Desktop: &DesktopRow{Servers: []DesktopServerRow{{ID: "blender"}}, ComputerUse: "app"}}
	got := DesktopConsent(m)
	srv := manifest.DescribeDesktopServer(manifest.DesktopServer{ID: "blender"})
	want := []string{"Can use " + srv + ".", "Can see and use Blender's windows on your computer."}
	if !slices.Equal(got, want) {
		t.Fatalf("consent = %q, want %q", got, want)
	}
	if !strings.HasPrefix(srv, "Blender on your computer through its MCP server (") || !strings.Contains(srv, "1 runs code inside Blender") {
		t.Fatalf("the core's words moved: %q", srv)
	}
	narrowed := DesktopConsent(&Model{Desktop: &DesktopRow{Servers: []DesktopServerRow{{ID: "blender", Tools: []string{"look"}}}}})
	if len(narrowed) != 1 || !strings.Contains(narrowed[0], "(1 tool)") {
		t.Fatalf("a narrowed server's consent: %q", narrowed)
	}
	if DesktopConsent(&Model{}) != nil || DesktopConsent(&Model{Desktop: &DesktopRow{}}) != nil {
		t.Fatal("no line asks for consent")
	}
	s := m.Summarize(nil)
	for _, w := range want {
		if !slices.Contains(s.Grants, w) {
			t.Fatalf("the summary lacks %q: %q", w, s.Grants)
		}
	}
}

// The form offers every entry of the catalogue built into the app, with each
// tool a run may be given and its class, and never a withheld tool.
func TestTheFormOffersTheCatalogue(t *testing.T) {
	apps := DesktopApps()
	all := appserver.Builtin().All()
	if len(apps) != len(all) || len(apps) == 0 {
		t.Fatalf("offered %d of %d entries", len(apps), len(all))
	}
	for i, e := range all {
		a := apps[i]
		if a.ID != e.ID || a.Application != e.Application || len(a.Tools) != len(e.Tools) {
			t.Fatalf("entry %s offered as %+v", e.ID, a)
		}
		for j, tool := range e.Tools {
			if a.Tools[j].Name != tool.Name || a.Tools[j].Class != string(tool.Class) {
				t.Fatalf("%s tool %d offered as %+v", e.ID, j, a.Tools[j])
			}
		}
		for _, w := range e.Withheld {
			for _, tool := range a.Tools {
				if tool.Name == w {
					t.Fatalf("%s offers the withheld %s", e.ID, w)
				}
			}
		}
	}
}
