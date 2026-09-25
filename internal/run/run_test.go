package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
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

// TestHome pins the CLI's exact derivation: $AGENTRY_RUNNER_HOME wins,
// otherwise ~/.agentry/runner — one cache for both clients.
func TestHome(t *testing.T) {
	t.Setenv("AGENTRY_RUNNER_HOME", "/custom/runner")
	if got, err := Home(); err != nil || got != "/custom/runner" {
		t.Fatalf("Home = %q %v", got, err)
	}
	t.Setenv("AGENTRY_RUNNER_HOME", "")
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if got, err := Home(); err != nil || got != filepath.Join(tmp, ".agentry", "runner") {
		t.Fatalf("Home = %q %v, want the CLI default under $HOME", got, err)
	}
	t.Setenv("HOME", "")
	if _, err := Home(); err == nil || !strings.Contains(err.Error(), "AGENTRY_RUNNER_HOME") {
		t.Fatalf("err = %v, want a hint naming the override", err)
	}
}

func TestDigestSuffix(t *testing.T) {
	if got := installdir.DigestSuffix("sha256:0123456789abcdef0123"); got != "0123456789ab" {
		t.Fatalf("digestSuffix = %q", got)
	}
	if got := installdir.DigestSuffix("short"); got != "short" {
		t.Fatalf("installdir.DigestSuffix(short) = %q", got)
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
	meta := &installdir.Meta{
		Ref: "acme/reporter@1.2.3", Namespace: "acme", Name: "reporter",
		Version: "1.2.3", Digest: "sha256:abc", Source: "registry",
	}
	if err := installdir.Write(installDir, src, meta); err != nil {
		t.Fatal(err)
	}
	got, ok := installdir.Load(installDir, "sha256:abc")
	if !ok || got.Ref != meta.Ref || got.Source != "registry" {
		t.Fatalf("reuse failed: %+v %v", got, ok)
	}
	if _, ok := installdir.Load(installDir, "sha256:different"); ok {
		t.Fatal("a digest mismatch must not reuse the cached install")
	}
}

// TestResolveOutDirDefault pins the desktop default output location.
func TestResolveOutDirDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // never touch the real ~/Downloads
	sig := testSig(t, nil)
	got, err := resolveOutDir("", sig, "cafe0123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, filepath.Join("Downloads", "agentry-out", "acme-reporter-cafe0123")) {
		t.Fatalf("default out dir = %q", got)
	}
	if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
		t.Fatalf("out dir not created: %v", err)
	}
}

// TestResolveOutDirExplicit: a picked folder is absolutized and created.
func TestResolveOutDirExplicit(t *testing.T) {
	sig := testSig(t, nil)
	dir := filepath.Join(t.TempDir(), "picked", "out")
	got, err := resolveOutDir(dir, sig, "cafe0123")
	if err != nil || got != dir {
		t.Fatalf("resolveOutDir = %q %v, want %q", got, err, dir)
	}
	if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
		t.Fatalf("out dir not created: %v", err)
	}
}

// TestResolveOutDirNoHome: without a home directory the default cannot be
// derived and the error tells the user to pick a folder.
func TestResolveOutDirNoHome(t *testing.T) {
	t.Setenv("HOME", "")
	sig := testSig(t, nil)
	if _, err := resolveOutDir("", sig, "cafe0123"); err == nil || !strings.Contains(err.Error(), "output folder") {
		t.Fatalf("err = %v, want the pick-a-folder hint", err)
	}
}

// TestManifestFromWorkspace covers the verified-tree parse and both
// refusals (missing and unparsable agent.yaml).
func TestManifestFromWorkspace(t *testing.T) {
	dir := t.TempDir()
	if _, err := manifestFromWorkspace(dir); err == nil || !strings.Contains(err.Error(), "read agent.yaml") {
		t.Fatalf("err = %v, want the missing-file refusal", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte(":\tnot yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manifestFromWorkspace(dir); err == nil || !strings.Contains(err.Error(), "parse agent.yaml") {
		t.Fatalf("err = %v, want the parse refusal", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.yaml"), []byte(runManifestYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifestFromWorkspace(dir)
	if err != nil || m.Name != "reporter" {
		t.Fatalf("manifest = %+v %v", m, err)
	}
}

// TestProfileInfoReportsPolicyDefaultKeep: with no profile on disk the
// keep toggle starts where the operator policy says (§P2
// profiles.keepByDefault), read WITHOUT creating the policy file.
func TestProfileInfoReportsPolicyDefaultKeep(t *testing.T) {
	isolateRunEnv(t)
	sig := testSig(t, []string{"EXAMPLE_TOKEN"})

	// No policy file: the shipped default (true) applies.
	info, err := profileInfo(sig)
	if err != nil {
		t.Fatal(err)
	}
	if info.Exists || !info.Keep {
		t.Fatalf("info = %+v, want no profile + shipped keep default", info)
	}
	if len(info.DeclaredSecrets) != 1 || info.DeclaredSecrets[0] != "EXAMPLE_TOKEN" {
		t.Fatalf("declared secrets = %v", info.DeclaredSecrets)
	}
	if len(info.CapabilityLines) == 0 {
		t.Fatal("capability lines missing")
	}

	// An operator policy with keepByDefault=false seeds the toggle false.
	path, err := policy.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	pol := policy.Default()
	no := false
	pol.Profiles.KeepByDefault = &no
	if err := policy.Save(path, pol); err != nil {
		t.Fatal(err)
	}
	info, err = profileInfo(sig)
	if err != nil {
		t.Fatal(err)
	}
	if info.Keep {
		t.Fatal("keep toggle must start at the policy default (false)")
	}

	// A broken policy file must not break status surfaces: the read-only
	// loader falls back to the shipped defaults.
	if err := os.WriteFile(path, []byte("not: [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err = profileInfo(sig)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Keep {
		t.Fatal("a broken policy must fall back to the shipped keep default")
	}
}

// TestProfileStatusAndSaveProfile drives the exported §P3 wrappers against
// the fake registry: status without a profile, the guided form's save, and
// status reflecting the stored names afterwards.
func TestProfileStatusAndSaveProfile(t *testing.T) {
	isolateRunEnv(t)
	regURL := startRunRegistry(t, &fakeRunRegistry{})
	ctx := context.Background()

	before, err := ProfileStatus(ctx, regURL, "acme/reporter")
	if err != nil {
		t.Fatal(err)
	}
	if before.Exists || before.Ref != "acme/reporter@1.0.0" {
		t.Fatalf("before = %+v", before)
	}
	if len(before.DeclaredSecrets) != 1 || before.DeclaredSecrets[0] != "RUN_TEST_TOKEN" {
		t.Fatalf("declared secrets = %v", before.DeclaredSecrets)
	}

	saved, err := SaveProfile(ctx, regURL, "acme/reporter", map[string]string{"RUN_TEST_TOKEN": "v"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Exists || saved.Keep {
		t.Fatalf("saved = %+v, want exists with keep=false", saved)
	}

	after, err := ProfileStatus(ctx, regURL, "acme/reporter")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Exists || len(after.StoredSecretNames) != 1 || after.StoredSecretNames[0] != "RUN_TEST_TOKEN" {
		t.Fatalf("after = %+v", after)
	}

	// The guided form refuses names outside the declared contract through
	// the exported path too.
	if _, err := SaveProfile(ctx, regURL, "acme/reporter", map[string]string{"UNDECLARED": "v"}, true); err == nil {
		t.Fatal("undeclared secret name must be refused")
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

// D-012 closed (desktop#7): an explicit keep=true on a release declaring
// no secrets creates the empty profile that records the choice; a nil
// (untouched toggle) or false choice, or a release with secrets (whose
// profile the guided form owns), creates nothing.
func TestEnsureKeepProfileRecordsExplicitChoice(t *testing.T) {
	home := withRunnerHome(t)
	noSecrets := testSig(t, nil)
	dir := profile.Dir(home, "acme", "reporter", "1.2.3", noSecrets.DigestSuffix())
	yes, no := true, false

	if p, err := ensureKeepProfile(dir, noSecrets, nil, nil); err != nil || p != nil {
		t.Fatalf("nil choice must create nothing: %v %v", p, err)
	}
	if p, err := ensureKeepProfile(dir, noSecrets, nil, &no); err != nil || p != nil {
		t.Fatalf("keep=false must create nothing: %v %v", p, err)
	}
	if got, _ := profile.Load(dir); got != nil {
		t.Fatal("profile created without an explicit keep")
	}

	p, err := ensureKeepProfile(dir, noSecrets, nil, &yes)
	if err != nil || p == nil || !p.Keep || len(p.Secrets) != 0 {
		t.Fatalf("explicit keep must create an empty kept profile: %+v %v", p, err)
	}
	stored, err := profile.Load(dir)
	if err != nil || stored == nil || !stored.Keep || stored.Digest != noSecrets.Digest {
		t.Fatalf("stored = %+v %v", stored, err)
	}
	// Existing profile: returned as-is.
	if again, _ := ensureKeepProfile(dir, noSecrets, stored, &yes); again != stored {
		t.Fatal("existing profile must be returned untouched")
	}

	withSecrets := testSig(t, []string{"EXAMPLE_TOKEN"})
	dir2 := profile.Dir(home, "acme", "reporter", "1.2.3", withSecrets.DigestSuffix()+"s")
	if p, err := ensureKeepProfile(dir2, withSecrets, nil, &yes); err != nil || p != nil {
		t.Fatalf("a release with secrets is the guided form's job: %v %v", p, err)
	}
}

// guidedManifestYAML is runManifestYAML with manifest v2's guide block
// (DEV-141): what the desktop's run setup shows a person before a run.
const guidedManifestYAML = runManifestYAML + `guide:
  overview: |
    Writes a **report** from your notes.
  prepare:
    - Your notes, as text
    - A token for the example API
  files: "Your notes."
  task: "e.g. the weekly status"
  connect:
    RUN_TEST_TOKEN: "Create one at https://api.example.com/tokens with read access."
`

// TestProfileStatusCarriesTheGuide (agentry-desktop#228): a release whose
// manifest carries a guide resolves through the strict decoder of the
// pinned core, and ProfileStatus hands the guide to the run setup as the
// author wrote it. A release without one reports none.
func TestProfileStatusCarriesTheGuide(t *testing.T) {
	isolateRunEnv(t)
	regURL := startRunRegistry(t, &fakeRunRegistry{
		manifestDoc: manifestDocFromYAML(t, guidedManifestYAML),
		archive: buildRunArchive(t, map[string]string{
			"agent.yaml": guidedManifestYAML,
			"SKILL.md":   "# Reporter\n\nWrite the report.\n",
		}),
	})
	info, err := ProfileStatus(context.Background(), regURL, "acme/reporter")
	if err != nil {
		t.Fatal(err)
	}
	g := info.Guide
	if g == nil {
		t.Fatal("a guided release must report its guide")
	}
	if g.Overview != "Writes a **report** from your notes.\n" || g.Files != "Your notes." || g.Task != "e.g. the weekly status" {
		t.Fatalf("guide = %+v", g)
	}
	if len(g.Prepare) != 2 || g.Prepare[1] != "A token for the example API" {
		t.Fatalf("prepare = %v", g.Prepare)
	}
	if g.Connect["RUN_TEST_TOKEN"] != "Create one at https://api.example.com/tokens with read access." {
		t.Fatalf("connect = %v", g.Connect)
	}

	isolateRunEnv(t)
	plain, err := ProfileStatus(context.Background(), startRunRegistry(t, &fakeRunRegistry{}), "acme/reporter")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Guide != nil {
		t.Fatalf("a release without a guide reports one: %+v", plain.Guide)
	}
}
