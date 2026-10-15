package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/blaktron/agentry/pkg/apiclient"
)

// redirectingRegistry answers every request with a 307 to elsewhere's same
// path, and counts what reaches elsewhere.
func redirectingRegistry(t *testing.T) (reg string, reached *atomic.Int32) {
	t.Helper()
	reached = new(atomic.Int32)
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"1","apiVersion":"v1","features":{}}`))
	}))
	t.Cleanup(elsewhere.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(srv.Close)
	return srv.URL, reached
}

// A request carrying the stored token never follows a redirect, whatever it
// points at: Go keeps a hand-set Authorization on a same-domain redirect
// whatever the scheme, which would carry the token past the https rule.
// The generated client and the archive download both stop at the registry
// that was asked.
func TestATokenNeverFollowsARedirect(t *testing.T) {
	ctx := context.Background()
	reg, reached := redirectingRegistry(t)

	client, err := New(reg, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.GetCurrentUserWithResponse(ctx)
	if err != nil {
		t.Fatalf("a refused redirect is an answer, not a transport error: %v", err)
	}
	if res.StatusCode() != http.StatusTemporaryRedirect || reached.Load() != 0 {
		t.Fatalf("generated client: status %d, %d requests elsewhere; want the 307 and none", res.StatusCode(), reached.Load())
	}

	if _, err := Download(ctx, reg, "agy_test", &apiclient.ResolveResult{ArchiveUrl: "/api/v1/agents/a/b/versions/1.0.0/download"}); err == nil || reached.Load() != 0 {
		t.Fatalf("download: err %v, %d requests elsewhere; want a refusal and none", err, reached.Load())
	}
}

// A public read, which carries no token, still follows a redirect as
// net/http does by default: the rule is the token's, decided on
// purpose.
func TestAPublicReadStillFollowsARedirect(t *testing.T) {
	reg, reached := redirectingRegistry(t)
	client, err := New(reg, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.GetMetaWithResponse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode() != http.StatusOK || reached.Load() != 1 {
		t.Fatalf("status %d, %d requests elsewhere; want 200 from the redirect's target", res.StatusCode(), reached.Load())
	}
}

// An archive URL the registry names gets the token only on the registry's
// own origin: the same host over http, on another port, or another host
// gets the download without it.
func TestADownloadCarriesTheTokenOnlyToTheRegistrysOrigin(t *testing.T) {
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("archive"))
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	for _, c := range []struct {
		reg  string
		want string
	}{
		{srv.URL, "Bearer agy_test"},
		{srv.URL + "/", "Bearer agy_test"},
		{"https://" + host, ""},
		{"http://localhost" + host[strings.LastIndex(host, ":"):], ""},
	} {
		auth.Store("unset")
		if _, err := Download(context.Background(), c.reg, "agy_test", &apiclient.ResolveResult{ArchiveUrl: srv.URL + "/archive"}); err != nil {
			t.Fatalf("%s: %v", c.reg, err)
		}
		if got := auth.Load(); got != c.want {
			t.Errorf("registry %s: the archive at %s was sent %q, want %q", c.reg, srv.URL, got, c.want)
		}
	}
}

// The attestation an answer names gets the token only on the registry's
// own origin, as the archive does.
func TestAnAttestationCarriesTheTokenOnlyToTheRegistrysOrigin(t *testing.T) {
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")

	for _, c := range []struct {
		reg, att, want string
	}{
		{srv.URL, "/api/v1/agents/a/b/versions/1.0.0/attestation", "Bearer agy_test"},
		{srv.URL, srv.URL + "/att", "Bearer agy_test"},
		{"https://" + host, "http://" + host + "/att", ""},
		{"http://localhost" + host[strings.LastIndex(host, ":"):], srv.URL + "/att", ""},
	} {
		auth.Store("unset")
		VerifyAttestation(context.Background(), c.reg, "agy_test", c.att, "sha256:00")
		if got := auth.Load(); got != c.want {
			t.Errorf("registry %s, attestation %s: sent %q, want %q", c.reg, c.att, got, c.want)
		}
	}
}
