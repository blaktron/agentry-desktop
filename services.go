// Bound services: the thin, JSON-shaped surface the frontend calls. Logic
// lives in internal/…; these methods map API-client types onto flat view
// models (plain strings and ints) so the generated TypeScript bindings stay
// faithful to what actually crosses the JSON boundary.
package main

import (
	"fmt"
	"time"

	"github.com/blaktron/agentry/pkg/apiclient"

	"github.com/blaktron/agentry-cli/pkg/installed"
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
}

func settingsView() (*SettingsView, error) {
	s, err := settings.Load()
	if err != nil {
		return nil, err
	}
	path, _ := settings.Path()
	v := &SettingsView{DefaultRegistry: s.DefaultRegistry, Registries: []RegistryView{}, ConfigPath: path}
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

// UserView is the signed-in identity.
type UserView struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
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
	return &UserView{Handle: u.Handle, DisplayName: u.DisplayName, Email: u.Email}, nil
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
	Kind       string `json:"kind,omitempty"`
	Capability string `json:"capability,omitempty"`
	License    string `json:"license,omitempty"`
	Sort       string `json:"sort,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
}

// SearchHitView is one search result row.
type SearchHitView struct {
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Summary     string `json:"summary"`
	Installs30d int    `json:"installs30d"`
	Quarantine  string `json:"quarantine,omitempty"`
}

// SearchView is a search response.
type SearchView struct {
	Total   int                       `json:"total"`
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
	if q.Kind != "" {
		k := apiclient.AgentKind(q.Kind)
		params.Kind = &k
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
	if resp.JSON200.Total != nil {
		out.Total = *resp.JSON200.Total
	}
	if resp.JSON200.Facets != nil {
		out.Facets = *resp.JSON200.Facets
	}
	for _, hit := range resp.JSON200.Results {
		h := SearchHitView{
			Namespace: hit.Namespace, Name: hit.Name,
			Kind: string(hit.Kind), Summary: hit.Summary,
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
}

// AgentView is the agent header.
type AgentView struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Summary    string `json:"summary"`
	Visibility string `json:"visibility"`
	Quarantine string `json:"quarantine,omitempty"`
	CreatedAt  string `json:"createdAt"`
	GitHTTPURL string `json:"gitHttpUrl,omitempty"`
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
			Namespace: a.Namespace, Name: a.Name, Kind: string(a.Kind),
			Summary: a.Summary, Visibility: string(a.Visibility),
			CreatedAt: a.CreatedAt.Format(time.RFC3339), GitHTTPURL: a.GitHttpUrl,
		},
		Versions: []VersionView{},
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
		out.Versions = append(out.Versions, vv)
		if vv.Status == "published" && !vv.Yanked && out.Latest == "" {
			// ListVersions returns newest-first; the first published,
			// non-yanked row is the latest.
			out.Latest = ver.Version
			out.LatestDigest = vv.ArchiveDigest
			if ver.Capabilities != nil {
				out.LatestCapabilities = *ver.Capabilities
			}
		}
	}
	return out, nil
}

// Readme returns the API's server-sanitized README HTML.
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
	VersionStatus  string         `json:"versionStatus"`
	ScanStatus     string         `json:"scanStatus,omitempty"`
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
// against the actual downloaded bytes.
func (b *BrowseAPI) Provenance(reg, ns, name, version, declaredDigest string) (*trust.AttestationResult, error) {
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
}

// Prepare resolves, downloads, and verifies; nothing is written. The plan
// carries every consent question for the UI to ask.
func (i *InstallAPI) Prepare(reg, ref, target, dir string) (*install.Plan, error) {
	return i.engine.Prepare(i.app.ctx, reg, ref, target, dir)
}

// Confirm applies the user's answers and writes the install.
func (i *InstallAPI) Confirm(planID string, acceptUnsigned, acceptCapabilities bool) (*install.Result, error) {
	return i.engine.Confirm(planID, acceptUnsigned, acceptCapabilities)
}

// Cancel discards a pending plan.
func (i *InstallAPI) Cancel(planID string) {
	i.engine.Cancel(planID)
}

// ListInstalled returns the shared install state (~/.config/agentry/
// installed.yaml — the same file the CLI writes).
func (i *InstallAPI) ListInstalled() ([]installed.Record, error) {
	return install.List()
}

// CheckUpdates resolves the latest version for every installed record.
func (i *InstallAPI) CheckUpdates() ([]install.UpdateInfo, error) {
	return install.CheckUpdates(i.app.ctx)
}

// Uninstall removes one installation (managed dirs only; see engine docs).
func (i *InstallAPI) Uninstall(k installed.Key) (string, error) {
	return install.Uninstall(k)
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
