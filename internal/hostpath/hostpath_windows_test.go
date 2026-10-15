//go:build windows

package hostpath

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The registry's machine Path always holds System32; reading it is what
// lets a relaunched app see what a new terminal sees.
func TestTheRegistrysPathIsRead(t *testing.T) {
	sys := strings.ToLower(filepath.Join(os.Getenv("SystemRoot"), "System32"))
	for _, d := range registryPath() {
		if windowsKey(d) == sys {
			return
		}
	}
	t.Fatalf("System32 (%s) not in the registry's Path: %q", sys, registryPath())
}

// The whole fault, end to end on Windows: with the first launch's PATH no
// docker is found; after ForGUI the per-user Docker Desktop's is, and so
// is the credential helper beside it that every pull execs.
func TestForGUIFindsAPerUserDockerDesktopOnWindows(t *testing.T) {
	local := t.TempDir()
	bin := filepath.Join(local, "Programs", "DockerDesktop", "resources", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"docker.exe", "docker-credential-desktop.exe"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("MZ"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("ProgramFiles", t.TempDir())
	t.Setenv("PATH", filepath.Join(os.Getenv("SystemRoot"), "System32"))
	if p, err := exec.LookPath("docker-credential-desktop"); err == nil && !strings.HasPrefix(p, bin) {
		t.Logf("note: another docker-credential-desktop is on the registry's PATH: %s", p)
	}
	ForGUI()
	for _, name := range []string{"docker", "docker-credential-desktop"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Fatalf("%s not found after ForGUI: %v (PATH %s)", name, err, os.Getenv("PATH"))
		}
	}
	if !strings.Contains(strings.ToLower(os.Getenv("PATH")), strings.ToLower(bin)) {
		t.Fatalf("PATH lacks %s: %s", bin, os.Getenv("PATH"))
	}
}
