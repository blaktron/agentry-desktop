package hostedrun

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// The runs client carries the account's token on clients of its own, so
// they hold the same rule: a registry that answers 307 to another
// server gets the refusal on the JSON calls and on the output download,
// and the other server sees nothing.
func TestTheRunsClientNeverFollowsARedirect(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhere.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	c := New(srv.URL, "agy_test")
	if _, err := c.List(context.Background()); err == nil {
		t.Fatal("List followed the redirect or answered nothing")
	}
	if rc, err := c.Output(context.Background(), "run-1", "output.tar"); err == nil {
		_ = rc.Close()
		t.Fatal("Output followed the redirect or answered nothing")
	}
	if n := elsewhere.Load(); n != 0 {
		t.Fatalf("%d requests reached the redirect's target with the token", n)
	}
}
