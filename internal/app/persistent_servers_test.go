package app

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/build"
)

// wordServer is a package server with no network, the kind that may see the
// share.
var wordServer = build.MCPServerRow{Source: "pypi", Package: "office-word-mcp-server", Version: "1.1.11"}

// serverEntry is the view's roster entry for the agent's one server.
func serverEntry(t *testing.T, v *PersistentAgentView) PersistentRosterEntry {
	t.Helper()
	for _, e := range v.Roster {
		if e.Kind == string(persistent.KindServer) {
			return e
		}
	}
	t.Fatalf("no server on the roster: %+v", v.Roster)
	return PersistentRosterEntry{}
}

// A server's "Can see your share folder" switch is a roster change like any
// other: turning it on puts the server's sentence that says it sees the
// share to the person in the native dialog, a No changes nothing, a Yes
// turns it on, and turning it off asks nothing.
func TestAServersShareSwitchAsksInTheNativeDialog(t *testing.T) {
	rig := newPersistentRig(t)
	v, err := rig.p.AddServer("assistant", wordServer)
	if err != nil {
		t.Fatal(err)
	}
	e := serverEntry(t, v)
	if e.ShareSwitch == "" || e.Share || e.ShareRefusal != "" {
		t.Fatalf("a package server with no network offers no switch, or it is on: %+v", e)
	}
	before := len(rig.dialogs())
	rig.answer = false
	if _, err := rig.p.Configure("assistant", PersistentChange{Switch: &PersistentSwitch{ID: e.ShareSwitch, On: true}}); !errors.Is(err, errNotConfirmed) {
		t.Fatalf("a No: %v", err)
	}
	d := rig.dialogs()
	if len(d) != before+1 || !strings.Contains(d[before], "sees your share folder (read only, and writes new files into it)") {
		t.Fatalf("the dialog: %q", d[before:])
	}
	cfg, err := persistent.LoadConfig(rig.home, "assistant")
	if err != nil || cfg.Switches[e.ShareSwitch] {
		t.Fatalf("a No turned the switch on (%v)", err)
	}
	rig.answer = true
	v, err = rig.p.Configure("assistant", PersistentChange{Switch: &PersistentSwitch{ID: e.ShareSwitch, On: true}})
	if err != nil {
		t.Fatal(err)
	}
	if e := serverEntry(t, v); !e.Share || !strings.Contains(e.Line, "sees your share folder") {
		t.Fatalf("after a Yes: %+v", e)
	}
	asked := len(rig.dialogs())
	v, err = rig.p.Configure("assistant", PersistentChange{Switch: &PersistentSwitch{ID: e.ShareSwitch, On: false}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rig.dialogs()) != asked || serverEntry(t, v).Share {
		t.Fatalf("turning it off asked, or left it on: %+v", serverEntry(t, v))
	}
}

// A remote server and a server that reaches the network are never given the
// share: the view offers them no switch and says why, and a switch asked
// for anyway is refused before any dialog.
func TestAServerThatMayNotSeeTheShareSaysWhy(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  build.MCPServerRow
		why  string
	}{
		{"remote", build.MCPServerRow{Source: "url", URL: "https://mcp.example.com/mcp", Auth: "none"}, "remote server"},
		{"egress", build.MCPServerRow{Source: "npm", Package: "fetcher", Version: "1.0.0", Egress: []string{"api.example.com"}}, "reaches api.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newPersistentRig(t)
			v, err := rig.p.AddServer("assistant", tc.row)
			if err != nil {
				t.Fatal(err)
			}
			e := serverEntry(t, v)
			if e.ShareSwitch != "" || !strings.Contains(e.ShareRefusal, tc.why) {
				t.Fatalf("the entry: %+v", e)
			}
			asked := len(rig.dialogs())
			if _, err := rig.p.Configure("assistant", PersistentChange{Switch: &PersistentSwitch{ID: e.ID + ":share", On: true}}); err == nil || !strings.Contains(err.Error(), tc.why) {
				t.Fatalf("the switch: %v", err)
			}
			if len(rig.dialogs()) != asked {
				t.Fatal("a refused switch asked")
			}
		})
	}
}

// A running agent's roster is its supervisor's: a server installing, and one
// whose install failed with its reason, reach the view as they are.
func TestAServersInstallStateReachesTheView(t *testing.T) {
	rig := newPersistentRig(t)
	if _, err := rig.p.SetEnabled("assistant", true); err != nil {
		t.Fatal(err)
	}
	rig.agent.roster = persistent.Roster{
		{ID: "server:pypi-a", Kind: persistent.KindServer, Name: "pypi:a@1", GrantedBy: persistent.GrantedByYou, State: persistent.StateInstalling, On: true},
		{ID: "server:pypi-b", Kind: persistent.KindServer, Name: "pypi:b@1", GrantedBy: persistent.GrantedByYou, State: persistent.StateFailed, On: true, Reason: "not found in the configured registry"},
	}
	if _, err := rig.p.Start("assistant"); err != nil {
		t.Fatal(err)
	}
	v, err := rig.p.Get("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Roster) != 2 || v.Roster[0].State != "installing" || v.Roster[1].State != "failed" || v.Roster[1].Reason != "not found in the configured registry" {
		t.Fatalf("the roster: %+v", v.Roster)
	}
}

// A stopped agent's share sentence, and the one its start asks to agree
// to, say what the share check does under the runner policy the app runs
// it with: checked with ClamAV when the policy's malware check is on, read
// unchecked when it is off.
func TestAStoppedAgentsShareSentenceFollowsTheMalwareCheck(t *testing.T) {
	for _, on := range []bool{false, true} {
		rig := newPersistentRig(t)
		polFile, err := policy.Resolve("")
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := policy.EnsureDefault(polFile.Path); err != nil {
			t.Fatal(err)
		}
		if on {
			raw, err := os.ReadFile(polFile.Path)
			if err != nil {
				t.Fatal(err)
			}
			turned := strings.Replace(string(raw), "\nmalwareCheck: false\n", "\nmalwareCheck: true\n", 1)
			if turned == string(raw) {
				t.Fatal("the default policy has no malwareCheck: false line to turn on")
			}
			if err := os.WriteFile(polFile.Path, []byte(turned), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		want := "unchecked"
		if on {
			want = "checked for malware with ClamAV"
		}
		v, err := rig.p.SetEnabled("assistant", true)
		if err != nil {
			t.Fatal(err)
		}
		if v.Roster[0].ID != persistent.EntryShare || !strings.Contains(v.Roster[0].Line, want) {
			t.Fatalf("malwareCheck %v, the stopped view's share: %q", on, v.Roster[0].Line)
		}
		rig.answer = false
		_, _ = rig.p.Start("assistant")
		if d := rig.dialogs(); len(d) != 1 || !strings.Contains(d[0], want) {
			t.Fatalf("malwareCheck %v, the start's dialog: %q", on, d)
		}
	}
}

// A failed install shows on a stopped agent with its reason, and Retry's
// two changes, the server off and on again, clear the failure without
// asking anything the person had agreed to.
func TestRetryingAFailedServerClearsItsFailure(t *testing.T) {
	rig := newPersistentRig(t)
	v, err := rig.p.AddServer("assistant", wordServer)
	if err != nil {
		t.Fatal(err)
	}
	id := serverEntry(t, v).ID
	cfg, err := persistent.LoadConfig(rig.home, "assistant")
	if err != nil {
		t.Fatal(err)
	}
	cfg.ServerFailures = map[string]persistent.ServerFailure{id: {Reason: "not found in the configured registry"}}
	if err := persistent.SaveConfig(rig.home, "assistant", cfg); err != nil {
		t.Fatal(err)
	}
	v, err = rig.p.Get("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if e := serverEntry(t, v); e.State != "failed" || e.Reason != "not found in the configured registry" {
		t.Fatalf("the failed entry: %+v", e)
	}
	asked := len(rig.dialogs())
	for _, on := range []bool{false, true} {
		if v, err = rig.p.Configure("assistant", PersistentChange{Switch: &PersistentSwitch{ID: id, On: on}}); err != nil {
			t.Fatal(err)
		}
	}
	if e := serverEntry(t, v); e.State != "on" || e.Reason != "" || len(rig.dialogs()) != asked {
		t.Fatalf("after Retry: %+v, %d dialogs", e, len(rig.dialogs())-asked)
	}
}
