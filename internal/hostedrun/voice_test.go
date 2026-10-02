package hostedrun

import (
	"strings"
	"testing"
	"time"
)

// The Voice Writer In Agentry is offered by one rule (plan writing-voice.md
// §6, D9, D17; agentry-desktop#510): absent signed out or where the registry
// keeps no voices, and otherwise offered, chosen only when nothing stands in
// the way, with Standard's spent cycle said with its date and the upgrade.
func TestTheVoiceWriterInAgentryIsOfferedByOneRule(t *testing.T) {
	next := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	scopes := []string{ScopeRunRead, ScopeRunWrite, ScopeVoiceRead, ScopeVoiceWrite}
	ready := func() VoiceFacts {
		return VoiceFacts{SignedIn: true, Voice: true, Runs: true, Scopes: scopes, ScopesKnown: true,
			Terms: &VoiceTerms{Enabled: true, Credits: 20}, StatusKnown: true, HasVoice: true,
			Caller: &CallerVoice{Allowed: true, Limited: true, Max: 1}}
	}
	cases := []struct {
		name      string
		facts     func() VoiceFacts
		job       string
		offered   bool
		available bool
		reason    string
		upgrade   bool
	}{
		{"signed out", func() VoiceFacts { f := ready(); f.SignedIn = false; return f }, VoiceCreate, false, false, "", false},
		{"no voices kept here", func() VoiceFacts { f := ready(); f.Voice = false; return f }, VoiceCreate, false, false, "", false},
		{"no runs extension", func() VoiceFacts { f := ready(); f.Runs = false; return f }, VoiceCreate, true, false, "does not run anything In Agentry", false},
		{"token without the voice scopes", func() VoiceFacts { f := ready(); f.Scopes = scopes[:2]; return f }, VoiceCreate, true, false, "voice:write", false},
		{"no Voice Writer here", func() VoiceFacts { f := ready(); f.Terms.Enabled = false; return f }, VoiceCreate, true, false, "not running the Voice Writer", false},
		{"nothing to improve", func() VoiceFacts { f := ready(); f.HasVoice = false; return f }, VoiceImprove, true, false, "no Writing Voice to improve", false},
		{"a create needs no voice", func() VoiceFacts { f := ready(); f.HasVoice = false; return f }, VoiceCreate, true, true, "", false},
		{"Standard's cycle spent", func() VoiceFacts {
			f := ready()
			f.Caller = &CallerVoice{Allowed: false, Limited: true, Used: 1, Max: 1, NextAt: &next}
			return f
		}, VoiceEdit, true, false, "this cycle's runs are used. The next is yours on 28 October 2026.", true},
		{"a run in flight", func() VoiceFacts { f := ready(); f.Active = &Run{Kind: KindKnowledgeBuild}; return f }, VoiceCreate, true, false, "already has a build", false},
		{"ready", ready, VoiceEdit, true, true, "", false},
	}
	for _, c := range cases {
		o := DecideVoice(c.facts(), c.job)
		if o.Offered != c.offered || o.Available != c.available || o.Upgrade != c.upgrade || !strings.Contains(o.Reason, c.reason) {
			t.Errorf("%s: got %+v", c.name, o)
		}
		if o.Offered && !o.Available && o.Reason == "" {
			t.Errorf("%s: unavailable with no reason", c.name)
		}
	}
}

func TestAVoiceWriterRunReadsAsOne(t *testing.T) {
	r := &Run{Kind: KindVoiceBuild}
	if !r.IsVoiceBuild() || r.IsBuild() || ActiveWhat(r) != "a Voice Writer run" {
		t.Fatalf("%+v", r)
	}
	if VoiceNoteSentence("gone") == "" || VoiceNoteSentence("unknown") != "" {
		t.Fatal("the notes")
	}
}
