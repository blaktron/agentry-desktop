//go:build !windows

package clipath

import (
	"runtime"
	"testing"
)

func TestScriptCLIIsInstallShsDefault(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	want := func(got, linux string) {
		t.Helper()
		if runtime.GOOS == "darwin" {
			linux = "" // nothing to refresh it with
		}
		if got != linux {
			t.Fatalf("got %q, want %q", got, linux)
		}
	}
	want(scriptCLI(env(map[string]string{"HOME": "/home/p"})), "/home/p/.local/bin/agentry")
	want(scriptCLI(env(map[string]string{"HOME": "/home/p", "XDG_BIN_HOME": "/home/p/bin"})), "/home/p/bin/agentry")
	want(scriptCLI(env(map[string]string{"HOME": "/home/p", "XDG_BIN_HOME": "rel"})), "/home/p/.local/bin/agentry")
	want(scriptCLI(env(map[string]string{})), "")
	want(bundledBeside("/usr/bin"), "/usr/bin/agentry")
	want(bundledBeside("/usr/bin/"), "/usr/bin/agentry")
	// A tarball or a build run from a home carries no pipeline CLI.
	if got := bundledBeside("/home/p/Downloads"); got != "" {
		t.Fatalf("a file beside an unpackaged app counts as carried: %q", got)
	}
}
