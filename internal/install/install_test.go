package install

// End-to-end engine tests against a fake registry. The point of a
// SecureAgentry client is what it REFUSES: a corrupted archive (digest
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
	"github.com/blaktron/agentry-cli/pkg/trust"
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"archiveUrl": "/archive.tar.zst", "attestationUrl": att,
			"digest": f.digest, "manifest": f.manifestDoc,
			"namespace": "acme", "name": "demo", "version": f.version,
			"yanked": false,
		})
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

// isolate points every agentry client file at a temp dir.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
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

	res, err := e.Confirm(plan.ID, false, false)
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
	if _, err := e.Confirm(plan.ID, false, false); err == nil {
		t.Fatal("unsigned install proceeded without acceptance")
	}
	if _, err := e.Confirm(plan.ID, true, false); err != nil {
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
	if _, err := e.Confirm(plan1.ID, false, false); err != nil {
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
	if _, err := e.Confirm(plan2.ID, false, false); err == nil {
		t.Fatal("widened contract installed without acceptance")
	}
	if _, err := e.Confirm(plan2.ID, false, true); err != nil {
		t.Fatalf("widened contract refused despite acceptance: %v", err)
	}
}
