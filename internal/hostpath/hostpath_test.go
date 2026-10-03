package hostpath

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// launchdPath is what an app opened from Finder inherits; it is what the
// signed v0.6.43 ran with on the Mac build host (agentry-desktop#573).
const launchdPath = "/usr/bin:/bin:/usr/sbin:/sbin"

func exists(set ...string) func(string) bool {
	m := map[string]bool{}
	for _, s := range set {
		m[s] = true
	}
	return func(p string) bool { return m[p] }
}

func TestAMacLaunchedFromFinderGetsDockerDesktopsUserBin(t *testing.T) {
	got, changed := extendFor("darwin", "/Users/p", launchdPath,
		exists("/Users/p/.docker/bin", "/Applications/Docker.app/Contents/Resources/bin"))
	want := launchdPath + ":/Users/p/.docker/bin:/Applications/Docker.app/Contents/Resources/bin"
	if !changed || got != want {
		t.Fatalf("got %q (changed %v), want %q", got, changed, want)
	}
}

func TestOnlyDirectoriesThatExistAreAdded(t *testing.T) {
	got, changed := extendFor("darwin", "/Users/p", launchdPath, exists())
	if changed || got != launchdPath {
		t.Fatalf("got %q (changed %v), want the PATH untouched", got, changed)
	}
}

func TestAPathFromATerminalKeepsItsOrderAndGainsNoDuplicate(t *testing.T) {
	// Started from a shell: /usr/local/bin is already there, ahead of
	// /usr/bin, and must stay where it is, once.
	path := "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
	got, _ := extendFor("darwin", "/Users/p", path,
		exists("/usr/local/bin", "/opt/homebrew/bin", "/Users/p/.rd/bin"))
	want := path + ":/Users/p/.rd/bin"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestEveryKnownLocationInItsOrder(t *testing.T) {
	all := exists("/Users/p/.docker/bin", "/usr/local/bin", "/opt/homebrew/bin",
		"/Applications/Docker.app/Contents/Resources/bin", "/Users/p/.orbstack/bin", "/Users/p/.rd/bin")
	got, _ := extendFor("darwin", "/Users/p", "/usr/bin", all)
	want := "/usr/bin:/Users/p/.docker/bin:/usr/local/bin:/opt/homebrew/bin:" +
		"/Applications/Docker.app/Contents/Resources/bin:/Users/p/.orbstack/bin:/Users/p/.rd/bin"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNoHomeSkipsTheHomeRelativeLocations(t *testing.T) {
	got, _ := extendFor("darwin", "", launchdPath, func(string) bool { return true })
	want := launchdPath + ":/usr/local/bin:/opt/homebrew/bin:/Applications/Docker.app/Contents/Resources/bin"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAnEmptyPathGetsTheLocationsAlone(t *testing.T) {
	got, _ := extendFor("darwin", "/Users/p", "", exists("/Users/p/.docker/bin"))
	if got != "/Users/p/.docker/bin" {
		t.Fatalf("got %q", got)
	}
}

func TestLinuxAndWindowsAreLeftAlone(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		got, changed := extendFor(goos, "/home/p", launchdPath, func(string) bool { return true })
		if changed || got != launchdPath {
			t.Errorf("%s: got %q (changed %v), want the PATH untouched", goos, got, changed)
		}
	}
}

// The whole fault, end to end on a Mac: with launchd's PATH no docker is
// found; after ForGUI the one in ~/.docker/bin is, and so is the
// credential helper beside it that every pull execs.
func TestForGUIFindsDockerAndItsCredentialHelperOnAMac(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("ForGUI changes nothing off macOS (TestLinuxAndWindowsAreLeftAlone)")
	}
	home := t.TempDir()
	bin := filepath.Join(home, ".docker", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"docker", "docker-credential-desktop"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", launchdPath)
	if p, err := exec.LookPath("docker-credential-desktop"); err == nil {
		t.Fatalf("precondition: found %s on launchd's PATH", p)
	}
	ForGUI()
	for _, name := range []string{"docker", "docker-credential-desktop"} {
		p, err := exec.LookPath(name)
		if err != nil {
			t.Fatalf("%s not found after ForGUI: %v (PATH %s)", name, err, os.Getenv("PATH"))
		}
		if name == "docker-credential-desktop" && filepath.Dir(p) != bin {
			t.Errorf("%s resolved to %s, want the one in %s", name, p, bin)
		}
	}
}
