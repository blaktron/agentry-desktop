package app

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runhost"
	rhsetup "github.com/blaktron/agentry-cli/pkg/runhost/setup"

	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func newRunhostsAPI(t *testing.T) (*RunhostsAPI, string) {
	t.Helper()
	_, home := isolateHomes(t)
	app := &App{ctx: context.Background()}
	return &RunhostsAPI{app: app, run: &RunAPI{app: app, runnerHomeDir: home}}, home
}

func saveRecord(t *testing.T, home string, rec runhost.Record) {
	t.Helper()
	recs, err := runhost.LoadRecords(home)
	if err != nil {
		t.Fatal(err)
	}
	recs.Put(rec)
	if err := recs.Save(home); err != nil {
		t.Fatal(err)
	}
}

// The Runhosts list is this computer's records, with their sign-ins and
// credits token by name and never a secret; a runhost made the default is
// where the run setup starts, and the run defaults say so.
func TestRunhostsListsTheRecordsAndTheDefault(t *testing.T) {
	r, home := newRunhostsAPI(t)
	saveRecord(t, home, runhost.Record{Name: "lab", Host: "10.0.0.5", Port: 22, HostKeyFingerprint: "ED25519 SHA256:x",
		Owner: runhost.Owner{Kind: runhost.OwnerDevices}, CLIVersion: "0.7.8",
		SignIns: []runhost.SignInRecord{{Vendor: "claude", Label: "person@example.com"}},
		Credits: &runhost.CreditsRecord{TokenName: "Runhost lab (credits only)"}})
	list, err := r.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list %+v %v", list, err)
	}
	if v := list[0]; v.Name != "lab" || v.Fingerprint != "ED25519 SHA256:x" || len(v.SignIns) != 1 || v.Credits != "Runhost lab (credits only)" || v.Default {
		t.Fatalf("view %+v", v)
	}
	if err := r.SetDefault("nowhere"); err == nil {
		t.Fatal("a runhost this computer does not know was made the default")
	}
	if err := r.SetDefault("lab"); err != nil {
		t.Fatal(err)
	}
	if s, _ := settings.Load(); s.RunWhere != "runhost:lab" {
		t.Fatalf("runWhere %q", s.RunWhere)
	}
	if list, _ := r.List(); !list[0].Default {
		t.Fatal("the default runhost is not marked")
	}
	d, err := runDefaultsView()
	if err != nil || d.Runhost != "lab" || d.Where != settings.RunWhereHere {
		t.Fatalf("run defaults %+v %v", d, err)
	}
	// The page cannot set a runhost through Run defaults' own call.
	if _, err := (&SettingsAPI{app: r.app}).SetRunWhere("runhost:lab"); err == nil {
		t.Fatal("SetRunWhere took a runhost")
	}
}

// The admin sign-in is a key file the native picker returned, or a
// password; a page-named path is refused, and sudo takes the sign-in
// password when no other is given.
func TestTheAdminSignInIsHeldToThePicker(t *testing.T) {
	r, _ := newRunhostsAPI(t)
	if _, err := r.adminAuth(RunhostAdmin{User: "root", KeyFile: "/home/someone/.ssh/id_ed25519"}); err == nil || !strings.Contains(err.Error(), "file dialog") {
		t.Fatalf("a key file the picker did not return: %v", err)
	}
	if _, err := r.adminAuth(RunhostAdmin{User: "root"}); err == nil {
		t.Fatal("an admin sign-in with neither a key nor a password")
	}
	if _, err := r.adminAuth(RunhostAdmin{Password: "pw"}); err == nil {
		t.Fatal("an admin sign-in with no account")
	}
	a, err := r.adminAuth(RunhostAdmin{User: "tester", Password: "pw"})
	if err != nil || a.Password != "pw" || a.Sudo != "pw" {
		t.Fatalf("a password sign-in: %+v %v", a, err)
	}
}

// The preflight's plan is put to the person with what it found that is not
// fine, and what it will install.
func TestThePlanSaysWhatTheInstallDoes(t *testing.T) {
	s := planSentence("10.0.0.5", rhsetup.Plan{InstallDocker: true, Findings: []rhsetup.Finding{
		{Level: rhsetup.LevelOK, Name: "os", Detail: "Debian 13"},
		{Level: rhsetup.LevelNote, Name: "lxd", Detail: "LXD's daemon runs here"},
	}})
	if !strings.Contains(s, "Install Docker Engine, the agentry account") || !strings.Contains(s, "lxd: LXD's daemon runs here") || strings.Contains(s, "Debian 13") {
		t.Fatalf("plan sentence %q", s)
	}
}

// A runhost is set up only by a build that carries a released CLI, whose
// signed build the server gets; and the CLI release is checked against this
// app's own update-signing keys.
func TestSetupNeedsAReleasedCLIAndTheAppsKeys(t *testing.T) {
	r, _ := newRunhostsAPI(t)
	if carriedCLIVersion() != "" {
		t.Skip("this test binary carries a released CLI")
	}
	if _, err := r.Add(RunhostAddRequest{Name: "lab", Host: "10.0.0.5", Admin: RunhostAdmin{User: "root", Password: "pw"}}); err == nil || !strings.Contains(err.Error(), "released CLI") {
		t.Fatalf("a setup from a build without a released CLI: %v", err)
	}
	keys, err := releaseKeys()
	if err != nil {
		t.Fatal(err)
	}
	app, err := selfupdate.NewKeySet(selfupdate.KeysJSON())
	if err != nil {
		t.Fatal(err)
	}
	if keys.Empty() != (len(app.Keys()) == 0) {
		t.Fatal("the CLI release check does not take the app's own key set")
	}
}

// A runhost's Unsafe Mode is turned on only after the native confirmation.
func TestUnsafeModeOnAsksFirst(t *testing.T) {
	r, home := newRunhostsAPI(t)
	saveRecord(t, home, runhost.Record{Name: "lab", Host: "10.0.0.5", Port: 22})
	asked := ""
	r.app.confirm = func(_ context.Context, title, _ string) (bool, error) { asked = title; return false, nil }
	if err := r.SetUnsafeMode("lab", true, RunhostAdmin{User: "root", Password: "pw"}); err == nil {
		t.Fatal("Unsafe Mode went on with the confirmation refused")
	}
	if !strings.Contains(asked, "Unsafe Mode") {
		t.Fatalf("the person was asked %q", asked)
	}
}

// A run on a runhost goes through the run lock and the run log as a local
// run does, and its result names the runhost it ran on.
func TestARunOnARunhostSaysWhereItRan(t *testing.T) {
	r, _ := newRunhostsAPI(t)
	var got run.RunhostRequest
	r.run.runhostRun = func(_ context.Context, req run.RunhostRequest, _ io.Writer) (*run.Result, *run.RanOn, error) {
		got = req
		req.Say("harness: working")
		return &run.Result{Status: "ok", RunID: "r1", Ref: req.Ref}, &run.RanOn{Name: req.Runhost, Host: "10.0.0.5", Fingerprint: "ED25519 SHA256:x"}, nil
	}
	v, err := r.run.RunOnRunhost("lab", "https://registry.test", "acme/demo@1.0.0", "the task", nil, "", 0, -1, false, map[string]string{"topic": "birds"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if v.RanOn == nil || v.RanOn.Name != "lab" || v.RanOn.Fingerprint != "ED25519 SHA256:x" || v.Status != "ok" {
		t.Fatalf("result %+v %+v", v, v.RanOn)
	}
	if got.Runhost != "lab" || got.Task != "the task" || got.Intake["topic"] != "birds" {
		t.Fatalf("request %+v", got)
	}
	// The run lock is free again.
	if _, err := r.run.RunOnRunhost("lab", "https://registry.test", "acme/demo@1.0.0", "", nil, "", 0, -1, false, nil, false); err != nil {
		t.Fatalf("a second run: %v", err)
	}
}

// A runhost's credits token is made on the person's account: refused while
// the app is signed in to none, before any browser opens.
func TestCreditsNeedASignedInAccount(t *testing.T) {
	r, home := newRunhostsAPI(t)
	saveRecord(t, home, runhost.Record{Name: "lab", Host: "10.0.0.5", Port: 22})
	asked := false
	r.app.confirm = func(context.Context, string, string) (bool, error) { asked = true; return true, nil }
	if _, err := r.Credits("lab"); err == nil || !strings.Contains(err.Error(), "sign in") {
		t.Fatalf("credits signed out: %v", err)
	}
	if asked {
		t.Fatal("the person was asked before the sign-in was checked")
	}
}
