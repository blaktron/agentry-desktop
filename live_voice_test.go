package main

// The Voice panel live (agentry-desktop#510): the bound methods driven as
// the page drives them, against a real registry that keeps voices, with the
// person's part (the native dialogs) answered by the test. Skipped unless
// the variables below are set, because it needs a registry with AI Review:
//
//	AGENTRY_LIVE_VOICE_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_VOICE_TOKEN=<a PAT with voice:read and voice:write; run:read and run:write for the hosted part> \
//	AGENTRY_LIVE_VOICE_HOSTED=1 \  (optional: an AI Edit In Agentry, 20 credits)
//	go test -run TestLiveVoice -v -timeout 30m .
//
// It keeps a voice on a fresh runner home of its own, submits it, waits for
// AI Review to make it live in the account, keeps the account's live voice
// on a second fresh home as Use does, and sets the account's switch off and
// back. With AGENTRY_LIVE_VOICE_HOSTED it runs an AI Edit In Agentry and
// waits for its version to be reviewed. The account keeps the last voice;
// its switch is put back as it was.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/knowledge"
	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/voice"
)

func TestLiveVoice(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_VOICE_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_VOICE_TOKEN")
	if reg == "" || token == "" {
		t.Skip("set AGENTRY_LIVE_VOICE_REGISTRY and _TOKEN to run the live Writing Voice check")
	}
	var hostedFinal *VoiceHostedView
	finals := make(chan *VoiceHostedView, 4)
	emit := emitEvent
	emitEvent = func(_ *App, name string, data ...any) {
		if name == eventVoiceHosted && len(data) > 0 {
			if v, ok := data[0].(*VoiceHostedView); ok && v.Final {
				finals <- v
			}
		}
	}
	t.Cleanup(func() { emitEvent = emit })

	api := func(home string) *VoiceAPI {
		env := knowledge.Env{
			Home: func() (string, error) { return home, nil }, Keys: &knowledgeKeys{m: map[string]string{}},
			TokenFor: func(u string) string { return map[bool]string{true: token}[strings.TrimRight(u, "/") == reg] },
			HTTP:     registry.HTTPClient,
		}
		app := &App{ctx: context.Background(), confirm: func(context.Context, string, string) (bool, error) { return true, nil }}
		return &VoiceAPI{app: app, run: &RunAPI{app: app, knowledgeEnv: &env, runnerHomeDir: home}}
	}
	home := t.TempDir()
	t.Setenv(policy.HomeEnv, home)
	v := api(home)

	before, err := v.View(reg)
	if err != nil || !before.SignedIn || !before.AccountOffered {
		t.Fatalf("the registry keeps no voices for this token: %+v, %v", before, err)
	}
	wasOn := before.Account != nil && before.Account.AlwaysAttach

	text := "# Live check voice (" + time.Now().UTC().Format(time.RFC3339) + ")\n\nWrite in short, plain sentences. Name who does what. Prefer concrete words to abstract ones.\n"
	saved, err := v.SaveLocal(reg, text)
	if err != nil || !saved.Created {
		t.Fatalf("keep here: %+v, %v", saved, err)
	}
	if _, err := v.Submit(reg); err != nil {
		t.Fatalf("submit: %v", err)
	}
	live := waitLive(t, v, reg, saved.Voice.Digest)
	t.Logf("submitted and live: %s, %d bytes", live.Digest, live.Bytes)

	// Use on another computer: a second, fresh runner home.
	other := api(t.TempDir())
	used, err := other.UseAccount(reg)
	if err != nil || used.Voice.Digest != live.Digest || used.Voice.Source != "download" {
		t.Fatalf("use: %+v, %v", used, err)
	}

	if a, err := v.SetAccountAttach(reg, !wasOn); err != nil || a.AlwaysAttach == wasOn {
		t.Fatalf("the account's switch: %+v, %v", a, err)
	}
	if _, err := v.SetAccountAttach(reg, wasOn); err != nil {
		t.Fatalf("put the switch back: %v", err)
	}

	if os.Getenv("AGENTRY_LIVE_VOICE_HOSTED") == "" {
		return
	}
	fastPoll(t)
	avail, err := v.HostedAvailability(reg, "edit")
	if err != nil || !avail.Available {
		t.Fatalf("an AI Edit In Agentry is not available: %+v, %v", avail, err)
	}
	start, err := v.StartHosted(reg, "edit", nil, "Add one rule: keep paragraphs to three sentences at most.")
	if err != nil || start.Refusal != "" || start.Run == nil {
		t.Fatalf("start: %+v, %v", start, err)
	}
	select {
	case hostedFinal = <-finals:
	case <-time.After(25 * time.Minute):
		t.Fatal("the AI Edit In Agentry did not finish")
	}
	if !hostedFinal.Applied || !hostedFinal.Settled || hostedFinal.SettledCredits != avail.Credits {
		t.Fatalf("the AI Edit In Agentry: %+v", hostedFinal)
	}
	t.Logf("AI Edit In Agentry %s settled %d credits", hostedFinal.ID, hostedFinal.SettledCredits)
	edited := waitLive(t, v, reg, "")
	if edited.Digest == live.Digest || edited.Source != "ai-edit" {
		t.Fatalf("the AI Edit's version did not go live: %+v", edited)
	}
}

// waitLive polls the account until its live version has digest (any new
// live version when digest is ""), failing on a flag.
func waitLive(t *testing.T, v *VoiceAPI, reg, digest string) *voice.Version {
	t.Helper()
	var first string
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		view, err := v.View(reg)
		if err != nil {
			t.Fatal(err)
		}
		a := view.Account
		if first == "" && a != nil && a.Live != nil {
			first = a.Live.Digest
		}
		if a != nil && a.Pending != nil && a.Pending.Review == "flagged" {
			t.Fatalf("AI Review flagged the version: %+v", a.Pending.Findings)
		}
		if a != nil && a.Live != nil && a.Pending == nil && (a.Live.Digest == digest || (digest == "" && a.Live.Source == "ai-edit")) {
			return a.Live
		}
		time.Sleep(3 * time.Second)
	}
	t.Fatalf("no live version %s after five minutes", digest)
	return nil
}
