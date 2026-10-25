package app

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// TestTheFastmailSetUpFromTheApp: Fastmail is asked about the token before
// anything is kept, and a token it refuses keeps nothing and leaves the
// provider as it was; an accepted one enables Fastmail, goes into the
// keychain and never back to the page, after the agreement that names
// Fastmail's server; replacing it asks first, and a No keeps the old one;
// Remove takes it out of the keychain; the Coding agent has none of it.
func TestTheFastmailSetUpFromTheApp(t *testing.T) {
	rig := newPersistentRig(t)
	keys := &memSecrets{}
	rig.p.secrets = keys
	var asked []string
	refuse := false
	rig.p.fastmailCheck = func(_ context.Context, token string) error {
		asked = append(asked, token)
		if refuse {
			return persistent.ErrFastmailRefused
		}
		return nil
	}
	account := persistent.SecretAccount("assistant", "FASTMAIL_TOKEN")
	v, err := rig.p.Get("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if v.Fastmail == nil || v.Fastmail.Token {
		t.Fatalf("a new Assistant's Fastmail: %+v", v.Fastmail)
	}
	if _, err := rig.p.SetFastmailToken("coder", "fmu1-planted-token-1"); err == nil {
		t.Fatal("the Coding agent took a Fastmail token")
	}

	refuse = true
	if _, err := rig.p.SetFastmailToken("assistant", "fmu1-planted-token-1"); !errors.Is(err, persistent.ErrFastmailRefused) {
		t.Fatalf("a refused token: %v", err)
	}
	if _, err := keys.Get(account); err == nil {
		t.Fatal("a token Fastmail refused was kept")
	}
	if v, _ = rig.p.Get("assistant"); fastmailEnabled(v) {
		t.Fatal("a refused token enabled Fastmail")
	}

	// Accepted, but the agreement refused: nothing kept, Fastmail disabled
	// again.
	refuse, rig.answer = false, false
	if _, err := rig.p.SetFastmailToken("assistant", "fmu1-planted-token-1"); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	if _, err := keys.Get(account); err == nil {
		t.Fatal("an unagreed token was kept")
	}
	if v, _ = rig.p.Get("assistant"); fastmailEnabled(v) {
		t.Fatal("an unagreed token left Fastmail enabled")
	}
	if d := rig.dialogs(); len(d) == 0 || !strings.Contains(d[len(d)-1], "https://api.fastmail.com/mcp") {
		t.Errorf("the agreement asked: %q", d)
	}

	rig.answer = true
	if v, err = rig.p.SetFastmailToken("assistant", "fmu1-planted-token-1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := keys.Get(account); got != "fmu1-planted-token-1" || !v.Fastmail.Token || v.Fastmail.Set == "" || !fastmailEnabled(v) {
		t.Fatalf("kept: keychain %q, view %+v", got, v.Fastmail)
	}
	if b, _ := json.Marshal(v); strings.Contains(string(b), "planted-token") {
		t.Fatal("the view carries the token")
	}
	if !slices.ContainsFunc(v.Roster, func(e PersistentRosterEntry) bool {
		return e.ID == "server:url-api-fastmail-com-mcp" && e.SetUpFastmail && e.IconKey == "mail"
	}) {
		t.Errorf("Fastmail's card: %+v", v.Roster)
	}

	// Replacing asks first; a No keeps the old token and asks Fastmail
	// nothing.
	before := len(asked)
	rig.answer = false
	if _, err := rig.p.SetFastmailToken("assistant", "fmu1-planted-token-2"); !errors.Is(err, errNotConfirmed) || len(asked) != before {
		t.Fatalf("a refused replace: %v, Fastmail asked %d times", err, len(asked)-before)
	}
	if got, _ := keys.Get(account); got != "fmu1-planted-token-1" {
		t.Fatalf("a refused replace changed the keychain: %q", got)
	}
	rig.answer = true
	if _, err := rig.p.SetFastmailToken("assistant", "fmu1-planted-token-2"); err != nil {
		t.Fatal(err)
	}
	if got, _ := keys.Get(account); got != "fmu1-planted-token-2" {
		t.Fatalf("replaced: %q", got)
	}

	if v, err = rig.p.RemoveFastmailToken("assistant"); err != nil {
		t.Fatal(err)
	}
	if _, err := keys.Get(account); err == nil || v.Fastmail.Token {
		t.Fatalf("removed: %v, %+v", err, v.Fastmail)
	}
}

// fastmailEnabled says the view has the Fastmail provider enabled.
func fastmailEnabled(v *PersistentAgentView) bool {
	return slices.ContainsFunc(v.MailProviders, func(p PersistentMailProviderView) bool { return p.Key == "fastmail" && p.Enabled })
}
