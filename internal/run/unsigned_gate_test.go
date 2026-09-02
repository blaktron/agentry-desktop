package run

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/apiclient"
)

// The unsigned-run consent is enforced in Go (desktop#3, D-011 closure): a
// registry with no attestation refuses the run unless AcceptUnsigned was
// given, and then marks the receipt unsigned.
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

	if _, err := verifyRunAttestation(context.Background(), sig, "sha256:abc", false, io.Discard); err == nil || !strings.Contains(err.Error(), "unsigned") {
		t.Fatalf("unsigned release ran without consent: %v", err)
	}
	att, err := verifyRunAttestation(context.Background(), sig, "sha256:abc", true, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if att.State != "unsigned" || att.Verified {
		t.Fatalf("accepted unsigned run not marked: %+v", att)
	}
}
