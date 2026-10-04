package hostpath

import (
	"strings"
	"testing"
)

// staleAppPath is the shape of the PATH the operator's 0.6.53 carried on
// 2026-10-04 (agentry-desktop#624), read out of the running process: the
// first launch's, with the machine-wide Docker directory that no longer
// existed and without the per-user one Docker Desktop had moved to.
const staleAppPath = `C:\WINDOWS\system32;C:\WINDOWS;C:\Program Files\Docker\Docker\resources\bin;` +
	`C:\Users\p\AppData\Local\Microsoft\WindowsApps`

const perUserDocker = `C:\Users\p\AppData\Local\Programs\DockerDesktop\resources\bin`

// registryNow is that machine's registry Path afterwards: machine, then user.
var registryNow = []string{
	`C:\WINDOWS\system32`, `C:\WINDOWS`, `C:\Program Files\Git\cmd`, "",
	`C:\Users\p\AppData\Local\Microsoft\WindowsApps`, perUserDocker,
	`C:\Users\p\AppData\Local\Programs\Agentry Desktop\bin`,
}

func winEnv(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

var noVars = winEnv(nil)

func TestAWindowsAppWithTheFirstLaunchsPathGainsTheMovedDockerDesktop(t *testing.T) {
	got, changed := extendFor("windows", staleAppPath, windowsCandidates(registryNow, noVars),
		exists(`C:\WINDOWS\system32`, `C:\WINDOWS`, `C:\Program Files\Git\cmd`,
			`C:\Users\p\AppData\Local\Microsoft\WindowsApps`, perUserDocker,
			`C:\Users\p\AppData\Local\Programs\Agentry Desktop\bin`))
	want := staleAppPath + `;C:\Program Files\Git\cmd;` + perUserDocker +
		`;C:\Users\p\AppData\Local\Programs\Agentry Desktop\bin`
	if !changed || got != want {
		t.Fatalf("got  %q (changed %v)\nwant %q", got, changed, want)
	}
}

func TestWindowsEntriesMatchWithoutCaseOrATrailingSeparator(t *testing.T) {
	path := `C:\Windows\System32\;c:\program files\git\CMD`
	got, changed := extendFor("windows", path,
		[]string{`C:\WINDOWS\system32`, `C:/Program Files/Git/cmd/`}, func(string) bool { return true })
	if changed || got != path {
		t.Fatalf("got %q (changed %v), want the PATH untouched", got, changed)
	}
}

func TestDockerDesktopsKnownDirectoriesFollowTheRegistry(t *testing.T) {
	dirs := windowsCandidates([]string{`C:\x`}, winEnv(map[string]string{
		"ProgramFiles": `C:\Program Files`, "LOCALAPPDATA": `C:\Users\p\AppData\Local\`,
	}))
	want := []string{
		`C:\x`,
		`C:\Program Files\Docker\Docker\resources\bin`,
		`C:\Users\p\AppData\Local\Programs\DockerDesktop\resources\bin`,
		`C:\Program Files\Rancher Desktop\resources\resources\win32\bin`,
		`C:\Users\p\AppData\Local\Programs\Rancher Desktop\resources\resources\win32\bin`,
	}
	if strings.Join(dirs, "|") != strings.Join(want, "|") {
		t.Fatalf("got  %q\nwant %q", dirs, want)
	}
}

func TestAKnownDirectoryWhoseVariableIsUnsetIsSkipped(t *testing.T) {
	dirs := windowsCandidates(nil, winEnv(map[string]string{"LOCALAPPDATA": `C:\L`}))
	for _, d := range dirs {
		if !strings.HasPrefix(d, `C:\L\`) {
			t.Fatalf("got %q, want only LOCALAPPDATA's", dirs)
		}
	}
	if len(dirs) != 2 {
		t.Fatalf("got %q, want the two per-user directories", dirs)
	}
}

func TestOnlyWindowsDirectoriesThatExistAreAdded(t *testing.T) {
	got, changed := extendFor("windows", staleAppPath, windowsCandidates(registryNow, noVars), exists())
	if changed || got != staleAppPath {
		t.Fatalf("got %q (changed %v), want the PATH untouched", got, changed)
	}
}

func TestAnEmptyWindowsPathGetsTheDirectoriesAlone(t *testing.T) {
	got, _ := extendFor("windows", "", []string{perUserDocker}, exists(perUserDocker))
	if got != perUserDocker {
		t.Fatalf("got %q", got)
	}
}
