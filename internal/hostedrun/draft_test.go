package hostedrun

import (
	"strings"
	"testing"
)

func TestDecideDraft(t *testing.T) {
	terms := &DraftTerms{Enabled: true, HoldCredits: 41, MaxSearches: 10, CreditsPerSearch: 1, AIReviewCredits: 1}
	hold := &TierPrice{HoldCredits: 41}
	scopes := []string{ScopeRunRead, ScopeRunWrite}
	ready := DraftFacts{SignedIn: true, Runs: true, Scopes: scopes, ScopesKnown: true, Terms: terms, StatusKnown: true, Hold: hold}
	with := func(edit func(*DraftFacts)) DraftFacts { f := ready; edit(&f); return f }
	cases := []struct {
		name               string
		f                  DraftFacts
		offered, available bool
		reason             string
	}{
		{"signed out", DraftFacts{}, true, false, "needs an account"},
		{"no runs extension", with(func(f *DraftFacts) { f.Runs = false }), false, false, ""},
		{"scopes unknown", with(func(f *DraftFacts) { f.ScopesKnown = false }), true, false, "did not say what the stored token may do"},
		{"an old token", with(func(f *DraftFacts) { f.Scopes = []string{"agent:read"} }), true, false, "does not carry run:read and run:write"},
		{"no status", with(func(f *DraftFacts) { f.StatusKnown = false }), true, false, "did not answer"},
		{"no draft kind", with(func(f *DraftFacts) { f.Terms = nil }), true, false, "not taking drafts"},
		{"drafts off", with(func(f *DraftFacts) { f.Terms = &DraftTerms{} }), true, false, "not taking drafts"},
		{"no caller line", with(func(f *DraftFacts) { f.Hold = nil }), true, false, "not taking drafts"},
		{"a build in flight", with(func(f *DraftFacts) { f.Active = &Run{Kind: KindKnowledgeBuild} }), true, false, "already has a build In Agentry"},
		{"a draft in flight", with(func(f *DraftFacts) { f.Active = &Run{Kind: KindDraft} }), true, false, "already has a draft In Agentry"},
		{"a run in flight", with(func(f *DraftFacts) { f.Active = &Run{} }), true, false, "already has a run In Agentry"},
		{"ready", ready, true, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := DecideDraft(c.f)
			if o.Offered != c.offered || o.Available != c.available || !strings.Contains(o.Reason, c.reason) {
				t.Fatalf("%+v", o)
			}
			if o.Offered && !o.Available && o.Remedy == "" {
				t.Fatal("an unavailable draft says what to do")
			}
		})
	}
}

// The site's protected sentences, word for word (agentry-web
// lib/__tests__/runs.test.ts).
func TestDraftSentencesAreTheSites(t *testing.T) {
	got := DraftHoldSentence(41, DraftTerms{MaxSearches: 10, CreditsPerSearch: 1, AIReviewCredits: 1})
	want := "This draft holds 41 credits and settles from what it uses: its model's work, up to 10 web searches at 1 credit each, and 1 credit for AI Review, never more than it holds."
	if got != want {
		t.Fatalf("got %q", got)
	}
	if got := DraftHoldSentence(0, DraftTerms{}); !strings.Contains(got, "holds no credits") {
		t.Fatalf("unmetered: %q", got)
	}
	two, one := int64(2), int64(1)
	if got := strings.Join(DraftSettledParts(&Draft{Searched: 2, SearchCredits: &two, AIReviewCredits: &one}), ", "); got != "2 for 2 searches, 1 for AI Review" {
		t.Fatalf("parts %q", got)
	}
	if len(DraftSettledParts(&Draft{})) != 0 {
		t.Fatal("a part the record did not write is not guessed")
	}
}

func TestCheckDraftHoldsTheWordsToTheRegistrysBounds(t *testing.T) {
	d, h, err := CheckDraft("  Summarise notes. ", []string{"", " The services   or sites it works with: Slack\n", "  "})
	if err != nil || d != "Summarise notes." || len(h) != 1 || h[0] != "The services or sites it works with: Slack" {
		t.Fatalf("%q %q %v", d, h, err)
	}
	for name, c := range map[string]struct {
		d string
		h []string
	}{
		"empty":           {" ", nil},
		"long":            {strings.Repeat("a", MaxDraftDescriptionBytes+1), nil},
		"not text":        {"a\xff", nil},
		"a long hint":     {"a", []string{strings.Repeat("h", MaxDraftHintBytes+1)}},
		"too many hints":  {"a", strings.Split(strings.Repeat("h,", MaxDraftHints+1), ",")[:MaxDraftHints+1]},
		"a hint not text": {"a", []string{"\xff"}},
	} {
		if _, _, err := CheckDraft(c.d, c.h); err == nil {
			t.Errorf("%s: not refused", name)
		}
	}
}
