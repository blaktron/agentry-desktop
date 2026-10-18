package app

// Deliverables live, as an end-to-end check: the bound methods driven as the
// page drives them, with the real engine, this machine's docker and a model
// that is a scripted stand-in on the LAN (one marked Markdown report, one
// unmarked note, one marked PDF). Skipped unless AGENTRY_LIVE_DELIVERABLES=1:
//
//	AGENTRY_LIVE_DELIVERABLES=1 \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home whose policy names the stand-in> \
//	HOME=<a scratch home: the Documents folder and the default output folder are under it> \
//	go test -run TestLiveDeliverables -v ./internal/app
//
// It refuses to run on the person's own settings. The registry and release
// default to dev's runs-checker-125725/hosted-smoke@0.1.0
// (AGENTRY_LIVE_DELIVERABLES_REGISTRY, AGENTRY_LIVE_DELIVERABLES_REF).
//
// What it holds: the run is given the Documents folder's Agentry folder; its
// marked files are copied there by the engine with the unmarked note left
// out; the result lists them by digest and a click's file is the copy; Home's
// card lists them; and with the switch off the next run copies nothing.

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/deliverables"
)

func TestLiveDeliverables(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_DELIVERABLES") != "1" {
		t.Skip("runs a real run: set AGENTRY_LIVE_DELIVERABLES=1 (see the file's comment)")
	}
	if os.Getenv(configdir.EnvVar) == "" || os.Getenv(policy.HomeEnv) == "" || os.Getenv("HOME") == "" {
		t.Fatalf("set %s, %s and HOME to scratch directories", configdir.EnvVar, policy.HomeEnv)
	}
	reg := cmp.Or(os.Getenv("AGENTRY_LIVE_DELIVERABLES_REGISTRY"), "https://dev.secureagentry.net")
	ref := cmp.Or(os.Getenv("AGENTRY_LIVE_DELIVERABLES_REF"), "runs-checker-125725/hosted-smoke@0.1.0")

	// The suite points the default folder at a scratch one; this test wants
	// the platform's own Documents folder, which HOME places.
	oldRoot := defaultDeliverablesRoot
	defaultDeliverablesRoot = deliverables.DefaultRoot
	t.Cleanup(func() { defaultDeliverablesRoot = oldRoot })

	old := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if len(data) == 1 {
			if s, ok := data[0].(string); ok {
				t.Logf("[%s] %s", name, s)
			}
		}
	}
	t.Cleanup(func() { emitEvent = old })

	app := &App{ctx: context.Background()}
	r := &RunAPI{app: app}
	var opened []string
	r.openFile = func(p string) error { opened = append(opened, p); return nil }

	root, isDefault, on, err := deliverablesFolder()
	if err != nil || !on || !isDefault {
		t.Fatalf("deliverablesFolder = %q default=%v on=%v: %v", root, isDefault, on, err)
	}
	if !strings.HasPrefix(root, os.Getenv("HOME")) || filepath.Base(root) != "Agentry" {
		t.Fatalf("the default Deliverables folder is %q, want Agentry inside the Documents folder under %s", root, os.Getenv("HOME"))
	}
	t.Logf("the Deliverables folder: %s", root)

	res, err := r.RunAgent(reg, ref, "Write this week's digest.", nil, "", nil, 0, -1, false, nil, false, false, nil, false, nil, nil)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	t.Logf("status %s, exit %d, run %s, folder %s", res.Status, res.ExitCode, res.RunID, res.OutDir)
	if res.Status != "ok" {
		t.Fatalf("the run ended %q: %s", res.Status, res.Error)
	}

	v, err := r.RunDeliverables(res.RunID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]DeliverableView{}
	for _, f := range v.Files {
		got[f.Path] = f
		t.Logf("deliverable %s: title %q, %d bytes, canOpen %v, view %q, copied to %q", f.Path, f.Title, f.Bytes, f.CanOpen, f.ViewKind, f.CopiedTo)
	}
	md, pdf := got["reports/weekly.md"], got["reports/weekly.pdf"]
	if md.Title != "Weekly digest" || !md.CanOpen || md.ViewKind != "markdown" || !md.Marked {
		t.Errorf("the marked Markdown report: %+v", md)
	}
	if pdf.Title != "Weekly digest (PDF)" || !pdf.CanOpen || pdf.ViewKind != "pdf" {
		t.Errorf("the marked PDF: %+v", pdf)
	}
	if _, ok := got["reports/notes.md"]; ok || len(v.Files) != 2 || v.Withheld != 0 {
		t.Errorf("the list holds %d files (withheld %d), want the two marked ones", len(v.Files), v.Withheld)
	}
	for _, f := range []DeliverableView{md, pdf} {
		if f.CopiedTo == "" || !strings.HasPrefix(f.CopiedTo, root) {
			t.Errorf("%s is not copied under %s: %q", f.Path, root, f.CopiedTo)
		}
	}
	if _, err := os.Stat(filepath.Join(res.OutDir, "reports", "notes.md")); err != nil {
		t.Errorf("the run folder lost the unmarked note: %v", err)
	}
	if _, err := os.Stat(filepath.Join(md.CopiedTo, "weekly.md")); err != nil {
		t.Errorf("the copy of the report is not at %s: %v", md.CopiedTo, err)
	}
	if _, err := os.Stat(filepath.Join(md.CopiedTo, "notes.md")); err == nil {
		t.Error("the unmarked note was copied")
	}

	// A click opens the copy, never the run folder's file.
	if err := r.OpenRunDeliverable(res.RunID, "reports/weekly.pdf"); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 || filepath.Dir(opened[0]) != pdf.CopiedTo {
		t.Errorf("Open handed %v to the system, want the copy in %s", opened, pdf.CopiedTo)
	}
	text, err := r.RunDeliverableText(res.RunID, "reports/weekly.md")
	if err != nil || text.Kind != "markdown" || !strings.Contains(text.Text, "Weekly digest") {
		t.Errorf("View of the report: %+v, %v", text, err)
	}
	if b64, err := r.RunDeliverablePDF(res.RunID, "reports/weekly.pdf"); err != nil || len(b64) < 100 {
		t.Errorf("View of the PDF: %d bytes of base64, %v", len(b64), err)
	}

	// Home's card lists the same files.
	card, err := r.RecentDeliverables(5)
	if err != nil || len(card.Files) != 2 || card.Files[0].RunID != res.RunID {
		t.Fatalf("Home's card: %+v, %v", card, err)
	}

	// Off: the next run copies nothing and says so.
	if _, err := (&SettingsAPI{app: app}).SetDeliverables(false); err != nil {
		t.Fatal(err)
	}
	res2, err := r.RunAgent(reg, ref, "Write this week's digest again.", nil, "", nil, 0, -1, false, nil, false, false, nil, false, nil, nil)
	if err != nil || res2.Status != "ok" {
		t.Fatalf("the second run: %+v, %v", res2, err)
	}
	v2, err := r.RunDeliverables(res2.RunID)
	if err != nil || len(v2.Files) != 2 || v2.On {
		t.Fatalf("with the switch off: %+v, %v", v2, err)
	}
	for _, f := range v2.Files {
		if f.CopiedTo != "" {
			t.Errorf("a run with the switch off copied %s to %s", f.Path, f.CopiedTo)
		}
	}
}
