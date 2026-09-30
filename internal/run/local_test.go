package run

// A QuickAgent installed from a folder on this computer (the CLI's DR-52;
// agentry-desktop#425): the desktop's run finds it before any registry,
// holds it to its integrity record, asks the unsigned consent on every run,
// and keeps its record local when it records the run's consent.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/localinstall"
)

// installLocalFixture lays the fixture release down from a folder, as the
// Build tab's Install does, and returns the installed release directory.
func installLocalFixture(t *testing.T, home string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"agent.yaml": runManifestYAML,
		"SKILL.md":   "# Reporter\n\nWrite the report.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := localinstall.Stage(dir, extractLimits())
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	defer func() { _ = s.Close() }()
	rel, err := s.Install(home, "agents-md")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	return rel
}

// totalHits is every request the fake registry served.
func totalHits(f *fakeRunRegistry) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.hits {
		n += c
	}
	return n
}

func TestRunOfALocalInstallAsksNoRegistry(t *testing.T) {
	fr, regURL, _, home := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	installLocalFixture(t, home)

	var log strings.Builder
	res, err := Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &log)
	if err != nil {
		t.Fatalf("Run: %v\nlog:\n%s", err, log.String())
	}
	if res.Status != "ok" || res.AttestationState != "local" || res.Ref != "acme/reporter@1.0.0" {
		t.Fatalf("view = %+v", res)
	}
	if n := totalHits(fr); n != 0 {
		t.Errorf("the registry served %d requests for a local install: %v", n, fr.hits)
	}
	if !strings.Contains(log.String(), "installed from a folder on this computer") {
		t.Errorf("log does not say the release is local:\n%s", log.String())
	}
	// The run's consent keeps the record the local install's.
	st, err := installed.Load()
	if err != nil {
		t.Fatal(err)
	}
	rec := st.Find(installdir.ConsentKey(home, "acme", "reporter"))
	if rec == nil || !rec.Local() || !rec.Managed || rec.Digest == "" {
		t.Errorf("record after the run = %+v, want the local, managed install", rec)
	}
}

func TestRunOfALocalInstallNeedsTheUnsignedConsent(t *testing.T) {
	_, regURL, _, home := runFixture(t)
	installLocalFixture(t, home)
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	req.AcceptUnsigned = false

	_, err := Run(context.Background(), req, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "local and unsigned") {
		t.Fatalf("Run without the unsigned consent = %v, want the local refusal", err)
	}
}

func TestAChangedLocalInstallIsRefusedWithItsFolderAsTheWayBack(t *testing.T) {
	_, regURL, _, home := runFixture(t)
	rel := installLocalFixture(t, home)
	if err := os.WriteFile(filepath.Join(rel, "workspace", "SKILL.md"), []byte("# Reporter\n\nDo something else.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "changed SKILL.md") || !strings.Contains(err.Error(), localRepair) {
		t.Fatalf("Run of a changed local install = %v, want the refusal naming the Build tab's Install", err)
	}
}

func TestARangeOrAnotherVersionAsksTheRegistry(t *testing.T) {
	fr, regURL, _, home := runFixture(t)
	installLocalFixture(t, home)

	for _, ref := range []string{"acme/reporter@^1.0", "acme/reporter@beta", "acme/reporter@2.0.0"} {
		sig, err := resolveForRun(context.Background(), regURL, ref, false)
		if sig != nil && sig.Local() {
			t.Errorf("%s resolved to the local install", ref)
		}
		_ = err // the fake registry serves 1.0.0 only; what matters is that it was asked
	}
	if totalHits(fr) == 0 {
		t.Error("the registry was never asked")
	}
	sig, err := resolveForRun(context.Background(), regURL, "acme/reporter", false)
	if err != nil || !sig.Local() {
		t.Fatalf("acme/reporter = %+v, %v; want the local install", sig, err)
	}
}
