package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
	"github.com/blaktron/agentry-cli/pkg/schedule"

	"github.com/blaktron/agentry-desktop/internal/schedules"
)

func TestAVulnerableServersScheduleIsMarkedAndNeedsAnAgentryThatKeepsTheMark(t *testing.T) {
	asked := 0
	version := func(line string) func() (string, error) {
		return func() (string, error) { asked++; return line, nil }
	}

	plain := schedules.Request{Ref: "acme/x@1.0.0"}
	if err := markInUnsafeMode(&plain, version("agentry version 0.7.20")); err != nil || plain.InUnsafeMode || asked != 0 {
		t.Fatalf("a schedule with no Unsafe Mode consent: err %v, marked %v, asked %d", err, plain.InUnsafeMode, asked)
	}

	vuln := schedules.Request{Ref: "acme/x@1.0.0", AllowVulnerableMCP: true}
	err := markInUnsafeMode(&vuln, version("agentry version 0.7.37 default registry: https://secureagentry.net"))
	if err == nil || !strings.Contains(err.Error(), "v0.7.38 or later") {
		t.Fatalf("an agentry that cannot keep the mark: %v", err)
	}
	if err := markInUnsafeMode(&vuln, version("agentry version 0.7.38 default registry: https://secureagentry.net")); err != nil || !vuln.InUnsafeMode {
		t.Fatalf("0.7.38: err %v, marked %v", err, vuln.InUnsafeMode)
	}
	if err := markInUnsafeMode(&vuln, func() (string, error) { return "", errors.New("gone") }); err == nil {
		t.Fatal("a command that does not answer was taken as new enough")
	}
}

// A schedule made in Unsafe Mode is listed as held, with the managed
// settings' reason, while they forbid Unsafe Mode, and its Run now is
// refused with that reason before the agentry command is asked; a schedule
// made without Unsafe Mode is not held, and neither is held once the lock
// lifts.
func TestAScheduleMadeInUnsafeModeIsHeldWhileManagedSettingsForbidIt(t *testing.T) {
	freshConfig(t)
	home := os.Getenv(policy.HomeEnv)
	make := func(name string, unsafe bool) string {
		sc := schedule.Schedule{Name: name, Ref: "acme/" + name, Version: "1.0.0", Digest: "sha256:aa",
			Run:  schedule.RunSpec{OutDir: t.TempDir(), MaxTurns: -1, MaxCost: -1, AllowVulnerableMCP: unsafe, UnsafeMode: unsafe},
			When: schedule.Recurrence{Kind: schedule.Daily, Time: "09:00", Zone: "UTC", Start: time.Now(), End: schedule.End{Kind: schedule.EndNever}}}
		if err := schedule.StoreIn(home).Create(&sc, nil); err != nil {
			t.Fatal(err)
		}
		return sc.ID
	}
	unsafeID, plainID := make("unsafe", true), make("plain", false)
	asked := 0
	s := &ScheduleAPI{app: &App{ctx: context.Background()}, findCLI: func(context.Context) (*schedules.CLI, error) {
		asked++
		return nil, errors.New("the agentry command was asked")
	}}
	held := func() map[string]string {
		l, err := s.List()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, it := range l.Schedules {
			out[it.Name] = it.Held
		}
		return out
	}

	holdManaged(t, forbidding("acme"))
	h := held()
	if !strings.Contains(h["unsafe"], "forbidden by acme's managed settings") || h["plain"] != "" {
		t.Fatalf("held under the lock: %q", h)
	}
	err := s.RunNow(unsafeID)
	if err == nil || !strings.Contains(err.Error(), "forbidden by acme's managed settings") || !strings.Contains(err.Error(), "consents were given in Unsafe Mode") || asked != 0 {
		t.Fatalf("Run now of the held schedule: %v (agentry asked %d times)", err, asked)
	}
	if err := s.RunNow(plainID); err == nil || !strings.Contains(err.Error(), "the agentry command was asked") {
		t.Fatalf("Run now of a schedule that is not held went elsewhere: %v", err)
	}

	holdManaged(t, managed.Effective{})
	if h := held(); h["unsafe"] != "" || h["plain"] != "" {
		t.Fatalf("held once the lock lifted: %q", h)
	}
}
