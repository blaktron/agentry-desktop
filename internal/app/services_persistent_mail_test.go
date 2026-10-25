package app

import (
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// TestTheAssistantsMailProviders: a new Assistant lists Google disabled
// and has no Google Workspace card; SetMailProvider enables it and its
// cards appear, then disables it and they go while its sign-in stays; an
// unknown provider and the Coding agent are refused.
func TestTheAssistantsMailProviders(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	gogCards := func(v *PersistentAgentView) int {
		n := 0
		for _, e := range v.Roster {
			if strings.HasPrefix(e.ID, persistent.ImageServerEntryID("gog")) {
				n++
			}
		}
		return n
	}
	v, err := rig.p.Get("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.MailProviders) != 3 || v.MailProviders[0].Key != "google" || v.MailProviders[1].Key != "microsoft" || v.MailProviders[2].Key != "fastmail" || v.MailProviders[0].Enabled || gogCards(v) != 0 {
		t.Fatalf("a new Assistant: providers %+v, %d gog cards", v.MailProviders, gogCards(v))
	}
	if v.MailProviders[0].Accounts == nil || v.MailProviders[0].SetUp == nil {
		t.Errorf("the page is given null lists: %+v", v.MailProviders[0])
	}
	if v, err = rig.p.SetMailProvider("assistant", "google", true); err != nil {
		t.Fatal(err)
	}
	if !v.MailProviders[0].Enabled || gogCards(v) < 2 {
		t.Fatalf("enabled: providers %+v, %d gog cards", v.MailProviders, gogCards(v))
	}

	cfg, err := persistent.LoadConfig(rig.home, "assistant")
	if err != nil {
		t.Fatal(err)
	}
	cfg.SignIns = map[string]persistent.ImageSignIn{"gog": {Account: "person@example.com"}}
	if err := persistent.SaveConfig(rig.home, "assistant", cfg); err != nil {
		t.Fatal(err)
	}
	if v, err = rig.p.SetMailProvider("assistant", "google", false); err != nil {
		t.Fatal(err)
	}
	if v.MailProviders[0].Enabled || gogCards(v) != 0 || strings.Join(v.MailProviders[0].Accounts, ",") != "person@example.com" {
		t.Fatalf("disabled: providers %+v, %d gog cards", v.MailProviders, gogCards(v))
	}
	if after, _ := persistent.LoadConfig(rig.home, "assistant"); after.SignIns["gog"].Account != "person@example.com" {
		t.Errorf("disabling lost the sign-in: %+v", after.SignIns)
	}

	if _, err := rig.p.SetMailProvider("assistant", "yahoo", true); err == nil || !strings.Contains(err.Error(), "google") {
		t.Errorf("an unknown provider: %v", err)
	}
	if _, err := rig.p.SetMailProvider("coder", "google", true); err == nil {
		t.Error("the Coding agent took a mail provider")
	}
	if c, _ := rig.p.Get("coder"); len(c.MailProviders) != 0 {
		t.Errorf("the Coding agent lists providers: %+v", c.MailProviders)
	}
}
