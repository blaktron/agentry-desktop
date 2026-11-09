package library

import (
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestTheBlenderEntryIsTheVettedServerAtItsPin(t *testing.T) {
	b, ok := Builtin().Lookup("blender")
	if !ok {
		t.Fatal("the library has no blender entry")
	}
	if b.Application != "Blender" || b.Attachment != AttachmentStdio || b.Package.Ref() != "pypi:mcp-for-blender@2.2.7" {
		t.Fatalf("blender is %s, %s, %s", b.Application, b.Attachment, b.Package.Ref())
	}
	if want := []string{"uvx", "mcp-for-blender==2.2.7"}; !slices.Equal(b.Launch.Command, want) {
		t.Fatalf("launch %q, want %q", b.Launch.Command, want)
	}
	if want := map[string]string{"DISABLE_TELEMETRY": "true", "BLENDER_MCP_SAFE_MODE": "1"}; !maps.Equal(b.Launch.Env, want) {
		t.Fatalf("launch environment %v, want %v", b.Launch.Env, want)
	}
	if b.Listener.Port != 9876 {
		t.Fatalf("listener port %d, want the add-on's 9876", b.Listener.Port)
	}
	classes := map[string]Class{}
	for _, tool := range b.Tools {
		classes[tool.Name] = tool.Class
	}
	want := map[string]Class{
		"execute_blender_code": ClassExec,
		"generate_3d":          ClassAct, "import_asset": ClassAct,
		"search_assets": ClassRead, "look": ClassRead, "get_scene_info": ClassRead, "get_addon_status": ClassRead,
	}
	if !maps.Equal(classes, want) {
		t.Fatalf("tool classes %v, want %v", classes, want)
	}
	for _, off := range []string{"disable_telemetry", "record_trajectory_feedback"} {
		if _, given := b.Tool(off); given || !slices.Contains(b.Withheld, off) {
			t.Fatalf("%s is given to a run or not withheld", off)
		}
	}
	for _, host := range []string{"api.polyhaven.com", "api.sketchfab.com", "hyperhuman.deemos.com"} {
		if !slices.Contains(b.Egress, host) {
			t.Fatalf("egress %v lacks %s, which the asset and generation tools reach", b.Egress, host)
		}
	}
	for _, name := range []string{"generate_3d", "import_asset", "search_assets"} {
		if tool, _ := b.Tool(name); !tool.Egress {
			t.Fatalf("%s reaches the egress hosts and is not marked so", name)
		}
	}
}

func TestEveryEntryClassesEveryToolFromTheClosedSet(t *testing.T) {
	for _, e := range Builtin().All() {
		for _, tool := range e.Tools {
			if !slices.Contains(Classes(), tool.Class) {
				t.Errorf("%s: tool %s has class %q", e.ID, tool.Name, tool.Class)
			}
		}
	}
	for _, class := range []string{"", "admin", "READ", "write"} {
		doc := strings.Replace(validDoc, "class: exec", "class: "+class, 1)
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("a tool of class %q was accepted", class)
		}
	}
}

func TestAnEntryWithoutAnExactPinIsRefused(t *testing.T) {
	for name, doc := range map[string]string{
		"no version":              strings.Replace(validDoc, "version: 1.2.3", `version: ""`, 1),
		"a range":                 strings.Replace(validDoc, "version: 1.2.3", `version: ">=1.2"`, 1),
		"a wildcard":              strings.Replace(validDoc, "version: 1.2.3", `version: "1.*"`, 1),
		"latest":                  strings.Replace(validDoc, "version: 1.2.3", "version: latest", 1),
		"a launch without it":     strings.Replace(validDoc, "[uvx, example-mcp==1.2.3]", "[uvx, example-mcp]", 1),
		"a launch at another one": strings.Replace(validDoc, "[uvx, example-mcp==1.2.3]", "[uvx, example-mcp==1.2.4]", 1),
	} {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := Parse([]byte(validDoc)); err != nil {
		t.Fatalf("the valid document: %v", err)
	}
}

func TestTheLibraryRefusesWhatItCannotVouchFor(t *testing.T) {
	for name, doc := range map[string]string{
		"an unknown field":          strings.Replace(validDoc, "    attachment: stdio\n", "    attachment: stdio\n    shell: true\n", 1),
		"no library version":        strings.Replace(validDoc, "version: 1\n", "", 1),
		"a malformed id":            strings.Replace(validDoc, "id: example", "id: Example_App", 1),
		"an unknown ecosystem":      strings.Replace(validDoc, "ecosystem: pypi", "ecosystem: cargo", 1),
		"an http source":            strings.Replace(validDoc, "https://example.com/src", "http://example.com/src", 1),
		"an unknown attachment":     strings.Replace(validDoc, "attachment: stdio", "attachment: tcp", 1),
		"an unknown platform":       strings.Replace(validDoc, "[linux]", "[plan9]", 1),
		"a port out of range":       strings.Replace(validDoc, "port: 9000", "port: 70000", 1),
		"a tool listed twice":       strings.Replace(validDoc, "{name: read_thing, class: read}", "{name: run_code, class: read}", 1),
		"a tool given and withheld": strings.Replace(validDoc, "[hidden_tool]", "[read_thing]", 1),
		"an address as egress":      strings.Replace(validDoc, "[api.example.com]", "[127.0.0.1]", 1),
		"a wildcard as egress":      strings.Replace(validDoc, "[api.example.com]", `["*.example.com"]`, 1),
		"no steps":                  strings.Replace(validDoc, "steps: [Install it.]", "steps: []", 1),
		"another schema":            strings.Replace(validDoc, "schema: 1\n", "schema: 2\n", 1),
		"no schema":                 strings.Replace(validDoc, "schema: 1\n", "", 1),
		"no category":               strings.Replace(validDoc, "    category: Examples\n", "", 1),
		"a long summary":            strings.Replace(validDoc, "    category: Examples\n", "    category: Examples\n    summary: "+strings.Repeat("x", 121)+"\n", 1),
		"no listener on loopback": strings.NewReplacer(
			"attachment: stdio", "attachment: loopback-http",
			"    launch:\n      command: [uvx, example-mcp==1.2.3]\n", "    endpoint: http://127.0.0.1:9000/mcp\n",
			"    listener:\n      port: 9000\n", "").Replace(validDoc),
		"a blank variable with no note": strings.Replace(validDoc, "      command: [uvx, example-mcp==1.2.3]\n",
			"      command: [uvx, example-mcp==1.2.3]\n      env: {EXAMPLE_KEY: \"\"}\n", 1),
		"no tools": strings.Replace(validDoc, "      - {name: run_code, class: exec}\n      - {name: read_thing, class: read}\n",
			"      []\n", 1),
	} {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestALoopbackServerIsReachedOnLoopbackAlone(t *testing.T) {
	loopback := strings.NewReplacer(
		"attachment: stdio", "attachment: loopback-http",
		"    launch:\n      command: [uvx, example-mcp==1.2.3]\n", "    endpoint: http://127.0.0.1:9000/mcp\n",
	).Replace(validDoc)
	if _, err := Parse([]byte(loopback)); err != nil {
		t.Fatalf("a loopback endpoint: %v", err)
	}
	for _, endpoint := range []string{"http://localhost:9000/mcp", "http://192.168.1.5:9000/mcp", "https://127.0.0.1:9000/mcp", "http://u@127.0.0.1:9000/mcp", "http://127.0.0.1:8000/mcp", "http://127.0.0.1/mcp"} {
		doc := strings.Replace(loopback, "http://127.0.0.1:9000/mcp", endpoint, 1)
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("endpoint %s was accepted", endpoint)
		}
	}
}

// A step that names the server's release names the pinned one.
func TestTheStepsNameThePinnedRelease(t *testing.T) {
	for _, e := range Builtin().All() {
		re := regexp.MustCompile(regexp.QuoteMeta(e.Package.Name) + `(==|@)([0-9][^\s,]*)`)
		for _, step := range e.Steps {
			for _, m := range re.FindAllStringSubmatch(step, -1) {
				if m[2] != e.Package.Version {
					t.Errorf("%s: a step names %s, the pin is %s", e.ID, m[0], e.Package.Version)
				}
			}
		}
	}
}

func TestLookupHandsOutACopy(t *testing.T) {
	a, _ := Builtin().Lookup("blender")
	a.Tools[0].Class = ClassRead
	a.Launch.Env["BLENDER_MCP_SAFE_MODE"] = "0"
	b, _ := Builtin().Lookup("blender")
	if b.Tools[0].Class == ClassRead && b.Tools[0].Name == "execute_blender_code" {
		t.Fatal("a caller's change reached the library's tool classes")
	}
	if b.Launch.Env["BLENDER_MCP_SAFE_MODE"] != "1" {
		t.Fatal("a caller's change reached the library's launch environment")
	}
}

const validDoc = `schema: 1
version: 1
presets:
  - id: example
    application: Example
    category: Examples
    platforms: [linux]
    server:
      ecosystem: pypi
      name: example-mcp
      version: 1.2.3
      source: https://example.com/src
    attachment: stdio
    launch:
      command: [uvx, example-mcp==1.2.3]
    listener:
      port: 9000
    tools:
      - {name: run_code, class: exec}
      - {name: read_thing, class: read}
    withheld: [hidden_tool]
    egress: [api.example.com]
    steps: [Install it.]
    windows:
      linux: [example]
`

// The application's own plugin is a server no package registry holds: an
// entry names it only to be reached on loopback, never to be launched.
func TestTheApplicationsOwnServerIsReachedNotLaunched(t *testing.T) {
	own := strings.NewReplacer(
		"ecosystem: pypi", "ecosystem: application",
		"attachment: stdio", "attachment: loopback-http",
		"    launch:\n      command: [uvx, example-mcp==1.2.3]\n", "    endpoint: http://127.0.0.1:9000/mcp\n",
	).Replace(validDoc)
	if _, err := Parse([]byte(own)); err != nil {
		t.Fatalf("the application's own server on loopback: %v", err)
	}
	launched := strings.Replace(validDoc, "ecosystem: pypi", "ecosystem: application", 1)
	if _, err := Parse([]byte(launched)); err == nil || !strings.Contains(err.Error(), "the application's command, not uvx") {
		t.Fatalf("the application's own server through a package runner: %v", err)
	}
	ownCommand := strings.NewReplacer("ecosystem: pypi", "ecosystem: application", "[uvx, example-mcp==1.2.3]", "[xcrun, mcpbridge]").Replace(validDoc)
	if _, err := Parse([]byte(ownCommand)); err != nil {
		t.Fatalf("the application's own command: %v", err)
	}
}

// Unreal Engine's entry is Epic's own plugin on the editor's loopback
// endpoint: nothing launched, its three tools classed, no code tool, no
// egress, and steps that say it is Experimental and unauthenticated.
func TestTheUnrealEntryIsEpicsPluginOnLoopback(t *testing.T) {
	e, ok := Builtin().Lookup("unreal")
	if !ok {
		t.Fatal("the library has no unreal entry")
	}
	if e.Application != "Unreal Engine" || e.Attachment != AttachmentLoopbackHTTP || e.Launch != nil ||
		e.Endpoint != "http://127.0.0.1:8000/mcp" || e.Listener.Port != 8000 || e.Package.Ecosystem != EcosystemApplication {
		t.Fatalf("unreal = %+v", e)
	}
	want := map[string]Class{"list_toolsets": ClassRead, "describe_toolset": ClassRead, "call_tool": ClassAct}
	if len(e.Tools) != len(want) {
		t.Fatalf("unreal tools = %+v", e.Tools)
	}
	for _, tool := range e.Tools {
		if want[tool.Name] != tool.Class || tool.Egress {
			t.Errorf("unreal tool %+v", tool)
		}
	}
	if len(e.Egress) != 0 {
		t.Errorf("unreal egress = %v", e.Egress)
	}
	steps := strings.Join(e.Steps, " ")
	for _, w := range []string{"Unreal MCP", "All Toolsets", "Experimental", "no authentication", "8000", "Enable Tool Search"} {
		if !strings.Contains(steps, w) {
			t.Errorf("the steps do not say %q: %s", w, steps)
		}
	}
}

// A preset names its windows on every platform it is offered on, for
// computer use, and its environment notes only for variables its launch
// sets; Blender's preset screens scripts by default and says so.
func TestAPresetNamesItsWindowsAndItsDefaults(t *testing.T) {
	for _, p := range Builtin().All() {
		for _, goos := range p.Platforms {
			if len(p.WindowsOn(goos)) == 0 {
				t.Errorf("%s names no windows on %s", p.ID, goos)
			}
		}
	}
	b, _ := Builtin().Lookup("blender")
	if b.Launch.Env["BLENDER_MCP_SAFE_MODE"] != "1" || !strings.Contains(b.EnvNotes["BLENDER_MCP_SAFE_MODE"], "Screens each script") {
		t.Fatalf("Blender's screening default: %+v %+v", b.Launch.Env, b.EnvNotes)
	}
	noWindows := strings.Replace(validDoc, "    windows:\n      linux: [example]\n", "", 1)
	if _, err := Parse([]byte(noWindows)); err == nil || !strings.Contains(err.Error(), "window") {
		t.Fatalf("a preset with no windows: %v", err)
	}
	strayNote := validDoc + "    envNotes:\n      NOT_SET: something\n"
	if _, err := Parse([]byte(strayNote)); err == nil || !strings.Contains(err.Error(), "NOT_SET") {
		t.Fatalf("a note for a variable the launch does not set: %v", err)
	}
}

// A stdio server that needs no port of the application's names none, and
// a variable the person fills in is left blank with a note saying what goes
// there.
func TestAPresetMayNeedNoPortAndAskForAValue(t *testing.T) {
	doc := strings.NewReplacer(
		"    listener:\n      port: 9000\n", "",
		"      command: [uvx, example-mcp==1.2.3]\n", "      command: [uvx, example-mcp==1.2.3]\n      env: {EXAMPLE_KEY: \"\"}\n",
	).Replace(validDoc) + "    envNotes:\n      EXAMPLE_KEY: The key the application shows in its settings.\n"
	c, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("no port and a value to fill in: %v", err)
	}
	p, _ := c.Lookup("example")
	if p.Listener.Port != 0 || p.Launch.Env["EXAMPLE_KEY"] != "" || p.EnvNotes["EXAMPLE_KEY"] == "" {
		t.Fatalf("example = %+v", p)
	}
}

// A copy read from the repository may carry fields a later build added to
// the same schema; the copy built in may not.
func TestAFetchedCopyPassesOverFieldsALaterBuildAdded(t *testing.T) {
	later := strings.Replace(validDoc, "    category: Examples\n", "    category: Examples\n    icon: example.svg\n", 1)
	if _, err := Parse([]byte(later)); err == nil {
		t.Fatal("the built-in parse accepted an unknown field")
	}
	if _, err := ParseFetched([]byte(later)); err != nil {
		t.Fatalf("a fetched copy with a later field: %v", err)
	}
	bad := strings.Replace(later, "version: 1.2.3", `version: ">=1"`, 1)
	if _, err := ParseFetched([]byte(bad)); err == nil {
		t.Fatal("a fetched copy skipped the checks")
	}
}

// Every preset of the built-in library is offered somewhere, says what it
// is, and its stdio launch holds its pin or is the application's own
// command.
func TestTheBuiltinLibraryIsWhole(t *testing.T) {
	lib := Builtin()
	if lib.Len() < 2 {
		t.Fatalf("the library has %d presets", lib.Len())
	}
	for _, p := range lib.All() {
		if p.Category == "" || len(p.Platforms) == 0 || len(p.Steps) == 0 || len(p.Tools) == 0 {
			t.Errorf("%s is incomplete: %+v", p.ID, p)
		}
		if p.Attachment == AttachmentStdio && p.Package.Ecosystem != EcosystemApplication && !slices.Contains(p.Launch.Command, p.Package.pinnedArg()) {
			t.Errorf("%s launches %q without its pin %s", p.ID, p.Launch.Command, p.Package.pinnedArg())
		}
	}
}

// A variable the library leaves blank is one the person must fill in: its
// note never calls it optional. An optional one is left out of the launch,
// and a step says when to add it.
func TestABlankVariableIsAlwaysRequired(t *testing.T) {
	optional := regexp.MustCompile(`(?i)\b(if |optional|leave (it )?empty|only needed|if needed)`)
	for _, p := range Builtin().All() {
		if p.Launch == nil {
			continue
		}
		for k, v := range p.Launch.Env {
			if v == "" && optional.MatchString(p.EnvNotes[k]) {
				t.Errorf("%s leaves %s blank, and its note says it is optional: %s", p.ID, k, p.EnvNotes[k])
			}
		}
	}
}
