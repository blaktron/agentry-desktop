package run

// Lifecycle tests for the desktop run engine, against the fake registry
// (registryfake_test.go) and the CLI's dockerx.Fake: materialize's
// verified-install cache, ensurePlan's reuse/recompile/tamper rules
// (§R6/§R9), and Run()'s whole resolve → verify → materialize → compile →
// run → receipt pipeline with §P2 profile semantics.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
	"github.com/blaktron/agentry-cli/pkg/runner/compile"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/profile"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"

	"github.com/blaktron/agentry-desktop/internal/version"
)

// resolveFixture resolves the fixture release against a fake registry.
func resolveFixture(t *testing.T, regURL string) *Signature {
	t.Helper()
	sig, err := ResolveSignature(context.Background(), regURL, "acme/reporter@1.0.0", true)
	if err != nil {
		t.Fatal(err)
	}
	return sig
}

// TestResolveSignature pins the identity fields a resolve carries into
// every later stage (§P2: the signature keys the caches and profiles).
func TestResolveSignature(t *testing.T) {
	isolateRunEnv(t)
	fr := &fakeRunRegistry{}
	regURL := startRunRegistry(t, fr)

	sig := resolveFixture(t, regURL)
	if sig.Ref != "acme/reporter@1.0.0" || sig.Namespace != "acme" || sig.Name != "reporter" || sig.Version != "1.0.0" {
		t.Fatalf("signature identity = %+v", sig)
	}
	if sig.Digest != fr.digest || sig.DigestSuffix() != installdir.DigestSuffix(fr.digest) {
		t.Fatalf("digest = %q, want %q", sig.Digest, fr.digest)
	}
	if sig.Yanked {
		t.Fatal("fixture is not yanked")
	}
	if got := sig.Manifest.Capabilities.Secrets; len(got) != 1 || got[0] != "RUN_TEST_TOKEN" {
		t.Fatalf("manifest secrets = %v", got)
	}

	if _, err := ResolveSignature(context.Background(), regURL, "not a ref", false); err == nil {
		t.Fatal("an invalid ref must refuse before any network call")
	}
	if _, err := ResolveSignature(context.Background(), "http://127.0.0.1:1", "acme/reporter", false); err == nil {
		t.Fatal("an unreachable registry must surface as an error")
	}
}

// TestMaterializeCachesVerifiedInstall pins the §R1 gate and the shared
// cache contract: download → digest check → attestation → extract →
// install.json in the CLI's exact layout, then reuse with no re-download.
func TestMaterializeCachesVerifiedInstall(t *testing.T) {
	home := isolateRunEnv(t)
	fr := &fakeRunRegistry{}
	regURL := startRunRegistry(t, fr)
	sig := resolveFixture(t, regURL)

	var log strings.Builder
	meta, installDir, err := materialize(context.Background(), sig, home, true, false, &log)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(home, "agents", "acme", "reporter", "1.0.0-"+sig.DigestSuffix())
	if installDir != wantDir {
		t.Fatalf("install dir = %q, want %q", installDir, wantDir)
	}
	if meta.Attestation.State != "unsigned" || meta.Attestation.Verified {
		t.Fatalf("attestation = %+v, want unsigned (registry 404s the attestation)", meta.Attestation)
	}
	if !strings.Contains(log.String(), "unsigned") {
		t.Errorf("log must warn about the unsigned release:\n%s", log.String())
	}
	if fr.hitCount("/.well-known/agentry/signing-keys.json") != 0 {
		t.Error("a 404 attestation must not trigger a signing-key fetch")
	}
	for _, rel := range []string{"workspace/agent.yaml", "workspace/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(installDir, rel)); err != nil {
			t.Errorf("extracted install missing %s: %v", rel, err)
		}
	}

	// install.json is field-for-field the CLI's struct: exactly these keys.
	raw, err := os.ReadFile(filepath.Join(installDir, "install.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ref", "namespace", "name", "version", "digest", "source", "attestation"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("install.json missing key %q:\n%s", key, raw)
		}
	}
	if len(doc) != 7 {
		t.Errorf("install.json has %d keys, want the CLI's 7:\n%s", len(doc), raw)
	}
	if meta.Ref != "acme/reporter@1.0.0" || meta.Digest != fr.digest || meta.Source != "registry" {
		t.Fatalf("meta = %+v", meta)
	}

	// Second call: cache reuse — the archive downloads exactly once.
	if got := fr.hitCount("/archive.tar.zst"); got != 1 {
		t.Fatalf("archive downloads after first materialize = %d", got)
	}
	log.Reset()
	meta2, dir2, err := materialize(context.Background(), sig, home, true, false, &log)
	if err != nil {
		t.Fatal(err)
	}
	if dir2 != installDir || meta2.Digest != meta.Digest {
		t.Fatalf("cache reuse returned %q / %+v", dir2, meta2)
	}
	if got := fr.hitCount("/archive.tar.zst"); got != 1 {
		t.Fatalf("cached install re-downloaded the archive (%d hits)", got)
	}
	// The fixture's registry publishes no attestation, so this install is
	// unsigned; the reuse is narrated honestly — a cached unsigned release
	// says so, and that the consent given now admits it — rather than
	// calling itself verified (M23; the CLI's DR-21 shape), and without
	// the consent it is refused.
	if meta.Attestation.State != "unsigned" {
		t.Fatalf("the fixture's install is %q, want unsigned", meta.Attestation.State)
	}
	if !strings.Contains(log.String(), "cached install") || !strings.Contains(log.String(), "unsigned") {
		t.Errorf("reuse must say what it is doing with what:\n%s", log.String())
	}
	if meta2.Attestation.Consent != receipt.ConsentAllowUnsigned {
		t.Errorf("a cached unsigned run carries consent %q, want %q", meta2.Attestation.Consent, receipt.ConsentAllowUnsigned)
	}
	if _, _, err := materialize(context.Background(), sig, home, false, false, &log); err == nil || !strings.Contains(err.Error(), "cached install") {
		t.Fatalf("a cached unsigned install ran without the consent: %v", err)
	}
}

// TestMaterializeRefusesDigestMismatch: one flipped byte in the served
// archive and nothing lands on disk.
func TestMaterializeRefusesDigestMismatch(t *testing.T) {
	home := isolateRunEnv(t)
	fr := &fakeRunRegistry{corruptArchive: true}
	regURL := startRunRegistry(t, fr)
	sig := resolveFixture(t, regURL)

	var log strings.Builder
	_, _, err := materialize(context.Background(), sig, home, true, false, &log)
	if err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("err = %v, want digest mismatch refusal", err)
	}
	installDir := filepath.Join(home, "agents", "acme", "reporter", "1.0.0-"+sig.DigestSuffix())
	if _, statErr := os.Stat(installDir); !os.IsNotExist(statErr) {
		t.Fatal("a refused archive must not leave an install dir behind")
	}
}

// TestMaterializeRefusesUnverifiableAttestation pins the run gate's
// no-override rule: an attestation endpoint that errors (neither verified
// nor absent) refuses the run outright.
func TestMaterializeRefusesUnverifiableAttestation(t *testing.T) {
	home := isolateRunEnv(t)
	fr := &fakeRunRegistry{attStatus: 500}
	regURL := startRunRegistry(t, fr)
	sig := resolveFixture(t, regURL)

	var log strings.Builder
	_, _, err := materialize(context.Background(), sig, home, true, false, &log)
	if err == nil || !strings.Contains(err.Error(), "refusing to run") {
		t.Fatalf("err = %v, want attestation refusal", err)
	}
}

// planFixture lays out an install (workspace + meta) for ensurePlan tests.
func planFixture(t *testing.T) (*manifest.Manifest, *installdir.Meta, string, string) {
	t.Helper()
	m, err := manifest.Parse([]byte(runManifestYAML))
	if err != nil {
		t.Fatal(err)
	}
	installDir := t.TempDir()
	workspaceDir := filepath.Join(installDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "SKILL.md"), []byte("Do the fixture task.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := &installdir.Meta{
		Ref: "acme/reporter@1.0.0", Namespace: "acme", Name: "reporter",
		Version: "1.0.0", Digest: "sha256:feedface", Source: "registry",
	}
	return m, meta, installDir, workspaceDir
}

// TestEnsurePlanReuseAndRecompile pins the CLI's exact plan-reuse rule:
// digest, policy fingerprint, adapter, and file integrity all must hold —
// and any miss recompiles instead of running something stale.
func TestEnsurePlanReuseAndRecompile(t *testing.T) {
	home := isolateRunEnv(t)
	m, meta, installDir, workspaceDir := planFixture(t)
	pol := cataloguedPolicy()
	planDir := filepath.Join(installDir, "plan", "autonomous")
	canary := filepath.Join(planDir, "canary")
	// plant drops a marker file in the plan dir; a recompile clears the
	// whole dir, so the marker's survival is the reuse signal.
	plant := func() {
		t.Helper()
		if err := os.WriteFile(canary, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	surviving := func() bool { _, err := os.Stat(canary); return err == nil }

	var log strings.Builder
	plan, gotDir, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha-1", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotDir != planDir || plan.Digest != meta.Digest || plan.PolicySHA256 != "pol-sha-1" {
		t.Fatalf("fresh compile: dir=%q plan=%+v", gotDir, plan)
	}

	// Reuse: same digest + policy + adapter, files untouched.
	plant()
	if _, _, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha-1", adapter.DefaultName, nil, manifest.ModeAutonomous, nil); err != nil {
		t.Fatal(err)
	}
	if !surviving() {
		t.Fatal("matching plan was recompiled instead of reused")
	}

	// Policy change: the fingerprint participates in the reuse key (§R9).
	plant()
	plan2, _, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	if err != nil {
		t.Fatal(err)
	}
	if surviving() || plan2.PolicySHA256 != "pol-sha-2" {
		t.Fatalf("policy change must recompile (canary=%v, sha=%s)", surviving(), plan2.PolicySHA256)
	}

	// Another build compiled it (same schema, possibly different output;
	// desktop#94): recompiled, and the log names both builds.
	plant()
	b, err := os.ReadFile(filepath.Join(planDir, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	if plan2.CompiledBy == "" {
		t.Fatal("the desktop must record its build in the plans it compiles")
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), []byte(strings.Replace(string(b), plan2.CompiledBy, "other-build", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	seal(t, home, planDir)
	log.Reset()
	planB, _, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	if err != nil {
		t.Fatal(err)
	}
	if surviving() || planB.CompiledBy != plan2.CompiledBy || !strings.Contains(log.String(), "compiled by another build (other-build") {
		t.Fatalf("a plan from another build was reused (canary=%v, compiledBy=%q, log=%q)", surviving(), planB.CompiledBy, log.String())
	}

	// An unstamped build of other code recorded the bare version: this test
	// binary is unstamped too, so the version alone would match and the old
	// compile would be reused (agentry-desktop#218, the CLI's #144). The
	// build identity adds the executable's digest, so it recompiles.
	plant()
	b, err = os.ReadFile(filepath.Join(planDir, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(planB.CompiledBy, version.Version+"+exe.") {
		t.Fatalf("an unstamped build must record its executable's digest, got CompiledBy=%q", planB.CompiledBy)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), []byte(strings.Replace(string(b), planB.CompiledBy, version.Version, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	seal(t, home, planDir)
	log.Reset()
	planD, _, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	if err != nil {
		t.Fatal(err)
	}
	if surviving() || planD.CompiledBy != planB.CompiledBy || !strings.Contains(log.String(), "compiled by another build ("+version.Version+",") {
		t.Fatalf("a plan recorded as bare %s was reused (canary=%v, compiledBy=%q, log=%q)", version.Version, surviving(), planD.CompiledBy, log.String())
	}

	// Another runner image (a policy that pins a different tag compiled
	// it, or a different content version): recompiled, named in the log.
	plant()
	b, err = os.ReadFile(filepath.Join(planDir, "plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), []byte(strings.Replace(string(b), planB.Proxy.Image, "agentry-runner:elsewhere", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	seal(t, home, planDir)
	log.Reset()
	planC, _, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	if err != nil {
		t.Fatal(err)
	}
	if surviving() || planC.Proxy.Image != planB.Proxy.Image || !strings.Contains(log.String(), "names runner image agentry-runner:elsewhere") {
		t.Fatalf("a plan for another runner image was reused (canary=%v, image=%q, log=%q)", surviving(), planC.Proxy.Image, log.String())
	}

	// Digest change (a different release at the same path) recompiles.
	plant()
	changed := *meta
	changed.Digest = "sha256:0ther"
	plan3, _, err := ensurePlan(&log, home, m, &changed, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	if err != nil {
		t.Fatal(err)
	}
	if surviving() || plan3.Digest != "sha256:0ther" {
		t.Fatal("digest change must recompile")
	}

	// Tampering with a generated file fails compile.VerifyFiles and
	// recompiles, with the log saying why.
	plant()
	var tampered string
	for rel := range plan3.Files {
		tampered = filepath.Join(planDir, filepath.FromSlash(rel))
		break
	}
	if err := os.WriteFile(tampered, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	log.Reset()
	if _, _, err := ensurePlan(&log, home, m, &changed, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil, manifest.ModeAutonomous, nil); err != nil {
		t.Fatal(err)
	}
	if surviving() {
		t.Fatal("tampered plan was reused")
	}
	if !strings.Contains(log.String(), "integrity") {
		t.Errorf("tamper recompile must be logged:\n%s", log.String())
	}

	// A corrupt plan.json this home sealed recompiles rather than erroring.
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	seal(t, home, planDir)
	if _, _, err := ensurePlan(&log, home, m, &changed, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil, manifest.ModeAutonomous, nil); err != nil {
		t.Fatal(err)
	}
}

// seal stands in for a compile by this runner home that produced the
// plan.json now on disk: the reuse tests rewrite fields to model another
// build's or another policy's compile, which carries its own record.
func seal(t *testing.T, home, planDir string) {
	t.Helper()
	if err := compile.Seal(mustKey(t, home), planDir); err != nil {
		t.Fatal(err)
	}
}

// widenEgress edits an installed plan.json the way an attacker with write
// access to the install dir would: the egress contract opened to all, every
// field ensurePlan compares untouched. It returns the edited bytes.
func widenEgress(t *testing.T, planPath string) string {
	t.Helper()
	b, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	doc["capabilities"].(map[string]any)["egressMode"] = "all"
	eb, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(planPath, eb, 0o644); err != nil {
		t.Fatal(err)
	}
	return string(eb)
}

// TestEnsurePlanIntegrityRecord pins the CLI's plan.json integrity rule
// (agentry-cli#266, desktop#255) in the desktop: every compile writes the
// record under the runner home's key; a plan.json edited after that
// (fields ensurePlan does not compare, here the egress contract) refuses
// the run with the verify class and is left in place; a plan with no
// record (an older desktop's) is recompiled, never reused.
func TestEnsurePlanIntegrityRecord(t *testing.T) {
	home := isolateRunEnv(t)
	m, meta, installDir, workspaceDir := planFixture(t)
	pol := cataloguedPolicy()
	planDir := filepath.Join(installDir, "plan", "autonomous")
	planPath := filepath.Join(planDir, "plan.json")

	var log strings.Builder
	plan, _, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(planDir, compile.IntegrityFile)); err != nil {
		t.Fatalf("the compile wrote no integrity record: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(home, compile.IntegrityKeyFile)); err != nil || fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("the integrity key must live in the runner home, owner-only: %v %v", fi, err)
	}
	if plan.Capabilities.EgressMode != "hosts" {
		t.Fatalf("fixture plan must declare host egress for the edit below, got %q", plan.Capabilities.EgressMode)
	}

	// Widen the contract in place; every field ensurePlan compares holds.
	edited := widenEgress(t, planPath)
	_, _, err = ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	var tampered *compile.TamperedPlanError
	var classified *engine.ClassifiedError
	if !errors.As(err, &tampered) || !errors.As(err, &classified) || classified.Code != engine.ExitVerify {
		t.Fatalf("an edited plan.json must refuse with the verify class (exit %d), got %v", engine.ExitVerify, err)
	}
	if got, _ := os.ReadFile(planPath); string(got) != edited {
		t.Fatal("the refused plan must be left in place for a person to look at")
	}

	// No record (an older desktop compiled it): recompiled, and said so.
	if err := os.Remove(filepath.Join(planDir, compile.IntegrityFile)); err != nil {
		t.Fatal(err)
	}
	log.Reset()
	plan2, _, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "no integrity record from this runner home") {
		t.Errorf("the recompile must be logged:\n%s", log.String())
	}
	if plan2.Capabilities.EgressMode != "hosts" {
		t.Fatal("a plan with no record was used instead of recompiled")
	}
	if _, err := compile.ReadInstalledPlan(mustKey(t, home), planDir); err != nil {
		t.Fatalf("the recompile must write a record this home accepts: %v", err)
	}
}

func mustKey(t *testing.T, home string) []byte {
	t.Helper()
	key, err := compile.IntegrityKey(home)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// TestEnsurePlanPolicyRefusalSurfaces: a wildcard-egress manifest against
// the default policy is a typed refusal, not a wrapped compile error.
func TestEnsurePlanPolicyRefusalSurfaces(t *testing.T) {
	home := isolateRunEnv(t)
	_, meta, installDir, workspaceDir := planFixture(t)
	wild, err := manifest.Parse([]byte(strings.Replace(runManifestYAML,
		"egress: [api.example.com]", `egress: ["*"]`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	_, _, err = ensurePlan(&log, home, wild, meta, installDir, workspaceDir, policy.Default(), "sha", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	if err == nil || !strings.Contains(err.Error(), "refused by policy") {
		t.Fatalf("err = %v, want a policy refusal", err)
	}

	// An exec command the policy does not catalogue is the compiler's
	// refusal too (agentry-cli#100): the fixture declares exec: [echo] and
	// the default policy maps no package to it.
	m, _, installDir, workspaceDir := planFixture(t)
	_, _, err = ensurePlan(&log, home, m, meta, installDir, workspaceDir, policy.Default(), "sha", adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
	if err == nil || !strings.Contains(err.Error(), "refused by policy") || !strings.Contains(err.Error(), "does not catalogue") {
		t.Fatalf("err = %v, want the exec catalogue refusal", err)
	}
}

// runFixture stands up everything Run needs: isolated env, fake registry,
// ready fake docker scripted for a happy run, model key. Returns the
// registry handle, its URL, the fake docker, and the runner home.
func runFixture(t *testing.T) (*fakeRunRegistry, string, *dockerx.Fake, string) {
	t.Helper()
	home := isolateRunEnv(t)
	saveCataloguedPolicy(t)
	fr := &fakeRunRegistry{}
	regURL := startRunRegistry(t, fr)
	fake := readyImagesFake()
	scriptHappyAgent(fake)
	withFakeDocker(t, fake)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	return fr, regURL, fake, home
}

// request builds a Request against the fixture release with the policy
// budgets kept (MaxTurns -1 = no override).
func request(regURL, outDir string) Request {
	return Request{AcceptUnsigned: true,
		Registry: regURL,
		Ref:      "acme/reporter@1.0.0",
		Task:     "do the fixture task",
		OutDir:   outDir,
		MaxTurns: -1,
	}
}

// TestRunHappyPath drives the whole §R6 lifecycle through Run and asserts
// the envelope-derived view plus the on-disk side effects the frontend
// depends on.
func TestRunHappyPath(t *testing.T) {
	fr, regURL, fd, home := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")

	outDir := filepath.Join(t.TempDir(), "out")
	inputFile := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(inputFile, []byte("input"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := request(regURL, outDir)
	req.InputPaths = []string{inputFile}

	var log strings.Builder
	res, err := Run(context.Background(), req, &log)
	if err != nil {
		t.Fatalf("Run: %v\nlog:\n%s", err, log.String())
	}
	if res.Status != "ok" || res.ExitCode != engine.ExitOK || res.Error != "" {
		t.Fatalf("view = %+v", res)
	}
	if res.Ref != "acme/reporter@1.0.0" || res.Digest != fr.digest || res.RunID == "" {
		t.Errorf("identity fields = %+v", res)
	}
	// The fixture declares exec: {commands: [echo]}, so the run builds the
	// per-QuickAgent exec image the plan names (agentry-cli#9, desktop#17).
	execTag := fixtureExecTag()
	builtExec := false
	for _, op := range fd.Ops() {
		if strings.HasPrefix(op, "Build:"+execTag+"<-") {
			builtExec = true
		}
	}
	if !builtExec {
		t.Errorf("exec image %s not built; docker ops: %v", execTag, fd.Ops())
	}
	if res.ResultJSON != `"final answer"` {
		t.Errorf("result = %s", res.ResultJSON)
	}
	if res.InputTokens != 100 || res.OutputTokens != 40 || res.CostUSD != 0.05 || res.Turns != 2 {
		t.Errorf("usage fields = %+v", res)
	}
	if res.RunPlanSHA == "" || res.AttestationState != "unsigned" {
		t.Errorf("receipt fields = %+v", res)
	}
	if res.OutDir != outDir || !strings.HasPrefix(res.ReceiptsDir, outDir) {
		t.Errorf("out dirs = %q / %q", res.OutDir, res.ReceiptsDir)
	}
	assertPolicyBlock(t, res)
	for _, rel := range []string{"result.json", "receipts/run-plan.json"} {
		if _, err := os.Stat(filepath.Join(outDir, rel)); err != nil {
			t.Errorf("output folder missing %s: %v", rel, err)
		}
	}
	if got := fd.Foreground["-agent"].GotStdin; got != "do the fixture task" {
		t.Errorf("task did not reach the agent stdin: %q", got)
	}

	// The install cache is the CLI's layout and the accepted capability
	// contract was recorded under the shared consent key (D-010).
	installDir := filepath.Join(home, "agents", "acme", "reporter", "1.0.0-"+installdir.DigestSuffix(fr.digest))
	if _, err := os.Stat(filepath.Join(installDir, "install.json")); err != nil {
		t.Errorf("install cache missing: %v", err)
	}
	state, err := installed.Load()
	if err != nil {
		t.Fatal(err)
	}
	rec := state.Find(installed.Key{
		Registry: "runner", Namespace: "acme", Name: "reporter",
		Target: "runner", Dir: filepath.Dir(installDir),
	})
	if rec == nil || rec.Version != "1.0.0" {
		t.Errorf("capability consent not recorded: %+v", state.Records)
	}

	// No profile existed and none was created; the model key came from the
	// environment and the secret from the env fallback.
	profDir := profile.Dir(home, "acme", "reporter", "1.0.0", installdir.DigestSuffix(fr.digest))
	if p, err := profile.Load(profDir); err != nil || p != nil {
		t.Errorf("no profile should exist after an env-secret run: %v %v", p, err)
	}
}

// TestRunWithNoTaskRunsTheAgentsOwnInstruction keeps the run setup's
// promise ("Left empty, its own instruction runs with no extra task"): a
// run whose task is left empty hands the harness the release's own
// instruction, not an empty stdin, which docker-agent refuses (exit 14;
// agentry-cli#449, taken with the pin on v0.6.18, #372).
func TestRunWithNoTaskRunsTheAgentsOwnInstruction(t *testing.T) {
	_, regURL, fd, _ := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	req.Task = ""

	var log strings.Builder
	res, err := Run(context.Background(), req, &log)
	if err != nil {
		t.Fatalf("Run: %v\nlog:\n%s", err, log.String())
	}
	if res.Status != "ok" {
		t.Fatalf("view = %+v", res)
	}
	// The fixture release's SKILL.md (runFixture), as the compile reads it.
	if got := fd.Foreground["-agent"].GotStdin; got != "# Reporter\n\nWrite the report.\n" {
		t.Errorf("agent stdin = %q, want the release's own instruction", got)
	}
}

// TestRunRefusesAnEditedPlan is desktop#255's acceptance through Run: a
// plan.json edited after the desktop compiled it (the contract widened,
// every compared field intact) refuses the next run with the verify class
// before any container starts, the same outcome as `agentry run` (exit 10).
func TestRunRefusesAnEditedPlan(t *testing.T) {
	fr, regURL, fd, home := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	var log strings.Builder
	if _, err := Run(context.Background(), req, &log); err != nil {
		t.Fatalf("first run: %v\nlog:\n%s", err, log.String())
	}

	planPath := filepath.Join(home, "agents", "acme", "reporter", "1.0.0-"+installdir.DigestSuffix(fr.digest), "plan", "autonomous", "plan.json")
	widenEgress(t, planPath)

	var started bool
	fd.OnForeground = func(dockerx.RunSpec) { started = true }
	req.OutDir = filepath.Join(t.TempDir(), "out2")
	res, err := Run(context.Background(), req, &log)
	var classified *engine.ClassifiedError
	var tampered *compile.TamperedPlanError
	if res != nil || !errors.As(err, &classified) || classified.Code != engine.ExitVerify || !errors.As(err, &tampered) {
		t.Fatalf("an edited plan.json must refuse the run with exit %d, got res=%+v err=%v", engine.ExitVerify, res, err)
	}
	if started {
		t.Fatal("a container started for a refused plan")
	}
}

// TestRunSecretFromProfile pins §P2 consumption: the profile's stored
// value satisfies a declared secret the host environment does not have,
// and keep=true retains the profile with lastUsedAt stamped.
func TestRunSecretFromProfile(t *testing.T) {
	fr, regURL, _, home := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "restore-guard")
	_ = os.Unsetenv("RUN_TEST_TOKEN")

	sig := resolveFixture(t, regURL)
	if _, err := saveProfileForSig(sig, map[string]string{"RUN_TEST_TOKEN": "from-profile"}, true); err != nil {
		t.Fatal(err)
	}

	var log strings.Builder
	res, err := Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &log)
	if err != nil {
		t.Fatalf("profile value should satisfy the secret: %v\n%s", err, log.String())
	}
	if res.Status != "ok" {
		t.Fatalf("status = %s (%s)", res.Status, res.Error)
	}
	profDir := profile.Dir(home, "acme", "reporter", "1.0.0", installdir.DigestSuffix(fr.digest))
	kept, err := profile.Load(profDir)
	if err != nil || kept == nil {
		t.Fatalf("keep=true profile must survive the run: %v %v", kept, err)
	}
	if kept.LastUsedAt == "" {
		t.Error("run completion must stamp lastUsedAt")
	}
}

// TestRunKeepProfilePrecedence pins the §P2 completion-choice order:
// request parameter > stored profile > policy default.
func TestRunKeepProfilePrecedence(t *testing.T) {
	fr, regURL, _, home := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	sig := resolveFixture(t, regURL)
	profDir := profile.Dir(home, "acme", "reporter", "1.0.0", installdir.DigestSuffix(fr.digest))
	outDir := func() string { return filepath.Join(t.TempDir(), "out") }
	saveProf := func(keep bool) {
		t.Helper()
		if _, err := saveProfileForSig(sig, map[string]string{"RUN_TEST_TOKEN": "v"}, keep); err != nil {
			t.Fatal(err)
		}
	}
	exists := func() bool {
		p, err := profile.Load(profDir)
		if err != nil {
			t.Fatal(err)
		}
		return p != nil
	}
	boolPtr := func(b bool) *bool { return &b }

	// Parameter beats a stored keep=true: the profile is removed.
	saveProf(true)
	req := request(regURL, outDir())
	req.KeepProfile = boolPtr(false)
	if _, err := Run(context.Background(), req, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if exists() {
		t.Fatal("KeepProfile=false parameter must override the stored keep=true")
	}

	// Parameter beats a stored keep=false: the profile survives.
	saveProf(false)
	req = request(regURL, outDir())
	req.KeepProfile = boolPtr(true)
	if _, err := Run(context.Background(), req, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if !exists() {
		t.Fatal("KeepProfile=true parameter must override the stored keep=false")
	}

	// No parameter: the stored keep=false beats the policy default (true).
	saveProf(false)
	if _, err := Run(context.Background(), request(regURL, outDir()), &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if exists() {
		t.Fatal("stored keep=false must override the policy default")
	}
}

// TestRunAgentFailureStillReturnsView pins the §R8 contract the frontend
// depends on: once the engine starts, failures ride in the envelope
// (status/exit class/error), never a rejected promise that loses the
// receipt paths. Also covers the yanked-release warning.
func TestRunAgentFailureStillReturnsView(t *testing.T) {
	isolateRunEnv(t)
	saveCataloguedPolicy(t)
	fr := &fakeRunRegistry{yanked: true}
	regURL := startRunRegistry(t, fr)
	fake := readyImagesFake()
	fake.Foreground = map[string]*dockerx.FakeForeground{
		"-probe": {ExitCode: 0},
		"-agent": {Stdout: `{"type":"error","message":{"content":"model refused"}}` + "\n", ExitCode: 3},
	}
	withFakeDocker(t, fake)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	t.Setenv("RUN_TEST_TOKEN", "env-secret")

	var log strings.Builder
	res, err := Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &log)
	if err != nil {
		t.Fatalf("an agent failure must still return the view: %v", err)
	}
	if res.Status != "error" || res.ExitCode != engine.ExitHarness {
		t.Fatalf("view = status %q exit %d, want error/%d", res.Status, res.ExitCode, engine.ExitHarness)
	}
	if !strings.Contains(res.Error, "model refused") {
		t.Errorf("view error = %q, want the agent's detail", res.Error)
	}
	if res.OutDir == "" || res.ReceiptsDir == "" {
		t.Errorf("failure view lost the receipt paths: %+v", res)
	}
	if !strings.Contains(log.String(), "YANKED") {
		t.Errorf("yanked release must be warned about:\n%s", log.String())
	}
}

// TestRunBudgetOverrides pins the request-level budget plumbing: MaxTurns
// and TimeoutSeconds on the request override the policy budgets.
func TestRunBudgetOverrides(t *testing.T) {
	_, regURL, fd, _ := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")

	// A one-turn budget kills the chatty agent with the budget class.
	fd.Foreground["-agent"] = &dockerx.FakeForeground{
		Stdout: strings.Repeat(`{"type":"tool_call"}`+"\n", 5), Block: true,
	}
	req := request(regURL, filepath.Join(t.TempDir(), "out"))
	req.MaxTurns = 1
	res, err := Run(context.Background(), req, &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "error" || res.ExitCode != engine.ExitBudget {
		t.Fatalf("view = %q/%d, want error/%d", res.Status, res.ExitCode, engine.ExitBudget)
	}

	// A one-second deadline kills the silent agent with the timeout class.
	fd.Foreground["-agent"] = &dockerx.FakeForeground{Block: true}
	req = request(regURL, filepath.Join(t.TempDir(), "out"))
	req.TimeoutSeconds = 1
	res, err = Run(context.Background(), req, &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "error" || res.ExitCode != engine.ExitTimeout {
		t.Fatalf("view = %q/%d, want error/%d", res.Status, res.ExitCode, engine.ExitTimeout)
	}
}

// TestRunMissingSecretRefusesWithExit11 pins the §P2 fallback: no profile
// and no host env value refuses inside the engine with the variable named
// (v1 exit-11 semantics), still returning the envelope view.
func TestRunMissingSecretRefusesWithExit11(t *testing.T) {
	_, regURL, _, _ := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "restore-guard")
	_ = os.Unsetenv("RUN_TEST_TOKEN")

	res, err := Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &strings.Builder{})
	if err != nil {
		t.Fatalf("the refusal must ride in the view: %v", err)
	}
	if res.Status != "error" || res.ExitCode != engine.ExitPolicy {
		t.Fatalf("view = %q/%d, want error/%d", res.Status, res.ExitCode, engine.ExitPolicy)
	}
	if !strings.Contains(res.Error, "RUN_TEST_TOKEN") {
		t.Errorf("refusal must name the variable: %q", res.Error)
	}
}

// TestRunDockerEnvironmentFailure: a docker daemon that does not answer is
// a pre-engine error (no envelope exists yet), wrapped with the gateway
// class's wording.
func TestRunDockerEnvironmentFailure(t *testing.T) {
	isolateRunEnv(t)
	saveCataloguedPolicy(t)
	regURL := startRunRegistry(t, &fakeRunRegistry{})
	withFakeDocker(t, &dockerx.Fake{AvailableErr: os.ErrPermission})
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	t.Setenv("RUN_TEST_TOKEN", "env-secret")

	_, err := Run(context.Background(), request(regURL, filepath.Join(t.TempDir(), "out")), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "gateway/docker environment") {
		t.Fatalf("err = %v, want the docker-environment wrap", err)
	}
}

// TestMaterializeRefusesBadArchive: bytes whose digest matches but that do
// not extract (the registry served junk) refuse at the extract step.
func TestMaterializeRefusesBadArchive(t *testing.T) {
	home := isolateRunEnv(t)
	fr := &fakeRunRegistry{archive: []byte("junk, not a tar.zst")}
	regURL := startRunRegistry(t, fr)
	sig := resolveFixture(t, regURL)

	var log strings.Builder
	_, _, err := materialize(context.Background(), sig, home, true, false, &log)
	if err == nil || !strings.Contains(err.Error(), "extract archive") {
		t.Fatalf("err = %v, want the extract refusal", err)
	}
}

// TestRunDefaultOutDir pins the desktop default output location shape:
// ~/Downloads/agentry-out/<ns>-<name>-<runid>.
func TestRunDefaultOutDir(t *testing.T) {
	_, regURL, _, _ := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")

	res, err := Run(context.Background(), request(regURL, ""), &strings.Builder{})
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "Downloads", "agentry-out", "acme-reporter-"+res.RunID)
	if res.OutDir != want {
		t.Fatalf("default out dir = %q, want %q", res.OutDir, want)
	}
	if _, err := os.Stat(filepath.Join(res.OutDir, "result.json")); err != nil {
		t.Errorf("default out dir not populated: %v", err)
	}
}

// receiptExecLine reads the exec line of capabilities_enforced from the
// run's result.json (the §R8 envelope) and the plan's own exec text from
// receipts/run-plan.json.
func receiptExecLine(t *testing.T, outDir string) (line, planned string, present bool) {
	t.Helper()
	var env struct {
		CapabilitiesEnforced map[string]string `json:"capabilities_enforced"`
	}
	raw, err := os.ReadFile(filepath.Join(outDir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Enforcement map[string]string `json:"enforcement"`
	}
	raw, err = os.ReadFile(filepath.Join(outDir, "receipts", "run-plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	line, present = env.CapabilitiesEnforced["exec"]
	return line, plan.Enforcement["exec"], present
}

// TestRunReceiptNamesResolvedExecPackages (desktop#95): a release that
// declares exec gets the exec image's resolved package versions on its
// receipt's exec line, the CLI's wording byte for byte — the compiled
// text, then "; packages resolved at image build: " and the versions the
// build recorded (the fake serves "busybox" as that record).
func TestRunReceiptNamesResolvedExecPackages(t *testing.T) {
	_, regURL, _, _ := runFixture(t)
	t.Setenv("RUN_TEST_TOKEN", "env-secret")
	outDir := filepath.Join(t.TempDir(), "out")
	var log strings.Builder
	if _, err := Run(context.Background(), request(regURL, outDir), &log); err != nil {
		t.Fatalf("Run: %v\nlog:\n%s", err, log.String())
	}
	line, planned, present := receiptExecLine(t, outDir)
	if !present || planned == "" {
		t.Fatalf("no exec line: receipt %q, plan %q", line, planned)
	}
	if want := planned + "; packages resolved at image build: busybox"; line != want {
		t.Errorf("exec line =\n  %q\nwant\n  %q", line, want)
	}
}

// TestRunReceiptWithoutExecIsUnchanged (desktop#95): a release that
// declares no exec builds no exec image and its receipt carries no exec
// line and no package claim — nothing was installed, so nothing is named.
func TestRunReceiptWithoutExecIsUnchanged(t *testing.T) {
	isolateRunEnv(t)
	saveCataloguedPolicy(t)
	noExec := strings.Replace(runManifestYAML, "  exec: { commands: [echo] }\n", "", 1)
	if noExec == runManifestYAML {
		t.Fatal("the fixture's exec line was not removed")
	}
	fr := &fakeRunRegistry{
		manifestDoc: manifestDocFromYAML(t, noExec),
		archive:     buildRunArchive(t, map[string]string{"agent.yaml": noExec, "SKILL.md": "Do the fixture task.\n"}),
		namespace:   "acme", name: "reporter", version: "1.0.0",
	}
	regURL := startRunRegistry(t, fr)
	fake := readyImagesFake()
	scriptHappyAgent(fake)
	withFakeDocker(t, fake)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-key")
	t.Setenv("RUN_TEST_TOKEN", "env-secret")

	outDir := filepath.Join(t.TempDir(), "out")
	var log strings.Builder
	if _, err := Run(context.Background(), request(regURL, outDir), &log); err != nil {
		t.Fatalf("Run: %v\nlog:\n%s", err, log.String())
	}
	for _, op := range fake.Ops() {
		if strings.HasPrefix(op, "Build:") && strings.Contains(op, "exec") {
			t.Errorf("an exec image was built for a release without exec: %s", op)
		}
	}
	line, planned, present := receiptExecLine(t, outDir)
	if present && (line != planned || strings.Contains(line, "resolved at image build")) {
		t.Errorf("exec line %q (plan %q) for a release without exec", line, planned)
	}
}

// TestEnsurePlanConcurrentRecompiles (agentry-desktop#265): a desktop run
// and a CLI run started together on one release and mode share the plan
// directory under the runner home's install. Compile replaces that
// directory whole and ReadInstalledPlan waits out an in-flight publish
// (agentry-cli#289), so no caller may clear the directory first: a clear
// could delete the plan another run just published, or fail with
// "directory not empty" while a publish lands. Each caller here stands for
// one client (the CLI's ensurePlan has the same read-then-compile shape
// over the same module); alternating policy fingerprints force a recompile
// on nearly every call, and every call must end with a plan, never a
// "clear stale plan" or a tamper refusal.
func TestEnsurePlanConcurrentRecompiles(t *testing.T) {
	home := isolateRunEnv(t)
	m, meta, installDir, workspaceDir := planFixture(t)
	pol := cataloguedPolicy()

	const clients, rounds = 6, 8
	errs := make(chan error, clients*rounds)
	var wg sync.WaitGroup
	for c := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range rounds {
				polSHA := fmt.Sprintf("pol-sha-%d", (c+r)%2)
				plan, _, err := ensurePlan(io.Discard, home, m, meta, installDir, workspaceDir, pol, polSHA, adapter.DefaultName, nil, manifest.ModeAutonomous, nil)
				switch {
				case err != nil:
					errs <- fmt.Errorf("client %d round %d: %w", c, r, err)
				case plan.PolicySHA256 != polSHA:
					errs <- fmt.Errorf("client %d round %d: plan for %s, asked for %s", c, r, plan.PolicySHA256, polSHA)
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	// What is left is one whole, verifiable plan.
	var log strings.Builder
	if _, _, err := ensurePlan(&log, home, m, meta, installDir, workspaceDir, pol, "pol-sha-0", adapter.DefaultName, nil, manifest.ModeAutonomous, nil); err != nil {
		t.Fatalf("after the race: %v (log %q)", err, log.String())
	}
}

// assertPolicyBlock is agentry-desktop#269's acceptance on a flattened
// envelope: a desktop run or session names the operator policy file it
// enforced (agentry-cli#152) — the runner home's policy.yaml, source "env"
// because the tests relocate the home with AGENTRY_RUNNER_HOME, and the
// digest of that file's bytes, which is what the plan was compiled against.
func assertPolicyBlock(t *testing.T, res *Result) {
	t.Helper()
	path := filepath.Join(os.Getenv(policy.HomeEnv), "policy.yaml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	want := receipt.Policy{Path: path, Source: receipt.PolicySourceEnv, SHA256: hex.EncodeToString(sum[:])}
	if res.Policy == nil || *res.Policy != want {
		t.Errorf("policy block = %+v, want %+v", res.Policy, want)
	}
}
