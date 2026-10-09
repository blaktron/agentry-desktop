//go:build windows

package clipath

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestScriptCLIIsInstallPs1sDefault(t *testing.T) {
	got := scriptCLI(func(k string) string {
		if k == "LOCALAPPDATA" {
			return `C:\Users\p\AppData\Local`
		}
		return ""
	})
	if want := `C:\Users\p\AppData\Local\Programs\agentry\bin\agentry.exe`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if scriptCLI(func(string) string { return "" }) != "" {
		t.Fatal("a place without LOCALAPPDATA")
	}
	if got, want := bundledBeside(`C:\Program Files\Agentry Desktop`), `C:\Program Files\Agentry Desktop\bin\agentry.exe`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// A script copy that is running (an MCP session) is renamed aside and
// replaced, as the installer does; the running process keeps going.
func TestSwapInReplacesARunningExe(t *testing.T) {
	if os.Getenv("CLIPATH_SLEEP") == "1" {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	target := filepath.Join(dir, cliName)
	body, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, body, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(target, "-test.run=^TestSwapInReplacesARunningExe$")
	cmd.Env = append(os.Environ(), "CLIPATH_SLEEP=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	time.Sleep(500 * time.Millisecond)
	tmp := filepath.Join(dir, ".new")
	if err := os.WriteFile(tmp, []byte("the carried cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	aside, err := swapIn(tmp, target)
	if err != nil {
		t.Fatalf("swapIn over a running exe: %v", err)
	}
	if !aside {
		t.Fatal("the running copy was removed, or not reported left aside")
	}
	if got, _ := os.ReadFile(target); string(got) != "the carried cli" {
		t.Fatalf("target holds %d bytes of something else", len(got))
	}
	// The next refresh clears the .old once nothing holds it.
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	tmp2 := filepath.Join(dir, ".new2")
	if err := os.WriteFile(tmp2, []byte("again"), 0o755); err != nil {
		t.Fatal(err)
	}
	if aside, err := swapIn(tmp2, target); err != nil || aside {
		t.Fatalf("second swap: aside=%v err=%v", aside, err)
	}
	if _, err := os.Stat(target + ".old"); !os.IsNotExist(err) {
		t.Fatal(".old left behind")
	}
}
