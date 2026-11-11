package app

import (
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry/pkg/managedsettings"

	"github.com/blaktron/agentry-cli/pkg/managed"
)

// The machine settings reach the page: where they were read, the pinned
// registry, the organization they require and, while the person is not
// signed in to it as a member, the sentence the app opens on.
func TestTheMachineSettingsReachThePage(t *testing.T) {
	if machineView(managed.Effective{}) != nil {
		t.Fatal("a computer with no machine settings has a machine view")
	}
	m := &managed.Machine{Where: "/etc/agentry/managed.json", Registry: "https://registry.example.com", RequireOrg: "acme"}
	v := machineView(managed.Effective{Machine: m})
	if v.Where != m.Where || v.Registry != "https://registry.example.com" || v.RequireOrg != "acme" {
		t.Fatalf("view %+v", v)
	}
	if !strings.Contains(v.Refusal, "requires you to be signed in to https://registry.example.com as an active member of acme") {
		t.Fatalf("refusal %q", v.Refusal)
	}
	if strings.Contains(v.Refusal, "agentry login") {
		t.Fatalf("the app's sentence names a terminal command: %q", v.Refusal)
	}
	member := managed.State{Registry: "https://registry.example.com", Document: managedsettings.Document{Org: "acme", Version: 3, IssuedAt: time.Now()}}
	if v := machineView(managed.Effective{Machine: m, Documents: []managed.State{member}}); v.Refusal != "" {
		t.Fatalf("a member is refused: %q", v.Refusal)
	}
}

// Settings' landing names the machine settings by where they were read,
// never with a version they do not have.
func TestTheLandingNamesTheMachineSettings(t *testing.T) {
	m := &managed.Machine{Where: "/etc/agentry/managed.json", Settings: map[string]managedsettings.Setting{managedsettings.UnsafeMode: {Locked: true}}}
	doc := managed.State{Document: managedsettings.Document{Schema: managedsettings.Schema, Org: managed.MachineOrg, Settings: m.Settings}}
	views := managedViews(managed.Effective{Machine: m, Documents: []managed.State{doc}})
	if len(views) != 1 || views[0].Summary != "set by this computer's administrator (/etc/agentry/managed.json)" {
		t.Fatalf("views %+v", views)
	}
	probs := managedProblems(managed.Effective{Problems: []managed.Problem{{Machine: true, Err: errStr("the file is writable by its group")}}})
	if len(probs) != 1 || !strings.Contains(probs[0], "until its administrator corrects them") {
		t.Fatalf("problems %v", probs)
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }
