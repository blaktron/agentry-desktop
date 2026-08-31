package run

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func withOSRelease(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "os-release")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	old := osReleasePath
	osReleasePath = p
	t.Cleanup(func() { osReleasePath = old })
}

func TestOSReleaseParsing(t *testing.T) {
	withOSRelease(t, "NAME=\"Fedora Linux\"\nID=fedora\nID_LIKE=\"rhel centos\"\n")
	if got := readOSReleaseID(); got != "fedora" {
		t.Fatalf("ID = %q", got)
	}
	if got := readOSReleaseIDLike(); len(got) != 2 || got[0] != "rhel" {
		t.Fatalf("ID_LIKE = %v", got)
	}
}

func TestDetectMapsDistroToCommand(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only mapping")
	}
	withOSRelease(t, "ID=ubuntu\n")
	p := DetectPrereqs(t.Context())
	if p.Command == "" || !strings.Contains(p.Command, "docker.io") {
		t.Fatalf("ubuntu command = %q", p.Command)
	}
	if !strings.Contains(p.Command, "usermod -aG docker") {
		t.Fatalf("command misses the group step: %q", p.Command)
	}
}

func TestDetectUnknownDistroFallsBackToBrowser(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only mapping")
	}
	withOSRelease(t, "ID=beos\n")
	p := DetectPrereqs(t.Context())
	if p.CanInstall || p.Command != "" {
		t.Fatalf("unknown distro should not offer an install: %+v", p)
	}
	if p.DownloadURL == "" {
		t.Fatal("no download URL fallback")
	}
}

func TestDetectViaIDLike(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only mapping")
	}
	withOSRelease(t, "ID=nobara\nID_LIKE=\"fedora\"\n")
	p := DetectPrereqs(t.Context())
	if !strings.Contains(p.Command, "moby-engine") {
		t.Fatalf("ID_LIKE=fedora should map to moby-engine: %q", p.Command)
	}
}
