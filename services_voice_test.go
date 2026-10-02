package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/voice"
)

// The Voice panel's bound surface (plan writing-voice.md §8, D6, D11, D12;
// agentry-desktop#510): this computer's voice and the account's, Submit
// and Use between them, the dialogs that guard a replace or a delete, and
// the voice a run writes in.

type voiceFixture struct {
	v      *VoiceAPI
	reg    *knowledgetest.Registry
	steps  []string
	answer bool
}

func newVoiceFixture(t *testing.T, signedIn bool) *voiceFixture {
	t.Helper()
	home := t.TempDir()
	t.Setenv(policy.HomeEnv, home)
	reg := knowledgetest.NewRegistry(t)
	tok := ""
	if signedIn {
		tok = reg.Token("ana")
	}
	f := &voiceFixture{reg: reg, answer: true}
	env := knowledge.Env{
		Home:     func() (string, error) { return home, nil },
		Keys:     &knowledgeKeys{m: map[string]string{}},
		TokenFor: func(u string) string { return map[bool]string{true: tok}[u == reg.URL] },
		HTTP:     reg.Client(),
	}
	app := &App{
		ctx: context.Background(),
		confirm: func(_ context.Context, title, _ string) (bool, error) {
			f.steps = append(f.steps, "ask: "+title)
			return f.answer, nil
		},
	}
	f.v = &VoiceAPI{app: app, run: &RunAPI{app: app, knowledgeEnv: &env, runnerHomeDir: t.TempDir()}}
	return f
}

const voiceText = "# Ana's voice\n\nShort sentences. Name who does what.\n"

func TestThePanelShowsThisComputersVoiceAndSignedInTheAccounts(t *testing.T) {
	f := newVoiceFixture(t, false)
	view, err := f.v.View(f.reg.URL)
	if err != nil || view.Local.Exists || view.SignedIn || view.AccountOffered {
		t.Fatalf("signed out with nothing kept: %+v, %v", view, err)
	}
	saved, err := f.v.SaveLocal(f.reg.URL, voiceText)
	if err != nil || !saved.Created || !saved.Voice.AlwaysAttach {
		t.Fatalf("the first voice here is on for every run: %+v, %v", saved, err)
	}

	f = newVoiceFixture(t, true)
	view, err = f.v.View(f.reg.URL)
	if err != nil || !view.SignedIn || !view.AccountOffered || view.Account == nil || view.Account.Exists {
		t.Fatalf("signed in, no voice in the account: %+v, %v", view, err)
	}
}

func TestUseAsksBeforeItReplacesADifferentVoiceHere(t *testing.T) {
	f := newVoiceFixture(t, true)
	if _, err := f.v.UseAccount(f.reg.URL); !errors.Is(err, voice.ErrNoLiveVoice) {
		t.Fatalf("Use with nothing live: %v", err)
	}
	if _, err := f.v.SaveAccount(f.reg.URL, voiceText); err != nil {
		t.Fatal(err)
	}
	f.reg.Review("ana", knowledgetest.VoicePass)
	if _, err := f.v.SaveLocal(f.reg.URL, "# Another voice\n\nLong sentences.\n"); err != nil {
		t.Fatal(err)
	}
	f.answer = false
	if _, err := f.v.UseAccount(f.reg.URL); err == nil {
		t.Fatal("Use answered No replaced the voice here")
	}
	if view, _ := f.v.View(f.reg.URL); view.Local.Text == voiceText {
		t.Fatal("a No still replaced it")
	}
	f.answer = true
	used, err := f.v.UseAccount(f.reg.URL)
	if err != nil || used.Voice.Text != voiceText || used.Voice.Source != "download" {
		t.Fatalf("Use: %+v, %v", used, err)
	}
	// The same voice again is not a replacement, and asks nothing.
	f.steps = nil
	if _, err := f.v.UseAccount(f.reg.URL); err != nil || len(f.steps) != 0 {
		t.Fatalf("Use of the voice already here asked %v: %v", f.steps, err)
	}
}

func TestSubmitAndDeleteKeepTheirPlaces(t *testing.T) {
	f := newVoiceFixture(t, true)
	if _, err := f.v.SaveLocal(f.reg.URL, voiceText); err != nil {
		t.Fatal(err)
	}
	a, err := f.v.Submit(f.reg.URL)
	if err != nil || a.Pending == nil || a.Pending.Review != "reviewing" {
		t.Fatalf("submit: %+v, %v", a, err)
	}
	f.answer = false
	if err := f.v.DeleteLocal(f.reg.URL); err == nil {
		t.Fatal("a delete answered No deleted")
	}
	f.answer = true
	if err := f.v.DeleteLocal(f.reg.URL); err != nil {
		t.Fatal(err)
	}
	view, err := f.v.View(f.reg.URL)
	if err != nil || view.Local.Exists || view.Account.Pending == nil {
		t.Fatalf("deleting this computer's voice touched the account's: %+v, %v", view, err)
	}
}

func TestTheVoiceWriterTakesOnlyPickedSamplesAndANoteForAnEdit(t *testing.T) {
	f := newVoiceFixture(t, false)
	if _, err := f.v.Write(f.reg.URL, "rewrite", nil, ""); err == nil {
		t.Fatal("a job that is not create, improve or edit ran")
	}
	if _, err := f.v.Write(f.reg.URL, "create", []string{"/etc/passwd"}, ""); err == nil || !strings.Contains(err.Error(), "file dialog") {
		t.Fatalf("a sample no picker returned: %v", err)
	}
	if _, err := f.v.Write(f.reg.URL, "edit", nil, " "); err == nil {
		t.Fatal("an AI Edit with no note ran")
	}
	if _, err := f.v.StartHosted(f.reg.URL, "create", nil, ""); err == nil {
		t.Fatal("a Create with no samples ran In Agentry")
	}
}

func TestARunWritesInThisComputersVoiceOnlyWhenAsked(t *testing.T) {
	f := newVoiceFixture(t, false)
	ctx := context.Background()
	if v, err := f.v.run.voiceFor(ctx, f.reg.URL, false); v != nil || err != nil {
		t.Fatalf("not asked: %v, %v", v, err)
	}
	if _, err := f.v.run.voiceFor(ctx, f.reg.URL, true); err == nil || !strings.Contains(err.Error(), "keeps no Writing Voice") {
		t.Fatalf("asked with none kept: %v", err)
	}
	if _, err := f.v.SaveLocal(f.reg.URL, voiceText); err != nil {
		t.Fatal(err)
	}
	v, err := f.v.run.voiceFor(ctx, f.reg.URL, true)
	if err != nil || string(v.Text) != voiceText || !strings.Contains(v.Origin, "not reviewed") {
		t.Fatalf("asked: %+v, %v", v, err)
	}
}

func TestAHostedRunAsksForTheAccountsVoiceOnlyWhenTicked(t *testing.T) {
	f := &hostedFake{
		enabled: true,
		scopes:  []string{"agent:read", "user:profile", "run:read", "run:write", "voice:read"},
		outputs: map[string]string{"result.json": hostedEnvelope},
	}
	reg := f.start(t)
	signIn(t, reg)
	fastPoll(t)
	r, events := newRunAPI(t)
	outDir := t.TempDir()
	r.app.markPicked(outDir)
	for _, want := range []bool{true, false} {
		f.mu.Lock()
		f.run, f.created = nil, nil
		f.mu.Unlock()
		events.mu.Lock()
		events.runs = nil
		events.mu.Unlock()
		if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, nil, want); err != nil {
			t.Fatal(err)
		}
		f.mu.Lock()
		created := f.created
		f.mu.Unlock()
		if created == nil || created.Voice != want {
			t.Fatalf("ticked %v sent %+v", want, created)
		}
		f.finish(hostedrun.OutcomeSucceeded, "ok", 1)
		waitForFinal(t, events)
	}
	if !slices.Contains(requestedScopes, hostedrun.ScopeVoiceRead) || !slices.Contains(requestedScopes, hostedrun.ScopeVoiceWrite) {
		t.Fatal("the sign-in does not ask for the voice scopes")
	}
}

func TestACreateAsksBeforeItReplacesTheVoiceHere(t *testing.T) {
	f := newVoiceFixture(t, false)
	if _, err := f.v.SaveLocal(f.reg.URL, voiceText); err != nil {
		t.Fatal(err)
	}
	sample := t.TempDir()
	f.v.app.markPicked(sample)
	f.answer = false
	if _, err := f.v.Write(f.reg.URL, "create", []string{sample}, ""); err == nil {
		t.Fatal("a Create answered No ran")
	}
	if strings.Join(f.steps, "|") != "ask: Replace this computer's voice?" {
		t.Fatalf("asked %v", f.steps)
	}
	if view, _ := f.v.View(f.reg.URL); view.Local.Text != voiceText {
		t.Fatal("a No still replaced the voice")
	}
}

func TestTheVoiceWriterInAgentrySendsNoTokenToAURLSettingsRefuses(t *testing.T) {
	f := newVoiceFixture(t, true)
	if _, err := f.v.HostedAvailability("http://example.com", "create"); err == nil {
		t.Fatal("availability asked a registry Settings refuses")
	}
	sample := t.TempDir()
	f.v.app.markPicked(sample)
	if _, err := f.v.StartHosted("http://example.com", "create", []string{sample}, ""); err == nil || !strings.Contains(err.Error(), "registry") {
		t.Fatalf("a start at a refused URL: %v", err)
	}
}
