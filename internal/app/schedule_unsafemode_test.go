package app

import (
	"errors"
	"strings"
	"testing"

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
