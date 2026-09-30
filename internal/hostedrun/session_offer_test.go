package hostedrun

import (
	"strings"
	"testing"
)

// Talk to it In Agentry as a table (plan hosted-sessions.md §6, D12 as
// amended; agentry-desktop#403): offered only where the release declares the
// interactive mode, the registry's status runs sessions and says the
// caller's payer may start one; a payer on Standard sees why, with the way
// up. Every fact is one the app already has (D-022): nothing here probes a
// session route.
func TestSessionOfferMatrix(t *testing.T) {
	yes, no := true, false
	full := []string{ScopeRunRead, ScopeRunWrite, "agent:read"}
	base := Facts{
		SignedIn: true, Extension: true, Modes: []string{"interactive"}, Scopes: full, ScopesKnown: true,
		Enabled: true, StatusKnown: true, SessionsEnabled: true, SessionAllowed: &yes,
	}
	with := func(f func(*Facts)) Facts {
		c := base
		f(&c)
		return c
	}
	cases := []struct {
		name      string
		facts     Facts
		offered   bool
		available bool
		upgrade   bool
		says      string
	}{
		{"everything holds", base, true, true, false, ""},
		{"both modes", with(func(f *Facts) { f.Modes = []string{"autonomous", "interactive"} }), true, true, false, ""},
		{"autonomous only", with(func(f *Facts) { f.Modes = []string{"autonomous"} }), false, false, false, ""},
		{"not signed in", with(func(f *Facts) { f.SignedIn = false }), false, false, false, ""},
		{"no runs extension", with(func(f *Facts) { f.Extension = false }), false, false, false, ""},
		// The registry does not run sessions, or predates them: the choice
		// is absent and the local modes are as they were.
		{"sessions off", with(func(f *Facts) { f.SessionsEnabled = false }), false, false, false, ""},
		{"a registry from before sessions", with(func(f *Facts) { f.SessionsEnabled, f.SessionAllowed = false, nil }), false, false, false, ""},
		{"no caller line", with(func(f *Facts) { f.SessionAllowed = nil }), false, false, false, ""},
		{"status unreadable", with(func(f *Facts) { f.StatusKnown, f.SessionsEnabled, f.SessionAllowed = false, false, nil }), false, false, false, ""},
		// A payer on Standard (agentry-enterprise#286): offered, not
		// available, the registry's reason and the way up.
		{"a Standard payer", with(func(f *Facts) { f.SessionAllowed = &no }), true, false, true, "Pro"},
		{"a Standard payer with an old token", with(func(f *Facts) { f.SessionAllowed, f.Scopes = &no, []string{"agent:read"} }), true, false, true, "Pro"},
		{"an old token", with(func(f *Facts) { f.Scopes = []string{"agent:read"} }), true, false, false, "run:read"},
		{"scopes unreadable", with(func(f *Facts) { f.ScopesKnown, f.Scopes = false, nil }), true, false, false, "did not say"},
		{"a declared secret, no vault", with(func(f *Facts) { f.DeclaredSecrets = []string{"PAGERDUTY_TOKEN"} }), true, false, false, "PAGERDUTY_TOKEN"},
		{"a declared secret, the vault and the scope", with(func(f *Facts) {
			f.DeclaredSecrets, f.Vault, f.Scopes = []string{"PAGERDUTY_TOKEN"}, true, append(full, ScopeSecretWrite)
		}), true, true, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := DecideSession(c.facts)
			if o.Offered != c.offered || o.Available != c.available || o.Upgrade != c.upgrade {
				t.Fatalf("offered=%v available=%v upgrade=%v, want %v/%v/%v (%+v)", o.Offered, o.Available, o.Upgrade, c.offered, c.available, c.upgrade, o)
			}
			if c.says != "" && !strings.Contains(o.Reason, c.says) {
				t.Fatalf("reason %q, want it to mention %q", o.Reason, c.says)
			}
			if o.Offered && !o.Available && (o.Reason == "" || o.Remedy == "") {
				t.Fatalf("unavailable without a reason and a way out: %+v", o)
			}
			if !o.Offered && (o.Reason != "" || o.Remedy != "") {
				t.Fatalf("a choice that is not offered says nothing: %+v", o)
			}
		})
	}
	// The autonomous choice is not moved by any session fact.
	for _, f := range []Facts{with(func(f *Facts) { f.Modes = []string{"autonomous"}; f.SessionAllowed = &no }), with(func(f *Facts) { f.Modes = []string{"autonomous"}; f.SessionsEnabled = false })} {
		if o := Decide(f); !o.Offered || !o.Available {
			t.Fatalf("a session fact moved the autonomous offer: %+v", o)
		}
	}
}
