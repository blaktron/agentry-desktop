//go:build !windows

package clipath

import (
	"runtime"
	"testing"
)

func TestScriptCLIIsInstallShsDefault(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	want := func(got, w string) {
		t.Helper()
		if got != w {
			t.Fatalf("got %q, want %q", got, w)
		}
	}
	want(scriptCLI(env(map[string]string{"HOME": "/home/p"})), "/home/p/.local/bin/agentry")
	want(scriptCLI(env(map[string]string{"HOME": "/home/p", "XDG_BIN_HOME": "/home/p/bin"})), "/home/p/bin/agentry")
	want(scriptCLI(env(map[string]string{"HOME": "/home/p", "XDG_BIN_HOME": "rel"})), "/home/p/.local/bin/agentry")
	want(scriptCLI(env(map[string]string{})), "")
	if runtime.GOOS == "darwin" {
		// The bundle's own CLI; the packages' place means nothing here.
		want(bundledBeside("/Applications/Agentry Desktop.app/Contents/MacOS"), "/Applications/Agentry Desktop.app/Contents/Resources/bin/agentry")
		want(bundledBeside("/usr/bin"), "")
		return
	}
	want(bundledBeside("/usr/bin"), "/usr/bin/agentry")
	want(bundledBeside("/usr/bin/"), "/usr/bin/agentry")
	// A tarball or a build run from a home carries no pipeline CLI.
	if got := bundledBeside("/home/p/Downloads"); got != "" {
		t.Fatalf("a file beside an unpackaged app counts as carried: %q", got)
	}
}
