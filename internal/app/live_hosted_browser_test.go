package app

// A run In Agentry of a release that declares a browser, live: the bound
// methods driven as the run panel drives them, against a real registry whose
// sandbox host carries a browser. Skipped unless the variables are set,
// because the run spends the account's credits:
//
//	AGENTRY_LIVE_HOSTED_BROWSER_REGISTRY=https://dev.secureagentry.net \
//	AGENTRY_LIVE_HOSTED_BROWSER_TOKEN=<a PAT: run:read, run:write, secret:write, agent:read> \
//	AGENTRY_LIVE_HOSTED_BROWSER_REF=ses-t748-pro-727604/browser-565@0.1.0 \
//	AGENTRY_LIVE_HOSTED_BROWSER_SITE=https://the-internet.herokuapp.com/login \
//	AGENTRY_LIVE_HOSTED_BROWSER_USER=<the site's user name> \
//	AGENTRY_LIVE_HOSTED_BROWSER_PASSWORD=<its password> \
//	go test -run TestLiveHostedRunWithABrowser -v -timeout 30m ./internal/app
//
// The release asks for the site as its `site` question and declares
// SITE_USER and SITE_PASSWORD for its browser to type. The test checks that
// the mode is offered, that a private site is refused by the registry
// (browser-site) before anything is held, and that a run with the two
// secrets held for it alone finishes, its output in the folder with the
// browser's screenshots and neither value in any file or event.

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

func TestLiveHostedRunWithABrowser(t *testing.T) {
	reg := strings.TrimRight(os.Getenv("AGENTRY_LIVE_HOSTED_BROWSER_REGISTRY"), "/")
	token := os.Getenv("AGENTRY_LIVE_HOSTED_BROWSER_TOKEN")
	ref := os.Getenv("AGENTRY_LIVE_HOSTED_BROWSER_REF")
	site := os.Getenv("AGENTRY_LIVE_HOSTED_BROWSER_SITE")
	user := os.Getenv("AGENTRY_LIVE_HOSTED_BROWSER_USER")
	password := os.Getenv("AGENTRY_LIVE_HOSTED_BROWSER_PASSWORD")
	if reg == "" || token == "" || ref == "" || site == "" || user == "" || password == "" {
		t.Skip("set AGENTRY_LIVE_HOSTED_BROWSER_REGISTRY, _TOKEN, _REF, _SITE, _USER and _PASSWORD to run one hosted run with a browser")
	}
	keyring.MockInit()
	setUserHome(t, t.TempDir())
	t.Setenv("AGENTRY_RUNNER_HOME", t.TempDir())
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	s, err := settings.Load()
	if err != nil {
		t.Fatal(err)
	}
	s.Upsert(settings.Registry{URL: settings.Normalize(reg), Token: token})
	if err := settings.Save(s); err != nil {
		t.Fatal(err)
	}
	r, events := newRunAPI(t)

	avail, err := r.HostedAvailability(reg, ref)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("availability: offered %v, available %v (%s)", avail.Offered, avail.Available, avail.Reason)
	if !avail.Offered || !avail.Available {
		t.Fatalf("a run In Agentry of %s is not offered: %+v", ref, avail)
	}

	const task = "Check that the account can sign in."
	private := t.TempDir()
	r.app.markPicked(private)
	start, err := r.StartHostedRun(reg, ref, task, nil, private, map[string]string{"site": "http://192.168.1.20/"}, nil, false, false, nil)
	t.Logf("a private site: %+v, %v", start, err)
	if err == nil && (start == nil || start.Run != nil) {
		t.Fatalf("a private site was not refused: %+v", start)
	}
	if text := refusalText(start, err); !strings.Contains(text, "not a public address") {
		t.Fatalf("the private site's refusal does not say why: %q", text)
	}

	for name, value := range map[string]string{"SITE_USER": user, "SITE_PASSWORD": password} {
		if err := r.SetHostedSecret(reg, name, value, false); err != nil {
			t.Fatal(err)
		}
	}
	out := t.TempDir()
	r.app.markPicked(out)
	start, err = r.StartHostedRun(reg, ref, task, nil, out, map[string]string{"site": site}, nil, false, false, nil)
	if err != nil || start.Run == nil {
		t.Fatalf("the start: %+v, %v", start, err)
	}
	t.Logf("run %s: %s", start.Run.ID, start.Run.Status)

	deadline := time.Now().Add(20 * time.Minute)
	var final *HostedRunView
	for final == nil && time.Now().Before(deadline) {
		for _, v := range events.states() {
			if v.Final {
				final = v
			}
		}
		time.Sleep(2 * time.Second)
	}
	if final == nil {
		t.Fatal("the run never reported a final state")
	}
	t.Logf("run %s: %s, outcome %s, exit %s, error %q", final.ID, final.State, final.Outcome, final.ExitClass, final.Error)
	if final.Outcome != "succeeded" {
		t.Fatalf("the run did not succeed: %+v", final)
	}
	for _, l := range events.lines() {
		if strings.Contains(l, user) || strings.Contains(l, password) {
			t.Fatal("an event carried a secret's value")
		}
	}
	// The output lands in the folder (unpacked by the watch): the browser's
	// screenshots under reports/browser/, and no file holds either value.
	waitFor := time.Now().Add(2 * time.Minute)
	var pngs []string
	for len(pngs) == 0 && time.Now().Before(waitFor) {
		pngs = nil
		_ = filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && strings.HasSuffix(path, ".png") && strings.Contains(path, filepath.Join("reports", "browser")) {
				pngs = append(pngs, path)
			}
			return nil
		})
		time.Sleep(2 * time.Second)
	}
	t.Logf("screenshots: %v", pngs)
	if len(pngs) == 0 {
		t.Fatal("no screenshot under reports/browser/ in the run folder")
	}
	_ = filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path) //nolint:gosec // the test's own run folder
		if strings.Contains(string(b), password) || strings.Contains(string(b), user) {
			t.Errorf("%s holds a secret's value", path)
		}
		return nil
	})
}

// refusalText is what a start that was refused says, from its view or its
// error.
func refusalText(start *HostedStartView, err error) string {
	if err != nil {
		return err.Error()
	}
	if start == nil {
		return ""
	}
	return start.Refusal + " " + start.Detail
}
