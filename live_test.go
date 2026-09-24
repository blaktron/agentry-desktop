package main

// Live end-to-end verification against a real registry — the §10 checklist
// as an executable. Skipped unless AGENTRY_LIVE_REGISTRY is set, because it
// needs a running stack (the dev compose stack, usually http://agentry:8080)
// and it reads/writes the REAL ~/.config/agentry install state on purpose:
// shared state with the CLI is one of the claims under test.
//
//	AGENTRY_LIVE_REGISTRY=http://agentry:8080 \
//	AGENTRY_LIVE_CLI=../agentry-cli/bin/agentry \
//	AGENTRY_LIVE_EXAMPLE=../agentry/examples/injection-defense \
//	go test -run TestLive -v .
//
// It cleans up the install records it creates.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/trust"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/validate"
)

func TestLiveEndToEnd(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_REGISTRY"), "/")
	if reg == "" {
		t.Skip("set AGENTRY_LIVE_REGISTRY (e.g. http://agentry:8080) to run the live E2E")
	}
	liveRef := os.Getenv("AGENTRY_LIVE_REF")
	if liveRef == "" {
		liveRef = "demo1/incident-summarizer"
	}
	liveNS, liveName, _ := strings.Cut(liveRef, "/")
	ctx := context.Background()

	// --- Browse: the demo agent is findable -----------------------------
	client, err := registry.New(reg, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := registry.Resolve(ctx, client, liveRef, false)
	if err != nil {
		t.Fatalf("resolve demo1/incident-summarizer: %v", err)
	}
	t.Logf("resolved %s@%s digest %s", liveRef, res.Version, res.Digest)

	// --- Report: scanned, with a ruleset version ------------------------
	rresp, err := client.GetVersionReportWithResponse(ctx, liveNS, liveName, res.Version)
	if err != nil || rresp.JSON200 == nil {
		t.Fatalf("report: %v (HTTP %d)", err, rresp.StatusCode())
	}
	report := rresp.JSON200
	if report.RulesetVersion == nil || *report.RulesetVersion == "" {
		t.Error("report has no ruleset version")
	} else {
		t.Logf("report: status=%s ruleset=%s findings=%d", report.VersionStatus, *report.RulesetVersion, len(report.Findings))
	}

	// --- Provenance: DSSE verification against the declared digest ------
	att := trust.VerifyAttestation(ctx, registry.HTTPClient, reg, "",
		fmt.Sprintf("/api/v1/agents/%s/%s/versions/%s/attestation", liveNS, liveName, res.Version), res.Digest)
	if att.State != trust.StateVerified {
		t.Fatalf("provenance: %s (%s)", att.State, att.Detail)
	}
	t.Logf("provenance: verified, key %s, ruleset %s (TOFU firstUse=%v)", att.KeyID, att.RulesetVer, att.FirstUse)

	// --- Install via the desktop engine ---------------------------------
	destParent := t.TempDir()
	eng := install.NewEngine()
	plan, err := eng.Prepare(ctx, reg, liveRef, "claude-skill", destParent)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if plan.Attestation.State != trust.StateVerified {
		t.Fatalf("install-time attestation: %s (%s)", plan.Attestation.State, plan.Attestation.Detail)
	}
	key := installed.Key{Registry: reg, Namespace: plan.Namespace, Name: plan.Name, Target: plan.Target, Dir: plan.Paths.Dest}
	defer func() { // leave the machine as found even on failure
		if msg, err := install.Uninstall(key); err == nil {
			t.Logf("cleanup: %s", msg)
		}
	}()
	accept := plan.NeedsCapabilityConfirm || plan.RequiresWidenedConfirm
	if _, err := eng.Confirm(plan.ID, false, accept, false); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	skill := filepath.Join(plan.Paths.Dest, "SKILL.md")
	if _, err := os.Stat(skill); err != nil {
		t.Fatalf("exported skill missing: %v", err)
	}
	st, err := installed.Load()
	if err != nil {
		t.Fatal(err)
	}
	if st.Find(key) == nil {
		t.Fatalf("desktop install not recorded in installed.yaml: %+v", st.Records)
	}
	t.Logf("installed to %s and recorded in the shared installed.yaml", plan.Paths.Dest)

	// --- Shared-state proof: a CLI install lands in the SAME file -------
	if cli := os.Getenv("AGENTRY_LIVE_CLI"); cli != "" {
		cliDest := t.TempDir()
		cmd := exec.Command(cli, "--registry", reg, "install", liveRef,
			"--target", "claude-skill", "--dir", cliDest, "--yes")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("CLI install failed: %v\n%s", err, out)
		}
		cliKey := installed.Key{Registry: reg, Namespace: liveNS, Name: liveName,
			Target: "claude-skill", Dir: filepath.Join(cliDest, liveName)}
		defer func() {
			if _, err := install.Uninstall(cliKey); err != nil {
				t.Logf("cleanup of CLI record: %v", err)
			}
		}()
		st2, err := installed.Load()
		if err != nil {
			t.Fatal(err)
		}
		if st2.Find(cliKey) == nil {
			t.Fatalf("CLI install not visible in the shared state: %+v", st2.Records)
		}
		if st2.Find(key) == nil {
			t.Fatal("desktop record lost after CLI install")
		}
		t.Log("shared-state proof: desktop and CLI records coexist in installed.yaml")
	} else {
		t.Log("AGENTRY_LIVE_CLI not set — skipping the CLI coexistence step")
	}

	// --- Validate: the benign fixture passes -----------------------------
	if example := os.Getenv("AGENTRY_LIVE_EXAMPLE"); example != "" {
		vres, err := validate.Run(example)
		if err != nil {
			t.Fatalf("validate %s: %v", example, err)
		}
		if !vres.Passed {
			t.Fatalf("benign fixture failed: manifestErr=%q issues=%+v inventory=%+v findings=%+v",
				vres.ManifestError, vres.Issues, vres.Inventory, vres.Findings)
		}
		t.Logf("validate: %s PASSED (%d files, bundle %s)", example, vres.FileCount, vres.BundleVersion)
	} else {
		t.Log("AGENTRY_LIVE_EXAMPLE not set — skipping the fixture validation step")
	}
}

// One In-Agentry run against a real registry with the runs extension
// (agentry-desktop#77, plan hosted-runs.md §4.7): the same path the panel
// takes — status, create, watch to a terminal state, download the
// artefacts into a chosen folder, read the receipt — with the app's own
// client and the account's PAT. Skipped unless the environment names a
// registry, a token and a release:
//
//	AGENTRY_HOSTED_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_HOSTED_TOKEN=agy_… \
//	AGENTRY_HOSTED_REF=runs-checker-125725/hosted-smoke@0.1.0 \
//	AGENTRY_HOSTED_TASK="say hello" \
//	AGENTRY_HOSTED_TIMEOUT=15m \
//	go test -run TestLiveHostedRun -v .
//
// The token needs run:read and run:write. Nothing is written outside the
// test's temp folder, and the run costs the account credits.
func TestLiveHostedRun(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_HOSTED_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_HOSTED_TOKEN")
	ref := os.Getenv("AGENTRY_HOSTED_REF")
	if reg == "" || token == "" || ref == "" {
		t.Skip("set AGENTRY_HOSTED_REGISTRY, AGENTRY_HOSTED_TOKEN and AGENTRY_HOSTED_REF to run one hosted run")
	}
	wait := 15 * time.Minute
	if v := os.Getenv("AGENTRY_HOSTED_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("AGENTRY_HOSTED_TIMEOUT: %v", err)
		}
		wait = d
	}
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	c := hostedrun.New(reg, token)

	st, err := c.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.Enabled {
		t.Fatalf("this registry is not taking hosted runs: %+v", st)
	}
	if st.Caller == nil {
		t.Fatal("the status route told this token nothing about its tier, caps or hold")
	}
	t.Logf("hosted runs: %d runners online (%d hosts, %d workers), %d queued; this account is %s, a run holds %d credits (%d tokens per credit), caps %ds/%d turns/%d tokens",
		st.RunnersOnline(), st.HostsOnline, st.WorkersOnline, st.Queued, st.Caller.Tier, st.Caller.HoldCredits, st.Caller.TokensPerCredit,
		st.Caller.Caps.TimeoutSeconds, st.Caller.Caps.MaxTurns, st.Caller.Caps.MaxTokens)

	task := os.Getenv("AGENTRY_HOSTED_TASK")
	if task == "" {
		task = "Write a one-line summary of the input you were given."
	}
	run, err := c.Create(ctx, hostedrun.CreateRequest{Ref: ref, Task: task})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Logf("run %s queued: %s, holding %d credits, queue position %d", run.ID, run.Ref, run.HoldCredits, run.QueuePosition)

	final, err := c.Watch(ctx, run.ID, hostedrun.WatchOptions{
		OnStatus: func(r *hostedrun.Run) {
			t.Logf("status: %s — %s%s", r.Status, hostedrun.Read(r).Line, map[bool]string{true: " (cancel requested)", false: ""}[r.CancelRequested])
		},
	})
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	verdict := hostedrun.Read(final)
	turns, wall := 0, int64(0)
	if final.Usage != nil {
		turns, wall = final.Usage.Turns, final.Usage.WallMS
	}
	t.Logf("finished: %s (%s) — %s; %d turns, %d tokens, %d ms",
		final.Outcome, final.ExitClass, verdict.Line, turns, final.Usage.Tokens(), wall)
	if final.SettledCredits != nil {
		t.Logf("credits: %d held, %d settled", final.HoldCredits, *final.SettledCredits)
	}
	if final.Status != hostedrun.StatusFinished {
		t.Fatalf("run ended as %s, not finished: %+v", final.Status, final)
	}

	dir := t.TempDir()
	got, err := c.DownloadOutputs(ctx, final, dir)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	for _, a := range got {
		t.Logf("artefact %s (%d bytes) → %s", a.Name, a.Bytes, a.Path)
		if !strings.HasPrefix(a.Path, dir) {
			t.Fatalf("%s landed outside the run folder", a.Name)
		}
	}
	if final.Outcome == hostedrun.OutcomeSucceeded && len(got) == 0 {
		t.Fatal("a succeeded run wrote no artefacts")
	}
	// A pushed run's receipt is the envelope in its outputs (the registry
	// keeps none): read it from result.json, as the panel does.
	if env, err := os.ReadFile(filepath.Join(dir, "result.json")); err == nil {
		hostedrun.ReceiptFromEnvelope(final, env)
	}
	t.Logf("attestation %s, adapter %s", final.Receipt.Attestation.State, final.Receipt.Harness.Adapter)
	if len(final.Receipt.CapabilitiesEnforced) == 0 {
		t.Log("the run reported no capabilities_enforced — check the receipt on the run")
	}
	for k, v := range final.Receipt.CapabilitiesEnforced {
		t.Logf("enforced %s: %s", k, v)
	}
}
