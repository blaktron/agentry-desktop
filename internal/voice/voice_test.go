package voice

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/knowledge/knowledgetest"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/voice/build"
	"github.com/blaktron/agentry-cli/pkg/voice/remote"
	"github.com/blaktron/agentry-cli/pkg/voice/writer"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The desktop's two voices (plan writing-voice.md D6, D11; agentry-desktop
// #510): this computer's in the CLI's store, sealed and free; the account's
// in the registry, reviewed before it is live; Submit and Use the only ways
// between them. The registry is the CLI's own fake, which serves the core's
// voice routes and decides a review when told to.

type memKeys struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKeys) Get(a string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[a]
	if !ok {
		return "", store.ErrNoEntry
	}
	return v, nil
}

func (k *memKeys) Set(a, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[a] = v
	return nil
}

func (k *memKeys) Delete(a string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, a)
	return nil
}

const owner = "writer"

// testEnv is the voice wired as this app wires it, to a runner home of its
// own, a keychain in memory, and a token for reg when signedIn.
func testEnv(t *testing.T, signedIn bool) (Env, *knowledgetest.Registry) {
	t.Helper()
	reg := knowledgetest.NewRegistry(t)
	token := ""
	if signedIn {
		token = reg.Token(owner)
	}
	home := t.TempDir()
	return Env{Env: knowledge.Env{
		Home: func() (string, error) { return home, nil },
		Keys: &memKeys{m: map[string]string{}},
		TokenFor: func(u string) string {
			if token != "" && strings.TrimRight(u, "/") == reg.URL {
				return token
			}
			return ""
		},
		HTTP: reg.Client(),
	}}, reg
}

const text = "# My voice\n\nWrite short sentences. Name who does what.\n"

func TestThisComputersVoiceIsKeptSealedFreeAndTheFirstComesOn(t *testing.T) {
	e, reg := testEnv(t, false)
	ctx := context.Background()
	got, err := e.Local(ctx, reg.URL)
	if err != nil || got.Exists {
		t.Fatalf("no voice yet: %+v, %v", got, err)
	}
	l, created, err := e.SaveLocal(ctx, reg.URL, text)
	if err != nil {
		t.Fatal(err)
	}
	if !created || !l.AlwaysAttach || l.Source != "edit" {
		t.Fatalf("the first voice comes on for every run, written by hand: created %v, %+v", created, l)
	}
	// Sealed at rest: no file under the runner home holds the text.
	home, _ := e.Home()
	_ = filepath.WalkDir(home, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if b, _ := os.ReadFile(p); bytes.Contains(b, []byte("Name who does what")) {
				t.Errorf("%s holds the voice in the clear", p)
			}
		}
		return nil
	})
	got, err = e.Local(ctx, reg.URL)
	if err != nil || got.Text != text || got.Digest != l.Digest {
		t.Fatalf("read back: %+v, %v", got, err)
	}
	if err := e.SetLocalAttach(reg.URL, false); err != nil {
		t.Fatal(err)
	}
	if got, _ = e.Local(ctx, reg.URL); got.AlwaysAttach {
		t.Fatal("the switch did not turn off")
	}
	// A second save is not the first: the flag the person set stays.
	if _, created, _ := e.SaveLocal(ctx, reg.URL, text+"More.\n"); created {
		t.Fatal("a second save reads as the first")
	}
	if got, _ = e.Local(ctx, reg.URL); got.AlwaysAttach {
		t.Fatal("a save turned the switch back on")
	}
}

func TestALocalRunAttachesThisComputersVoiceAndSaysItWasNotReviewed(t *testing.T) {
	e, reg := testEnv(t, false)
	ctx := context.Background()
	if v, err := e.Attach(ctx, reg.URL); v != nil || err != nil {
		t.Fatalf("no voice attaches nothing: %v, %v", v, err)
	}
	l, _, err := e.SaveLocal(ctx, reg.URL, text)
	if err != nil {
		t.Fatal(err)
	}
	v, err := e.Attach(ctx, reg.URL)
	if err != nil || v == nil {
		t.Fatalf("attach: %v, %v", v, err)
	}
	if string(v.Text) != text || v.Digest != l.Digest || !strings.Contains(v.Origin, "not reviewed") {
		t.Fatalf("attached %q %s, origin %q", v.Text, v.Digest, v.Origin)
	}
}

func TestSubmitSendsThisComputersVoiceForReviewAndChangesNothingHere(t *testing.T) {
	e, reg := testEnv(t, true)
	ctx := context.Background()
	l, _, err := e.SaveLocal(ctx, reg.URL, text)
	if err != nil {
		t.Fatal(err)
	}
	a, err := e.Submit(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if a.Live != nil || a.Pending == nil || a.Pending.Review != string(remote.Reviewing) || a.Pending.Digest != l.Digest {
		t.Fatalf("a submission waits for AI Review: %+v", a)
	}
	reg.Review(owner, knowledgetest.VoicePass)
	a, err = e.Account(ctx, reg.URL)
	if err != nil || a.Live == nil || a.Live.Digest != l.Digest || a.Live.Text != text {
		t.Fatalf("after the review: %+v, %v", a, err)
	}
	if got, _ := e.Local(ctx, reg.URL); got.Digest != l.Digest || got.Source != "edit" {
		t.Fatalf("Submit changed this computer's voice: %+v", got)
	}
}

func TestAFlaggedSubmissionKeepsItsFindingsAndIsNotLive(t *testing.T) {
	e, reg := testEnv(t, true)
	ctx := context.Background()
	if _, err := e.SaveAccount(ctx, reg.URL, text); err != nil {
		t.Fatal(err)
	}
	reg.Review(owner, knowledgetest.VoiceFlag)
	a, err := e.Account(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if a.Live != nil || a.Pending == nil || a.Pending.Review != string(remote.Flagged) || len(a.Pending.Findings) == 0 {
		t.Fatalf("a flagged version: %+v", a)
	}
	if _, _, err := e.UseAccount(ctx, reg.URL); !errors.Is(err, ErrNoLiveVoice) {
		t.Fatalf("Use with nothing live: %v", err)
	}
}

func TestUseKeepsTheAccountsLiveVoiceHereAsDownloaded(t *testing.T) {
	e, reg := testEnv(t, true)
	ctx := context.Background()
	if _, err := e.SaveAccount(ctx, reg.URL, text); err != nil {
		t.Fatal(err)
	}
	reg.Review(owner, knowledgetest.VoicePass)
	l, created, err := e.UseAccount(ctx, reg.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !created || l.Text != text || l.Source != "download" {
		t.Fatalf("used: created %v, %+v", created, l)
	}
	v, err := e.Attach(ctx, reg.URL)
	if err != nil || !strings.Contains(v.Origin, "not checked again here") {
		t.Fatalf("a downloaded voice's receipt line: %v, %v", v, err)
	}
	a, err := e.SetAccountAttach(ctx, reg.URL, false)
	if err != nil || a.AlwaysAttach {
		t.Fatalf("the account's switch: %+v, %v", a, err)
	}
}

func TestSignedOutTheAccountIsNotAsked(t *testing.T) {
	e, reg := testEnv(t, false)
	if _, err := e.Account(context.Background(), reg.URL); !errors.Is(err, remote.ErrSignedOut) {
		t.Fatalf("signed out: %v", err)
	}
	if _, err := e.Account(context.Background(), "http://example.com"); err == nil || errors.Is(err, remote.ErrSignedOut) {
		t.Fatalf("a registry URL Settings would refuse is refused before a token is looked up: %v", err)
	}
}

func TestTheVoiceWriterOnThisComputerKeepsWhatItWrote(t *testing.T) {
	e, reg := testEnv(t, false)
	ctx := context.Background()
	// The policy is the runner home's: relocate it, so no test reads or
	// writes the operator's own (agentry-cli#149).
	home, _ := e.Home()
	t.Setenv(policy.HomeEnv, home)
	var seen build.Options
	fake := func(_ context.Context, _ dockerx.Client, o build.Options) (*build.Result, error) {
		seen = o
		return &build.Result{Text: []byte("# Voice by the Writer\n\nKeep it plain.\n"), Samples: []string{"a.txt"}}, nil
	}
	if _, err := e.Build(ctx, BuildRequest{Registry: reg.URL, Job: build.Edit, Brief: "shorter", Build: fake}); err == nil {
		t.Fatal("an AI Edit with no voice here ran")
	}
	res, err := e.Build(ctx, BuildRequest{Registry: reg.URL, Job: build.Create, Samples: []string{"a.txt"}, Brief: " plain ", Build: fake})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.Voice.Source != "create" || !res.Voice.AlwaysAttach || seen.Brief != "plain" || seen.Current != nil {
		t.Fatalf("create: %+v, saw %+v", res, seen)
	}
	if _, err := e.Build(ctx, BuildRequest{Registry: reg.URL, Job: build.Edit, Brief: "shorter", Build: fake}); err != nil {
		t.Fatal(err)
	}
	if string(seen.Current) != "# Voice by the Writer\n\nKeep it plain.\n" {
		t.Fatalf("an AI Edit works from the voice kept here: %q", seen.Current)
	}
	if got, _ := e.Local(ctx, reg.URL); got.Source != "ai-edit" {
		t.Fatalf("after the edit: %+v", got)
	}
}

// Run defaults' no-limit switch for the built-in agents (agentry-desktop#606,
// D-062; on by default since agentry-desktop#699): while it is on, the Voice
// Writer runs on this computer with no timeout and to the hidden failsafe
// of 200 turns; off, it has its floor and the policy's turns.
func TestTheBuiltinAgentsSwitchGivesTheVoiceWriterNoTimeOrTurnLimitByDefault(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	e, reg := testEnv(t, false)
	ctx := context.Background()
	home, _ := e.Home()
	t.Setenv(policy.HomeEnv, home)
	var seen build.Options
	fake := func(_ context.Context, _ dockerx.Client, o build.Options) (*build.Result, error) {
		seen = o
		return &build.Result{Text: []byte("# Voice by the Writer\n\nKeep it plain.\n"), Samples: []string{"a.txt"}}, nil
	}
	if _, err := e.Build(ctx, BuildRequest{Registry: reg.URL, Job: build.Create, Samples: []string{"a.txt"}, Build: fake}); err != nil {
		t.Fatal(err)
	}
	if seen.Budgets == nil || seen.Budgets.TimeoutSeconds != 0 || seen.Budgets.MaxTurns != settings.BuiltinFailsafeTurns {
		t.Fatalf("by default the Writer ran under %+v, want no timeout and the failsafe's turns", seen.Budgets)
	}

	st, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	off := false
	st.BuiltinNoLimit = &off
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Build(ctx, BuildRequest{Registry: reg.URL, Job: build.Edit, Brief: "shorter", Build: fake}); err != nil {
		t.Fatal(err)
	}
	if seen.Budgets == nil || seen.Budgets.TimeoutSeconds < writer.LocalTimeoutFloorSeconds || seen.Budgets.MaxTurns == settings.BuiltinFailsafeTurns {
		t.Fatalf("with the switch off the Writer ran under %+v, want at least its floor and the policy's turns", seen.Budgets)
	}
}
