// Package run is the desktop run engine (RUNNER-PLAN-2 §P6): the same
// lifecycle `agentry run` executes — resolve → verify → materialize →
// compile → enforce → run → receipt — driven through the public
// agentry-cli/pkg/runner packages, never a parallel implementation (§P8).
// The install cache, plan layout, and MCP profiles under ~/.agentry/runner
// are byte-compatible with the CLI's, so either client can pick up where
// the other left off.
package run

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/blaktron/agentry-cli/runnerimage"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
	"github.com/blaktron/agentry-cli/pkg/runner/mcpinstall"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/profile"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/pkg/trust"

	"github.com/blaktron/agentry-desktop/internal/mcpvuln"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// newDockerClient builds the docker client every run/setup surface talks
// to. A package variable purely as a test seam: tests substitute
// dockerx.Fake (the CLI engine tests' technique, RUNNER-DEVIATIONS DR-7)
// so the whole lifecycle runs without a docker daemon. Production code
// never reassigns it.
var newDockerClient = func() dockerx.Client { return dockerx.New() }

// Home is ~/.agentry/runner, or $AGENTRY_RUNNER_HOME — the CLI's exact
// derivation (cmd_run.go runnerHome), so both clients share one cache.
func Home() (string, error) { return policy.Home() }

// runnerImageTag derives the local runner image tag from the embedded
// build context's content version — the same value the CLI uses — so both
// clients build, probe and run one image per context (agentry-cli#6,
// D-009). A policy `images.runner` pin still wins in both.
func runnerImageTag(pol *policy.Policy) string {
	return pol.RunnerImageTag(runnerimage.ContentVersion())
}

// installMeta is <installDir>/install.json — field-for-field the CLI's
// struct (cmd_run.go), because both clients read each other's installs.
type installMeta struct {
	Ref         string              `json:"ref"`
	Namespace   string              `json:"namespace"`
	Name        string              `json:"name"`
	Version     string              `json:"version"`
	Digest      string              `json:"digest"`
	Source      string              `json:"source"`
	Attestation receipt.Attestation `json:"attestation"`
}

// Signature is one resolved release: the identity (ns/name@version +
// digest) the install cache and MCP profiles are keyed by (§P2).
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
}

// DigestSuffix is the 12-char digest form used in directory names.
func (s *Signature) DigestSuffix() string { return digestSuffix(s.Digest) }

// Unscanned says whether the registry admitted the resolved version
// without a scan (core M21): read from the resolve answer, before any
// attestation is looked for, because the absence is the fact and not a
// missing signature. A registry that predates the field never published
// one, so a nil answer is "no".
func (s *Signature) Unscanned() bool { return registry.Unscanned(s.resolve) }

// ResolveSignature resolves a ref against a registry and decodes the
// manifest the resolve result carries. installIntent adds the CLI's
// X-Agentry-Install hint (a run is an install, §R5).
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

// ProfileInfo is a BriefAgent signature's MCP-profile state (§P2/§P3).
// Secret VALUES never appear here — names only.
type ProfileInfo struct {
	Ref               string
	Digest            string
	DeclaredSecrets   []string
	Exists            bool
	Keep              bool
	StoredSecretNames []string
	CapabilityLines   []string                  // Intake are the release's up-front questions.
	Intake            []manifest.IntakeQuestion // Modes are the run modes the release declares; Opening its interactive
	// opening line.
	Modes   []string
	Opening string
}

// Declared reports what a release declares, from the registry's resolve
// alone: the run modes, and every secret name — its own and its declared
// MCP servers' — that a run would have to supply. The In-Agentry
// availability rule (services_hostedrun.go) reads this rather than
// ProfileStatus, because a hosted run must touch nothing on this machine:
// no operator policy, no MCP profile, no docker (hosted-runs.md §4.7).
func Declared(ctx context.Context, regURL, ref string) (modes, secrets []string, err error) {
	sig, err := ResolveSignature(ctx, regURL, ref, false)
	if err != nil {
		return nil, nil, err
	}
	for _, mode := range sig.Manifest.SupportedModes() {
		modes = append(modes, string(mode))
	}
	return modes, allSecretNames(sig.Manifest), nil
}

// ProfileStatus resolves a ref and reports its profile state. Keep on a
// non-existent profile is the policy default, so the form's toggle starts
// where guided setup would.
func ProfileStatus(ctx context.Context, regURL, ref string) (*ProfileInfo, error) {
	sig, err := ResolveSignature(ctx, regURL, ref, false)
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
		Keep:            loadPolicyForRead().KeepProfilesByDefault(),
		CapabilityLines: capabilityLines(sig.Manifest.Capabilities),
		Intake:          sig.Manifest.Intake,
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
// guided form (§P3 as a form). Empty-string values mean "resolve from the
// environment at run time" and are recorded as absent, not empty; an
// existing profile's CreatedAt survives reconfiguration.
func SaveProfile(ctx context.Context, regURL, ref string, secrets map[string]string, keep bool) (*ProfileInfo, error) {
	sig, err := ResolveSignature(ctx, regURL, ref, false)
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
	// AllowVulnerableMCP runs even when the registry reports a high or
	// critical advisory for a declared MCP server (the CLI's
	// --allow-vulnerable-mcp); recorded in the receipt.
	AllowVulnerableMCP bool
	Task               string
	// InputPaths feed the read-only /inputs channel (§P4).
	InputPaths []string
	// OutDir is where reports/receipts land ("" = ~/Downloads/agentry-out/
	// <ns>-<name>-<runid>).
	OutDir string
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
	// method meets the same gate (desktop#3, D-011 closure). Digest
	// mismatch and unverifiable attestations refuse regardless.
	AcceptUnsigned bool
	// AcceptUnscanned is the explicit consent to run a release the registry
	// admitted without a scan (the CLI's --allow-unscanned; plan
	// organization-subscriptions §4): its own consent, which AcceptUnsigned
	// does not give. Enforced here, as AcceptUnsigned is, and on every run
	// from the cache — the cache remembers the verdict, not the consent.
	AcceptUnscanned bool
	// Intake answers the manifest's up-front questions by id (plan §3.5);
	// a required question without an answer refuses the run.
	Intake map[string]string
	// PathPicked, when set, judges every host path the request carries
	// that the caller did not already judge — a file-typed intake answer
	// — before it is staged: the desktop passes the native picker's
	// record, since a page can type a path as easily as pick one (D-030).
	PathPicked func(what, path string) error
}

// Result is the envelope-derived outcome. It is populated on failure too:
// the §R8 envelope always exists once the engine starts, and earlier
// failures are returned as errors instead.
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
}

// Run executes the whole §R6 lifecycle for one registry release. Progress
// and diagnostics stream to logw (the Wails event channel upstairs).
// prepared is everything Run and StartSession share: the verified install,
// the plan for the mode, the images, the MCP profile and the engine
// parameters. One code path, two ways to drive the harness (DR-15).
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
	policyPath, err := policy.DefaultPath()
	if err != nil {
		return nil, err
	}
	pol, polSHA, err := policy.EnsureDefault(policyPath)
	if err != nil {
		return nil, fmt.Errorf("operator policy: %w", err)
	}

	sig, err := ResolveSignature(ctx, req.Registry, req.Ref, true)
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

	// The §R5 capability record: the app showed the declared contract on
	// the page and in the run panel, and the Run click is the acceptance
	// the CLI collects as a prompt; record it under the same key so CLI
	// and desktop runs share one consent history (D-010).
	recordCapabilities(m, meta, installDir, logw)

	ad, err := adapter.Get(adapter.DefaultName)
	if err != nil {
		return nil, err
	}

	// Declared MCP servers (RUNNER-PLAN-3): resolve pins (offline when the
	// signature's store matches), compile them into the plan, install into
	// the store with the images below.
	serversDir := mcpinstall.StoreDir(profile.Dir(home, sig.Namespace, sig.Name, sig.Version, sig.DigestSuffix()))
	var resolvedServers []mcpinstall.Resolved
	var serverSpecs []spec.MCPServerSpec
	var vulnLine string
	if len(m.Capabilities.MCPServers) > 0 {
		// The registry's vulnerability verdict on each declared server
		// (agentry#177, DR-16), the same rule as `agentry run`: a known
		// high/critical advisory refuses unless the request or the policy
		// allows it; an unreachable registry does not stop the run; the
		// receipt records the outcome.
		statuses := mcpinstall.CheckVulnerabilities(ctx, nil, req.Registry, m.Capabilities.MCPServers)
		_, _ = fmt.Fprintf(logw, "declared MCP servers, per the registry: %s\n", mcpinstall.Summary(statuses))
		var refuseErr error
		vulnLine, refuseErr = mcpvuln.Verdict(statuses, req.Registry, req.AllowVulnerableMCP, pol.MCPServers.AllowVulnerable)
		if refuseErr != nil {
			return nil, refuseErr
		}
		resolvedServers, err = ensureServerResolutions(ctx, logw, serversDir, pol, m)
		if err != nil {
			return nil, err
		}
		serverSpecs = mcpinstall.Specs(resolvedServers, m.Capabilities.MCPServers, pol.Images)
	}

	plan, planDir, err := ensurePlan(logw, m, meta, installDir, workspaceDir, pol, polSHA, adapter.DefaultName, serverSpecs, mode)
	if err != nil {
		return nil, err
	}
	if vulnLine != "" {
		if plan.Enforcement == nil {
			plan.Enforcement = map[string]string{}
		}
		plan.Enforcement["mcpServers.vulnerabilities"] = vulnLine
	}

	docker := newDockerClient()
	// The last argument asks for the pinned harness image. A host-run
	// harness runs no container, so pulling it would be a download nobody
	// asked for; the desktop only compiles the default container adapter
	// today, and the plan is the honest place to read that from.
	if err := gateway.EnsureInstalled(ctx, docker, pol, runnerImageTag(pol), filepath.Join(home, "image"), logw,
		plan.Harness.Host == nil); err != nil {
		return nil, fmt.Errorf("gateway/docker environment: %w", err)
	}
	// The per-BriefAgent exec image the plan names when the agent declares
	// exec (agentry-cli#9, desktop#17): built once, same as the CLI, from
	// the spec the compiler put on the plan (the catalogued packages and
	// the mirror, not just the command list; agentry-cli#100). The package
	// versions it resolved are the CLI's receipt line; the desktop's
	// receipt omits them (desktop#95).
	if plan.ExecImage != nil {
		if _, err := gateway.EnsureExecImage(ctx, docker, runnerImageTag(pol), *plan.ExecImage, filepath.Join(home, "image"), logw); err != nil {
			return nil, fmt.Errorf("exec image: %w", err)
		}
	}
	if len(resolvedServers) > 0 {
		// pol.Docker.Runtime: the operator's container runtime (gVisor when
		// set, agentry-cli#136) applies to the server install builds too.
		if err := mcpinstall.EnsureInstalled(ctx, docker, resolvedServers, serversDir,
			pol.MCPRegistries, pol.Images, pol.Docker.Runtime, logw); err != nil {
			return nil, fmt.Errorf("mcp servers: %w", err)
		}
	}

	// The MCP profile (§P2): the desktop never prompts here — the guided
	// form (SaveProfile) runs before this, and a missing secret still
	// refuses inside the engine with the variable named (v1 exit-11
	// semantics, nothing degrades silently).
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

	runID, err := engine.NewRunID()
	if err != nil {
		return nil, err
	}
	outDir, err := resolveOutDir(req.OutDir, sig, runID)
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
	answers, err := intake.Collect(nil, m.Intake, req.Intake, m.Capabilities.Secrets)
	if err != nil {
		finishProfile(logw, prof, profDir, keep)
		return nil, engine.Classify(engine.ExitCLI, err)
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
	return &prepared{
		sig: sig, m: m, plan: plan, docker: docker, prof: prof, profDir: profDir, keep: keep,
		params: engine.Params{
			Plan:            plan,
			PlanDir:         planDir,
			WorkspaceSrc:    workspaceDir,
			RunRoot:         installDir,
			OutDir:          outDir,
			Task:            task,
			InputPaths:      req.InputPaths,
			Intake:          answers,
			Adapter:         ad,
			Policy:          pol,
			SecretOverrides: overrides,
			ServersDir:      serversDir,
			Budgets:         budgets,
			Attestation:     meta.Attestation,
			RunID:           runID,
			Stderr:          logw,
		},
	}, nil
}

func Run(ctx context.Context, req Request, logw io.Writer) (*Result, error) {
	p, err := prepare(ctx, req, logw, manifest.ModeAutonomous)
	if err != nil {
		return nil, err
	}
	env, runErr := engine.Run(ctx, p.docker, p.params)
	// The run completed its lifecycle (success or failure): apply the
	// profile's keep/remove choice exactly like the CLI (§P2).
	finishProfile(logw, p.prof, p.profDir, p.keep)

	if env == nil {
		return nil, runErr
	}
	return resultFromEnvelope(env), nil
}

// resultFromEnvelope flattens the §R8 envelope; the failure classes ride
// in Status/ExitCode/Error so the frontend renders them instead of a
// rejected promise losing the receipt paths.
func resultFromEnvelope(env *receipt.Envelope) *Result {
	return &Result{
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
	}
}

// materialize resolves the install for a signature, reusing the CLI's
// cache when the digest matches — under the same consent gate the fresh
// path applies, asked again on every run — and creating it (download →
// digest check → DSSE verify → hardened extract) when not: the gate
// `agentry install` and `agentry run` apply (§R1, DR-21, DR-27).
func materialize(ctx context.Context, sig *Signature, home string, acceptUnsigned, acceptUnscanned bool, logw io.Writer) (*installMeta, string, error) {
	installDir := filepath.Join(home, "agents", sig.Namespace, sig.Name,
		fmt.Sprintf("%s-%s", sig.Version, sig.DigestSuffix()))
	if meta, ok := loadInstallMeta(installDir, sig.Digest); ok {
		// The registry's word today outranks the cache's memory: an
		// install cached before the unscanned state existed, against a
		// registry that now says the version was admitted without a
		// scan, is unscanned.
		if sig.Unscanned() && meta.Attestation.State != trust.StateUnscanned {
			meta.Attestation = receipt.Attestation{State: trust.StateUnscanned}
		}
		// The cache remembers the verdict, not the consent: the gate is
		// asked again on every run (the CLI's DR-21), and the receipt
		// carries the consent given now.
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

	meta := &installMeta{
		Ref:       sig.Ref,
		Namespace: sig.Namespace, Name: sig.Name, Version: sig.Version,
		Digest: sig.Digest, Source: "registry", Attestation: att,
	}
	// The cache holds the verdict and never the consent, which is this
	// run's alone.
	cached := *meta
	cached.Attestation.Consent = ""
	if err := writeInstall(installDir, staging, &cached); err != nil {
		return nil, "", err
	}
	return meta, installDir, nil
}

// cachedAttestationGate re-applies the consent gates to an install the
// cache already holds, and records on the receipt the consent given now:
// a cached unsigned or unscanned release is not "verified", and the
// consent is asked for on every run.
func cachedAttestationGate(meta *installMeta, acceptUnsigned, acceptUnscanned bool, logw io.Writer) error {
	switch meta.Attestation.State {
	case trust.StateUnscanned:
		if !acceptUnscanned {
			return fmt.Errorf("unscanned — the registry admitted %s without a scan (cached install): no findings, no report and no attestation stand behind it; tick \"Run an unscanned release\" to run it anyway", meta.Ref)
		}
		meta.Attestation.Consent = receipt.ConsentAllowUnscanned
		_, _ = fmt.Fprintf(logw, "warning: unscanned — running %s (cached install), which the registry admitted without a scan, because it was explicitly accepted; the run is marked unscanned in its receipts\n", meta.Ref)
	case trust.StateUnsigned, "local":
		// "local" is the CLI's record of an unpublished directory it ran;
		// the two share this cache, and the CLI spells the state without
		// a constant.
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
// now enforced in Go — desktop#3 closes D-011) and refuses otherwise;
// unscanned — a version the registry admitted without a scan, read from
// the resolve answer before any attestation is looked for — proceeds
// MARKED only with acceptUnscanned, its own consent; mismatch/unverifiable
// always refuses — there is no override for a signature that fails to
// verify. The receipt names the consent that admitted the release.
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
		att = trust.VerifyAttestation(ctx, registry.HTTPClient, sig.Registry, settings.TokenFor(sig.Registry), attURL, archiveDigest)
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

// writeInstall lands workspace + install.json under installDir — build
// aside, then rename, so a half-written install never looks installed
// (the CLI's exact procedure).
func writeInstall(installDir, srcTree string, meta *installMeta) error {
	if err := os.RemoveAll(installDir); err != nil {
		return fmt.Errorf("clear install dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(installDir), 0o755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}
	tmp := installDir + ".partial"
	if err := os.RemoveAll(tmp); err != nil {
		return fmt.Errorf("clear partial install: %w", err)
	}
	if err := copyTree(srcTree, filepath.Join(tmp, "workspace")); err != nil {
		return err
	}
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal install metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "install.json"), append(b, '\n'), 0o644); err != nil { //nolint:gosec // metadata, not secret
		return fmt.Errorf("write install metadata: %w", err)
	}
	if err := os.Rename(tmp, installDir); err != nil {
		return fmt.Errorf("finalize install dir: %w", err)
	}
	return nil
}

// loadInstallMeta reuses a cached verified install iff its recorded digest
// matches what the registry says now (the CLI's reuse rule).
func loadInstallMeta(installDir, wantDigest string) (*installMeta, bool) {
	b, err := os.ReadFile(filepath.Join(installDir, "install.json")) //nolint:gosec // our install layout
	if err != nil {
		return nil, false
	}
	var meta installMeta
	if err := json.Unmarshal(b, &meta); err != nil || meta.Digest != wantDigest {
		return nil, false
	}
	if _, err := os.Stat(filepath.Join(installDir, "workspace")); err != nil {
		return nil, false
	}
	return &meta, true
}

// ensurePlan reuses the installed plan when nothing it depends on changed
// (§R6/§R9), otherwise recompiles — the CLI's exact reuse rule: digest,
// policy fingerprint, adapter, and file integrity all must hold.
func ensurePlan(logw io.Writer, m *manifest.Manifest, meta *installMeta, installDir, workspaceDir string,
	pol *policy.Policy, polSHA, adapterName string, serverSpecs []spec.MCPServerSpec, mode manifest.Mode,
) (*spec.Plan, string, error) {
	// One plan per run mode (plan/<mode>/), exactly like the CLI; a
	// plan/plan.json from the single-plan layout is a stale install.
	if _, err := os.Stat(filepath.Join(installDir, "plan", "plan.json")); err == nil {
		_ = os.RemoveAll(filepath.Join(installDir, "plan"))
	}
	planDir := filepath.Join(installDir, "plan", string(mode))
	if b, err := os.ReadFile(filepath.Join(planDir, "plan.json")); err == nil { //nolint:gosec // our install layout
		if plan, perr := spec.ParsePlan(b); perr == nil &&
			plan.PlanVersion == spec.PlanVersion &&
			plan.Mode == string(mode) &&
			plan.Digest == meta.Digest &&
			plan.PolicySHA256 == polSHA &&
			plan.Harness.Adapter == adapterName &&
			reflect.DeepEqual(plan.MCPServers, serverSpecs) {
			switch {
			case plan.CompiledBy != version.Version:
				// Same schema, another build: the compiler's output can
				// differ without a schema bump (the exec image spec, the
				// gateway's pinned flags, the servers' env all did while
				// the pins were behind; desktop#94, the CLI's own guard is
				// agentry-cli#105). A plan the CLI compiled reads as another
				// build too, which is right: each client compiles its own.
				_, _ = fmt.Fprintf(logw, "installed plan was compiled by another build (%s, this is %s) — recompiling\n", cmp.Or(plan.CompiledBy, "unknown"), version.Version)
			case plan.Proxy.Image != runnerImageTag(pol):
				_, _ = fmt.Fprintf(logw, "installed plan names runner image %s, this build uses %s — recompiling\n", plan.Proxy.Image, runnerImageTag(pol))
			default:
				if verr := compile.VerifyFiles(plan, planDir); verr == nil {
					return plan, planDir, nil
				}
				_, _ = fmt.Fprintln(logw, "installed plan failed integrity check — recompiling")
			}
		}
	}
	if err := os.RemoveAll(planDir); err != nil {
		return nil, "", fmt.Errorf("clear stale plan: %w", err)
	}
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
		CompiledBy:   version.Version,
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
func recordCapabilities(m *manifest.Manifest, meta *installMeta, installDir string, logw io.Writer) {
	key := installed.Key{
		Registry: "runner", Namespace: meta.Namespace, Name: meta.Name,
		Target: "runner", Dir: filepath.Dir(installDir),
	}
	state, err := installed.Load()
	if err != nil {
		state = installed.State{}
	}
	state.Upsert(key, meta.Version, installed.Snapshot(m.Capabilities), false)
	if err := installed.Save(state); err != nil {
		_, _ = fmt.Fprintf(logw, "warning: %v — the accepted capability contract could not be recorded\n", err)
	}
}

// ensureKeepProfile records an EXPLICIT keep choice for a release that
// declares no secrets (D-012 closed, desktop#7): the guided form never
// runs for such a release, so nothing else would persist the choice. Only
// an explicit true creates the (empty) profile — the panel sends nil when
// the toggle was left alone, so no-secret runs do not grow profiles by
// default; the CLI's `--keep-profile=true` does the same.
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
// finished, success or failure (§P2): keep → persist + stamp lastUsedAt;
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

// resolveOutDir picks and creates the output folder for a local run.
func resolveOutDir(dir string, sig *Signature, runID string) (string, error) {
	return OutputDir(dir, sig.Namespace, sig.Name, runID)
}

// OutputDir picks and creates a run's output folder. The desktop default
// is ~/Downloads/agentry-out/<ns>-<name>-<runid> — a GUI has no meaningful
// cwd for the CLI's ./agentry-out (same reasoning as D-002). A run In
// Agentry downloads its artefacts into the same folder a local run would
// have written into, so the two kinds land in one place.
func OutputDir(dir, namespace, name, runID string) (string, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home directory (pick an output folder instead): %w", err)
		}
		dir = filepath.Join(home, "Downloads", "agentry-out",
			fmt.Sprintf("%s-%s-%s", namespace, name, runID))
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

// extractLimits bounds what the app unpacks — identical to the install
// engine's (and the CLI's): the client's job is only to refuse a bomb.
func extractLimits() pkgarchive.ExtractLimits {
	return pkgarchive.ExtractLimits{
		MaxFileBytes:  registry.ExtractMaxFileBytes,
		MaxTotalBytes: registry.ExtractMaxTotalBytes,
		MaxFiles:      20000,
		MaxRatio:      500,
	}
}

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

// digestSuffix shortens sha256:… for directory names (CLI parity).
func digestSuffix(digest string) string {
	s := strings.TrimPrefix(digest, "sha256:")
	if len(s) > 12 {
		s = s[:12]
	}
	return s
}

// copyTree copies src into dest preserving permission bits. Replicated
// from internal/install rather than shared — the D-006 call, same reasons.
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
