package clipath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// cliFiles writes a carried CLI and a script copy with different bytes, and
// a version func that answers for each by path.
func cliFiles(t *testing.T, bundledVer, targetVer string) (bundled, target string, version VersionFunc) {
	t.Helper()
	dir := t.TempDir()
	bundled = filepath.Join(dir, "carried", cliName)
	target = filepath.Join(dir, "script", cliName)
	for p, body := range map[string]string{bundled: "new cli " + bundledVer, target: "old cli " + targetVer} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	versions := map[string]string{bundled: bundledVer, target: targetVer}
	version = func(p string) ([]byte, error) {
		v, ok := versions[p]
		if !ok {
			return nil, fmt.Errorf("asked for %s", p)
		}
		if v == "" {
			return nil, errors.New("exit status 1")
		}
		return []byte("agentry version " + v + " default registry: https://secureagentry.net\n"), nil
	}
	return bundled, target, version
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRefreshReplacesAnOlderScriptCopy(t *testing.T) {
	for _, tc := range []struct{ bundled, target string }{
		{"0.6.87", "0.6.52-dev.2"}, // the operator's laptop, 2026-10-09
		{"0.6.87", "0.6.52"},
		{"0.6.87", "0.6.87-dev.1"},        // a dev prerelease sorts below its release
		{"0.6.87-dev.10", "0.6.87-dev.2"}, // dev.N by number
	} {
		t.Run(tc.target+"→"+tc.bundled, func(t *testing.T) {
			bundled, target, version := cliFiles(t, tc.bundled, tc.target)
			r, err := Refresh(bundled, target, version)
			if err != nil {
				t.Fatal(err)
			}
			if r.Outcome != Replaced || r.TargetVer != tc.target || r.BundledVer != tc.bundled {
				t.Fatalf("got %+v", r)
			}
			if got, want := mustRead(t, target), mustRead(t, bundled); got != want {
				t.Fatalf("target holds %q, want the carried CLI's %q", got, want)
			}
			if runtime.GOOS != "windows" {
				if fi, _ := os.Stat(target); fi.Mode().Perm()&0o111 == 0 {
					t.Fatalf("target is not executable: %v", fi.Mode())
				}
			}
			left, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".*"))
			old, _ := filepath.Glob(target + ".old")
			if len(left)+len(old) != 0 {
				t.Fatalf("left behind: %v %v", left, old)
			}
		})
	}
}

func TestRefreshNeverDowngradesOrGuesses(t *testing.T) {
	for _, tc := range []struct{ name, bundled, target string }{
		{"equal", "0.6.87", "0.6.87"},
		{"newer", "0.6.87", "0.6.90"},
		{"a release over its own dev build", "0.6.87-dev.1", "0.6.87"},
		{"unreadable", "0.6.87", ""},
		{"not semver", "0.6.87", "devel"},
		{"carried unreadable", "", "0.6.52"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bundled, target, version := cliFiles(t, tc.bundled, tc.target)
			before := mustRead(t, target)
			r, err := Refresh(bundled, target, version)
			if err != nil {
				t.Fatal(err)
			}
			if r.Outcome != Kept || r.KeptBecause == "" {
				t.Fatalf("got %+v, want Kept with a reason", r)
			}
			if mustRead(t, target) != before {
				t.Fatal("target changed")
			}
		})
	}
}

func TestRefreshNothingToDo(t *testing.T) {
	t.Run("no script copy", func(t *testing.T) {
		bundled, target, version := cliFiles(t, "0.6.87", "0.6.52")
		if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
		r, err := Refresh(bundled, target, version)
		if err != nil || r.Outcome != Nothing {
			t.Fatalf("got %+v, %v", r, err)
		}
		if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("a CLI was put where none was")
		}
	})
	t.Run("a build that carries none", func(t *testing.T) {
		bundled, target, version := cliFiles(t, "0.6.87", "0.6.52")
		if err := os.Remove(bundled); err != nil {
			t.Fatal(err)
		}
		if r, err := Refresh(bundled, target, version); err != nil || r.Outcome != Nothing {
			t.Fatalf("got %+v, %v", r, err)
		}
	})
	t.Run("no place known", func(t *testing.T) {
		if r, err := Refresh("", "", nil); err != nil || r.Outcome != Nothing {
			t.Fatalf("got %+v, %v", r, err)
		}
	})
	t.Run("already the carried bytes", func(t *testing.T) {
		bundled, target, _ := cliFiles(t, "0.6.87", "0.6.52")
		if err := os.WriteFile(target, []byte(mustRead(t, bundled)), 0o755); err != nil {
			t.Fatal(err)
		}
		never := func(string) ([]byte, error) { t.Fatal("ran --version for identical bytes"); return nil, nil }
		if r, err := Refresh(bundled, target, never); err != nil || r.Outcome != Current {
			t.Fatalf("got %+v, %v", r, err)
		}
	})
	t.Run("a symlink", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need a privilege on Windows")
		}
		bundled, target, version := cliFiles(t, "0.6.87", "0.6.52")
		other := target + "-real"
		if err := os.Rename(target, other); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(other, target); err != nil {
			t.Fatal(err)
		}
		if r, err := Refresh(bundled, target, version); err != nil || r.Outcome != Nothing {
			t.Fatalf("got %+v, %v", r, err)
		}
		if mustRead(t, other) != "old cli 0.6.52" {
			t.Fatal("the link's target changed")
		}
	})
}

func TestParseVersionReadsTheCLIsLine(t *testing.T) {
	v, err := parseVersion([]byte("agentry version 0.6.52-dev.2 default registry: https://dev.secureagentry.net\r\n"))
	if err != nil || v.Original() != "0.6.52-dev.2" {
		t.Fatalf("got %v, %v", v, err)
	}
	// The Linux and macOS builds print the registry on a line of its own.
	v, err = parseVersion([]byte("agentry version 0.6.87\ndefault registry: https://secureagentry.net\n"))
	if err != nil || v.Original() != "0.6.87" {
		t.Fatalf("two-line form: got %v, %v", v, err)
	}
	if _, err := parseVersion([]byte("some other program 1.2.3\n")); err == nil {
		t.Fatal("read a version from another program's output")
	}
}
