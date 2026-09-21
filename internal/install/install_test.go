package install

// End-to-end engine tests against a fake registry. The point of a
// Agentry client is what it REFUSES: a corrupted archive (digest
// mismatch), a signature no pinned key produced, an unsigned release without
// explicit acceptance, a widened capability contract without explicit
// acceptance. Each has a test; the happy path proves the refusals are not
// vacuous.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/attest"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/trust"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// fakeRegistry is one test registry serving a single agent version.
type fakeRegistry struct {
	t       *testing.T
	srv     *httptest.Server
	archive []byte
	digest  string

	manifestDoc map[string]any
	version     string

	// signer is the key the key-set endpoint advertises.
	signer *ecdsa.PrivateKey
	// envelopeSigner actually signs the envelope (differs in the bad-key test).
	envelopeSigner *ecdsa.PrivateKey
	// unsigned makes the attestation endpoint 404.
	unsigned bool
	// omitAttURL makes the resolve result carry no attestationUrl, so the
	// client must fall back to the canonical attestation path.
	omitAttURL bool
	// corruptArchive flips one byte of the served archive bytes.
	corruptArchive bool
	// unscanned makes the resolve result say the version was admitted
	// without a scan (scanStatus skipped, no attestationUrl).
	unscanned bool
}

// newKey generates a P-256 signing key for the fake registry.
func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// pubPEM renders a key's public half as the PEM the key set advertises.
func pubPEM(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// buildTestArchive produces a real deterministic release archive holding a
// SKILL.md without frontmatter (so the exporter's injection is observable).
func buildTestArchive(t *testing.T) []byte {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "SKILL.md"), []byte("# Demo\n\nDo the demo thing.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := pkgarchive.WalkWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := pkgarchive.BuildArchive(ws, files, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// startRegistry fills defaults, starts the fake server, and returns its
// URL; the server dies with the test.
func startRegistry(t *testing.T, f *fakeRegistry) string {
	t.Helper()
	f.t = t
	if f.version == "" {
		f.version = "1.0.0"
	}
	if f.manifestDoc == nil {
		f.manifestDoc = map[string]any{
			"schemaVersion": "1", "name": "demo", "namespace": "acme",
			"kind": "skill", "version": f.version, "summary": "Test agent",
			"entrypoints": map[string]any{"skill": "SKILL.md"}, "publish": map[string]any{"include": []string{"SKILL.md"}},
		}
	}
	if f.archive == nil {
		f.archive = buildTestArchive(t)
	}
	sum := sha256.Sum256(f.archive)
	f.digest = "sha256:" + hexEncode(sum[:])
	if f.signer == nil {
		f.signer = newKey(t)
	}
	if f.envelopeSigner == nil {
		f.envelopeSigner = f.signer
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agentry/signing-keys.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"keyId": "test-key-1", "algorithm": "ecdsa-p256-sha256",
				"publicKeyPem": pubPEM(t, f.signer),
			}},
		})
	})
	mux.HandleFunc("/api/v1/resolve", func(w http.ResponseWriter, _ *http.Request) {
		att := "/att"
		if f.omitAttURL {
			att = ""
		}
		w.Header().Set("Content-Type", "application/json")
		doc := map[string]any{
			"archiveUrl": "/archive.tar.zst", "attestationUrl": att,
			"digest": f.digest, "manifest": f.manifestDoc,
			"namespace": "acme", "name": "demo", "version": f.version,
			"yanked": false, "scanStatus": "pipeline",
		}
		if f.unscanned {
			delete(doc, "attestationUrl")
			doc["scanStatus"] = "skipped"
		}
		_ = json.NewEncoder(w).Encode(doc)
	})
	mux.HandleFunc("/archive.tar.zst", func(w http.ResponseWriter, _ *http.Request) {
		data := f.archive
		if f.corruptArchive {
			data = append([]byte(nil), data...)
			data[len(data)/2] ^= 0xFF
		}
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/att", func(w http.ResponseWriter, _ *http.Request) {
		if f.unsigned {
			http.NotFound(w, nil)
			return
		}
		stmt := attest.BuildStatement("test", "acme/demo", f.version, f.digest, "", "upload",
			attest.ScanRunRef{ID: "run-1", RulesetVer: "1.0.0", ReportDigest: "sha256:0"}, 1)
		payload, err := json.Marshal(stmt)
		if err != nil {
			t.Error(err)
		}
		digest := sha256.Sum256(attest.PAE(attest.PayloadType, payload))
		sig, err := ecdsa.SignASN1(rand.Reader, f.envelopeSigner, digest[:])
		if err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(attest.DSSEEnvelope{
			PayloadType: attest.PayloadType,
			Payload:     base64.StdEncoding.EncodeToString(payload),
			Signatures:  []attest.DSSESignature{{KeyID: "test-key-1", Sig: base64.StdEncoding.EncodeToString(sig)}},
		})
	})

	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f.srv.URL
}

// hexEncode is a dependency-free hex encoder for the digest fixture.
func hexEncode(b []byte) string {
	const hextable = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hextable[v>>4]
		out[i*2+1] = hextable[v&0x0f]
	}
	return string(out)
}

// isolate points every agentry client file at a temp dir — the config
// dir, the runner home, and HOME itself, so a test that exports a Claude
// skill with no directory picked lands in a temp ~/.claude/skills and
// never in the person's (one did, on 2026-09-21, before HOME was here).
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}

func TestInstallHappyPathVerifiesAndExports(t *testing.T) {
	isolate(t)
	reg := startRegistry(t, &fakeRegistry{})
	dest := t.TempDir()

	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", dest)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Attestation.State != trust.StateVerified {
		t.Fatalf("attestation = %s (%s), want verified", plan.Attestation.State, plan.Attestation.Detail)
	}
	if plan.Unsigned || plan.NeedsCapabilityConfirm || plan.RequiresWidenedConfirm {
		t.Fatalf("no consent gates expected on a signed, capability-free first install: %+v", plan)
	}

	res, err := e.Confirm(plan.ID, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	skill, err := os.ReadFile(filepath.Join(dest, "demo", "SKILL.md"))
	if err != nil {
		t.Fatalf("exported skill missing: %v (result: %+v)", err, res)
	}
	if !bytes.HasPrefix(skill, []byte("---\n")) {
		t.Errorf("SKILL.md frontmatter not injected:\n%s", skill)
	}

	st, err := installed.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Records) != 1 || st.Records[0].Version != "1.0.0" {
		t.Fatalf("install not recorded: %+v", st.Records)
	}
	if st.Records[0].Managed {
		t.Error("a user-picked directory must not be recorded as managed")
	}
}

// TestInstallRefusesCorruptedArchive is the §10.4 negative test: flip one
// byte of the downloaded archive and the install must refuse at the digest
// check, before any signature or write.
func TestInstallRefusesCorruptedArchive(t *testing.T) {
	isolate(t)
	reg := startRegistry(t, &fakeRegistry{corruptArchive: true})

	e := NewEngine()
	_, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", t.TempDir())
	if err == nil {
		t.Fatal("a corrupted archive was accepted")
	}
	if !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("refusal reason = %v, want digest mismatch", err)
	}
}

// TestInstallRefusesWrongKeySignature: the envelope is signed by a key the
// registry never advertised. Worse evidence than no signature — refuse.
func TestInstallRefusesWrongKeySignature(t *testing.T) {
	isolate(t)
	f := &fakeRegistry{envelopeSigner: nil}
	f.signer = nil
	// Advertise key A, sign with key B.
	tKeyA := newKey(t)
	tKeyB := newKey(t)
	f.signer = tKeyA
	f.envelopeSigner = tKeyB
	reg := startRegistry(t, f)

	e := NewEngine()
	_, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", t.TempDir())
	if err == nil {
		t.Fatal("a signature from an unadvertised key was accepted")
	}
	if !strings.Contains(err.Error(), "attestation") || !strings.Contains(err.Error(), "refusing to install") {
		t.Fatalf("refusal reason = %v", err)
	}
}

// TestUnsignedRequiresExplicitAcceptance mirrors the CLI's --allow-unsigned.
func TestUnsignedRequiresExplicitAcceptance(t *testing.T) {
	isolate(t)
	reg := startRegistry(t, &fakeRegistry{unsigned: true})
	dest := t.TempDir()

	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", dest)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Unsigned {
		t.Fatalf("plan should be unsigned: %+v", plan.Attestation)
	}
	if _, err := e.Confirm(plan.ID, false, false, false); err == nil {
		t.Fatal("unsigned install proceeded without acceptance")
	}
	if _, err := e.Confirm(plan.ID, true, false, false); err != nil {
		t.Fatalf("unsigned install refused despite acceptance: %v", err)
	}
}

// TestWidenedCapabilitiesRequireAcceptance: an upgrade that adds egress must
// be diffed against the recorded contract and gated.
func TestWidenedCapabilitiesRequireAcceptance(t *testing.T) {
	isolate(t)
	dest := t.TempDir()
	e := NewEngine()

	// v1: no capabilities.
	reg1 := startRegistry(t, &fakeRegistry{})
	plan1, err := e.Prepare(context.Background(), reg1, "acme/demo", "claude-skill", dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan1.ID, false, false, false); err != nil {
		t.Fatal(err)
	}

	// v2 from the SAME registry URL: adds network egress. The key includes
	// the registry URL, so the second server must impersonate the first —
	// instead, rewrite the recorded registry to the new server's URL.
	f2 := &fakeRegistry{
		version: "2.0.0",
		manifestDoc: map[string]any{
			"schemaVersion": "1", "name": "demo", "namespace": "acme",
			"kind": "skill", "version": "2.0.0", "summary": "Test agent",
			"entrypoints": map[string]any{"skill": "SKILL.md"}, "publish": map[string]any{"include": []string{"SKILL.md"}},
			"capabilities": map[string]any{
				"network": map[string]any{"egress": []string{"api.example"}},
			},
		},
	}
	reg2 := startRegistry(t, f2)
	st, err := installed.Load()
	if err != nil {
		t.Fatal(err)
	}
	for i := range st.Records {
		st.Records[i].Registry = strings.TrimRight(reg2, "/")
	}
	if err := installed.Save(st); err != nil {
		t.Fatal(err)
	}

	plan2, err := e.Prepare(context.Background(), reg2, "acme/demo@2.0.0", "claude-skill", dest)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.PrevVersion != "1.0.0" {
		t.Fatalf("prev version = %q, want 1.0.0", plan2.PrevVersion)
	}
	if !plan2.RequiresWidenedConfirm || plan2.Diff == nil || !plan2.Diff.HasWidened() {
		t.Fatalf("widening not detected: %+v", plan2)
	}
	found := false
	for _, w := range plan2.Diff.Widened {
		if strings.Contains(w, "api.example") {
			found = true
		}
	}
	if !found {
		t.Errorf("diff does not name the new egress host: %+v", plan2.Diff)
	}
	if _, err := e.Confirm(plan2.ID, false, false, false); err == nil {
		t.Fatal("widened contract installed without acceptance")
	}
	if _, err := e.Confirm(plan2.ID, false, true, false); err != nil {
		t.Fatalf("widened contract refused despite acceptance: %v", err)
	}
}

// TestUnscannedRequiresItsOwnAcceptance mirrors the CLI's --allow-unscanned
// (M23; the core's M21): a version the registry admitted without a scan is
// its own outcome — not unsigned — and needs its own consent, which
// accepting an unsigned release does not give.
func TestUnscannedRequiresItsOwnAcceptance(t *testing.T) {
	isolate(t)
	reg := startRegistry(t, &fakeRegistry{unscanned: true})
	dest := t.TempDir()

	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", dest)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Unscanned || plan.Unsigned || plan.Attestation.State != "unscanned" {
		t.Fatalf("plan should be unscanned and not unsigned: unscanned=%t unsigned=%t %+v", plan.Unscanned, plan.Unsigned, plan.Attestation)
	}
	if _, err := e.Confirm(plan.ID, false, false, false); err == nil || !strings.Contains(err.Error(), "unscanned") {
		t.Fatalf("unscanned install proceeded without acceptance: %v", err)
	}
	if _, err := e.Confirm(plan.ID, true, false, false); err == nil || !strings.Contains(err.Error(), "unscanned") {
		t.Fatalf("accepting unsigned admitted an unscanned install: %v", err)
	}
	if _, err := e.Confirm(plan.ID, false, false, true); err != nil {
		t.Fatalf("unscanned install refused despite acceptance: %v", err)
	}
}

// TestRunnerTargetInstallsIntoTheRunnerHome (agentry-desktop#146, plan
// desktop-one-click-install.md D3/D9/D10): the runner target — the
// default — lays the verified release down in the runner home the CLI's
// run reads (installdir), writes the export beside it in the style the
// policy implies (no policy: AGENTS.md), records it under the run's own
// consent key, managed; a newer version replaces the older once recorded;
// uninstall removes the agent's directory and the record.
func TestRunnerTargetInstallsIntoTheRunnerHome(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	reg := startRegistry(t, &fakeRegistry{})
	e := NewEngine()

	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target != installdir.Target || plan.Paths.Dest != installdir.AgentDir(home, "acme", "demo") || !plan.Paths.Managed {
		t.Fatalf("plan: target %q paths %+v", plan.Target, plan.Paths)
	}
	res, err := e.Confirm(plan.ID, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	releaseDir := installdir.ReleaseDir(home, "acme", "demo", "1.0.0", plan.Digest)
	meta, ok := installdir.Load(releaseDir, plan.Digest)
	if !ok || meta.Export != "agents-md" || meta.Attestation.State != "verified" || !meta.Attestation.Verified || meta.Attestation.Consent != "" {
		t.Fatalf("install.json: %+v ok=%v (result %+v)", meta, ok, res)
	}
	for _, want := range []string{"workspace/SKILL.md", "export/AGENTS.md", "export/agents/demo/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(releaseDir, filepath.FromSlash(want))); err != nil {
			t.Fatalf("%s: %v", want, err)
		}
	}
	st, err := installed.Load()
	if err != nil || len(st.Records) != 1 {
		t.Fatalf("records: %+v err %v", st.Records, err)
	}
	if rec := st.Records[0]; rec.Registry != installdir.ConsentRegistry || rec.Target != installdir.Target || rec.Dir != installdir.AgentDir(home, "acme", "demo") || !rec.Managed {
		t.Fatalf("record: %+v", rec)
	}
	items, _ := List()
	if len(items) != 1 || !items[0].Manageable || !items[0].Present {
		t.Fatalf("List: %+v", items)
	}

	// v2 from another server: the key names no registry, so the record is
	// found; the older release directory goes once the new one is recorded.
	f2 := &fakeRegistry{version: "2.0.0"}
	reg2 := startRegistry(t, f2)
	plan2, err := e.Prepare(context.Background(), reg2, "acme/demo", installdir.Target, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan2.PrevVersion != "1.0.0" {
		t.Fatalf("the installed version was not found for the diff: %+v", plan2)
	}
	if _, err := e.Confirm(plan2.ID, false, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(releaseDir); !os.IsNotExist(err) {
		t.Fatal("1.0.0's directory survived the update")
	}
	if _, ok := installdir.Load(installdir.ReleaseDir(home, "acme", "demo", "2.0.0", plan2.Digest), plan2.Digest); !ok {
		t.Fatal("2.0.0 is not installed")
	}
	st, _ = installed.Load()
	if len(st.Records) != 1 || st.Records[0].Version != "2.0.0" {
		t.Fatalf("records after update: %+v", st.Records)
	}

	// --dir is for the coding-agent targets.
	if _, err := e.Prepare(context.Background(), reg2, "acme/demo", installdir.Target, t.TempDir()); err == nil || !strings.Contains(err.Error(), "runner home") {
		t.Fatalf("a directory with the runner target: %v", err)
	}

	// Uninstall removes the agent's directory and the record.
	msg, err := Uninstall(installdir.ConsentKey(home, "acme", "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(installdir.AgentDir(home, "acme", "demo")); !os.IsNotExist(err) {
		t.Fatalf("uninstall left the agent's directory (%s)", msg)
	}
	if st, _ := installed.Load(); len(st.Records) != 0 {
		t.Fatalf("uninstall left the record: %+v", st.Records)
	}
}

// TestRunnerTargetExportFollowsThePolicy: the policy's coding agent, then
// its provider, decide the export written beside the release (D2).
func TestRunnerTargetExportFollowsThePolicy(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	reg := startRegistry(t, &fakeRegistry{})
	pol := policy.Default()
	pol.Harness = map[string]policy.HarnessPolicy{"claude": {}}
	if err := policy.Save(filepath.Join(home, "policy.yaml"), pol); err != nil {
		t.Fatal(err)
	}
	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan.ID, false, false, false); err != nil {
		t.Fatal(err)
	}
	releaseDir := installdir.ReleaseDir(home, "acme", "demo", "1.0.0", plan.Digest)
	skill, err := os.ReadFile(filepath.Join(releaseDir, "export", "demo", "SKILL.md"))
	if err != nil {
		t.Fatalf("the Claude skill export is missing: %v", err)
	}
	if !bytes.HasPrefix(skill, []byte("---\n")) {
		t.Errorf("SKILL.md frontmatter not injected:\n%s", skill)
	}
	if meta, ok := installdir.Load(releaseDir, plan.Digest); !ok || meta.Export != "claude-skill" {
		t.Fatalf("install.json export: %+v", meta)
	}
}

// TestExportPlanDerivesFromTheRunnerPlan (agentry-desktop#148): with
// Settings' "also install into <coding agent>" on, a one-click install
// writes the coding-agent export from the same verified bytes as the
// runner-home install — one download, one attestation verdict, two
// destinations, two records — and the export plan owes the gates its own
// record owes.
func TestExportPlanDerivesFromTheRunnerPlan(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	userHome := os.Getenv("HOME")
	reg := startRegistry(t, &fakeRegistry{})
	e := NewEngine()

	runner, err := e.Prepare(context.Background(), reg, "acme/demo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	export, err := e.PrepareExport(runner.ID, "claude-skill", "")
	if err != nil {
		t.Fatal(err)
	}
	if export.ID == runner.ID || export.Target != "claude-skill" || export.Paths.Dest != filepath.Join(userHome, ".claude", "skills", "demo") || !export.Paths.Managed {
		t.Fatalf("export plan: %+v", export)
	}
	if export.Digest != runner.Digest || export.Attestation.State != runner.Attestation.State || export.Version != runner.Version || export.Registry != runner.Registry {
		t.Fatalf("the export plan is not the runner plan's bytes: %+v vs %+v", export, runner)
	}
	if _, err := e.Confirm(runner.ID, false, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(export.ID, false, false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(userHome, ".claude", "skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("the Claude skill was not written: %v", err)
	}
	if _, ok := installdir.Load(installdir.ReleaseDir(home, "acme", "demo", "1.0.0", runner.Digest), runner.Digest); !ok {
		t.Fatal("the runner-home install was not written")
	}
	st, _ := installed.Load()
	if len(st.Records) != 2 {
		t.Fatalf("two records expected: %+v", st.Records)
	}
	targets := map[string]bool{}
	for _, r := range st.Records {
		targets[r.Target] = true
	}
	if !targets[installdir.Target] || !targets["claude-skill"] {
		t.Fatalf("records: %+v", st.Records)
	}

	// The export plan reads its own record: a second one-click install of
	// the same version is an update for the export too, with the version
	// it found; a first export of an egress-declaring release owes the
	// capability consent even when the runner record already has it.
	runner2, err := e.Prepare(context.Background(), reg, "acme/demo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	export2, err := e.PrepareExport(runner2.ID, "claude-skill", "")
	if err != nil {
		t.Fatal(err)
	}
	if export2.PrevVersion != "1.0.0" || export2.NeedsCapabilityConfirm {
		t.Fatalf("the export's own record was not read: %+v", export2)
	}
	project := t.TempDir()
	agentsMD, err := e.PrepareExport(runner2.ID, "agents-md", project)
	if err != nil {
		t.Fatal(err)
	}
	if agentsMD.PrevVersion != "" || agentsMD.Paths.AgentsMD != filepath.Join(project, "AGENTS.md") {
		t.Fatalf("a first AGENTS.md export: %+v", agentsMD)
	}
	e.Cancel(runner2.ID)
	e.Cancel(export2.ID)
	e.Cancel(agentsMD.ID)

	// An export needs a pending runner plan to derive from, and is one of
	// the two coding-agent exports.
	if _, err := e.PrepareExport(runner2.ID, "claude-skill", ""); err == nil {
		t.Fatal("an export from a spent plan")
	}
	runner3, _ := e.Prepare(context.Background(), reg, "acme/demo", "", "")
	if _, err := e.PrepareExport(runner3.ID, installdir.Target, ""); err == nil {
		t.Fatal("the runner target is not an export")
	}
	if _, err := e.PrepareExport(runner3.ID, "agents-md", ""); err == nil {
		t.Fatal("AGENTS.md with no project directory")
	}
}

// TestExportSettingsFollowThePolicyAndTheSwitch (agentry-desktop#148):
// what Settings → Coding agents shows and what a one-click install with
// the switch on writes — the export the operator policy implies (the same
// rule as the runner home's export/), its folder, and whether a folder is
// still to be chosen.
func TestExportSettingsFollowThePolicyAndTheSwitch(t *testing.T) {
	isolate(t)
	home := t.TempDir()
	t.Setenv("AGENTRY_RUNNER_HOME", home)
	userHome := os.Getenv("HOME")
	skills := filepath.Join(userHome, ".claude", "skills")

	// No policy, nothing set: AGENTS.md, no folder yet, off.
	x, err := ExportSettings()
	if err != nil {
		t.Fatal(err)
	}
	if x.Target != "agents-md" || x.Dir != "" || !x.NeedsDir || x.Enabled {
		t.Fatalf("defaults: %+v", x)
	}

	pol := policy.Default()
	pol.Harness = map[string]policy.HarnessPolicy{"claude": {}}
	if err := policy.Save(filepath.Join(home, "policy.yaml"), pol); err != nil {
		t.Fatal(err)
	}
	s, _ := settings.Load()
	s.InstallIntoCodingAgent = true
	s.CodingAgentProjectDir = "/home/me/work/oncall"
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	x, err = ExportSettings()
	if err != nil {
		t.Fatal(err)
	}
	if x.Target != "claude-skill" || x.Dir != skills || x.NeedsDir || !x.Enabled {
		t.Fatalf("claude chosen: %+v", x)
	}

	// Codex alone: AGENTS.md in the project directory Settings holds.
	pol.Harness = map[string]policy.HarnessPolicy{"codex": {}}
	if err := policy.Save(filepath.Join(home, "policy.yaml"), pol); err != nil {
		t.Fatal(err)
	}
	x, _ = ExportSettings()
	if x.Target != "agents-md" || x.Dir != "/home/me/work/oncall" || x.NeedsDir {
		t.Fatalf("codex chosen: %+v", x)
	}
}
