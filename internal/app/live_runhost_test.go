package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/settings"
)

// TestLiveRunhostFromTheApp sets a real server up as a runhost through the
// app's own calls, runs a published release on it, and removes it. It needs a
// test server and a model key, so it runs only when asked:
//
//	AGENTRY_LIVE_RUNHOST_HOST     the server's address
//	AGENTRY_LIVE_RUNHOST_USER     its admin account (sudo)
//	AGENTRY_LIVE_RUNHOST_PASSWORD that account's password
//	AGENTRY_LIVE_RUNHOST_REGISTRY the registry the release is on, signed in
//	AGENTRY_LIVE_RUNHOST_REF      a release with no secrets
//	ANTHROPIC_API_KEY             the model's key, which travels with the run
func TestLiveRunhostFromTheApp(t *testing.T) {
	host := os.Getenv("AGENTRY_LIVE_RUNHOST_HOST")
	if host == "" {
		t.Skip("sets a server up as a runhost and runs on it: set AGENTRY_LIVE_RUNHOST_HOST and the rest")
	}
	user, pw := os.Getenv("AGENTRY_LIVE_RUNHOST_USER"), os.Getenv("AGENTRY_LIVE_RUNHOST_PASSWORD")
	reg, ref := os.Getenv("AGENTRY_LIVE_RUNHOST_REGISTRY"), os.Getenv("AGENTRY_LIVE_RUNHOST_REF")
	runner := os.Getenv("AGENTRY_RUNNER_HOME")
	if runner == "" {
		t.Fatal("set AGENTRY_RUNNER_HOME to a scratch runner home whose policy names the model")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()
	var asked []string
	app := &App{ctx: ctx, confirm: func(_ context.Context, title, msg string) (bool, error) {
		asked = append(asked, title)
		t.Logf("dialog %q: %s", title, firstLine(msg))
		return true, nil
	}}
	run := &RunAPI{app: app, runnerHomeDir: runner}
	r := &RunhostsAPI{app: app, run: run}
	st, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	st.DefaultRegistry = reg
	if err := settings.Save(st); err != nil {
		t.Fatal(err)
	}

	admin := RunhostAdmin{User: user, Password: pw}
	v, err := r.Add(RunhostAddRequest{Name: "lab", Host: host, Admin: admin})
	if err != nil {
		t.Fatalf("add: %v (dialogs %v)", err, asked)
	}
	t.Logf("set up %s on %s, CLI %s, owner %s, host key %s", v.Name, v.Host, v.CLIVersion, v.Owner, v.Fingerprint)
	defer func() {
		if err := r.Remove("lab", admin, false, false); err != nil {
			t.Errorf("remove: %v", err)
		}
		if list, _ := r.List(); len(list) != 0 {
			t.Errorf("the record stayed: %+v", list)
		}
	}()
	s, err := r.Status("lab")
	if err != nil || !s.Reachable || !s.SameVersion {
		t.Fatalf("status %+v %v", s, err)
	}
	if err := r.SetDefault("lab"); err != nil {
		t.Fatal(err)
	}

	// The output folder as the native picker would have returned it.
	out := filepath.Join(t.TempDir(), "out")
	app.markPicked(out)
	res, err := run.RunOnRunhost("lab", reg, ref, "Write the report the release asks for.", nil, out, 0, -1, false, nil, false)
	if err != nil {
		t.Fatalf("run on lab: %v", err)
	}
	t.Logf("run %s: %s exit %d, ran on %+v, output %s", res.RunID, res.Status, res.ExitCode, res.RanOn, res.OutDir)
	if res.RanOn == nil || res.RanOn.Name != "lab" || res.RanOn.Fingerprint != v.Fingerprint {
		t.Fatalf("the result does not say it ran on lab: %+v", res.RanOn)
	}
	b, err := os.ReadFile(filepath.Join(res.OutDir, "result.json"))
	if err != nil || !strings.Contains(string(b), `"runhost"`) {
		t.Fatalf("result.json does not name the runhost: %v", err)
	}
	pf, _ := policy.Resolve("")
	t.Logf("policy %s", pf.Path)
}
