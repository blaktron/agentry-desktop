package linkservice

import (
	"context"
	"errors"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
)

// A persistent agent's segment binds whether or not the application is
// running: its tools are the catalogue's until the application answers,
// a call while it does not is refused with the step to take, and the next
// call once it does starts the installed server and runs. Opening or
// closing the application never refuses the bind.
func TestAnAgentsBindReadsTheApplicationAtEachCall(t *testing.T) {
	r := newRig(t)
	running := false
	r.s.opt.Listening = func(context.Context, int) error {
		if !running {
			return errors.New("connection refused")
		}
		return nil
	}
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "R", Ref: "agentry/assistant@1.20.0", Agent: "Assistant", Lazy: true,
		Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"get_scene_info", "look"}}}})
	if ref != nil {
		t.Fatalf("an agent's bind with Blender closed was refused: %+v", ref)
	}
	if len(b.Servers["blender"]) != 2 || len(r.launches) != 0 {
		t.Fatalf("the bind listed %+v and launched %d servers", b.Servers, len(r.launches))
	}
	if _, ref := call(r, b.Token, "blender", "get_scene_info", `{}`); ref == nil || ref.Reason != receipt.DesktopNotInstalled || ref.Message == "" {
		t.Fatalf("a call with Blender closed: %+v", ref)
	}
	running = true
	if _, ref := call(r, b.Token, "blender", "get_scene_info", `{}`); ref != nil {
		t.Fatalf("a call with Blender open: %+v", ref)
	}
	if len(r.launches) != 1 {
		t.Fatalf("the installed server was launched %d times", len(r.launches))
	}
	// Closed again: the next call is refused with the step, though the
	// installed server still runs.
	running = false
	if _, ref := call(r, b.Token, "blender", "get_scene_info", `{}`); ref == nil || ref.Reason != receipt.DesktopNotInstalled {
		t.Fatalf("a call after Blender closed: %+v", ref)
	}
}

// An exec call asks unless the person agreed to the agent's code for that
// application; a run's bind never carries the agreement.
func TestAnAgentsExecAgreementSkipsTheAsk(t *testing.T) {
	r := newRig(t)
	r.answer = ExecDecline
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "R", Ref: "agentry/assistant@1.20.0", Agent: "Assistant", Lazy: true,
		Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"execute_blender_code"}, ExecAgreed: true}}})
	if ref != nil {
		t.Fatal(ref)
	}
	if _, ref := call(r, b.Token, "blender", "execute_blender_code", `{"code":"print(1)"}`); ref != nil || len(r.asked) != 0 {
		t.Fatalf("an agreed exec call: %+v, asked %d", ref, len(r.asked))
	}
	r2 := newRig(t)
	b2, ref := r2.s.Bind(context.Background(), desktop.BindRequest{Run: "R", Ref: "acme/scene@1.0.0",
		Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"execute_blender_code"}, ExecAgreed: true}}})
	if ref != nil {
		t.Fatal(ref)
	}
	if _, ref := call(r2, b2.Token, "blender", "execute_blender_code", `{"code":"print(1)"}`); ref == nil || len(r2.asked) != 1 {
		t.Fatalf("a run's exec call with the flag set: %+v, asked %d", ref, len(r2.asked))
	}
}

// A bind that names an agent but is not lazy (a run over the link socket)
// never carries the agreement.
func TestOnlyALazyAgentBindCarriesTheExecAgreement(t *testing.T) {
	r := newRig(t)
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{Run: "R", Ref: "acme/scene@1.0.0", Agent: "Someone",
		Servers: []desktop.BindServer{{ID: "blender", Tools: []string{"execute_blender_code"}, ExecAgreed: true}}})
	if ref != nil {
		t.Fatal(ref)
	}
	if _, ref := call(r, b.Token, "blender", "execute_blender_code", `{"code":"print(1)"}`); ref == nil || len(r.asked) != 1 {
		t.Fatalf("a non-lazy agent bind's exec call: %+v, asked %d", ref, len(r.asked))
	}
}
