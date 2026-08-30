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

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/trust"

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
	if _, err := eng.Confirm(plan.ID, false, accept); err != nil {
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
