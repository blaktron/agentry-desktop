//go:build agentry_dev_loopback

package selfupdate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/selfupdate/keystest"
)

// With the agentry_dev_loopback tag, and only then, a plain-http channel on
// this machine gets past the scheme rule (it then fails here on the empty
// channel, which is the point: it was fetched), while a remote one is still
// refused. Run with: go test -tags agentry_dev_loopback.
func TestDevBuildAcceptsAPlainHTTPLoopbackChannelOnly(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	_, entry := keystest.Key(t, "prod", false)
	_, err := selfupdate.Check(context.Background(), srv.Client(), srv.URL+"/downloads/manifest.json", "0.1.0", keystest.Set(t, entry))
	if err != nil && strings.Contains(err.Error(), "not https") {
		t.Fatalf("dev build refused a loopback http channel: %v", err)
	}
	_, err = selfupdate.Check(context.Background(), nil, "http://downloads.example.test/manifest.json", "0.1.0", keystest.Set(t, entry))
	if err == nil || !strings.Contains(err.Error(), "not https") {
		t.Fatalf("dev build: err = %v, want a remote plain-http channel refused", err)
	}
}
