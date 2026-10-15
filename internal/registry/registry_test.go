package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
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
