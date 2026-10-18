package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	kbuild "github.com/blaktron/agentry-cli/pkg/knowledge/build"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/schedules"
)

// A path the page names is judged with its links resolved: a link inside a
// picked folder that leads out of it names nothing the person chose.

// linkedOut returns a picked folder holding "link", a link to a folder
// outside it, and the path of a key file reached through that link.
func linkedOut(t *testing.T) (picked, through string) {
	t.Helper()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "id_rsa"), []byte("not for upload"), 0o600); err != nil {
		t.Fatal(err)
	}
	picked = t.TempDir()
	if err := os.Symlink(outside, filepath.Join(picked, "link")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	return picked, filepath.Join(picked, "link", "id_rsa")
}

func TestAPickedPathIsJudgedWithItsLinksResolved(t *testing.T) {
	app := &App{ctx: context.Background()}
	picked, through := linkedOut(t)
	app.markPicked(picked)
	if err := os.Mkdir(filepath.Join(picked, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), filepath.Join(picked, "gone")); err != nil {
		t.Fatal(err)
	}
	sep := string(filepath.Separator)
	refused := map[string]string{
		"a file through the link":                                  through,
		"the link itself":                                          filepath.Join(picked, "link"),
		"a file not made yet under the link":                       filepath.Join(picked, "link", "new", "out.txt"),
		"a path with a .. element":                                 picked + sep + "sub" + sep + ".." + sep + "a.txt",
		"a link whose target is missing":                           filepath.Join(picked, "gone"),
		"a file not made yet under a link whose target is missing": filepath.Join(picked, "gone", "out.txt"),
	}
	for name, p := range refused {
		if err := app.requirePicked("input", p); err == nil {
			t.Errorf("%s (%s) passed", name, p)
		}
	}
	if err := app.requirePicked("input", through); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("a file through the link: %v", err)
	}

	// A link that stays inside the picked folder, and a name not made yet
	// under it, both pass.
	if err := os.Symlink(filepath.Join(picked, "sub"), filepath.Join(picked, "inner")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(picked, "inner", "a.txt"), filepath.Join(picked, "new", "out.txt"), filepath.Join(picked, "sub")} {
		if err := app.requirePicked("input", p); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}

	// A folder picked through a linked home directory admits what lies
	// under it, since both sides are resolved.
	realHome := t.TempDir()
	if err := os.MkdirAll(filepath.Join(realHome, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realHome, "docs", "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkedHome := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(realHome, linkedHome); err != nil {
		t.Fatal(err)
	}
	app.markPicked(filepath.Join(linkedHome, "docs"))
	if err := app.requirePicked("input", filepath.Join(linkedHome, "docs", "a.txt")); err != nil {
		t.Fatalf("a file under a folder picked through a linked home: %v", err)
	}

	// A link the person picked is what they chose.
	app.markPicked(filepath.Join(picked, "link"))
	if err := app.requirePicked("input", filepath.Join(picked, "link")); err != nil {
		t.Fatalf("a picked link: %v", err)
	}
}

func TestTheBindingsRefuseALinkOutOfAPickedFolder(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
	r := &RunAPI{app: app}
	b := &BuildAPI{app: app}
	picked, through := linkedOut(t)
	app.markPicked(picked)
	linkDir := filepath.Join(picked, "link")
	for name, call := range map[string]func() error{
		"RunAgent inputs": func() error {
			_, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", []string{through}, "", nil, 0, 0, false, nil, false, false, nil, false, nil, nil, nil)
			return err
		},
		"RunAgent outDir": func() error {
			_, err := r.RunAgent("https://r.example", "acme/x@1.0.0", "go", nil, linkDir, nil, 0, 0, false, nil, false, false, nil, false, nil, nil, nil)
			return err
		},
		"StartSession inputs": func() error {
			_, err := r.StartSession("https://r.example", "acme/x@1.0.0", "go", []string{through}, "", nil, 0, false, nil, false, false, nil, false, nil, nil, nil)
			return err
		},
		"StartSession outDir": func() error {
			_, err := r.StartSession("https://r.example", "acme/x@1.0.0", "go", nil, linkDir, nil, 0, false, nil, false, false, nil, false, nil, nil, nil)
			return err
		},
		"BuildAPI.Save":     func() error { return b.Save(linkDir, build.Model{}) },
		"BuildAPI.Submit":   func() error { _, err := b.Submit("https://r.example", linkDir); return err },
		"BuildAPI.Install":  func() error { _, err := b.Install(linkDir); return err },
		"BuildAPI.Validate": func() error { _, err := b.Validate(linkDir); return err },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "through a link") {
			t.Errorf("%s through a link out of the picked folder: err = %v, want the link refusal", name, err)
		}
	}
}

func TestAHostedRunSendsNothingThroughALink(t *testing.T) {
	f := &hostedFake{enabled: true}
	reg := f.start(t)
	signIn(t, reg)
	r, _ := newRunAPI(t)
	picked, through := linkedOut(t)
	out := t.TempDir()
	r.app.markPicked(picked, out)
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", []string{through}, out, nil, nil, false, false, nil); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("an input through a link: %v", err)
	}
	if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, filepath.Join(picked, "link"), nil, nil, false, false, nil); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("an output folder through a link: %v", err)
	}
	if _, err := r.AttachHostedRun(reg, filepath.Join(picked, "link")); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("a reattach into a folder through a link: %v", err)
	}
	f.mu.Lock()
	created := f.created
	f.mu.Unlock()
	if created != nil {
		t.Fatalf("a run was created: %+v", created)
	}
}

func TestAKnowledgeBuildReadsNothingThroughALink(t *testing.T) {
	f := newKnowledgeFixture(t)
	built := false
	f.k.build = func(context.Context, dockerx.Client, kbuild.Options) (*kbuild.Result, error) {
		built = true
		return nil, errors.New("not reached")
	}
	picked, through := linkedOut(t)
	f.k.app.markPicked(picked)
	if _, err := f.k.Create(f.reg.URL, "notes", []string{through}, "", false); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("create through a link: %v", err)
	}
	if _, err := f.k.Add(f.reg.URL, "handbook", []string{through}, "", false); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("add through a link: %v", err)
	}
	if built {
		t.Fatal("a build ran on a file reached through a link")
	}
}

func TestASchedulePicksNothingThroughALink(t *testing.T) {
	f := &scheduleFake{check: readyCheck, answer: true}
	s := newScheduleAPI(t, f, true)
	picked, through := linkedOut(t)
	out := t.TempDir()
	s.app.markPicked(picked, out)
	req := scheduleReq(out)
	req.Files = []string{through}
	if _, err := s.Check(req); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("a scheduled input through a link: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("the command ran: %q", f.calls)
	}

	sr := &scheduleRoutes{}
	hs, reg, hf := hostedScheduleAPI(t, sr, nil)
	hs.app.markPicked(picked)
	hreq := HostedScheduleRequest{Registry: reg, Ref: "acme/reporter@1.0.0", Task: "sum up", Files: []string{through},
		When: schedules.When{Kind: "daily", Time: "09:00", Zone: "Europe/London"}}
	if _, err := hs.HostedAdd(hreq); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("a hosted schedule's file through a link: %v", err)
	}
	hf.mu.Lock()
	uploads := hf.uploads
	hf.mu.Unlock()
	if len(sr.calls()) != 0 || uploads != 0 {
		t.Fatalf("sent %v, uploads %d", sr.calls(), uploads)
	}
}

func TestTheVoiceReviewAndRevisionRefuseALink(t *testing.T) {
	picked, through := linkedOut(t)
	linkDir := filepath.Join(picked, "link")

	v := newVoiceFixture(t, false)
	v.v.app.markPicked(picked)
	if _, err := v.v.Write(v.reg.URL, "create", []string{through}, ""); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("a writing sample through a link: %v", err)
	}

	rv := newReviewFixture(t)
	rv.withKey(t)
	rv.b.app.markPicked(picked)
	if _, err := rv.b.ReviewLocally(rv.reg, linkDir); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("a review of a folder through a link: %v", err)
	}
	if len(rv.asked) != 0 || len(rv.runs) != 0 {
		t.Fatalf("asked %v, ran %d", rv.asked, len(rv.runs))
	}

	b := &BuildAPI{app: &App{ctx: context.Background()}}
	b.app.markPicked(picked)
	if _, err := b.PublishThisVersion("https://r.example", linkDir, "k"); err == nil || !strings.Contains(err.Error(), "through a link") {
		t.Fatalf("a revision saved through a link: %v", err)
	}
}
