package linkservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"

	"github.com/blaktron/agentry-desktop/internal/library"
	"github.com/blaktron/agentry-desktop/internal/yourcomputer"
)

// myCAD is an application the person set up by hand: a stdio server with
// one tool classed read and one nobody classed, no windows.
func myCAD() yourcomputer.Application {
	return yourcomputer.Application{ID: "my-cad", Name: "My CAD",
		Server:  &yourcomputer.Server{Attachment: library.AttachmentStdio, Command: []string{"my-cad-mcp"}},
		Tools:   []yourcomputer.Tool{{Name: "export_drawing", Class: library.ClassRead}},
		Approve: yourcomputer.Approval{MCP: true}}
}

// kdenlive is an application the person set up for its windows alone.
func kdenlive() yourcomputer.Application {
	return yourcomputer.Application{ID: "kdenlive", Name: "Kdenlive", Windows: []string{"kdenlive", "org.kde.kdenlive"},
		Approve: yourcomputer.Approval{Computer: true}}
}

// personRig is a link service over a list of apps, whose servers list
// tools, and a fake window system.
func personRig(t *testing.T, tools []string, apps ...yourcomputer.Application) (*rig, *screenBackend) {
	t.Helper()
	r, sb := computerRig(t, true, false)
	r.sess.tools = tools
	r.list = approvedList(t, apps...)
	r.s.opt.Applications = r.list
	return r, sb
}

// Nothing is approved by default: an application on the list whose server
// the person has not approved refuses the bind desktop-not-granted with the
// switch to turn on, and one whose windows they have not approved refuses
// computer use the same way. Nothing is launched.
func TestAnUnapprovedApplicationRefusesTheBind(t *testing.T) {
	unapproved := blenderApp(t)
	unapproved.Approve = yourcomputer.Approval{}
	r, _ := personRig(t, standInTools, unapproved)
	_, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "R", Servers: []desktop.BindServer{{ID: "blender"}}})
	if ref == nil || ref.Reason != receipt.DesktopNotGranted || ref.Server != "blender" || !strings.Contains(ref.Message, `"Agents may use its MCP server"`) {
		t.Fatalf("an unapproved server: %+v", ref)
	}
	_, ref = r.s.Bind(context.Background(), desktop.BindRequest{Protocol: desktop.Protocol, Run: "S", ComputerUse: "app",
		ComputerTools: []string{"screenshot"}, Windows: []string{"blender"}})
	if ref == nil || ref.Reason != receipt.DesktopNotGranted || ref.Server != spec.DesktopComputerID || !strings.Contains(ref.Message, `"Agents may see and use its windows"`) {
		t.Fatalf("unapproved windows: %+v", ref)
	}
	if len(r.launches) != 0 || len(r.s.Runs()) != 0 {
		t.Fatalf("an unapproved bind launched %+v or stayed bound", r.launches)
	}
}

// A server the person set up binds by its id, from: person, every tool it
// lists given; a tool nobody classed is exec and asks before each call, and
// once the person classes it read it no longer asks.
func TestAnUnclassedToolAsksFirst(t *testing.T) {
	r, _ := personRig(t, []string{"export_drawing", "run_macro"}, myCAD())
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{Protocol: desktop.Protocol, Run: "R", Ref: "acme/cad@1.0.0", Servers: []desktop.BindServer{{ID: "my-cad"}}})
	if ref != nil {
		t.Fatal(ref)
	}
	if b.From["my-cad"] != desktop.FromPerson {
		t.Fatalf("from %+v", b.From)
	}
	classes := map[string]string{}
	for _, tool := range b.Servers["my-cad"] {
		classes[tool.Name] = tool.Class
	}
	if classes["export_drawing"] != "read" || classes["run_macro"] != "exec" {
		t.Fatalf("classes %v", classes)
	}
	if !slices.Equal(r.launches[0].Command, []string{"my-cad-mcp"}) {
		t.Fatalf("launched %+v", r.launches)
	}
	if _, ref := call(r, b.Token, "my-cad", "run_macro", `{"macro":"x"}`); ref == nil || ref.Reason != receipt.DesktopDeclined {
		t.Fatalf("an unclassed tool's call: %+v", ref)
	}
	if q := r.questions(); len(q) != 1 || q[0].Tool != "run_macro" || q[0].Application != "My CAD" {
		t.Fatalf("asked %+v", q)
	}
	if _, ref := call(r, b.Token, "my-cad", "export_drawing", `{}`); ref != nil {
		t.Fatal(ref)
	}
	if len(r.questions()) != 1 {
		t.Fatal("a read call asked")
	}
	if _, err := r.list.Update(func(d *yourcomputer.Doc) error {
		return d.Change("my-cad", func(a *yourcomputer.Application) error {
			a.Tools = append(a.Tools, yourcomputer.Tool{Name: "run_macro", Class: library.ClassRead})
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	b2, ref := r.s.Bind(context.Background(), desktop.BindRequest{Protocol: desktop.Protocol, Run: "S", Servers: []desktop.BindServer{{ID: "my-cad"}}})
	if ref != nil {
		t.Fatal(ref)
	}
	if _, ref := r.s.Call(context.Background(), desktop.CallRequest{Run: "S", Token: b2.Token, Server: "my-cad", Tool: "run_macro"}); ref != nil {
		t.Fatal(ref)
	}
	if len(r.questions()) != 1 {
		t.Fatal("a tool the person classed read still asked")
	}
}

// An application the person chooses is bound by the chosen entry and named
// in the answer; an id the list does not have is put to the person, whose
// choice is remembered for that QuickAgent; with no one to ask, each is
// refused with the step to take; a library preset not set up names its
// set-up.
func TestTheBindResolvesEveryIDOnThePersonsComputer(t *testing.T) {
	r, _ := personRig(t, []string{"export_drawing"}, myCAD())
	var asked []ChooseQuestion
	r.s.opt.Choose = func(_ context.Context, q ChooseQuestion) (string, error) {
		asked = append(asked, q)
		return "my-cad", nil
	}
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{Protocol: desktop.Protocol, Run: "R", Ref: "acme/cad@1.0.0",
		Servers: []desktop.BindServer{{ID: spec.DesktopChosenID, Chosen: true}}})
	if ref != nil {
		t.Fatal(ref)
	}
	if b.Chosen == nil || b.Chosen.ID != "my-cad" || b.Chosen.Name != "My CAD" || len(b.Servers[spec.DesktopChosenID]) != 1 {
		t.Fatalf("the chosen entry: %+v %+v", b.Chosen, b.Servers)
	}
	if len(asked) != 1 || asked[0].Entry != "" || asked[0].Use != UseMCP || len(asked[0].Candidates) != 1 {
		t.Fatalf("asked %+v", asked)
	}
	if _, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "S", Ref: "acme/draw@2.0.0", Servers: []desktop.BindServer{{ID: "cad-program"}}}); ref != nil {
		t.Fatal(ref)
	}
	if len(asked) != 2 || asked[1].Entry != "cad-program" {
		t.Fatalf("an unmatched id was not put to the person: %+v", asked)
	}
	r.s.opt.Choose = nil
	if _, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "T", Ref: "acme/draw@2.1.0", Servers: []desktop.BindServer{{ID: "cad-program"}}}); ref != nil {
		t.Fatalf("the remembered choice did not hold for a later version: %+v", ref)
	}
	_, ref = r.s.Bind(context.Background(), desktop.BindRequest{Run: "U", Ref: "acme/other@1.0.0", Servers: []desktop.BindServer{{ID: "cad-program"}}})
	if ref == nil || ref.Reason != receipt.DesktopNotInstalled || !strings.Contains(ref.Message, "an application called cad-program") {
		t.Fatalf("an unmatched id with no one to ask: %+v", ref)
	}
	_, ref = r.s.Bind(context.Background(), desktop.BindRequest{Run: "V", Ref: "acme/scene@1.0.0", Servers: []desktop.BindServer{{ID: "blender"}}})
	if ref == nil || ref.Reason != receipt.DesktopNotInstalled || !strings.Contains(ref.Message, "add it from the library") || !strings.Contains(ref.Message, "Blender") {
		t.Fatalf("a preset not set up: %+v", ref)
	}
	r.s.opt.Choose = func(context.Context, ChooseQuestion) (string, error) { return "", errors.New("declined") }
	_, ref = r.s.Bind(context.Background(), desktop.BindRequest{Run: "W", Ref: "acme/fresh@1.0.0", Servers: []desktop.BindServer{{ID: spec.DesktopChosenID, Chosen: true}}})
	if ref == nil || ref.Reason != receipt.DesktopDeclined {
		t.Fatalf("a declined choice: %+v", ref)
	}
}

// Computer use reaches the windows of each application the bind names,
// approved for its windows, an application with no MCP server included;
// the window system is held to that application's identifiers.
func TestAWindowOnlyApplicationIsUsedThroughComputerUse(t *testing.T) {
	r, sb := personRig(t, nil, kdenlive())
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{Protocol: desktop.Protocol, Run: "R", Ref: "acme/cut@1.0.0",
		ComputerUse: "app", ComputerTools: desktoplink.ComputerToolNames(), Windows: []string{"kdenlive"}})
	if ref != nil {
		t.Fatal(ref)
	}
	if b.Computer == nil || !slices.Equal(b.Computer.Windows, []string{"Kdenlive"}) {
		t.Fatalf("bound %+v", b.Computer)
	}
	if len(sb.req.Apps) != 1 || !slices.Equal(sb.req.Apps[0].Classes, []string{"kdenlive", "org.kde.kdenlive"}) {
		t.Fatalf("the window system was opened for %+v", sb.req)
	}
	if len(r.launches) != 0 {
		t.Fatalf("a windows-only application launched %+v", r.launches)
	}
	_, ref = r.s.Bind(context.Background(), desktop.BindRequest{Protocol: desktop.Protocol, Run: "S", Servers: []desktop.BindServer{{ID: "kdenlive"}}})
	if ref == nil || !strings.Contains(ref.Message, "has no MCP server set up") {
		t.Fatalf("a windows-only application's server: %+v", ref)
	}
}

// The library sets up; it does not gate: no code of the link service or
// computer use names a preset's id.
func TestNoBranchOnALibraryID(t *testing.T) {
	var ids []string
	for _, p := range library.Builtin().All() {
		ids = append(ids, regexp.QuoteMeta(p.ID))
	}
	re := regexp.MustCompile(`"(` + strings.Join(ids, "|") + `)"`)
	for _, dir := range []string{".", "../computeruse"} {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if m := re.Find(b); m != nil {
				t.Errorf("%s names the library's %s", path, m)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// A persistent agent's segment binds a hand-added application lazily: with
// the application closed, the tools the list knows stand for its server's
// and a call is refused with the step; once it opens, the next call starts
// its server and is answered, the class the person gave it applied.
func TestAHandAddedApplicationBindsLazilyForAnAgent(t *testing.T) {
	cad := yourcomputer.Application{ID: "my-cad", Name: "My CAD",
		Server:  &yourcomputer.Server{Attachment: library.AttachmentLoopbackHTTP, Endpoint: "http://127.0.0.1:9123/mcp", Listener: 9123},
		Tools:   []yourcomputer.Tool{{Name: "export_drawing", Class: library.ClassRead}},
		Approve: yourcomputer.Approval{MCP: true}}
	r, _ := personRig(t, []string{"export_drawing"}, cad)
	open := false
	r.s.opt.Listening = func(context.Context, int) error {
		if open {
			return nil
		}
		return errors.New("refused")
	}
	r.s.opt.DialHTTP = func(context.Context, string) (Session, error) { return r.sess, nil }
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{Protocol: desktop.Protocol, Run: "R", Agent: "acme.helper", Lazy: true, Servers: []desktop.BindServer{{ID: "my-cad"}}})
	if ref != nil {
		t.Fatal(ref)
	}
	if len(b.Servers["my-cad"]) != 1 || b.Servers["my-cad"][0].Name != "export_drawing" || b.Servers["my-cad"][0].Class != "read" || b.From["my-cad"] != desktop.FromPerson {
		t.Fatalf("the lazy binding: %+v %+v", b.Servers, b.From)
	}
	if _, ref := call(r, b.Token, "my-cad", "export_drawing", `{}`); ref == nil || ref.Reason != receipt.DesktopNotInstalled || !strings.Contains(ref.Message, "port 9123") {
		t.Fatalf("a call while My CAD is closed: %+v", ref)
	}
	open = true
	if _, ref := call(r, b.Token, "my-cad", "export_drawing", `{}`); ref != nil {
		t.Fatalf("a call once My CAD is open: %+v", ref)
	}
	if len(r.questions()) != 0 {
		t.Fatal("a tool the person classed read asked")
	}
}
