package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// The Assistant's Google Workspace card is drawn with its glyph, its Set up
// is Google's (the server's and its switches'), and the view says what the
// set-up holds, never more. The Coding agent has none.
func TestTheAssistantsGoogleCardAndView(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	v, err := rig.p.Get("assistant")
	if err != nil {
		t.Fatal(err)
	}
	var gog []PersistentRosterEntry
	for _, e := range v.Roster {
		if strings.HasPrefix(e.ID, persistent.ImageServerEntryID("gog")) {
			gog = append(gog, e)
		}
	}
	if len(gog) < 2 {
		t.Fatalf("the Assistant's Google Workspace entries: %+v", gog)
	}
	for _, e := range gog {
		if !e.SetUpGoogle || e.IconKey != "mail" || e.SetUpSecret != "" || e.SetUpAccount != "" {
			t.Errorf("%s: %+v", e.ID, e)
		}
	}
	if v.Google == nil || v.Google.Client || v.Google.Account != "" {
		t.Errorf("a fresh agent's Google set-up: %+v", v.Google)
	}
	c, err := rig.p.Get(coderAgent)
	if err != nil {
		t.Fatal(err)
	}
	if c.Google != nil {
		t.Errorf("the Coding agent has a Google set-up: %+v", c.Google)
	}
	for _, e := range c.Roster {
		if e.SetUpGoogle {
			t.Errorf("a Coding agent entry sets Google up: %+v", e)
		}
	}
}

// The client file is the native picker's, read in Go and checked before
// anything else runs: a cancel changes nothing, a file that is not a
// Desktop app's client is refused, and so is a big one. The sign-in is
// refused for an address that is not an email, and both are refused for an
// agent without Google Workspace.
func TestTheGoogleSetUpRefusesBeforeAnythingRuns(t *testing.T) {
	rig := newPersistentRig(t)
	ran := false
	rig.p.googleSetUp = func(context.Context, persistent.Agent) (persistent.Google, error) {
		ran = true
		return persistent.Google{}, nil
	}
	dir := t.TempDir()
	pick := ""
	rig.p.pickFile = func() (string, error) { return pick, nil }

	if _, err := rig.p.SetGoogleClient("assistant"); err != nil {
		t.Errorf("a cancel: %v", err)
	}
	web := filepath.Join(dir, "web.json")
	if err := os.WriteFile(web, []byte(`{"web":{"client_id":"1-a.apps.googleusercontent.com","client_secret":"s"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	pick = web
	if _, err := rig.p.SetGoogleClient("assistant"); err == nil {
		t.Error("a web application's client was taken")
	}
	big := filepath.Join(dir, "big.json")
	if err := os.WriteFile(big, make([]byte, maxGoogleClientFile+1), 0o600); err != nil {
		t.Fatal(err)
	}
	pick = big
	if _, err := rig.p.SetGoogleClient("assistant"); err == nil || !strings.Contains(err.Error(), "bytes") {
		t.Errorf("a big file: %v", err)
	}
	pick = dir
	if _, err := rig.p.SetGoogleClient("assistant"); err == nil {
		t.Error("a folder was read as the client")
	}
	if ran {
		t.Error("the engine was reached for a file that is not a client")
	}
	if _, err := rig.p.GoogleSignIn("assistant", "not an address"); err == nil || !strings.Contains(err.Error(), "not an email address") {
		t.Errorf("a bad address: %v", err)
	}
	if _, err := rig.p.SetGoogleClient(coderAgent); err == nil || !strings.Contains(err.Error(), "no Google Workspace") {
		t.Errorf("the Coding agent: %v", err)
	}
	if _, err := rig.p.GoogleSignIn(coderAgent, "a@example.com"); err == nil {
		t.Error("the Coding agent signed in to Google")
	}
	rig.p.CancelGoogleSignIn("assistant")
}
