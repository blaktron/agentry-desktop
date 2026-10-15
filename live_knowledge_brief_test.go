package main

// Create knowledge from a brief alone, live: the bound methods driven as the
// Knowledge panel drives them, on this computer and In Agentry, with no
// document. Each is skipped unless its variables are set, because each
// spends a model's tokens or the account's credits.
//
// On this computer, against the runner home and policy AGENTRY_RUNNER_HOME
// names (docker must see it), the model key the policy names, and, with
// _WEB=1, the person's own Jev key and search key as the policy names them:
//
//	AGENTRY_LIVE_KNOWLEDGE_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_KNOWLEDGE_TOKEN=<a PAT with knowledge:read and knowledge:write> \
//	AGENTRY_LIVE_KNOWLEDGE_BRIEF=1 AGENTRY_LIVE_KNOWLEDGE_BRIEF_WEB=1 \
//	go test -run TestLiveKnowledgeBrief -v -timeout 70m .
//
// In Agentry, with a brief alone and Search the web on, as the step's
// default has it for a new pack with no documents:
//
//	AGENTRY_LIVE_HOSTED_KNOWLEDGE_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_HOSTED_KNOWLEDGE_TOKEN=<a PAT with run:read, run:write, knowledge:read, knowledge:write, user:profile> \
//	AGENTRY_LIVE_HOSTED_KNOWLEDGE_BRIEF=1 \
//	go test -run TestLiveHostedKnowledgeBrief -v -timeout 30m .
//
// Either deletes the pack it made, here and in Agentry.

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/registry"
)

const liveBrief = "How to look after a café's espresso machine, for new staff: daily and weekly cleaning, backflushing, descaling, " +
	"grind size and dose, and what to do when a shot runs too fast or too slow."

func TestLiveKnowledgeBrief(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_KNOWLEDGE_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_KNOWLEDGE_TOKEN")
	if reg == "" || token == "" || os.Getenv("AGENTRY_LIVE_KNOWLEDGE_BRIEF") == "" {
		t.Skip("set AGENTRY_LIVE_KNOWLEDGE_REGISTRY, _TOKEN and _BRIEF=1 to run the live check of a brief-only build on this computer")
	}
	web := os.Getenv("AGENTRY_LIVE_KNOWLEDGE_BRIEF_WEB") != ""
	name := "live-brief-" + time.Now().UTC().Format("20060102-150405")
	var mu sync.Mutex
	var log []string
	emit := emitEvent
	emitEvent = func(_ *App, ev string, data ...any) {
		if ev != eventKnowledgeLog || len(data) == 0 {
			return
		}
		if s, ok := data[0].(string); ok {
			mu.Lock()
			log = append(log, s)
			mu.Unlock()
		}
	}
	t.Cleanup(func() { emitEvent = emit })

	env := knowledge.Env{
		Home: policy.Home, Keys: store.OSKeychain{},
		TokenFor: func(u string) string { return map[bool]string{true: token}[strings.TrimRight(u, "/") == reg] },
		HTTP:     registry.HTTPClient,
	}
	app := &App{ctx: context.Background()}
	k := &KnowledgeAPI{app: app, run: &RunAPI{app: app, knowledgeEnv: &env}}

	status, err := k.CreatorWeb()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Search the web on this computer: ready %v (%s) %s", status.Ready, status.Provider, status.Why)
	res, err := k.Create(reg, name, nil, liveBrief, web)
	mu.Lock()
	t.Logf("build log:\n%s", strings.Join(log, "\n"))
	mu.Unlock()
	if err != nil {
		t.Fatalf("Create knowledge from a brief: %v", err)
	}
	t.Cleanup(func() { _ = env.Delete(context.Background(), reg, name, true) })
	t.Logf("made %s: %s, %d bytes; read %v; searched %v, without the web %q; stopped %q; the Creator used %d in, %d out tokens",
		res.Name, kn.CountPages(res.Pages), res.Bytes, res.Documents, res.Searched, res.WithoutWeb, res.Stopped, res.InputTokens, res.OutputTokens)
	if res.Pages == 0 || !hasString(res.Documents, "brief.md") {
		t.Fatalf("the pack: %+v", res)
	}
	if web && status.Ready && !res.Searched {
		t.Fatalf("Search the web was asked for and this computer has the keys, and the Creator was not given the web: %+v", res)
	}
	if web && !status.Ready && res.WithoutWeb == "" {
		t.Fatalf("Search the web was asked for without the keys, and the result does not say why it ran without: %+v", res)
	}
	p, err := k.Preview(reg, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("preview: %d pages", len(p.Pages))
}

func TestLiveHostedKnowledgeBrief(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_HOSTED_KNOWLEDGE_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_HOSTED_KNOWLEDGE_TOKEN")
	if reg == "" || token == "" || os.Getenv("AGENTRY_LIVE_HOSTED_KNOWLEDGE_BRIEF") == "" {
		t.Skip("set AGENTRY_LIVE_HOSTED_KNOWLEDGE_REGISTRY, _TOKEN and _BRIEF=1 to run the live check of a brief-only build In Agentry")
	}
	name := "live-hosted-brief-" + time.Now().UTC().Format("20060102-150405")
	var mu sync.Mutex
	var views []*KnowledgeHostedBuildView
	emit := emitEvent
	emitEvent = func(_ *App, ev string, data ...any) {
		if ev != eventKnowledgeHosted || len(data) != 1 {
			return
		}
		if v, ok := data[0].(*KnowledgeHostedBuildView); ok {
			mu.Lock()
			views = append(views, v)
			mu.Unlock()
			t.Logf("build: %s %s %s", v.Status, v.State, v.StatusLine)
		}
	}
	t.Cleanup(func() { emitEvent = emit })

	home := t.TempDir()
	env := knowledge.Env{
		Home:     func() (string, error) { return home, nil },
		Keys:     &knowledgeKeys{m: map[string]string{}},
		TokenFor: func(u string) string { return map[bool]string{true: token}[strings.TrimRight(u, "/") == reg] },
		HTTP:     registry.HTTPClient,
	}
	app := &App{ctx: context.Background()}
	k := &KnowledgeAPI{app: app, run: &RunAPI{app: app, knowledgeEnv: &env}}

	avail, err := k.HostedAvailability(reg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("availability: available %v (%s); holds %d; brief %v up to %d bytes; web %v: %s",
		avail.Available, avail.Reason, avail.HoldCredits, avail.TakesBrief, avail.MaxBriefBytes, avail.Web, avail.WebNote)
	if !avail.Available || !avail.TakesBrief || !avail.Web {
		t.Fatalf("a brief-only build that searches the web is not offered here: %+v", avail)
	}
	start, err := k.StartHostedBuild(reg, name, "", nil, liveBrief, true)
	if err != nil {
		t.Fatal(err)
	}
	if start.Build == nil {
		t.Fatalf("the registry refused the build: %s %s", start.Refusal, start.Detail)
	}
	t.Logf("build %s created for %s, holding %d, brief %q", start.Build.ID, start.Build.Name, start.Build.HoldCredits, start.Build.Brief)
	t.Cleanup(func() {
		if err := env.Delete(context.Background(), reg, name, true); err != nil && !strings.Contains(err.Error(), "not") {
			t.Logf("cleanup: %v", err)
		}
	})
	if start.Build.Brief != liveBrief || len(start.Build.Documents) != 0 {
		t.Fatalf("the build's record: brief %q, documents %v", start.Build.Brief, start.Build.Documents)
	}

	var final *KnowledgeHostedBuildView
	deadline := time.Now().Add(25 * time.Minute)
	for final == nil && time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		mu.Lock()
		for _, v := range views {
			if v.Final {
				final = v
			}
		}
		mu.Unlock()
	}
	if final == nil {
		t.Fatal("the build did not end within 25 minutes")
	}
	t.Logf("final: %s (%s); applied %v, %d pages, %d bytes; note %q; %d held, %d settled (scan %d); %d turns",
		final.State, final.OutcomeLine, final.Applied, final.Pages, final.Bytes, final.Note, final.HoldCredits, final.SettledCredits, final.ScanCredits, final.Turns)
	t.Logf("web: %s", final.WebLine)
	t.Logf("kept: downloaded %v %q; error %q; scan %q (%s)", final.Downloaded, final.Kept, final.DownloadError, final.Scan, final.ScanState)
	if final.Status != "finished" || !final.Applied || !final.Downloaded || final.ScanState != knowledge.ScanScanned || final.WebLine == "" {
		t.Fatalf("the build did not land a scanned pack here, with its searches recorded: %+v", final)
	}
}
