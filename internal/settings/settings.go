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
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// KeyringService matches the CLI's go-keyring service name; the account key
// is the registry host, so multiple registries coexist and both clients see
// the same credential. The CLI's model keys live under the same service as
// provider:<name> (its DR-24), so the service names everything an uninstall
// with "also remove my sign-in" takes (ForgetKeychain).
const KeyringService = "agentry"

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
	// Onboarded is set once the first-run wizard has been completed or
	// explicitly skipped. While it is false and no registry is signed in,
	// the desktop app opens the guided setup on start (agentry-desktop#41).
	Onboarded bool `yaml:"onboarded,omitempty" json:"onboarded"`
	// CheckForUpdates is Settings → "Check for newer versions"
	// (agentry-desktop#87). A POINTER because the default is on and this file
	// is parsed leniently: absent must mean "on", and a plain bool cannot
	// tell an absent field from a deliberate false. Read it through
	// UpdateCheckEnabled rather than dereferencing it.
	CheckForUpdates *bool `yaml:"checkForUpdates,omitempty" json:"checkForUpdates,omitempty"`
	// InstallUpdatesOnQuit is Settings → "Install automatically when I quit"
	// (agentry-desktop#88, plan D6). A plain bool, not a pointer, because
	// this one defaults OFF: absent and false mean the same thing, and the
	// operator's D6 asked for off by default so that nothing replaces the
	// application without the person having said so at least once.
	InstallUpdatesOnQuit bool `yaml:"installUpdatesOnQuit,omitempty" json:"installUpdatesOnQuit,omitempty"`
	// InstallIntoCodingAgent is Settings → Coding agents → "Also install
	// into <coding agent>" (agentry-desktop#148, plan
	// desktop-one-click-install.md D6): with it on, a one-click Install
	// also writes the coding-agent export — the Claude skill or AGENTS.md,
	// whichever the operator policy implies — beside the runner-home
	// install. A plain bool because it defaults OFF: nothing is written
	// outside the runner home until the person has said so once.
	InstallIntoCodingAgent bool `yaml:"installIntoCodingAgent,omitempty" json:"installIntoCodingAgent,omitempty"`
	// CodingAgentProjectDir is the project directory the AGENTS.md export
	// is written to (<dir>/AGENTS.md and <dir>/agents/<name>, D-002),
	// chosen through the native picker and nowhere else (D-030); "" means
	// none chosen yet. The Claude skill has no such setting: it lives in
	// ~/.claude/skills, where Claude Code reads.
	CodingAgentProjectDir string `yaml:"codingAgentProjectDir,omitempty" json:"codingAgentProjectDir,omitempty"`
	// RunWhere is Settings → Run defaults → Where to run (plan
	// settings-cards.md D2, D13; agentry-desktop#347): RunWhereHere or
	// RunWhereAgentry, "" meaning this computer. It only pre-selects the run
	// setup's Where step; the credit hold and every consent are still asked
	// on each run.
	RunWhere string `yaml:"runWhere,omitempty" json:"runWhere,omitempty"`
	// DefaultOutputDir is Settings → Run defaults → Output folder, chosen
	// through the native picker on that page and nowhere else, "" for none.
	// A run's gate admits it as an output folder because it is read from
	// this file, never as an input root (D-044).
	DefaultOutputDir string `yaml:"defaultOutputDir,omitempty" json:"defaultOutputDir,omitempty"`
	// ChatIdleTimeoutSeconds is Settings → Run defaults → Chat idle timeout
	// (plan desktop-review-key-and-run-cards.md D21, D32;
	// agentry-desktop#468): a chat on this computer ends after this much
	// silence from the person. 0, the default, is none: such a chat has no
	// idle timeout, wall clock or count of the person's turns, and ends when
	// the person ends it or quits. It lives here, not in the runner policy,
	// which the CLI's chats read and which keeps their limits.
	ChatIdleTimeoutSeconds int `yaml:"chatIdleTimeoutSeconds,omitempty" json:"chatIdleTimeoutSeconds,omitempty"`
	// UnsafeMode is Settings → Run defaults → Unsafe Mode (plan
	// unsafe-mode.md D5, D6; agentry-desktop#418): while it is on, a run on
	// this computer is compiled without this machine's exec catalogue, its
	// declared commands installed from the Unsafe Mode image instead. A
	// plain bool because it defaults OFF. It lives in this file and never in
	// the runner policy, so the CLI and a run In Agentry cannot read it.
	// Save does not write it: only SetUnsafeMode does, which the app calls
	// after a native confirmation and the image's pull (D-030: the page
	// cannot turn it on, and no other setting's save can carry it).
	UnsafeMode bool `yaml:"unsafeMode,omitempty" json:"unsafeMode,omitempty"`
}

// MaxChatIdleTimeoutSeconds is the longest Chat idle timeout, a week: a
// chat that should wait longer wants none.
const MaxChatIdleTimeoutSeconds = 7 * 24 * 60 * 60

// CheckChatIdleTimeout says whether seconds is a Chat idle timeout: 0 for
// none, or 1 second to a week. The setting's save refuses anything else, and
// a chat refuses to start on a desktop.yaml edited by hand to hold it.
func CheckChatIdleTimeout(seconds int) error {
	if seconds < 0 || seconds > MaxChatIdleTimeoutSeconds {
		return fmt.Errorf("a chat idle timeout is none or 1 second to a week (%d seconds), not %d seconds", MaxChatIdleTimeoutSeconds, seconds)
	}
	return nil
}

// Where a run goes when Run defaults says nothing else.
const (
	RunWhereHere    = "here"
	RunWhereAgentry = "agentry"
)

// UpdateCheckEnabled says whether the app should look for a newer Agentry
// Desktop. Unset means yes: a person who has never opened Settings should
// still be told a release exists.
func (s Settings) UpdateCheckEnabled() bool {
	return s.CheckForUpdates == nil || *s.CheckForUpdates
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

// saveMu serializes this process's writes of the settings file, so the
// Unsafe Mode switch a write keeps is the one on disk at that moment.
var saveMu sync.Mutex

// Save writes the settings file with 0600 permissions (it may carry a PAT
// when the keychain is unavailable).
//
// It never changes the Unsafe Mode switch: whatever s.UnsafeMode says, the
// file keeps the value it has. Every other setting is saved by a binding
// that loads, changes one field and saves, with no lock across the three;
// were the switch saved the same way, a save that loaded it on before it
// was turned off would turn it back on with no confirmation (the review of
// agentry-desktop#418). SetUnsafeMode is its one writer.
func Save(s Settings) error {
	saveMu.Lock()
	defer saveMu.Unlock()
	cur, err := Load()
	s.UnsafeMode = err == nil && cur.UnsafeMode
	return write(s)
}

// SetUnsafeMode writes the Unsafe Mode switch and nothing else. The caller
// is the one binding that may: RunAPI.SetUnsafeMode, after the native
// confirmation and the image's pull when it is turning it on.
func SetUnsafeMode(on bool) error {
	saveMu.Lock()
	defer saveMu.Unlock()
	s, err := Load()
	if err != nil {
		return err
	}
	s.UnsafeMode = on
	return write(s)
}

func write(s Settings) error {
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

// SetOnboarded records that the first-run wizard has been completed or
// skipped, so it does not reopen on the next start. It is idempotent: a
// no-op when the flag is already set, and it writes a fresh settings file
// (with the shippable default registry) when none exists yet.
func SetOnboarded() error {
	s, err := Load()
	if err != nil {
		return err
	}
	if s.Onboarded {
		return nil
	}
	s.Onboarded = true
	return Save(s)
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
	if u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("registry URL %q has no host", regURL)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		// The keychain token is keyed by host, so a plain-http registry at
		// a real host would carry the same PAT in clear (security review
		// 2026-09-20 §8.6; D-030). A dev stack on this machine keeps http.
		if h := u.Hostname(); h == "localhost" || strings.HasSuffix(h, ".localhost") {
			return nil
		}
		if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("registry URL %q must use https off this machine: the token would travel in clear", regURL)
	default:
		return fmt.Errorf("registry URL must be https (or http on this machine), got %q", regURL)
	}
}

// TokenFor resolves the PAT for a registry: the OS keychain first, then the
// settings-file fallback. Empty means anonymous.
func TokenFor(regURL string) string {
	host, err := Host(regURL)
	if err == nil {
		if token, err := keyring.Get(KeyringService, host); err == nil && token != "" {
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
		if token, err := keyring.Get(KeyringService, host); err == nil && token != "" {
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
	if kerr := keyring.Set(KeyringService, host, token); kerr == nil {
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
		_ = keyring.Delete(KeyringService, host)
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

// ForgetKeychain removes every keychain entry under KeyringService: the
// registry tokens of both clients and the CLI's model keys. Nothing stored
// is not an error. It is the uninstaller's "also remove my sign-in"
// (internal/uninstall) and nothing in the running app calls it.
func ForgetKeychain() error {
	err := keyring.DeleteAll(KeyringService)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
