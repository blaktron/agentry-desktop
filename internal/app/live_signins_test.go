package app

// A live check of the app's two registry sign-ins against real registries,
// through the bound methods Settings calls. Skipped unless
// AGENTRY_LIVE_SIGNINS names the SecureAgentry registry to build for (for
// example https://dev.secureagentry.net) and AGENTRY_LIVE_SIGNINS_TOKEN holds
// a personal access token for it. AGENTRY_LIVE_SIGNINS_SELFHOSTED names a
// self-hosted registry the same token signs in to (an http://localhost port
// forwarded to a registry will do), and AGENTRY_LIVE_SIGNINS_OTHER a second
// address of one, refused as a second self-hosted registry. The keychain is
// held in memory, so the person's own sign-ins are neither read nor changed.

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// sentTo records the origin of every request and whether it carried a
// token.
type sentTo struct {
	next http.RoundTripper
	mu   sync.Mutex
	auth map[string]int
	anon map[string]int
}

func (s *sentTo) RoundTrip(r *http.Request) (*http.Response, error) {
	o := settings.Origin(r.URL.String())
	s.mu.Lock()
	if r.Header.Get("Authorization") != "" {
		s.auth[o]++
	} else {
		s.anon[o]++
	}
	s.mu.Unlock()
	return s.next.RoundTrip(r)
}

func TestLiveTwoSignIns(t *testing.T) {
	sa := settings.Normalize(os.Getenv("AGENTRY_LIVE_SIGNINS"))
	token := os.Getenv("AGENTRY_LIVE_SIGNINS_TOKEN")
	if sa == "" || token == "" {
		t.Skip("set AGENTRY_LIVE_SIGNINS and AGENTRY_LIVE_SIGNINS_TOKEN to sign in to a real registry")
	}
	self := settings.Normalize(os.Getenv("AGENTRY_LIVE_SIGNINS_SELFHOSTED"))
	other := settings.Normalize(os.Getenv("AGENTRY_LIVE_SIGNINS_OTHER"))

	keyring.MockInit()
	setUserHome(t, t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	was := settings.DefaultRegistryURL
	settings.DefaultRegistryURL = sa
	t.Cleanup(func() { settings.DefaultRegistryURL = was })
	saved := resealSchedules
	resealSchedules = func(*App) {}
	t.Cleanup(func() { resealSchedules = saved })
	rec := &sentTo{next: http.DefaultTransport, auth: map[string]int{}, anon: map[string]int{}}
	prev := registry.HTTPClient.Transport
	registry.HTTPClient.Transport = rec
	t.Cleanup(func() { registry.HTTPClient.Transport = prev })

	s := &SettingsAPI{app: &App{ctx: context.Background()}, open: func(string) error {
		t.Error("a sign-in opened the browser")
		return nil
	}}

	res, err := s.SignIn(sa, token)
	if err != nil {
		t.Fatalf("sign in to %s: %v", sa, err)
	}
	t.Logf("signed in to %s as %s (%s)", sa, res.User.Handle, res.TokenStore)
	if u, err := s.Whoami(sa); err != nil || u.Handle != res.User.Handle {
		t.Fatalf("whoami with the stored SecureAgentry sign-in: %v, %v", u, err)
	}

	// Another SecureAgentry address is refused before anything is sent.
	for _, addr := range []string{"https://secureagentry.net", "https://dev.secureagentry.net"} {
		if settings.SameOrigin(addr, sa) {
			continue
		}
		if _, err := s.SignIn(addr, token); err == nil {
			t.Errorf("signed in to %s from a build for %s", addr, sa)
		} else {
			t.Logf("refused %s: %v", addr, err)
		}
		if _, err := s.SignInViaBrowser(addr); err == nil {
			t.Errorf("a browser sign-in to %s started", addr)
		}
		if got := settings.TokenFor(addr); got != "" {
			t.Errorf("%s gets a token", addr)
		}
	}

	if self != "" {
		if _, err := s.SignIn(self, token); err != nil {
			t.Fatalf("sign in to the self-hosted %s: %v", self, err)
		}
		if _, err := s.Whoami(self); err != nil {
			t.Fatalf("whoami with the stored self-hosted sign-in: %v", err)
		}
		if other != "" {
			if _, err := s.SignIn(other, token); err == nil || !strings.Contains(err.Error(), "one self-hosted registry") {
				t.Errorf("a second self-hosted registry %s: %v", other, err)
			} else {
				t.Logf("refused %s: %v", other, err)
			}
		}
	}

	v, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range v.Registries {
		t.Logf("Settings: %s — %s, signed in %v (%s) %s", r.URL, r.SignIn, r.HasToken, r.TokenSource, r.SignInRefused)
	}

	allowed := map[string]bool{settings.Origin(sa): true}
	if self != "" {
		allowed[settings.Origin(self)] = true
	}
	for o, n := range rec.auth {
		if !allowed[o] {
			t.Errorf("%d request(s) carried a token to %s", n, o)
		}
		t.Logf("with a token: %s ×%d", o, n)
	}
	for o, n := range rec.anon {
		t.Logf("without a token: %s ×%d", o, n)
	}
	for _, addr := range []string{"https://secureagentry.net", other} {
		if addr == "" || settings.SameOrigin(addr, sa) {
			continue
		}
		if n := rec.auth[settings.Origin(addr)] + rec.anon[settings.Origin(addr)]; n != 0 {
			t.Errorf("the refused %s was sent %d request(s)", addr, n)
		}
	}
}
