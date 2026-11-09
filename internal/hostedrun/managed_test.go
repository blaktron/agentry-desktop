package hostedrun

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/managedsettings"

	"github.com/blaktron/agentry-cli/pkg/managed"
)

func holding(t *testing.T, settings map[string]managedsettings.Setting) {
	t.Helper()
	managed.Enforce(managed.ClientRows...)
	was := currentManaged
	t.Cleanup(func() { currentManaged = was })
	doc := managedsettings.Document{Registry: "https://r.example", Org: "acme", Version: 2, Settings: settings}
	currentManaged = func() managed.Effective {
		return managed.Effective{Documents: []managed.State{{Registry: doc.Registry, Document: doc}}}
	}
}

func setting(v string) managedsettings.Setting {
	return managedsettings.Setting{Value: json.RawMessage(v), Locked: true}
}

// A run In Agentry, and a hosted schedule, that the managed settings held
// here refuse are refused before anything reaches the registry: the
// client's base is an address nothing listens on.
func TestManagedSettingsRefuseBeforeAnythingIsSent(t *testing.T) {
	c := New("http://127.0.0.1:1", testToken)
	ctx := t.Context()

	holding(t, map[string]managedsettings.Setting{managedsettings.RunWhere: setting(`["local"]`)})
	_, err := c.Create(ctx, CreateRequest{Ref: "acme/reporter@1.0.0"})
	if r := RefusalOf(err); r == nil || r.Slug != ManagedSlug || !strings.Contains(r.Sentence(), "acme's managed settings (run.where allows local)") {
		t.Fatalf("Create = %v", err)
	}
	if _, err := c.CreateSchedule(ctx, ScheduleRequest{}); RefusalOf(err) == nil || RefusalOf(err).Slug != ManagedSlug {
		t.Fatalf("CreateSchedule = %v", err)
	}

	holding(t, map[string]managedsettings.Setting{managedsettings.Schedules: setting(`"forbidden"`)})
	if _, err := c.RunScheduleNow(ctx, "s1"); RefusalOf(err) == nil || !strings.Contains(err.Error(), "(schedules)") {
		t.Fatalf("RunScheduleNow = %v", err)
	}
	if _, err := c.UpdateSchedule(ctx, "s1", ScheduleRequest{}); RefusalOf(err) == nil {
		t.Fatalf("UpdateSchedule = %v", err)
	}
	// Schedules forbidden says nothing of a single run In Agentry.
	if _, err := c.Create(ctx, CreateRequest{Ref: "acme/reporter@1.0.0"}); RefusalOf(err) != nil && RefusalOf(err).Slug == ManagedSlug {
		t.Fatalf("a run was refused by a lock on schedules: %v", err)
	}
}
