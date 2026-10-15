package app

// Describe it In Agentry live: the bound methods driven as the Build tab
// drives them, against a real registry that drafts (the runs extension's
// draft kind), with the folder picker answered by the test. Skipped unless
// both variables are set, because it spends the account's credits (a draft
// holds 41 and settles what it uses):
//
//	AGENTRY_LIVE_HOSTED_DRAFT_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_HOSTED_DRAFT_TOKEN=<a PAT with run:read, run:write, user:profile, agent:read, agent:publish> \
//	go test -run TestLiveDescribeInAgentry -v -timeout 60m ./internal/app
//
// It drafts from a description In Agentry, follows the draft to its end,
// unpacks its output into a fresh HOME's Downloads, decodes draft.json into
// the Model, saves the Model into a folder the picker gave, and validates
// it. With AGENTRY_LIVE_HOSTED_DRAFT_AGAIN=1 it then describes again from
// the saved draft and a note, and with AGENTRY_LIVE_HOSTED_DRAFT_PUBLISH=1
// it submits the (last) draft, publicly, which sends it into the pipeline;
// the account's email must be verified for that.
//
// With AGENTRY_LIVE_HOSTED_DRAFT_SUBMIT=1 it takes the summary's path
// instead of the form's: the draft lands, BuildAPI.Summary draws it, the
// Builder's own settings hold, and Submit, the second click, saves it into
// the picked folder and submits it publicly with no edit in between, its
// native warning (the cost and public) answered Yes. The form's checks must
// pass as the draft came, or Submit would be greyed.

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/drafter"
	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/draft"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveDescribeInAgentry(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_HOSTED_DRAFT_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_HOSTED_DRAFT_TOKEN")
	if reg == "" || token == "" {
		t.Skip("set AGENTRY_LIVE_HOSTED_DRAFT_REGISTRY and _TOKEN to run the live check of a draft In Agentry")
	}
	// The token under test, and no other: this machine's own keychain may
	// hold a desktop token for the same registry.
	keyring.MockInit()
	home := t.TempDir()
	setUserHome(t, home)
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

	var mu sync.Mutex
	var finals []*HostedDraftView
	emit := emitEvent
	emitEvent = func(_ *App, ev string, data ...any) {
		if ev != eventDraftHosted || len(data) != 1 {
			return
		}
		if v, ok := data[0].(*HostedDraftView); ok {
			t.Logf("draft %s: %s %s — %s", v.ID, v.Status, v.State, v.StatusLine)
			if v.Final {
				mu.Lock()
				finals = append(finals, v)
				mu.Unlock()
			}
		}
	}
	t.Cleanup(func() { emitEvent = emit })
	// Submit's warning is answered Yes, and what it said is kept for the
	// test to read: public, and the account's cost.
	var submitWarning string
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, msg string) (bool, error) {
		if strings.HasPrefix(title, "Submit ") {
			submitWarning = msg
		}
		return true, nil
	}}
	b := &BuildAPI{app: app, run: &RunAPI{app: app}}

	avail, err := b.DescribeInAgentryStatus(reg)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("offer: offered %v, available %v (%s %s); %s balance %d (known %v, short %v); caps %ds, %d turns, %d pages",
		avail.Offered, avail.Available, avail.Reason, avail.Remedy, avail.HoldSentence, avail.Balance, avail.BalanceKnown, avail.Short,
		avail.TimeoutSeconds, avail.MaxTurns, avail.MaxPages)
	if !avail.Available {
		t.Fatalf("a draft In Agentry is not available here: %s", avail.Reason)
	}

	wait := func(n int) *HostedDraftView {
		t.Helper()
		deadline := time.Now().Add(40 * time.Minute)
		for time.Now().Before(deadline) {
			mu.Lock()
			if len(finals) >= n {
				v := finals[n-1]
				mu.Unlock()
				return v
			}
			mu.Unlock()
			time.Sleep(2 * time.Second)
		}
		t.Fatal("the draft never reported its final word")
		return nil
	}
	report := func(v *HostedDraftView) *DraftView {
		t.Helper()
		t.Logf("final: %s; %s", v.OutcomeLine, v.CreditsLine)
		if v.Error != "" || v.DraftError != "" {
			t.Logf("error %q, draft error %q", v.Error, v.DraftError)
		}
		d := v.Draft
		if d == nil {
			t.Fatalf("no draft came of it: %s %s", v.OutcomeLine, v.DraftError)
		}
		t.Logf("draft: %s/%s@%s — %s; %d decode notes %q; AI Review (%s) read %d, passed %d, withheld %d, not scanned %d; %d matches, %d sources, %d open questions; out %s",
			d.Model.Namespace, d.Model.Name, d.Model.Version, d.Model.Summary, len(d.DecodeNotes), d.DecodeNotes, d.AIReview.Scan, d.AIReview.Read,
			d.AIReview.Passed, d.AIReview.Withheld, d.AIReview.NotScanned, len(d.Matches), len(d.Sources), len(d.OpenQuestions), d.OutDir)
		if d.Where != "agentry" || !strings.HasPrefix(d.OutDir, filepath.Join(home, "Downloads", "agentry-out")) {
			t.Fatalf("where %q, out %q", d.Where, d.OutDir)
		}
		if c := d.Model.Check(); c.First != nil {
			t.Logf("the form's checks name what is left: %s: %s", c.First.Section, c.First.Problem)
		}
		return d
	}

	description := "Every Monday, read the week's closed GitHub issues in one repository I name and write a short Markdown summary of what changed, grouped by label."
	hints := []string{"The services or sites it works with: GitHub", "It runs on its own, from a task, with no conversation.",
		"An example of the output wanted: a Markdown list, one line per issue, with a link"}
	// With AGENTRY_LIVE_HOSTED_DRAFT_PACK, the name of a knowledge pack the
	// account keeps in Agentry, the draft reads it first: the draft records
	// it as scanned in Agentry.
	var packs []string
	if p := os.Getenv("AGENTRY_LIVE_HOSTED_DRAFT_PACK"); p != "" {
		packs = []string{p}
		if avail, _ := b.DescribeInAgentryStatus(reg); !avail.KnowledgeEnabled {
			t.Fatalf("a pack was named, and this registry's drafts read none: %+v", avail)
		}
	}
	start, err := b.DescribeInAgentry(reg, description, hints, packs)
	if err != nil {
		t.Fatal(err)
	}
	if start.Draft == nil {
		t.Fatalf("refused: %s %s", start.Refusal, start.Detail)
	}
	d := report(wait(1))
	if len(packs) > 0 {
		if len(d.Knowledge) != 1 || d.Knowledge[0].Name != packs[0] || d.Knowledge[0].Scan != drafter.ScannedInAgentry {
			t.Fatalf("the draft's knowledge record: %+v", d.Knowledge)
		}
		t.Logf("knowledge: %s scanned in Agentry, %d file(s) withheld; accepts %q", d.Knowledge[0].Name, len(d.Knowledge[0].Withheld), d.Model.KnowledgePrompt)
	}

	dir := t.TempDir()
	app.markPicked(dir)
	if err := b.Save(dir, *d.Model); err != nil {
		t.Fatal(err)
	}
	res, err := b.Validate(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("validate: passed %v, %d errors, %d warnings, %d findings %s", res.Passed, res.ErrorCount, res.WarnCount, len(res.Findings), res.ManifestError)

	if os.Getenv("AGENTRY_LIVE_HOSTED_DRAFT_SUBMIT") == "1" {
		sum := b.Summary(*d.Model, d.Unmentioned)
		t.Logf("summary: %s — %s; %d keys, %d websites, %d servers, grants %q; unnamed %v; check %+v",
			sum.Name, sum.Summary, len(sum.Keys), len(sum.Websites), len(sum.Servers), sum.Grants, sum.AnyUnnamed, d.Check)
		for _, k := range sum.Keys {
			t.Logf("  key %s for %v, help %d chars", k.Name, k.UsedBy, len(k.Help))
		}
		for _, w := range sum.Websites {
			t.Logf("  website %s (unnamed %v)", w.Host, w.Unnamed)
		}
		for _, sv := range sum.Servers {
			t.Logf("  server %s tools %v keys %v config %v hosts %+v", sv.Ref, sv.Tools, sv.Keys, sv.Config, sv.Hosts)
		}
		m := d.Model
		if m.Version != build.BuilderVersion || m.InstructionFile != "AGENT.md" || len(m.PublishInclude) != 0 {
			t.Fatalf("the Builder's settings do not hold: version %q, file %q, include %v", m.Version, m.InstructionFile, m.PublishInclude)
		}
		names, err := ownNames(context.Background(), reg, token, m.Namespace)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(names, m.Name) {
			t.Fatalf("the draft is named %s, which %s already has", m.Name, m.Namespace)
		}
		if len(sum.Keys) != len(declaredKeys(m)) || (m.EgressMode == "hosts" && len(sum.Websites) != len(m.EgressHosts)) || len(sum.Servers) != len(m.MCPServers) {
			t.Fatalf("the summary does not list what the model declares: %+v", sum)
		}
		if c := m.Check(); c.First != nil {
			t.Fatalf("Submit would be greyed: the form's checks name %s: %s", c.First.Section, c.First.Problem)
		}
		sdir := t.TempDir()
		app.markPicked(sdir)
		if err := b.Save(sdir, *m); err != nil {
			t.Fatal(err)
		}
		pub, err := b.Submit(reg, sdir)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("submit warning:\n%s", submitWarning)
		if !strings.Contains(submitWarning, "makes it public") || !strings.Contains(submitWarning, "credit") {
			t.Fatalf("the warning does not say public and the cost:\n%s", submitWarning)
		}
		if pub.Visibility != "public" {
			t.Fatalf("submitted, yet %s: %s", pub.Visibility, pub.VisibilityNote)
		}
		t.Logf("submit: %s %s/%s@%s %s", pub.Status, m.Namespace, m.Name, pub.Version, pub.Detail)
		// The pipeline takes it: published, or held for a person's review.
		if pub.Status != "published" && pub.Status != "needs_review" {
			t.Fatalf("the pipeline did not take the draft: %s %s", pub.Status, pub.Detail)
		}
		return
	}

	if os.Getenv("AGENTRY_LIVE_HOSTED_DRAFT_AGAIN") == "1" {
		text, err := b.DescribeAgainText(*d.Model, "Also post the summary to a Slack channel the person names, and ask for that channel before a run.", d.Description)
		if err != nil {
			t.Fatal(err)
		}
		if _, first, ok := draft.SplitDescribeAgain(text); !ok || first != description {
			t.Fatalf("Describe again's description does not carry the first one: %q", first)
		}
		again, err := b.DescribeInAgentry(reg, text, d.Hints, nil)
		if err != nil {
			t.Fatal(err)
		}
		if again.Draft == nil {
			t.Fatalf("refused: %s %s", again.Refusal, again.Detail)
		}
		d = report(wait(2))
		if FirstDescriptionOf(d) != description {
			t.Fatalf("the new draft's description reads back as %q", FirstDescriptionOf(d))
		}
		dir = t.TempDir()
		app.markPicked(dir)
		if err := b.Save(dir, *d.Model); err != nil {
			t.Fatal(err)
		}
	}

	if os.Getenv("AGENTRY_LIVE_HOSTED_DRAFT_PUBLISH") == "1" {
		// The person fixes in Step by step what the form's checks name
		// before they publish: here, the two rules a draft has been seen to
		// fail: a guide that says too much, a choice named twice.
		m := *d.Model
		for i := range m.Guide.Connect {
			if len(m.Guide.Connect[i].Help) > 300 {
				m.Guide.Connect[i].Help = m.Guide.Connect[i].Help[:297] + "..."
			}
		}
		if len(m.Guide.Prepare) > 10 {
			m.Guide.Prepare = m.Guide.Prepare[:10]
		}
		for i := range m.Intake {
			seen := map[string]bool{}
			var kept []string
			for _, c := range m.Intake[i].Choices {
				if !seen[c] {
					seen[c] = true
					kept = append(kept, c)
				}
			}
			m.Intake[i].Choices = kept
		}
		if c := m.Check(); c.First != nil {
			t.Fatalf("the form's checks still name %s: %s", c.First.Section, c.First.Problem)
		}
		if err := b.Save(dir, m); err != nil {
			t.Fatal(err)
		}
		pub, err := b.Submit(reg, dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("publish: %s %s@%s %s", pub.Status, d.Model.Name, pub.Version, pub.Detail)
		if pub.Status == "refused" {
			t.Fatalf("the pipeline refused the draft: %s", pub.Detail)
		}
	}
}

// declaredKeys is every key the model declares, its own and its servers',
// each once.
func declaredKeys(m *build.Model) []string {
	var out []string
	for _, k := range append(append([]string{}, m.Secrets...), serverKeys(m)...) {
		if k = strings.TrimSpace(k); k != "" && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

func serverKeys(m *build.Model) []string {
	var out []string
	for _, s := range m.MCPServers {
		out = append(out, s.Secrets...)
	}
	return out
}

// FirstDescriptionOf is what the person first asked a draft for.
func FirstDescriptionOf(d *DraftView) string { return draft.FirstDescription(d.Description) }
