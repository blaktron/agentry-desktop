package yourcomputer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"

	"github.com/blaktron/agentry-desktop/internal/library"
)

func myCAD() Application {
	return Application{ID: "my-cad", Name: "My CAD",
		Server: &Server{Attachment: library.AttachmentStdio, Command: []string{"my-cad-mcp", "--stdio"}},
		Tools:  []Tool{{Name: "export_drawing", Class: library.ClassRead}, {Name: "run_macro"}}}
}

// Adding an application approves nothing, whatever the added entry said;
// the file is the person's alone; a tool nobody classed is exec.
func TestAddingApprovesNothing(t *testing.T) {
	s := InDir(t.TempDir())
	a := myCAD()
	a.Approve = Approval{MCP: true, Computer: true}
	d, err := s.Update(func(d *Doc) error { return d.Add(a) })
	if err != nil {
		t.Fatal(err)
	}
	got, _ := d.Lookup("my-cad")
	if got.Approve.MCP || got.Approve.Computer || got.UsesMCP() {
		t.Fatalf("an added application came approved: %+v", got.Approve)
	}
	if got.ClassOf("run_macro") != library.ClassExec || got.Classed("run_macro") || got.ClassOf("export_drawing") != library.ClassRead {
		t.Fatalf("classes: %+v", got.Tools)
	}
	if got.From.Protocol() != desktop.FromPerson || !strings.Contains(got.From.Words(), "set up by you") {
		t.Fatalf("from %+v", got.From)
	}
	fi, err := os.Stat(s.Path())
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("the list's file: %v %v", fi.Mode(), err)
	}
	if _, err := s.Update(func(d *Doc) error { return d.Add(myCAD()) }); err == nil {
		t.Fatal("a second application with the same id was added")
	}
}

// A preset becomes an application with its server, its classes, its
// withheld tools and its windows on this system, nothing approved; changing
// its server makes it the library's, changed by the person.
func TestAPresetIsSetUpUnapprovedAndEditsAreMarked(t *testing.T) {
	lib := library.Builtin()
	p, _ := lib.Lookup("blender")
	a := FromPreset(p, lib.Version(), "linux")
	if a.Approve != (Approval{}) || a.From.Protocol() != desktop.FromLibrary || a.Server.Env["BLENDER_MCP_SAFE_MODE"] != "1" ||
		!slices.Contains(a.Windows, "org.blender.Blender") || !a.Withholds("disable_telemetry") || a.ClassOf("execute_blender_code") != library.ClassExec {
		t.Fatalf("Blender from the library: %+v", a)
	}
	same := *a.Server
	a.SetServer(&same)
	if a.From.Edited {
		t.Fatal("setting the same server marked it edited")
	}
	changed := *a.Server
	changed.Env = map[string]string{"DISABLE_TELEMETRY": "true"}
	a.SetServer(&changed)
	if a.From.Protocol() != desktop.FromLibraryEdited {
		t.Fatalf("an edited preset is %s", a.From.Protocol())
	}
}

// The discovery adds, once, each preset found ready here, unapproved; one
// whose launcher or application is missing is not added; a preset whose
// screening the person had turned off comes without its variable.
func TestDiscoveredApplicationsAreAddedUnapproved(t *testing.T) {
	lib := library.Builtin()
	probe := Probe{
		LookPath: func(name string) (string, error) {
			if name == "uvx" {
				return "/usr/bin/uvx", nil
			}
			return "", errors.New("not found")
		},
		Flatpak:   func(id string) bool { return id == "org.blender.Blender" },
		Exists:    func(string) bool { return false },
		Listening: func(context.Context, int) bool { return false },
	}
	var d Doc
	added := Discover(context.Background(), &d, lib, "linux", probe, func(id string) bool { return id == "blender" })
	if !slices.Equal(added, []string{"blender"}) || !d.Discovered {
		t.Fatalf("added %v", added)
	}
	b, _ := d.Lookup("blender")
	if b.Approve != (Approval{}) {
		t.Fatalf("a discovered application came approved: %+v", b.Approve)
	}
	if _, kept := b.Server.Env["BLENDER_MCP_SAFE_MODE"]; kept || b.From.Protocol() != desktop.FromLibraryEdited {
		t.Fatalf("screening the person turned off came back: %+v %+v", b.Server.Env, b.From)
	}
	if again := Discover(context.Background(), &d, lib, "linux", probe, nil); again != nil {
		t.Fatalf("the discovery ran twice: %v", again)
	}
	// Removed, it stays removed.
	if err := d.Remove("blender"); err != nil {
		t.Fatal(err)
	}
	if again := Discover(context.Background(), &d, lib, "linux", probe, nil); again != nil {
		t.Fatalf("a removed application came back: %v", again)
	}

	var none Doc
	noUV := probe
	noUV.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	if added := Discover(context.Background(), &none, lib, "linux", noUV, nil); len(added) != 0 {
		t.Fatalf("a preset whose launcher is missing was added: %v", added)
	}
	var listening Doc
	unreal := Probe{Listening: func(_ context.Context, port int) bool { return port == 8000 }}
	if added := Discover(context.Background(), &listening, lib, "linux", unreal, nil); !slices.Equal(added, []string{"unreal"}) {
		t.Fatalf("an application whose listener answers: %v", added)
	}
}

// A choice for a QuickAgent holds across its versions and goes with the
// application it named.
func TestAChoiceIsRememberedPerQuickAgent(t *testing.T) {
	var d Doc
	if err := d.Add(myCAD()); err != nil {
		t.Fatal(err)
	}
	d.Choose("acme/cad@1.0.0", "chosen", "my-cad")
	if a, ok := d.Choice("acme/cad@1.2.0", "chosen"); !ok || a.ID != "my-cad" {
		t.Fatalf("the choice for a later version: %+v %v", a, ok)
	}
	if _, ok := d.Choice("acme/other@1.0.0", "chosen"); ok {
		t.Fatal("a choice reached another QuickAgent")
	}
	if err := d.Remove("my-cad"); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Choice("acme/cad@1.0.0", "chosen"); ok || len(d.Choices) != 0 {
		t.Fatalf("a choice outlived its application: %+v", d.Choices)
	}
}

// What the list refuses: an id a release cannot name or the link's own,
// an application with neither a server nor windows, a server off loopback,
// a class outside the three; a file a newer version wrote.
func TestTheListRefusesWhatCannotBeUsed(t *testing.T) {
	bad := []Application{
		{ID: "My CAD", Name: "x", Windows: []string{"x"}},
		{ID: "chosen", Name: "x", Windows: []string{"x"}},
		{ID: "x", Name: "x"},
		{ID: "x", Name: "x", Server: &Server{Attachment: library.AttachmentLoopbackHTTP, Endpoint: "http://example.com:8000/mcp"}},
		{ID: "x", Name: "x", Server: &Server{Attachment: library.AttachmentStdio}},
		{ID: "x", Name: "x", Windows: []string{"x"}, Tools: []Tool{{Name: "t", Class: "anything"}}},
	}
	for _, a := range bad {
		if err := a.Validate(); err == nil {
			t.Errorf("accepted %+v", a)
		}
	}
	ok := Application{ID: "kdenlive", Name: "Kdenlive", Windows: []string{"kdenlive"}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Server{Attachment: library.AttachmentLoopbackHTTP, Endpoint: "http://[::1]:8000/mcp"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if IDFor("My CAD 2!") != "my-cad-2" || IDFor("Computer") != "computer-app" {
		t.Fatalf("ids: %q %q", IDFor("My CAD 2!"), IDFor("Computer"))
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("version: 9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InDir(dir).Load(); err == nil || !strings.Contains(err.Error(), "newer Agentry Desktop") {
		t.Fatalf("a newer file: %v", err)
	}
}

// Loosening is any move toward fewer questions.
func TestLooseningIsTowardFewerQuestions(t *testing.T) {
	for _, c := range []struct {
		from, to library.Class
		want     bool
	}{{library.ClassExec, library.ClassRead, true}, {library.ClassExec, library.ClassAct, true}, {library.ClassAct, library.ClassRead, true},
		{library.ClassRead, library.ClassExec, false}, {"", library.ClassAct, true}, {library.ClassAct, library.ClassAct, false}} {
		if Loosens(c.from, c.to) != c.want {
			t.Errorf("%q → %q", c.from, c.to)
		}
	}
}
