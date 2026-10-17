package run

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry/pkg/apiclient"
)

// The unsigned-run consent is enforced in Go: a registry with no
// attestation refuses the run unless AcceptUnsigned was given, and then
// marks the receipt unsigned.
func TestVerifyRunAttestationUnsignedGate(t *testing.T) {
	reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/attestation") {
			http.Error(w, `{"title":"not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"title":"not found"}`, http.StatusNotFound)
	}))
	defer reg.Close()
	sig := &Signature{Registry: reg.URL, Ref: "acme/demo@1.0.0", Namespace: "acme", Name: "demo", Version: "1.0.0", resolve: &apiclient.ResolveResult{}}

	if _, err := verifyRunAttestation(context.Background(), sig, "sha256:abc", false, false, io.Discard); err == nil || !strings.Contains(err.Error(), "unsigned") {
		t.Fatalf("unsigned release ran without consent: %v", err)
	}
	att, err := verifyRunAttestation(context.Background(), sig, "sha256:abc", true, false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if att.State != "unsigned" || att.Verified {
		t.Fatalf("accepted unsigned run not marked: %+v", att)
	}
}

// The unscanned-run consent: a version the registry admitted without a scan
// is its own outcome, read from resolve before any attestation is looked
// for; it refuses without AcceptUnscanned — accepting unsigned does not
// stand in — and, accepted, marks the receipt unscanned with the consent
// that admitted it. On a run from the cache the gate is asked again, the
// cache holds no consent, and an install cached as unsigned by an older
// desktop is unscanned once the registry says so.
func TestUnscannedIsItsOwnConsent(t *testing.T) {
	home := isolateRunEnv(t)
	fr := &fakeRunRegistry{unscanned: true}
	regURL := startRunRegistry(t, fr)
	sig := resolveFixture(t, regURL)
	if sig.resolve.ScanStatus == nil || string(*sig.resolve.ScanStatus) != "skipped" {
		t.Fatalf("the fixture's resolve does not say skipped: %+v", sig.resolve.ScanStatus)
	}

	var log strings.Builder
	if _, _, err := materialize(context.Background(), sig, home, false, false, &log); err == nil || !strings.Contains(err.Error(), "unscanned") {
		t.Fatalf("an unscanned release ran without consent: %v", err)
	}
	if _, _, err := materialize(context.Background(), sig, home, true, false, &log); err == nil || !strings.Contains(err.Error(), "unscanned") {
		t.Fatalf("accepting unsigned admitted an unscanned release: %v", err)
	}
	meta, installDir, err := materialize(context.Background(), sig, home, false, true, &log)
	if err != nil {
		t.Fatalf("with the consent: %v", err)
	}
	if meta.Attestation.State != "unscanned" || meta.Attestation.Verified || meta.Attestation.Consent != receipt.ConsentAllowUnscanned {
		t.Fatalf("the receipt's attestation = %+v", meta.Attestation)
	}
	if !strings.Contains(log.String(), "warning: unscanned") {
		t.Fatalf("an accepted unscanned run must say so:\n%s", log.String())
	}
	if cached, ok := installdir.Load(installDir, sig.Digest); !ok || cached.Attestation.State != "unscanned" || cached.Attestation.Consent != "" {
		t.Fatalf("the cache must hold the verdict and no consent: %+v", cached)
	}

	// From the cache: asked again, narrated, the consent this run's.
	log.Reset()
	if _, _, err := materialize(context.Background(), sig, home, true, false, &log); err == nil || !strings.Contains(err.Error(), "cached install") {
		t.Fatalf("a cached unscanned install ran on unsigned consent: %v", err)
	}
	log.Reset()
	again, _, err := materialize(context.Background(), sig, home, false, true, &log)
	if err != nil || again.Attestation.Consent != receipt.ConsentAllowUnscanned || !strings.Contains(log.String(), "cached install") {
		t.Fatalf("a cached unscanned run with the consent: %v, %+v, log %q", err, again.Attestation, log.String())
	}

	// A cache written before the state existed, so before integrity records
	// too: unsigned in the file, no record, and the registry says skipped
	// now — fetched again and unscanned, whatever the file says.
	stale := &installdir.Meta{Ref: sig.Ref, Namespace: sig.Namespace, Name: sig.Name, Version: sig.Version, Digest: sig.Digest, Source: "registry", Attestation: receipt.Attestation{State: "unsigned"}}
	raw, err := json.MarshalIndent(stale, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "install.json"), raw, 0o644); err != nil { //nolint:gosec // test fixture
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(installDir, installdir.RecordFile)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := materialize(context.Background(), sig, home, true, false, &log); err == nil || !strings.Contains(err.Error(), "unscanned") {
		t.Fatalf("a stale unsigned cache was admitted on unsigned consent against a registry that says skipped: %v", err)
	}
}
