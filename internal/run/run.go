// Package run is the desktop run engine: the same lifecycle `agentry run`
// executes — resolve → verify → materialize → compile → enforce → run →
// receipt — driven through the public agentry-cli/pkg/runner packages,
// never a parallel implementation. The install cache, plan layout, and MCP
// profiles under ~/.agentry/runner are byte-compatible with the CLI's, so
// either client can pick up where the other left off.
package run

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/runnerimage"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/blaktron/agentry/pkg/apiclient"
	pkgarchive "github.com/blaktron/agentry/pkg/archive"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/adapter"
	"github.com/blaktron/agentry-cli/pkg/runner/compile"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/gateway"
	"github.com/blaktron/agentry-cli/pkg/runner/intake"
	"github.com/blaktron/agentry-cli/pkg/runner/localinstall"
	"github.com/blaktron/agentry-cli/pkg/runner/mcpinstall"
	"github.com/blaktron/agentry-cli/pkg/runner/modelsignin"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/profile"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/pkg/runner/unsafeexec"
	"github.com/blaktron/agentry-cli/pkg/trust"

	"github.com/blaktron/agentry-desktop/internal/mcpvuln"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// newDockerClient builds the docker client every run/setup surface talks
// to. A package variable purely as a test seam: tests substitute
// dockerx.Fake (the CLI engine tests' technique) so the whole lifecycle
// runs without a docker daemon. Production code never reassigns it.
var newDockerClient = func() dockerx.Client { return dockerx.New() }

// Home is ~/.agentry/runner, or $AGENTRY_RUNNER_HOME — the CLI's exact
// derivation (cmd_run.go runnerHome), so both clients share one cache.
func Home() (string, error) { return policy.Home() }

// runnerImageTag derives the local runner image tag from the embedded build
// context's content version — the same value the CLI uses — so both clients
// build, probe and run one image per context. A policy `images.runner` pin
// still wins in both.
func runnerImageTag(pol *policy.Policy) string {
	return pol.RunnerImageTag(runnerimage.ContentVersion())
}

// Signature is one resolved release: the identity (ns/name@version +
// digest) the install cache and MCP profiles are keyed by.
type Signature struct {
	Registry  string
	Ref       string // canonical ns/name@version
	Namespace string
	Name      string
	Version   string
	Digest    string
	Yanked    bool
	Manifest  *manifest.Manifest
	resolve   *apiclient.ResolveResult
	// local is the release directory of a QuickAgent installed from a folder on
	// this computer, and localMeta what its integrity record vouched for: a run
	// reads it there and asks no registry.
	local     string
	localMeta *installdir.Meta
}

// Local says the signature is a local install's, never a registry's.
func (s *Signature) Local() bool { return s.local != "" }

// DigestSuffix is the 12-char digest form used in directory names.
func (s *Signature) DigestSuffix() string { return installdir.DigestSuffix(s.Digest) }

// Unscanned says whether the registry admitted the resolved version
// without a scan: read from the resolve answer, before any attestation
// is looked for, because the absence is the fact and not a missing
// signature. A registry that sends no such field never published one, so
// a nil answer is "no".
func (s *Signature) Unscanned() bool { return registry.Unscanned(s.resolve) }

// ResolveSignature resolves a ref against a registry and decodes the
// manifest the resolve result carries. installIntent adds the CLI's
// X-Agentry-Install hint (a run is an install).
func ResolveSignature(ctx context.Context, regURL, ref string, installIntent bool) (*Signature, error) {
	if _, err := manifest.ParseRef(ref); err != nil {
		return nil, err
	}
	regURL = settings.Normalize(regURL)
	client, err := registry.New(regURL, settings.TokenFor(regURL))
	if err != nil {
		return nil, err
	}
	res, err := registry.Resolve(ctx, client, ref, installIntent)
	if err != nil {
		return nil, err
	}
	m, err := registry.ManifestFromResolve(res)
	if err != nil {
		return nil, err
	}
	return &Signature{
		Registry:  regURL,
		Ref:       fmt.Sprintf("%s/%s@%s", res.Namespace, res.Name, res.Version),
		Namespace: res.Namespace,
		Name:      res.Name,
		Version:   res.Version,
		Digest:    res.Digest,
		Yanked:    res.Yanked,
		Manifest:  m,
		resolve:   res,
	}, nil
}

// localRepair is how a changed local install is laid down again from the
// desktop: its folder, through the Build tab, never a registry, which does
// not hold it.
const localRepair = "Build → Open folder… → Install"

// resolveForRun is what a run on this computer resolves a ref through: a
// local install of ns/name first (localinstall.Materialize, which holds it
// to its integrity record), then the registry. A version range or another
// channel is the registry's, as in the CLI's run. A hosted run and the
// published agent's page keep ResolveSignature: neither has anything to do
// with this machine's installs.
func resolveForRun(ctx context.Context, regURL, ref string, installIntent bool) (*Signature, error) {
	parsed, err := manifest.ParseRef(ref)
	if err != nil {
		return nil, err
	}
	if parsed.Channel == "" || parsed.Channel == "stable" {
		if home, herr := Home(); herr == nil {
			meta, dir, ok, lerr := localinstall.Materialize(home, parsed.Namespace, parsed.Name, parsed.Range)
			if ok {
				if lerr != nil {
					var changed *installdir.ChangedError
					if errors.As(lerr, &changed) {
						changed.Repair = localRepair
					}
					return nil, lerr
				}
				m, err := manifestFromWorkspace(filepath.Join(dir, "workspace"))
				if err != nil {
					return nil, err
				}
				return &Signature{
					Registry:  settings.Normalize(regURL),
					Ref:       fmt.Sprintf("%s/%s@%s", meta.Namespace, meta.Name, meta.Version),
					Namespace: meta.Namespace, Name: meta.Name, Version: meta.Version,
					Digest:   meta.Digest,
					Manifest: m,
					local:    dir, localMeta: meta,
				}, nil
			}
			// An install record that cannot be read is not a reason to
			// refuse a registry release: the registry answers, as the
			// CLI's run does.
		}
	}
	return ResolveSignature(ctx, regURL, ref, installIntent)
}

// ProfileInfo is a QuickAgent signature's MCP-profile state. Secret
// VALUES never appear here — names only.
type ProfileInfo struct {
	Ref             string
	Digest          string
	DeclaredSecrets []string
	// OptionalSecrets are those of DeclaredSecrets a run can start without
	// the keys form marks them, and a value for one is set there or left
	// empty.
	OptionalSecrets   []string
	Exists            bool
	Keep              bool
	StoredSecretNames []string
	CapabilityLines   []string                  // Intake are the release's up-front questions.
	Intake            []manifest.IntakeQuestion // Modes are the run modes the release declares; Opening its interactive
	// opening line.
	Modes   []string
	Opening string
	// Guide is the author's words for the run setup (manifest v2), shown to the
	// person and never to the agent. Nil when the release carries none.
	Guide *manifest.Guide
	// ShipsKnowledge names the knowledge packs the release ships in its
	// archive (knowledge.ships), mounted in every run beside what the
	// person attaches.
	ShipsKnowledge []string
	// AcceptsKnowledge is what the release asks the person to attach, shown
	// in the run setup's Knowledge step and never to the agent; nil when it
	// asks for nothing.
	AcceptsKnowledge *manifest.KnowledgeAccepts
	// BrowserSites are the url questions the release's browser names as sites
	// (capabilities.browser.sites): the run setup asks each for the site to
	// test, an http or https address, never as an input source to fetch. Nil
	// when it declares no browser.
	BrowserSites []string
}

// Declared reports what a release declares, from the registry's resolve
// alone: the run modes, every secret name — its own and its declared MCP
// servers' — that a run would have to supply, and its target hosts, which
// only a run on this computer reaches. The In-Agentry availability rule
// (services_hostedrun.go) reads this rather than ProfileStatus, because a
// hosted run must touch nothing on this machine: no operator policy, no
// MCP profile, no docker.
func Declared(ctx context.Context, regURL, ref string) (*Declaration, error) {
	sig, err := ResolveSignature(ctx, regURL, ref, false)
	if err != nil {
		return nil, err
	}
	d := &Declaration{Secrets: allSecretNames(sig.Manifest), Optional: optionalSecretNames(sig.Manifest), TargetHosts: TargetHostIDs(sig.Manifest)}
	for _, mode := range sig.Manifest.SupportedModes() {
		d.Modes = append(d.Modes, string(mode))
	}
	return d, nil
}

// Declaration is what Declared reads from a release's resolve.
type Declaration struct {
	// Modes are the run modes it declares.
	Modes []string
	// Secrets are every secret name, its own and its servers', and
	// Optional those a run can start without.
	Secrets, Optional []string
	// TargetHosts are its host questions' ids.
	TargetHosts []string
}

// ProfileStatus resolves a ref and reports its profile state. Keep on a
// non-existent profile is the policy default, so the form's toggle starts
// where guided setup would.
func ProfileStatus(ctx context.Context, regURL, ref string) (*ProfileInfo, error) {
	sig, err := resolveForRun(ctx, regURL, ref, false)
	if err != nil {
		return nil, err
	}
	return profileInfo(sig)
}

func profileInfo(sig *Signature) (*ProfileInfo, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	dir := profile.Dir(home, sig.Namespace, sig.Name, sig.Version, sig.DigestSuffix())
	prof, err := profile.Load(dir)
	if err != nil {
		return nil, err
	}
	info := &ProfileInfo{
		Ref:             sig.Ref,
		Digest:          sig.Digest,
		DeclaredSecrets: allSecretNames(sig.Manifest),
		OptionalSecrets: optionalSecretNames(sig.Manifest),
		Keep:            loadPolicyForRead().KeepProfilesByDefault(),
		CapabilityLines: capabilityLines(sig.Manifest.Contract()),
		Intake:          sig.Manifest.Intake,
		Guide:           sig.Manifest.Guide,
		ShipsKnowledge:  sig.Manifest.ShippedPacks(),
	}
	if b := sig.Manifest.Capabilities.Browser; b != nil {
		info.BrowserSites = b.Sites
	}
	if k := sig.Manifest.Knowledge; k != nil {
		info.AcceptsKnowledge = k.Accepts
	}
	for _, mode := range sig.Manifest.SupportedModes() {
		info.Modes = append(info.Modes, string(mode))
	}
	if sig.Manifest.Interactive != nil {
		info.Opening = sig.Manifest.Interactive.Opening
	}
	if prof != nil {
		info.Exists = true
		info.Keep = prof.Keep
		info.StoredSecretNames = prof.SecretNames()
	}
	return info, nil
}

// SaveProfile writes (or replaces) the signature's MCP profile from the
// guided form. Empty-string values mean "resolve from the environment
// at run time" and are recorded as absent, not empty; an existing
// profile's CreatedAt survives reconfiguration.
func SaveProfile(ctx context.Context, regURL, ref string, secrets map[string]string, keep bool) (*ProfileInfo, error) {
	sig, err := resolveForRun(ctx, regURL, ref, false)
	if err != nil {
		return nil, err
	}
	return saveProfileForSig(sig, secrets, keep)
}

// saveProfileForSig is SaveProfile past the resolve — separated so the
// persistence semantics are unit-testable without a registry.
func saveProfileForSig(sig *Signature, secrets map[string]string, keep bool) (*ProfileInfo, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	dir := profile.Dir(home, sig.Namespace, sig.Name, sig.Version, sig.DigestSuffix())
	existing, err := profile.Load(dir)
	if err != nil {
		return nil, err
	}
	declaredNames := allSecretNames(sig.Manifest)
	declared := map[string]bool{}
	for _, n := range declaredNames {
		declared[n] = true
	}
	values := map[string]string{}
	for name, v := range secrets {
		if v == "" {
			continue // absent = resolve from the environment at run time
		}
		if !declared[name] {
			return nil, fmt.Errorf("%q is not a secret this release declares (declared: %s)",
				name, strings.Join(declaredNames, ", "))
		}
		values[name] = v
	}
	prof := profile.New(sig.Ref, sig.Digest, keep, values)
	if existing != nil {
		prof.CreatedAt = existing.CreatedAt
	}
	if err := prof.Save(dir); err != nil {
		return nil, err
	}
	return profileInfo(sig)
}

// Request parameterizes one run.
type Request struct {
	Registry string
	Ref      string
	// RunID names the run's docker objects (label agentry.run=<id>); ""
	// mints one. The app sets it so it can Sweep the run on quit.
	RunID string
	// InFlightDir is the directory of the crash path's records, where the
	// app recorded RunID. When set, the prepared run's profile and its keep
	// decision go on the record before the engine starts (RecordProfile),
	// so a run that never completes still has its unkept profile removed.
	InFlightDir string
	// AllowVulnerableMCP is the person's consent, asked in a native dialog, to run
	// a declared MCP server whose advisory the registry's gate rates high or
	// critical. It lets the run through only in Unsafe Mode (Unsafe), and the
	// receipt names it. The policy's mcpServers.allowVulnerable is the CLI's way
	// through and lets nothing through here.
	AllowVulnerableMCP bool
	Task               string
	// InputPaths feed the read-only /inputs channel.
	InputPaths []string
	// Knowledge are the knowledge packs attached to the run, already opened by
	// the caller (the local store's attach, after the picker gates): the engine
	// copies each into the run, mounts it read-only at /knowledge/<name> on the
	// tools container only, and names it in the receipt. The caller removes its
	// own opened copies once the run returns.
	Knowledge []engine.AttachedPack
	// Voice is this computer's Writing Voice when the run writes in it: the engine
	// stages it read-only beside the packs and grants voice_read, and the receipt
	// names it; the plan and its hash do not move. nil runs without one.
	Voice *engine.AttachedVoice
	// ReportTheme is the report design export_pdf draws the run's PDFs in, and
	// what chose it: the account's design, whole, when the app is signed in to the
	// run's registry and the account chose one, else Plain. The model never
	// chooses it, and the receipt names it. The zero value is Plain, chosen by the
	// default.
	ReportTheme engine.ReportTheme
	// OutDir is where reports/receipts land ("" = ~/Downloads/agentry-out/
	// <ns>-<name>-<runid>).
	OutDir string
	// OutDirIsRoot says OutDir is a folder every run shares — Run defaults' saved
	// folder — so this run writes into <ns>-<name>-<runid> inside it: the agent's
	// output mount is read-write, and one run must not read or change what another
	// left. A folder picked for one run is used as it is.
	OutDirIsRoot bool
	// KeepProfile overrides the completion choice (nil = stored profile,
	// else the policy default).
	KeepProfile *bool
	// TimeoutSeconds overrides the policy timeout when > 0; MaxTurns
	// overrides the tool-call budget when >= 0 (0 = unlimited).
	TimeoutSeconds int
	MaxTurns       int
	// AcceptUnsigned is the explicit consent to run a release the registry
	// publishes no attestation for (the CLI's --allow-unsigned). Enforced
	// HERE, not only in the panel's checkbox, so any caller of the bound
	// method meets the same gate. Digest mismatch and unverifiable
	// attestations refuse regardless.
	AcceptUnsigned bool
	// AcceptUnscanned is the explicit consent to run a release the registry
	// admitted without a scan (the CLI's --allow-unscanned): its own consent,
	// which AcceptUnsigned does not give. Enforced here, as AcceptUnsigned is, and
	// on every run from the cache — the cache remembers the verdict, not the
	// consent.
	AcceptUnscanned bool
	// Intake answers the manifest's up-front questions by id; a required
	// question without an answer refuses the run.
	Intake map[string]string
	// SourceTokens are access tokens given in the run setup for this run, by the
	// id of an input-source question. Each goes to that source's fetch step alone;
	// a source with none uses the key saved for its host, else none.
	SourceTokens map[string]string `json:"-"`
	// HostSignIns are the sign-ins given in the run setup for its target hosts, by
	// the id of a host question. A host with none signs in with the one saved for
	// it in the system keychain, or the run is refused.
	HostSignIns map[string]HostSignIn `json:"-"`
	// TrustHostKey asks the person, in a native dialog the page cannot
	// answer, whether to accept a target host's key that neither their
	// known_hosts nor the runner home's holds; prompt is the CLI's own,
	// naming the key's type and SHA256 fingerprint. nil is no one to ask,
	// and such a key refuses the run.
	TrustHostKey func(prompt string) (bool, error)
	// KeyFilePicked judges a target host's key file: one the native picker
	// returned for that purpose, or the run is refused. The desktop always
	// passes it.
	KeyFilePicked func(what, path string) error
	// PathPicked, when set, judges every host path the request carries that the
	// caller did not already judge — a file-typed intake answer — before it is
	// staged: the desktop passes the native picker's record, since a page can
	// type a path as easily as pick one.
	PathPicked func(what, path string) error
	// OnBudgets, when set, is told the run's effective budgets once it is prepared
	// (the policy's, with TimeoutSeconds and MaxTurns applied), and OnUsage the
	// harness's usage as it arrives (the engine's own hook): what the desktop's
	// progress dialog shows. Both are called on the run's own path and must not
	// block it.
	OnBudgets func(timeoutSeconds, maxTurns int)
	OnUsage   func(receipt.Usage)
	// Unsafe is Unsafe Mode for this run: the caller read the switch from
	// desktop.yaml, which only Go writes after a native confirmation, and no bound
	// method takes it from the page. The run is compiled without this machine's
	// exec catalogue, its declared commands installed from the Unsafe Mode image.
	// False is the default posture, under which a command the catalogue does not
	// carry refuses the run as it always has.
	Unsafe bool
	// ChatIdleTimeoutSeconds is a chat's idle timeout, Run defaults' Chat idle
	// timeout from desktop.yaml, 0 for none. StartSession runs every chat with no
	// wall clock, no count of the person's turns and no run timeout, and this
	// idle timeout, whatever the runner policy's sessions block and the release's
	// own idle hint say; TimeoutSeconds does not apply to a chat. Run, which is
	// not a chat, ignores it.
	ChatIdleTimeoutSeconds int
	// PlanCompiled, when set, is called once the run holds its plan, just compiled
	// or reused, before anything of it starts, with whether the run is in Unsafe
	// Mode: its plan is compiled so, or Unsafe Mode let a declared server the gate
	// rates high through. The desktop keeps the red edge up while such a run or
	// chat is live, whatever the switch says meanwhile; false clears the edge of a
	// run the switch was on for that Unsafe Mode changed nothing of (no declared
	// commands, no such server).
	PlanCompiled func(unsafe bool)
}

// Result is the envelope-derived outcome. It is populated on failure too:
// the envelope always exists once the engine starts, and earlier failures
// are returned as errors instead.
type Result struct {
	Status           string
	ExitCode         int
	Error            string
	RunID            string
	Ref              string
	Digest           string
	ResultJSON       string
	OutDir           string
	ReceiptsDir      string
	RunPlanSHA       string
	InputTokens      int64
	OutputTokens     int64
	CostUSD          float64
	WallMS           int64
	Turns            int
	AttestationState string
	// Policy is the envelope's policy block: the operator policy file the
	// run enforced, how its path was chosen (default, env) and the digest
	// the plan was compiled against. Nil when the envelope carries none.
	Policy *receipt.Policy
	// Knowledge is the receipt's knowledge line: what was mounted and
	// how, "" when no pack was attached.
	Knowledge string
	// Exec is the receipt's exec line: the allowlist run_command enforced
	// and the image it ran in, "" when the release declares no commands.
	// Unsafe says the run's plan was compiled in Unsafe Mode, so that line
	// is the one a result shows under it.
	Exec   string
	Unsafe bool
	// Decode is the receipt's decode line: how the run's input files were
	// read, and what ClamAV and the Hybrid Analysis lookup made of them; ""
	// when the run had no files.
	Decode string
	// ExitClass names ExitCode in the CLI's words (receipt.ExitClassName);
	// Stage and Reason say where and why a failed run stopped, in the CLI's
	// closed words, nil and "" when they do not. A blocked run's reason says
	// whether the QuickAgent or the runner's tools stopped it.
	ExitClass string
	Stage     string
	Reason    *receipt.Reason
}

// Run executes the whole lifecycle for one registry release. Progress and
// diagnostics stream to logw (the Wails event channel upstairs). prepared
// is everything Run and StartSession share: the verified install, the plan
// for the mode, the images, the MCP profile and the engine parameters. One
// code path, two ways to drive the harness.
type prepared struct {
	sig     *Signature
	m       *manifest.Manifest
	plan    *spec.Plan
	docker  dockerx.Client
	prof    *profile.Profile
	profDir string
	keep    bool
	params  engine.Params
}

// prepare does the install → plan → images → profile → intake steps for
// the run mode; it is Run without the run.
func prepare(ctx context.Context, req Request, logw io.Writer, mode manifest.Mode) (*prepared, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	// The operator policy file this run obeys and how its path was chosen (the
	// runner home's, or a home relocated by AGENTRY_RUNNER_HOME): the same path is
	// read here and handed to the engine, which records it in the envelope's
	// policy block beside the plan's digest of those bytes. The desktop has no
	// --policy.
	polFile, err := policy.Resolve("")
	if err != nil {
		return nil, err
	}
	pol, polSHA, err := policy.EnsureDefault(polFile.Path)
	if err != nil {
		return nil, fmt.Errorf("operator policy: %w", err)
	}

	sig, err := resolveForRun(ctx, req.Registry, req.Ref, true)
	if err != nil {
		return nil, err
	}
	if sig.Yanked {
		_, _ = fmt.Fprintf(logw, "warning: %s is YANKED — running only because the version was pinned exactly\n", sig.Ref)
	}

	meta, installDir, err := materialize(ctx, sig, home, req.AcceptUnsigned, req.AcceptUnscanned, logw)
	if err != nil {
		return nil, err
	}
	workspaceDir := filepath.Join(installDir, "workspace")
	m, err := manifestFromWorkspace(workspaceDir)
	if err != nil {
		return nil, err
	}
	// The packs the release ships, mounted beside the person's own.
	knowledgePacks, err := withShippedKnowledge(workspaceDir, m, meta, req.Knowledge, logw)
	if err != nil {
		return nil, err
	}

	// The capability record: the app showed the declared contract on the
	// page and in the run panel, and the Run click is the acceptance the
	// CLI collects as a prompt; record it under the same key so CLI and
	// desktop runs share one consent history.
	recordCapabilities(m, meta, home, logw)

	// The harness model setup needs (adapter.DefaultFor): docker-agent for a
	// key, the Codex, Claude Code or Copilot CLI image for a model sign-in.
	adapterName := adapter.DefaultFor(pol)
	ad, err := adapter.Get(adapterName)
	if err != nil {
		return nil, err
	}

	// Declared MCP servers: resolve pins (offline when the signature's store
	// matches), compile them into the plan, install into the store with the
	// images below.
	serversDir := mcpinstall.StoreDir(profile.Dir(home, sig.Namespace, sig.Name, sig.Version, sig.DigestSuffix()))
	var resolvedServers []mcpinstall.Resolved
	var serverSpecs []spec.MCPServerSpec
	var vulnLine string
	unsafeServer := false
	if len(m.Capabilities.MCPServers) > 0 {
		// The registry's vulnerability verdict on each declared server: an advisory
		// the gate rates high or critical refuses; on this computer the way through
		// is Unsafe Mode with the person's consent for the run; an unreachable
		// registry does not stop the run; the receipt records the outcome.
		statuses := mcpinstall.CheckVulnerabilities(ctx, nil, req.Registry, m.Capabilities.MCPServers)
		_, _ = fmt.Fprintf(logw, "declared MCP servers, per the registry: %s\n", mcpinstall.Summary(statuses))
		var refuseErr error
		vulnLine, refuseErr = mcpvuln.Verdict(statuses, req.Registry, mcpvuln.WayThrough{Unsafe: req.Unsafe, Consented: req.AllowVulnerableMCP, PolicyAllows: pol.MCPServers.AllowVulnerable})
		if refuseErr != nil {
			return nil, refuseErr
		}
		// Let through in Unsafe Mode: the run is an Unsafe Mode run
		// whatever its plan's exec line says.
		unsafeServer = slices.ContainsFunc(statuses, mcpinstall.VulnStatus.Refuses)
		resolvedServers, err = ensureServerResolutions(ctx, logw, serversDir, pol, m)
		if err != nil {
			return nil, err
		}
		serverSpecs = mcpinstall.Specs(resolvedServers, m.Capabilities.MCPServers, pol.Images)
	}

	docker := newDockerClient()
	// Unsafe Mode: the base's command index and extras list, read from the image
	// the switch pulled (pulled again here if it was removed since). Nil compiles
	// the default posture.
	unsafe, err := unsafeData(ctx, docker, pol, m, req.Unsafe, logw)
	if err != nil {
		return nil, err
	}
	// Held to its record once more before it is compiled: the consents and
	// the images above take time, and a plan compiled from files edited
	// meanwhile would be kept and reused.
	if err := recheckInstall(sig, installDir); err != nil {
		return nil, err
	}
	plan, planDir, err := ensurePlan(logw, home, m, meta, installDir, workspaceDir, pol, polSHA, adapterName, serverSpecs, mode, unsafe)
	if err != nil {
		return nil, err
	}
	if planIsUnsafe(plan) {
		// The plan's own exec line, which the receipt carries: what Unsafe
		// Mode placed, what it could not, and what is not enforced.
		_, _ = fmt.Fprintln(logw, plan.Enforcement["exec"])
	}
	if req.PlanCompiled != nil {
		req.PlanCompiled(planIsUnsafe(plan) || unsafeServer)
	}
	if vulnLine != "" {
		if plan.Enforcement == nil {
			plan.Enforcement = map[string]string{}
		}
		plan.Enforcement["mcpServers.vulnerabilities"] = vulnLine
	}

	// The person's model sign-in, when the plan spends one, as `agentry
	// run` hands it (engine.ModelSignInFor), and a connected account's key
	// readied (a Hugging Face token renewed into the keychain entry the run
	// reads, a Poe key past its expiry refused), before any image is
	// prepared for a run that could not start. A desktop run on this
	// machine is never a hosted one.
	signIns := modelsignin.Open(home, connectionKeys())
	if err := signIns.ReadyForRun(ctx, plan); err != nil {
		return nil, engine.Classify(engine.ExitPolicy, err)
	}
	signIn, renewSignIn, err := engine.ModelSignInFor(ctx, signIns, plan, false)
	if err != nil {
		return nil, err
	}
	// The images the plan names, its harness's included: pulled, or for
	// Claude Code and Copilot CLI built on this machine from their pinned
	// artifacts, never one the plan does not use.
	if err := gateway.EnsureInstalledFor(ctx, docker, pol, runnerImageTag(pol), filepath.Join(home, "image"), logw, plan); err != nil {
		return nil, fmt.Errorf("gateway/docker environment: %w", err)
	}
	// The per-QuickAgent exec image the plan names when the agent declares exec:
	// built once, same as the CLI, from the spec the compiler put on the plan (the
	// catalogued packages and the mirror, not just the command list). The package
	// versions the build resolved go to the engine, which appends them to the
	// receipt's exec line exactly as the CLI's does ("packages resolved at image
	// build: …"); a plan with no exec image passes none and the line is the
	// compiled text alone.
	var execPackages []string
	if plan.ExecImage != nil {
		if execPackages, err = gateway.EnsureExecImage(ctx, docker, runnerImageTag(pol), *plan.ExecImage, filepath.Join(home, "image"), logw); err != nil {
			return nil, fmt.Errorf("exec image: %w", err)
		}
	}
	if len(resolvedServers) > 0 {
		// A run killed mid-install left its builder running under the same fixed
		// name: sweep what dead runs left before building, as `agentry run` does. The
		// engine sweeps again when the run starts; the second finds nothing.
		engine.SweepAbandoned(ctx, docker, logw)
		// pol.Docker, whole: the operator's container runtime (gVisor when set) and
		// the caps and bounds every run container gets apply to the server install
		// builds too.
		if err := mcpinstall.EnsureInstalled(ctx, docker, resolvedServers, serversDir,
			pol.MCPRegistries, pol.Images, pol.Docker, logw); err != nil {
			return nil, fmt.Errorf("mcp servers: %w", err)
		}
	}

	// The MCP profile: the desktop never prompts here — the guided form
	// (SaveProfile) runs before this, and a missing secret still refuses
	// inside the engine with the variable named (v1 exit-11 semantics,
	// nothing degrades silently).
	profDir := profile.Dir(home, sig.Namespace, sig.Name, sig.Version, sig.DigestSuffix())
	prof, err := profile.Load(profDir)
	if err != nil {
		return nil, err
	}
	keep := pol.KeepProfilesByDefault()
	if prof != nil {
		keep = prof.Keep
	}
	if req.KeepProfile != nil {
		keep = *req.KeepProfile
	}
	if prof, err = ensureKeepProfile(profDir, sig, prof, req.KeepProfile); err != nil {
		return nil, err
	}
	// The profile as this run holds it, for its crash-path record: read now,
	// as it was loaded, so a profile saved again before the engine starts is
	// the newer writer's.
	held, err := heldProfile(profDir, installDir, keep)
	if err != nil {
		finishProfile(logw, prof, profDir, keep)
		return nil, err
	}

	// The connections of the servers that sign in by OAuth: made on the run
	// setup's Connect tools step, never asked for here; each is refreshed under
	// its lock before the sandbox starts, and a missing one refuses the run by
	// name.
	conns, err := openConnections(ctx, home, meta.Ref, meta.Digest, m)
	if err != nil {
		finishProfile(logw, prof, profDir, keep)
		return nil, err
	}
	if conns.line != "" {
		if plan.Enforcement == nil {
			plan.Enforcement = map[string]string{}
		}
		plan.Enforcement["mcpServers.connections"] = conns.line
	}

	// The caller may choose the run id (the app does, so that quitting
	// mid-run can sweep the run's label); else it is minted here.
	runID := req.RunID
	if runID == "" {
		if runID, err = engine.NewRunID(); err != nil {
			return nil, err
		}
	}
	outDir, err := resolveOutDir(req.OutDir, req.OutDirIsRoot, sig, runID)
	if err != nil {
		return nil, err
	}

	budgets := plan.Budgets
	if req.TimeoutSeconds > 0 {
		budgets.TimeoutSeconds = req.TimeoutSeconds
	}
	if req.MaxTurns >= 0 {
		budgets.MaxTurns = req.MaxTurns
	}

	var overrides map[string]string
	if prof != nil {
		overrides = prof.Secrets
	}
	var task io.Reader
	if req.Task != "" {
		task = strings.NewReader(req.Task)
	}

	if req.PathPicked != nil {
		for _, q := range m.Intake {
			if q.Type != manifest.IntakeFile {
				continue
			}
			if v := req.Intake[q.ID]; v != "" {
				if err := req.PathPicked("intake "+q.ID, v); err != nil {
					finishProfile(logw, prof, profDir, keep)
					return nil, engine.Classify(engine.ExitUsage, err)
				}
			}
		}
	}
	// CollectRun, not Collect: a url question the browser names as a site is
	// the browser's address, never an input source to fetch.
	answers, err := intake.CollectRun(nil, m, req.Intake, nil)
	if err != nil {
		finishProfile(logw, prof, profDir, keep)
		return nil, engine.Classify(engine.ExitCLI, err)
	}
	if err := giveSourceTokens(answers, req.SourceTokens, func(msg string) {
		_, _ = fmt.Fprintf(logw, "warning: %s\n", msg)
	}); err != nil {
		finishProfile(logw, prof, profDir, keep)
		return nil, engine.Classify(engine.ExitCLI, err)
	}
	// Each target host, readied as `agentry run` readies it, once the
	// images are in place and before anything of the run starts.
	if err := readyTargetHosts(ctx, answers, m.Capabilities.TargetHosts, hostReadiness{
		pol: pol, home: home, given: req.HostSignIns, picked: req.KeyFilePicked, trust: req.TrustHostKey,
		say: func(msg string) { _, _ = fmt.Fprintln(logw, msg) },
	}); err != nil {
		finishProfile(logw, prof, profDir, keep)
		return nil, err
	}
	if len(answers.Secrets) > 0 {
		merged := map[string]string{}
		for k, v := range overrides {
			merged[k] = v
		}
		for id, v := range answers.Secrets {
			merged[intake.SecretName(id)] = v
		}
		overrides = merged
	}
	// The run's profile goes on its crash-path record before the engine
	// starts, so a run that never completes still has an unkept profile
	// removed. Best effort, as the record itself is: a run is not refused
	// because the runner home is unwritable.
	if req.InFlightDir != "" && held != nil {
		if err := RecordProfile(req.InFlightDir, runID, *held); err != nil {
			_, _ = fmt.Fprintf(logw, "warning: the run's profile could not be noted for removal if the app is closed mid-run: %v\n", err)
		}
	}

	// RunnerHome is the home the CLI shares. Where relays share the host's one
	// network (a platform with no docker filter), a run whose plan needs relays
	// holds its relays.lock for the whole run, so a desktop run and a CLI run
	// cannot both list an empty relay network and start relays side by side;
	// sessions take it the same way. Behind the filter (Linux, and macOS and
	// Windows from CLI v0.6.117), each run's relays are on networks of its own and
	// the engine takes no lock.
	return &prepared{
		sig: sig, m: m, plan: plan, docker: docker, prof: prof, profDir: profDir, keep: keep,
		params: engine.Params{
			Plan:             plan,
			PlanDir:          planDir,
			WorkspaceSrc:     workspaceDir,
			CheckWorkspace:   copyCheck(sig, installDir),
			RunRoot:          installDir,
			RunnerHome:       home,
			OutDir:           outDir,
			Task:             task,
			InputPaths:       req.InputPaths,
			Knowledge:        knowledgePacks,
			Voice:            req.Voice,
			ReportTheme:      req.ReportTheme,
			Intake:           answers,
			Adapter:          ad,
			Policy:           pol,
			PolicyFile:       &polFile,
			SecretOverrides:  overrides,
			ServersDir:       serversDir,
			ExecPackages:     execPackages,
			Connections:      conns.tokens,
			RenewConnection:  conns.renew,
			ModelSignIn:      signIn,
			RenewModelSignIn: renewSignIn,
			Budgets:          budgets,
			Attestation:      meta.Attestation,
			RunID:            runID,
			Stderr:           logw,
			OnUsage:          req.OnUsage,
		},
	}, nil
}

func Run(ctx context.Context, req Request, logw io.Writer) (*Result, error) {
	p, err := prepare(ctx, req, logw, manifest.ModeAutonomous)
	if err != nil {
		return nil, err
	}
	if req.OnBudgets != nil {
		req.OnBudgets(p.params.Budgets.TimeoutSeconds, p.params.Budgets.MaxTurns)
	}
	// A release's one-shot run ends blocked, not ok, when its QuickAgent could
	// not do the task, as `agentry run` does. A session (StartSession) and a
	// built-in's run (internal/draft and the others, through localrun) never
	// take it.
	p.params.EndBlocked = true
	env, runErr := engine.Run(ctx, p.docker, p.params)
	// The run completed its lifecycle (success or failure): apply the
	// profile's keep/remove choice exactly like the CLI.
	finishProfile(logw, p.prof, p.profDir, p.keep)

	if env == nil {
		return nil, runErr
	}
	return resultFromEnvelope(env, p.plan), nil
}

// resultFromEnvelope flattens the envelope; the failure classes ride in
// Status/ExitCode/Error so the frontend renders them instead of a
// rejected promise losing the receipt paths.
func resultFromEnvelope(env *receipt.Envelope, plan *spec.Plan) *Result {
	return &Result{
		Unsafe:           planIsUnsafe(plan),
		Status:           env.Status,
		ExitCode:         env.ExitCode,
		Error:            env.Error,
		RunID:            env.RunID,
		Ref:              env.Ref,
		Digest:           env.Digest,
		ResultJSON:       string(env.Result),
		OutDir:           env.OutDir,
		ReceiptsDir:      env.Receipts.Dir,
		RunPlanSHA:       env.Receipts.RunPlanSHA256,
		InputTokens:      env.Usage.InputTokens,
		OutputTokens:     env.Usage.OutputTokens,
		CostUSD:          env.Usage.CostUSD,
		WallMS:           env.Usage.WallMS,
		Turns:            env.Usage.Turns,
		AttestationState: env.Attestation.State,
		Policy:           env.Policy,
		Knowledge:        env.CapabilitiesEnforced["knowledge"],
		Exec:             env.CapabilitiesEnforced["exec"],
		Decode:           env.CapabilitiesEnforced["decode"],
		ExitClass:        receipt.ExitClassName(env.ExitCode),
		Stage:            env.Stage,
		Reason:           env.Reason,
	}
}

// materialize resolves the install for a signature, reusing the CLI's
// cache when the digest matches — under the same consent gate the fresh
// path applies, asked again on every run — and creating it (download →
// digest check → DSSE verify → hardened extract) when not: the gate
// `agentry install` and `agentry run` apply.
func materialize(ctx context.Context, sig *Signature, home string, acceptUnsigned, acceptUnscanned bool, logw io.Writer) (*installdir.Meta, string, error) {
	if sig.Local() {
		// Installed from a folder: no registry holds it, and nothing on disk gives it
		// another state than local, which needs the unsigned consent on every run.
		meta := *sig.localMeta
		if err := cachedAttestationGate(&meta, acceptUnsigned, acceptUnscanned, logw); err != nil {
			return nil, "", err
		}
		_, _ = fmt.Fprintf(logw, "installed from a folder on this computer: %s is local and unsigned (%s)\n", sig.Ref, sig.DigestSuffix())
		return &meta, sig.local, nil
	}
	installDir := installdir.ReleaseDir(home, sig.Namespace, sig.Name, sig.Version, sig.Digest)
	// The installed tree is held to its integrity record before anything
	// trusts it: a release that changed after it was laid down is refused
	// here, before anything is fetched, and Repair is the way back; one that
	// no record of this runner home vouches for is fetched and verified again
	// below as a first fetch is, keeping the export it had.
	meta, ierr := installdir.Inspect(installDir, sig.Digest)
	var changed *installdir.ChangedError
	reExport := ""
	switch {
	case errors.As(ierr, &changed):
		return nil, "", engine.Classify(engine.ExitVerify, asChanged(sig, ierr))
	case errors.Is(ierr, installdir.ErrNoRecord):
		_, _ = fmt.Fprintf(logw, "installed %s has no integrity record this runner home vouches for — fetching it again and checking it as a first fetch is\n", sig.Ref)
		reExport = meta.Export
	case ierr != nil && !errors.Is(ierr, installdir.ErrNotInstalled):
		return nil, "", engine.Classify(engine.ExitCLI, ierr)
	}
	if ierr == nil {
		// The registry's word outranks the cache's memory: an install cached with
		// another state, against a registry that says the version was admitted
		// without a scan, is unscanned.
		if sig.Unscanned() && meta.Attestation.State != trust.StateUnscanned {
			meta.Attestation = receipt.Attestation{State: trust.StateUnscanned}
		}
		// The cache remembers the verdict, not the consent: the gate is asked again
		// on every run (as in the CLI), and the receipt carries the consent given
		// now.
		if err := cachedAttestationGate(meta, acceptUnsigned, acceptUnscanned, logw); err != nil {
			return nil, "", err
		}
		return meta, installDir, nil
	}

	_, _ = fmt.Fprintf(logw, "downloading %s (%s)\n", sig.Ref, sig.DigestSuffix())
	token := settings.TokenFor(sig.Registry)
	archive, err := registry.Download(ctx, sig.Registry, token, sig.resolve)
	if err != nil {
		return nil, "", err
	}
	computed := registry.Digest(archive)
	if computed != sig.Digest {
		return nil, "", fmt.Errorf("digest mismatch: registry says %s, archive is %s — refusing to run", sig.Digest, computed)
	}
	att, err := verifyRunAttestation(ctx, sig, computed, acceptUnsigned, acceptUnscanned, logw)
	if err != nil {
		return nil, "", err
	}

	staging, err := os.MkdirTemp("", "agentry-run-*")
	if err != nil {
		return nil, "", fmt.Errorf("create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()
	// The hardened extractor with the client caps every install uses:
	// traversal, symlink escapes, and bombs refused rather than written.
	if err := pkgarchive.Extract(bytes.NewReader(archive), staging, int64(len(archive)), extractLimits()); err != nil {
		return nil, "", fmt.Errorf("extract archive: %w", err)
	}

	meta = &installdir.Meta{
		Ref:       sig.Ref,
		Namespace: sig.Namespace, Name: sig.Name, Version: sig.Version,
		Digest: sig.Digest, Source: "registry", Attestation: att,
	}
	// The cache holds the verdict and never the consent, which is this
	// run's alone.
	cached := *meta
	cached.Attestation.Consent = ""
	if reExport != "" {
		// An install with no record keeps the export it had when it is laid
		// down again.
		m, err := manifestFromWorkspace(staging)
		if err != nil {
			return nil, "", err
		}
		if err := installdir.Install(installDir, staging, m, &cached, reExport); err != nil {
			return nil, "", err
		}
		return meta, installDir, nil
	}
	if err := installdir.Write(installDir, staging, &cached); err != nil {
		return nil, "", err
	}
	return meta, installDir, nil
}

// cachedAttestationGate re-applies the consent gates to an install the
// cache already holds, and records on the receipt the consent given now:
// a cached unsigned or unscanned release is not "verified", and the
// consent is asked for on every run.
func cachedAttestationGate(meta *installdir.Meta, acceptUnsigned, acceptUnscanned bool, logw io.Writer) error {
	switch meta.Attestation.State {
	case trust.StateUnscanned:
		if !acceptUnscanned {
			return fmt.Errorf("unscanned — the registry admitted %s without a scan (cached install): no findings, no report and no attestation stand behind it; tick \"Run an unscanned release\" to run it anyway", meta.Ref)
		}
		meta.Attestation.Consent = receipt.ConsentAllowUnscanned
		_, _ = fmt.Fprintf(logw, "warning: unscanned — running %s (cached install), which the registry admitted without a scan, because it was explicitly accepted; the run is marked unscanned in its receipts\n", meta.Ref)
	case trust.StateLocal:
		// A QuickAgent installed from a folder, or the CLI's cache of an
		// unpublished directory it ran: no registry scanned or signed it.
		if !acceptUnsigned {
			return fmt.Errorf("local and unsigned — %s was installed from a folder on this computer, and no registry scanned or signed it; tick \"Run an unsigned release\" to run it anyway", meta.Ref)
		}
		meta.Attestation.Consent = receipt.ConsentAllowUnsigned
		_, _ = fmt.Fprintf(logw, "warning: local and unsigned — running %s because it was explicitly accepted; the run is marked local in its receipts\n", meta.Ref)
	case trust.StateUnsigned:
		if !acceptUnsigned {
			return fmt.Errorf("unsigned — the registry offers no attestation for %s (cached install); tick \"Run an unsigned release\" to run it anyway", meta.Ref)
		}
		meta.Attestation.Consent = receipt.ConsentAllowUnsigned
		_, _ = fmt.Fprintf(logw, "warning: unsigned — running %s (cached install) because it was explicitly accepted; the run is marked unsigned in its receipts\n", meta.Ref)
	case trust.StateVerified:
		_, _ = fmt.Fprintf(logw, "using verified install %s\n", meta.Ref)
	default:
		// mismatch and unverifiable never reach the cache; a state this
		// build does not know is not run on the strength of a cache file.
		return fmt.Errorf("attestation %s for %s (cached install) — refusing to run", meta.Attestation.State, meta.Ref)
	}
	return nil
}

// verifyRunAttestation mirrors the CLI's run gate: verified proceeds;
// unsigned proceeds MARKED only with acceptUnsigned (the panel's checkbox,
// enforced in Go) and refuses otherwise; unscanned — a version the
// registry admitted without a scan, read from the resolve answer before
// any attestation is looked for — proceeds MARKED only with
// acceptUnscanned, its own consent; mismatch/unverifiable always refuses —
// there is no override for a signature that fails to verify. The receipt
// names the consent that admitted the release.
func verifyRunAttestation(ctx context.Context, sig *Signature, archiveDigest string, acceptUnsigned, acceptUnscanned bool, logw io.Writer) (receipt.Attestation, error) {
	var att trust.AttestationResult
	if sig.Unscanned() {
		att = trust.Unscanned()
	} else {
		attURL := ""
		if sig.resolve.AttestationUrl != nil {
			attURL = *sig.resolve.AttestationUrl
		}
		if attURL == "" {
			attURL = fmt.Sprintf("/api/v1/agents/%s/%s/versions/%s/attestation", sig.Namespace, sig.Name, sig.Version)
		}
		att = registry.VerifyAttestation(ctx, sig.Registry, settings.TokenFor(sig.Registry), attURL, archiveDigest)
	}
	switch att.State {
	case trust.StateVerified:
		_, _ = fmt.Fprintf(logw, "attestation: verified (key %s)\n", att.KeyID)
		return receipt.Attestation{State: trust.StateVerified, KeyID: att.KeyID, Verified: true}, nil
	case trust.StateUnsigned:
		if !acceptUnsigned {
			return receipt.Attestation{}, fmt.Errorf("unsigned — the registry offers no attestation for %s (%s); tick \"Run an unsigned release\" to run it anyway", sig.Ref, att.Detail)
		}
		_, _ = fmt.Fprintf(logw, "warning: unsigned — the registry offers no attestation for %s; running because it was explicitly accepted; the run is marked unsigned in its receipts\n", sig.Ref)
		return receipt.Attestation{State: trust.StateUnsigned, Consent: receipt.ConsentAllowUnsigned}, nil
	case trust.StateUnscanned:
		if !acceptUnscanned {
			return receipt.Attestation{}, fmt.Errorf("unscanned — the registry admitted %s without a scan: no findings, no report and no attestation stand behind it; tick \"Run an unscanned release\" to run it anyway", sig.Ref)
		}
		_, _ = fmt.Fprintf(logw, "warning: unscanned — running %s, which the registry admitted without a scan, because it was explicitly accepted; the run is marked unscanned in its receipts\n", sig.Ref)
		return receipt.Attestation{State: trust.StateUnscanned, Consent: receipt.ConsentAllowUnscanned}, nil
	default:
		hint := ""
		if att.KeyRotated {
			hint = " (the registry advertises signing key ids you have not pinned — re-pin deliberately if this rotation is expected)"
		}
		return receipt.Attestation{}, fmt.Errorf("attestation %s for %s: %s%s — refusing to run",
			att.State, sig.Ref, att.Detail, hint)
	}
}

// ensurePlan reuses the installed plan when nothing it depends on changed,
// otherwise recompiles — the CLI's exact reuse rule: digest, policy
// fingerprint, adapter, and file integrity all must hold. A reused
// plan.json is first checked against the integrity record its compile
// wrote under the runner home's key, the CLI's own mechanism
// (compile.ReadInstalledPlan): a plan with no record this home can check
// is recompiled, and one that does not match its record (changed after it
// was compiled) refuses the run with the verify class, leaving the edited
// plan in place for a person to look at.
func ensurePlan(logw io.Writer, home string, m *manifest.Manifest, meta *installdir.Meta, installDir, workspaceDir string,
	pol *policy.Policy, polSHA, adapterName string, serverSpecs []spec.MCPServerSpec, mode manifest.Mode,
	unsafe *unsafeexec.Data,
) (*spec.Plan, string, error) {
	key, err := compile.IntegrityKey(home)
	if err != nil {
		return nil, "", engine.Classify(engine.ExitCLI, err)
	}
	// One plan per run mode (plan/<mode>/), exactly like the CLI; a
	// plan/plan.json from the single-plan layout is a stale install.
	if _, err := os.Stat(filepath.Join(installDir, "plan", "plan.json")); err == nil {
		_ = os.RemoveAll(filepath.Join(installDir, "plan"))
	}
	planDir := filepath.Join(installDir, "plan", string(mode))
	// Unsafe Mode changes a plan only where the release declares commands:
	// with none the compile builds no exec image either way.
	wantUnsafe := unsafe != nil && len(execCommands(m)) > 0
	b, rerr := compile.ReadInstalledPlan(key, planDir)
	var tampered *compile.TamperedPlanError
	switch {
	case errors.As(rerr, &tampered):
		return nil, "", engine.Classify(engine.ExitVerify, rerr)
	case errors.Is(rerr, compile.ErrNoIntegrityRecord):
		// A plan an older desktop or CLI compiled, or one from another
		// runner home: never reused, always recompiled.
		_, _ = fmt.Fprintln(logw, "installed plan has no integrity record from this runner home — recompiling")
	}
	if rerr == nil {
		if plan, perr := spec.ParsePlan(b); perr == nil &&
			plan.PlanVersion == spec.PlanVersion &&
			plan.Mode == string(mode) &&
			plan.Digest == meta.Digest &&
			plan.PolicySHA256 == polSHA &&
			plan.Harness.Adapter == adapterName &&
			reflect.DeepEqual(plan.MCPServers, serverSpecs) {
			switch {
			case plan.CompiledBy != version.Build():
				// Same schema, another build: the compiler's output can differ without a
				// schema bump (the exec image spec, the gateway's pinned flags, the
				// servers' env all did while the pins were behind). A plan the CLI compiled
				// reads as another build too, which is right: each client compiles its own.
				_, _ = fmt.Fprintf(logw, "installed plan was compiled by another build (%s, this is %s) — recompiling\n", cmp.Or(plan.CompiledBy, "unknown"), version.Build())
			case plan.Proxy.Image != runnerImageTag(pol):
				_, _ = fmt.Fprintf(logw, "installed plan names runner image %s, this build uses %s — recompiling\n", plan.Proxy.Image, runnerImageTag(pol))
			case planIsUnsafe(plan) != wantUnsafe:
				// The switch changed since this plan was compiled: a run with Unsafe Mode
				// off never reuses a plan compiled with it on, and a run with it on never
				// reuses the catalogue's. The CLI's own reuse refuses an Unsafe Mode plan
				// the same way.
				if wantUnsafe {
					_, _ = fmt.Fprintln(logw, "installed plan was compiled without Unsafe Mode, which is on — recompiling")
				} else {
					_, _ = fmt.Fprintln(logw, "installed plan was compiled in Unsafe Mode, which is off — recompiling without it")
				}
			case wantUnsafe && plan.ExecImage.Unsafe.Base != unsafe.Base:
				_, _ = fmt.Fprintf(logw, "installed plan names the Unsafe Mode image %s, this run uses %s — recompiling\n", plan.ExecImage.Unsafe.Base, unsafe.Base)
			default:
				if verr := compile.VerifyFiles(plan, planDir); verr == nil {
					return plan, planDir, nil
				}
				_, _ = fmt.Fprintln(logw, "installed plan failed integrity check — recompiling")
			}
		}
	}
	// No clearing first: Compile replaces the plan directory whole — it writes a
	// hidden staging sibling and publishes it by rename — so this compile does not
	// delete a plan a concurrent CLI or desktop run has just published, and a
	// publish landing mid-clear cannot refuse this run with "directory not empty".
	// Another build's recompile can still replace the directory after this
	// returns; the engine re-checks every plan file against the plan when it
	// copies them, so that run is refused, never run on the other build's files.
	plan, err := compile.Compile(compile.Inputs{
		Manifest:     m,
		Mode:         mode,
		Digest:       meta.Digest,
		Source:       meta.Source,
		WorkspaceDir: workspaceDir,
		Policy:       pol,
		PolicySHA:    polSHA,
		AdapterName:  adapterName,
		RunnerImage:  runnerImageTag(pol),
		MCPServers:   serverSpecs,
		CompiledBy:   version.Build(),
		IntegrityKey: key,
		Unsafe:       unsafe,
	}, planDir)
	if err != nil {
		var refusal *compile.PolicyRefusal
		if errors.As(err, &refusal) {
			return nil, "", err
		}
		return nil, "", fmt.Errorf("compile run plan: %w", err)
	}
	return plan, planDir, nil
}

// recordCapabilities upserts the accepted contract under the same
// installed-state key the CLI's run consent uses, so neither client
// re-asks for a contract the operator already accepted on this machine.
func recordCapabilities(m *manifest.Manifest, meta *installdir.Meta, home string, logw io.Writer) {
	// The key names the agent's directory under the runner home, the way
	// the CLI's run and an install do, so all three share one record.
	key := installdir.ConsentKey(home, meta.Namespace, meta.Name)
	state, err := installed.Load()
	if err != nil {
		state = installed.State{}
	}
	// An install owns the agent's directory (managed); a run that re-consents
	// keeps that and records nothing managed of its own — its cache is not an
	// install (the CLI's rule). Reconsent also keeps a local install's mark, and
	// records nothing over a local install when this run is of another release.
	if !state.Reconsent(key, meta.Version, installed.Snapshot(m.Contract()), meta.Digest) {
		_, _ = fmt.Fprintf(logw, "the record for %s/%s is its local install; this run's consent is not kept\n", meta.Namespace, meta.Name)
		return
	}
	if err := installed.Save(state); err != nil {
		_, _ = fmt.Fprintf(logw, "warning: %v — the accepted capability contract could not be recorded\n", err)
	}
}

// ensureKeepProfile records an EXPLICIT keep choice for a release that
// declares no secrets: the guided form never runs for such a release, so
// nothing else would persist the choice. Only an explicit true creates
// the (empty) profile — the panel sends nil when the toggle was left
// alone, so no-secret runs do not grow profiles by default; the CLI's
// `--keep-profile=true` does the same.
func ensureKeepProfile(profDir string, sig *Signature, prof *profile.Profile, explicit *bool) (*profile.Profile, error) {
	if prof != nil || explicit == nil || !*explicit || len(allSecretNames(sig.Manifest)) > 0 {
		return prof, nil
	}
	prof = profile.New(sig.Ref, sig.Digest, true, nil)
	if err := prof.Save(profDir); err != nil {
		return nil, err
	}
	return prof, nil
}

// finishProfile applies the completion choice after the run's lifecycle
// finished, success or failure: keep → persist + stamp lastUsedAt;
// remove → the profile dies with the run.
func finishProfile(logw io.Writer, prof *profile.Profile, dir string, keep bool) {
	if prof == nil {
		return
	}
	if keep {
		prof.Keep = true
		if err := prof.Touch(dir); err != nil {
			_, _ = fmt.Fprintf(logw, "warning: %v\n", err)
		}
		return
	}
	if err := profile.Remove(dir); err != nil {
		_, _ = fmt.Fprintf(logw, "warning: %v\n", err)
		return
	}
	_, _ = fmt.Fprintln(logw, "MCP profile removed (run completed with keep-profile=false)")
}

// resolveOutDir picks and creates the output folder for a local run: a
// folder of its own inside dir when dir is shared by every run.
func resolveOutDir(dir string, root bool, sig *Signature, runID string) (string, error) {
	if root && dir != "" {
		dir = RunFolder(dir, sig.Namespace, sig.Name, runID)
	}
	return OutputDir(dir, sig.Namespace, sig.Name, runID)
}

// RunFolder is a run's own folder inside root, named as the default's is.
func RunFolder(root, namespace, name, runID string) string {
	return filepath.Join(root, fmt.Sprintf("%s-%s-%s", namespace, name, runID))
}

// OutputDir picks and creates a run's output folder. The desktop default
// is ~/Downloads/agentry-out/<ns>-<name>-<runid> — a GUI has no meaningful
// cwd for the CLI's ./agentry-out (the same reasoning). A run In Agentry
// downloads its artefacts into the same folder a local run would have
// written into, so the two kinds land in one place.
func OutputDir(dir, namespace, name, runID string) (string, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home directory (pick an output folder instead): %w", err)
		}
		dir = RunFolder(filepath.Join(home, "Downloads", "agentry-out"), namespace, name, runID)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve output dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}
	return abs, nil
}

// manifestFromWorkspace parses agent.yaml at a tree root (the verified
// extracted archive — the manifest that gets compiled is the one that was
// verified, not the resolve response).
func manifestFromWorkspace(dir string) (*manifest.Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, "agent.yaml")) //nolint:gosec // package root
	if err != nil {
		return nil, fmt.Errorf("read agent.yaml: %w", err)
	}
	m, err := manifest.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("parse agent.yaml: %w", err)
	}
	return m, nil
}

// extractLimits is the app's one bound on what it unpacks.
func extractLimits() pkgarchive.ExtractLimits { return registry.ExtractLimits() }

// loadPolicyForRead loads the operator policy without creating it: status
// surfaces must not write files as a side effect of looking.
func loadPolicyForRead() *policy.Policy {
	path, err := policy.DefaultPath()
	if err != nil {
		return policy.Default()
	}
	pol, _, err := policy.Load(path)
	if err != nil {
		return policy.Default()
	}
	return pol
}
