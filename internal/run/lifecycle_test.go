package run

// Lifecycle tests for the desktop run engine, against the fake registry
// (registryfake_test.go) and the CLI's dockerx.Fake: materialize's
// verified-install cache, ensurePlan's reuse/recompile/tamper rules
// (§R6/§R9), and Run()'s whole resolve → verify → materialize → compile →
// run → receipt pipeline with §P2 profile semantics.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/profile"
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
	if sig.Digest != fr.digest || sig.DigestSuffix() != digestSuffix(fr.digest) {
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
	meta, installDir, err := materialize(context.Background(), sig, home, true, &log)
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
	meta2, dir2, err := materialize(context.Background(), sig, home, true, &log)
	if err != nil {
		t.Fatal(err)
	}
	if dir2 != installDir || meta2.Digest != meta.Digest {
		t.Fatalf("cache reuse returned %q / %+v", dir2, meta2)
	}
	if got := fr.hitCount("/archive.tar.zst"); got != 1 {
		t.Fatalf("cached install re-downloaded the archive (%d hits)", got)
	}
	if !strings.Contains(log.String(), "using verified install") {
		t.Errorf("reuse must say so:\n%s", log.String())
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
	_, _, err := materialize(context.Background(), sig, home, true, &log)
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
	_, _, err := materialize(context.Background(), sig, home, true, &log)
	if err == nil || !strings.Contains(err.Error(), "refusing to run") {
		t.Fatalf("err = %v, want attestation refusal", err)
	}
}

// planFixture lays out an install (workspace + meta) for ensurePlan tests.
func planFixture(t *testing.T) (*manifest.Manifest, *installMeta, string, string) {
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
	meta := &installMeta{
		Ref: "acme/reporter@1.0.0", Namespace: "acme", Name: "reporter",
		Version: "1.0.0", Digest: "sha256:feedface", Source: "registry",
	}
	return m, meta, installDir, workspaceDir
}

// TestEnsurePlanReuseAndRecompile pins the CLI's exact plan-reuse rule:
// digest, policy fingerprint, adapter, and file integrity all must hold —
// and any miss recompiles instead of running something stale.
func TestEnsurePlanReuseAndRecompile(t *testing.T) {
	isolateRunEnv(t)
	m, meta, installDir, workspaceDir := planFixture(t)
	pol := policy.Default()
	planDir := filepath.Join(installDir, "plan")
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
	plan, gotDir, err := ensurePlan(&log, m, meta, installDir, workspaceDir, pol, "pol-sha-1", adapter.DefaultName, nil)
	if err != nil {
		t.Fatal(err)
	}
	if gotDir != planDir || plan.Digest != meta.Digest || plan.PolicySHA256 != "pol-sha-1" {
		t.Fatalf("fresh compile: dir=%q plan=%+v", gotDir, plan)
	}

	// Reuse: same digest + policy + adapter, files untouched.
	plant()
	if _, _, err := ensurePlan(&log, m, meta, installDir, workspaceDir, pol, "pol-sha-1", adapter.DefaultName, nil); err != nil {
		t.Fatal(err)
	}
	if !surviving() {
		t.Fatal("matching plan was recompiled instead of reused")
	}

	// Policy change: the fingerprint participates in the reuse key (§R9).
	plant()
	plan2, _, err := ensurePlan(&log, m, meta, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil)
	if err != nil {
		t.Fatal(err)
	}
	if surviving() || plan2.PolicySHA256 != "pol-sha-2" {
		t.Fatalf("policy change must recompile (canary=%v, sha=%s)", surviving(), plan2.PolicySHA256)
	}

	// Digest change (a different release at the same path) recompiles.
	plant()
	changed := *meta
	changed.Digest = "sha256:0ther"
	plan3, _, err := ensurePlan(&log, m, &changed, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil)
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
	if _, _, err := ensurePlan(&log, m, &changed, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil); err != nil {
		t.Fatal(err)
	}
	if surviving() {
		t.Fatal("tampered plan was reused")
	}
	if !strings.Contains(log.String(), "integrity") {
		t.Errorf("tamper recompile must be logged:\n%s", log.String())
	}

	// A corrupt plan.json silently recompiles rather than erroring.
	if err := os.WriteFile(filepath.Join(planDir, "plan.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ensurePlan(&log, m, &changed, installDir, workspaceDir, pol, "pol-sha-2", adapter.DefaultName, nil); err != nil {
		t.Fatal(err)
	}
}

// TestEnsurePlanPolicyRefusalSurfaces: a wildcard-egress manifest against
// the default policy is a typed refusal, not a wrapped compile error.
func TestEnsurePlanPolicyRefusalSurfaces(t *testing.T) {
	isolateRunEnv(t)
	_, meta, installDir, workspaceDir := planFixture(t)
	wild, err := manifest.Parse([]byte(strings.Replace(runManifestYAML,
		"egress: [api.example.com]", `egress: ["*"]`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	_, _, err = ensurePlan(&log, wild, meta, installDir, workspaceDir, policy.Default(), "sha", adapter.DefaultName, nil)
	if err == nil || !strings.Contains(err.Error(), "refused by policy") {
		t.Fatalf("err = %v, want a policy refusal", err)
	}
}

// runFixture stands up everything Run needs: isolated env, fake registry,
// ready fake docker scripted for a happy run, model key. Returns the
// registry handle, its URL, the fake docker, and the runner home.
func runFixture(t *testing.T) (*fakeRunRegistry, string, *dockerx.Fake, string) {
	t.Helper()
	home := isolateRunEnv(t)
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
	installDir := filepath.Join(home, "agents", "acme", "reporter", "1.0.0-"+digestSuffix(fr.digest))
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
	profDir := profile.Dir(home, "acme", "reporter", "1.0.0", digestSuffix(fr.digest))
	if p, err := profile.Load(profDir); err != nil || p != nil {
		t.Errorf("no profile should exist after an env-secret run: %v %v", p, err)
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
	profDir := profile.Dir(home, "acme", "reporter", "1.0.0", digestSuffix(fr.digest))
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
	profDir := profile.Dir(home, "acme", "reporter", "1.0.0", digestSuffix(fr.digest))
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
	_, _, err := materialize(context.Background(), sig, home, true, &log)
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
