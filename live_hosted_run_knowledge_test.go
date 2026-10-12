package main

// A run In Agentry with a knowledge pack attached, live (agentry-desktop#275;
// agentry-enterprise#172): the bound methods driven as the run panel drives
// them, against a real registry that attaches knowledge to a hosted run,
// with the output folder picker answered by the test. Skipped unless the
// variables are set, because the run spends the account's credits:
//
//	AGENTRY_LIVE_HOSTED_RUN_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_HOSTED_RUN_TOKEN=<a PAT with run:read, run:write, knowledge:read, user:profile> \
//	AGENTRY_LIVE_HOSTED_RUN_REF=smoke98944/hello-dev@0.1.0 \
//	AGENTRY_LIVE_HOSTED_RUN_PACK=m6-dev-runbook \
//	AGENTRY_LIVE_HOSTED_RUN_TASK="…" \
//	go test -run TestLiveHostedRunWithKnowledge -v -timeout 20m .
//
// The pack must be one the account keeps in Agentry, scanned there. The
// test checks that the availability offers attaching, that the Knowledge
// list offers the pack for a run In Agentry, that the run it starts is
// handed the pack, and that the result carries the receipt's knowledge
// line; it keeps nothing outside its temp folders.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveHostedRunWithKnowledge(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_HOSTED_RUN_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_HOSTED_RUN_TOKEN")
	ref := os.Getenv("AGENTRY_LIVE_HOSTED_RUN_REF")
	pack := os.Getenv("AGENTRY_LIVE_HOSTED_RUN_PACK")
	if reg == "" || token == "" || ref == "" || pack == "" {
		t.Skip("set AGENTRY_LIVE_HOSTED_RUN_REGISTRY, _TOKEN, _REF and _PACK to run one hosted run with a pack attached")
	}
	task := os.Getenv("AGENTRY_LIVE_HOSTED_RUN_TASK")
	if task == "" {
		task = "Answer from the knowledge you were given, citing where you read it."
	}
	// The token under test, and no other: this machine's own keychain may
	// hold a desktop token for the same registry.
	keyring.MockInit()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	s, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Upsert(settings.Registry{URL: settings.Normalize(reg), Token: token})
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	r, events := newRunAPI(t)

	avail, err := r.HostedAvailability(reg, ref)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("availability: offered %v, available %v (%s); knowledge attach %v, at most %d (%s)", avail.Offered, avail.Available, avail.Reason, avail.KnowledgeAttach, avail.MaxPacks, avail.KnowledgeNote)
	if !avail.Available || !avail.KnowledgeAttach {
		t.Fatalf("a run In Agentry with knowledge is not available here: %+v", avail)
	}
	list, err := r.knowledge().List(r.ctx(), reg)
	if err != nil {
		t.Fatal(err)
	}
	offered := false
	for _, p := range list.Packs {
		t.Logf("pack %s: where %q, in Agentry %v, ready there %v, scan %q", p.Name, p.Where, p.InAgentry, p.AgentryReady, p.Scan)
		offered = offered || (p.Name == pack && p.InAgentry && p.AgentryReady)
	}
	if !offered {
		t.Fatalf("the Knowledge list does not offer %s for a run In Agentry", pack)
	}

	outDir := t.TempDir()
	r.app.markPicked(outDir)
	start, err := r.StartHostedRun(reg, ref, task, nil, outDir, nil, []string{pack}, false, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if start.Run == nil {
		t.Fatalf("the registry refused the run: %s %s", start.Refusal, start.Detail)
	}
	t.Logf("run %s queued, holding %d, attaching %+v", start.Run.ID, start.Run.HoldCredits, start.Run.Knowledge)
	if len(start.Run.Knowledge) != 1 || start.Run.Knowledge[0].Name != pack {
		t.Fatalf("the created run attaches %+v", start.Run.Knowledge)
	}

	var final *HostedRunView
	deadline := time.Now().Add(15 * time.Minute)
	for final == nil && time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		events.mu.Lock()
		for _, v := range events.runs {
			if v.Final {
				final = v
			}
		}
		events.mu.Unlock()
	}
	if final == nil {
		t.Fatal("the run did not end within 15 minutes")
	}
	t.Logf("final: %s (%s); %d held, %d settled; %d outputs, error %q", final.State, final.OutcomeLine, final.HoldCredits, final.SettledCredits, len(final.Outputs), final.DownloadError)
	t.Logf("knowledge: %+v", final.Knowledge)
	t.Logf("knowledge line: %s", final.KnowledgeLine)
	if final.Outcome != "succeeded" || len(final.Knowledge) != 1 || final.Knowledge[0].Digest == "" ||
		!strings.Contains(final.KnowledgeLine, pack+" mounted read-only at /knowledge/"+pack) {
		t.Fatalf("the run did not come back with its pack: %+v", final)
	}
	for _, a := range final.Outputs {
		if strings.HasPrefix(a.Name, "reports/") {
			b, err := os.ReadFile(a.Path)
			if err == nil {
				t.Logf("%s:\n%s", a.Name, b)
			}
		}
	}
}
