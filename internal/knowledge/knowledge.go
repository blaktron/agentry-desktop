// Package knowledge is the desktop's side of the local knowledge store
// (plan knowledge.md §4.3, §4.8; agentry-desktop#273, D-039): the Knowledge
// panel's packs, kept by the CLI's own pkg/knowledge/store under the runner
// home the two clients share, so a pack either client keeps is the other's
// too (agentry-cli#319, DR-42). Nothing here seals, wraps or lays out a pack
// itself: the store does, and this package wires it to this app — the
// runner home, the OS keychain, and the registry token Settings keeps —
// and says in the panel's words what each pack is and where it lives.
//
// A pack's data key is wrapped by the registry the person is signed in to
// (the deployment's key on a core registry, the account's certificate on
// SecureAgentry), so the pack opens on any computer they sign in on. Signed
// out, this computer's key in the keychain wraps it instead, and the next
// sign-in rewraps it (D20). The words say what holds the key and never call
// a pack "safe", "secure" or "private" (plan §3).
package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/remote"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Env is what the store is wired to. Default is this app's; a test passes
// a temporary runner home, a keychain in memory and a fake registry's token.
type Env struct {
	// Home is the runner home, the CLI's derivation (policy.Home).
	Home func() (string, error)
	// Keys is the OS keychain the store caches data keys in and keeps
	// this computer's key in (nil: neither).
	Keys store.Keychain
	// TokenFor is the credential this app holds for a registry, "" when
	// signed out of it.
	TokenFor func(regURL string) string
	// HTTP is the client every registry request goes through.
	HTTP *http.Client
}

// Default is this app's wiring: the runner home the CLI shares, the OS
// keychain under the CLI's service (AGENTRY_NO_KEYRING honoured, as the CLI
// honours it), and the sign-in Settings keeps.
func Default() Env {
	return Env{Home: policy.Home, Keys: store.OSKeychain{}, TokenFor: settings.TokenFor, HTTP: registry.HTTPClient}
}

// Store is the local store, with current as the registry new packs are
// wrapped by and uploaded to: the one the panel is showing, when this app
// is signed in to it.
func (e Env) Store(current string) (*store.Store, error) {
	home, err := e.Home()
	if err != nil {
		return nil, err
	}
	return store.New(home, store.Options{Keys: e.Keys, Current: settings.Normalize(current), Registries: e.registry}), nil
}

// registry is the signed-in client of the registry at u, or ErrSignedOut.
func (e Env) registry(u string) (store.Registry, error) {
	token := ""
	if e.TokenFor != nil {
		token = e.TokenFor(u)
	}
	if token == "" {
		return nil, fmt.Errorf("%s: %w (sign in to it in Settings)", u, store.ErrSignedOut)
	}
	return remote.New(u, token, e.HTTP)
}

// SignedIn says whether this app holds a credential for the registry.
func (e Env) SignedIn(reg string) bool {
	return reg != "" && e.TokenFor != nil && e.TokenFor(settings.Normalize(reg)) != ""
}

// Pack is one knowledge pack as the panel lists it: kept on this computer,
// kept in Agentry, or both.
type Pack struct {
	Name string `json:"name"`
	// ID is the pack's own id (kp_…) when it is kept here; AgentryID is
	// the registry's id for its copy there.
	ID        string `json:"id,omitempty"`
	AgentryID string `json:"agentryId,omitempty"`
	Pages     int    `json:"pages"`
	Bytes     int64  `json:"bytes"`
	// Scan says what read the pack, in the words the CLI's receipt uses;
	// ScanState is its one-word kind for the badge: not-scanned,
	// unconfirmed, scanning, scanned, failed or empty.
	Scan      string `json:"scan"`
	ScanState string `json:"scanState"`
	Local     bool   `json:"local"`
	InAgentry bool   `json:"inAgentry"`
	// Changed is a pack kept in both places that changed here since its
	// upload.
	Changed bool   `json:"changed"`
	Where   string `json:"where"`
	// Key says what the data key is wrapped to, for a pack kept here
	// ("your account's key at <host>, which Agentry holds").
	Key string `json:"key,omitempty"`
	// Wrapper is "registry" or "machine" for a pack kept here.
	Wrapper string `json:"wrapper,omitempty"`
	// Registry is the host whose account key wraps it, "" for this
	// computer's key.
	Registry string `json:"registry,omitempty"`
	// OpensOffline says the key is on this computer: this computer's key,
	// or a key the registry unwrapped once and the keychain caches. It is
	// read from the cache's presence marker, never the key.
	OpensOffline bool   `json:"opensOffline"`
	Updated      string `json:"updated,omitempty"`
}

// List is the panel: the packs, and what the registry shown is to them.
type List struct {
	Packs []Pack `json:"packs"`
	// Registry is the registry the panel shows; SignedIn says whether this
	// app is signed in to it, so new packs are wrapped to the account.
	Registry string `json:"registry"`
	SignedIn bool   `json:"signedIn"`
	// Agentry says why Agentry's copies are not listed, "" when they are.
	Agentry string `json:"agentry,omitempty"`
	// Problems are entries of the store that could not be read.
	Problems []string `json:"problems,omitempty"`
}

// The one-word kinds of Pack.ScanState.
const (
	ScanNotScanned  = "not-scanned"
	ScanUnconfirmed = "unconfirmed"
	ScanScanning    = "scanning"
	ScanScanned     = "scanned"
	ScanFailed      = "failed"
	ScanEmpty       = "empty"
)

// List is every pack kept on this computer and, signed in to current,
// every pack its account keeps in Agentry, by name. A store entry that
// cannot be read is named in Problems beside the ones that can; a registry
// that cannot be asked is said in Agentry, and the panel still lists what
// is kept here.
func (e Env) List(ctx context.Context, current string) (*List, error) {
	s, err := e.Store(current)
	if err != nil {
		return nil, err
	}
	reg := settings.Normalize(current)
	out := &List{Packs: []Pack{}, Registry: reg, SignedIn: e.SignedIn(reg)}
	entries, lerr := s.List()
	if lerr != nil {
		for _, line := range strings.Split(lerr.Error(), "\n") {
			if line != "" {
				out.Problems = append(out.Problems, line)
			}
		}
	}
	var remotes []store.Remote
	listed := false
	switch {
	case reg == "":
		out.Agentry = "No registry is chosen, so there is no copy in Agentry to list."
	case !out.SignedIn:
		out.Agentry = "Not signed in to " + hostOf(reg) + ", so its copies in Agentry are not listed."
	default:
		if c, err := e.registry(reg); err != nil {
			out.Agentry = "Agentry's copies could not be listed: " + err.Error()
		} else if remotes, err = c.List(ctx); err != nil {
			out.Agentry = "Agentry's copies could not be listed: " + err.Error()
		} else {
			listed = true
		}
	}
	matched := map[string]bool{}
	for _, en := range entries {
		p := localPack(s, en)
		if a := en.Key.Agentry; a != nil && listed && settings.Normalize(a.Registry) == reg {
			if r, ok := findRemote(remotes, a.ID); ok {
				matched[r.ID] = true
				p.AgentryState(r, en.Manifest.SHA256)
			} else {
				// The record says Agentry keeps a copy, and Agentry's own
				// list says it does not (deleted from the site, or by the
				// CLI on another computer).
				p.InAgentry, p.Changed, p.AgentryID = false, false, ""
				p.Where = "this computer (Agentry no longer keeps its copy)"
			}
		}
		out.Packs = append(out.Packs, p)
	}
	for _, r := range remotes {
		if !matched[r.ID] {
			out.Packs = append(out.Packs, remotePack(r))
		}
	}
	sort.SliceStable(out.Packs, func(i, j int) bool {
		if out.Packs[i].Name != out.Packs[j].Name {
			return out.Packs[i].Name < out.Packs[j].Name
		}
		return out.Packs[i].Local && !out.Packs[j].Local
	})
	return out, nil
}

// localPack is a pack kept here, as the store describes it.
func localPack(s *store.Store, en store.Entry) Pack {
	m := en.Manifest
	p := Pack{
		Name: m.Name, ID: m.ID, Pages: m.Pages, Bytes: m.Bytes,
		Scan: m.Scan.Describe(), ScanState: localScanState(m.Scan),
		Local: true, InAgentry: en.Key.Agentry != nil, Where: en.Where(),
		Key: en.KeyHolder(), Wrapper: string(en.Key.Wrapper),
	}
	if a := en.Key.Agentry; a != nil {
		p.AgentryID = a.ID
		p.Changed = a.Digest != m.SHA256
	}
	switch en.Key.Wrapper {
	case store.WrapperMachine:
		p.OpensOffline = true
	case store.WrapperRegistry:
		p.Registry = hostOf(en.Key.Registry)
		p.OpensOffline = s.Cached(m.ID)
	}
	if !m.Updated.IsZero() {
		p.Updated = m.Updated.UTC().Format("2006-01-02T15:04:05Z")
	}
	return p
}

func localScanState(sc kn.Scan) string {
	switch {
	case !sc.Scanned:
		return ScanNotScanned
	case !sc.Verified:
		return ScanUnconfirmed
	}
	return ScanScanned
}

// AgentryState reads what Agentry's own list says of a pack kept in both
// places: a copy of this very tree that its scan has finished with reads
// as scanned there, confirmed by the registry that scanned it, not by the
// record in the file.
func (p *Pack) AgentryState(r store.Remote, localDigest string) {
	p.InAgentry = true
	switch {
	case r.State == store.StateScanning:
		p.Scan, p.ScanState = "being scanned in Agentry", ScanScanning
	case r.State == store.StateFailed:
		p.Scan, p.ScanState = "Agentry's scan of it failed", ScanFailed
	case r.State == store.StateReady && len(r.Scan) > 0 && r.Digest == localDigest:
		p.Scan, p.ScanState = scannedInAgentry(r.Scan), ScanScanned
	}
}

// remotePack is a pack Agentry keeps and this computer does not.
func remotePack(r store.Remote) Pack {
	p := Pack{Name: r.Name, AgentryID: r.ID, Pages: r.Pages, Bytes: r.Bytes, InAgentry: true, Where: "Agentry"}
	switch r.State {
	case store.StateReady:
		if len(r.Scan) > 0 {
			p.Scan, p.ScanState = scannedInAgentry(r.Scan), ScanScanned
		} else {
			p.Scan, p.ScanState = "kept in Agentry with no scan report", ScanUnconfirmed
		}
	case store.StateScanning:
		p.Scan, p.ScanState = "being scanned in Agentry", ScanScanning
	case store.StateFailed:
		p.Scan, p.ScanState = "Agentry's scan of it failed", ScanFailed
	default:
		p.Scan, p.ScanState = "made in Agentry, with nothing uploaded to it yet", ScanEmpty
	}
	if !r.Updated.IsZero() {
		p.Updated = r.Updated.UTC().Format("2006-01-02T15:04:05Z")
	}
	return p
}

// scannedInAgentry says a finished scan with its finding count ("scanned,
// N findings" is the product's register, never "clean").
func scannedInAgentry(report json.RawMessage) string {
	var rep struct {
		Findings []json.RawMessage `json:"findings"`
	}
	if json.Unmarshal(report, &rep) != nil {
		return "scanned in Agentry"
	}
	switch n := len(rep.Findings); n {
	case 1:
		return "scanned in Agentry, 1 finding"
	default:
		return fmt.Sprintf("scanned in Agentry, %d findings", n)
	}
}

func findRemote(rs []store.Remote, id string) (store.Remote, bool) {
	for _, r := range rs {
		if r.ID == id {
			return r, true
		}
	}
	return store.Remote{}, false
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Host
	}
	return u
}

// Upload keeps a copy of the pack in Agentry, sealed and with its wrapped
// key as they are; Agentry scans what it receives. It answers the
// sentence the panel shows.
func (e Env) Upload(ctx context.Context, current, name string) (string, error) {
	s, err := e.Store(current)
	if err != nil {
		return "", err
	}
	if !e.SignedIn(current) {
		return "", fmt.Errorf("upload knowledge pack %s: sign in to %s in Settings first", name, hostOf(settings.Normalize(current)))
	}
	_, r, err := s.Upload(ctx, name)
	if err != nil {
		return "", err
	}
	if r.State == store.StateScanning {
		return fmt.Sprintf("Uploaded %s to %s. Agentry is scanning it; the scan's findings show here once it ends.", name, hostOf(settings.Normalize(current))), nil
	}
	return fmt.Sprintf("Uploaded %s to %s.", name, hostOf(settings.Normalize(current))), nil
}

// Download keeps Agentry's copy of the pack on this computer, checked in
// full, under the key Agentry wrapped.
func (e Env) Download(ctx context.Context, current, name string) (string, error) {
	s, err := e.Store(current)
	if err != nil {
		return "", err
	}
	if !e.SignedIn(current) {
		return "", fmt.Errorf("download knowledge pack %s: sign in to %s in Settings first", name, hostOf(settings.Normalize(current)))
	}
	en, err := s.Download(ctx, name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Downloaded %s: %s, kept on this computer, encrypted to %s.", name, kn.CountPages(en.Manifest.Pages), en.KeyHolder()), nil
}

// Delete removes the pack from this computer — its sealed tree, its
// wrapped key and the key the keychain caches — and with everywhere,
// Agentry's copy too, first, so a refusal there leaves the pack here.
func (e Env) Delete(ctx context.Context, current, name string, everywhere bool) error {
	s, err := e.Store(current)
	if err != nil {
		return err
	}
	_, err = s.Delete(ctx, name, everywhere)
	return err
}

// Find is the kept pack name, for the bindings' dialogs.
func (e Env) Find(current, name string) (store.Entry, error) {
	s, err := e.Store(current)
	if err != nil {
		return store.Entry{}, err
	}
	return s.Find(name)
}

// Rewrap moves every pack this computer's key wraps to the account's key
// at current, once this app is signed in to it (D20), and answers their
// names. Signed out, it does nothing.
func (e Env) Rewrap(ctx context.Context, current string) ([]string, error) {
	if !e.SignedIn(current) {
		return nil, nil
	}
	s, err := e.Store(current)
	if err != nil {
		return nil, err
	}
	return s.Rewrap(ctx)
}

// IsUsage says whether err is the person's to fix by naming another pack:
// one that is not kept, or a name already kept.
func IsUsage(err error) bool {
	return errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrNameTaken)
}
