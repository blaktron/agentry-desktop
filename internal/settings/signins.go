package settings

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/idna"
)

// Sign-ins. A personal access token is a bearer credential: whichever host
// receives it can act as the account. The app therefore keeps at most two,
// each attached only to requests to the registry it was issued by:
//
//   - the SecureAgentry sign-in, for the SecureAgentry registry this build is
//     for (DefaultRegistryURL, when that is a SecureAgentry address);
//   - the self-hosted sign-in, for one registry the person added that is not
//     a SecureAgentry address.
//
// Any other registry, another SecureAgentry address or a second self-hosted
// one, gets no token from the store and cannot be signed in to. Registries
// are compared by origin (scheme, host and port), the scope the keychain's
// per-host entries already have, the same rules the CLI holds its
// SECUREAGENTRY_TOKEN and AGENTRY_TOKEN to.
const (
	SignInSecureAgentry = "secureagentry"
	SignInSelfHosted    = "self-hosted"
)

// secureAgentryDomain is SecureAgentry's registrable domain: the registry
// and every one of its environments are this host or a subdomain of it.
const secureAgentryDomain = "secureagentry.net"

// IsSecureAgentry reports whether regURL is a SecureAgentry address. A host
// that cannot be put in the ASCII form a request would use counts as one, so
// a self-hosted token is never sent to a name that might be SecureAgentry.
func IsSecureAgentry(regURL string) bool {
	u, err := url.Parse(strings.TrimSpace(regURL))
	if err != nil {
		return false
	}
	h, ok := lookupHost(u.Hostname())
	if !ok {
		return true
	}
	return h == secureAgentryDomain || strings.HasSuffix(h, "."+secureAgentryDomain)
}

// lookupHost is host as a request resolves it: lower-cased, without trailing
// dots, and a non-ASCII name in its ASCII (punycode) form, so that two
// spellings of one host compare equal. ok is false for a name that has no
// such form.
func lookupHost(host string) (string, bool) {
	h := strings.TrimRight(strings.ToLower(host), ".")
	for i := 0; i < len(h); i++ {
		if h[i] >= 0x80 {
			a, err := idna.Lookup.ToASCII(h)
			if err != nil {
				return "", false
			}
			return strings.TrimRight(a, "."), true
		}
	}
	return h, true
}

// Origin is regURL's scheme, host and port, lower-cased, with the scheme's
// default port dropped; "" for a URL with no host or a host with no ASCII
// form.
func Origin(regURL string) string {
	u, err := url.Parse(strings.TrimSpace(regURL))
	if err != nil || u.Host == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	host, ok := lookupHost(u.Hostname())
	if !ok || host == "" {
		return ""
	}
	port := u.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if port != "" {
		host += ":" + port
	}
	return scheme + "://" + host
}

// SameOrigin reports whether a and b share an origin. Two URLs with no
// origin are not the same.
func SameOrigin(a, b string) bool {
	oa := Origin(a)
	return oa != "" && oa == Origin(b)
}

// DisplayOrigin names a registry in a sentence: its origin, never a path or
// userinfo someone may have typed into the address.
func DisplayOrigin(regURL string) string {
	if o := Origin(regURL); o != "" {
		return o
	}
	return "this registry"
}

// SecureAgentryRegistry is the SecureAgentry registry this build is for, or
// "" when its default registry is not a SecureAgentry address.
func SecureAgentryRegistry() string {
	if IsSecureAgentry(DefaultRegistryURL) {
		return Normalize(DefaultRegistryURL)
	}
	return ""
}

// SelfHostedRegistry is the self-hosted registry the person added: the
// first registry in the list that is not a SecureAgentry address, "" when
// there is none.
func (s Settings) SelfHostedRegistry() string {
	for _, r := range s.Registries {
		if r.URL != "" && !IsSecureAgentry(r.URL) && Origin(r.URL) != "" {
			return Normalize(r.URL)
		}
	}
	return ""
}

// SignInFor names the sign-in that may be sent to regURL: its kind and the
// registry it is for. Both are "" when no sign-in this app keeps belongs to
// regURL's origin.
func (s Settings) SignInFor(regURL string) (kind, reg string) {
	if sa := SecureAgentryRegistry(); sa != "" && SameOrigin(regURL, sa) {
		return SignInSecureAgentry, sa
	}
	if IsSecureAgentry(regURL) {
		return "", ""
	}
	if self := s.SelfHostedRegistry(); self != "" && SameOrigin(regURL, self) {
		return SignInSelfHosted, self
	}
	return "", ""
}

// Admit says whether regURL may be added to the list and signed in to: the
// SecureAgentry registry this build is for, the self-hosted registry already
// added, or a self-hosted one when none is. The refusal says which sign-in
// stands in the way.
func (s Settings) Admit(regURL string) error {
	if Origin(regURL) == "" {
		return fmt.Errorf("%q is not a registry address", DisplayOrigin(regURL))
	}
	if IsSecureAgentry(regURL) {
		sa := SecureAgentryRegistry()
		if sa == "" {
			return fmt.Errorf("this build of Agentry Desktop is for no SecureAgentry registry, so it signs in to none, %s included", DisplayOrigin(regURL))
		}
		if !SameOrigin(regURL, sa) {
			return fmt.Errorf("this Agentry Desktop signs in to SecureAgentry at %s only, not %s", DisplayOrigin(sa), DisplayOrigin(regURL))
		}
		return nil
	}
	if self := s.SelfHostedRegistry(); self != "" && !SameOrigin(regURL, self) {
		return fmt.Errorf("this Agentry Desktop keeps one self-hosted registry, and it is %s: remove it in Settings first to use %s", DisplayOrigin(self), DisplayOrigin(regURL))
	}
	return nil
}
