package run

// The container engine's virtual machine: which one runs the engine this
// computer drives, how much memory it has, and how its memory is raised.
// Docker Engine and podman on Linux run on this computer itself and have no
// VM to size. Agentry's engine (Colima), Rancher Desktop and a podman
// machine each have a supported way to set their memory; another VM is
// sized in its own settings.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-cli/pkg/runner/procx"
)

// The kinds of virtual machine an engine can run in.
const (
	VMNone          = ""                // the engine runs on this computer
	VMManaged       = "managed"         // Agentry's engine (Colima)
	VMRancher       = "rancher-desktop" // Rancher Desktop
	VMPodmanMachine = "podman-machine"  // a podman machine
	VMDockerDesktop = "docker-desktop"  // Docker Desktop
	VMOther         = "other"           // OrbStack, Colima or Lima of the person's own, Windows
)

// VM is the virtual machine of the engine this computer drives.
type VM struct {
	// Kind is one of the VM constants; VMNone when the engine runs on this
	// computer.
	Kind string
	// Name says it in a sentence: "Agentry's engine", "Rancher Desktop".
	Name string
	// MemoryBytes is the memory the engine reports it has, 0 when it could
	// not be read.
	MemoryBytes uint64
	// Resizable says the app can raise its memory itself.
	Resizable bool
}

// InVM says the engine runs in a virtual machine.
func (v VM) InVM() bool { return v.Kind != VMNone }

// EngineVM names the virtual machine of the engine detection has adopted,
// and reads its memory. An engine not found is VMNone with no memory.
func EngineVM(ctx context.Context) VM {
	e := DetectEngine(ctx, false)
	if !e.Found {
		return VM{}
	}
	v := vmOf(runtime.GOOS, e)
	v.MemoryBytes = engineMemory(ctx, e)
	return v
}

// vmOf names an engine's virtual machine from how detection found it and
// where it answers.
func vmOf(goos string, e Engine) VM {
	ep := e.Endpoint
	switch {
	case e.How == managedHow:
		return VM{Kind: VMManaged, Name: "Agentry's engine", Resizable: true}
	case strings.Contains(ep, "/.rd/docker.sock") || strings.Contains(e.How, "Rancher Desktop"):
		return VM{Kind: VMRancher, Name: "Rancher Desktop", Resizable: goos != "windows"}
	case strings.Contains(e.How, "podman machine") || (e.Kind == dockerx.KindPodman && goos == "darwin"):
		return VM{Kind: VMPodmanMachine, Name: "the podman machine", Resizable: true}
	case strings.Contains(ep, "/.docker/run/docker.sock") || strings.Contains(ep, "/.docker/desktop/docker.sock") || strings.Contains(e.How, "Docker Desktop"):
		return VM{Kind: VMDockerDesktop, Name: "Docker Desktop"}
	case goos == "windows":
		return VM{Kind: VMOther, Name: "the container engine's virtual machine"}
	case goos == "darwin":
		return VM{Kind: VMOther, Name: "the container engine's virtual machine"}
	}
	return VM{}
}

// engineMemory is the memory the engine reports, in bytes; 0 when it
// cannot be read.
func engineMemory(ctx context.Context, e Engine) uint64 {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	kind := e.CLI
	if kind == "" {
		kind = e.Kind
	}
	format := "{{.MemTotal}}"
	if kind == dockerx.KindPodman {
		format = "{{.Host.MemTotal}}"
	}
	cmd := exec.CommandContext(ctx, engineBin(kind), "info", "--format", format) //nolint:gosec // the engine's own binary, fixed args
	procx.NoWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// ResizeVM raises the memory of the engine's virtual machine to memGiB and,
// for Agentry's engine, its processors to cpus, restarting it, and waits
// for the engine to answer again. Nothing may run on the engine meanwhile:
// the caller holds the setup lock.
func ResizeVM(ctx context.Context, v VM, memGiB, cpus int, logw io.Writer) error {
	if !v.Resizable {
		return fmt.Errorf("%s's memory is set in its own settings, not by Agentry Desktop", v.Name)
	}
	if memGiB < 1 {
		return errors.New("no memory size was given")
	}
	switch v.Kind {
	case VMManaged:
		return resizeManaged(ctx, memGiB, cpus, logw)
	case VMRancher:
		return resizeRancher(ctx, memGiB, logw)
	case VMPodmanMachine:
		return resizePodmanMachine(ctx, memGiB, logw)
	}
	return fmt.Errorf("%s cannot be resized by Agentry Desktop", v.Name)
}

// resizeManaged records the size Agentry's engine starts with, then stops
// and starts it with that size.
func resizeManaged(ctx context.Context, memGiB, cpus int, logw io.Writer) error {
	e := managedEngine()
	if e == nil || !e.Installed() {
		return errors.New("the app's own engine is not installed")
	}
	e.Log = logw
	if err := e.SetResources(memGiB, cpus); err != nil {
		return err
	}
	if err := e.Stop(ctx); err != nil {
		return err
	}
	return startManaged(ctx, e, logw)
}

// rdctl is Rancher Desktop's command: on PATH, else under ~/.rd/bin.
func rdctl() string {
	if p, err := lookPath("rdctl"); err == nil {
		return p
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".rd", "bin", "rdctl")
	}
	return "rdctl"
}

// resizeRancher sets Rancher Desktop's virtual machine memory; Rancher
// Desktop restarts its VM to apply it.
func resizeRancher(ctx context.Context, memGiB int, logw io.Writer) error {
	_, _ = fmt.Fprintf(logw, "Giving Rancher Desktop %d GB of memory; it restarts its virtual machine…\n", memGiB)
	cmd := exec.CommandContext(ctx, rdctl(), "set", "--virtual-machine.memory-in-gb="+strconv.Itoa(memGiB)) //nolint:gosec // Rancher Desktop's own command, fixed args
	procx.NoWindow(cmd)
	cmd.Stdout, cmd.Stderr = logw, logw
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rdctl set: %w", err)
	}
	// rdctl may return before the restart begins: wait for the engine to
	// go, then for it to answer again.
	waitForEngineDown(ctx, rancherGoWait)
	return waitForEngine(ctx, logw)
}

// resizePodmanMachine stops the default podman machine, sets its memory and
// starts it again.
func resizePodmanMachine(ctx context.Context, memGiB int, logw io.Writer) error {
	bin := engineBin(dockerx.KindPodman)
	step := func(args ...string) error {
		cmd := exec.CommandContext(ctx, bin, args...) //nolint:gosec // the engine's own binary, fixed args
		procx.NoWindow(cmd)
		cmd.Stdout, cmd.Stderr = logw, logw
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("podman %s: %w", strings.Join(args, " "), err)
		}
		return nil
	}
	_, _ = fmt.Fprintf(logw, "Giving the podman machine %d GB of memory; it is stopped and started again…\n", memGiB)
	if err := step("machine", "stop"); err != nil {
		return err
	}
	if err := step("machine", "set", "--memory", strconv.Itoa(memGiB*1024)); err != nil {
		_ = step("machine", "start")
		return err
	}
	if err := step("machine", "start"); err != nil {
		return err
	}
	return waitForEngine(ctx, logw)
}

// rancherGoWait bounds the wait for Rancher Desktop's engine to go once its
// memory is set.
var rancherGoWait = time.Minute

// resizeWait bounds how long a resized engine may take to answer again.
var resizeWait = 5 * time.Minute

// engineAnswers says whether the adopted engine answers now, with the
// line that says what it is; a test substitutes its own.
var engineAnswers = func(ctx context.Context) (bool, string) {
	e := DetectEngine(ctx, false)
	return e.Found, e.Summary()
}

// enginePoll is how often a resized engine is asked whether it answers.
var enginePoll = 2 * time.Second

// waitForEngineDown waits, at most for, for the adopted engine to stop
// answering.
func waitForEngineDown(ctx context.Context, most time.Duration) {
	deadline := time.Now().Add(most)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		if ok, _ := engineAnswers(ctx); !ok {
			return
		}
		time.Sleep(enginePoll)
	}
}

// waitForEngine waits for the adopted engine to answer again after its
// virtual machine restarted.
func waitForEngine(ctx context.Context, logw io.Writer) error {
	deadline := time.Now().Add(resizeWait)
	for {
		if ok, line := engineAnswers(ctx); ok {
			_, _ = fmt.Fprintf(logw, "%s\n", line)
			return nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return errors.New("the engine did not answer again after its virtual machine restarted: Re-detect in Settings → Runs → Container engine once it is up")
		}
		time.Sleep(enginePoll)
	}
}
