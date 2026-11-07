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
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Lifting an application's script screening is the person's word in a native
// dialog the page cannot answer: a No changes nothing, a Yes turns it off, and
// turning it back on asks nothing.
func TestTurningScreeningOffAsksInANativeDialog(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	answer := false
	var asked []string
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, _ string) (bool, error) {
		asked = append(asked, title)
		return answer, nil
	}}
	y := newYourComputerAPI(app, &RunAPI{app: app})
	if _, err := y.SetScreening("blender", false); err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatalf("a No script screening turned off: %v", err)
	}
	if st, _ := settings.Load(); st.IsScreeningOff("blender") {
		t.Fatal("a No script screening turned off")
	}
	answer = true
	if _, err := y.SetScreening("blender", false); err != nil {
		t.Fatal(err)
	}
	if st, _ := settings.Load(); !st.IsScreeningOff("blender") {
		t.Fatal("a Yes did not turn script screening off")
	}
	if _, err := y.SetScreening("blender", true); err != nil {
		t.Fatal(err)
	}
	if st, _ := settings.Load(); st.IsScreeningOff("blender") {
		t.Fatal("script screening was not turned back on")
	}
	if len(asked) != 2 || asked[0] != "Turn off Blender's script screening?" {
		t.Fatalf("dialogs: %q", asked)
	}
	if _, err := y.SetScreening("no-such-app", false); err == nil {
		t.Fatal("an application with no script screening was switched")
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
	y.link = linkservice.New(linkservice.Options{
		LookPath:  func(string) (string, error) { return "/usr/bin/uvx", nil },
		Listening: func(context.Context, int) error { return nil },
		Launch:    fakeLaunch,
		GOOS:      "linux",
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
