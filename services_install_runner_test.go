package main

import (
	"context"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/installed"

	"github.com/blaktron/agentry-desktop/internal/install"
)

// The one-click install's gates (agentry-desktop#146, D-031) are native
// dialogs: a signed, scanned release within its declared capabilities
// raises none; an unsigned or unscanned one raises "Install anyway?"; a
// first install that declares egress or exec, or an update that widens,
// raises the contract itself; the first No is the refusal.
func TestInstallRunnerGatesAreNativeDialogs(t *testing.T) {
	var asked []string
	answer := true
	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		asked = append(asked, title+"|"+message)
		return answer, nil
	}}
	i := &InstallAPI{app: app, engine: install.NewEngine()}

	clean := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.0.0"}
	if err := i.consentForRunner(clean); err != nil || len(asked) != 0 {
		t.Fatalf("a clean plan raised a dialog: %v %v", err, asked)
	}

	first := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.0.0", NeedsCapabilityConfirm: true,
		Capabilities: installed.CapSnapshot{EgressHosts: []string{"api.example.com"}, ExecCommands: []string{"jq"}, FSRead: "workspace"}}
	if err := i.consentForRunner(first); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || !strings.Contains(asked[0], "Accept what acme/demo may do?") || !strings.Contains(asked[0], "api.example.com") || !strings.Contains(asked[0], "exec: jq") || !strings.Contains(asked[0], "filesystem read: workspace") {
		t.Fatalf("the contract dialog: %v", asked)
	}

	asked = nil
	widened := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.1.0", PrevVersion: "1.0.0", RequiresWidenedConfirm: true,
		Diff: &installed.Diff{Widened: []string{"network egress: added api.other.example"}}}
	unsigned := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.0.0", Unsigned: true}
	unscanned := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.0.0", Unscanned: true, Unsigned: false}
	for _, p := range []*install.Plan{widened, unsigned, unscanned} {
		if err := i.consentForRunner(p); err != nil {
			t.Fatal(err)
		}
	}
	if len(asked) != 3 || !strings.Contains(asked[0], "Updating from 1.0.0 to 1.1.0 widens") || !strings.Contains(asked[0], "api.other.example") ||
		!strings.Contains(asked[1], "Install anyway?|This install accepts a release with no attestation") ||
		!strings.Contains(asked[2], "admitted without a scan") {
		t.Fatalf("dialogs: %v", asked)
	}

	// No: the refusal names the dialog, and the second gate is never reached.
	asked = nil
	answer = false
	both := &install.Plan{Namespace: "acme", Name: "demo", Version: "1.0.0", Unsigned: true, NeedsCapabilityConfirm: true}
	err := i.consentForRunner(both)
	if err == nil || !strings.Contains(err.Error(), "not confirmed") || len(asked) != 1 {
		t.Fatalf("declined: err %v asked %v", err, asked)
	}
	// An unreachable registry is a refusal before any plan exists.
	if _, err := i.InstallRunner("https://127.0.0.1:1", "acme/demo"); err == nil {
		t.Fatal("an unreachable registry installed")
	}

	// The bound Confirm cannot be the way around the dialogs: a runner
	// plan the page prepared and confirms with every box ticked gets the
	// plan's own gates raised here, and a No installs nothing and leaves
	// nothing pending (D-030, D-031).
	asked = nil
	answer = false
	i.engine.AddPendingForTest(install.Plan{ID: "runner-plan", Target: "runner", Namespace: "acme", Name: "demo", Version: "1.0.0", NeedsCapabilityConfirm: true,
		Capabilities: installed.CapSnapshot{ExecCommands: []string{"rm"}}})
	_, err = i.Confirm("runner-plan", true, true, true)
	if err == nil || !strings.Contains(err.Error(), "not confirmed") || len(asked) != 1 || !strings.Contains(asked[0], "exec: rm") {
		t.Fatalf("Confirm on a runner plan with the page's booleans: err %v asked %v", err, asked)
	}
	if _, ok := i.engine.Plan("runner-plan"); ok {
		t.Fatal("a declined runner plan was left pending")
	}
}
