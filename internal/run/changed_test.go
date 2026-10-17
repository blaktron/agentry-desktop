package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"

	"github.com/blaktron/agentry-desktop/internal/install"
)

// installedFixture materializes the fixture release once, as a first run
// would, and returns its signature and release directory.
func installedFixture(t *testing.T) (*fakeRunRegistry, *Signature, string, string) {
	t.Helper()
	home := isolateRunEnv(t)
	fr := &fakeRunRegistry{}
	regURL := startRunRegistry(t, fr)
	sig := resolveFixture(t, regURL)
	var log strings.Builder
	_, installDir, err := materialize(context.Background(), sig, home, true, false, &log)
	if err != nil {
		t.Fatal(err)
	}
	return fr, sig, home, installDir
}

// wantChanged asserts err is the verify-class refusal of a changed registry
// install, naming the release and the paths.
func wantChanged(t *testing.T, err error, ref string, changed, added []string) {
	t.Helper()
	var classified *engine.ClassifiedError
	if !errors.As(err, &classified) || classified.Code != engine.ExitVerify {
		t.Fatalf("err = %v, want the verify class", err)
	}
	var ce *ChangedInstallError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want a changed install", err)
	}
	if ce.Ref != ref || !slices.Equal(ce.Changed, changed) || !slices.Equal(ce.Added, added) {
		t.Fatalf("changed install = %+v, want %s changed %v added %v", ce.ChangedError, ref, changed, added)
	}
	msg := err.Error()
	if !strings.Contains(msg, "Repair, in the run setup") || strings.Contains(msg, "agentry install --repair") {
		t.Fatalf("the refusal must name the desktop's way back, not the CLI's: %s", msg)
	}
	for _, p := range append(slices.Clone(changed), added...) {
		if !strings.Contains(msg, p) {
			t.Fatalf("the refusal must name %s: %s", p, msg)
		}
	}
}

func TestAChangedInstallIsRefusedBeforeAnyDownload(t *testing.T) {
	fr, sig, home, installDir := installedFixture(t)
	if err := os.WriteFile(filepath.Join(installDir, "workspace", "SKILL.md"), []byte("edited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "workspace", "extra.md"), []byte("added\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	_, _, err := materialize(context.Background(), sig, home, true, false, &log)
	wantChanged(t, err, "acme/reporter@1.0.0", []string{"SKILL.md"}, []string{"extra.md"})
	if got := fr.hitCount("/archive.tar.zst"); got != 1 {
		t.Fatalf("a changed install was refused after %d downloads, want the first install's one", got)
	}
	// Nothing was replaced: the edit is still there for the person to see.
	if b, _ := os.ReadFile(filepath.Join(installDir, "workspace", "SKILL.md")); string(b) != "edited\n" {
		t.Fatalf("the refusal replaced the changed file: %q", b)
	}

	// The run setup's check reads the same refusal, every path included.
	ce, err := InstalledChanges(context.Background(), sig.Registry, sig.Ref)
	if err != nil || ce == nil || ce.Ref != sig.Ref || !slices.Equal(ce.Changed, []string{"SKILL.md"}) || !slices.Equal(ce.Added, []string{"extra.md"}) {
		t.Fatalf("InstalledChanges = %+v, %v", ce, err)
	}

	// Repair is the install of this exact release: its plan names the
	// changed copy, confirming it lays the release down again, and the next
	// run uses it.
	e := install.NewEngine()
	plan, err := e.Prepare(context.Background(), sig.Registry, sig.Ref, installdir.Target, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changed == nil {
		t.Fatal("the repair's plan does not name the changed copy")
	}
	if _, err := e.Confirm(plan.ID, true, true, false); err != nil {
		t.Fatal(err)
	}
	if ce, err := InstalledChanges(context.Background(), sig.Registry, sig.Ref); err != nil || ce != nil {
		t.Fatalf("after the repair: %+v, %v", ce, err)
	}
	log.Reset()
	if _, _, err := materialize(context.Background(), sig, home, true, false, &log); err != nil {
		t.Fatalf("the run after the repair: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "workspace", "extra.md")); !os.IsNotExist(err) {
		t.Fatal("the repair kept the added file")
	}
}

func TestAWholeInstallHasNoChanges(t *testing.T) {
	_, sig, _, _ := installedFixture(t)
	ce, err := InstalledChanges(context.Background(), sig.Registry, sig.Ref)
	if err != nil || ce != nil {
		t.Fatalf("a whole install reads as changed: %+v, %v", ce, err)
	}
}

func TestTheRecheckBeforeTheCompileRefusesAnEditMadeMeanwhile(t *testing.T) {
	_, sig, _, installDir := installedFixture(t)
	if err := recheckInstall(sig, installDir); err != nil {
		t.Fatalf("a whole install was refused: %v", err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "workspace", "SKILL.md"), []byte("edited while the setup was open\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantChanged(t, recheckInstall(sig, installDir), "acme/reporter@1.0.0", []string{"SKILL.md"}, nil)
}

func TestARunsCopyChangedAfterTheCheckIsRefused(t *testing.T) {
	_, sig, _, installDir := installedFixture(t)
	check := copyCheck(sig, installDir)

	whole := t.TempDir()
	if err := os.CopyFS(whole, os.DirFS(filepath.Join(installDir, "workspace"))); err != nil {
		t.Fatal(err)
	}
	if err := check(whole); err != nil {
		t.Fatalf("a faithful copy was refused: %v", err)
	}

	edited := t.TempDir()
	if err := os.CopyFS(edited, os.DirFS(filepath.Join(installDir, "workspace"))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(edited, "SKILL.md"), []byte("edited between the check and the copy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := check(edited)
	var ce *ChangedInstallError
	if !errors.As(err, &ce) || ce.Ref != sig.Ref || !slices.Equal(ce.Changed, []string{"SKILL.md"}) {
		t.Fatalf("a changed copy = %v, want refused naming SKILL.md", err)
	}
}

func TestAnInstallWithNoRecordIsFetchedAgainKeepingItsExport(t *testing.T) {
	fr, sig, home, installDir := installedFixture(t)
	// Lay it down as the install path does, with an export, then take the
	// record away, as an older client left it.
	m, err := manifestFromWorkspace(filepath.Join(installDir, "workspace"))
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.CopyFS(src, os.DirFS(filepath.Join(installDir, "workspace"))); err != nil {
		t.Fatal(err)
	}
	meta, err := installdir.Inspect(installDir, sig.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := installdir.Repair(installDir, src, m, meta, manifest.InterfaceAgentsMD); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(installDir, installdir.RecordFile)); err != nil {
		t.Fatal(err)
	}

	var log strings.Builder
	if _, _, err := materialize(context.Background(), sig, home, true, false, &log); err != nil {
		t.Fatal(err)
	}
	if got := fr.hitCount("/archive.tar.zst"); got != 2 {
		t.Fatalf("an install with no record was downloaded %d times in all, want fetched again (2)", got)
	}
	if !strings.Contains(log.String(), "no integrity record") {
		t.Errorf("the run must say why it fetched again:\n%s", log.String())
	}
	again, err := installdir.Inspect(installDir, sig.Digest)
	if err != nil {
		t.Fatalf("the install laid down again is not sealed: %v", err)
	}
	if again.Export != manifest.InterfaceAgentsMD {
		t.Fatalf("the export = %q, want the one it had (%s)", again.Export, manifest.InterfaceAgentsMD)
	}
}
