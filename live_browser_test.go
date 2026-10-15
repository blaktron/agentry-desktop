package main

// A browser live: the bound methods driven as the run setup drives them,
// against this machine's Docker, with a QuickAgent installed from a folder
// whose url question is its browser's site. Skipped unless
// AGENTRY_LIVE_BROWSER=1:
//
//	AGENTRY_LIVE_BROWSER=1 \
//	AGENTRY_CONFIG_DIR=<a scratch config dir> \
//	AGENTRY_RUNNER_HOME=<a scratch runner home> \
//	AGENTRY_LIVE_BROWSER_SITE=<the site, default https://example.com> \
//	AGENTRY_LIVE_BROWSER_MODEL_URL=<an OpenAI-compatible base URL, optional> \
//	AGENTRY_LIVE_BROWSER_OUT=<a folder to keep the run's output in, optional> \
//	go test -run TestLiveBrowser -v -timeout 30m .
//
// With AGENTRY_LIVE_BROWSER_MODEL_URL the test writes the runner home's
// policy from the CLI's defaults with that endpoint as a custom provider
// (model "stub", key variable AGENTRY_LIVE_BROWSER_KEY), so a scripted model
// can drive the browser where no real model key answers; without it the
// runner home's own policy is used.
//
// What it holds: the site question is the browser's, so the answer is in
// intake.json and nothing under inputs/ was fetched for it; the run's
// browser reached the site and its screenshot is in reports/browser/; and
// the envelope's enforced capabilities (result.json) name the browser's
// scope and its proxy's refusals, which receipts/browser-proxy.log lists.

import (
	"cmp"
	"context"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

const liveBrowserManifest = `schemaVersion: "2"
name: browser-probe
namespace: local
modes: [autonomous]
version: 0.1.0
summary: "Opens the site it is given and takes a screenshot (live check)"
entrypoints:
  instruction: SKILL.md
intake:
  - id: site
    prompt: "Which site should I test?"
    type: url
    required: true
capabilities:
  browser:
    sites: [site]
publish:
  include: [agent.yaml, SKILL.md]
`

func TestLiveBrowser(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_BROWSER") != "1" {
		t.Skip("drives a browser to a site: set AGENTRY_LIVE_BROWSER=1 (see the file's comment)")
	}
	home := os.Getenv(policy.HomeEnv)
	if os.Getenv(configdir.EnvVar) == "" || home == "" {
		t.Fatalf("set %s and %s to scratch directories", configdir.EnvVar, policy.HomeEnv)
	}
	site := cmp.Or(os.Getenv("AGENTRY_LIVE_BROWSER_SITE"), "https://example.com")
	outRoot := cmp.Or(os.Getenv("AGENTRY_LIVE_BROWSER_OUT"), t.TempDir())

	if base := os.Getenv("AGENTRY_LIVE_BROWSER_MODEL_URL"); base != "" {
		u, err := url.Parse(base)
		if err != nil || u.Host == "" {
			t.Fatalf("AGENTRY_LIVE_BROWSER_MODEL_URL %q", base)
		}
		port := u.Port()
		if port == "" {
			port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
		}
		p := policy.Default()
		p.Model = policy.Model{Provider: "custom", Name: "stub", BaseURL: base, APIKeyEnv: "AGENTRY_LIVE_BROWSER_KEY", ProxyAllow: []string{u.Hostname() + ":" + port}}
		p.Budgets.MaxTurns = 8
		if err := policy.Save(filepath.Join(home, "policy.yaml"), p); err != nil {
			t.Fatalf("write the scratch policy: %v", err)
		}
		t.Setenv("AGENTRY_LIVE_BROWSER_KEY", "not-a-key")
	}

	app := &App{ctx: context.Background(), confirm: func(_ context.Context, title, message string) (bool, error) {
		t.Logf("asked: %s\n%s", title, message)
		return true, nil
	}}
	r := &RunAPI{app: app}
	b := &BuildAPI{app: app, run: r}

	dir := filepath.Join(outRoot, "probe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	app.markPicked(dir)
	for name, body := range map[string]string{
		"agent.yaml": liveBrowserManifest,
		"SKILL.md": "# Browser probe\n\nOpen the site from /inputs/intake.json with browser_navigate, take a screenshot named home.png " +
			"with browser_take_screenshot, and report the page's title in one line. Finish in at most 3 tool calls.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := b.Install(dir); err != nil {
		t.Fatalf("install the probe: %v", err)
	}
	const ref = "local/browser-probe@0.1.0"

	// The run setup's view of the question: the browser's site.
	prof, err := r.ProfileStatus("https://secureagentry.net", ref)
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	if len(prof.Intake) != 1 || !prof.Intake[0].Site {
		t.Fatalf("the site question is not marked the browser's: %+v", prof.Intake)
	}
	if lines := strings.Join(prof.CapabilityLines, "\n"); !strings.Contains(lines, "browser: a web browser that can open the site you name for site and its subdomains") {
		t.Fatalf("capability lines:\n%s", lines)
	}

	out := filepath.Join(outRoot, "run")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	app.markPicked(out)
	res, err := r.RunAgent("https://secureagentry.net", ref, "", nil, out, nil, 0, -1, true, map[string]string{"site": site}, false, false, nil, false, nil, nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Logf("run: status %s, exit %d, %s; result %s", res.Status, res.ExitCode, res.Error, res.ResultJSON)

	var shots, intakeJSON, receipts []string
	_ = filepath.WalkDir(res.OutDir, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(res.OutDir, p)
		switch {
		case strings.Contains(rel, filepath.Join("reports", "browser")+string(filepath.Separator)) && strings.HasSuffix(p, ".png"):
			shots = append(shots, rel)
		case filepath.Base(p) == "intake.json":
			raw, _ := os.ReadFile(p)
			intakeJSON = append(intakeJSON, string(raw))
		case strings.Contains(rel, "receipts"+string(filepath.Separator)) || rel == "result.json":
			raw, _ := os.ReadFile(p)
			receipts = append(receipts, string(raw))
		}
		if strings.Contains(rel, filepath.Join("inputs", "site")) {
			t.Errorf("the site was fetched as an input source: %s", rel)
		}
		return nil
	})
	if len(shots) == 0 {
		t.Errorf("no screenshot in reports/browser/ under %s", res.OutDir)
	} else {
		t.Logf("screenshots: %v", shots)
	}
	all := strings.Join(receipts, "\n")
	for _, want := range []string{"browser.scope", "browser.proxy"} {
		if !strings.Contains(all, want) {
			t.Errorf("the receipt does not name %s", want)
		}
	}
	if len(intakeJSON) > 0 && !strings.Contains(strings.Join(intakeJSON, "\n"), site) {
		t.Errorf("intake.json does not hold the site: %s", intakeJSON)
	}
}
