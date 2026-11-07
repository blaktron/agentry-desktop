package run

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/runnerimage/desktoplink"
)

func blenderPrepared() *prepared {
	plan := &spec.Plan{Ref: "acme/scene@1.0.0", Digest: "sha256:abc"}
	plan.Capabilities.Desktop = &spec.DesktopCap{Servers: []spec.DesktopServerCap{{
		ID: "blender", Application: "Blender", Package: "pypi:mcp-for-blender@2.2.7",
		Tools: []spec.DesktopTool{{Name: "get_scene_info", Class: "read"}, {Name: "execute_blender_code", Class: "exec"}},
	}}}
	return &prepared{plan: plan, params: engine.Params{RunID: "R"}}
}

// The run binds last before its sandbox, naming its release and each
// application server with the tools the release gives it; with no link
// service it is refused, exit 11, naming Agentry Desktop; a link service's
// refusal keeps its reason and sentence; and the binding is ended once,
// whoever ends it first.
func TestARunBindsToTheLinkServiceBeforeItsSandbox(t *testing.T) {
	var ce *engine.ClassifiedError
	if err := blenderPrepared().bindDesktop(context.Background(), nil); !errors.As(err, &ce) || ce.Code != engine.ExitPolicy || !strings.Contains(err.Error(), "needs Agentry Desktop") {
		t.Fatalf("no link service: %v", err)
	}
	refusing := func(context.Context, desktop.BindRequest) (*engine.DesktopLink, error) {
		return nil, &desktop.Refusal{Reason: receipt.DesktopNotInstalled, Server: "blender", Message: "Blender does not answer on port 9876 on this computer"}
	}
	if err := blenderPrepared().bindDesktop(context.Background(), refusing); !errors.As(err, &ce) || ce.Code != engine.ExitPolicy || !strings.Contains(err.Error(), "port 9876") {
		t.Fatalf("a refused bind: %v", err)
	}
	var got desktop.BindRequest
	unbinds := 0
	p := blenderPrepared()
	err := p.bindDesktop(context.Background(), func(_ context.Context, req desktop.BindRequest) (*engine.DesktopLink, error) {
		got = req
		return &engine.DesktopLink{Token: "tok", Servers: map[string][]desktoplink.Tool{"blender": {{Name: "get_scene_info"}}},
			Call: func(context.Context, desktop.CallRequest) (desktoplink.Answer, error) {
				return desktoplink.Answer{}, nil
			},
			Unbind: func(context.Context) error { unbinds++; return nil }}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Run != "R" || got.Ref != "acme/scene@1.0.0" || got.Digest != "sha256:abc" || len(got.Servers) != 1 || got.Servers[0].ID != "blender" ||
		!slices.Equal(got.Servers[0].Tools, []string{"get_scene_info", "execute_blender_code"}) {
		t.Fatalf("the bind asked for %+v", got)
	}
	if p.params.Desktop == nil || p.params.Desktop.Token != "tok" {
		t.Fatal("the engine was not given the binding")
	}
	_ = p.params.Desktop.Unbind(context.Background())
	p.unbindDesktop()
	if unbinds != 1 {
		t.Fatalf("unbound %d times", unbinds)
	}
	// A plan that declares no application binds nothing.
	none := &prepared{plan: &spec.Plan{}}
	if err := none.bindDesktop(context.Background(), nil); err != nil || none.params.Desktop != nil {
		t.Fatalf("a plan with no desktop line: %v", err)
	}
}
