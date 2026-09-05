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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/blaktron/agentry/pkg/apiclient"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/export"
	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/trust"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// validTargets is the §5.6 closed set.
// validTargets is the §5.6 closed set; the mcp target left with the withdrawn
// v1 tool-server package kind (DEV-088).
var validTargets = []string{manifest.InterfaceClaudeSkill, manifest.InterfaceAgentsMD}

// DefaultTarget picks the §5.6 install target from runtime.interfaces (the
// first one the desktop can emit; claude-skill when nothing usable is
// listed). Interfaces are independent of run mode.
func DefaultTarget(m *manifest.Manifest) string {
	for _, it := range m.Runtime.Interfaces {
		for _, t := range validTargets {
			if it == t {
				return it
			}
		}
	}
	return manifest.InterfaceClaudeSkill
}

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
	// AgentsMD is the AGENTS.md to upsert (agents-md target only).
	AgentsMD string `json:"agentsMd,omitempty"`
}

// ResolvePaths computes the destination for a target.
//
// The desktop differs from the CLI in one deliberate way: the CLI's mcp and
// agents-md defaults are relative to the process working directory, which a
// GUI does not meaningfully have. Here those targets take a PROJECT directory
// the user picked, and lay out <project>/agents/<name> + <project>/.mcp.json
// or <project>/AGENTS.md — the exact layout the CLI produces when run at a
// project root. claude-skill defaults to ~/.claude/skills/<name> like the
// CLI; a picked parent dir yields <dir>/<name>, unmanaged.
func ResolvePaths(target, dir, name string) (Paths, error) {
	switch target {
	case manifest.InterfaceClaudeSkill:
		if dir != "" {
			return Paths{Dest: filepath.Join(dir, name)}, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("locate home directory (pick a directory instead): %w", err)
		}
		return Paths{Dest: filepath.Join(home, ".claude", "skills", name), Managed: true}, nil

	case manifest.InterfaceAgentsMD:
		if dir == "" {
			return Paths{}, fmt.Errorf("the agents-md target needs a project directory — pick one")
		}
		return Paths{
			Dest: filepath.Join(dir, "agents", name), Managed: true,
			AgentsMD: filepath.Join(dir, "AGENTS.md"),
		}, nil

	default:
		return Paths{}, fmt.Errorf("unknown target %q (want %s)", target, strings.Join(validTargets, ", "))
	}
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

	Capabilities installed.CapSnapshot `json:"capabilities"`
	// NeedsCapabilityConfirm: first install declaring egress or exec (§11).
	NeedsCapabilityConfirm bool `json:"needsCapabilityConfirm"`
	// PrevVersion is the version currently at this key ("" on first install).
	PrevVersion string `json:"prevVersion,omitempty"`
	// Diff vs the installed contract (nil on first install).
	Diff *installed.Diff `json:"diff,omitempty"`
	// RequiresWidenedConfirm: the diff widens the contract (§11).
	RequiresWidenedConfirm bool `json:"requiresWidenedConfirm"`

	Target string `json:"target"`
	Paths  Paths  `json:"paths"`
	// TargetWarning is set when runtime.interfaces omits the chosen target.
	TargetWarning string `json:"targetWarning,omitempty"`
	// Secrets lists declared secret NAMES the user must fill in (mcp env).
	Secrets []string `json:"secrets,omitempty"`
}

// Result reports a completed install.
type Result struct {
	Message string `json:"message"`
	Dest    string `json:"dest"`
	// SecretsNote reminds about empty env placeholders (mcp target).
	SecretsNote string `json:"secretsNote,omitempty"`
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

// extractLimits bounds what the app unpacks, mirroring the CLI: looser than
// the server's §19 publish quotas (which the archive already passed) because
// the client's job is only to refuse a bomb.
func extractLimits() pkgarchive.ExtractLimits {
	return pkgarchive.ExtractLimits{
		MaxFileBytes:  registry.ExtractMaxFileBytes,
		MaxTotalBytes: registry.ExtractMaxTotalBytes,
		MaxFiles:      20000,
		MaxRatio:      500,
	}
}

// Prepare runs everything up to the consent gates and parks the verified
// archive under the returned plan ID.
func (e *Engine) Prepare(ctx context.Context, regURL, ref, target, dir string) (*Plan, error) {
	if _, err := manifest.ParseRef(ref); err != nil {
		return nil, err
	}
	if target != "" {
		ok := false
		for _, t := range validTargets {
			if t == target {
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("unknown target %q (want %s)", target, strings.Join(validTargets, ", "))
		}
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
	case trust.StateVerified, trust.StateUnsigned:
		// verified proceeds; unsigned proceeds only behind acceptUnsigned at
		// Confirm — the plan carries the state either way.
	default: // mismatch, unverifiable
		hint := ""
		if att.KeyRotated {
			hint = " (the registry advertises signing key ids you have not pinned — re-pin deliberately if this rotation is expected)"
		}
		return nil, fmt.Errorf("attestation %s for %s/%s@%s: %s%s — refusing to install",
			att.State, res.Namespace, res.Name, res.Version, att.Detail, hint)
	}

	if target == "" {
		target = DefaultTarget(m)
	}
	paths, err := ResolvePaths(target, dir, m.Name)
	if err != nil {
		return nil, err
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

		Capabilities: installed.Snapshot(m.Capabilities),
		Target:       target,
		Paths:        paths,
		Secrets:      append([]string(nil), m.Capabilities.Secrets...),
	}
	if w := undeclaredTargetWarning(m, target); w != "" {
		plan.TargetWarning = w
	}

	key := installed.Key{
		Registry: regURL, Namespace: res.Namespace, Name: m.Name,
		Target: target, Dir: paths.Dest,
	}
	state, serr := installed.Load()
	if serr != nil {
		// Bookkeeping trouble must not block an install; it costs a prompt.
		state = installed.State{}
	}
	if prev := state.Find(key); prev != nil {
		plan.PrevVersion = prev.Version
		d := installed.DiffCapabilities(prev.Capabilities, plan.Capabilities)
		plan.Diff = &d
		plan.RequiresWidenedConfirm = d.HasWidened()
	} else {
		plan.NeedsCapabilityConfirm = len(m.Capabilities.Network.Egress.Hosts) > 0 || len(m.Capabilities.Exec.Commands) > 0
	}

	e.pending[plan.ID] = &pending{plan: plan, archive: archive, manifest: m}
	return &plan, nil
}

// verifyAttestation resolves the attestation URL (falling back to the
// canonical path when the resolve result omits it) and verifies.
func verifyAttestation(ctx context.Context, regURL, token string, res *apiclient.ResolveResult, computed string) trust.AttestationResult {
	attURL := ""
	if res.AttestationUrl != nil {
		attURL = *res.AttestationUrl
	}
	if attURL == "" {
		attURL = fmt.Sprintf("/api/v1/agents/%s/%s/versions/%s/attestation", res.Namespace, res.Name, res.Version)
	}
	return trust.VerifyAttestation(ctx, registry.HTTPClient, regURL, token, attURL, computed)
}

// Confirm applies the consent gates and writes the install. Both booleans
// are explicit decisions the UI collected from the user; nothing defaults on.
func (e *Engine) Confirm(id string, acceptUnsigned, acceptCapabilities bool) (*Result, error) {
	p, ok := e.pending[id]
	if !ok {
		return nil, fmt.Errorf("install plan %s not found (already confirmed or cancelled)", id)
	}
	plan := p.plan

	if plan.Unsigned && !acceptUnsigned {
		return nil, fmt.Errorf("unsigned — the registry offers no attestation for %s/%s@%s; installing requires explicitly accepting an unsigned release",
			plan.Namespace, plan.Name, plan.Version)
	}
	if (plan.NeedsCapabilityConfirm || plan.RequiresWidenedConfirm) && !acceptCapabilities {
		return nil, fmt.Errorf("the declared capability contract requires explicit acceptance before installing")
	}

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

	res := &Result{Dest: plan.Paths.Dest}
	switch plan.Target {
	case manifest.InterfaceClaudeSkill:
		err = exportClaudeSkill(staging, plan.Paths, p.manifest)
		res.Message = fmt.Sprintf("installed %s/%s@%s as Claude skill: %s", plan.Namespace, plan.Name, plan.Version, plan.Paths.Dest)
	case manifest.InterfaceAgentsMD:
		err = exportAgentsMD(staging, plan.Paths, p.manifest, plan.Namespace, plan.Version)
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
	key := installed.Key{
		Registry: plan.Registry, Namespace: plan.Namespace, Name: plan.Name,
		Target: plan.Target, Dir: plan.Paths.Dest,
	}
	state, serr := installed.Load()
	if serr != nil {
		state = installed.State{}
	}
	state.Upsert(key, plan.Version, plan.Capabilities, plan.Paths.Managed)
	if err := installed.Save(state); err != nil {
		res.Message += fmt.Sprintf(" — warning: %v (the next install of this agent will re-ask about capabilities)", err)
	}

	delete(e.pending, id)
	return res, nil
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
	if err := os.RemoveAll(p.Dest); err != nil {
		return fmt.Errorf("clear %s: %w", p.Dest, err)
	}
	return nil
}

// exportClaudeSkill writes the archive into Dest, guaranteeing SKILL.md
// frontmatter (§5.6 claude-skill row).
func exportClaudeSkill(staging string, p Paths, m *manifest.Manifest) error {
	if err := prepareDest(p); err != nil {
		return err
	}
	if err := copyTree(staging, p.Dest); err != nil {
		return err
	}
	skillRel := m.Entrypoints.Skill
	if skillRel == "" {
		skillRel = "SKILL.md"
	}
	skillPath := filepath.Join(p.Dest, filepath.FromSlash(skillRel))
	content, err := os.ReadFile(skillPath) //nolint:gosec // path derives from the verified archive
	if err != nil {
		return fmt.Errorf("read skill entrypoint %s: %w", skillRel, err)
	}
	withFM := export.EnsureSkillFrontmatter(content, m.Name, m.Summary)
	if err := os.WriteFile(skillPath, withFM, 0o644); err != nil { //nolint:gosec // skill files are world-readable content
		return fmt.Errorf("write skill entrypoint: %w", err)
	}
	return nil
}

// exportAgentsMD extracts to Dest and upserts the agent's section in
// AgentsMD (§5.6 agents-md row).
func exportAgentsMD(staging string, p Paths, m *manifest.Manifest, ns, version string) error {
	if err := prepareDest(p); err != nil {
		return err
	}
	if err := copyTree(staging, p.Dest); err != nil {
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
	if len(m.Runtime.Interfaces) == 0 {
		return ""
	}
	for _, it := range m.Runtime.Interfaces {
		if it == target {
			return ""
		}
	}
	return fmt.Sprintf("manifest runtime.interfaces %v does not list target %q", m.Runtime.Interfaces, target)
}

// copyTree copies the extracted archive into dest, preserving each file's
// permission bits (the archive normalizes modes, D-15).
func copyTree(src, dest string) error {
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, rerr := os.ReadFile(p) //nolint:gosec // reading our own staging dir
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		return fmt.Errorf("copy files to %s: %w", dest, err)
	}
	return nil
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

// List returns the shared install state, sorted for display.
func List() ([]installed.Record, error) {
	st, err := installed.Load()
	if err != nil {
		return nil, err
	}
	sortRecords(st.Records)
	return st.Records, nil
}
