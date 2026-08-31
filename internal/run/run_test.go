package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/profile"
)

// testSig builds a Signature without a registry round-trip.
func testSig(t *testing.T, secrets []string) *Signature {
	t.Helper()
	return &Signature{
		Registry:  "http://registry.test",
		Ref:       "acme/reporter@1.2.3",
		Namespace: "acme",
		Name:      "reporter",
		Version:   "1.2.3",
		Digest:    "sha256:aaaabbbbccccdddd0000111122223333aaaabbbbccccdddd0000111122223333",
		Manifest:  &manifest.Manifest{Capabilities: manifest.Capabilities{Secrets: secrets}},
	}
}

func withRunnerHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	return home
}

func TestDigestSuffix(t *testing.T) {
	if got := digestSuffix("sha256:0123456789abcdef0123"); got != "0123456789ab" {
		t.Fatalf("digestSuffix = %q", got)
	}
	if got := digestSuffix("short"); got != "short" {
		t.Fatalf("digestSuffix(short) = %q", got)
	}
}

// TestSaveProfileDropsEmptyValues pins §P3: an empty form field means
// "resolve from the environment at run time" and is recorded as ABSENT —
// never as an empty string a run would then trust.
func TestSaveProfileDropsEmptyValues(t *testing.T) {
	home := withRunnerHome(t)
	sig := testSig(t, []string{"EXAMPLE_TOKEN", "OTHER_TOKEN"})

	info, err := saveProfileForSig(sig, map[string]string{"EXAMPLE_TOKEN": "s3cret", "OTHER_TOKEN": ""}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Exists || !info.Keep {
		t.Fatalf("info = %+v, want exists+keep", info)
	}
	if len(info.StoredSecretNames) != 1 || info.StoredSecretNames[0] != "EXAMPLE_TOKEN" {
		t.Fatalf("stored names = %v, want [EXAMPLE_TOKEN]", info.StoredSecretNames)
	}

	dir := profile.Dir(home, "acme", "reporter", "1.2.3", sig.DigestSuffix())
	prof, err := profile.Load(dir)
	if err != nil || prof == nil {
		t.Fatalf("load profile: %v %v", prof, err)
	}
	if _, ok := prof.Secrets["OTHER_TOKEN"]; ok {
		t.Fatal("empty value must be recorded as absent, not empty")
	}
	if prof.Secrets["EXAMPLE_TOKEN"] != "s3cret" {
		t.Fatal("stored value lost")
	}
	// dir 0700, file 0600 (§P2).
	if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("profile dir perms = %v %v, want 0700", fi.Mode().Perm(), err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "profile.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("profile file perms = %v %v, want 0600", fi.Mode().Perm(), err)
	}
}

// TestSaveProfileRejectsUndeclaredName: the form cannot store keys the
// contract never declared — the profile is the credentialed half of the
// DECLARED contract, nothing wider.
func TestSaveProfileRejectsUndeclaredName(t *testing.T) {
	withRunnerHome(t)
	sig := testSig(t, []string{"EXAMPLE_TOKEN"})
	if _, err := saveProfileForSig(sig, map[string]string{"SNEAKY": "v"}, true); err == nil {
		t.Fatal("undeclared secret name must be refused")
	} else if !strings.Contains(err.Error(), "SNEAKY") {
		t.Fatalf("error %q should name the refused variable", err)
	}
}

// TestSaveProfilePreservesCreatedAt: reconfiguring keeps the original
// audit stamp (the CLI's ensureProfile does the same).
func TestSaveProfilePreservesCreatedAt(t *testing.T) {
	home := withRunnerHome(t)
	sig := testSig(t, []string{"EXAMPLE_TOKEN"})
	dir := profile.Dir(home, "acme", "reporter", "1.2.3", sig.DigestSuffix())

	first := profile.New(sig.Ref, sig.Digest, true, map[string]string{"EXAMPLE_TOKEN": "old"})
	first.CreatedAt = "2026-01-02T03:04:05Z"
	if err := first.Save(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := saveProfileForSig(sig, map[string]string{"EXAMPLE_TOKEN": "new"}, false); err != nil {
		t.Fatal(err)
	}
	prof, err := profile.Load(dir)
	if err != nil || prof == nil {
		t.Fatalf("load: %v %v", prof, err)
	}
	if prof.CreatedAt != "2026-01-02T03:04:05Z" {
		t.Fatalf("CreatedAt = %q, want the original stamp", prof.CreatedAt)
	}
	if prof.Keep {
		t.Fatal("keep=false must persist")
	}
}

// TestFinishProfile pins §P2 completion: keep stamps lastUsedAt, remove
// deletes the directory.
func TestFinishProfile(t *testing.T) {
	home := withRunnerHome(t)
	sig := testSig(t, []string{"EXAMPLE_TOKEN"})
	dir := profile.Dir(home, "acme", "reporter", "1.2.3", sig.DigestSuffix())
	prof := profile.New(sig.Ref, sig.Digest, true, map[string]string{"EXAMPLE_TOKEN": "v"})
	if err := prof.Save(dir); err != nil {
		t.Fatal(err)
	}

	var log strings.Builder
	finishProfile(&log, prof, dir, true)
	kept, err := profile.Load(dir)
	if err != nil || kept == nil {
		t.Fatalf("keep=true must retain the profile: %v %v", kept, err)
	}
	if kept.LastUsedAt == "" {
		t.Fatal("keep must stamp lastUsedAt")
	}

	finishProfile(&log, prof, dir, false)
	if gone, err := profile.Load(dir); err != nil || gone != nil {
		t.Fatalf("keep=false must remove the profile: %v %v", gone, err)
	}
}

// TestInstallMetaRoundTrip pins the shared cache contract: install.json
// written here is reused only when the digest still matches (the CLI's
// rule, byte-compatible layout).
func TestInstallMetaRoundTrip(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "agent.yaml"), []byte("x: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	installDir := filepath.Join(root, "agents", "acme", "reporter", "1.2.3-aaaabbbbcccc")
	meta := &installMeta{
		Ref: "acme/reporter@1.2.3", Namespace: "acme", Name: "reporter",
		Version: "1.2.3", Digest: "sha256:abc", Source: "registry",
	}
	if err := writeInstall(installDir, src, meta); err != nil {
		t.Fatal(err)
	}
	got, ok := loadInstallMeta(installDir, "sha256:abc")
	if !ok || got.Ref != meta.Ref || got.Source != "registry" {
		t.Fatalf("reuse failed: %+v %v", got, ok)
	}
	if _, ok := loadInstallMeta(installDir, "sha256:different"); ok {
		t.Fatal("a digest mismatch must not reuse the cached install")
	}
}

// TestResolveOutDirDefault pins the desktop default output location.
func TestResolveOutDirDefault(t *testing.T) {
	sig := testSig(t, nil)
	got, err := resolveOutDir("", sig, "cafe0123")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(got) }()
	if !strings.HasSuffix(got, filepath.Join("Downloads", "agentry-out", "acme-reporter-cafe0123")) {
		t.Fatalf("default out dir = %q", got)
	}
	if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
		t.Fatalf("out dir not created: %v", err)
	}
}

// TestFormPrompterNeverSuppliesSecrets: the desktop's Prompter keeps
// defaults and never invents secret values.
func TestFormPrompterNeverSuppliesSecrets(t *testing.T) {
	var log strings.Builder
	p := &formPrompter{log: &log}
	if v, _ := p.Ask("q", "def"); v != "def" {
		t.Fatalf("Ask = %q", v)
	}
	if v, _ := p.AskSecret("q"); v != "" {
		t.Fatalf("AskSecret = %q, must be empty (leave unset)", v)
	}
	if v, _ := p.Confirm("q", true); v != true {
		t.Fatal("Confirm must keep the default")
	}
	p.Say("hello")
	if !strings.Contains(log.String(), "hello") {
		t.Fatal("Say must reach the log")
	}
}
