package app

// The Scheduled page's Install the agentry command, live: the bound methods
// driven as the page drives them, against a real site's /install.sh and
// downloads manifest, with the native dialog answered Yes by the test.
// Skipped unless AGENTRY_LIVE_INSTALL_CLI=1, because it downloads the CLI
// and writes a home's ~/.local/bin and shell startup file:
//
//	AGENTRY_LIVE_INSTALL_CLI=1 \
//	AGENTRY_LIVE_INSTALL_CLI_SITE=https://dev.secureagentry.net \
//	HOME=<a scratch home> PATH=/usr/bin:/bin SHELL=/bin/zsh \
//	go test -run TestLiveInstallCLI -v -timeout 15m ./internal/app
//
// It refuses to run in the account's own home. On macOS, run it in the GUI
// session (a one-shot agent in gui/<uid>, the wiki's "Checks in the Mac's
// GUI session"), where the environment is launchd's, as the app's is.
//
// What it holds: with no command, Availability offers Install and the
// scheduling notice stands; the install puts the site's CLI version at
// ~/.local/bin/agentry with one marked PATH line in the shell's startup
// file; the page then finds it, is available and offers Reinstall; and a
// Reinstall of the same version changes nothing and adds no second line.

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blaktron/agentry-desktop/internal/schedules"
	"github.com/blaktron/agentry-desktop/internal/selfupdate"
)

func TestLiveInstallCLI(t *testing.T) {
	if os.Getenv("AGENTRY_LIVE_INSTALL_CLI") != "1" {
		t.Skip("AGENTRY_LIVE_INSTALL_CLI=1 runs the site's install script into a scratch HOME")
	}
	home := os.Getenv("HOME")
	if u, err := user.Current(); err != nil || home == "" || filepath.Clean(home) == filepath.Clean(u.HomeDir) {
		t.Fatalf("HOME must name a scratch home, not the account's own (%q)", home)
	}
	site := strings.TrimRight(cmp.Or(os.Getenv("AGENTRY_LIVE_INSTALL_CLI_SITE"), "https://dev.secureagentry.net"), "/")
	saved := selfupdate.ManifestURL
	selfupdate.ManifestURL = site + "/downloads/manifest.json"
	t.Cleanup(func() { selfupdate.ManifestURL = saved })

	want := servedCLIVersion(t, selfupdate.ManifestURL)
	t.Logf("%s serves agentry %s; SHELL=%q PATH=%q", site, want, os.Getenv("SHELL"), os.Getenv("PATH"))

	f := &scheduleFake{answer: true}
	s := newScheduleAPI(t, f, false)
	s.findCLI = nil // the real command, found and asked its version

	before, err := s.Availability()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("before: available=%v installCLI=%q reason=%q", before.Available, before.InstallCLI, before.Reason)
	if _, ferr := schedules.FindCLI(); ferr != nil && (before.InstallCLI != installCLILabel || before.Available) {
		t.Fatalf("no command, yet %+v", before)
	}

	got, err := s.InstallCLI()
	if err != nil {
		t.Fatalf("InstallCLI: %v", err)
	}
	t.Logf("installed %s at %s\nnotes: %q\n--- the script printed ---\n%s", got.Version, got.Path, got.Notes, got.Log)
	path := filepath.Join(home, ".local", "bin", "agentry")
	if got.Path != path || got.Version != want {
		t.Fatalf("installed %q at %q, want %q at %q", got.Version, got.Path, want, path)
	}
	if len(f.asked) != 1 || !strings.Contains(f.asked[0], site+"/install.sh") {
		t.Errorf("the dialog asked %q", f.asked)
	}
	rc, marked := startupFileMarked(t, home)
	if marked != 1 {
		t.Fatalf("%s carries the installer's marker %d times", rc, marked)
	}

	after, err := s.Availability()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("after: available=%v installCLI=%q reason=%q", after.Available, after.InstallCLI, after.Reason)
	if !after.Available || after.InstallCLI != reinstallCLILabel {
		t.Fatalf("after the install: %+v", after)
	}
	if p, err := schedules.FindCLI(); err != nil || p != path {
		t.Fatalf("FindCLI = %q, %v", p, err)
	}

	again, err := s.InstallCLI()
	if err != nil {
		t.Fatalf("Reinstall: %v", err)
	}
	if !strings.Contains(again.Log, "is already installed") {
		t.Errorf("a Reinstall of the same version said:\n%s", again.Log)
	}
	if _, n := startupFileMarked(t, home); n != 1 {
		t.Errorf("after a Reinstall the marker is there %d times", n)
	}
}

// servedCLIVersion is the CLI version the site's manifest lists.
func servedCLIVersion(t *testing.T, manifest string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, manifest, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var m struct {
		CLI struct {
			Version string `json:"version"`
		} `json:"cli"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil || m.CLI.Version == "" {
		t.Fatalf("%s: no cli.version (%v)", manifest, err)
	}
	return m.CLI.Version
}

// startupFileMarked finds the startup file that carries the installer's
// marker in home and counts the marker there.
func startupFileMarked(t *testing.T, home string) (string, int) {
	t.Helper()
	const marker = "# Added by the agentry CLI installer: "
	best, most := "", 0
	for _, rc := range []string{".zshrc", ".bashrc", ".bash_profile", ".profile", ".config/fish/conf.d/agentry.fish"} {
		b, err := os.ReadFile(filepath.Join(home, rc))
		if err != nil {
			continue
		}
		if n := strings.Count(string(b), marker); n > most {
			best, most = rc, n
		}
	}
	return best, most
}
