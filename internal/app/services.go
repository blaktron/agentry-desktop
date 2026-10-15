// Bound services: the thin, JSON-shaped surface the frontend calls. Logic
// lives in internal/…; these methods map API-client types onto flat view
// models (plain strings and ints) so the generated TypeScript bindings stay
// faithful to what actually crosses the JSON boundary.
package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry/pkg/apiclient"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/trust"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/mcpview"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/validate"
)

// ---------------------------------------------------------------- settings

// SettingsAPI manages registries and credentials.
type SettingsAPI struct {
	app *App
	// open replaces App.OpenExternal in tests; nil means the real browser.
	open func(url string) error
	// knowledgeEnv is the local knowledge store's wiring, for the rewrap a
	// sign-in makes; nil is this app's.
	knowledgeEnv *knowledge.Env
}

// RegistryView is one configured registry as the frontend sees it.
type RegistryView struct {
	URL      string `json:"url"`
	HasToken bool   `json:"hasToken"`
	// TokenSource is "keyring", "file", or "" when signed out.
	TokenSource string `json:"tokenSource,omitempty"`
	IsDefault   bool   `json:"isDefault"`
	// SignIn is which of the app's two sign-ins this registry has:
	// "secureagentry", "self-hosted", or "" when it is neither and the app
	// will not sign in to it (SignInRefused says why).
	SignIn        string `json:"signIn"`
	SignInRefused string `json:"signInRefused,omitempty"`
}

// SettingsView is the settings page's model.
type SettingsView struct {
	DefaultRegistry string         `json:"defaultRegistry"`
	Registries      []RegistryView `json:"registries"`
	// SecureAgentryRegistry is the SecureAgentry registry this build signs
	// in to, "" when it is built for none; SelfHostedRegistry is the one
	// self-hosted registry it signs in to, "" until one is added.
	SecureAgentryRegistry string `json:"secureAgentryRegistry"`
	SelfHostedRegistry    string `json:"selfHostedRegistry"`
	ConfigPath            string `json:"configPath"`
	// Onboarded is false until the first-run wizard is completed or skipped;
	// the frontend opens the guided setup while it is false and nothing is
	// signed in.
	Onboarded bool `json:"onboarded"`
	// UnsafeMode is Run defaults' switch: the shell draws the red edge and
	// the sidebar's badge from it.
	UnsafeMode bool `json:"unsafeMode"`
}

func settingsView() (*SettingsView, error) {
	s, err := settings.Load()
	if err != nil {
		return nil, err
	}
	path, _ := settings.Path()
	v := &SettingsView{
		DefaultRegistry: s.DefaultRegistry, Registries: []RegistryView{}, ConfigPath: path, Onboarded: s.Onboarded, UnsafeMode: s.UnsafeMode,
		SecureAgentryRegistry: settings.SecureAgentryRegistry(), SelfHostedRegistry: s.SelfHostedRegistry(),
	}
	for _, r := range s.Registries {
		src := settings.TokenSource(r.URL)
		kind, _ := s.SignInFor(r.URL)
		rv := RegistryView{
			URL: settings.Normalize(r.URL), HasToken: src != "", TokenSource: src,
			IsDefault: settings.Normalize(r.URL) == settings.Normalize(s.DefaultRegistry),
			SignIn:    kind,
		}
		if kind == "" {
			if err := s.Admit(r.URL); err != nil {
				rv.SignInRefused = err.Error()
			}
		}
		v.Registries = append(v.Registries, rv)
	}
	return v, nil
}

// Get returns the current settings.
func (s *SettingsAPI) Get() (*SettingsView, error) { return settingsView() }

// MarkOnboarded records that the first-run wizard finished or was skipped,
// so it does not reopen on the next start. The wizard only orchestrates
// existing services; this flag is the sole state it adds.
func (s *SettingsAPI) MarkOnboarded() (*SettingsView, error) {
	if err := settings.SetOnboarded(); err != nil {
		return nil, err
	}
	return settingsView()
}

// CodingAgentsView is Settings → Coding agents: the coding agents the
// operator policy records as chosen (read, never probed — the page has Look
// for that), and the export they imply with its folder and the switch.
type CodingAgentsView struct {
	Chosen []HarnessView             `json:"chosen"`
	Export install.CodingAgentExport `json:"export"`
}

func codingAgentsView() (*CodingAgentsView, error) {
	chosen, err := run.ChosenHarnesses()
	if err != nil {
		return nil, err
	}
	x, err := install.ExportSettings()
	if err != nil {
		return nil, err
	}
	return &CodingAgentsView{Chosen: harnessViews(chosen), Export: x}, nil
}

// CodingAgents reads the section.
func (s *SettingsAPI) CodingAgents() (*CodingAgentsView, error) { return codingAgentsView() }

// SetInstallIntoCodingAgent is the switch "also install into <coding
// agent>": on, a one-click Install also writes the export. It will not go
// on while the export is AGENTS.md with no project directory chosen,
// since nothing could be written.
func (s *SettingsAPI) SetInstallIntoCodingAgent(on bool) (*CodingAgentsView, error) {
	if on {
		x, err := install.ExportSettings()
		if err != nil {
			return nil, err
		}
		if x.NeedsDir {
			return nil, fmt.Errorf("choose the project directory for AGENTS.md first")
		}
	}
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	st.InstallIntoCodingAgent = on
	if err := settings.Save(st); err != nil {
		return nil, err
	}
	return codingAgentsView()
}

// ChooseCodingAgentProjectDir opens the native picker for the project
// directory AGENTS.md is written to and stores what it returned — the
// binding takes no path, so the directory is the picker's and never the
// page's. A cancelled picker changes nothing.
func (s *SettingsAPI) ChooseCodingAgentProjectDir() (*CodingAgentsView, error) {
	dir, err := s.app.chooseDirectory("Choose the project directory for AGENTS.md")
	if err != nil {
		return nil, err
	}
	if dir != "" {
		st, err := settings.Load()
		if err != nil {
			return nil, err
		}
		st.CodingAgentProjectDir = dir
		if err := settings.Save(st); err != nil {
			return nil, err
		}
	}
	return codingAgentsView()
}

// AddRegistry records a new registry.
func (s *SettingsAPI) AddRegistry(url string) (*SettingsView, error) {
	url = settings.Normalize(url)
	if err := settings.ValidateURL(url); err != nil {
		return nil, err
	}
	cfg, err := settings.Load()
	if err != nil {
		return nil, err
	}
	if err := cfg.Admit(url); err != nil {
		return nil, err
	}
	if _, reg := cfg.SignInFor(url); reg != "" {
		url = reg
	}
	cfg.Upsert(settings.Registry{URL: url})
	if cfg.DefaultRegistry == "" {
		cfg.DefaultRegistry = url
	}
	if err := settings.Save(cfg); err != nil {
		return nil, err
	}
	return settingsView()
}

// RemoveRegistry forgets a registry (its stored credential too).
func (s *SettingsAPI) RemoveRegistry(url string) (*SettingsView, error) {
	if err := settings.DeleteToken(url); err != nil {
		return nil, err
	}
	resealSchedules(s.app)
	cfg, err := settings.Load()
	if err != nil {
		return nil, err
	}
	cfg.Remove(url)
	if err := settings.Save(cfg); err != nil {
		return nil, err
	}
	return settingsView()
}

// SetDefault marks a registry as the default.
func (s *SettingsAPI) SetDefault(url string) (*SettingsView, error) {
	cfg, err := settings.Load()
	if err != nil {
		return nil, err
	}
	if cfg.Find(url) == nil {
		return nil, fmt.Errorf("registry %s is not configured", url)
	}
	cfg.DefaultRegistry = settings.Normalize(url)
	if err := settings.Save(cfg); err != nil {
		return nil, err
	}
	return settingsView()
}

// UserView is the signed-in identity, with the organizations it belongs to.
// Those are the core's own facts — every registry answers them, whether or
// not it sells anything — so a membership shows on Settings even where there
// are no tiers to speak of.
type UserView struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	// EmailVerified is the directory's word on the address, which the
	// Profile card shows beside it as the site does.
	EmailVerified bool            `json:"emailVerified"`
	Orgs          []OrgMembership `json:"orgs,omitempty"`
	// PendingOrg is the organization waiting to approve this person: its
	// domain folded them in, or its directory provisioned them. They hold
	// no seat and spend from their own account until an admin approves
	// them, so the app says so rather than showing a membership.
	PendingOrg *PendingOrgView `json:"pendingOrg,omitempty"`
}

// OrgMembership is one membership: the organization, the role in it, and
// whether it is in force ("active") or waiting for an admin ("pending").
type OrgMembership struct {
	Handle string `json:"handle"`
	Role   string `json:"role"`
	State  string `json:"state,omitempty"`
}

// PendingOrgView names the door a pending fold-in came through, so the
// app can say why this organization is asking for them.
type PendingOrgView struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Via         string `json:"via"`
}

// SignInResult reports a completed sign-in.
type SignInResult struct {
	User UserView `json:"user"`
	// TokenStore says where the PAT landed: "keyring" or "file".
	TokenStore string `json:"tokenStore"`
	// PinNote is the signing-key TOFU outcome.
	PinNote string `json:"pinNote"`
	// KnowledgeRewrapped names the knowledge packs this computer's key
	// wrapped that the sign-in moved to the account's key; KnowledgeNote
	// says why some were not moved.
	KnowledgeRewrapped []string `json:"knowledgeRewrapped,omitempty"`
	KnowledgeNote      string   `json:"knowledgeNote,omitempty"`
}

// SignIn validates a PAT against the registry, stores it, and pins the
// registry's signing keys (TOFU) — login is the moment a human deliberately
// chose this registry, which is exactly when trust should be established.
func (s *SettingsAPI) SignIn(url, token string) (*SignInResult, error) {
	url = settings.Normalize(url)
	if err := settings.ValidateURL(url); err != nil {
		return nil, err
	}
	// Only the two sign-ins the app keeps: the token is not even sent to
	// a registry outside them to be checked.
	cfg, err := settings.Load()
	if err != nil {
		return nil, err
	}
	if err := cfg.Admit(url); err != nil {
		return nil, err
	}
	if _, reg := cfg.SignInFor(url); reg != "" {
		url = reg
	}
	user, err := whoami(s.app, url, token)
	if err != nil {
		return nil, err
	}
	// Sign-in implies "keep it": the registry joins the list before its
	// token is stored, which is what makes it the self-hosted sign-in.
	if cfg.Find(url) == nil {
		cfg.Upsert(settings.Registry{URL: url})
		if cfg.DefaultRegistry == "" {
			cfg.DefaultRegistry = url
		}
		if err := settings.Save(cfg); err != nil {
			return nil, fmt.Errorf("the PAT is valid but %s could not be added to Settings: %w", settings.DisplayOrigin(url), err)
		}
	}
	store, err := settings.StoreToken(url, token)
	if err != nil {
		return nil, fmt.Errorf("the PAT is valid but could not be stored: %w", err)
	}
	pin := trust.PinAtLogin(s.app.ctx, registry.HTTPClient, url)
	// A schedule that runs signed out carries the registry token sealed
	// beside it: the new one is sealed in its place.
	resealSchedules(s.app)
	res := &SignInResult{User: *user, TokenStore: store, PinNote: pin}
	// Packs kept while signed out are encrypted to this computer's key; from
	// this sign-in on, the account's key at this registry wraps them, so
	// they open on the account's other computers too.
	res.KnowledgeRewrapped, res.KnowledgeNote = rewrapKnowledge(s.app.ctx, knowledgeEnv(s.knowledgeEnv), url)
	return res, nil
}

// SignOut removes the stored credential for a registry.
func (s *SettingsAPI) SignOut(url string) (*SettingsView, error) {
	if err := settings.DeleteToken(url); err != nil {
		return nil, err
	}
	resealSchedules(s.app)
	return settingsView()
}

// Whoami reports the identity the stored credential resolves to.
func (s *SettingsAPI) Whoami(url string) (*UserView, error) {
	url, token, err := storedCredential(url)
	if err != nil {
		return nil, err
	}
	return whoami(s.app, url, token)
}

// storedCredential is a page-supplied registry URL held to Settings' rule
// before its token is looked up: the keychain keys a token by host, so an
// http:// spelling of an added https registry would otherwise send it in
// clear.
func storedCredential(url string) (string, string, error) {
	url = settings.Normalize(url)
	if err := settings.ValidateURL(url); err != nil {
		return "", "", err
	}
	token := settings.TokenFor(url)
	if token == "" {
		return "", "", fmt.Errorf("not signed in to %s", originOf(url))
	}
	return url, token, nil
}

// whoami's errors name the registry by origin, never by a URL someone may
// have typed a credential into, since Settings renders them.
func whoami(app *App, url, token string) (*UserView, error) {
	client, err := registry.New(url, token)
	if err != nil {
		return nil, fmt.Errorf("%s is not a registry address this app can use", originOf(url))
	}
	resp, err := client.GetCurrentUserWithResponse(app.ctx)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", originOf(url), err)
	}
	if resp.JSON200 == nil {
		return nil, registry.Problem("sign in", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	return userViewOf(resp.JSON200), nil
}

// userViewOf is the registry's user as Settings shows it.
func userViewOf(u *apiclient.User) *UserView {
	v := &UserView{Handle: u.Handle, DisplayName: u.DisplayName, Email: u.Email, EmailVerified: u.EmailVerified}
	if u.Orgs != nil {
		for _, o := range *u.Orgs {
			m := OrgMembership{Handle: o.Handle, Role: o.Role}
			if o.State != nil {
				m.State = string(*o.State)
			}
			v.Orgs = append(v.Orgs, m)
		}
	}
	if u.PendingOrg != nil {
		v.PendingOrg = &PendingOrgView{Handle: u.PendingOrg.Handle, DisplayName: u.PendingOrg.DisplayName, Via: string(u.PendingOrg.Via)}
	}
	return v
}

// MetaView is a registry's self-description.
type MetaView struct {
	Version       string          `json:"version"`
	APIVersion    string          `json:"apiVersion"`
	MinCLIVersion string          `json:"minCliVersion,omitempty"`
	Features      map[string]bool `json:"features"`
}

// RegistryMeta pings a registry's /meta.
func (s *SettingsAPI) RegistryMeta(url string) (*MetaView, error) {
	client, err := registry.New(url, "")
	if err != nil {
		return nil, err
	}
	resp, err := client.GetMetaWithResponse(s.app.ctx)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", url, err)
	}
	if resp.JSON200 == nil {
		return nil, registry.Problem("meta", resp.StatusCode(), nil, resp.Body)
	}
	m := resp.JSON200
	v := &MetaView{Version: m.Version, APIVersion: m.ApiVersion, Features: m.Features}
	if m.MinCLIVersion != nil {
		v.MinCLIVersion = *m.MinCLIVersion
	}
	return v, nil
}

// ------------------------------------------------------------------ browse

// BrowseAPI serves the search and agent-page surfaces.
type BrowseAPI struct {
	app *App
}

// SearchQuery mirrors the GET /search facets.
type SearchQuery struct {
	Q          string `json:"q"`
	Mode       string `json:"mode,omitempty"`
	Capability string `json:"capability,omitempty"`
	License    string `json:"license,omitempty"`
	Sort       string `json:"sort,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
}

// SearchHitView is one search result row.
type SearchHitView struct {
	Namespace   string   `json:"namespace"`
	Name        string   `json:"name"`
	Modes       []string `json:"modes"`
	Summary     string   `json:"summary"`
	Installs30d int      `json:"installs30d"`
	Quarantine  string   `json:"quarantine,omitempty"`
}

// SearchView is a search response. Total is a pointer because the registry
// may not send one (older cores never did), and a count the registry did not
// give is not a count: the page prints nothing then, rather than a zero.
type SearchView struct {
	Total   *int                      `json:"total,omitempty"`
	Results []SearchHitView           `json:"results"`
	Facets  map[string]map[string]int `json:"facets,omitempty"`
}

// Search queries a registry.
func (b *BrowseAPI) Search(reg string, q SearchQuery) (*SearchView, error) {
	client, err := registry.New(reg, settings.TokenFor(reg))
	if err != nil {
		return nil, err
	}
	params := &apiclient.SearchAgentsParams{}
	if q.Q != "" {
		params.Q = &q.Q
	}
	if q.Mode != "" {
		mo := apiclient.AgentMode(q.Mode)
		params.Mode = &mo
	}
	if q.Capability != "" {
		params.Capability = &q.Capability
	}
	if q.License != "" {
		params.License = &q.License
	}
	if q.Sort != "" {
		s := apiclient.SearchAgentsParamsSort(q.Sort)
		params.Sort = &s
	}
	if q.Limit > 0 {
		params.Limit = &q.Limit
	}
	if q.Offset > 0 {
		params.Offset = &q.Offset
	}
	resp, err := client.SearchAgentsWithResponse(b.app.ctx, params)
	if err != nil {
		return nil, fmt.Errorf("search %s: %w", reg, err)
	}
	if resp.JSON200 == nil {
		return nil, registry.Problem("search", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	out := &SearchView{Results: []SearchHitView{}}
	out.Total = resp.JSON200.Total
	if resp.JSON200.Facets != nil {
		out.Facets = *resp.JSON200.Facets
	}
	for _, hit := range resp.JSON200.Results {
		h := SearchHitView{
			Namespace: hit.Namespace, Name: hit.Name,
			Modes: apiModes(hit.Modes), Summary: hit.Summary,
		}
		if hit.Installs30d != nil {
			h.Installs30d = *hit.Installs30d
		}
		if hit.QuarantineState != nil && *hit.QuarantineState != "none" {
			h.Quarantine = string(*hit.QuarantineState)
		}
		out.Results = append(out.Results, h)
	}
	return out, nil
}

// VersionView is one row of an agent's version history.
type VersionView struct {
	Version       string `json:"version"`
	Channel       string `json:"channel"`
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	PublishedAt   string `json:"publishedAt,omitempty"`
	ArchiveDigest string `json:"archiveDigest,omitempty"`
	Yanked        bool   `json:"yanked"`
	YankReason    string `json:"yankReason,omitempty"`
	// ScanStatus is how the registry admitted the version: `pipeline`, or
	// `skipped` for one admitted without a scan, which the page labels
	// unscanned and the install and run surfaces ask consent for. Empty from
	// a registry that predates the field, which never published unscanned.
	ScanStatus string `json:"scanStatus,omitempty"`
}

// AgentView is the agent header.
type AgentView struct {
	Namespace  string   `json:"namespace"`
	Name       string   `json:"name"`
	Modes      []string `json:"modes"`
	Summary    string   `json:"summary"`
	Visibility string   `json:"visibility"`
	Quarantine string   `json:"quarantine,omitempty"`
	CreatedAt  string   `json:"createdAt"`
	GitHTTPURL string   `json:"gitHttpUrl,omitempty"`
	// ForkedFrom is what a fork was taken from; ForkState its copy (copying,
	// ready, failed) and ForkDetail why a copy failed. ForkCount is how many
	// forks a public QuickAgent has, a number only.
	ForkedFrom *AgentForkOrigin `json:"forkedFrom,omitempty"`
	ForkState  string           `json:"forkState,omitempty"`
	ForkDetail string           `json:"forkDetail,omitempty"`
	ForkCount  int              `json:"forkCount,omitempty"`
}

// AgentForkOrigin is a fork's origin as its page says it: Available is
// whether the origin can still be read, so its name opens its page.
type AgentForkOrigin struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Version   string `json:"version"`
	Available bool   `json:"available"`
}

// AgentDetailView is the agent page's model.
type AgentDetailView struct {
	Agent    AgentView     `json:"agent"`
	Versions []VersionView `json:"versions"`
	// Latest is the newest PUBLISHED version ("" when none is published).
	Latest string `json:"latest"`
	// LatestCapabilities is the declared capabilities block of Latest, as stored.
	LatestCapabilities map[string]interface{} `json:"latestCapabilities,omitempty"`
	// LatestDigest is Latest's archive digest (provenance panel).
	LatestDigest string `json:"latestDigest,omitempty"`
	// LatestScanStatus is Latest's ScanStatus (provenance panel).
	LatestScanStatus string `json:"latestScanStatus,omitempty"`
	// DependencyAlerts are the signed-in viewer's open alerts on this agent:
	// versions they published that declare an MCP server version with a
	// newly known high or critical vulnerability. Empty for a viewer who is
	// not the publisher or not signed in.
	DependencyAlerts []DependencyAlertView `json:"dependencyAlerts"`
	// CanPublish is the registry's answer to whether this sign-in may add a
	// version to the QuickAgent (GET /agents canPublish): what the page
	// offers Edit on. False for a viewer who is not signed in, and from a
	// registry that predates it.
	CanPublish bool `json:"canPublish"`
}

// DependencyAlertView is one open dependency alert.
type DependencyAlertView struct {
	ID             string   `json:"id"`
	Version        string   `json:"version"`
	Ecosystem      string   `json:"ecosystem"`
	Package        string   `json:"package"`
	PackageVersion string   `json:"packageVersion"`
	Severity       string   `json:"severity"`
	AdvisoryIDs    []string `json:"advisoryIds"`
	FixedIn        []string `json:"fixedIn"`
	CreatedAt      string   `json:"createdAt"`
}

// Agent fetches the agent header plus version history.
func (b *BrowseAPI) Agent(reg, ns, name string) (*AgentDetailView, error) {
	client, err := registry.New(reg, settings.TokenFor(reg))
	if err != nil {
		return nil, err
	}
	aresp, err := client.GetAgentWithResponse(b.app.ctx, ns, name)
	if err != nil {
		return nil, fmt.Errorf("fetch agent: %w", err)
	}
	if aresp.JSON200 == nil {
		return nil, registry.Problem("agent", aresp.StatusCode(), aresp.ApplicationproblemJSONDefault, aresp.Body)
	}
	a := aresp.JSON200
	out := &AgentDetailView{
		Agent: AgentView{
			Namespace: a.Namespace, Name: a.Name, Modes: apiModes(a.Modes),
			Summary: a.Summary, Visibility: string(a.Visibility),
			CreatedAt: a.CreatedAt.Format(time.RFC3339), GitHTTPURL: a.GitHttpUrl,
		},
		Versions:         []VersionView{},
		DependencyAlerts: []DependencyAlertView{},
		CanPublish:       a.CanPublish != nil && *a.CanPublish,
	}
	if settings.TokenFor(reg) != "" {
		// The alerts endpoint is the caller's own; a viewer who is not the
		// publisher gets none, and a registry that cannot answer (an older
		// release, a lapsed token) leaves the list empty rather than
		// failing the page.
		if dresp, derr := client.ListUserDependencyAlertsWithResponse(b.app.ctx); derr == nil && dresp.JSON200 != nil {
			for _, al := range *dresp.JSON200 {
				if al.Agent != ns+"/"+name {
					continue
				}
				out.DependencyAlerts = append(out.DependencyAlerts, DependencyAlertView{
					ID: al.Id.String(), Version: al.Version, Ecosystem: string(al.Ecosystem),
					Package: al.Package, PackageVersion: al.PackageVersion, Severity: string(al.Severity),
					AdvisoryIDs: emptyNotNil(al.AdvisoryIds), FixedIn: emptyNotNil(al.FixedIn),
					CreatedAt: al.CreatedAt.Format(time.RFC3339),
				})
			}
		}
	}
	if a.QuarantineState != "none" {
		out.Agent.Quarantine = string(a.QuarantineState)
	}
	if f := a.ForkedFrom; f != nil {
		out.Agent.ForkedFrom = &AgentForkOrigin{Namespace: f.Namespace, Name: f.Name, Version: f.Version, Available: f.Available}
	}
	if a.ForkState != nil {
		out.Agent.ForkState = string(*a.ForkState)
	}
	if a.ForkDetail != nil {
		out.Agent.ForkDetail = *a.ForkDetail
	}
	if a.ForkCount != nil {
		out.Agent.ForkCount = *a.ForkCount
	}

	vresp, err := client.ListVersionsWithResponse(b.app.ctx, ns, name)
	if err != nil {
		return nil, fmt.Errorf("fetch versions: %w", err)
	}
	if vresp.JSON200 == nil {
		return nil, registry.Problem("versions", vresp.StatusCode(), vresp.ApplicationproblemJSONDefault, vresp.Body)
	}
	for _, ver := range *vresp.JSON200 {
		vv := VersionView{
			Version: ver.Version, Channel: string(ver.Channel), Status: string(ver.Status),
			CreatedAt: ver.CreatedAt.Format(time.RFC3339),
			Yanked:    ver.YankedAt != nil,
		}
		if ver.PublishedAt != nil {
			vv.PublishedAt = ver.PublishedAt.Format(time.RFC3339)
		}
		if ver.ArchiveDigest != nil {
			vv.ArchiveDigest = *ver.ArchiveDigest
		}
		if ver.YankReason != nil {
			vv.YankReason = *ver.YankReason
		}
		if ver.ScanStatus != nil {
			vv.ScanStatus = string(*ver.ScanStatus)
		}
		out.Versions = append(out.Versions, vv)
		if vv.Status == "published" && !vv.Yanked && out.Latest == "" {
			// ListVersions returns newest-first; the first published,
			// non-yanked row is the latest.
			out.Latest = ver.Version
			out.LatestDigest = vv.ArchiveDigest
			out.LatestScanStatus = vv.ScanStatus
			if ver.Capabilities != nil {
				out.LatestCapabilities = *ver.Capabilities
			}
		}
	}
	return out, nil
}

// Readme returns the API's server-sanitized README HTML.
// apiModes renders the API's mode set for a view (never nil: the
// frontend maps over it).
func apiModes(modes apiclient.AgentModes) []string {
	out := make([]string, 0, len(modes))
	for _, m := range modes {
		out = append(out, string(m))
	}
	return out
}

func (b *BrowseAPI) Readme(reg, ns, name, version string) (string, error) {
	client, err := registry.New(reg, settings.TokenFor(reg))
	if err != nil {
		return "", err
	}
	return registry.Readme(b.app.ctx, client, ns, name, version)
}

// InstructionsView is a release's instruction file as the Agent tab shows it
// under the README: the path the manifest names (`AGENT.md` when it names
// none) and its text. The text is shown as text, never rendered: the client
// parses no markdown from the archive.
type InstructionsView struct {
	Path string `json:"path"`
	// Content is "" with Binary set when the registry would not read the
	// file as text.
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"`
}

// Instructions reads a version's instruction file for the Agent tab.
func (b *BrowseAPI) Instructions(reg, ns, name, version string) (*InstructionsView, error) {
	client, err := registry.New(reg, settings.TokenFor(reg))
	if err != nil {
		return nil, err
	}
	_, path, f, err := readInstructionFile(b.app.ctx, client, ns, name, version)
	if err != nil {
		return nil, err
	}
	out := &InstructionsView{Path: path, Truncated: f.Truncated, Binary: f.Binary || f.Content == nil}
	if !out.Binary {
		out.Content = *f.Content
	}
	return out, nil
}

// readInstructionFile reads a version's manifest, takes the instruction
// file it names, and reads that file through the version's file endpoint:
// what Edit loads into the form and the Agent tab shows. The callers part
// on what a truncated or binary file means to them.
func readInstructionFile(ctx context.Context, client *apiclient.ClientWithResponses, ns, name, version string) (manifest []byte, path string, f *apiclient.ReviewFileContent, err error) {
	mresp, err := client.GetVersionManifestWithResponse(ctx, ns, name, version)
	if err != nil {
		return nil, "", nil, fmt.Errorf("fetch the manifest of %s: %w", version, err)
	}
	if mresp.JSON200 == nil {
		return nil, "", nil, registry.Problem("manifest", mresp.StatusCode(), mresp.ApplicationproblemJSONDefault, mresp.Body)
	}
	path = build.ReleaseInstructionPath(mresp.Body)
	fresp, err := client.GetVersionFileWithResponse(ctx, ns, name, version, &apiclient.GetVersionFileParams{Path: path})
	if err != nil {
		return nil, "", nil, fmt.Errorf("fetch %s of %s: %w", path, version, err)
	}
	if fresp.JSON200 == nil {
		return nil, "", nil, registry.Problem("file", fresp.StatusCode(), fresp.ApplicationproblemJSONDefault, fresp.Body)
	}
	return mresp.Body, path, fresp.JSON200, nil
}

// FindingView is one report-card finding.
type FindingView struct {
	Scanner       string `json:"scanner"`
	Severity      string `json:"severity"`
	Confidence    string `json:"confidence"`
	Title         string `json:"title"`
	FilePath      string `json:"filePath,omitempty"`
	LineStart     int    `json:"lineStart,omitempty"`
	LineEnd       int    `json:"lineEnd,omitempty"`
	Excerpt       string `json:"excerpt,omitempty"`
	Verdict       string `json:"verdict"`
	VerdictReason string `json:"verdictReason,omitempty"`
}

// ReportView is the security report card: findings by severity, ruleset
// version, scan date — never a "safe" claim.
type ReportView struct {
	VersionStatus string `json:"versionStatus"`
	ScanStatus    string `json:"scanStatus,omitempty"`
	// Verdict is what the report is: `scanned` where a run stands behind it,
	// `unscanned` where the registry admitted the version without one —
	// never conflated with a scan that found nothing — and empty while a
	// scan is pending.
	Verdict        string         `json:"verdict,omitempty"`
	RulesetVersion string         `json:"rulesetVersion,omitempty"`
	JudgeModel     string         `json:"judgeModel,omitempty"`
	FinishedAt     string         `json:"finishedAt,omitempty"`
	FindingCounts  map[string]int `json:"findingCounts,omitempty"`
	Findings       []FindingView  `json:"findings"`
}

// Report fetches a version's scan report.
func (b *BrowseAPI) Report(reg, ns, name, version string) (*ReportView, error) {
	client, err := registry.New(reg, settings.TokenFor(reg))
	if err != nil {
		return nil, err
	}
	resp, err := client.GetVersionReportWithResponse(b.app.ctx, ns, name, version)
	if err != nil {
		return nil, fmt.Errorf("fetch report: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, registry.Problem("report", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	r := resp.JSON200
	out := &ReportView{VersionStatus: string(r.VersionStatus), Findings: []FindingView{}}
	if r.ScanStatus != nil {
		out.ScanStatus = *r.ScanStatus
	}
	if r.Verdict != nil {
		out.Verdict = string(*r.Verdict)
	}
	if r.RulesetVersion != nil {
		out.RulesetVersion = *r.RulesetVersion
	}
	if r.JudgeModel != nil {
		out.JudgeModel = *r.JudgeModel
	}
	if r.FinishedAt != nil {
		out.FinishedAt = r.FinishedAt.Format(time.RFC3339)
	}
	if r.FindingCounts != nil {
		out.FindingCounts = *r.FindingCounts
	}
	for _, f := range r.Findings {
		fv := FindingView{
			Scanner: f.Scanner, Severity: string(f.Severity), Confidence: string(f.Confidence),
			Title: f.Title, Verdict: string(f.Verdict),
		}
		if f.FilePath != nil {
			fv.FilePath = *f.FilePath
		}
		if f.LineStart != nil {
			fv.LineStart = *f.LineStart
		}
		if f.LineEnd != nil {
			fv.LineEnd = *f.LineEnd
		}
		if f.Excerpt != nil {
			fv.Excerpt = *f.Excerpt
		}
		if f.VerdictReason != nil {
			fv.VerdictReason = *f.VerdictReason
		}
		out.Findings = append(out.Findings, fv)
	}
	return out, nil
}

// Provenance verifies the version's DSSE attestation against the DECLARED
// archive digest — no download involved, so this is the browse-time badge:
// "the registry's signing key vouches for this digest". Install re-verifies
// against the actual downloaded bytes. scanStatus is the version's as the
// registry lists it: `skipped` is a version admitted without a scan,
// unscanned before any attestation is looked for, because the absence is the
// fact and not a missing signature.
func (b *BrowseAPI) Provenance(reg, ns, name, version, declaredDigest, scanStatus string) (*trust.AttestationResult, error) {
	if scanStatus == string(apiclient.AgentVersionScanStatusSkipped) {
		att := trust.Unscanned()
		return &att, nil
	}
	reg = settings.Normalize(reg)
	attURL := fmt.Sprintf("/api/v1/agents/%s/%s/versions/%s/attestation", ns, name, version)
	att := trust.VerifyAttestation(b.app.ctx, registry.HTTPClient, reg, settings.TokenFor(reg), attURL, declaredDigest)
	return &att, nil
}

// ----------------------------------------------------------------- install

// InstallAPI drives the install/installed surfaces.
type InstallAPI struct {
	app    *App
	engine *install.Engine
	// The last check's result: read by the chrome for the Installed badge,
	// never started by a read. checking keeps a slow launch round from
	// landing after a later one and overwriting it with the older answer.
	checking sync.Mutex
	mu       sync.Mutex
	updates  InstallUpdatesView
}

// InstallUpdatesView is a snapshot of the last check of the installed
// QuickAgents for newer versions: UpdatesStatus never starts one, so a
// paint costs nothing. The app checks once on launch and when a
// QuickAgent's page is opened (that one through CheckUpdate); nothing is
// applied on its own — Update is the person's click.
type InstallUpdatesView struct {
	// Checked is when the last check finished; zero means none has.
	Checked string `json:"checked,omitempty"`
	// Available is how many records this app manages, whose directory is
	// still there, have a newer version on their registry: the badge's
	// number — the rows the Installed page would offer Update for.
	Available int `json:"available"`
	// Updates is every record's answer, errors included per record.
	Updates []install.UpdateInfo `json:"updates"`
	// Err is why the state file could not be read; per-record trouble is
	// on the record.
	Err string `json:"err,omitempty"`
}

// start runs the launch check in its own goroutine, once (on launch and on
// opening a QuickAgent's page; a person's "Check for updates" asks again;
// nothing on a timer). It must not delay the first paint.
func (i *InstallAPI) start(ctx context.Context) {
	go i.check(ctx)
}

// check runs one round over every record and stores the result: whatever
// this round concluded is what the badge shows. Rounds do not overlap.
func (i *InstallAPI) check(ctx context.Context) []install.UpdateInfo {
	i.checking.Lock()
	defer i.checking.Unlock()
	view := InstallUpdatesView{Updates: []install.UpdateInfo{}}
	infos, err := install.CheckUpdates(ctx)
	if err != nil {
		view.Err = err.Error()
	} else {
		view.Updates = infos
	}
	view.Checked = time.Now().UTC().Format(time.RFC3339)
	i.mu.Lock()
	i.updates = view
	i.recount()
	i.mu.Unlock()
	return infos
}

// recount sets Available from the rows; the caller holds mu.
func (i *InstallAPI) recount() {
	i.updates.Available = 0
	for _, u := range i.updates.Updates {
		if u.UpdateAvailable && u.Manageable && u.Present {
			i.updates.Available++
		}
	}
}

// refreshRow re-asks about one record after this app changed it — an
// install, an update, a removal — so the badge and the page agree at
// once rather than after the next round. A record that is gone loses its
// row.
func (i *InstallAPI) refreshRow(key installed.Key) {
	info, err := install.CheckUpdateForKey(i.app.ctx, key)
	i.mu.Lock()
	defer i.mu.Unlock()
	rows := i.updates.Updates[:0:0]
	for _, u := range i.updates.Updates {
		if u.Key != key {
			rows = append(rows, u)
		}
	}
	if err == nil && info != nil {
		rows = append(rows, *info)
	}
	i.updates.Updates = rows
	i.recount()
}

// UpdatesStatus returns the last check's result without starting one.
func (i *InstallAPI) UpdatesStatus() InstallUpdatesView {
	i.mu.Lock()
	defer i.mu.Unlock()
	v := i.updates
	if v.Updates == nil {
		v.Updates = []install.UpdateInfo{}
	}
	return v
}

// CheckUpdate asks about one QuickAgent's runner-home record, the way the
// agent page does on opening: nil when nothing is installed for it. The
// stored view takes the answer, so the badge agrees with the page.
func (i *InstallAPI) CheckUpdate(namespace, name string) (*install.UpdateInfo, error) {
	info, err := install.CheckUpdateFor(i.app.ctx, namespace, name)
	if err != nil || info == nil {
		return info, err
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	replaced := false
	for n := range i.updates.Updates {
		if i.updates.Updates[n].Key == info.Key {
			i.updates.Updates[n] = *info
			replaced = true
			break
		}
	}
	if !replaced {
		i.updates.Updates = append(i.updates.Updates, *info)
	}
	i.recount()
	return info, nil
}

// Prepare resolves, downloads, and verifies; nothing is written. The plan
// carries every consent question for the UI to ask.
func (i *InstallAPI) Prepare(reg, ref, target, dir string) (*install.Plan, error) {
	return i.engine.Prepare(i.app.ctx, reg, ref, target, dir)
}

// Confirm applies the user's answers and writes the install. Each boolean is
// one consent the page collected; acceptUnscanned is the consent for a
// release the registry admitted without a scan, which acceptUnsigned does
// not give.
func (i *InstallAPI) Confirm(planID string, acceptUnsigned, acceptCapabilities, acceptUnscanned bool) (*install.Result, error) {
	// A runner plan's record is the run's consent record: what the page says
	// it accepted is not what a run will believe, so the plan's own gates
	// are raised here as native dialogs and the page's booleans are not read
	// — a page could otherwise prepare a runner plan and confirm the
	// contract on every future run's behalf.
	plan, ok := i.engine.Plan(planID)
	if ok && plan.Target == installdir.Target {
		if err := i.consentForRunner(plan); err != nil {
			i.engine.Cancel(planID)
			return nil, err
		}
		res, err := i.engine.Confirm(planID, plan.Unsigned, plan.NeedsCapabilityConfirm || plan.RequiresWidenedConfirm, plan.Unscanned)
		if err == nil {
			i.refreshRow(installdir.ConsentKey(runnerHomeOr(plan.Paths.Dest), plan.Namespace, plan.Name))
		}
		return res, err
	}
	// A consent the page passed is put to the person in a native dialog the
	// page cannot answer; the capabilities acceptance is the wizard's own
	// reading of the contract and stays the page's.
	if err := i.askProvenanceConsent(acceptUnsigned, acceptUnscanned); err != nil {
		return nil, err
	}
	res, err := i.engine.Confirm(planID, acceptUnsigned, acceptCapabilities, acceptUnscanned)
	if err == nil && ok {
		i.refreshRow(installed.Key{Registry: plan.Registry, Namespace: plan.Namespace, Name: plan.Name, Target: plan.Target, Dir: plan.Paths.Dest})
	}
	return res, err
}

// askProvenanceConsent puts the provenance consents to the person: a
// release with no attestation, one the registry admitted without a scan,
// or both in one dialog. Nothing to accept raises nothing.
func (i *InstallAPI) askProvenanceConsent(unsigned, unscanned bool) error {
	var accepting []string
	if unsigned {
		accepting = append(accepting, "a release with no attestation")
	}
	if unscanned {
		accepting = append(accepting, "a release the registry admitted without a scan")
	}
	if len(accepting) == 0 {
		return nil
	}
	return i.app.askConsent("Install anyway?", "This install accepts "+strings.Join(accepting, ", and ")+". Continue?")
}

// Cancel discards a pending plan.
func (i *InstallAPI) Cancel(planID string) {
	i.engine.Cancel(planID)
}

// InstallRunner is the one-click install: the release resolved, verified and
// laid down in the runner home — the directory a run reads — with no target
// or directory to choose. With Settings → Coding agents' "also install into
// <coding agent>" on, the same verified bytes are also written where the
// coding agent reads — the Claude skill or AGENTS.md, whichever the policy
// implies — as an export with its own record. The gates are the only
// interruption, each put to the person in the native dialog the page cannot
// answer: an unsigned release, one the registry admitted without a scan, and
// the capability contract on a first install that declares egress or exec or
// on an update that widens it — the contract itself in the dialog, since no
// page is showing it, and where the export goes too, since the coding agent
// enforces none of it. A declined dialog installs nothing and says so; an
// export that cannot be written is the result's warning, not a failed
// install.
func (i *InstallAPI) InstallRunner(reg, ref string) (*install.Result, error) {
	plan, err := i.engine.Prepare(i.app.ctx, reg, ref, installdir.Target, "")
	if err != nil {
		return nil, err
	}
	export, exportNote := i.exportPlanFor(plan)
	cancelExport := func() {
		if export != nil {
			i.engine.Cancel(export.ID)
		}
	}
	if err := i.consentForInstall(plan, export); err != nil {
		i.engine.Cancel(plan.ID)
		cancelExport()
		return nil, err
	}
	res, err := i.engine.Confirm(plan.ID, plan.Unsigned, plan.NeedsCapabilityConfirm || plan.RequiresWidenedConfirm, plan.Unscanned)
	if err != nil {
		cancelExport()
		return nil, err
	}
	// The record changed: the badge must not go on counting an update
	// that was just applied, and the page reads the snapshot next.
	i.refreshRow(installdir.ConsentKey(runnerHomeOr(plan.Paths.Dest), plan.Namespace, plan.Name))
	if export != nil {
		eres, eerr := i.engine.Confirm(export.ID, export.Unsigned, export.NeedsCapabilityConfirm || export.RequiresWidenedConfirm, export.Unscanned)
		if eerr != nil {
			exportNote = fmt.Sprintf("not installed into %s: %v", exportLabel(export.Target), eerr)
		} else {
			res.Message += "; " + eres.Message
			i.refreshRow(installed.Key{Registry: export.Registry, Namespace: export.Namespace, Name: export.Name, Target: export.Target, Dir: export.Paths.Dest})
		}
	}
	if exportNote != "" {
		if res.Warning != "" {
			res.Warning += "; "
		}
		res.Warning += exportNote
		res.Message += " — " + exportNote
	}
	return res, nil
}

// exportPlanFor derives the coding-agent export a one-click install also
// writes when Settings says so: nil with the switch off; nil with the
// sentence for the result when the export cannot be prepared (no project
// directory chosen for AGENTS.md, a settings file that will not read).
func (i *InstallAPI) exportPlanFor(runner *install.Plan) (*install.Plan, string) {
	x, err := install.ExportSettings()
	if err != nil {
		return nil, "the coding-agent export was not written: " + err.Error()
	}
	if !x.Enabled {
		return nil, ""
	}
	if x.NeedsDir {
		return nil, "not installed into " + exportLabel(x.Target) + ": no project directory is chosen in Settings → Coding agents"
	}
	export, err := i.engine.PrepareExport(runner.ID, x.Target, x.PlanDir())
	if err != nil {
		return nil, "not installed into " + exportLabel(x.Target) + ": " + err.Error()
	}
	return export, ""
}

// exportLabel names a coding-agent export the way the page does.
func exportLabel(target string) string {
	if target == manifest.InterfaceClaudeSkill {
		return "the Claude skill"
	}
	return "AGENTS.md"
}

// consentForInstall raises the gates a one-click install owes, each in the
// native dialog, and returns the first refusal: nothing is written past a
// No. The provenance consent is one question for the one release, whatever
// it is written as; the contract dialog is raised when the runner install
// or the export owes it, and names the export when there is one, since a
// coding agent enforces nothing of the contract. A plan with no gate
// raises nothing.
func (i *InstallAPI) consentForInstall(plan, export *install.Plan) error {
	if err := i.askProvenanceConsent(plan.Unsigned, plan.Unscanned); err != nil {
		return err
	}
	owes := func(p *install.Plan) bool {
		return p != nil && (p.NeedsCapabilityConfirm || p.RequiresWidenedConfirm)
	}
	if owes(plan) || owes(export) {
		contract := plan
		if !owes(plan) {
			contract = export
		}
		if err := i.app.askConsent("Accept what "+plan.Namespace+"/"+plan.Name+" may do?", contractConsentText(contract, export)); err != nil {
			return err
		}
	}
	return nil
}

// consentForRunner is consentForInstall with no export.
func (i *InstallAPI) consentForRunner(plan *install.Plan) error {
	return i.consentForInstall(plan, nil)
}

// contractConsentText is the capability contract as the consent dialog
// states it: on an update, what widened since the installed version; on a
// first install, what the release declares — both when the runner home is
// updated and the export is a first install, since the export's record
// will carry the whole declaration. The dialog is the one reading of the
// contract the person gets on a one-click install; with an export it says
// where else the release goes, and that nothing enforces the contract
// there.
func contractConsentText(plan, export *install.Plan) string {
	var lines []string
	widened := export
	if plan.RequiresWidenedConfirm && plan.Diff != nil {
		widened = plan
	}
	if widened != nil && widened.RequiresWidenedConfirm && widened.Diff != nil {
		lines = append(lines, "Updating from "+widened.PrevVersion+" to "+widened.Version+" widens what it may do:")
		for _, w := range widened.Diff.Widened {
			lines = append(lines, "  • "+w)
		}
	}
	if plan.NeedsCapabilityConfirm || (export != nil && export.NeedsCapabilityConfirm) {
		lines = append(lines, "Version "+plan.Version+" declares:")
		c := plan.Capabilities
		if len(c.EgressHosts) > 0 {
			lines = append(lines, "  • network egress to "+strings.Join(c.EgressHosts, ", "))
		}
		if len(c.ExecCommands) > 0 {
			lines = append(lines, "  • exec: "+strings.Join(c.ExecCommands, ", "))
		}
		if c.FSRead != "" {
			lines = append(lines, "  • filesystem read: "+c.FSRead+pathsNote(c.FSReadPaths))
		}
		if c.FSWrite != "" {
			lines = append(lines, "  • filesystem write: "+c.FSWrite+pathsNote(c.FSWritePaths))
		}
		if len(c.Secrets) > 0 {
			lines = append(lines, "  • secrets: "+strings.Join(c.Secrets, ", "))
		}
		// A target host and a browser, in the CLI's consent words: either
		// one alone asks for this consent, so the dialog names it.
		if len(c.TargetHosts) > 0 {
			lines = append(lines, "  • target hosts: "+strings.Join(c.TargetHosts, ", ")+" (asks for a server when it runs, and can run any command on it as the account you sign in with)")
		}
		if c.Browser != nil {
			lines = append(lines, "  • browser: "+mcpview.BrowserLine(c.Browser))
		}
		if len(c.Knowledge) > 0 {
			lines = append(lines, "  • knowledge: ships "+shippedKnowledgeText(plan))
		}
	}
	closing := "The runner is configured to enforce this contract."
	if export != nil {
		closing += " It is also installed as " + exportLabel(export.Target) + " (" + export.Paths.Dest + "), which does not enforce it."
	}
	lines = append(lines, "", closing+" Install?")
	return strings.Join(lines, "\n")
}

// shippedKnowledgeText names the knowledge packs a release ships with their
// sizes where the archive gave them, "aws-docs (4.2 MiB), notes" (the CLI's
// consent line, in the same binary units as the install review's
// fmtBinaryBytes).
func shippedKnowledgeText(plan *install.Plan) string {
	size := map[string]int64{}
	for _, p := range plan.ShippedKnowledge {
		if p.Bytes != nil {
			size[p.Name] = *p.Bytes
		}
	}
	parts := make([]string, 0, len(plan.Capabilities.Knowledge))
	for _, name := range plan.Capabilities.Knowledge {
		if n, ok := size[name]; ok {
			name += " (" + byteSize(n) + ")"
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}

// byteSize is a size in the binary units the CLI's consent uses: "216
// bytes", "46.9 KiB", "4.2 MiB".
func byteSize(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d bytes", n)
	case n < 1<<20:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
}

// runnerHomeOr is the runner home a runner plan's agent directory sits
// under (the key's home), from the directory itself when the policy's
// home cannot be read.
func runnerHomeOr(agentDir string) string {
	if home, err := policy.Home(); err == nil {
		return home
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(agentDir)))
}

func pathsNote(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return " (" + strings.Join(paths, ", ") + ")"
}

// ListInstalled returns the shared install state (~/.config/agentry/
// installed.yaml — the same file the CLI writes).
func (i *InstallAPI) ListInstalled() ([]install.Item, error) {
	return install.List()
}

// Forget drops a record whose directory is gone; nothing on disk is touched.
// Its pin goes when no other install of it is left.
func (i *InstallAPI) Forget(k installed.Key) error {
	if err := install.Forget(k); err != nil {
		return err
	}
	i.refreshRow(k)
	unpinIfGone(k.Namespace + "/" + k.Name)
	return nil
}

// CheckUpdates asks every record's registry now — the person's "Check for
// updates" — and the snapshot takes the round, so the badge agrees with
// the page that asked.
func (i *InstallAPI) CheckUpdates() ([]install.UpdateInfo, error) {
	infos := i.check(i.app.ctx)
	if v := i.UpdatesStatus(); v.Err != "" {
		return nil, errors.New(v.Err)
	}
	return infos, nil
}

// Uninstall removes one installation (managed dirs only; see engine docs),
// and its pin when no other install of it is left, and its connections when
// no runner install of it is.
func (i *InstallAPI) Uninstall(k installed.Key) (string, error) {
	msg, err := install.Uninstall(k)
	if err == nil {
		i.refreshRow(k)
		unpinIfGone(k.Namespace + "/" + k.Name)
		// Its connections go with its last runner install, revoked at the
		// vendor where it offers that.
		msg += removeConnectionsIfGone(k)
	}
	return msg, err
}

// UpdateDir reconstructs the directory argument an update needs so it lands
// on the same install key the original install recorded.
func (i *InstallAPI) UpdateDir(rec installed.Record) (string, error) {
	return install.DirForRecord(rec)
}

// ---------------------------------------------------------------- validate

// ValidateAPI is the author drop-zone.
type ValidateAPI struct {
	app *App
}

// Run validates a local agent folder with the same packages the publication
// pipeline runs (parity by construction).
func (v *ValidateAPI) Run(dir string) (*validate.Result, error) {
	if dir == "" {
		return nil, fmt.Errorf("pick a folder to validate")
	}
	return validate.Run(dir)
}
