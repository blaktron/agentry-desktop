// Package install is the desktop install engine (§11 semantics, GUI shape):
// resolve → download → verify digest AND the DSSE attestation signature
// (§8.8) → surface the declared capability contract diffed against what this
// machine last accepted → extract → export to one §5.6 target.
//
// It is split into Prepare (everything up to and including the questions a
// human must answer) and Confirm (the writes), because a GUI shows the
// capability diff and the unsigned warning on screen and waits — the two
// consent gates the CLI implements as typed prompts arrive here as explicit
// booleans, and nothing is written to disk until they are given.
package install

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/blaktron/agentry-desktop/internal/mcpview"
	"github.com/blaktron/agentry-desktop/internal/mcpvuln"
	"github.com/blaktron/agentry-desktop/internal/pathguard"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/blaktron/agentry/pkg/apiclient"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/export"
	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/trust"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// validTargets is the §5.6 closed set plus the runner home (the CLI's
// DR-28, agentry-desktop#146): the directory a run reads, and the two
// coding-agent exports. The mcp target left with the withdrawn v1
// tool-server package kind (DEV-088).
var validTargets = []string{installdir.Target, manifest.InterfaceClaudeSkill, manifest.InterfaceAgentsMD}

// DefaultTarget is the runner target (D-031, the CLI's DR-28): the release
// laid down where a run reads it, whatever runtime.interfaces lists — those
// name the coding-agent exports, which the install wizard asks for by name.
func DefaultTarget(*manifest.Manifest) string { return installdir.Target }

// modeStrings renders a mode set for a view.
func modeStrings(modes []manifest.Mode) []string {
	out := make([]string, 0, len(modes))
	for _, m := range modes {
		out = append(out, string(m))
	}
	return out
}

// Paths is where one install target writes.
type Paths struct {
	// Dest is the directory the archive's files land in.
	Dest string `json:"dest"`
	// Managed is true when this app chose Dest from the target's default
	// layout. Only a managed directory is cleared on reinstall or removed on
	// uninstall — a directory the user picked is theirs.
	Managed bool `json:"managed"`
	// ManagedRoot is the directory a managed Dest sits directly under
	// (~/.claude/skills, or the project's agents/): the one shape a
	// managed clear may have, so a Dest derived from a bad name cannot be
	// cleared whatever it resolved to.
	ManagedRoot string `json:"managedRoot,omitempty"`
	// AgentsMD is the AGENTS.md to upsert (agents-md target only).
	AgentsMD string `json:"agentsMd,omitempty"`
	// ReleaseDir is the release's own directory under Dest (runner target
	// only): <version>-<digest12>, the layout the CLI's run reads.
	ReleaseDir string `json:"releaseDir,omitempty"`
}

// ResolvePaths computes the destination for a target.
//
// The runner target has one home (the CLI's, AGENTRY_RUNNER_HOME): the
// agent's directory under it, managed, no directory to pick — a picked
// one is refused, since the runner reads nowhere else (D-031). For the two
// coding-agent exports the desktop differs from the CLI in one deliberate
// way: the CLI's agents-md default is relative to the process working
// directory, which a GUI does not meaningfully have. Here that target
// takes a PROJECT directory the user picked, and lays out
// <project>/agents/<name> + <project>/AGENTS.md — the exact layout the CLI
// produces when run at a project root (D-002). claude-skill defaults to
// ~/.claude/skills/<name> like the CLI; a picked parent dir yields
// <dir>/<name>, unmanaged.
func ResolvePaths(target, dir, namespace, name string) (Paths, error) {
	switch target {
	case installdir.Target:
		if dir != "" {
			return Paths{}, fmt.Errorf("the runner target has one home (the runner home, AGENTRY_RUNNER_HOME); a directory is for the Claude skill and AGENTS.md exports")
		}
		home, err := policy.Home()
		if err != nil {
			return Paths{}, err
		}
		return Paths{Dest: installdir.AgentDir(home, namespace, name), Managed: true, ManagedRoot: installdir.AgentsRoot(home)}, nil

	case manifest.InterfaceClaudeSkill:
		if dir != "" {
			return Paths{Dest: filepath.Join(dir, name)}, nil
		}
		root, err := claudeSkillsRoot()
		if err != nil {
			return Paths{}, fmt.Errorf("%w (pick a directory instead)", err)
		}
		return Paths{Dest: filepath.Join(root, name), Managed: true, ManagedRoot: root}, nil

	case manifest.InterfaceAgentsMD:
		if dir == "" {
			return Paths{}, fmt.Errorf("the agents-md target needs a project directory — pick one")
		}
		root := filepath.Join(dir, "agents")
		return Paths{
			Dest: filepath.Join(root, name), Managed: true, ManagedRoot: root,
			AgentsMD: filepath.Join(dir, "AGENTS.md"),
		}, nil

	default:
		return Paths{}, fmt.Errorf("unknown target %q (want %s)", target, strings.Join(validTargets, ", "))
	}
}

// claudeSkillsRoot is where Claude Code reads skills, the Claude skill
// target's default layout (the CLI's too).
func claudeSkillsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	return filepath.Join(home, ".claude", "skills"), nil
}

// Plan is everything a human needs to see before an install writes anything.
type Plan struct {
	ID        string   `json:"id"`
	Registry  string   `json:"registry"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Version   string   `json:"version"`
	Modes     []string `json:"modes"`
	Summary   string   `json:"summary"`
	Digest    string   `json:"digest"`
	Yanked    bool     `json:"yanked"`

	Attestation trust.AttestationResult `json:"attestation"`
	// Unsigned is true when the registry offers no attestation; Confirm then
	// requires acceptUnsigned, mirroring the CLI's --allow-unsigned.
	Unsigned bool `json:"unsigned"`
	// Unscanned is true when the registry admitted the version without a
	// scan (a private organization agent its administrator took off the
	// pipeline; plan organization-subscriptions §4): no findings, no report
	// and no attestation stand behind it. Confirm then requires
	// acceptUnscanned — its own consent, which acceptUnsigned does not give
	// — mirroring the CLI's --allow-unscanned.
	Unscanned bool `json:"unscanned"`

	Capabilities installed.CapSnapshot `json:"capabilities"`
	// DeclaredSecrets is every secret the release declares, its own then each
	// declared server's, each once, and OptionalSecrets those a run can start
	// without (capabilities.optionalSecrets, agentry-desktop#655). The
	// consent names each whatever its marking: moving one between optional
	// and required asks for no new consent (plan optional-secrets.md D6),
	// which holds only while every name is shown.
	DeclaredSecrets []string `json:"declaredSecrets"`
	OptionalSecrets []string `json:"optionalSecrets"`
	// ShippedKnowledge is each knowledge pack the release ships in its
	// archive (the manifest's knowledge.ships, whose names are in
	// Capabilities.Knowledge), with its size where the archive's headers
	// gave it.
	ShippedKnowledge []ShippedPack `json:"shippedKnowledge"`
	// MCPServers is each declared MCP server's pin, tool allowlist and
	// plain env, for the review (agentry-desktop#62); never a secret.
	MCPServers []mcpview.Server `json:"mcpServers"`
	// NeedsCapabilityConfirm: first install declaring egress, exec, a
	// target host or a browser (§11; plan runtime-hosts.md §8,
	// agentry-desktop#812; plan browser-testing.md §5, agentry-desktop#878).
	NeedsCapabilityConfirm bool `json:"needsCapabilityConfirm"`
	// PrevVersion is the version currently at this key ("" on first install).
	PrevVersion string `json:"prevVersion,omitempty"`
	// Diff vs the installed contract (nil on first install).
	Diff *installed.Diff `json:"diff,omitempty"`
	// RequiresWidenedConfirm: the diff widens the contract (§11).
	RequiresWidenedConfirm bool `json:"requiresWidenedConfirm"`

	// Vulnerabilities is the registry's verdict on each declared MCP server
	// (agentry#177): VulnerabilityLine the receipt line a run would record,
	// VulnerabilityRefusal the refusal a run meets by default ("" when it
	// would run). Installing never refuses; the run panel has the override.
	Vulnerabilities      []mcpvuln.Status `json:"vulnerabilities"`
	VulnerabilityLine    string           `json:"vulnerabilityLine,omitempty"`
	VulnerabilityRefusal string           `json:"vulnerabilityRefusal,omitempty"`
	Target               string           `json:"target"`
	Paths                Paths            `json:"paths"`
	// TargetWarning is set when runtime.interfaces omits the chosen target.
	TargetWarning string `json:"targetWarning,omitempty"`
	// Secrets lists the release's own declared secret NAMES, for the mcp
	// target's env placeholders the user fills in; DeclaredSecrets above is
	// every declared name, for the consent.
	Secrets []string `json:"secrets,omitempty"`
}

// Result reports a completed install.
type Result struct {
	Message string `json:"message"`
	Dest    string `json:"dest"`
	// SecretsNote reminds about empty env placeholders (mcp target).
	SecretsNote string `json:"secretsNote,omitempty"`
	// Warning is what the install did not do although it succeeded —
	// the coding-agent export it could not write (agentry-desktop#148), or
	// the runner home's policy it could not read, so the export took the
	// generic AGENTS.md shape (agentry-desktop#220) — for a page with no
	// result screen to show beside the button.
	Warning string `json:"warning,omitempty"`
}

// pending holds the verified bytes between Prepare and Confirm.
type pending struct {
	plan     Plan
	archive  []byte
	manifest *manifest.Manifest
}

// Engine holds pending installs. One Engine lives for the app's lifetime.
type Engine struct {
	pending map[string]*pending
}

// NewEngine returns an empty engine.
func NewEngine() *Engine { return &Engine{pending: map[string]*pending{}} }

// extractLimits is the app's one bound on what it unpacks.
func extractLimits() pkgarchive.ExtractLimits { return registry.ExtractLimits() }

// Prepare runs everything up to the consent gates and parks the verified
// archive under the returned plan ID.
func (e *Engine) Prepare(ctx context.Context, regURL, ref, target, dir string) (*Plan, error) {
	if _, err := manifest.ParseRef(ref); err != nil {
		return nil, err
	}
	if target != "" && !manageable(target) {
		return nil, fmt.Errorf("unknown target %q (want %s)", target, strings.Join(validTargets, ", "))
	}
	// The registry is the page's word (D-030): judged the way the settings
	// writers judge one — https off loopback — before a token is looked up
	// for its host, so a page cannot aim the keyring's PAT at a plain-http
	// host by naming it here.
	if err := settings.ValidateURL(regURL); err != nil {
		return nil, err
	}
	regURL = settings.Normalize(regURL)
	token := settings.TokenFor(regURL)
	client, err := registry.New(regURL, token)
	if err != nil {
		return nil, err
	}

	res, err := registry.Resolve(ctx, client, ref, true)
	if err != nil {
		return nil, err
	}
	m, err := registry.ManifestFromResolve(res)
	if err != nil {
		return nil, err
	}

	archive, err := registry.Download(ctx, regURL, token, res)
	if err != nil {
		return nil, err
	}
	computed := registry.Digest(archive)
	if computed != res.Digest {
		return nil, fmt.Errorf("digest mismatch: registry says %s, archive is %s — refusing to install", res.Digest, computed)
	}

	// Provenance (§8.8): the digest above only says the bytes match what the
	// registry SAID; the attestation says who vouched for them, verified
	// against the digest just computed from the actual bytes.
	att := verifyAttestation(ctx, regURL, token, res, computed)
	switch att.State {
	case trust.StateVerified, trust.StateUnsigned, trust.StateUnscanned:
		// verified proceeds; unsigned proceeds only behind acceptUnsigned at
		// Confirm, unscanned only behind acceptUnscanned — the plan carries
		// the state either way.
	default: // mismatch, unverifiable
		hint := ""
		if att.KeyRotated {
			hint = " (the registry advertises signing key ids you have not pinned — re-pin deliberately if this rotation is expected)"
		}
		return nil, fmt.Errorf("attestation %s for %s/%s@%s: %s%s — refusing to install",
			att.State, res.Namespace, res.Name, res.Version, att.Detail, hint)
	}

	// The resolve response's manifest is not covered by the digest or the
	// attestation (both are over the archive), and the paths below are
	// derived from it: hold it to the publish rules before any path is,
	// so a name of "../../.." or an entrypoint of "../../.bashrc" is a
	// refusal here, not a directory cleared or a file written (security
	// review 2026-09-20 §8.1, §8.2).
	if blocking := manifest.Blocking(manifest.Validate(m)); len(blocking) > 0 {
		msgs := make([]string, 0, len(blocking))
		for _, e := range blocking {
			msgs = append(msgs, e.Error())
		}
		return nil, fmt.Errorf("the registry's manifest for %s/%s@%s fails validation — refusing to install:\n  %s",
			res.Namespace, res.Name, res.Version, strings.Join(msgs, "\n  "))
	}
	if m.Name != res.Name || m.Namespace != res.Namespace {
		return nil, fmt.Errorf("the registry's manifest names %s/%s but the release is %s/%s — refusing to install",
			m.Namespace, m.Name, res.Namespace, res.Name)
	}

	if target == "" {
		target = DefaultTarget(m)
	}
	paths, err := ResolvePaths(target, dir, res.Namespace, m.Name)
	if err != nil {
		return nil, err
	}
	if target == installdir.Target {
		home, herr := policy.Home()
		if herr != nil {
			return nil, herr
		}
		paths.ReleaseDir = installdir.ReleaseDir(home, res.Namespace, m.Name, res.Version, computed)
	}

	plan := Plan{
		ID:        newPlanID(),
		Registry:  regURL,
		Namespace: res.Namespace,
		Name:      res.Name,
		Version:   res.Version,
		Modes:     modeStrings(m.SupportedModes()),
		Summary:   m.Summary,
		Digest:    computed,
		Yanked:    res.Yanked,

		Attestation: att,
		Unsigned:    att.State == trust.StateUnsigned,
		Unscanned:   att.State == trust.StateUnscanned,

		Capabilities:    installed.Snapshot(m.Contract()),
		DeclaredSecrets: declaredSecrets(m),
		OptionalSecrets: optionalSecrets(m),
		MCPServers:      mcpview.Of(m.Capabilities),
		Target:          target,
		Paths:           paths,
		Secrets:         append([]string(nil), m.Capabilities.Secrets...),
	}
	// The packs the release ships, with their sizes from the verified
	// archive's headers, for the consent (plan knowledge.md §4.6): a size
	// that cannot be read leaves the name alone.
	sizes, _ := shippedPackSizes(archive, m.ShippedPacks())
	for _, name := range m.ShippedPacks() {
		sp := ShippedPack{Name: name}
		if n, ok := sizes[name]; ok {
			sp.Bytes = &n
		}
		plan.ShippedKnowledge = append(plan.ShippedKnowledge, sp)
	}
	if w := undeclaredTargetWarning(m, target); w != "" {
		plan.TargetWarning = w
	}
	vr := mcpvuln.Check(ctx, regURL, m.Capabilities.MCPServers)
	plan.Vulnerabilities, plan.VulnerabilityLine, plan.VulnerabilityRefusal = vr.Statuses, vr.Line, vr.Refusal

	readRecord(&plan, m)

	e.pending[plan.ID] = &pending{plan: plan, archive: archive, manifest: m}
	return &plan, nil
}

// readRecord fills what the plan owes to the record already at its key:
// the installed version and the capability diff against it, or on a first
// install the consent a declared egress or exec owes.
func readRecord(plan *Plan, m *manifest.Manifest) {
	state, serr := installed.Load()
	if serr != nil {
		// Bookkeeping trouble must not block an install; it costs a prompt.
		state = installed.State{}
	}
	if prev := state.Find(recordKey(*plan)); prev != nil {
		plan.PrevVersion = prev.Version
		d := installed.DiffCapabilities(prev.Capabilities, plan.Capabilities)
		plan.Diff = &d
		plan.RequiresWidenedConfirm = d.HasWidened()
	} else {
		plan.NeedsCapabilityConfirm = needsCapabilityConfirm(m.Capabilities)
	}
}

// needsCapabilityConfirm reports whether a first install of c asks for
// the capability consent: declared network egress or exec (§11), a
// target host, a machine the person names and the agent works as the
// account they sign in with (the CLI's install gate, agentry-cli#861), or
// a browser, which opens the sites the person names (agentry-cli#915).
func needsCapabilityConfirm(c manifest.Capabilities) bool {
	return len(c.Network.Egress.Hosts) > 0 || len(c.Exec.Commands) > 0 || len(c.TargetHosts) > 0 || c.Browser != nil
}

// PrepareExport derives a coding-agent export from a pending runner plan
// (agentry-desktop#148): the same resolved, downloaded and verified bytes
// — one download, one attestation verdict — laid down where the coding
// agent reads (the Claude skill, or AGENTS.md in a project directory), as
// its own plan with its own record, so a one-click install with Settings'
// "also install into <coding agent>" on writes both. The export plan owes
// the gates its own record owes: a first export of a release that
// declares egress or exec needs the capability consent even when the
// runner record already carries it, since the coding agent enforces
// nothing. The runner plan stays pending; each is confirmed on its own.
func (e *Engine) PrepareExport(runnerPlanID, target, dir string) (*Plan, error) {
	p, ok := e.pending[runnerPlanID]
	if !ok || p.manifest == nil {
		return nil, fmt.Errorf("install plan %s not found (already confirmed or cancelled)", runnerPlanID)
	}
	if target != manifest.InterfaceClaudeSkill && target != manifest.InterfaceAgentsMD {
		return nil, fmt.Errorf("unknown coding-agent export %q (want %s or %s)", target, manifest.InterfaceClaudeSkill, manifest.InterfaceAgentsMD)
	}
	paths, err := ResolvePaths(target, dir, p.plan.Namespace, p.plan.Name)
	if err != nil {
		return nil, err
	}
	plan := p.plan
	plan.ID = newPlanID()
	plan.Target = target
	plan.Paths = paths
	plan.PrevVersion, plan.Diff, plan.RequiresWidenedConfirm, plan.NeedsCapabilityConfirm = "", nil, false, false
	plan.TargetWarning = undeclaredTargetWarning(p.manifest, target)
	readRecord(&plan, p.manifest)
	e.pending[plan.ID] = &pending{plan: plan, archive: p.archive, manifest: p.manifest}
	return &plan, nil
}

// verifyAttestation resolves the attestation URL (falling back to the
// canonical path when the resolve result omits it) and verifies — unless
// the registry admitted the version without a scan, which is its own
// outcome, read before any attestation is looked for: the absence is the
// fact, not a missing signature.
func verifyAttestation(ctx context.Context, regURL, token string, res *apiclient.ResolveResult, computed string) trust.AttestationResult {
	if registry.Unscanned(res) {
		return trust.Unscanned()
	}
	attURL := ""
	if res.AttestationUrl != nil {
		attURL = *res.AttestationUrl
	}
	if attURL == "" {
		attURL = fmt.Sprintf("/api/v1/agents/%s/%s/versions/%s/attestation", res.Namespace, res.Name, res.Version)
	}
	return trust.VerifyAttestation(ctx, registry.HTTPClient, regURL, token, attURL, computed)
}

// Confirm applies the consent gates and writes the install. The booleans
// are explicit decisions the UI collected from the user; nothing defaults
// on, and none stands in for another: an unscanned release is not an
// unsigned one, and accepting unsigned says nothing about it.
func (e *Engine) Confirm(id string, acceptUnsigned, acceptCapabilities, acceptUnscanned bool) (*Result, error) {
	p, ok := e.pending[id]
	if !ok {
		return nil, fmt.Errorf("install plan %s not found (already confirmed or cancelled)", id)
	}
	plan := p.plan

	if plan.Unscanned && !acceptUnscanned {
		return nil, fmt.Errorf("unscanned — the registry admitted %s/%s@%s without a scan: no findings, no report and no attestation stand behind it; installing requires explicitly accepting an unscanned release",
			plan.Namespace, plan.Name, plan.Version)
	}
	if plan.Unsigned && !acceptUnsigned {
		return nil, fmt.Errorf("unsigned — the registry offers no attestation for %s/%s@%s; installing requires explicitly accepting an unsigned release",
			plan.Namespace, plan.Name, plan.Version)
	}
	if (plan.NeedsCapabilityConfirm || plan.RequiresWidenedConfirm) && !acceptCapabilities {
		return nil, fmt.Errorf("the declared capability contract requires explicit acceptance before installing")
	}

	// Past the gates the plan is spent whatever happens next: a failure
	// while writing must not leave the archive's bytes pending under an id
	// the page could confirm again.
	defer delete(e.pending, id)
	staging, err := os.MkdirTemp("", "agentry-install-*")
	if err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	// The hardened extractor: the same one the pipeline runs on submission,
	// so an archive that survived the server's unpack is unpacked here by the
	// same rules — path escapes, symlinks, device nodes, file-count and
	// compression-ratio bombs all refused rather than written.
	if err := pkgarchive.Extract(bytes.NewReader(p.archive), staging, int64(len(p.archive)), extractLimits()); err != nil {
		return nil, fmt.Errorf("extract archive: %w", err)
	}

	// The archive is the verified bytes; when it carries its manifest, that
	// is the one the export reads (a v1 release may ship without one, and
	// then the resolve manifest validated at Prepare stands).
	m, err := manifestFromArchive(staging, p.manifest, plan)
	if err != nil {
		return nil, err
	}
	// Held before anything is written; the record then carries the
	// archive's contract, never wider, possibly narrower (agentry-desktop#163).
	if err := holdToConsented(p.manifest, m, plan); err != nil {
		return nil, err
	}
	plan.Capabilities = installed.Snapshot(m.Contract())
	plan.DeclaredSecrets, plan.OptionalSecrets = declaredSecrets(m), optionalSecrets(m)

	res := &Result{Dest: plan.Paths.Dest}
	// after runs once the record is saved: the runner target's replace of
	// the older version (D9) waits for the record, so a failed save never
	// leaves a directory gone and a record naming it.
	var after func() error
	switch plan.Target {
	case installdir.Target:
		var style string
		style, res.Warning, err = installRunner(staging, plan.Paths, m, plan)
		res.Dest = plan.Paths.ReleaseDir
		res.Message = fmt.Sprintf("installed %s/%s@%s into the runner home: %s (export: %s)", plan.Namespace, plan.Name, plan.Version, plan.Paths.ReleaseDir, style)
		if res.Warning != "" {
			res.Message += " — warning: " + res.Warning
		}
		after = func() error { return installdir.RemoveOtherVersions(plan.Paths.Dest, plan.Paths.ReleaseDir) }
	case manifest.InterfaceClaudeSkill:
		err = exportClaudeSkill(staging, plan.Paths, m)
		res.Message = fmt.Sprintf("installed %s/%s@%s as Claude skill: %s", plan.Namespace, plan.Name, plan.Version, plan.Paths.Dest)
	case manifest.InterfaceAgentsMD:
		err = exportAgentsMD(staging, plan.Paths, m, plan.Namespace, plan.Version)
		res.Message = fmt.Sprintf("installed %s/%s@%s as prompt agent: %s (section updated in %s)",
			plan.Namespace, plan.Name, plan.Version, plan.Paths.Dest, plan.Paths.AgentsMD)
	default:
		err = fmt.Errorf("unknown target %q", plan.Target)
	}
	if err != nil {
		return nil, err
	}

	// Record only after the files actually landed: a record claiming an
	// install that failed would suppress the next run's capability prompt.
	state, serr := installed.Load()
	if serr != nil {
		state = installed.State{}
	}
	state.Upsert(recordKey(plan), plan.Version, plan.Capabilities, plan.Paths.Managed)
	recorded := true
	if err := installed.Save(state); err != nil {
		recorded = false
		res.Message += fmt.Sprintf(" — warning: %v (the next install of this agent will re-ask about capabilities)", err)
	}
	if after != nil && recorded {
		// The release and its record landed; an older version's directory
		// that would not go is said, not an error that reads as a failed
		// install.
		if err := after(); err != nil {
			res.Message += fmt.Sprintf(" — warning: %v", err)
		}
	}
	return res, nil
}

// Plan returns a pending plan by id: what a caller that did not prepare it
// (the bound Confirm) needs to know to raise the gates the plan owes.
func (e *Engine) Plan(id string) (*Plan, bool) {
	if e == nil {
		return nil, false
	}
	p, ok := e.pending[id]
	if !ok {
		return nil, false
	}
	plan := p.plan
	return &plan, true
}

// AddPendingForTest puts a plan into the engine as if Prepare had made it,
// with no archive behind it, for a test of what the bound surface does
// with a plan before Confirm writes.
func (e *Engine) AddPendingForTest(plan Plan) { e.pending[plan.ID] = &pending{plan: plan} }

// recordKey is the installed.yaml key a plan's install lives under: the
// run's own consent key for the runner target (registry `runner`, the
// agent's directory — so a consent given here covers a run, and there is
// one record per QuickAgent), the registry and the destination for an
// export.
func recordKey(plan Plan) installed.Key {
	if plan.Target == installdir.Target {
		return installed.Key{
			Registry: installdir.ConsentRegistry, Namespace: plan.Namespace, Name: plan.Name,
			Target: installdir.Target, Dir: plan.Paths.Dest,
		}
	}
	return installed.Key{
		Registry: plan.Registry, Namespace: plan.Namespace, Name: plan.Name,
		Target: plan.Target, Dir: plan.Paths.Dest,
	}
}

// installRunner lays the verified release down in the runner home with
// the CLI's installdir.Install (agentry-cli#192), so a run finds it by
// digest and fetches nothing, and writes the coding-agent shape beside it
// in the style the runner home's policy implies (installdir.StyleForHome,
// whose warning for a policy it cannot read is returned for the page). The
// cache holds the verdict and never the consent, and says whether the
// verdict was a verified signature the way a run's does. Returns the style
// written and the warning.
func installRunner(staging string, p Paths, m *manifest.Manifest, plan Plan) (style, warning string, err error) {
	// The runner home Prepare laid the paths out under (ManagedRoot is its
	// agents/), so the style is read from the home the release goes to.
	home := filepath.Dir(p.ManagedRoot)
	meta := &installdir.Meta{
		Ref:       fmt.Sprintf("%s/%s@%s", plan.Namespace, plan.Name, plan.Version),
		Namespace: plan.Namespace, Name: plan.Name, Version: plan.Version,
		Digest: plan.Digest, Source: "registry",
		Attestation: receipt.Attestation{
			State: string(plan.Attestation.State), KeyID: plan.Attestation.KeyID,
			Verified: plan.Attestation.State == trust.StateVerified,
		},
	}
	style, warning = installdir.StyleForHome(home)
	if err := installdir.Install(p.ReleaseDir, staging, m, meta, style); err != nil {
		return "", "", err
	}
	return style, warning, nil
}

// Cancel discards a pending plan.
func (e *Engine) Cancel(id string) { delete(e.pending, id) }

// prepareDest clears a managed destination so files an older version shipped
// cannot survive an upgrade as orphans. A user-picked destination is left
// alone and merged into.
func prepareDest(p Paths) error {
	if !p.Managed {
		return nil
	}
	// A managed destination is one directory directly under its managed
	// root and nothing else: never the root, never the home directory or
	// an ancestor of it (the refusal uninstall applies), never a directory
	// a relative or traversing name resolved to.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home directory: %w", err)
	}
	if err := pathguard.RefuseDangerousDir(p.Dest, home); err != nil {
		return fmt.Errorf("refusing to clear %s: %v", p.Dest, err)
	}
	clean := filepath.Clean(p.Dest)
	if p.ManagedRoot == "" || !filepath.IsAbs(p.ManagedRoot) || filepath.Dir(clean) != filepath.Clean(p.ManagedRoot) ||
		filepath.Base(clean) == "." || filepath.Base(clean) == ".." {
		return fmt.Errorf("refusing to clear %s: a managed install lives directly under its managed root (%q)", p.Dest, p.ManagedRoot)
	}
	if err := os.RemoveAll(clean); err != nil {
		return fmt.Errorf("clear %s: %w", p.Dest, err)
	}
	return nil
}

// manifestFromArchive reads agent.yaml out of the extracted archive when it
// is there, validates it, and requires it to name the release the plan
// was for; absent (a legacy v1 release), the resolve manifest the plan was
// prepared from is returned.
func manifestFromArchive(staging string, fromResolve *manifest.Manifest, plan Plan) (*manifest.Manifest, error) {
	b, err := os.ReadFile(filepath.Join(staging, "agent.yaml")) //nolint:gosec // the extracted archive's root
	if errors.Is(err, os.ErrNotExist) {
		return fromResolve, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read the archive's agent.yaml: %w", err)
	}
	m, err := manifest.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("the archive's agent.yaml does not parse — refusing to install: %w", err)
	}
	if blocking := manifest.Blocking(manifest.Validate(m)); len(blocking) > 0 {
		return nil, fmt.Errorf("the archive's agent.yaml fails validation — refusing to install: %v", blocking[0])
	}
	if m.Name != plan.Name || m.Namespace != plan.Namespace {
		return nil, fmt.Errorf("the archive's manifest names %s/%s but the release is %s/%s — refusing to install",
			m.Namespace, m.Name, plan.Namespace, plan.Name)
	}
	return m, nil
}

// holdToConsented refuses an archive manifest whose capability contract is
// wider than the resolve manifest's, the one the consent dialog and the
// diff were raised on (agentry-desktop#163). The digest and the
// attestation cover the archive, so a widening means the registry's
// resolve answer diverged from the bytes it served: the case an install
// client refuses rather than reconciles. The rule is the CLI's
// installed.ArchiveWidenings, the one both clients refuse by
// (agentry-cli#252). A legacy archive without agent.yaml hands back the
// resolve manifest itself, which has nothing to be held to.
func holdToConsented(consented, archive *manifest.Manifest, plan Plan) error {
	if consented == nil || archive == nil || consented == archive {
		return nil
	}
	widened := installed.ArchiveWidenings(consented.Contract(), archive.Contract())
	if len(widened) == 0 {
		return nil
	}
	return fmt.Errorf("the archive of %s/%s@%s declares more than the registry said it would — refusing to install: its agent.yaml %s",
		plan.Namespace, plan.Name, plan.Version, strings.Join(widened, "; "))
}

// exportClaudeSkill lays the release down as the Claude skill with the
// CLI's writer, so the two clients' installs of one release are the same
// bytes (§5.6; agentry-desktop#162). prepareDest — the managed clear and
// its guards — runs first.
func exportClaudeSkill(staging string, p Paths, m *manifest.Manifest) error {
	if err := prepareDest(p); err != nil {
		return err
	}
	return export.WriteClaudeSkillTree(staging, p.Dest, m)
}

// exportAgentsMD extracts to Dest and upserts the agent's section in
// AgentsMD (§5.6 agents-md row).
func exportAgentsMD(staging string, p Paths, m *manifest.Manifest, ns, version string) error {
	if err := prepareDest(p); err != nil {
		return err
	}
	if err := export.CopyTree(staging, p.Dest); err != nil {
		return err
	}
	existing, err := os.ReadFile(p.AgentsMD) //nolint:gosec // fixed doc name under a user-chosen dir
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", p.AgentsMD, err)
	}
	// The section names the extraction path relative to the project root, the
	// same rendering the CLI produces from its working directory.
	rel, rerr := filepath.Rel(filepath.Dir(p.AgentsMD), p.Dest)
	if rerr != nil {
		rel = p.Dest
	}
	updated := export.UpsertAgentsMDSection(existing, ns, m.Name, version, m.Summary, filepath.ToSlash(rel))
	if err := os.WriteFile(p.AgentsMD, updated, 0o644); err != nil { //nolint:gosec // project doc, not a secret
		return fmt.Errorf("write %s: %w", p.AgentsMD, err)
	}
	return nil
}

// undeclaredTargetWarning warns when runtime.interfaces exists but omits the
// chosen target (§5.2: interfaces lists what §5.6 may emit).
func undeclaredTargetWarning(m *manifest.Manifest, target string) string {
	if len(m.Runtime.Interfaces) == 0 || target == installdir.Target {
		// The runner target is not an interface a release declares.
		return ""
	}
	for _, it := range m.Runtime.Interfaces {
		if it == target {
			return ""
		}
	}
	return fmt.Sprintf("manifest runtime.interfaces %v does not list target %q", m.Runtime.Interfaces, target)
}

// newPlanID returns a random 16-hex-char id.
func newPlanID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is unrecoverable weirdness; a constant id only
		// risks colliding with another pending plan in this process.
		return "plan-fallback"
	}
	return hex.EncodeToString(b[:])
}

// Uninstall removes an installation. A managed directory is deleted and its
// target's config entry de-merged; a user-picked directory is left in place
// (the record is dropped and the message says so) — no agentry client ever
// deletes a directory the user named.
func Uninstall(k installed.Key) (string, error) {
	state, err := installed.Load()
	if err != nil {
		return "", err
	}
	rec := state.Find(k)
	if rec == nil {
		return "", fmt.Errorf("no install record for %s/%s (%s) at %s", k.Namespace, k.Name, k.Target, k.Dir)
	}
	msg := ""
	if rec.Managed {
		if rec.Target == installdir.Target {
			// The runner's record names the agent's directory under the
			// runner home and nothing else: a record edited to point
			// elsewhere — or to name a namespace or name that is a path
			// — is refused, not swept.
			home, herr := policy.Home()
			if herr != nil {
				return "", herr
			}
			want := installdir.AgentDir(home, rec.Namespace, rec.Name)
			if !manifest.ValidNamespace(rec.Namespace) || !manifest.ValidName(rec.Name) ||
				filepath.Clean(rec.Dir) != want || !pathguard.Under(installdir.AgentsRoot(home), filepath.Clean(rec.Dir)) {
				return "", fmt.Errorf("refusing to remove %s: a runner install lives at %s", rec.Dir, want)
			}
		}
		if err := os.RemoveAll(rec.Dir); err != nil {
			return "", fmt.Errorf("remove %s: %w", rec.Dir, err)
		}
		msg = fmt.Sprintf("removed %s", rec.Dir)
		if note := demerge(*rec); note != "" {
			msg += "; " + note
		}
	} else {
		msg = fmt.Sprintf("record removed; files left in place at %s (directory was user-chosen)", rec.Dir)
	}
	state.Remove(k)
	if err := installed.Save(state); err != nil {
		return "", err
	}
	return msg, nil
}

// demerge best-effort removes the config entry a managed install merged:
// the .mcp.json server entry or the AGENTS.md section. The managed layout is
// <project>/agents/<name> with the config file at <project>, so the paths
// are derivable. Failures are reported as notes, never errors — the files
// are the user's project config and may have been hand-edited.
func demerge(rec installed.Record) string {
	project := filepath.Dir(filepath.Dir(rec.Dir))
	switch rec.Target {
	case manifest.InterfaceMCP:
		cfg := filepath.Join(project, ".mcp.json")
		if err := removeMCPEntry(cfg, rec.Name); err != nil {
			return fmt.Sprintf("could not remove the %q entry from %s: %v", rec.Name, cfg, err)
		}
		return fmt.Sprintf("removed the %q server entry from %s", rec.Name, cfg)
	case manifest.InterfaceAgentsMD:
		doc := filepath.Join(project, "AGENTS.md")
		if err := removeAgentsMDSection(doc, rec.Namespace, rec.Name); err != nil {
			return fmt.Sprintf("could not remove the section from %s: %v", doc, err)
		}
		return fmt.Sprintf("removed the %s/%s section from %s", rec.Namespace, rec.Name, doc)
	}
	return ""
}

// removeMCPEntry deletes one server stanza from a .mcp.json, preserving
// everything else (the inverse of export.MergeMCPConfig, same deterministic
// output shape).
func removeMCPEntry(configPath, name string) error {
	raw, err := os.ReadFile(configPath) //nolint:gosec // fixed config name under the project dir
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	root := map[string]any{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("not valid JSON (left untouched): %w", err)
	}
	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		return nil
	}
	if _, ok := servers[name]; !ok {
		return nil
	}
	delete(servers, name)
	root["mcpServers"] = servers
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, append(out, '\n'), 0o644) //nolint:gosec // client config, not a secret
}

// removeAgentsMDSection drops the "## ns/name (…)" section from AGENTS.md,
// using the same section-boundary rule as export.UpsertAgentsMDSection.
func removeAgentsMDSection(docPath, ns, name string) error {
	raw, err := os.ReadFile(docPath) //nolint:gosec // fixed doc name under the project dir
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	prefix := "## " + ns + "/" + name + " ("
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	var out []string
	changed := false
	for i := 0; i < len(lines); {
		if !strings.HasPrefix(lines[i], prefix) {
			out = append(out, lines[i])
			i++
			continue
		}
		changed = true
		j := i + 1
		for j < len(lines) && !strings.HasPrefix(lines[j], "## ") {
			j++
		}
		i = j
	}
	if !changed {
		return nil
	}
	// Collapse the doubled blank line a removed middle section leaves.
	doc := strings.Join(out, "\n")
	for strings.Contains(doc, "\n\n\n") {
		doc = strings.ReplaceAll(doc, "\n\n\n", "\n\n")
	}
	doc = strings.TrimRight(doc, "\n") + "\n"
	return os.WriteFile(docPath, []byte(doc), 0o644) //nolint:gosec // project doc, not a secret
}

// UpdateInfo is one installed record's update-check result.
type UpdateInfo struct {
	Key             installed.Key `json:"key"`
	Current         string        `json:"current"`
	Latest          string        `json:"latest,omitempty"`
	UpdateAvailable bool          `json:"updateAvailable"`
	Error           string        `json:"error,omitempty"`
	// Manageable and Present are the record's (Item): an update is offered
	// only for a record this app manages whose directory is still there,
	// so only those count toward the badge.
	Manageable bool `json:"manageable"`
	Present    bool `json:"present"`
}

// sortRecords orders records for stable display.
func sortRecords(recs []installed.Record) {
	sort.SliceStable(recs, func(i, j int) bool {
		a, b := recs[i], recs[j]
		if a.Namespace != b.Namespace {
			return a.Namespace < b.Namespace
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Dir < b.Dir
	})
}

// Item is one install record with the facts the Installed page sorts on
// (agentry-desktop#63): whether its directory is still there, whether this
// app can uninstall it, and which registry to look it up at. The state
// file is shared with the CLI; since D-031 a runner record — written by
// an install here or by `agentry run`'s consent — is one this app manages.
type Item struct {
	installed.Record
	// Origin is the registry this record's QuickAgent is looked up at: the
	// record's own registry for an export, the app's default registry for
	// a runner record, whose key names none (a run resolves through the
	// configured registry; D-031).
	Origin string `json:"origin"`
	// Present is true when Dir exists and is a directory.
	Present bool `json:"present"`
	// Local is true for a QuickAgent installed from a folder on this
	// computer (the CLI's DR-52; agentry-desktop#425): unsigned, held by no
	// registry, so it has no Origin and no update to look for.
	Local bool `json:"local"`
	// Manageable is true for a target this app installs and uninstalls:
	// the runner home and the two exports. (A run's consent record shares
	// the runner target's key since D-031, so it is the install's record.)
	Manageable bool `json:"manageable"`
}

// manageable reports whether target is one this app installs and
// uninstalls: the runner home (D-031) and the §5.6 exports.
func manageable(target string) bool {
	for _, t := range validTargets {
		if t == target {
			return true
		}
	}
	return false
}

// List returns the shared install state, sorted for display, each record
// with its Present and Manageable facts.
func List() ([]Item, error) {
	st, err := installed.Load()
	if err != nil {
		return nil, err
	}
	sortRecords(st.Records)
	items := make([]Item, 0, len(st.Records))
	for _, rec := range st.Records {
		info, statErr := os.Stat(rec.Dir)
		it := Item{
			Record:     rec,
			Present:    statErr == nil && info.IsDir(),
			Manageable: manageable(rec.Target),
			Local:      rec.Local(),
		}
		if !it.Local {
			it.Origin = originFor(rec)
		}
		items = append(items, it)
	}
	return items, nil
}

// originFor is the registry a record's QuickAgent is looked up at: the
// record's own for an export; for a runner record, whose key names none,
// the app's default registry — the one a run resolves through.
func originFor(rec installed.Record) string {
	if rec.Registry != installdir.ConsentRegistry {
		return rec.Registry
	}
	if st, err := settings.Load(); err == nil && st.DefaultRegistry != "" {
		return settings.Normalize(st.DefaultRegistry)
	}
	return settings.Normalize(settings.DefaultRegistryURL)
}

// Forget drops an install record and touches nothing on disk. It is for a
// record whose directory is gone (a project deleted, a runner workspace
// cleaned up): there is nothing left to uninstall, only a line to remove
// from the state file. A record whose directory still exists is refused —
// that is an uninstall, and this must never become a way to leave files
// behind unrecorded.
func Forget(k installed.Key) error {
	state, err := installed.Load()
	if err != nil {
		return err
	}
	rec := state.Find(k)
	if rec == nil {
		return fmt.Errorf("no install record for %s/%s (%s) at %s", k.Namespace, k.Name, k.Target, k.Dir)
	}
	if info, statErr := os.Stat(rec.Dir); statErr == nil && info.IsDir() {
		return fmt.Errorf("%s still exists; uninstall it instead of forgetting the record", rec.Dir)
	}
	state.Remove(k)
	return installed.Save(state)
}

// declaredSecrets is every secret the release declares, its own then each
// declared server's, each once, in the manifest's order.
func declaredSecrets(m *manifest.Manifest) []string {
	out := []string{}
	for _, n := range append(slices.Clone(m.Capabilities.Secrets), serverSecrets(m)...) {
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

func serverSecrets(m *manifest.Manifest) []string {
	var out []string
	for _, srv := range m.Capabilities.MCPServers {
		out = append(out, srv.Secrets...)
	}
	return out
}

// optionalSecrets is the declared secrets a run can start without.
func optionalSecrets(m *manifest.Manifest) []string {
	out := []string{}
	for _, n := range declaredSecrets(m) {
		if m.Capabilities.IsOptionalSecret(n) {
			out = append(out, n)
		}
	}
	return out
}
