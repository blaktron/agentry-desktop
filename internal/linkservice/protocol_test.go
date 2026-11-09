package linkservice

import (
	"context"
	"slices"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
)

// A release that does not narrow an application's tools binds with none
// named: the run is given every tool its server lists, eagerly and lazily,
// and never one the person withholds. A narrowing binds as before.
func TestAnEmptyNarrowingBindsEveryOfferedTool(t *testing.T) {
	r := newRig(t)
	e := blenderApp(t)
	var want []string
	for _, tool := range e.Tools {
		if !tool.Withheld {
			want = append(want, tool.Name)
		}
	}
	b := bindBlender(t, r)
	var got []string
	for _, tool := range b.Servers["blender"] {
		got = append(got, tool.Name)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) || slices.Contains(got, "disable_telemetry") {
		t.Fatalf("an unnarrowed bind gave %v, want the server's tools less those withheld %v", got, want)
	}

	lazy := newRig(t)
	lb, ref := lazy.s.Bind(context.Background(), desktop.BindRequest{Run: "S", Agent: "acme.helper", Lazy: true, Servers: []desktop.BindServer{{ID: "blender"}}})
	if ref != nil || len(lb.Servers["blender"]) != len(want) {
		t.Fatalf("a lazy unnarrowed bind: %+v, %+v", lb.Servers["blender"], ref)
	}

	narrow := newRig(t)
	if nb := bindBlender(t, narrow, "look"); len(nb.Servers["blender"]) != 1 || nb.Servers["blender"][0].Name != "look" {
		t.Fatalf("a narrowed bind gave %+v", nb.Servers["blender"])
	}
}

// A bind that names the link protocol's revision is answered with where
// each application's set-up came from and each tool's class; one that
// names none is answered without them, since a run of an earlier revision
// decodes the answer strictly.
func TestTheProtocolRevisionGetsFromAndClass(t *testing.T) {
	r := newRig(t)
	b, ref := r.s.Bind(context.Background(), desktop.BindRequest{Protocol: desktop.Protocol, Run: "R", Servers: []desktop.BindServer{{ID: "blender"}}})
	if ref != nil {
		t.Fatal(ref)
	}
	if b.From["blender"] != desktop.FromLibrary {
		t.Fatalf("from %+v", b.From)
	}
	classes := map[string]string{}
	for _, tool := range b.Servers["blender"] {
		classes[tool.Name] = tool.Class
	}
	if classes["execute_blender_code"] != "exec" || classes["get_scene_info"] != "read" || classes["import_asset"] != "act" {
		t.Fatalf("classes %v", classes)
	}

	old := newRig(t)
	ob := bindBlender(t, old)
	if ob.From != nil {
		t.Fatalf("a bind without the revision got from %+v", ob.From)
	}
	for _, tool := range ob.Servers["blender"] {
		if tool.Class != "" {
			t.Fatalf("a bind without the revision got the class of %s", tool.Name)
		}
	}
}

// The listing is the person's applications, where each came from, its
// server as approved, its windows as approved while computer use is on, and
// whether each is ready, with the step when not.
func TestTheListingIsThePersonsApplications(t *testing.T) {
	r := newRig(t)
	on := false
	r.s.opt.Computer.Enabled = func() bool { return on }
	var l desktop.Lister = r.s
	got, ref := l.List(context.Background())
	if ref != nil {
		t.Fatal(ref)
	}
	var ids []string
	for _, a := range got.Applications {
		ids = append(ids, a.ID)
		if a.From != desktop.FromLibrary || !a.MCP || a.Computer || !a.Ready || a.Step != "" || a.Name == "" {
			t.Errorf("listed %+v", a)
		}
	}
	if !slices.Contains(ids, "blender") {
		t.Fatalf("listed %v", ids)
	}
	on = true
	r.s.opt.Listening = func(context.Context, int) error { return context.DeadlineExceeded }
	got, _ = l.List(context.Background())
	for _, a := range got.Applications {
		if a.ID == "blender" && (!a.Computer || a.Ready || a.Step == "") {
			t.Errorf("with computer use on and Blender not listening: %+v", a)
		}
	}
}

// A run of the app's own binds in process and hands the engine what the
// socket would: where each application's set-up came from and each tool's
// class, so the receipt names a library application as the library's and
// counts its calls by class.
func TestAnInProcessBindingCarriesFromAndClass(t *testing.T) {
	r := newRig(t)
	link, err := r.s.BindInProcess(context.Background(), desktop.BindRequest{Protocol: desktop.Protocol, Run: "R", Servers: []desktop.BindServer{{ID: "blender"}}})
	if err != nil {
		t.Fatal(err)
	}
	if link.From["blender"] != desktop.FromLibrary || desktop.SetByPerson(link.From["blender"]) {
		t.Fatalf("the in-process binding's from: %+v", link.From)
	}
	for _, tool := range link.Servers["blender"] {
		if tool.Class == "" {
			t.Fatalf("the in-process binding gave %s no class", tool.Name)
		}
	}
}
