// Bound services: the thin, JSON-shaped surface the frontend calls. Logic
// lives in internal/…; these methods map API-client types onto flat view
// models (plain strings and ints) so the generated TypeScript bindings stay
// faithful to what actually crosses the JSON boundary.
package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry/pkg/apiclient"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/trust"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/validate"
)

// ---------------------------------------------------------------- settings

// SettingsAPI manages registries and credentials.
type SettingsAPI struct {
	app *App
	// open replaces App.OpenExternal in tests; nil means the real browser.
	open func(url string) error
}

// RegistryView is one configured registry as the frontend sees it.
type RegistryView struct {
	URL      string `json:"url"`
	HasToken bool   `json:"hasToken"`
	// TokenSource is "keyring", "file", or "" when signed out.
	TokenSource string `json:"tokenSource,omitempty"`
	IsDefault   bool   `json:"isDefault"`
}

// SettingsView is the settings page's model.
type SettingsView struct {
	DefaultRegistry string         `json:"defaultRegistry"`
	Registries      []RegistryView `json:"registries"`
	ConfigPath      string         `json:"configPath"`
	// Onboarded is false until the first-run wizard is completed or skipped;
	// the frontend opens the guided setup while it is false and nothing is
	// signed in (agentry-desktop#41).
	Onboarded bool `json:"onboarded"`
}

func settingsView() (*SettingsView, error) {
	s, err := settings.Load()
	if err != nil {
		return nil, err
	}
	path, _ := settings.Path()
	v := &SettingsView{DefaultRegistry: s.DefaultRegistry, Registries: []RegistryView{}, ConfigPath: path, Onboarded: s.Onboarded}
	for _, r := range s.Registries {
		src := settings.TokenSource(r.URL)
		v.Registries = append(v.Registries, RegistryView{
			URL: settings.Normalize(r.URL), HasToken: src != "", TokenSource: src,
			IsDefault: settings.Normalize(r.URL) == settings.Normalize(s.DefaultRegistry),
		})
	}
	return v, nil
}

// Get returns the current settings.
func (s *SettingsAPI) Get() (*SettingsView, error) { return settingsView() }

// MarkOnboarded records that the first-run wizard finished or was skipped,
// so it does not reopen on the next start (agentry-desktop#41). The wizard
// only orchestrates existing services; this flag is the sole state it adds.
func (s *SettingsAPI) MarkOnboarded() (*SettingsView, error) {
	if err := settings.SetOnboarded(); err != nil {
		return nil, err
	}
	return settingsView()
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

// UserView is the signed-in identity, with the organizations it belongs
// to. Those are the core's own facts — every registry answers them,
// whether or not it sells anything — so a membership shows on Settings
// even where there are no tiers to speak of (plan
// organization-subscriptions.md §8, M12).
type UserView struct {
	Handle      string          `json:"handle"`
	DisplayName string          `json:"displayName"`
	Email       string          `json:"email"`
	Orgs        []OrgMembership `json:"orgs,omitempty"`
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
	// PinNote is the signing-key TOFU outcome, §11 login row.
	PinNote string `json:"pinNote"`
}

// SignIn validates a PAT against the registry, stores it, and pins the
// registry's signing keys (TOFU) — login is the moment a human deliberately
// chose this registry, which is exactly when trust should be established.
func (s *SettingsAPI) SignIn(url, token string) (*SignInResult, error) {
	url = settings.Normalize(url)
	if err := settings.ValidateURL(url); err != nil {
		return nil, err
	}
	user, err := whoami(s.app, url, token)
	if err != nil {
		return nil, err
	}
	store, err := settings.StoreToken(url, token)
	if err != nil {
		return nil, fmt.Errorf("the PAT is valid but could not be stored: %w", err)
	}
	// Make sure the registry is in the list (sign-in implies "keep it").
	cfg, err := settings.Load()
	if err == nil {
		if cfg.Find(url) == nil {
			cfg.Upsert(settings.Registry{URL: url})
			if cfg.DefaultRegistry == "" {
				cfg.DefaultRegistry = url
			}
			_ = settings.Save(cfg)
		}
	}
	pin := trust.PinAtLogin(s.app.ctx, registry.HTTPClient, url)
	return &SignInResult{User: *user, TokenStore: store, PinNote: pin}, nil
}

// SignOut removes the stored credential for a registry.
func (s *SettingsAPI) SignOut(url string) (*SettingsView, error) {
	if err := settings.DeleteToken(url); err != nil {
		return nil, err
	}
	return settingsView()
}

// Whoami reports the identity the stored credential resolves to.
func (s *SettingsAPI) Whoami(url string) (*UserView, error) {
	token := settings.TokenFor(url)
	if token == "" {
		return nil, fmt.Errorf("not signed in to %s", url)
	}
	return whoami(s.app, url, token)
}

func whoami(app *App, url, token string) (*UserView, error) {
	client, err := registry.New(url, token)
	if err != nil {
		return nil, err
	}
	resp, err := client.GetCurrentUserWithResponse(app.ctx)
	if err != nil {
		return nil, fmt.Errorf("reach %s: %w", url, err)
	}
	if resp.JSON200 == nil {
		return nil, registry.Problem("sign in", resp.StatusCode(), resp.ApplicationproblemJSONDefault, resp.Body)
	}
	u := resp.JSON200
	v := &UserView{Handle: u.Handle, DisplayName: u.DisplayName, Email: u.Email}
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
	return v, nil
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

// SearchQuery mirrors the GET /search facets (§9.2).
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
// may not send one (cores before agentry#290 never did), and a count the
// registry did not give is not a count: the page prints nothing then,
// rather than the zero this used to carry (agentry-desktop#63).
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
	// `skipped` for one admitted without a scan (plan
	// organization-subscriptions §4), which the page labels unscanned and
	// the install and run surfaces ask consent for. Empty from a registry
	// that predates the field, which never published unscanned.
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
}

// AgentDetailView is the agent page's model.
type AgentDetailView struct {
	Agent    AgentView     `json:"agent"`
	Versions []VersionView `json:"versions"`
	// Latest is the newest PUBLISHED version ("" when none is published).
	Latest string `json:"latest"`
	// LatestCapabilities is the declared §5.4 block of Latest, as stored.
	LatestCapabilities map[string]interface{} `json:"latestCapabilities,omitempty"`
	// LatestDigest is Latest's archive digest (provenance panel).
	LatestDigest string `json:"latestDigest,omitempty"`
	// LatestScanStatus is Latest's ScanStatus (provenance panel).
	LatestScanStatus string `json:"latestScanStatus,omitempty"`
	// DependencyAlerts are the signed-in viewer's open alerts on this agent
	// (agentry#177): versions they published that declare an MCP server
	// version with a newly known high or critical vulnerability. Empty for
	// a viewer who is not the publisher or not signed in.
	DependencyAlerts []DependencyAlertView `json:"dependencyAlerts"`
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

// ReportView is the security report card (§10.3): findings by severity,
// ruleset version, scan date — never a "safe" claim.
type ReportView struct {
	VersionStatus string `json:"versionStatus"`
	ScanStatus    string `json:"scanStatus,omitempty"`
	// Verdict is what the report is (core M21): `scanned` where a run
	// stands behind it, `unscanned` where the registry admitted the
	// version without one — never conflated with a scan that found
	// nothing — and empty while a scan is pending.
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
// registry lists it: `skipped` is a version admitted without a scan (plan
// organization-subscriptions §4), unscanned before any attestation is
// looked for, because the absence is the fact and not a missing signature.
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
	// The last check's result (agentry-desktop#147, D8): read by the chrome
	// for the Installed badge, never started by a read. checking keeps a
	// slow launch round from landing after a later one and overwriting it
	// with the older answer.
	checking sync.Mutex
	mu       sync.Mutex
	updates  InstallUpdatesView
}

// InstallUpdatesView is a snapshot of the last check of the installed
// BriefAgents for newer versions: UpdatesStatus never starts one, so a
// paint costs nothing. The app checks once on launch and when a
// BriefAgent's page is opened (that one through CheckUpdate); nothing is
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

// start runs the launch check in its own goroutine, once (D8: on launch and
// on opening a BriefAgent's page; a person's "Check for updates" asks
// again; nothing on a timer). It must not delay the first paint.
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

// CheckUpdate asks about one BriefAgent's runner-home record, the way the
// agent page does on opening (D8): nil when nothing is installed for it.
// The stored view takes the answer, so the badge agrees with the page.
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

// Confirm applies the user's answers and writes the install. Each boolean
// is one consent the page collected; acceptUnscanned is the consent for a
// release the registry admitted without a scan, which acceptUnsigned does
// not give (M23).
func (i *InstallAPI) Confirm(planID string, acceptUnsigned, acceptCapabilities, acceptUnscanned bool) (*install.Result, error) {
	// A runner plan's record is the run's consent record: what the page
	// says it accepted is not what a run will believe, so the plan's own
	// gates are raised here as native dialogs and the page's booleans are
	// not read (D-030, D-031) — a page could otherwise prepare a runner
	// plan and confirm the contract on every future run's behalf.
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
	// A consent the page passed is put to the person in a native dialog
	// the page cannot answer (D-030); the capabilities acceptance is the
	// wizard's own reading of the contract and stays the page's.
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

// InstallRunner is the one-click install (D-031): the release resolved,
// verified and laid down in the runner home — the directory a run reads —
// with no target or directory to choose. The gates are the only
// interruption, each put to the person in the native dialog the page
// cannot answer (D-030): an unsigned release, one the registry admitted
// without a scan, and the capability contract on a first install that
// declares egress or exec or on an update that widens it — the contract
// itself in the dialog, since no page is showing it. A declined dialog
// installs nothing and says so.
func (i *InstallAPI) InstallRunner(reg, ref string) (*install.Result, error) {
	plan, err := i.engine.Prepare(i.app.ctx, reg, ref, installdir.Target, "")
	if err != nil {
		return nil, err
	}
	if err := i.consentForRunner(plan); err != nil {
		i.engine.Cancel(plan.ID)
		return nil, err
	}
	res, err := i.engine.Confirm(plan.ID, plan.Unsigned, plan.NeedsCapabilityConfirm || plan.RequiresWidenedConfirm, plan.Unscanned)
	if err == nil {
		// The record changed: the badge must not go on counting an update
		// that was just applied, and the page reads the snapshot next.
		i.refreshRow(installdir.ConsentKey(runnerHomeOr(plan.Paths.Dest), plan.Namespace, plan.Name))
	}
	return res, err
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

// consentForRunner raises the gates a one-click install owes, each in the
// native dialog, and returns the first refusal: nothing is written past a
// No. A plan with no gate raises nothing.
func (i *InstallAPI) consentForRunner(plan *install.Plan) error {
	if err := i.askProvenanceConsent(plan.Unsigned, plan.Unscanned); err != nil {
		return err
	}
	if plan.NeedsCapabilityConfirm || plan.RequiresWidenedConfirm {
		if err := i.app.askConsent("Accept what "+plan.Namespace+"/"+plan.Name+" may do?", contractConsentText(plan)); err != nil {
			return err
		}
	}
	return nil
}

// contractConsentText is the capability contract as the consent dialog
// states it: on a first install, what the release declares; on an update,
// what widened since the installed version. The dialog is the one reading
// of the contract the person gets on a one-click install.
func contractConsentText(plan *install.Plan) string {
	var lines []string
	if plan.RequiresWidenedConfirm && plan.Diff != nil {
		lines = append(lines, "Updating from "+plan.PrevVersion+" to "+plan.Version+" widens what it may do:")
		for _, w := range plan.Diff.Widened {
			lines = append(lines, "  • "+w)
		}
	} else {
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
	}
	lines = append(lines, "", "The runner is configured to enforce this contract. Install?")
	return strings.Join(lines, "\n")
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

// Forget drops a record whose directory is gone; nothing on disk is touched
// (agentry-desktop#63).
func (i *InstallAPI) Forget(k installed.Key) error {
	if err := install.Forget(k); err != nil {
		return err
	}
	i.refreshRow(k)
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

// Uninstall removes one installation (managed dirs only; see engine docs).
func (i *InstallAPI) Uninstall(k installed.Key) (string, error) {
	msg, err := install.Uninstall(k)
	if err == nil {
		i.refreshRow(k)
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
// pipeline runs (D-12 parity by construction).
func (v *ValidateAPI) Run(dir string) (*validate.Result, error) {
	if dir == "" {
		return nil, fmt.Errorf("pick a folder to validate")
	}
	return validate.Run(dir)
}
