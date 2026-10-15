package main

// A release that ships a knowledge pack, live: installed to the runner home
// through the bound install methods, its consent and record naming the
// shipped pack, then run on this computer, the receipt naming the pack as
// the release's. Skipped unless the three variables are set; it needs docker
// and the operator policy's model key, and it uses this computer's runner
// home.
//
//	AGENTRY_LIVE_SHIPPED_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_SHIPPED_TOKEN=… \
//	AGENTRY_LIVE_SHIPPED_REF=chris/m9-ships-check@0.2.0 \
//	go test -run TestLiveShippedKnowledge -v .
//
// The install record is written to a config directory of the test's own.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveShippedKnowledge(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_SHIPPED_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_SHIPPED_TOKEN")
	ref := os.Getenv("AGENTRY_LIVE_SHIPPED_REF")
	if reg == "" || token == "" || ref == "" {
		t.Skip("set AGENTRY_LIVE_SHIPPED_REGISTRY, _TOKEN and _REF to run a release that ships a pack")
	}
	// The token under test and no other, and a record of the test's own.
	keyring.MockInit()
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	s, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Upsert(settings.Registry{URL: settings.Normalize(reg), Token: token})
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}

	var dialogs []string
	r, events := newRunAPI(t)
	r.app.confirm = func(_ context.Context, title, message string) (bool, error) {
		dialogs = append(dialogs, title+"\n"+message)
		return true, nil
	}
	i := &InstallAPI{app: r.app, engine: install.NewEngine()}

	// The install: the contract and the consent name the shipped pack.
	plan, err := i.Prepare(reg, ref, installdir.Target, "")
	if err != nil {
		t.Fatalf("prepare %s: %v", ref, err)
	}
	t.Logf("plan: contract %+v; shipped %+v; consent text:\n%s", plan.Capabilities, plan.ShippedKnowledge, contractConsentText(plan, nil))
	if len(plan.Capabilities.Knowledge) == 0 || len(plan.ShippedKnowledge) != len(plan.Capabilities.Knowledge) || plan.ShippedKnowledge[0].Bytes == nil {
		t.Fatalf("the plan does not carry the shipped knowledge: %+v / %+v", plan.Capabilities, plan.ShippedKnowledge)
	}
	pack := plan.Capabilities.Knowledge[0]
	if _, err := i.Confirm(plan.ID, false, false, false); err != nil {
		t.Fatalf("install: %v (dialogs %q)", err, dialogs)
	}
	state, err := installed.Load()
	if err != nil {
		t.Fatal(err)
	}
	var recorded bool
	for _, a := range state.Records {
		if strings.HasPrefix(ref, a.Namespace+"/"+a.Name+"@") && slices.Contains(a.Capabilities.Knowledge, pack) {
			recorded = true
		}
	}
	if !recorded {
		t.Fatalf("installed.yaml does not record %s in the contract: %+v", pack, state.Records)
	}

	// The run: the pack mounted from the release, named as the release's.
	outDir := t.TempDir()
	r.app.markPicked(outDir)
	run, err := r.RunAgent(reg, ref, "What is an EC2 instance, according to your knowledge pack? Name the page you read.",
		nil, outDir, nil, 0, -1, false, nil, false, false, nil, false, nil, nil)
	if err != nil {
		t.Fatalf("run %s: %v\n%s", ref, err, strings.Join(events.lines(), "\n"))
	}
	t.Logf("run %s: status %s, exit %d\nresult: %s\nknowledge: %s", run.RunID, run.Status, run.ExitCode, run.ResultJSON, run.Knowledge)
	if run.Status != "ok" || !strings.Contains(run.Knowledge, pack+" shipped with the release (archive sha256:") {
		t.Fatalf("the run: %+v", run)
	}
}
