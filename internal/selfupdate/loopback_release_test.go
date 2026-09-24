//go:build !agentry_dev_loopback

package selfupdate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/selfupdate/keystest"
)

// A shipped build makes no exception for this machine: a plain-http channel
// on a loopback address is refused like any other, before anything is
// fetched (agentry-desktop#127). Until then localhost and 127.0.0.1 were
// exempt in every build.
func TestCheckRefusesAPlainHTTPLoopbackChannel(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	_, entry := keystest.Key(t, "prod", false)
	for _, host := range []string{"127.0.0.1", "localhost", "[::1]", "127.1.2.3"} {
		u := "http://" + host + ":" + port + "/downloads/manifest.json"
		_, err := selfupdate.Check(context.Background(), srv.Client(), u, "0.1.0", keystest.Set(t, entry))
		if err == nil || !strings.Contains(err.Error(), "not https") {
			t.Errorf("%s: err = %v, want the plain-http loopback channel refused", u, err)
		}
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("the refused channel was fetched %d times; the scheme is checked before any request", n)
	}
}
