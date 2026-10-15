package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	voicepkg "github.com/blaktron/agentry-cli/pkg/voice"
	voicebuild "github.com/blaktron/agentry-cli/pkg/voice/build"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/voice"
)

// The Voice panel's bound surface: this computer's voice and the account's,
// Submit and Use between them, the dialogs that guard a replace or a delete,
// and the voice a run writes in.

type voiceFixture struct {
	v      *VoiceAPI
	reg    *knowledgetest.Registry
	steps  []string
	msgs   []string
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
		confirm: func(_ context.Context, title, msg string) (bool, error) {
			f.steps = append(f.steps, "ask: "+title)
			f.msgs = append(f.msgs, msg)
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
	// An AI Edit takes samples now, and still only picked ones.
	if _, err := f.v.Write(f.reg.URL, "edit", []string{"/etc/passwd"}, "shorter"); err == nil || !strings.Contains(err.Error(), "file dialog") {
		t.Fatalf("an AI Edit's sample no picker returned: %v", err)
	}
	if _, err := f.v.StartHosted(f.reg.URL, "create", nil, ""); err == nil {
		t.Fatal("a Create with no samples ran In Agentry")
	}
}

// Edit with AI on the Local Voice page is one dialog: with a voice here it
// runs an AI Edit with the note and the samples the person added; with none
// it runs a Create, whose samples are required and whose text is the brief.
func TestEditWithAIIsAnEditOverAVoiceHereAndACreateWithout(t *testing.T) {
	f := newVoiceFixture(t, false)
	var seen []voicebuild.Options
	f.v.build = func(_ context.Context, _ dockerx.Client, o voicebuild.Options) (*voicebuild.Result, error) {
		seen = append(seen, o)
		return &voicebuild.Result{Text: []byte("# Written by the Writer\n\nPlain words.\n")}, nil
	}
	if _, err := f.v.WriteWithAI(f.reg.URL, "sound like me", nil); err == nil || !strings.Contains(err.Error(), "samples") {
		t.Fatalf("with no voice and no samples: %v", err)
	}
	sample := t.TempDir()
	f.v.app.markPicked(sample)
	res, err := f.v.WriteWithAI(f.reg.URL, " sound like me ", []string{sample})
	if err != nil || !res.Created || res.Voice.Source != "create" {
		t.Fatalf("a Create from the dialog: %+v, %v", res, err)
	}
	if o := seen[0]; o.Job != voicebuild.Create || o.Brief != "sound like me" || len(o.Samples) != 1 || o.Current != nil {
		t.Fatalf("the Create was run as %+v", o)
	}
	if len(f.steps) != 0 {
		t.Fatalf("a Create with nothing to replace asked %v", f.steps)
	}
	// With a voice here: an AI Edit, the samples optional.
	if _, err := f.v.WriteWithAI(f.reg.URL, "", nil); err == nil || !strings.Contains(err.Error(), "what should change") {
		t.Fatalf("an Edit with no note: %v", err)
	}
	if _, err := f.v.WriteWithAI(f.reg.URL, "shorter sentences", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.v.WriteWithAI(f.reg.URL, "shorter sentences", []string{sample}); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{0, 1} {
		o := seen[i+1]
		if o.Job != voicebuild.Edit || o.Brief != "shorter sentences" || len(o.Samples) != want || string(o.Current) != "# Written by the Writer\n\nPlain words.\n" {
			t.Fatalf("Edit %d was run as %+v", i, o)
		}
	}
	if view, _ := f.v.View(f.reg.URL); view.Local.Source != "ai-edit" {
		t.Fatalf("after the edit: %+v", view.Local)
	}
	if len(f.steps) != 0 {
		t.Fatalf("an Edit asked %v", f.steps)
	}
}

// Download Cloud version warns, in the native confirm Use always asked
// before a replace, when the voice here was saved after the live version,
// and says so when a newer version is in review.
func TestDownloadWarnsWhenItWouldReplaceANewerVoiceOrOneIsInReview(t *testing.T) {
	f := newVoiceFixture(t, true)
	if _, err := f.v.SaveAccount(f.reg.URL, voiceText); err != nil {
		t.Fatal(err)
	}
	f.reg.Review("ana", knowledgetest.VoicePass)
	// Saved here after the live version was set.
	if _, err := f.v.SaveLocal(f.reg.URL, "# Another voice\n\nLong sentences.\n"); err != nil {
		t.Fatal(err)
	}
	view, err := f.v.View(f.reg.URL)
	if err != nil || view.Sync.DownloadSame || view.Sync.UploadSame {
		t.Fatalf("two different voices: %+v, %v", view.Sync, err)
	}
	f.answer = false
	if _, err := f.v.UseAccount(f.reg.URL); err == nil {
		t.Fatal("a download answered No replaced the voice here")
	}
	if strings.Join(f.steps, "|") != "ask: Replace this computer's voice?" || !strings.Contains(f.msgs[0], "cannot be recovered") {
		t.Fatalf("asked %v: %q", f.steps, f.msgs)
	}
	if view.Sync.LocalAfterLive && !strings.Contains(f.msgs[0], "was saved after the cloud version (") {
		t.Fatalf("a newer voice here was not said: %q", f.msgs[0])
	}
	// A newer version in review: said in the same confirm.
	if _, err := f.v.SaveAccount(f.reg.URL, voiceText+"\nNo jargon.\n"); err != nil {
		t.Fatal(err)
	}
	f.steps, f.msgs = nil, nil
	if _, err := f.v.UseAccount(f.reg.URL); err == nil || len(f.msgs) != 1 || !strings.Contains(f.msgs[0], "A newer version is in review in your account and is not live yet.") {
		t.Fatalf("with a version in review: %v, %q", err, f.msgs)
	}
	// With nothing here to replace, the version in review is still said.
	f.answer = true
	if err := f.v.DeleteLocal(f.reg.URL); err != nil {
		t.Fatal(err)
	}
	f.steps, f.msgs = nil, nil
	used, err := f.v.UseAccount(f.reg.URL)
	if err != nil || used.Voice.Text != voiceText {
		t.Fatalf("download: %+v, %v", used, err)
	}
	if strings.Join(f.steps, "|") != "ask: Download the cloud version?" || !strings.Contains(f.msgs[0], "Download the live version anyway?") {
		t.Fatalf("asked %v: %q", f.steps, f.msgs)
	}
	if view, _ := f.v.View(f.reg.URL); !view.Sync.DownloadSame || view.Sync.UploadSame || !view.Sync.InReview {
		t.Fatalf("after the download: %+v", view.Sync)
	}
}

// Upload to Cloud says "already the same" where the account's newest version
// is this computer's, and warns, saying what AI Review costs, when the
// account's newest version was set after this computer's.
func TestUploadWarnsWhenTheAccountsVersionIsNewerAndRefusesTheSame(t *testing.T) {
	f := newVoiceFixture(t, true)
	if _, err := f.v.SaveLocal(f.reg.URL, voiceText); err != nil {
		t.Fatal(err)
	}
	if _, err := f.v.Submit(f.reg.URL); err != nil || len(f.steps) != 0 {
		t.Fatalf("a first upload asked %v: %v", f.steps, err)
	}
	if _, err := f.v.Submit(f.reg.URL); !errors.Is(err, ErrAlreadyTheSame) {
		t.Fatalf("the same voice again: %v", err)
	}
	// A version set in the account after this computer's was saved (the
	// stamp here is to the second, the account's later).
	if _, err := f.v.SaveAccount(f.reg.URL, voiceText+"\nNo jargon.\n"); err != nil {
		t.Fatal(err)
	}
	f.answer = false
	if _, err := f.v.Submit(f.reg.URL); err == nil {
		t.Fatal("an upload answered No went up")
	}
	if strings.Join(f.steps, "|") != "ask: Upload this computer's voice?" ||
		!strings.Contains(f.msgs[0], "The cloud version was set after the one on this computer (") ||
		!strings.HasSuffix(f.msgs[0], "Upload this computer's voice anyway? It goes to AI Review.") {
		t.Fatalf("asked %v: %q", f.steps, f.msgs)
	}
	if view, _ := f.v.View(f.reg.URL); view.Account.Pending == nil || view.Account.Pending.Text == voiceText {
		t.Fatal("a No still uploaded")
	}
	f.answer = true
	a, err := f.v.Submit(f.reg.URL)
	if err != nil || a.Pending == nil || a.Pending.Text != voiceText {
		t.Fatalf("the upload: %+v, %v", a, err)
	}
}

func TestTheWarningsSayTheDatesAndTheCredit(t *testing.T) {
	saved := time.Date(2026, 10, 4, 14, 2, 0, 0, time.Local)
	set := time.Date(2026, 10, 3, 9, 15, 0, 0, time.Local)
	here := &voice.Local{Exists: true, Digest: "sha256:here", Updated: voicepkg.Stamp(saved)}
	a := &voice.Account{Exists: true, Live: &voice.Version{Review: "passed", Digest: "sha256:live", CreatedAt: set}}
	_, msg, ask := downloadWarning(here, a, voice.SyncOf(here, a))
	if !ask || msg != "The voice on this computer was saved after the cloud version (4 Oct 14:02 against 3 Oct 09:15). Replace it with the cloud version? The one here cannot be recovered." {
		t.Fatalf("download: %v %q", ask, msg)
	}
	older := &voice.Local{Exists: true, Digest: "sha256:here", Updated: voicepkg.Stamp(set.Add(-time.Hour))}
	a.Pending = &voice.Version{Review: "reviewing", Digest: "sha256:new", CreatedAt: saved}
	_, msg, ask = uploadWarning(older, a, voice.SyncOf(older, a), 1)
	if !ask || msg != "The cloud version was set after the one on this computer (4 Oct 14:02 against 3 Oct 08:15). Upload this computer's voice anyway? It goes to AI Review for 1 credit." {
		t.Fatalf("upload: %v %q", ask, msg)
	}
	if _, _, ask := uploadWarning(here, &voice.Account{Exists: true, Live: a.Live}, voice.SyncOf(here, &voice.Account{Exists: true, Live: a.Live}), 1); ask {
		t.Fatal("an upload of the newer voice warned")
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
		if _, err := r.StartHostedRun(reg, "acme/reporter@1.0.0", "go", nil, outDir, nil, nil, want, false, nil); err != nil {
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

// The Voice page reads AI Review's price from the runs status route with
// the token Settings keeps by host, so it asks only at a URL Settings'
// rule admits. http://0.0.0.0:<port> is refused by the rule (plain http
// off loopback) yet reaches this test's server, so a request is seen when
// a guard is missing.
func TestTheVoicePageAsksTheStatusRouteOnlyAtAnAdmittedURL(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		http.Error(w, "no", http.StatusNotFound)
	}))
	defer srv.Close()
	refused := "http://0.0.0.0:" + srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	if _, err := hostedrun.New(refused, "agy_test").Status(context.Background()); err != nil && hits.Load() == 0 {
		t.Skipf("%s does not reach the test server on this system: %v", refused, err)
	}
	hits.Store(0)
	f := newVoiceFixture(t, true)
	f.v.run.knowledgeEnv.TokenFor = func(string) string { return "agy_test" }
	if n := f.v.reviewCredits(context.Background(), refused); n != 0 || hits.Load() != 0 {
		t.Fatalf("reviewCredits at a refused URL: %d credits, %d requests", n, hits.Load())
	}
	if _, err := f.v.View(refused); err != nil {
		t.Logf("View at a refused URL: %v", err)
	}
	if hits.Load() != 0 {
		t.Fatalf("View sent %d requests to a URL Settings refuses", hits.Load())
	}
}

// The Cloud Voice page: Discard and Delete act on the account only after the
// person's Yes in a native dialog; Restore is asked nowhere, as the site's,
// and its line says what it does. Edit with AI In Agentry is an AI Edit over
// the account's newest version and a Create where it keeps none.

func TestDiscardAndDeleteInTheAccountAskFirstAndTouchNothingHere(t *testing.T) {
	f := newVoiceFixture(t, true)
	if _, err := f.v.SaveLocal(f.reg.URL, voiceText); err != nil {
		t.Fatal(err)
	}
	// The CLI's fake serves GET and PUT /voice only; these two reach the
	// fake's not-found answer, so a Yes is proven by the call's refusal
	// naming the route, and a No by no call at all.
	f.answer = false
	if _, err := f.v.DiscardAccount(f.reg.URL); err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatalf("a Discard answered No: %v", err)
	}
	if err := f.v.DeleteAccount(f.reg.URL); err == nil || !strings.Contains(err.Error(), "not confirmed") {
		t.Fatalf("a Delete answered No: %v", err)
	}
	if got := strings.Join(f.steps, "|"); got != "ask: Discard your new version?|ask: Delete your account's voice?" {
		t.Fatalf("asked %v", got)
	}
	if !strings.Contains(f.msgs[1], "not touched") {
		t.Fatalf("the Delete dialog does not say this computer's voice stays: %q", f.msgs[1])
	}
	f.answer = true
	if _, err := f.v.DiscardAccount(f.reg.URL); err == nil || !strings.Contains(err.Error(), "discard your new version") {
		t.Fatalf("a Discard answered Yes did not reach the registry: %v", err)
	}
	if view, _ := f.v.View(f.reg.URL); view.Local.Text != voiceText {
		t.Fatal("an account action touched this computer's voice")
	}
}

func TestEditWithAIInAgentryIsAnEditOverTheAccountsNewestVersionAndACreateWithout(t *testing.T) {
	for _, c := range []struct {
		name string
		a    *voice.Account
		want string
	}{
		{"no voice", &voice.Account{}, hostedrun.VoiceCreate},
		{"unread", nil, hostedrun.VoiceCreate},
		{"live", &voice.Account{Exists: true, Live: &voice.Version{}}, hostedrun.VoiceEdit},
		// A first version still in review or flagged is the voice to edit:
		// the site's Edit mode is the same.
		{"pending only", &voice.Account{Exists: true, Pending: &voice.Version{Review: "flagged"}}, hostedrun.VoiceEdit},
	} {
		if got := accountJob(c.a); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}

	f := newVoiceFixture(t, true)
	// No voice in the account: a Create, so samples are required, as the
	// dialog says.
	if _, err := f.v.HostedWithAI(f.reg.URL, "plain and direct", nil); err == nil || !strings.Contains(err.Error(), "samples") {
		t.Fatalf("a Create In Agentry with no samples: %v", err)
	}
	if _, err := f.v.SaveAccount(f.reg.URL, voiceText); err != nil {
		t.Fatal(err)
	}
	// A voice in the account: an AI Edit, so the note is required.
	if _, err := f.v.HostedWithAI(f.reg.URL, "  ", nil); err == nil || !strings.Contains(err.Error(), "what should change") {
		t.Fatalf("an AI Edit In Agentry with no note: %v", err)
	}
	// Samples come only from the native pickers.
	if _, err := f.v.HostedWithAI(f.reg.URL, "shorter", []string{t.TempDir()}); err == nil {
		t.Fatal("an AI Edit In Agentry took a sample no picker returned")
	}
}

// A Cloud Voice page opened again while the Voice Writer runs In Agentry
// (left and reopened, or after a restart) follows that run rather than
// offering a second: AttachHosted finds the account's voice_build in flight
// and never takes a build or a run of another kind for it.
func TestTheCloudPageFollowsAVoiceWriterRunAlreadyInFlight(t *testing.T) {
	f := &hostedFake{
		enabled: true,
		scopes:  []string{"agent:read", "user:profile", "run:read", "run:write", "voice:read", "voice:write"},
		others:  []*hostedrun.Run{{ID: "kb-1", Kind: hostedrun.KindKnowledgeBuild, Status: "running"}},
	}
	reg := f.start(t)
	signIn(t, reg)
	fastPoll(t)
	r, _ := newRunAPI(t)
	v := &VoiceAPI{app: r.app, run: r}

	none, err := v.AttachHosted(reg)
	if err != nil || none.Run != nil {
		t.Fatalf("a knowledge build was taken for a Voice Writer run: %+v, %v", none, err)
	}
	f.mu.Lock()
	f.run = &hostedrun.Run{ID: "vw-1", Kind: hostedrun.KindVoiceBuild, Status: "running", VoiceBuild: &hostedrun.VoiceBuild{Op: hostedrun.VoiceEdit}}
	f.mu.Unlock()
	got, err := v.AttachHosted(reg)
	if err != nil || got.Run == nil || got.Run.ID != "vw-1" || got.Run.Final {
		t.Fatalf("the run in flight: %+v, %v", got, err)
	}
	// Followed now: a second attach reads the same run, and a start is
	// refused rather than becoming a second run.
	again, err := v.AttachHosted(reg)
	if err != nil || again.Run == nil || again.Run.ID != "vw-1" {
		t.Fatalf("attached again: %+v, %v", again, err)
	}
	if err := v.hosted.claim("busy"); err == nil {
		t.Fatal("the slot was free while a run is followed")
	}
	if _, err := v.AttachHosted("http://example.com"); err == nil {
		t.Fatal("an attach at a URL Settings refuses was asked")
	}
	if cur := v.hosted.current(); cur != nil {
		cur.cancel()
		<-cur.done
	}
}
