package knowledge

// A build on this computer live (agentry-desktop#352): the real Creator,
// this machine's docker and the model a runner policy names, on a runner
// home of the test's own, with its progress recorded as the panel sees it.
// Skipped unless AGENTRY_LIVE_BUILD_POLICY names that policy file; the
// model key its apiKeyEnv names must be in the environment:
//
//	set -a; . ~/.config/agentry/llm.env; set +a
//	AGENTRY_LIVE_BUILD_POLICY=$HOME/.agentry/runner/policy.yaml \
//	  go test -run TestLiveBuildProgress -v -timeout 40m ./internal/knowledge/

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/knowledge/creator"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

func TestLiveBuildProgress(t *testing.T) {
	pol := os.Getenv("AGENTRY_LIVE_BUILD_POLICY")
	if pol == "" {
		t.Skip("set AGENTRY_LIVE_BUILD_POLICY to run a live build")
	}
	raw, err := os.ReadFile(pol)
	if err != nil {
		t.Fatal(err)
	}
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	home := homeOf(t, e)
	if err := os.WriteFile(filepath.Join(home, "policy.yaml"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(policy.HomeEnv, home)
	docs := t.TempDir()
	for name, body := range map[string]string{
		"depots.md":   "# Depots\n\nThe Rotterdam depot opens at 06:00 and closes at 22:00. Its cold room holds 40 pallets at 2 to 8 °C.\nThe Antwerp depot opens at 07:00 and has no cold room.\n",
		"alarms.md":   "# Cold-chain alarm\n\nWhen the Rotterdam cold room passes 8 °C for ten minutes, the shift lead moves the pallets to the backup room and calls the facilities desk on extension 4410.\n",
		"carriers.md": "# Carriers\n\nNordfreight carries chilled goods between Rotterdam and Hamburg on Tuesdays and Fridays. Van Dijk carries dry goods daily.\n",
	} {
		if err := os.WriteFile(filepath.Join(docs, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var mu sync.Mutex
	var got []Progress
	var log strings.Builder
	res, err := e.Create(context.Background(), BuildRequest{
		Registry: reg.URL, Name: "depots", Documents: []string{docs}, RunID: "live-352", Log: &log,
		Progress: func(p Progress) {
			mu.Lock()
			got = append(got, p)
			mu.Unlock()
			t.Logf("progress: turns=%d pages=%d input=%d output=%d timeout=%ds", p.Turns, p.Pages, p.InputTokens, p.OutputTokens, p.TimeoutSeconds)
		},
	})
	t.Logf("log:\n%s", log.String())
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Logf("result: %d pages, %d bytes, %d input and %d output tokens, stopped=%q", res.Pages, res.Bytes, res.InputTokens, res.OutputTokens, res.Stopped)
	if len(got) < 2 || got[0].TimeoutSeconds < creator.LocalTimeoutFloorSeconds {
		t.Fatalf("progress = %+v", got)
	}
	var sawPages, sawTurns bool
	for _, p := range got {
		sawPages = sawPages || p.Pages > 0
		sawTurns = sawTurns || p.Turns > 0
	}
	if !sawPages || !sawTurns || res.Pages == 0 {
		t.Fatalf("no pages or turns reported: %+v, result %+v", got, res)
	}
}

// A build on this computer past Agentry's 10 MB, live (agentry-desktop#354):
// one short note the Creator reads and an 11 MB file no decoder reads. The
// old cap refused the documents before anything ran; now the build runs,
// warns first, and makes the pack. Same variables as TestLiveBuildProgress.
func TestLiveBuildPastAgentrysCap(t *testing.T) {
	pol := os.Getenv("AGENTRY_LIVE_BUILD_POLICY")
	if pol == "" {
		t.Skip("set AGENTRY_LIVE_BUILD_POLICY to run a live build")
	}
	raw, err := os.ReadFile(pol)
	if err != nil {
		t.Fatal(err)
	}
	reg := knowledgetest.NewRegistry(t)
	e := testEnv(t, reg, reg.Token("ana"), newKeys())
	home := homeOf(t, e)
	if err := os.WriteFile(filepath.Join(home, "policy.yaml"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(policy.HomeEnv, home)
	docs := t.TempDir()
	if err := os.WriteFile(filepath.Join(docs, "depots.md"), []byte("# Depots\n\nThe Rotterdam depot opens at 06:00 and closes at 22:00. Its cold room holds 40 pallets.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docs, "scan.bin"), bytes.Repeat([]byte{0, 1, 2, 3}, (11<<20)/4), 0o600); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	res, err := e.Create(context.Background(), BuildRequest{Registry: reg.URL, Name: "big-docs", Documents: []string{docs}, RunID: "live-354", Log: &log})
	t.Logf("log:\n%s", log.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("result: %d pages, %d bytes, skipped %v", res.Pages, res.Bytes, res.Skipped)
	if !strings.Contains(log.String(), "can take a very long time") || res.Pages == 0 {
		t.Fatalf("result %+v", res)
	}
}
