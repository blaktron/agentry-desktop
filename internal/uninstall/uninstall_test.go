package uninstall

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
)

// fakeMachine gives the test a home of its own with the two configuration
// trees populated, a sibling under each parent that must survive, and a
// mocked keychain holding both clients' entries plus one of another program.
func fakeMachine(t *testing.T) (home, confDir, runnerHome string) {
	t.Helper()
	// Resolved: on macOS the temporary folder is reached through /var, a
	// link, which a share folder's guards refuse.
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	confDir = filepath.Join(home, ".config", "agentry")
	runnerHome = filepath.Join(home, ".agentry", "runner")
	for _, f := range []string{
		filepath.Join(confDir, "desktop.yaml"),
		filepath.Join(confDir, "installed.yaml"),
		filepath.Join(confDir, "keys", "dev.example.net.pub"),
		filepath.Join(runnerHome, "policy.yaml"),
		filepath.Join(runnerHome, "profiles", "default.yaml"),
		filepath.Join(home, ".config", "other-program", "settings.ini"),
		filepath.Join(home, ".agentry", "not-the-runner.txt"),
	} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	keyring.MockInit()
	for _, e := range [][2]string{{"agentry", "dev.example.net"}, {"agentry", "provider:openai"}, {"other-program", "acct"}} {
		if err := keyring.Set(e[0], e[1], "secret"); err != nil {
			t.Fatal(err)
		}
	}
	return home, confDir, runnerHome
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func TestRemoveTakesExactlyTheNamedThings(t *testing.T) {
	home, confDir, runnerHome := fakeMachine(t)
	t.Setenv("AGENTRY_CONFIG_DIR", "")
	t.Setenv("AGENTRY_RUNNER_HOME", "")

	var out bytes.Buffer
	if code := Main(&out); code != 0 {
		t.Fatalf("exit %d, output:\n%s", code, out.String())
	}
	for _, gone := range []string{confDir, runnerHome} {
		if exists(gone) {
			t.Errorf("%s still exists", gone)
		}
	}
	for _, kept := range []string{
		filepath.Join(home, ".config", "other-program", "settings.ini"),
		filepath.Join(home, ".agentry", "not-the-runner.txt"),
	} {
		if !exists(kept) {
			t.Errorf("%s was removed; only the agentry trees may go", kept)
		}
	}
	for _, acct := range []string{"dev.example.net", "provider:openai"} {
		if _, err := keyring.Get("agentry", acct); !errors.Is(err, keyring.ErrNotFound) {
			t.Errorf("agentry keychain entry %q still present (err=%v)", acct, err)
		}
	}
	if v, err := keyring.Get("other-program", "acct"); err != nil || v != "secret" {
		t.Errorf("another program's keychain entry was touched: %q, %v", v, err)
	}
	for _, want := range []string{confDir, runnerHome, "keychain"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Errorf("report does not name %s:\n%s", want, out.String())
		}
	}
}

func TestRemoveFollowsTheRelocationVariables(t *testing.T) {
	home, confDir, runnerHome := fakeMachine(t)
	relConf := filepath.Join(home, "elsewhere", "conf")
	relRunner := filepath.Join(home, "elsewhere", "runner")
	for _, d := range []string{relConf, relRunner} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "f"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("AGENTRY_CONFIG_DIR", relConf)
	t.Setenv("AGENTRY_RUNNER_HOME", relRunner)

	if code := Main(&bytes.Buffer{}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if exists(relConf) || exists(relRunner) {
		t.Error("the relocated trees were not removed")
	}
	if !exists(confDir) || !exists(runnerHome) {
		t.Error("the default trees were removed although the variables point elsewhere")
	}
}

func TestMissingTreesAndAnEmptyKeychainAreNotFailures(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("AGENTRY_CONFIG_DIR", "")
	t.Setenv("AGENTRY_RUNNER_HOME", "")
	keyring.MockInit()
	if code := Main(&bytes.Buffer{}); code != 0 {
		t.Fatalf("exit %d on a machine with nothing to remove", code)
	}
}

func TestRefusesToRemoveTheHomeOrAnAncestorOfIt(t *testing.T) {
	home, confDir, _ := fakeMachine(t)
	for _, bad := range []string{home, filepath.Dir(home), "relative/path", string(filepath.Separator)} {
		t.Run(bad, func(t *testing.T) {
			t.Setenv("AGENTRY_CONFIG_DIR", bad)
			t.Setenv("AGENTRY_RUNNER_HOME", "")
			var out bytes.Buffer
			if code := Main(&out); code == 0 {
				t.Fatalf("exit 0 with AGENTRY_CONFIG_DIR=%s", bad)
			}
			if !exists(confDir) {
				t.Fatal("the home was touched")
			}
			if !bytes.Contains(out.Bytes(), []byte("refus")) {
				t.Errorf("the refusal does not say so:\n%s", out.String())
			}
		})
	}
}

func TestAFailedKeychainIsReportedNotHidden(t *testing.T) {
	_, confDir, runnerHome := fakeMachine(t)
	t.Setenv("AGENTRY_CONFIG_DIR", "")
	t.Setenv("AGENTRY_RUNNER_HOME", "")
	keyring.MockInitWithError(errors.New("no credential store"))

	var out bytes.Buffer
	if code := Main(&out); code != 1 {
		t.Fatalf("exit %d, want 1 when the keychain could not be cleared", code)
	}
	if exists(confDir) || exists(runnerHome) {
		t.Error("the trees were kept because the keychain failed; each removal stands on its own")
	}
	if !bytes.Contains(out.Bytes(), []byte("no credential store")) {
		t.Errorf("the keychain error is not in the report:\n%s", out.String())
	}
}

// The persistent agents' share folders go with the settings, the defaults
// under ~/agentry and one the person chose, and nothing beside them.
func TestRemoveTakesTheAgentsShareFolders(t *testing.T) {
	home, _, runnerHome := fakeMachine(t)
	t.Setenv("AGENTRY_CONFIG_DIR", "")
	t.Setenv("AGENTRY_RUNNER_HOME", "")
	chosen := filepath.Join(home, "Documents", "assistant-files")
	for _, f := range []string{
		filepath.Join(home, "agentry", "mainshare", "report.pdf"),
		filepath.Join(chosen, "notes.md"),
		filepath.Join(home, "agentry", "my-own-folder", "keep.txt"),
		filepath.Join(home, "Documents", "keep.txt"),
	} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := persistent.SaveConfig(runnerHome, "assistant", persistent.Config{Version: persistent.ConfigVersion, Share: chosen}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := Main(&out); code != 0 {
		t.Fatalf("exit %d, output:\n%s", code, out.String())
	}
	for _, gone := range []string{filepath.Join(home, "agentry", "mainshare"), chosen} {
		if exists(gone) {
			t.Errorf("%s still exists", gone)
		}
	}
	for _, kept := range []string{filepath.Join(home, "agentry", "my-own-folder", "keep.txt"), filepath.Join(home, "Documents", "keep.txt")} {
		if !exists(kept) {
			t.Errorf("%s was removed", kept)
		}
	}
}
