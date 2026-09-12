package run

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
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

// fixtureMachine points every detection seam at fixtures: the engine
// answers or not, the unit and socket, /etc/group membership, and the
// groups this process carries (agentry-desktop#64).
type fixtureMachine struct {
	dockerErr   error
	unitActive  bool
	socket      bool
	groupLine   string // the docker line of /etc/group, "" for none
	processGIDs []int
	pkexec      bool
}

func withMachine(t *testing.T, m fixtureMachine) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("linux-only detection")
	}
	withOSRelease(t, "ID=fedora\n")
	f := &dockerx.Fake{AvailableErr: m.dockerErr}
	withFakeDocker(t, f)
	dir := t.TempDir()
	gp := filepath.Join(dir, "group")
	content := "root:x:0:\nwheel:x:10:someone\n"
	if m.groupLine != "" {
		content += m.groupLine + "\n"
	}
	if err := os.WriteFile(gp, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(dir, "docker.sock")
	if m.socket {
		if err := os.WriteFile(sock, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	oldGroup, oldSock, oldUnit, oldGroups, oldLook := groupPath, dockerSocketPath, dockerUnitActive, processGroups, lookPath
	groupPath, dockerSocketPath = gp, sock
	dockerUnitActive = func(context.Context) bool { return m.unitActive }
	processGroups = func() []int { return m.processGIDs }
	lookPath = func(file string) (string, error) {
		if file == "pkexec" && !m.pkexec {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + file, nil
	}
	t.Cleanup(func() {
		groupPath, dockerSocketPath, dockerUnitActive, processGroups, lookPath = oldGroup, oldSock, oldUnit, oldGroups, oldLook
	})
}

func TestDetectStates(t *testing.T) {
	me, _ := currentUsername()
	refused := errors.New("permission denied while trying to connect to the docker daemon socket")
	cases := []struct {
		name string
		m    fixtureMachine
		want string
	}{
		{"engine answers", fixtureMachine{pkexec: true}, StateReady},
		{"nothing installed, pkexec present", fixtureMachine{dockerErr: refused, pkexec: true}, StateInstallable},
		{"nothing installed, no pkexec", fixtureMachine{dockerErr: refused}, StateManual},
		{"installed, not in the group", fixtureMachine{dockerErr: refused, unitActive: true, groupLine: "docker:x:986:", pkexec: true}, StateGroupMissing},
		{"installed, in the group in /etc/group, socket still refuses", fixtureMachine{dockerErr: refused, socket: true, groupLine: "docker:x:986:" + me, processGIDs: []int{1000, 10}, pkexec: true}, StatePendingRestart},
		{"installed, in the group here and now, still refused", fixtureMachine{dockerErr: errors.New("daemon not running"), unitActive: true, groupLine: "docker:x:986:other," + me, processGIDs: []int{1000, 986}, pkexec: true}, StateManual},
		{"installed, no docker group at all", fixtureMachine{dockerErr: refused, unitActive: true, pkexec: true}, StateGroupMissing},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withMachine(t, c.m)
			p := DetectPrereqs(t.Context())
			if p.State != c.want {
				t.Fatalf("state = %q, want %q (%+v)", p.State, c.want, p)
			}
			// The group command exists only where the app can elevate.
			want := "usermod -aG docker " + me
			if !c.m.pkexec {
				want = ""
			}
			if p.GroupCommand != want {
				t.Fatalf("group command = %q, want %q", p.GroupCommand, want)
			}
		})
	}
}

// AddToDockerGroup applies only in groupMissing; elsewhere it names the
// state instead of running usermod.
func TestAddToDockerGroupRefusesOutsideGroupMissing(t *testing.T) {
	withMachine(t, fixtureMachine{pkexec: true})
	var out strings.Builder
	err := AddToDockerGroup(t.Context(), &out)
	if err == nil || !strings.Contains(err.Error(), "ready") {
		t.Fatalf("ready machine must refuse the group step: %v", err)
	}
}

// Restart never runs as root: the plain request first, pkexec only when
// polkit refuses, and a failure of both is an error, not a fallback to
// anything else.
func TestRestartAsksSessionManagerThenElevates(t *testing.T) {
	var calls []bool
	old := restartCommand
	restartCommand = func(_ context.Context, elevated bool) *exec.Cmd {
		calls = append(calls, elevated)
		if elevated {
			return exec.Command("true")
		}
		return exec.Command("false")
	}
	t.Cleanup(func() { restartCommand = old })
	var out strings.Builder
	if err := Restart(t.Context(), &out); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] || !calls[1] {
		t.Fatalf("calls = %v, want plain then elevated", calls)
	}
	restartCommand = func(context.Context, bool) *exec.Cmd { return exec.Command("false") }
	if err := Restart(t.Context(), &out); err == nil {
		t.Fatal("both refusals must surface as an error")
	}
}

// The group step runs exactly the usermod half through the elevated seam
// and reports pendingRestart afterwards: /etc/group now lists the user, the
// process's groups still do not.
func TestAddToDockerGroupRunsUsermodThenPendingRestart(t *testing.T) {
	me, _ := currentUsername()
	withMachine(t, fixtureMachine{dockerErr: errors.New("permission denied"), unitActive: true, groupLine: "docker:x:986:", pkexec: true})
	var ran []string
	old := elevatedShell
	elevatedShell = func(_ context.Context, command string) *exec.Cmd {
		ran = append(ran, command)
		// The real usermod rewrites the group database; do the same to the fixture.
		if err := os.WriteFile(groupPath, []byte("root:x:0:\ndocker:x:986:"+me+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return exec.Command("true")
	}
	t.Cleanup(func() { elevatedShell = old })
	var out strings.Builder
	if err := AddToDockerGroup(t.Context(), &out); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || ran[0] != "usermod -aG docker "+me {
		t.Fatalf("ran %v, want only the usermod half", ran)
	}
	if p := DetectPrereqs(t.Context()); p.State != StatePendingRestart {
		t.Fatalf("after the group step: %q, want pendingRestart", p.State)
	}
}
