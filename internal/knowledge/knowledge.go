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
// (the account's certificate on SecureAgentry; on a core registry a key it
// derives for the account from the deployment's own sealing key), so the
// pack opens on any computer they sign in on. Signed out, this computer's
// key in the keychain wraps it instead, and the next sign-in rewraps it
// (D20). The words say what holds the key, by the wrapper's kind, and never
// call a pack "safe", "secure" or "private" (plan §3).
//
// An organization's shared packs (Knowledge M8, agentry-desktop#276; plan
// §4.7, D13, D26) are listed beside the person's own for each organization
// they are an active member of, and kept by the same store (agentry-cli#380,
// DR-46): a shared pack's key is asked for through the organization's
// route every time it is opened, and never kept in the keychain or in this
// app's memory, so a member who is removed keeps a sealed copy they can no
// longer open. Export of a shared pack is the registry's to allow: its
// admins, and members once an admin marks the pack exportable.
package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

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
	// Orgs overrides how the signed-in person's active memberships at a
	// registry are read, for a test (nil: GET /user, as memberships reads
	// it); the panel lists their shared packs.
	Orgs func(ctx context.Context, regURL string) ([]Membership, error)
}

// Membership is an organization the person is an active member of, and
// their role in it.
type Membership struct {
	Org   string
	Admin bool
}

// Default is this app's wiring: the runner home the CLI shares, the OS
// keychain under the CLI's service (AGENTRY_NO_KEYRING honoured, as the CLI
// honours it), and the sign-in Settings keeps.
func Default() Env {
	return Env{Home: policy.Home, Keys: store.OSKeychain{}, TokenFor: settings.TokenFor, HTTP: registry.HTTPClient}
}

// memberships is the signed-in person's active memberships at regURL (GET
// /user). The URL is held to Settings' rule before a token is looked up
// for it, as registry does, because the token is found by host alone
// (agentry-desktop#307's lesson). A pending membership holds nothing, and
// is left out.
func (e Env) memberships(ctx context.Context, regURL string) ([]Membership, error) {
	if e.Orgs != nil {
		return e.Orgs(ctx, regURL)
	}
	if err := settings.ValidateURL(regURL); err != nil {
		return nil, fmt.Errorf("the registry: %w", err)
	}
	token := ""
	if e.TokenFor != nil {
		token = e.TokenFor(regURL)
	}
	if token == "" {
		return nil, nil
	}
	c, err := registry.New(regURL, token)
	if err != nil {
		return nil, err
	}
	res, err := c.GetCurrentUserWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, registry.Problem("read your organizations", res.StatusCode(), res.ApplicationproblemJSONDefault, res.Body)
	}
	var out []Membership
	if u := res.JSON200; u.Orgs != nil {
		for _, o := range *u.Orgs {
			if o.State != nil && string(*o.State) != "active" {
				continue
			}
			out = append(out, Membership{Org: o.Handle, Admin: string(o.Role) == "admin"})
		}
	}
	return out, nil
}

// Store is the local store, with current as the registry new packs are
// wrapped by and uploaded to: the one the panel is showing, when this app
// is signed in to it.
func (e Env) Store(current string) (*store.Store, error) {
	home, err := e.Home()
	if err != nil {
		return nil, err
	}
	return store.New(home, store.Options{Keys: e.Keys, Current: settings.Normalize(current), Registries: e.registry, Shared: e.shared}), nil
}

// shared is the signed-in client of org's shared packs at the registry at
// u, held to the same rule as registry: the URL first, then the token.
func (e Env) shared(u, org string) (store.Shared, error) {
	reg, err := e.registry(u)
	if err != nil {
		return nil, err
	}
	c, ok := reg.(*remote.Client)
	if !ok {
		return nil, fmt.Errorf("%s: this app's registry client does not reach an organization's packs", u)
	}
	return c.Shared(org), nil
}

// registry is the signed-in client of the registry at u, or ErrSignedOut.
// u is the page's registry or the one a pack's key.json names, a file on
// disk: it is held to the URL rule Settings holds a registry to (https, or
// http on this machine) before a token is looked up for it, because the
// token is found by host alone and a wrap or an unwrap carries a raw data
// key. A registry this app holds no token for gets none.
func (e Env) registry(u string) (store.Registry, error) {
	if err := settings.ValidateURL(u); err != nil {
		return nil, fmt.Errorf("the knowledge pack's registry: %w", err)
	}
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
	// AgentryReady says Agentry's copy has been scanned and can be
	// attached to a run In Agentry (agentry-desktop#275): its state there
	// is ready. For a pack kept in both places that changed here since its
	// upload, it is the older tree Agentry keeps.
	AgentryReady bool `json:"agentryReady"`
	// Changed is a pack kept in both places that changed here since its
	// upload.
	Changed bool `json:"changed"`
	// Where is where it is kept: "this computer", "Agentry", or "this
	// computer and Agentry"; Note says more when there is more to say
	// (Agentry no longer keeps the copy this computer's record names).
	Where string `json:"where"`
	Note  string `json:"note,omitempty"`
	// Key says what the data key is wrapped to, for a pack kept here, by
	// the wrapper's kind (KeyHolder).
	Key string `json:"key,omitempty"`
	// Wrapper is "registry" or "machine" for a pack kept here.
	Wrapper string `json:"wrapper,omitempty"`
	// Registry is the host whose account key wraps it, "" for this
	// computer's key.
	Registry string `json:"registry,omitempty"`
	// OpensOffline says the key is on this computer: this computer's key,
	// or a key the registry unwrapped once and the keychain caches. It is
	// read from the cache's presence marker, never the key.
	OpensOffline bool `json:"opensOffline"`
	// Shared is an organization's shared pack (Knowledge M8), Org its
	// organization; Exportable says an admin marked it exportable by
	// members, as the organization's list read for this answer says (false
	// when it could not be read).
	Shared     bool   `json:"shared"`
	Org        string `json:"org,omitempty"`
	Exportable bool   `json:"exportable"`
	// CanExport says the panel offers Export: the person's own pack, or a
	// shared one they administer or an admin marked exportable (D26). The
	// registry decides again when it is asked.
	CanExport bool `json:"canExport"`
	// Updated is when the copy kept here last changed (its manifest's
	// updated, else created), AgentryUpdated when Agentry's copy did, as
	// its list says; RFC 3339, or "" for a place the pack is not kept. The
	// Local and Cloud Knowledge pages each show their own
	// (agentry-desktop#619).
	Updated        string `json:"updated,omitempty"`
	AgentryUpdated string `json:"agentryUpdated,omitempty"`
	// AgentryScan, AgentryScanState, AgentryPages and AgentryBytes are
	// Agentry's copy as its list says it, for the Cloud Knowledge page: the
	// same as Scan, ScanState, Pages and Bytes for a pack kept only there or
	// unchanged here, and the older copy's for one changed here since it
	// was copied. Empty for a pack Agentry does not keep.
	AgentryScan      string `json:"agentryScan,omitempty"`
	AgentryScanState string `json:"agentryScanState,omitempty"`
	AgentryPages     int    `json:"agentryPages,omitempty"`
	AgentryBytes     int64  `json:"agentryBytes,omitempty"`

	// agentryRegistry is the registry a kept copy's record names for its
	// copy in Agentry, for matching it against that registry's list.
	agentryRegistry string
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
	// Shared says why an organization's shared packs are not listed, ""
	// when they are, or when there are none to list (Knowledge M8).
	Shared string `json:"shared,omitempty"`
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
		// A shared pack's copy in Agentry is the organization's, matched
		// against the organization's list (addShared), not the person's.
		if a := en.Key.Agentry; a != nil && en.Key.Org == "" && listed && settings.Normalize(a.Registry) == reg {
			if r, ok := findRemote(remotes, a.ID); ok {
				matched[r.ID] = true
				p.agentryCopy(r, en.Manifest.SHA256)
			} else {
				// The record says Agentry keeps a copy, and Agentry's own
				// list says it does not (deleted from the Cloud Knowledge
				// page, the site, or by the CLI on another computer). The
				// copy here may have been uploaded or copied here, so the
				// note says neither.
				p.InAgentry, p.Changed, p.AgentryID = false, false, ""
				p.Where, p.Note = whereLocal, "Agentry no longer keeps its copy of this pack: Copy to Cloud keeps one there again."
			}
		}
		out.Packs = append(out.Packs, p)
	}
	for _, r := range remotes {
		if !matched[r.ID] {
			out.Packs = append(out.Packs, remotePack(r))
		}
	}
	e.addShared(ctx, s, out, reg, out.SignedIn)
	sort.SliceStable(out.Packs, func(i, j int) bool {
		if out.Packs[i].Name != out.Packs[j].Name {
			return out.Packs[i].Name < out.Packs[j].Name
		}
		return out.Packs[i].Local && !out.Packs[j].Local
	})
	return out, nil
}

// addShared puts the organization's shared packs in the list: for each
// organization the person is an active member of, the packs it shares in
// Agentry, matched to the copies kept here, and the kept copies' export as
// the person's role and Agentry's mark allow. A read that does not answer
// is said in Agentry beside the rest, and the kept copies are still listed.
func (e Env) addShared(ctx context.Context, s *store.Store, out *List, reg string, signedIn bool) {
	var orgs []Membership
	if signedIn {
		var err error
		if orgs, err = e.memberships(ctx, reg); err != nil {
			out.Shared = "Your organizations' shared packs could not be listed: " + err.Error()
		}
	}
	admin := map[string]bool{}
	remotes := map[string][]store.Remote{}
	for _, m := range orgs {
		admin[m.Org] = m.Admin
		sh, err := e.shared(reg, m.Org)
		if err == nil {
			remotes[m.Org], err = sh.List(ctx)
		}
		if err != nil {
			out.Shared = strings.TrimSpace(out.Shared + " " + m.Org + "'s shared packs could not be listed: " + err.Error())
			delete(remotes, m.Org)
		}
	}
	matched := map[string]bool{}
	for i := range out.Packs {
		p := &out.Packs[i]
		if !p.Shared {
			continue
		}
		rs, ok := remotes[p.Org]
		if !ok || settings.Normalize(p.agentryRegistry) != reg {
			continue
		}
		if r, ok := findRemote(rs, p.AgentryID); ok {
			matched[r.ID] = true
			p.agentryCopy(r, "")
			p.Exportable = r.Exportable
			p.CanExport = admin[p.Org] || r.Exportable
		} else {
			p.InAgentry, p.AgentryID = false, ""
			p.Where, p.Note = whereLocal, p.Org+" no longer shares this pack, or you are no longer one of its members: the copy here opens only while you are."
		}
	}
	for org, rs := range remotes {
		for _, r := range rs {
			if !matched[r.ID] {
				p := remotePack(r)
				p.Shared, p.Org, p.Exportable = true, org, r.Exportable
				out.Packs = append(out.Packs, p)
			}
		}
	}
}

// Where a pack is kept, in the panel's words.
const (
	whereLocal   = "this computer"
	whereAgentry = "Agentry"
	whereBoth    = "this computer and Agentry"
)

// localPack is a pack kept here, as the store describes it.
func localPack(s *store.Store, en store.Entry) Pack {
	m := en.Manifest
	p := Pack{
		Name: m.Name, ID: m.ID, Pages: m.Pages, Bytes: m.Bytes,
		Scan: m.Scan.Describe(), ScanState: localScanState(m.Scan),
		Local: true, Where: whereLocal,
		Key: KeyHolder(en), Wrapper: string(en.Key.Wrapper),
		Shared: en.Key.Org != "", Org: en.Key.Org, CanExport: en.Key.Org == "",
		Updated: stamp(m.Updated, m.Created),
	}
	if a := en.Key.Agentry; a != nil {
		p.InAgentry, p.Where, p.AgentryID, p.agentryRegistry = true, whereBoth, a.ID, a.Registry
		p.Changed = a.Digest != m.SHA256
	}
	switch en.Key.Wrapper {
	case store.WrapperMachine:
		p.OpensOffline = true
	case store.WrapperRegistry:
		p.Registry = hostOf(en.Key.Registry)
		// A shared pack's key is never cached (DR-46): it does not open
		// offline, whatever the keychain holds.
		p.OpensOffline = en.Key.Org == "" && s.Cached(m.ID)
	}
	return p
}

// The wrapper kinds a registry answers on a wrapped key.
const (
	// kindCertificate is SecureAgentry's knowledge extension: the data
	// key wrapped to the account's X25519 certificate, the key that opens
	// it held by Agentry (agentry-enterprise#170).
	kindCertificate = "certificate"
	// kindDeployment is the open core's DeploymentWrapper: the data key
	// sealed under a key the registry derives for the account from its
	// own knowledge sealing key.
	kindDeployment = "deployment"
)

// KeyHolder says what a kept pack's data key is wrapped to, by the
// wrapper's kind: the plan's sentence for SecureAgentry's certificate, what
// is true of a core registry's deployment wrap, and the CLI's sentence for
// this computer's key. The CLI's Entry.KeyHolder words every registry wrap
// the first way (agentry-cli#357); this is the desktop's until it does not.
func KeyHolder(en store.Entry) string {
	if en.Key.Wrapper != store.WrapperRegistry {
		return en.KeyHolder()
	}
	host := hostOf(en.Key.Registry)
	if en.Key.Org != "" {
		return en.Key.Org + "'s organization key at " + host + ", which Agentry holds; it opens the pack's key only for the organization's current members"
	}
	switch en.Key.WrappedKey.Kind {
	case kindCertificate:
		return "your account's key at " + host + ", which Agentry holds"
	case kindDeployment:
		return "a key the registry at " + host + " keeps for your account"
	}
	return "a key the registry at " + host + " holds (a wrapping this app does not know: " + en.Key.WrappedKey.Kind + ")"
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

// agentryCopy reads what Agentry's own list says of a pack kept in both
// places. A copy of this very tree whose scan has finished reads as
// scanned there, confirmed by the registry that scanned it rather than by
// the record in the file; a copy of an older tree leaves the local record
// as it is.
func (p *Pack) agentryCopy(r store.Remote, localDigest string) {
	p.InAgentry, p.AgentryReady = true, r.State == store.StateReady
	p.AgentryUpdated = stamp(r.Updated, r.Created)
	p.agentryFacts(r)
	if r.State == store.StateReady && r.Digest != localDigest {
		return
	}
	if scan, state, ok := remoteScan(r); ok {
		p.Scan, p.ScanState = scan, state
	}
}

// remotePack is a pack Agentry keeps and this computer does not.
func remotePack(r store.Remote) Pack {
	p := Pack{
		Name: r.Name, AgentryID: r.ID, Pages: r.Pages, Bytes: r.Bytes, InAgentry: true, AgentryReady: r.State == store.StateReady,
		Where: whereAgentry, AgentryUpdated: stamp(r.Updated, r.Created),
	}
	p.Scan, p.ScanState, _ = remoteScan(r)
	p.agentryFacts(r)
	return p
}

// agentryFacts records what Agentry's list says of its copy.
func (p *Pack) agentryFacts(r store.Remote) {
	p.AgentryScan, p.AgentryScanState, _ = remoteScan(r)
	p.AgentryPages, p.AgentryBytes = r.Pages, r.Bytes
}

// remoteScan is what Agentry's state for a pack says of its scan; ok is
// false for a ready pack with no report, which says nothing of one.
func remoteScan(r store.Remote) (scan, state string, ok bool) {
	switch r.State {
	case store.StateReady:
		if len(r.Scan) > 0 {
			return scannedInAgentry(r.Scan), ScanScanned, true
		}
		return "kept in Agentry with no scan report", ScanUnconfirmed, false
	case store.StateScanning:
		return "being scanned in Agentry", ScanScanning, true
	case store.StateFailed:
		return "Agentry's scan of it failed", ScanFailed, true
	}
	return "made in Agentry, with nothing uploaded to it yet", ScanEmpty, true
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

// stamp is a pack's date for the panel: when it last changed, else when it
// was made, in RFC 3339; "" when neither is known.
func stamp(updated, created time.Time) string {
	if updated.IsZero() {
		updated = created
	}
	if updated.IsZero() {
		return ""
	}
	return updated.UTC().Format(time.RFC3339)
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
	host := hostOf(settings.Normalize(current))
	if !e.SignedIn(current) {
		return "", fmt.Errorf("upload knowledge pack %s: sign in to %s in Settings first", name, host)
	}
	_, r, err := s.Upload(ctx, name)
	if err != nil {
		return "", err
	}
	if r.State == store.StateScanning {
		return fmt.Sprintf("Uploaded %s to %s. Agentry is scanning it; the scan's findings show here once it ends.", name, host), nil
	}
	return fmt.Sprintf("Uploaded %s to %s.", name, host), nil
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
	return fmt.Sprintf("Downloaded %s: %s, kept on this computer, encrypted to %s.", name, kn.CountPages(en.Manifest.Pages), KeyHolder(en)), nil
}

// DownloadShared keeps a copy of org's shared pack on this computer,
// checked in full, under the organization's wrapped key, which is asked for
// again every time it is opened.
func (e Env) DownloadShared(ctx context.Context, current, org, name string) (string, error) {
	s, err := e.Store(current)
	if err != nil {
		return "", err
	}
	if !e.SignedIn(current) {
		return "", fmt.Errorf("download %s's knowledge pack %s: sign in to %s in Settings first", org, name, hostOf(settings.Normalize(current)))
	}
	en, err := s.DownloadShared(ctx, org, name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Downloaded %s: %s, kept on this computer, encrypted to %s. It opens only while you are a member and signed in.", name, kn.CountPages(en.Manifest.Pages), KeyHolder(en)), nil
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

// DeleteFromAgentry deletes the account's copy id in Agentry and nothing
// on this computer (plan knowledge-cards.md §4.3, D3; agentry-desktop#619):
// the Cloud Knowledge page's Delete. A copy kept here keeps its record of
// the deleted copy; the list then says Agentry no longer keeps it, and an
// upload makes a new copy, as the store does for any copy Agentry lost. A
// copy already gone is not an error. An organization's shared pack is not
// in the account's own list, so its id is refused there (a member never
// deletes one, D6).
func (e Env) DeleteFromAgentry(ctx context.Context, current, id string) error {
	if !IsAgentryID(id) {
		return fmt.Errorf("%q is not the id of a pack in Agentry", id)
	}
	c, err := e.registry(settings.Normalize(current))
	if err != nil {
		return err
	}
	if err := c.Delete(ctx, id); err != nil && !errors.Is(err, store.ErrRemoteNotFound) {
		return fmt.Errorf("delete Agentry's copy: %w", err)
	}
	return nil
}

// AgentryPack is the account's own pack id in Agentry, as its list says it
// now, for the bindings' dialogs: they name only a pack found by its id.
// An organization's shared pack is not in that list, nor is another
// account's or one already gone; each is refused (ErrRemoteNotFound).
func (e Env) AgentryPack(ctx context.Context, current, id string) (store.Remote, error) {
	if !IsAgentryID(id) {
		return store.Remote{}, fmt.Errorf("%q is not the id of a pack in Agentry", id)
	}
	c, err := e.registry(settings.Normalize(current))
	if err != nil {
		return store.Remote{}, err
	}
	rs, err := c.List(ctx)
	if err != nil {
		return store.Remote{}, fmt.Errorf("list your packs in Agentry: %w", err)
	}
	if r, ok := findRemote(rs, id); ok {
		return r, nil
	}
	return store.Remote{}, fmt.Errorf("no pack of yours in Agentry has the id %s: %w", id, store.ErrRemoteNotFound)
}

// IsAgentryID says id is the form a registry names a pack by, a UUID; any
// other string is refused before a registry is asked.
func IsAgentryID(id string) bool { return agentryID.MatchString(id) }

var agentryID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

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
