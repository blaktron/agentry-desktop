package main

// A run In Agentry reaches the page in credits, time and turns only, and a
// run on this computer in tokens: what Go sends the webview for a hosted
// run, session, build or draft carries no token count, no token cap, no rate
// and no dollar cost, whether the registry serves them or not; what it sends
// for the local run, session, build and draft keeps its tokens.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/progress"
)

// hostedEnvelopeWithUsage is the CLI's run/v1 envelope as a sandbox host
// keeps it in the sealed output: with its usage block, tokens and all.
const hostedEnvelopeWithUsage = `{"agentry":"1","status":"ok","exitClass":"ok","attestation":{"state":"verified","keyId":"k1"},` +
	`"capabilities_enforced":{"network.egress":"llm-proxy allowlist: api.example.com"},"harness":{"adapter":"docker-agent","model":"m"},` +
	`"usage":{"inputTokens":800,"outputTokens":200,"totalTokens":1000,"costUsd":0.0182,"turns":3,"wallMs":9000},"result":"the summary"}`

var tokenKey = regexp.MustCompile(`(?i)token|costUsd`)

// creditFields are credits named after what they pay for, which are kept:
// they leak no count.
var creditFields = []string{"tokenCredits", "tokensUnpaid"}

// tokenKeys lists every JSON key in v, at any depth, that names a token
// count, a token cap, a rate or a dollar cost.
func tokenKeys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	var out []string
	var walk func(path string, n any)
	walk = func(path string, n any) {
		switch n := n.(type) {
		case map[string]any:
			for k, c := range n {
				if tokenKey.MatchString(k) && !slices.Contains(creditFields, k) {
					out = append(out, path+"."+k)
				}
				walk(path+"."+k, c)
			}
		case []any:
			for _, c := range n {
				walk(path+"[]", c)
			}
		}
	}
	walk("", doc)
	return out
}

func assertNoTokens(t *testing.T, what string, v any) {
	t.Helper()
	if keys := tokenKeys(t, v); len(keys) > 0 {
		t.Fatalf("%s carries %v to the webview", what, keys)
	}
}

// A run In Agentry, start to finish, against today's routes and against
// routes that no longer serve the counts: the availability, every state
// the follow loop reports, and the finished result carry no tokens, and
// read the same turns, time, caps and credits either way.
func TestARunInAgentryReachesThePageInCreditsOnly(t *testing.T) {
	for _, gone := range []bool{false, true} {
		name := map[bool]string{false: "today's routes", true: "routes without the counts"}[gone]
		t.Run(name, func(t *testing.T) {
			f := &hostedFake{enabled: true, tokensGone: gone, outputs: map[string]string{"result.json": hostedEnvelopeWithUsage}}
			reg := f.start(t)
			signIn(t, reg)
			r, events := newRunAPI(t)
			fastPoll(t)

			avail, err := r.HostedAvailability(reg, "acme/reporter@1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			assertNoTokens(t, "the availability", avail)
			if avail.HoldCredits != 20 || avail.Caps == nil || avail.Caps.TimeoutSeconds != 300 || avail.Caps.MaxTurns != 20 {
				t.Fatalf("the hold and caps: %+v", avail)
			}

			outDir := t.TempDir()
			r.app.markPicked(outDir)
			start, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, nil, false, false, nil)
			if err != nil || start.Run == nil {
				t.Fatalf("start = %+v, %v", start, err)
			}
			assertNoTokens(t, "the started run", start)
			f.set(hostedrun.StatusRunning)
			waitForStatus(t, events, hostedrun.StatusRunning)
			f.finish(hostedrun.OutcomeSucceeded, "ok", 2)
			final := waitForFinal(t, events)
			for _, v := range events.states() {
				assertNoTokens(t, "a reported state", v)
			}
			if final.Turns != 3 || final.WallMS != 9000 || !final.Settled || final.SettledCredits != 2 || final.HoldCredits != 20 {
				t.Fatalf("final = %+v", final)
			}
			if final.Caps == nil || final.Caps.TimeoutSeconds != 300 || final.Caps.MaxTurns != 20 {
				t.Fatalf("caps = %+v", final.Caps)
			}
			// The result the page renders is the CLI's envelope without its
			// token counts; the file in the run folder keeps them.
			var env map[string]any
			if err := json.Unmarshal([]byte(final.ResultJSON), &env); err != nil {
				t.Fatalf("result = %q: %v", final.ResultJSON, err)
			}
			assertNoTokens(t, "the result the page renders", env)
			if env["result"] != "the summary" || env["usage"].(map[string]any)["turns"] != float64(3) {
				t.Fatalf("result = %s", final.ResultJSON)
			}
			kept, err := os.ReadFile(filepath.Join(final.OutDir, "result.json"))
			if err != nil || !strings.Contains(string(kept), `"inputTokens":800`) {
				t.Fatalf("the run folder's result.json = %s, %v", kept, err)
			}
		})
	}
}

// A finished run that used tokens, read as a run, a knowledge build and a
// draft, a session's record, and a session's frame: none carries a count.
func TestHostedViewsCarryNoTokenCounts(t *testing.T) {
	now := time.Now().UTC()
	settled := int64(12)
	// The registry's answer today, with every count it serves.
	served := `{"id":"01a0","ref":"acme/reporter@1.0.0","status":"finished","outcome":"budget","exitClass":"budget",
		"holdCredits":20,"settledCredits":12,"caps":{"timeoutSeconds":900,"maxTurns":60,"maxTokens":400000},
		"usage":{"inputTokens":184210,"outputTokens":41088,"totalTokens":225298,"costUsd":0.1893,"wallMs":61000,"turns":14},
		"mode":"interactive","session":{"freeMinutes":10,"creditsPerMinute":50,"paidThrough":12,"timeCredits":100,"tokenCredits":7,"tokensUnpaid":2,"idleSeconds":300}}`
	var run hostedrun.Run
	if err := json.Unmarshal([]byte(served), &run); err != nil {
		t.Fatal(err)
	}
	run.FinishedAt, run.SettledCredits = &now, &settled

	v := hostedRunView(&run, "", nil, "", "", true)
	assertNoTokens(t, "a run", v)
	if v.Turns != 14 || v.WallMS != 61000 || v.Caps.MaxTurns != 60 || v.Session.TokenCredits != 7 || v.Session.TokensUnpaid != 2 {
		t.Fatalf("run = %+v (session %+v)", v, v.Session)
	}
	if v.OutcomeLine != "stopped at its turn or cost limit" {
		t.Fatalf("budget reads %q", v.OutcomeLine)
	}
	assertNoTokens(t, "a session's state", HostedSessionStateView{Status: run.Status, Session: sessionView(run.Session)})

	run.Kind = hostedrun.KindKnowledgeBuild
	run.Build = &hostedrun.KnowledgeBuild{Name: "runbook"}
	assertNoTokens(t, "a knowledge build", hostedBuildView(&run, true))

	run.Kind, run.Build = hostedrun.KindDraft, nil
	d := hostedDraftView(&run, true)
	assertNoTokens(t, "a draft", d)

	var frame hostedrun.FrameEvent
	if err := json.Unmarshal([]byte(`{"kind":"turn_end","time":"2026-10-02T10:00:00Z","stopReason":"end_turn","inputTokens":5120,"outputTokens":1430,"totalTokens":6550}`), &frame); err != nil {
		t.Fatal(err)
	}
	assertNoTokens(t, "a session frame", frameView(3, frame))
}

// The Profile card's hosted-run line: the hold, the time and the turns.
func TestProfileCarriesNoTokenCapOrRate(t *testing.T) {
	srv, _ := profileRegistry(t, true, true)
	p, err := profileFor(&App{ctx: t.Context()}, srv.URL, "agy_test")
	if err != nil {
		t.Fatal(err)
	}
	assertNoTokens(t, "the profile", p.HostedRuns)
	if p.HostedRuns == nil || p.HostedRuns.HoldCredits != 20 || p.HostedRuns.Caps.TimeoutSeconds != 900 {
		t.Fatalf("hosted runs = %+v", p.HostedRuns)
	}
}

// Every outcome of a run, a build and a draft In Agentry reads without
// tokens; budget is the turn or cost limit, as the site says it.
func TestHostedOutcomeLinesNameNoTokens(t *testing.T) {
	outcomes := []string{
		hostedrun.OutcomeSucceeded, hostedrun.OutcomeFailed, hostedrun.OutcomeRefused, hostedrun.OutcomeTimeout,
		hostedrun.OutcomeBudget, hostedrun.OutcomeInterrupted, hostedrun.OutcomeError,
	}
	tok := regexp.MustCompile(`(?i)token`)
	for _, o := range outcomes {
		r := &hostedrun.Run{Status: hostedrun.StatusFinished, Outcome: o}
		for what, line := range map[string]string{
			"run": hostedrun.Read(r).Line, "build": buildOutcomeLine(r), "draft": draftOutcomeLine(r),
		} {
			if tok.MatchString(line) {
				t.Fatalf("a %s's %s outcome names tokens: %q", what, o, line)
			}
		}
	}
	r := &hostedrun.Run{Status: hostedrun.StatusFinished, Outcome: hostedrun.OutcomeBudget}
	if got := draftOutcomeLine(r); got != "Stopped at its turn or cost limit. No draft came of it." {
		t.Fatalf("a draft's budget reads %q", got)
	}
	if got := buildOutcomeLine(r); got != "Stopped at its turn or cost limit." {
		t.Fatalf("a build's budget reads %q", got)
	}
	if s := hostedrun.DraftHoldSentence(40, hostedrun.DraftTerms{MaxSearches: 10, CreditsPerSearch: 1, AIReviewCredits: 1}); tok.MatchString(s) {
		t.Fatalf("a draft's hold names tokens: %q", s)
	}
}

// The other half: a run, a session, a knowledge build and a draft on this
// computer keep their tokens, and none of them carries a credit.
func TestLocalViewsKeepTheirTokens(t *testing.T) {
	credit := regexp.MustCompile(`(?i)credit`)
	for what, v := range map[string]any{
		"a local run's result":     RunResultView{InputTokens: 5120, OutputTokens: 1430, CostUSD: 0.0182, Turns: 4},
		"a local session's event":  SessionEventView{Kind: "usage", InputTokens: 5120, OutputTokens: 1430, CostUSD: 0.0182},
		"a local build's result":   knowledge.BuildResult{InputTokens: 184210, OutputTokens: 41088},
		"a local draft's progress": draft.Progress{Report: progress.Report{Turns: 7, InputTokens: 48210, OutputTokens: 3904}},
		"a local build's progress": knowledge.Progress{Report: progress.Report{Turns: 9, InputTokens: 101400, OutputTokens: 6120}},
		"a local run's progress":   RunProgress{Report: progress.Report{Turns: 4, InputTokens: 5120, OutputTokens: 1430}},
	} {
		keys := tokenKeys(t, v)
		if !slices.Contains(keys, ".inputTokens") || !slices.Contains(keys, ".outputTokens") {
			t.Fatalf("%s lost its tokens: %v", what, keys)
		}
		b, _ := json.Marshal(v)
		if credit.Match(b) {
			t.Fatalf("%s names credits: %s", what, b)
		}
	}
}
