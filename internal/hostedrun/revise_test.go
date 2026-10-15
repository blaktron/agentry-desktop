package hostedrun

import (
	"strings"
	"testing"
)

func TestCheckRevisionHoldsTheTextsToTheRegistrysRule(t *testing.T) {
	in, pr, err := CheckRevision("  Add a weekly summary.\r\n  ", "")
	if err != nil || in != "Add a weekly summary." || pr != "" {
		t.Fatalf("%q %q %v", in, pr, err)
	}
	if _, _, err := CheckRevision("Line one\n\tindented", "It misses dates."); err != nil {
		t.Fatalf("line breaks and tabs: %v", err)
	}
	for what, c := range map[string][2]string{
		"both empty":          {" ", "\n"},
		"a control character": {"Add\x07 a summary.", ""},
		"not text":            {"", "\xff\xfe"},
		"over the bound":      {strings.Repeat("p", MaxRevisionTextBytes+1), ""},
	} {
		if _, _, err := CheckRevision(c[0], c[1]); err == nil {
			t.Errorf("%s: accepted", what)
		}
	}
}

func TestDecideReviseIsADraftsOfferGatedOnThePayer(t *testing.T) {
	ready := DraftFacts{SignedIn: true, Runs: true, ScopesKnown: true, Scopes: []string{ScopeRunRead, ScopeRunWrite},
		StatusKnown: true, Terms: &DraftTerms{Enabled: true}, Hold: &TierPrice{HoldCredits: 40}}
	allowed, refused := ready, ready
	allowed.Revise, refused.Revise = &CallerRevise{Allowed: true}, &CallerRevise{Allowed: false}
	if o := DecideRevise(allowed); !o.Offered || !o.Available {
		t.Fatalf("Pro: %+v", o)
	}
	if o := DecideRevise(refused); !o.Offered || o.Available || !o.Upgrade || !strings.Contains(o.Reason, "Pro and above") || !strings.Contains(o.Remedy, "on this computer") {
		t.Fatalf("Standard: %+v", o)
	}
	// A registry that sends no caller.revise offers none.
	if o := DecideRevise(ready); o.Offered {
		t.Fatalf("no caller.revise: %+v", o)
	}
	// What stops a draft stops a revision first, in a draft's words.
	busy := allowed
	busy.Active = &Run{Kind: KindDraft}
	if o := DecideRevise(busy); o.Available || !strings.Contains(o.Reason, "in flight") {
		t.Fatalf("busy: %+v", o)
	}
	if o := DecideRevise(DraftFacts{}); !o.Offered || o.Available || !strings.Contains(o.Reason, "account") {
		t.Fatalf("signed out: %+v", o)
	}
}
