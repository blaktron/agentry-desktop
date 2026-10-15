package settings

import (
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// buildFor makes this test run as a build for reg, as a release build's
// -X DefaultRegistryURL=… does.
func buildFor(t *testing.T, reg string) {
	t.Helper()
	was := DefaultRegistryURL
	DefaultRegistryURL = reg
	t.Cleanup(func() { DefaultRegistryURL = was })
}

// keychainHolds puts each host's token in an in-memory keychain, the way
// either client's sign-in leaves it.
func keychainHolds(t *testing.T, tokens map[string]string) {
	t.Helper()
	keyring.MockInit()
	for host, tok := range tokens {
		if err := keyring.Set(KeyringService, host, tok); err != nil {
			t.Fatal(err)
		}
	}
}

// saveRegistries writes the list Settings shows.
func saveRegistries(t *testing.T, urls ...string) {
	t.Helper()
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Registries = nil
	for _, u := range urls {
		s.Upsert(Registry{URL: u})
	}
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
}

const (
	saToken    = "agy_securetoken0000"
	selfToken  = "agy_selfhosted0000"
	devToken   = "agy_devtoken000000"
	otherToken = "agy_othertoken0000"
)

// Each sign-in goes to its own registry's origin, however that origin is
// spelled, and to nothing else: not the other sign-in's registry, not
// another SecureAgentry environment, not a host the CLI signed in to that
// the app keeps no sign-in for.
func TestEachSignInGoesOnlyToItsOwnRegistry(t *testing.T) {
	isolate(t)
	buildFor(t, "https://secureagentry.net")
	keychainHolds(t, map[string]string{
		"secureagentry.net":     saToken,
		"agentry.example.com":   selfToken,
		"dev.secureagentry.net": devToken,
		"other.example":         otherToken,
	})
	saveRegistries(t, "https://secureagentry.net", "https://agentry.example.com", "https://other.example")

	for url, want := range map[string]string{
		// The SecureAgentry sign-in.
		"https://secureagentry.net":           saToken,
		"https://secureagentry.net/":          saToken,
		"https://SecureAgentry.NET.":          saToken,
		"https://secureagentry.net:443":       saToken,
		"https://secureagentry.net/api/v1/x":  saToken,
		"https://secureagentry\u3002net":      saToken,
		"http://secureagentry.net":            "",
		"https://secureagentry.net:8443":      "",
		"https://dev.secureagentry.net":       "",
		"https://secureagentry.net.evil.test": "",
		"https://evil-secureagentry.net":      "",
		// The self-hosted sign-in.
		"https://agentry.example.com":      selfToken,
		"https://AGENTRY.example.com./":    selfToken,
		"https://agentry.example.com:8443": "",
		"http://agentry.example.com":       "",
		"https://sub.agentry.example.com":  "",
		// A registry in the list (from before) that is neither sign-in,
		// and one only the CLI knows.
		"https://other.example":    "",
		"https://unlisted.example": "",
	} {
		if got := TokenFor(url); got != want {
			t.Errorf("TokenFor(%q) = %q, want %q", url, got, want)
		}
	}
}

// A build for a SecureAgentry environment signs in to that environment and
// to no other: a dev build does not send its sign-in to production, and
// production's stored token does not reach it.
func TestADevBuildSignsInToDevOnly(t *testing.T) {
	isolate(t)
	buildFor(t, "https://dev.secureagentry.net")
	keychainHolds(t, map[string]string{"secureagentry.net": saToken, "dev.secureagentry.net": devToken})
	saveRegistries(t, "https://dev.secureagentry.net", "https://secureagentry.net")

	if got := TokenFor("https://dev.secureagentry.net"); got != devToken {
		t.Errorf("dev: %q, want the dev sign-in", got)
	}
	if got := TokenFor("https://secureagentry.net"); got != "" {
		t.Errorf("production from a dev build: %q, want none", got)
	}
	s, _ := Load()
	if s.SelfHostedRegistry() != "" {
		t.Errorf("a SecureAgentry address counted as self-hosted: %q", s.SelfHostedRegistry())
	}
}

// A build whose default registry is not a SecureAgentry address keeps no
// SecureAgentry sign-in at all, and its own default can be the self-hosted
// one.
func TestABuildForNoSecureAgentryRegistrySignsInToNone(t *testing.T) {
	isolate(t)
	buildFor(t, "http://localhost:8080")
	keychainHolds(t, map[string]string{"secureagentry.net": saToken, "localhost:8080": selfToken})

	if SecureAgentryRegistry() != "" {
		t.Fatalf("SecureAgentryRegistry() = %q, want none", SecureAgentryRegistry())
	}
	if got := TokenFor("https://secureagentry.net"); got != "" {
		t.Errorf("secureagentry.net: %q, want none", got)
	}
	if got := TokenFor("http://localhost:8080"); got != selfToken {
		t.Errorf("the default as the self-hosted registry: %q, want its sign-in", got)
	}
	s, _ := Load()
	if err := s.Admit("https://secureagentry.net"); err == nil || !strings.Contains(err.Error(), "no SecureAgentry registry") {
		t.Errorf("Admit(secureagentry.net) = %v, want the build-for-none refusal", err)
	}
}

// The app adds, and signs in to, the SecureAgentry registry it is built for
// and one self-hosted registry; a second of either is refused with the
// sign-in that stands in the way, and only removing the first lets another
// in.
func TestAdmitKeepsOneSignInOfEachKind(t *testing.T) {
	isolate(t)
	buildFor(t, "https://secureagentry.net")
	keyring.MockInit()
	saveRegistries(t, "https://secureagentry.net")
	s, _ := Load()

	for url, wantErr := range map[string]string{
		"https://secureagentry.net":     "",
		"https://SECUREAGENTRY.net:443": "",
		"https://dev.secureagentry.net": "signs in to SecureAgentry at https://secureagentry.net only",
		"https://a.example":             "",
		"ftp://":                        "not a registry address",
	} {
		err := s.Admit(url)
		if (err == nil) != (wantErr == "") || (err != nil && !strings.Contains(err.Error(), wantErr)) {
			t.Errorf("Admit(%q) = %v, want %q", url, err, wantErr)
		}
	}

	s.Upsert(Registry{URL: "https://a.example"})
	if err := s.Admit("https://A.example/"); err != nil {
		t.Errorf("the self-hosted registry again: %v", err)
	}
	err := s.Admit("https://b.example")
	if err == nil || !strings.Contains(err.Error(), "one self-hosted registry, and it is https://a.example") {
		t.Errorf("a second self-hosted registry: %v", err)
	}
	s.Remove("https://a.example")
	if err := s.Admit("https://b.example"); err != nil {
		t.Errorf("after removing the first: %v", err)
	}
}

// A sign-in is stored under its own registry's host only: storing for a
// registry the app keeps no sign-in for is refused, and a token stored for
// the new self-hosted registry after the old one was removed is that
// registry's alone, so switching never carries the other's token.
func TestStoreTokenKeepsTheTwoSignInsApart(t *testing.T) {
	isolate(t)
	buildFor(t, "https://secureagentry.net")
	keychainHolds(t, map[string]string{})
	saveRegistries(t, "https://secureagentry.net", "https://a.example")

	if _, err := StoreToken("https://SecureAgentry.net/", saToken); err != nil {
		t.Fatal(err)
	}
	if got, _ := keyring.Get(KeyringService, "secureagentry.net"); got != saToken {
		t.Errorf("stored under %q, not the registry's own host", got)
	}
	if _, err := StoreToken("https://a.example", selfToken); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"https://dev.secureagentry.net", "https://b.example"} {
		if _, err := StoreToken(url, otherToken); err == nil {
			t.Errorf("StoreToken(%q) stored a sign-in the app does not keep", url)
		}
	}
	if _, err := keyring.Get(KeyringService, "b.example"); err == nil {
		t.Error("a refused store reached the keychain")
	}

	// Switch the self-hosted registry: remove a, add b.
	if err := DeleteToken("https://a.example"); err != nil {
		t.Fatal(err)
	}
	s, _ := Load()
	s.Remove("https://a.example")
	s.Upsert(Registry{URL: "https://b.example"})
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	if got := TokenFor("https://b.example"); got != "" {
		t.Errorf("the new self-hosted registry got %q before its own sign-in", got)
	}
	if got := TokenFor("https://secureagentry.net"); got != saToken {
		t.Errorf("the SecureAgentry sign-in after the switch: %q", got)
	}
	if got := TokenFor("https://a.example"); got != "" {
		t.Errorf("the removed registry still gets %q", got)
	}
}

// The file fallback, used when there is no keychain, is scoped the same way.
func TestTheFileFallbackIsScopedLikeTheKeychain(t *testing.T) {
	isolate(t)
	buildFor(t, "https://secureagentry.net")
	keychainHolds(t, map[string]string{})
	s, _ := Load()
	s.Registries = []Registry{
		{URL: "https://secureagentry.net", Token: saToken},
		{URL: "https://a.example", Token: selfToken},
		{URL: "https://b.example", Token: otherToken},
		{URL: "https://dev.secureagentry.net", Token: devToken},
	}
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	for url, want := range map[string]string{
		"https://secureagentry.net":     saToken,
		"https://a.example/":            selfToken,
		"https://b.example":             "",
		"https://dev.secureagentry.net": "",
	} {
		if got := TokenFor(url); got != want {
			t.Errorf("TokenFor(%q) = %q, want %q", url, got, want)
		}
		if src, wantSrc := TokenSource(url), map[bool]string{true: "file", false: ""}[want != ""]; src != wantSrc {
			t.Errorf("TokenSource(%q) = %q, want %q", url, src, wantSrc)
		}
	}
}

// Hosts are compared the way a request resolves them, and a host with no
// ASCII form counts as SecureAgentry, so a self-hosted token is never sent
// to a name that might be it.
func TestOriginsCompareAsRequestsResolveThem(t *testing.T) {
	for _, c := range []struct {
		a, b string
		same bool
	}{
		{"https://B\u00fccher.example", "https://xn--bcher-kva.example/", true},
		{"https://x.example:443", "https://x.example", true},
		{"http://localhost:80", "http://localhost", true},
		{"http://x.example", "https://x.example", false},
		{"https://x.example:8443", "https://x.example", false},
		{"https://[::1]:8080", "https://[::1]:8080/api", true},
		{"https://", "https://", false},
	} {
		if got := SameOrigin(c.a, c.b); got != c.same {
			t.Errorf("SameOrigin(%q, %q) = %v, want %v", c.a, c.b, got, c.same)
		}
	}
	for url, want := range map[string]bool{
		"https://secureagentry.net":       true,
		"https://dev.secureagentry.net":   true,
		"https://secureagentry\u3002net":  true,
		"https://a\u200d.example":         true,
		"https://secureagentry.net.x.com": false,
		"https://xsecureagentry.net":      false,
		"https://agentry.example.com":     false,
	} {
		if got := IsSecureAgentry(url); got != want {
			t.Errorf("IsSecureAgentry(%q) = %v, want %v", url, got, want)
		}
	}
}
