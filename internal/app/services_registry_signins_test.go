package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// registryServer is a registry that knows one token, and records the
// Authorization of every request it is sent.
type registryServer struct {
	*httptest.Server
	requests atomic.Int32
	bearers  chan string
}

func newRegistryServer(t *testing.T, token string) *registryServer {
	t.Helper()
	rs := &registryServer{bearers: make(chan string, 64)}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.requests.Add(1)
		select {
		case rs.bearers <- r.Header.Get("Authorization"):
		default:
		}
		if r.URL.Path != "/api/v1/user" || r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"00000000-0000-0000-0000-000000000002","handle":"chris","email":"c@example.test","displayName":"Chris","emailVerified":true,"totpEnabled":false,"createdAt":"2026-01-02T03:04:05Z"}`))
	}))
	t.Cleanup(rs.Close)
	return rs
}

// twoSignInApp is a SettingsAPI for a build made for secureagentry.net, with
// its own config, keychain, home and runner home, and no schedules to
// reseal.
func twoSignInApp(t *testing.T) *SettingsAPI {
	t.Helper()
	keyring.MockInit()
	setUserHome(t, t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	was := settings.DefaultRegistryURL
	settings.DefaultRegistryURL = "https://secureagentry.net"
	t.Cleanup(func() { settings.DefaultRegistryURL = was })
	saved := resealSchedules
	resealSchedules = func(*App) {}
	t.Cleanup(func() { resealSchedules = saved })
	return &SettingsAPI{app: &App{ctx: context.Background()}, open: func(string) error {
		t.Error("a refused sign-in opened the browser")
		return nil
	}}
}

// The self-hosted sign-in is kept for its own registry: Settings says which
// sign-in each registry has, and its token is not the SecureAgentry one.
func TestSignInToASelfHostedRegistryIsItsOwnSignIn(t *testing.T) {
	s := twoSignInApp(t)
	self := newRegistryServer(t, "agy_selfhosted")
	if _, err := settings.StoreToken("https://secureagentry.net", "agy_secure"); err != nil {
		t.Fatal(err)
	}

	if _, err := s.SignIn(self.URL, "agy_selfhosted"); err != nil {
		t.Fatal(err)
	}
	v, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	if v.SecureAgentryRegistry != "https://secureagentry.net" || v.SelfHostedRegistry != self.URL {
		t.Errorf("the two sign-ins: %q and %q", v.SecureAgentryRegistry, v.SelfHostedRegistry)
	}
	kinds := map[string]string{}
	for _, r := range v.Registries {
		kinds[r.URL] = r.SignIn
		if !r.HasToken {
			t.Errorf("%s shows signed out", r.URL)
		}
	}
	if kinds["https://secureagentry.net"] != settings.SignInSecureAgentry || kinds[self.URL] != settings.SignInSelfHosted {
		t.Errorf("Settings labels the sign-ins %v", kinds)
	}
	if got := settings.TokenFor(self.URL); got != "agy_selfhosted" {
		t.Errorf("the self-hosted registry gets %q", got)
	}
	if got := settings.TokenFor("https://secureagentry.net"); got != "agy_secure" {
		t.Errorf("SecureAgentry gets %q", got)
	}
}

// A second self-hosted registry, or a SecureAgentry address the build is not
// for, is refused before anything is sent to it: no token reaches it to be
// checked, nothing is stored, and the browser does not open.
func TestASignInTheAppDoesNotKeepIsRefusedBeforeItIsSent(t *testing.T) {
	s := twoSignInApp(t)
	first := newRegistryServer(t, "agy_first")
	second := newRegistryServer(t, "agy_second")
	if _, err := s.SignIn(first.URL, "agy_first"); err != nil {
		t.Fatal(err)
	}

	_, err := s.SignIn(second.URL, "agy_second")
	if err == nil || !strings.Contains(err.Error(), "one self-hosted registry, and it is "+first.URL) {
		t.Fatalf("a second self-hosted sign-in: %v", err)
	}
	if _, err := s.SignInViaBrowser(second.URL); err == nil {
		t.Fatal("a browser sign-in to a second self-hosted registry started")
	}
	if _, err := s.AddRegistry(second.URL); err == nil {
		t.Fatal("a second self-hosted registry was added")
	}
	for _, url := range []string{"https://dev.secureagentry.net", "https://staging.secureagentry.net"} {
		if _, err := s.SignIn(url, "agy_first"); err == nil || !strings.Contains(err.Error(), "SecureAgentry at https://secureagentry.net only") {
			t.Errorf("SignIn(%s): %v", url, err)
		}
		if _, err := s.AddRegistry(url); err == nil {
			t.Errorf("%s was added", url)
		}
	}
	if n := second.requests.Load(); n != 0 {
		t.Errorf("the refused registry was sent %d request(s)", n)
	}
	if got := settings.TokenFor(second.URL); got != "" {
		t.Errorf("the refused registry gets %q", got)
	}

	// Removing the first lets the second in, with its own token only.
	if _, err := s.RemoveRegistry(first.URL); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignIn(second.URL, "agy_second"); err != nil {
		t.Fatal(err)
	}
	if got := settings.TokenFor(first.URL); got != "" {
		t.Errorf("the removed registry still gets %q", got)
	}
	for b := range drain(second.bearers) {
		if b != "" && b != "Bearer agy_second" {
			t.Errorf("the second registry was sent %q", b)
		}
	}
}

// A listed registry that is neither sign-in shows why the app will not sign
// in to it, and gets no token the CLI stored for it.
func TestSettingsSaysWhyARegistryHasNoSignIn(t *testing.T) {
	s := twoSignInApp(t)
	cfg, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Upsert(settings.Registry{URL: "https://a.example"})
	cfg.Upsert(settings.Registry{URL: "https://b.example"})
	cfg.Upsert(settings.Registry{URL: "https://dev.secureagentry.net"})
	if err := settings.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := keyring.Set(settings.KeyringService, "b.example", "agy_cli"); err != nil {
		t.Fatal(err)
	}
	v, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range v.Registries {
		switch r.URL {
		case "https://secureagentry.net":
			if r.SignIn != settings.SignInSecureAgentry {
				t.Errorf("%s: %+v", r.URL, r)
			}
		case "https://a.example":
			if r.SignIn != settings.SignInSelfHosted {
				t.Errorf("%s: %+v", r.URL, r)
			}
		case "https://b.example":
			if r.SignIn != "" || r.HasToken || !strings.Contains(r.SignInRefused, "one self-hosted registry, and it is https://a.example") {
				t.Errorf("%s: %+v", r.URL, r)
			}
		case "https://dev.secureagentry.net":
			if r.SignIn != "" || !strings.Contains(r.SignInRefused, "SecureAgentry at https://secureagentry.net only") {
				t.Errorf("%s: %+v", r.URL, r)
			}
		}
	}
}

func drain(c chan string) map[string]bool {
	seen := map[string]bool{}
	for {
		select {
		case b := <-c:
			seen[b] = true
		default:
			return seen
		}
	}
}
