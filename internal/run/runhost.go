package run

// A run on one of the person's remote runhosts. The release is resolved,
// materialized and verified here exactly as a local run's, consented to here,
// and sent with the job to the runhost, which verifies it again with its own
// pins, runs it under gVisor as a local run of its own, and seals its output
// to this computer; the output is opened into the run's folder and the
// envelope names the runhost it ran on (the CLI's pkg/runhost/remote).
//
// What a run from the app carries to a runhost: the task, the input files,
// the intake's text, choice, address and secret answers, the release's
// declared secrets (the saved MCP profile, then the environment or the
// keychain as the policy says) and the model's key, or nothing for a model
// paid by a plan's sign-in or Agentry credits, which the runhost keeps
// itself. A release that needs more than that (a connection, a knowledge
// pack, a target host, a file answer) is refused before anything is sent,
// with the CLI's `agentry run --on`, which carries it, named.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	"github.com/blaktron/agentry-cli/pkg/runhost/job"
	"github.com/blaktron/agentry-cli/pkg/runhost/payload"
	"github.com/blaktron/agentry-cli/pkg/runhost/remote"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/intake"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/runner/profile"
	"github.com/blaktron/agentry-cli/pkg/runner/receipt"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
	"github.com/blaktron/agentry-cli/pkg/trust"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// RunhostRequest is a run on runhost Runhost: the local run's own fields that
// a runhost takes.
type RunhostRequest struct {
	Runhost         string
	Registry        string
	Ref             string
	Task            string
	InputPaths      []string
	Intake          map[string]string
	OutDir          string
	OutDirIsRoot    bool
	TimeoutSeconds  int
	MaxTurns        int
	AcceptUnsigned  bool
	AcceptUnscanned bool
	// CLIVersion is the CLI release this app carries, which the runhost
	// must run too.
	CLIVersion string
	// Say takes each line of the runhost's narration.
	Say func(string)
}

// RanOn is the runhost a run ran on, for its result.
type RanOn struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Fingerprint string `json:"fingerprint"`
}

// runhostHTTP fetches a release's attestation to send with the job.
var runhostHTTP = &http.Client{Timeout: 60 * time.Second}

// RunOnRunhost runs an autonomous release on a runhost and collects it.
func RunOnRunhost(ctx context.Context, req RunhostRequest, logw io.Writer) (*Result, *RanOn, error) {
	say := req.Say
	if say == nil {
		say = func(string) {}
	}
	home, err := Home()
	if err != nil {
		return nil, nil, err
	}
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		return nil, nil, err
	}
	rec, ok := recs.Find(req.Runhost)
	if !ok {
		return nil, nil, engine.Classify(engine.ExitUsage, fmt.Errorf("no runhost is named %q on this computer", req.Runhost))
	}
	keys, err := runhost.LoadKeys(rec.Name)
	if err != nil {
		return nil, nil, engine.Classify(engine.ExitGateway, err)
	}
	ranOn := &RanOn{Name: rec.Name, Host: rec.Host, Fingerprint: rec.HostKeyFingerprint}

	sig, err := ResolveSignature(ctx, req.Registry, req.Ref, false)
	if err != nil {
		return nil, ranOn, err
	}
	if sig.Local() {
		return nil, ranOn, engine.Classify(engine.ExitUsage, fmt.Errorf("%s is installed from a folder on this computer; a runhost runs a published release", sig.Ref))
	}
	meta, installDir, err := materialize(ctx, sig, home, req.AcceptUnsigned, req.AcceptUnscanned, logw)
	if err != nil {
		return nil, ranOn, err
	}
	m, err := localrun.ManifestFrom(filepath.Join(installDir, "workspace"))
	if err != nil {
		return nil, ranOn, engine.Classify(engine.ExitCompile, err)
	}
	if what := RunhostCannotCarry(m); what != "" {
		return nil, ranOn, engine.Classify(engine.ExitUsage, fmt.Errorf("a run on a runhost from this app does not carry %s yet: run it with `agentry run %s --on %s` in a terminal", what, sig.Ref, rec.Name))
	}
	pf, err := policy.Resolve("")
	if err != nil {
		return nil, ranOn, err
	}
	pol, _, err := policy.EnsureDefault(pf.Path)
	if err != nil {
		return nil, ranOn, engine.Classify(engine.ExitPolicy, err)
	}
	modelKey, err := remote.ModelKey(pol)
	if err != nil {
		return nil, ranOn, engine.Classify(engine.ExitPolicy, err)
	}

	// The release as it is, held to what was verified here: the same digest,
	// and an attestation this computer's own pins verify.
	token := settings.TokenFor(sig.Registry)
	archive, err := registry.Download(ctx, sig.Registry, token, sig.resolve)
	if err != nil {
		return nil, ranOn, err
	}
	if got := registry.Digest(archive); got != sig.Digest {
		return nil, ranOn, engine.Classify(engine.ExitVerify, fmt.Errorf("digest mismatch: registry says %s, archive is %s — refusing to run", sig.Digest, got))
	}
	rel := job.Release{Namespace: sig.Namespace, Name: sig.Name, Version: sig.Version, Digest: sig.Digest, Size: int64(len(archive)), Registry: sig.Registry}
	switch meta.Attestation.State {
	case trust.StateVerified:
		attURL := fmt.Sprintf("/api/v1/agents/%s/%s/versions/%s/attestation", sig.Namespace, sig.Name, sig.Version)
		if sig.resolve.AttestationUrl != nil && *sig.resolve.AttestationUrl != "" {
			attURL = *sig.resolve.AttestationUrl
		}
		env, err := trust.FetchEnvelope(ctx, runhostHTTP, sig.Registry, token, attURL)
		if err != nil {
			return nil, ranOn, engine.Classify(engine.ExitVerify, err)
		}
		pins, err := trust.LoadPinned(sig.Registry)
		if err != nil {
			return nil, ranOn, engine.Classify(engine.ExitVerify, err)
		}
		if v := trust.VerifyEnvelope(env, pins, sig.Digest); v.State != trust.StateVerified {
			return nil, ranOn, engine.Classify(engine.ExitVerify, fmt.Errorf("the attestation sent to the runhost is %s here: %s", v.State, v.Detail))
		}
		rel.State, rel.Attestation, rel.Keys = job.StateVerified, env, pins
	case trust.StateUnsigned:
		rel.State = job.StateUnsigned
	case trust.StateUnscanned:
		rel.State = job.StateUnscanned
	default:
		return nil, ranOn, engine.Classify(engine.ExitVerify, fmt.Errorf("%s is %s; a runhost runs a verified, unsigned or unscanned release", sig.Ref, meta.Attestation.State))
	}

	run, err := runhostCarry(home, sig, m, pol, req)
	if err != nil {
		return nil, ranOn, err
	}
	var entries []payload.Entry
	seen := map[string]bool{}
	for _, p := range req.InputPaths {
		name := payload.KindInputs + "/" + payload.CleanName(filepath.Base(p))
		if seen[name] {
			return nil, ranOn, engine.Classify(engine.ExitUsage, fmt.Errorf("two input files are named %s; a run takes each name once", filepath.Base(p)))
		}
		seen[name] = true
		entries = append(entries, payload.Entry{Name: name, Src: p})
		run.Inputs = append(run.Inputs, name)
	}
	var files *payload.Packed
	if len(entries) > 0 {
		if files, err = payload.Pack(entries); err != nil {
			return nil, ranOn, engine.Classify(engine.ExitUsage, err)
		}
		defer files.Remove()
		run.Payload = &job.Payload{Size: files.Size, Digest: files.Digest}
	}
	id, err := remote.NewJobID()
	if err != nil {
		return nil, ranOn, err
	}
	run.Task, run.AllowUnsigned, run.AllowUnscanned = req.Task, req.AcceptUnsigned, req.AcceptUnscanned
	run.Timeout, run.MaxTurns, run.MaxCost = req.TimeoutSeconds, req.MaxTurns, -1
	if run.MaxTurns == 0 {
		run.MaxTurns = -1
	}
	run.Model, run.ModelKey = job.ModelOf(pol.Model), modelKey
	j := &job.Job{V: job.Version, ID: id, Device: rec.DeviceID, Release: rel, Run: run}
	if err := j.Validate(); err != nil {
		return nil, ranOn, engine.Classify(engine.ExitUsage, err)
	}

	c, err := runhost.DialRunhost(ctx, *rec, keys)
	if err != nil {
		return nil, ranOn, engine.Classify(engine.ExitGateway, err)
	}
	defer func() { _ = c.Close() }()
	if err := sameRunhostVersion(ctx, c, rec.Name, req.CLIVersion); err != nil {
		return nil, ranOn, engine.Classify(engine.ExitGateway, err)
	}
	say(fmt.Sprintf("running %s on %s (%s, host key %s)", sig.Ref, rec.Name, rec.Host, rec.HostKeyFingerprint))
	lj := remote.LocalJob{Job: j.ID, Runhost: rec.Name, Ref: sig.Ref, Namespace: sig.Namespace, Name: sig.Name, Version: sig.Version,
		Digest: sig.Digest, Out: req.OutDir, StartedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := remote.PutJob(home, lj); err != nil {
		return nil, ranOn, err
	}
	res, started, next, err := remote.Start(ctx, c, j, archive, files, false, say, nil, nil)
	switch {
	case err != nil && !started:
		remote.DropJob(home, j.ID)
		return nil, ranOn, engine.Classify(remote.SendClass(err), err)
	case err != nil:
		var c2 *runhost.Client
		res, c2, err = remote.Reattach(ctx, rec, keys, j.ID, next, lineWriter(say), say, nil)
		if err != nil {
			return nil, ranOn, engine.Classify(engine.ExitGateway, fmt.Errorf("lost %s: %w; the run goes on there, and `agentry runhost attach %s %s` collects it", rec.Name, err, rec.Name, j.ID))
		}
		defer func() { _ = c2.Close() }()
		c = c2
	}
	env, ferr := remote.Finish(ctx, c, rec, keys, lj, res.ExitCode, remote.FinishOptions{
		Home: home,
		OutDir: func(runID string) (string, error) {
			return resolveOutDir(req.OutDir, req.OutDirIsRoot, sig, runID)
		},
		Warn:        say,
		CollectHint: fmt.Sprintf(": agentry runhost collect %s %s", rec.Name, j.ID),
	})
	if env == nil {
		return nil, ranOn, ferr
	}
	return resultFromRunhost(env), ranOn, nil
}

// resultFromRunhost is a runhost run's result: its envelope, with no plan of
// this computer's (the plan was the runhost's), and Unsafe Mode as the
// runhost's own setting was when the job was sent.
func resultFromRunhost(env *receipt.Envelope) *Result {
	res := resultFromEnvelope(env, &spec.Plan{})
	res.Unsafe = env.Runhost != nil && env.Runhost.UnsafeMode
	return res
}

// RunhostCannotCarry names what of m a run on a runhost from this app does
// not carry yet, "" when it carries all of it.
func RunhostCannotCarry(m *manifest.Manifest) string {
	for _, s := range m.Capabilities.MCPServers {
		if s.NeedsConnection() {
			return "a connection (an MCP server that signs in)"
		}
	}
	if len(m.Capabilities.TargetHosts) > 0 {
		return "a target host"
	}
	for _, q := range m.Intake {
		switch q.Type {
		case manifest.IntakeFile:
			return "a file answer"
		case manifest.IntakeHost:
			return "a target host"
		case manifest.IntakeRepository:
			return "a repository input"
		}
	}
	return ""
}

// runhostCarry is what the job carries of the run beside its release and
// files: the intake's answers, the secrets the release and its servers
// declare, from the saved MCP profile and the secret answers, then the
// policy's source, and the person's lists of target hosts.
func runhostCarry(home string, sig *Signature, m *manifest.Manifest, pol *policy.Policy, req RunhostRequest) (job.Run, error) {
	var run job.Run
	overrides := map[string]string{}
	prof, err := profile.Load(profile.Dir(home, sig.Namespace, sig.Name, sig.Version, sig.DigestSuffix()))
	if err != nil {
		return run, err
	}
	if prof != nil {
		for k, v := range prof.Secrets {
			overrides[k] = v
		}
	}
	for _, q := range m.Intake {
		a, ok := req.Intake[q.ID]
		if !ok || a == "" {
			continue
		}
		if q.Type == manifest.IntakeSecret {
			overrides[intake.SecretName(q.ID)] = a
			continue
		}
		if run.Intake == nil {
			run.Intake = map[string]string{}
		}
		run.Intake[q.ID] = a
	}
	for _, name := range declaredSecrets(m) {
		v, ok := engine.ResolveSecret(name, overrides, pol.SecretSource)
		switch {
		case ok:
			if !job.SecretValueOK(v) {
				return run, engine.Classify(engine.ExitPolicy, fmt.Errorf("the secret %s is not one line with no space at either end, which a run on a runhost carries as it is", name))
			}
			if run.Secrets == nil {
				run.Secrets = map[string]string{}
			}
			run.Secrets[name] = v
		case m.Capabilities.IsOptionalSecret(name):
		default:
			return run, engine.Classify(engine.ExitPolicy, fmt.Errorf("%s needs the secret %s, and this computer has none: give it in the run's setup", sig.Ref, name))
		}
	}
	run.HostsAllow, run.HostsDeny = pol.Hosts.Allow, pol.Hosts.Deny
	return run, nil
}

// declaredSecrets are the secrets the release and its servers declare.
func declaredSecrets(m *manifest.Manifest) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, n := range m.Capabilities.Secrets {
		add(n)
	}
	for _, s := range m.Capabilities.MCPServers {
		for _, n := range s.Secrets {
			add(n)
		}
	}
	return out
}

// sameRunhostVersion refuses a runhost whose CLI is not the one this app
// carries: the run's plan, policy and envelope are that version's.
func sameRunhostVersion(ctx context.Context, c *runhost.Client, name, want string) error {
	if want == "" {
		return errors.New("this build of Agentry carries no released CLI, so it cannot run on a runhost: use a release of the app")
	}
	a, err := c.Ask(ctx, "version", nil, nil)
	if err != nil {
		return fmt.Errorf("ask %s its version: %w", name, err)
	}
	var v remote.Version
	if a.Code != 0 || json.Unmarshal(a.Result, &v) != nil {
		return fmt.Errorf("%s did not say its version", name)
	}
	if v.Version != want {
		return fmt.Errorf("%s runs CLI %s and this app %s; a run needs the same version on both: update the runhost from Settings → Runhosts", name, v.Version, want)
	}
	return nil
}

// lineWriter hands each line written to say.
type lineWriter func(string)

func (l lineWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			l(line)
		}
	}
	return len(p), nil
}
