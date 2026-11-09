package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"

	"github.com/blaktron/agentry-desktop/internal/linkservice"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/yourcomputer"
)

// Approving an application for its MCP server or its windows is the
// person's word in a native dialog the page cannot answer: a No changes
// nothing, a Yes approves, and turning an approval off asks nothing.
// Loosening a tool's class asks too; tightening it does not.
func TestApprovingAndLooseningAskInANativeDialog(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	answer := false
	var asked []string
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, _ string) (bool, error) {
		asked = append(asked, title)
		return answer, nil
	}}
	y := newYourComputerAPI(app, &RunAPI{app: app})
	if _, err := y.AddFromLibrary("blender"); err != nil {
		t.Fatal(err)
	}
	approved := func() yourcomputer.Approval {
		d, _ := y.list.Load()
		a, _ := d.Lookup("blender")
		return a.Approve
	}
	if approved() != (yourcomputer.Approval{}) {
		t.Fatal("an application from the library came approved")
	}
	if _, err := y.SetApproval("blender", "mcp", true); err == nil || !strings.Contains(err.Error(), "not confirmed") || approved().MCP {
		t.Fatalf("a No approved: %v", err)
	}
	answer = true
	if _, err := y.SetApproval("blender", "mcp", true); err != nil || !approved().MCP {
		t.Fatalf("a Yes did not approve: %v", err)
	}
	n := len(asked)
	if _, err := y.SetApproval("blender", "mcp", false); err != nil || approved().MCP || len(asked) != n {
		t.Fatalf("turning it off: %v, asked %d", err, len(asked)-n)
	}
	if asked[0] != "Let agents use Blender's MCP server?" {
		t.Fatalf("dialogs: %q", asked)
	}
	answer = false
	if _, err := y.SetToolClass("blender", "execute_blender_code", "read"); err == nil {
		t.Fatal("a No loosened a class")
	}
	n = len(asked)
	if _, err := y.SetToolClass("blender", "look", "exec"); err != nil || len(asked) != n {
		t.Fatalf("tightening asked or failed: %v", err)
	}
	if v := y.State(); len(v.Applications) != 1 || len(v.Library) != 1 || v.Library[0].ID != "unreal" {
		t.Fatalf("the view: %+v", v)
	}
}

// The run setup resolves each application a release names as the link
// service will: by its id, by the person's choice for that QuickAgent, or
// not yet, with the step and the approved applications to choose from.
func TestTheRunSetupResolvesEachApplication(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
	y := newYourComputerAPI(app, &RunAPI{app: app})
	if _, err := y.AddApplication(NewApplication{Name: "My CAD", Server: "stdio", Command: []string{"my-cad-mcp"}}, []string{"export_drawing"}); err != nil {
		t.Fatal(err)
	}
	if _, err := y.SetApproval("my-cad", "mcp", true); err != nil {
		t.Fatal(err)
	}
	got := y.RunApplications("acme/cad@1.0.0", []run.DesktopEntry{{ID: "my-cad", Use: run.UseMCP}, {Use: run.UseMCP}, {ID: "blender", Use: run.UseMCP}})
	if got[0].Resolved == nil || got[0].Resolved.ID != "my-cad" || got[0].Chosen {
		t.Fatalf("by its id: %+v", got[0])
	}
	if got[1].Resolved != nil || got[1].Ready || len(got[1].Candidates) != 1 || !strings.Contains(got[1].Missing, "an application you choose") {
		t.Fatalf("a chooser: %+v", got[1])
	}
	if got[2].Preset != "Blender" || !strings.Contains(got[2].Missing, "add it from the library") {
		t.Fatalf("a preset not set up: %+v", got[2])
	}
	if err := y.ChooseApplication("acme/cad@1.0.0", "", "my-cad"); err != nil {
		t.Fatal(err)
	}
	if err := y.ChooseApplication("acme/cad@1.0.0", "blender", "my-cad"); err != nil {
		t.Fatal(err)
	}
	got = y.RunApplications("acme/cad@1.1.0", []run.DesktopEntry{{Use: run.UseMCP}, {ID: "blender", Use: run.UseMCP}})
	for _, ra := range got {
		if ra.Resolved == nil || ra.Resolved.ID != "my-cad" || !ra.Chosen {
			t.Fatalf("a choice: %+v", ra)
		}
	}
	if err := y.ChooseApplication("acme/cad@1.0.0", "my-cad", "my-cad"); err == nil {
		t.Fatal("a choice was taken for an id on the list")
	}
}

// The exec dialog says who asks, in which application, through which tool,
// and quotes what would run.
func TestTheExecDialogQuotesWhatWouldRun(t *testing.T) {
	title, msg := execDialogText(linkservice.ExecQuestion{Who: "acme/scene@1.0.0", Application: "Blender", Tool: "execute_blender_code", What: "import bpy\nbpy.ops.object.delete()"})
	if title != "Run code inside Blender?" {
		t.Fatalf("title %q", title)
	}
	for _, want := range []string{"acme/scene@1.0.0 asks to run this inside Blender", "execute_blender_code", "import bpy\nbpy.ops.object.delete()", "outside the run's sandbox"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the message lacks %q:\n%s", want, msg)
		}
	}
	if execAllowRun != "Allow every exec call for this run" {
		t.Fatalf("the third button reads %q", execAllowRun)
	}
}

// With no link service, the app's own run has nothing to bind to.
func TestARunWithNoLinkServiceHasNothingToBindTo(t *testing.T) {
	r := &RunAPI{}
	if _, err := r.bindDesktop(context.Background(), desktop.BindRequest{Run: "R"}); !errors.Is(err, desktop.ErrNoService) {
		t.Fatalf("bind with no link service: %v", err)
	}
}

// The page may pause a run's use of the computer, never give it back
// without the person's word: Resume asks in a native dialog, and a No
// leaves the run paused.
func TestResumingAPausedRunAsksInANativeDialog(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	answer := false
	asked := 0
	app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { asked++; return answer, nil }}
	y := newYourComputerAPI(app, &RunAPI{app: app})
	if _, err := y.AddFromLibrary("blender"); err != nil {
		t.Fatal(err)
	}
	if _, err := y.list.Update(func(d *yourcomputer.Doc) error {
		return d.Change("blender", func(a *yourcomputer.Application) error { a.Approve.MCP = true; return nil })
	}); err != nil {
		t.Fatal(err)
	}
	y.link = linkservice.New(linkservice.Options{
		Applications: y.list,
		LookPath:     func(string) (string, error) { return "/usr/bin/uvx", nil },
		Listening:    func(context.Context, int) error { return nil },
		Launch:       fakeLaunch,
		GOOS:         "linux",
	})
	if _, r := y.link.Bind(context.Background(), desktop.BindRequest{Run: "R", Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"look"}}}}); r != nil {
		t.Fatal(r)
	}
	if err := y.Pause("R"); err != nil || asked != 0 {
		t.Fatalf("Pause: %v, asked %d", err, asked)
	}
	if err := y.Resume("R"); err == nil || y.link.Runs()[0].State != linkservice.StatePaused {
		t.Fatalf("a No resumed the run: %v", err)
	}
	answer = true
	if err := y.Resume("R"); err != nil || y.link.Runs()[0].State != linkservice.StateActive || asked != 2 {
		t.Fatalf("a Yes did not resume: %v, asked %d", err, asked)
	}
}

// fakeLaunch stands in for an installed server that lists Blender's look.
func fakeLaunch(context.Context, linkservice.LaunchSpec) (linkservice.Session, error) {
	return fakeServer{}, nil
}

type fakeServer struct{}

func (fakeServer) Tools(context.Context) ([]desktoplink.Tool, error) {
	return []desktoplink.Tool{{Name: "look"}}, nil
}
func (fakeServer) Call(context.Context, string, json.RawMessage) (desktoplink.Answer, error) {
	return desktoplink.TextAnswer("looked", false), nil
}
func (fakeServer) Era() string  { return linkservice.EraLegacy }
func (fakeServer) Close() error { return nil }
