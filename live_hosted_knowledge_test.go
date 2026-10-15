package main

// Create knowledge In Agentry live: the bound methods driven as the
// Knowledge panel drives them, against a real registry that builds knowledge
// (the runs extension's knowledge_build), with the folder picker answered by
// the test. Skipped unless both variables are set, because it spends the
// account's credits (a build holds 100 and settles a few):
//
//	AGENTRY_LIVE_HOSTED_KNOWLEDGE_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_HOSTED_KNOWLEDGE_TOKEN=<a PAT with run:read, run:write, knowledge:read, knowledge:write, user:profile> \
//	go test -run TestLiveHostedKnowledge -v -timeout 30m .
//
// It builds a pack from a folder of three notes In Agentry, follows the
// build to its end, waits for Agentry's scan, downloads the pack into a
// fresh runner home, lists it as the panel does with its scan, and deletes
// it here and in Agentry; nothing of it is left.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/registry"
)

func TestLiveHostedKnowledge(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_HOSTED_KNOWLEDGE_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_HOSTED_KNOWLEDGE_TOKEN")
	if reg == "" || token == "" {
		t.Skip("set AGENTRY_LIVE_HOSTED_KNOWLEDGE_REGISTRY and _TOKEN to run the live check of a build In Agentry")
	}
	name := "live-hosted-" + time.Now().UTC().Format("20060102-150405")
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
	t.Logf("availability: offered %v, available %v (%s %s); holds %d with the scan's %d; caps %ds/%d turns; balance %d (known %v)",
		avail.Offered, avail.Available, avail.Reason, avail.Remedy, avail.HoldCredits, avail.ScanCredits, avail.TimeoutSeconds, avail.MaxTurns, avail.Balance, avail.BalanceKnown)
	if !avail.Available {
		t.Fatalf("a build In Agentry is not available here: %s", avail.Reason)
	}

	dir := t.TempDir()
	for file, text := range map[string]string{
		"backups.md":   "# Backups\n\nThe database is backed up every night at 02:00 UTC to the object store and kept for 14 days.\nA restore is tested on the first Monday of each month.\n",
		"oncall.md":    "# On-call\n\nOne engineer is on call each week, Monday 09:00 to Monday 09:00.\nPages go to the on-call phone first, then to the team lead after 15 minutes.\n",
		"releases.txt": "Releases ship on Tuesdays and Thursdays. A release needs a green CI run and one reviewer.\nHotfixes may ship any day once the on-call engineer agrees.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app.markPicked(dir)
	start, err := k.StartHostedBuild(reg, name, "", []string{dir}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if start.Build == nil {
		t.Fatalf("the registry refused the build: %s %s", start.Refusal, start.Detail)
	}
	t.Logf("build %s created for %s, holding %d", start.Build.ID, start.Build.Name, start.Build.HoldCredits)
	t.Cleanup(func() {
		if err := env.Delete(context.Background(), reg, name, true); err != nil && !strings.Contains(err.Error(), "not") {
			t.Logf("cleanup: %v", err)
		}
	})

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
		final.State, final.OutcomeLine, final.Applied, final.Pages, final.Bytes, final.Note, final.HoldCredits, final.SettledCredits, final.ScanCredits,
		final.Turns)
	t.Logf("kept: downloaded %v %q; error %q; scan %q (%s)", final.Downloaded, final.Kept, final.DownloadError, final.Scan, final.ScanState)
	if final.Status != "finished" || !final.Applied || !final.Downloaded || final.ScanState != knowledge.ScanScanned {
		t.Fatalf("the build did not land a scanned pack here: %+v", final)
	}

	list, err := k.List(reg)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range list.Packs {
		if p.Name == name {
			found = true
			t.Logf("the panel lists %s: %d pages, where %q, scan %q (%s), key %q", p.Name, p.Pages, p.Where, p.Scan, p.ScanState, p.Key)
			if !p.Local || !p.InAgentry || p.ScanState != knowledge.ScanScanned {
				t.Fatalf("listed as %+v", p)
			}
		}
	}
	if !found {
		t.Fatalf("the panel does not list %s", name)
	}
	if err := env.Delete(context.Background(), reg, name, true); err != nil {
		t.Fatal(err)
	}
	after, err := k.List(reg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range after.Packs {
		if p.Name == name {
			t.Fatalf("%s is still listed after Delete everywhere: %+v", name, p)
		}
	}
}
