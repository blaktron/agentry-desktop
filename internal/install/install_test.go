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
	"reflect"
	"slices"
	"strings"
	"testing"

	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/attest"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/trust"

	"github.com/blaktron/agentry-desktop/internal/mcpview"
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
	return buildArchiveWith(t, "SKILL.md", "# Demo\n\nDo the demo thing.\n")
}

// buildArchiveWith packs a workspace holding one file.
func buildArchiveWith(t *testing.T, name, body string) []byte {
	t.Helper()
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, name), []byte(body), 0o644); err != nil {
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
// never in the person's.
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

// TestInstallRefusesCorruptedArchive is the negative test: flip one byte
// of the downloaded archive and the install must refuse at the digest
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

// TestASamePinServerWideningAsksOnUpdate: an update that keeps a declared
// MCP server's pin and gives it a new egress host is a widening the update
// dialog names in the core's words and gates. A record that names the
// server without its grants does not know them, and asks once.
func TestASamePinServerWideningAsksOnUpdate(t *testing.T) {
	versioned := func(caps map[string]any, version string) map[string]any {
		doc := demoManifest(caps)
		doc["version"] = version
		return doc
	}
	const id = "npm:@acme/osv-mcp@1.2.3"
	for _, tc := range []struct {
		name     string
		dropCaps bool // the record lacks the server's grants
		want     string
	}{
		{name: "recorded", want: "adds network egress to b.example for MCP server " + id},
		{name: "older record", dropCaps: true, want: "asks again for MCP server " + id + ", whose own grants your last consent did not record"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			dest := t.TempDir()
			e := NewEngine()
			reg1 := startRegistry(t, &fakeRegistry{manifestDoc: versioned(osvServer(map[string]any{"egress": []string{"a.example"}}), "1.0.0")})
			plan1, err := e.Prepare(context.Background(), reg1, "acme/demo", "claude-skill", dest)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Confirm(plan1.ID, false, true, false); err != nil {
				t.Fatal(err)
			}

			reg2 := startRegistry(t, &fakeRegistry{version: "2.0.0", manifestDoc: versioned(osvServer(map[string]any{"egress": []string{"a.example", "b.example"}}), "2.0.0")})
			st, err := installed.Load()
			if err != nil {
				t.Fatal(err)
			}
			for i := range st.Records {
				st.Records[i].Registry = strings.TrimRight(reg2, "/")
				if got := st.Records[i].Capabilities.MCPServerCaps[id].Egress; !slices.Equal(got, []string{"a.example"}) {
					t.Fatalf("the record keeps the server's egress as %v", got)
				}
				if tc.dropCaps {
					st.Records[i].Capabilities.MCPServerCaps = nil
				}
			}
			if err := installed.Save(st); err != nil {
				t.Fatal(err)
			}

			plan2, err := e.Prepare(context.Background(), reg2, "acme/demo@2.0.0", "claude-skill", dest)
			if err != nil {
				t.Fatal(err)
			}
			if !plan2.RequiresWidenedConfirm || plan2.Diff == nil || !slices.ContainsFunc(plan2.Diff.Widened, func(w string) bool { return strings.Contains(w, tc.want) }) {
				t.Fatalf("the update does not ask as %q: %+v", tc.want, plan2.Diff)
			}
			if _, err := e.Confirm(plan2.ID, false, false, false); err == nil {
				t.Fatal("a same-pin server widening installed without acceptance")
			}
			if _, err := e.Confirm(plan2.ID, false, true, false); err != nil {
				t.Fatalf("refused despite acceptance: %v", err)
			}
		})
	}
}

// demoManifest is acme/demo@1.0.0's manifest with the given capabilities
// block (nil: none), shipping agent.yaml beside SKILL.md.
func demoManifest(caps map[string]any) map[string]any {
	doc := map[string]any{
		"schemaVersion": "1", "name": "demo", "namespace": "acme",
		"kind": "skill", "version": "1.0.0", "summary": "Test agent",
		"entrypoints": map[string]any{"skill": "SKILL.md"}, "publish": map[string]any{"include": []string{"SKILL.md", "agent.yaml"}},
	}
	if caps != nil {
		doc["capabilities"] = caps
	}
	return doc
}

// archiveWithManifest packs SKILL.md and the given manifest as agent.yaml
// (JSON, which is YAML).
func archiveWithManifest(t *testing.T, doc map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return buildArchiveFiles(t, map[string]string{"SKILL.md": "# Demo\n\nDo the demo thing.\n", "agent.yaml": string(b)})
}

// osvServer is a declared MCP server with the given fields over its pin.
func osvServer(fields map[string]any) map[string]any {
	srv := map[string]any{"npm": "@acme/osv-mcp", "version": "1.2.3"}
	for k, v := range fields {
		srv[k] = v
	}
	return map[string]any{"mcpServers": []any{srv}}
}

// TestArchiveIsHeldToTheConsentedContract: the person consents to the
// resolve manifest's contract, and the exports compile their grants from
// the archive's own agent.yaml. An archive that declares more is refused at
// Confirm with the widening named and nothing written — no destination, no
// release directory, no record — a declared server's own egress, secrets
// and tool restriction included, in the core's words.
// One that matches or narrows installs, and the record carries the
// archive's contract.
func TestArchiveIsHeldToTheConsentedContract(t *testing.T) {
	egress := func(hosts ...string) map[string]any {
		return map[string]any{"network": map[string]any{"egress": hosts}}
	}
	for _, tc := range []struct {
		name             string
		target           string
		resolve, archive map[string]any
		refusal          string // "" installs
		wantRecordEgress []string
	}{
		{name: "adds egress", target: "claude-skill", archive: egress("evil.example"), refusal: "adds network egress to evil.example"},
		{name: "adds egress, runner home", target: installdir.Target, archive: egress("evil.example"), refusal: "adds network egress to evil.example"},
		{name: "adds exec", target: "claude-skill", archive: map[string]any{"exec": map[string]any{"commands": []string{"curl"}}}, refusal: "curl"},
		{name: "server egress", target: "claude-skill",
			resolve: osvServer(nil), archive: osvServer(map[string]any{"egress": []string{"evil.example"}}),
			refusal: "adds network egress to evil.example for MCP server npm:@acme/osv-mcp@1.2.3"},
		{name: "server secret", target: installdir.Target,
			resolve: osvServer(nil), archive: osvServer(map[string]any{"secrets": []string{"OSV_TOKEN"}}),
			refusal: "gives MCP server npm:@acme/osv-mcp@1.2.3 the secret OSV_TOKEN"},
		{name: "server tools unrestricted", target: "claude-skill",
			resolve: osvServer(map[string]any{"tools": []string{"query"}}), archive: osvServer(nil),
			refusal: "lifts the tool allowlist of MCP server npm:@acme/osv-mcp@1.2.3 (every tool it serves)"},
		{name: "server tool added", target: "claude-skill",
			resolve: osvServer(map[string]any{"tools": []string{"query"}}), archive: osvServer(map[string]any{"tools": []string{"query", "delete"}}),
			refusal: "adds tool delete to MCP server"},
		{name: "matches", target: installdir.Target, resolve: egress("api.example"), archive: egress("api.example"), wantRecordEgress: []string{"api.example"}},
		{name: "narrows", target: "claude-skill", resolve: egress("api.example"), archive: nil},
		{name: "server narrows", target: "claude-skill",
			resolve: osvServer(map[string]any{"egress": []string{"api.osv.dev", "b.example"}}), archive: osvServer(map[string]any{"egress": []string{"api.osv.dev"}, "tools": []string{"query"}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolate(t)
			home := os.Getenv("AGENTRY_RUNNER_HOME")
			dir := ""
			if tc.target != installdir.Target {
				dir = t.TempDir()
			}
			reg := startRegistry(t, &fakeRegistry{
				archive: archiveWithManifest(t, demoManifest(tc.archive)), manifestDoc: demoManifest(tc.resolve),
			})
			e := NewEngine()
			plan, err := e.Prepare(context.Background(), reg, "acme/demo", tc.target, dir)
			if err != nil {
				t.Fatal(err)
			}
			_, err = e.Confirm(plan.ID, false, true, false)
			st, lerr := installed.Load()
			if lerr != nil {
				t.Fatal(lerr)
			}
			if tc.refusal != "" {
				if err == nil {
					t.Fatal("an archive widening the consented contract was installed")
				}
				for _, want := range []string{"declares more than the registry said it would", tc.refusal} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("refusal %q does not say %q", err, want)
					}
				}
				if _, serr := os.Stat(plan.Paths.Dest); !os.IsNotExist(serr) {
					t.Errorf("%s written before the refusal (stat err %v)", plan.Paths.Dest, serr)
				}
				if tc.target == installdir.Target {
					if _, ok := installdir.Load(installdir.ReleaseDir(home, "acme", "demo", "1.0.0", plan.Digest), plan.Digest); ok {
						t.Error("the release was laid down in the runner home before the refusal")
					}
				}
				if len(st.Records) != 0 {
					t.Errorf("a refused install left a record: %+v", st.Records)
				}
				return
			}
			if err != nil {
				t.Fatalf("an archive within the consented contract was refused: %v", err)
			}
			if len(st.Records) != 1 {
				t.Fatalf("records = %+v, want one", st.Records)
			}
			if got := st.Records[0].Capabilities.EgressHosts; !slices.Equal(got, tc.wantRecordEgress) {
				t.Errorf("record egress = %v, want the archive's %v", got, tc.wantRecordEgress)
			}
		})
	}
}

// TestUnscannedRequiresItsOwnAcceptance mirrors the CLI's
// --allow-unscanned: a version the registry admitted without a scan is its
// own outcome — not unsigned — and needs its own consent, which accepting
// an unsigned release does not give.
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

// TestRunnerTargetInstallsIntoTheRunnerHome: the runner target — the
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
// its provider, decide the export written beside the release.
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

// TestExportPlanDerivesFromTheRunnerPlan: with Settings' "also install
// into <coding agent>" on, a one-click install writes the coding-agent
// export from the same verified bytes as the runner-home install — one
// download, one attestation verdict, two destinations, two records — and
// the export plan owes the gates its own record owes.
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

// TestExportSettingsFollowThePolicyAndTheSwitch: what Settings → Coding
// agents shows and what a one-click install with the switch on writes —
// the export the operator policy implies (the same rule as the runner
// home's export/), its folder, and whether a folder is still to be
// chosen.
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

// TestClaudeSkillExportOfAnInstructionOnlyRelease: the desktop's
// Claude-skill export is the CLI's writer — a prompt agent (AGENT.md, no
// SKILL.md) gets a SKILL.md made from its instruction with the
// contract's allowed-tools, as the CLI writes it; a release that names a
// skill file it does not carry is still refused.
func TestClaudeSkillExportOfAnInstructionOnlyRelease(t *testing.T) {
	isolate(t)
	archive := buildArchiveWith(t, "AGENT.md", "# Changelog\n\nRead the git log.\n")
	doc := map[string]any{
		"schemaVersion": "2", "name": "demo", "namespace": "acme", "version": "1.0.0", "summary": "Drafts a changelog.",
		"license": "Apache-2.0", "modes": []string{"autonomous"},
		"entrypoints":  map[string]any{"instruction": "AGENT.md"},
		"runtime":      map[string]any{"interfaces": []string{"agents-md"}},
		"capabilities": map[string]any{"exec": map[string]any{"commands": []string{"git"}}},
		"publish":      map[string]any{"include": []string{"AGENT.md"}},
	}
	reg := startRegistry(t, &fakeRegistry{archive: archive, manifestDoc: doc})
	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "claude-skill", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Confirm(plan.ID, false, true, false); err != nil {
		t.Fatalf("the Claude-skill export of an instruction-only release: %v", err)
	}
	skill, err := os.ReadFile(filepath.Join(plan.Paths.Dest, "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md was not written: %v", err)
	}
	for _, want := range []string{"name: demo", "description: Drafts a changelog.", "allowed-tools: Bash(git:*)", "# Changelog\n\nRead the git log.\n"} {
		if !strings.Contains(string(skill), want) {
			t.Fatalf("SKILL.md lacks %q:\n%s", want, skill)
		}
	}
	if _, err := os.Stat(filepath.Join(plan.Paths.Dest, "AGENT.md")); err != nil {
		t.Fatalf("the instruction stays beside the skill: %v", err)
	}
	st, _ := installed.Load()
	if len(st.Records) != 1 || st.Records[0].Target != "claude-skill" {
		t.Fatalf("record: %+v", st.Records)
	}

	// A named skill file the release does not carry: refused by name.
	doc2 := map[string]any{
		"schemaVersion": "1", "name": "demo", "namespace": "acme", "kind": "skill", "version": "1.0.0", "summary": "s",
		"entrypoints": map[string]any{"skill": "skills/LIAR.md"}, "publish": map[string]any{"include": []string{"AGENT.md"}},
	}
	reg2 := startRegistry(t, &fakeRegistry{archive: archive, manifestDoc: doc2})
	plan2, err := e.Prepare(context.Background(), reg2, "acme/demo", "claude-skill", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.Confirm(plan2.ID, false, false, false)
	if err == nil || !strings.Contains(err.Error(), "skills/LIAR.md") {
		t.Fatalf("a named skill file the release does not carry: %v", err)
	}
}

// The review shows each declared MCP server's pin, tools and plain env as
// sorted rows; a server with no env has none.
func TestPlanShowsEachDeclaredServersEnv(t *testing.T) {
	isolate(t)
	caps := map[string]any{"mcpServers": []any{
		map[string]any{"npm": "@acme/osv-mcp", "version": "1.2.3", "tools": []string{"query"},
			"env": map[string]string{"OSV_MODE": "offline", "API_BASE": "https://api.osv.dev"}},
		map[string]any{"pypi": "fetcher", "version": "0.4.0"},
	}}
	// env is a schema-v2 field.
	doc := demoManifest(caps)
	doc["schemaVersion"] = "2"
	delete(doc, "kind")
	doc["modes"] = []string{"autonomous"}
	doc["entrypoints"] = map[string]any{"instruction": "SKILL.md"}
	reg := startRegistry(t, &fakeRegistry{
		archive: archiveWithManifest(t, doc), manifestDoc: doc,
	})
	plan, err := NewEngine().Prepare(context.Background(), reg, "acme/demo", "claude-skill", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := []mcpview.Server{
		{ID: "npm:@acme/osv-mcp@1.2.3", Tools: []string{"query"},
			Env: []mcpview.EnvVar{{Name: "API_BASE", Value: "https://api.osv.dev"}, {Name: "OSV_MODE", Value: "offline"}}},
		{ID: "pypi:fetcher@0.4.0", Tools: []string{}, Env: []mcpview.EnvVar{}},
	}
	if !reflect.DeepEqual(plan.MCPServers, want) {
		t.Fatalf("plan servers = %+v\nwant %+v", plan.MCPServers, want)
	}
}

// TestRunnerInstallWarnsOnAnUnreadablePolicy: the runner home's policy is
// read by the CLI's installdir.StyleForHome, so a policy that cannot be
// read is said in the result's Warning, beside the button, and the
// install goes on with the AGENTS.md shape, as the CLI's does.
func TestRunnerInstallWarnsOnAnUnreadablePolicy(t *testing.T) {
	isolate(t)
	home := os.Getenv("AGENTRY_RUNNER_HOME")
	if err := os.WriteFile(policy.PathIn(home), []byte("not: [a policy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := startRegistry(t, &fakeRegistry{})
	e := NewEngine()
	plan, err := e.Prepare(context.Background(), reg, "acme/demo", "", "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.Confirm(plan.ID, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Warning, policy.PathIn(home)) || !strings.Contains(res.Warning, "could not be read") {
		t.Fatalf("warning %q does not name the unreadable policy", res.Warning)
	}
	if !strings.Contains(res.Message, "warning: "+res.Warning) {
		t.Fatalf("message %q does not carry the warning", res.Message)
	}
	meta, ok := installdir.Load(plan.Paths.ReleaseDir, plan.Digest)
	if !ok || meta.Export != "agents-md" || meta.Attestation.Consent != "" {
		t.Fatalf("install.json: %+v ok=%v", meta, ok)
	}
}

// The consent names every declared secret, the release's own and its
// servers', whatever its marking, and says which a run can start
// without.
func TestTheConsentNamesEveryDeclaredSecret(t *testing.T) {
	m := &manifest.Manifest{Capabilities: manifest.Capabilities{
		Secrets:         []string{"REQ_KEY", "OPT_KEY"},
		OptionalSecrets: []string{"OPT_KEY", "SRV_KEY"},
		MCPServers:      []manifest.MCPServer{{NPM: "some-search-server", Version: "1.2.3", Secrets: []string{"SRV_KEY", "REQ_KEY"}}},
	}}
	if got := declaredSecrets(m); !slices.Equal(got, []string{"REQ_KEY", "OPT_KEY", "SRV_KEY"}) {
		t.Fatalf("declaredSecrets = %v", got)
	}
	if got := optionalSecrets(m); !slices.Equal(got, []string{"OPT_KEY", "SRV_KEY"}) {
		t.Fatalf("optionalSecrets = %v", got)
	}
	if got := optionalSecrets(&manifest.Manifest{Capabilities: manifest.Capabilities{Secrets: []string{"A"}}}); len(got) != 0 {
		t.Fatalf("a release without optional secrets: %v", got)
	}
}

// Moving a secret between optional and required changes nothing the
// install record holds, so an update that only flips one asks for no new
// consent; a newly declared secret, optional or not, still does.
func TestAFlipBetweenOptionalAndRequiredAsksNoNewConsent(t *testing.T) {
	before := manifest.Capabilities{Secrets: []string{"A", "B"}, OptionalSecrets: []string{"B"}}
	flipped := manifest.Capabilities{Secrets: []string{"A", "B"}, OptionalSecrets: []string{"A"}}
	if d := installed.DiffCapabilities(installed.Snapshot(before), installed.Snapshot(flipped)); d.HasWidened() {
		t.Fatalf("a flip widened the contract: %+v", d)
	}
	added := manifest.Capabilities{Secrets: []string{"A", "B", "C"}, OptionalSecrets: []string{"B", "C"}}
	if d := installed.DiffCapabilities(installed.Snapshot(before), installed.Snapshot(added)); !d.HasWidened() {
		t.Fatalf("a new optional secret did not widen the contract: %+v", d)
	}
}

// A first install of a release that declares a target host asks for the
// capability consent, as the CLI's install gate does.
func TestATargetHostNeedsTheCapabilityConsent(t *testing.T) {
	if needsCapabilityConfirm(manifest.Capabilities{}) {
		t.Fatal("nothing declared asks for consent")
	}
	if !needsCapabilityConfirm(manifest.Capabilities{TargetHosts: []string{"server"}}) {
		t.Fatal("a target host asks for no consent")
	}
}

// A first install of a release that declares a browser asks for the
// capability consent, as the CLI's install gate does, and an update
// that widens the line asks again.
func TestABrowserNeedsTheCapabilityConsent(t *testing.T) {
	b := &manifest.Browser{Sites: []string{"site"}}
	if !needsCapabilityConfirm(manifest.Capabilities{Browser: b}) {
		t.Fatal("a browser asks for no consent")
	}
	before := manifest.Capabilities{Browser: b}
	for name, after := range map[string]*manifest.Browser{
		"a host":   {Sites: []string{"site"}, Hosts: []string{"cdn.example.com"}},
		"a secret": {Sites: []string{"site"}, Secrets: []string{"SITE_USER"}},
		"public":   {Sites: []string{"site"}, Scope: manifest.BrowserScopePublic},
	} {
		if d := installed.DiffCapabilities(installed.Snapshot(before), installed.Snapshot(manifest.Capabilities{Browser: after, Secrets: after.Secrets})); !d.HasWidened() {
			t.Errorf("%s did not widen the contract: %+v", name, d)
		}
	}
}

// A first install asks for the capability consent about exactly what the
// CLI's install gate asks about, in its words: a remote MCP server is
// network reach to its vendor, and a package server's own egress is
// network reach from its container, whatever the release's own egress
// says. A package server with no egress asks nothing.
func TestTheConsentNamesWhatTheCLIAsksAbout(t *testing.T) {
	osv := func(egress ...string) manifest.MCPServer {
		return manifest.MCPServer{NPM: "@acme/osv-mcp", Version: "1.2.3", Egress: egress}
	}
	for name, tc := range map[string]struct {
		caps manifest.Capabilities
		want []string
	}{
		"none":        {manifest.Capabilities{}, nil},
		"egress":      {manifest.Capabilities{Network: manifest.Network{Egress: manifest.Egress{Hosts: []string{"api.example.com"}}}}, []string{"network egress"}},
		"exec":        {manifest.Capabilities{Exec: manifest.ExecCapability{Commands: []string{"git"}}}, []string{"exec"}},
		"target host": {manifest.Capabilities{TargetHosts: []string{"server"}}, []string{"a target host"}},
		"two hosts":   {manifest.Capabilities{TargetHosts: []string{"web", "db"}}, []string{"target hosts"}},
		"browser":     {manifest.Capabilities{Browser: &manifest.Browser{Sites: []string{"site"}}}, []string{"a web browser"}},
		"remote": {manifest.Capabilities{MCPServers: []manifest.MCPServer{{URL: "https://mcp.deepwiki.com/mcp", Auth: "none"}}},
			[]string{"a remote MCP server"}},
		"server egress":         {manifest.Capabilities{MCPServers: []manifest.MCPServer{osv("api.osv.dev")}}, []string{"network egress for MCP server npm:@acme/osv-mcp@1.2.3"}},
		"server any host":       {manifest.Capabilities{MCPServers: []manifest.MCPServer{osv("*")}}, []string{"UNRESTRICTED network egress for MCP server npm:@acme/osv-mcp@1.2.3"}},
		"server without egress": {manifest.Capabilities{MCPServers: []manifest.MCPServer{osv()}}, nil},
	} {
		if got := ConfirmedCapabilities(tc.caps); !slices.Equal(got, tc.want) {
			t.Errorf("%s: names %q, want %q", name, got, tc.want)
		}
		if needs := needsCapabilityConfirm(tc.caps); needs != (len(tc.want) > 0) {
			t.Errorf("%s: needs confirmation = %v", name, needs)
		}
	}
}

// The consent dialog's lines for the servers' reach: a remote server by
// its address, a package server's egress by its hosts, any host called out.
func TestServerReachLines(t *testing.T) {
	c := manifest.Capabilities{MCPServers: []manifest.MCPServer{
		{URL: "https://mcp.deepwiki.com/mcp", Auth: "none"},
		{NPM: "@acme/osv-mcp", Version: "1.2.3", Egress: []string{"api.osv.dev", "osv.dev"}},
		{PyPI: "fetch-mcp", Version: "0.4.0", Egress: []string{"*"}},
		{NPM: "@acme/quiet", Version: "2.0.0"},
	}}
	want := []string{
		"a remote MCP server: https://mcp.deepwiki.com/mcp",
		"network egress for MCP server npm:@acme/osv-mcp@1.2.3 to api.osv.dev, osv.dev",
		"UNRESTRICTED network egress for MCP server pypi:fetch-mcp@0.4.0 (any host, policy-gated)",
	}
	if got := ServerReach(c); !slices.Equal(got, want) {
		t.Fatalf("ServerReach = %q\nwant %q", got, want)
	}
}
