// Package settings is the desktop app's own configuration: the list of known
// registries and the default one, stored at ~/.config/agentry/desktop.yaml
// (AGENTRY_CONFIG_DIR relocates it, like every other agentry client file).
//
// Credentials are NOT desktop-specific: PATs live in the OS keychain under
// the same service/account convention the CLI uses (service "agentry",
// account = registry host), so signing in from either client signs in both.
// When no keychain is available the token falls back into this file, 0600 —
// the same posture as the CLI's config-file fallback.
//
// This file is deliberately separate from the CLI's config.yaml: two programs
// upserting one YAML file with different feature sets is how a config file
// gets corrupted, and the credential interop happens in the keychain anyway.
package settings

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
	"gopkg.in/yaml.v3"

	"github.com/blaktron/agentry-cli/pkg/configdir"
)

// DefaultRegistryURL is the production registry (§1: base URLs must stay
// configurable — this is only the out-of-the-box value).
// A var, not a const, so a release build can bake its own registry in
// (-X .../internal/settings.DefaultRegistryURL=…) — the desktop app served
// from a site's /download page starts pointed at that site.
var DefaultRegistryURL = "https://secureagentry.net"

// keyringService matches the CLI's go-keyring service name; the account key
// is the registry host, so multiple registries coexist and both clients see
// the same credential.
const keyringService = "agentry"

// Registry is one configured registry.
type Registry struct {
	// URL is the registry base URL (no trailing slash).
	URL string `yaml:"url" json:"url"`
	// Token holds the PAT ONLY when the OS keychain was unavailable at
	// sign-in (file fallback, 0600). Never serialized to the frontend.
	Token string `yaml:"token,omitempty" json:"-"`
}

// Settings is ~/.config/agentry/desktop.yaml.
type Settings struct {
	DefaultRegistry string     `yaml:"defaultRegistry,omitempty" json:"defaultRegistry"`
	Registries      []Registry `yaml:"registries,omitempty" json:"registries"`
}

// Path returns the settings file path.
func Path() (string, error) {
	dir, err := configdir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "desktop.yaml"), nil
}

// Load reads the settings; a missing file yields the shippable default
// (the production registry, nothing signed in).
func Load() (Settings, error) {
	path, err := Path()
	if err != nil {
		return Settings{}, err
	}
	raw, err := os.ReadFile(path) //nolint:gosec // fixed, user-owned config path
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{
				DefaultRegistry: DefaultRegistryURL,
				Registries:      []Registry{{URL: DefaultRegistryURL}},
			}, nil
		}
		return Settings{}, fmt.Errorf("read %s: %w", path, err)
	}
	var s Settings
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return Settings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

// Save writes the settings file with 0600 permissions (it may carry a PAT
// when the keychain is unavailable).
func Save(s Settings) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	raw, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}

// Normalize canonicalizes a registry URL for storage and comparison.
func Normalize(regURL string) string {
	return strings.TrimRight(strings.TrimSpace(regURL), "/")
}

// Find returns the entry matching regURL (trailing slashes ignored), or nil.
func (s *Settings) Find(regURL string) *Registry {
	want := Normalize(regURL)
	for i := range s.Registries {
		if Normalize(s.Registries[i].URL) == want {
			return &s.Registries[i]
		}
	}
	return nil
}

// Upsert replaces the entry with e.URL or appends a new one.
func (s *Settings) Upsert(e Registry) {
	e.URL = Normalize(e.URL)
	if cur := s.Find(e.URL); cur != nil {
		*cur = e
		return
	}
	s.Registries = append(s.Registries, e)
}

// Remove deletes the entry for regURL, reporting whether one was present.
func (s *Settings) Remove(regURL string) bool {
	want := Normalize(regURL)
	for i := range s.Registries {
		if Normalize(s.Registries[i].URL) == want {
			s.Registries = append(s.Registries[:i], s.Registries[i+1:]...)
			if Normalize(s.DefaultRegistry) == want {
				s.DefaultRegistry = ""
				if len(s.Registries) > 0 {
					s.DefaultRegistry = s.Registries[0].URL
				}
			}
			return true
		}
	}
	return false
}

// Host extracts the host (the keyring account key) from a registry URL.
func Host(regURL string) (string, error) {
	u, err := url.Parse(regURL)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("invalid registry URL %q", regURL)
	}
	return u.Host, nil
}

// ValidateURL rejects anything that is not a plain http(s) base URL.
func ValidateURL(regURL string) error {
	u, err := url.Parse(regURL)
	if err != nil {
		return fmt.Errorf("invalid registry URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("registry URL must be http(s), got %q", regURL)
	}
	if u.Host == "" {
		return fmt.Errorf("registry URL %q has no host", regURL)
	}
	return nil
}

// TokenFor resolves the PAT for a registry: the OS keychain first, then the
// settings-file fallback. Empty means anonymous.
func TokenFor(regURL string) string {
	host, err := Host(regURL)
	if err == nil {
		if token, err := keyring.Get(keyringService, host); err == nil && token != "" {
			return token
		}
	}
	if s, err := Load(); err == nil {
		if e := s.Find(regURL); e != nil && e.Token != "" {
			return e.Token
		}
	}
	return ""
}

// TokenSource reports where a credential for regURL currently lives:
// "keyring", "file", or "" (none).
func TokenSource(regURL string) string {
	host, err := Host(regURL)
	if err == nil {
		if token, err := keyring.Get(keyringService, host); err == nil && token != "" {
			return "keyring"
		}
	}
	if s, err := Load(); err == nil {
		if e := s.Find(regURL); e != nil && e.Token != "" {
			return "file"
		}
	}
	return ""
}

// StoreToken saves a PAT for a registry: keychain first, settings-file
// fallback. It returns where the token landed ("keyring" or "file").
func StoreToken(regURL, token string) (string, error) {
	host, err := Host(regURL)
	if err != nil {
		return "", err
	}
	if kerr := keyring.Set(keyringService, host, token); kerr == nil {
		// A stale file-fallback copy must not linger once the keychain works.
		s, err := Load()
		if err == nil {
			if e := s.Find(regURL); e != nil && e.Token != "" {
				e.Token = ""
				if err := Save(s); err != nil {
					return "keyring", fmt.Errorf("token stored in keychain, but clearing the file fallback failed: %w", err)
				}
			}
		}
		return "keyring", nil
	}
	s, err := Load()
	if err != nil {
		return "", err
	}
	e := s.Find(regURL)
	if e == nil {
		s.Upsert(Registry{URL: regURL})
		e = s.Find(regURL)
	}
	e.Token = token
	if err := Save(s); err != nil {
		return "", err
	}
	return "file", nil
}

// DeleteToken removes the stored PAT for a registry from both stores.
func DeleteToken(regURL string) error {
	if host, err := Host(regURL); err == nil {
		// ErrNotFound is fine; any other keychain error still lets the file
		// fallback be cleared below.
		_ = keyring.Delete(keyringService, host)
	}
	s, err := Load()
	if err != nil {
		return err
	}
	if e := s.Find(regURL); e != nil && e.Token != "" {
		e.Token = ""
		return Save(s)
	}
	return nil
}
