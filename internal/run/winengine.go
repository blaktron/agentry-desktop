package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/blaktron/agentry-desktop/internal/windowsengine"
)

// The engine the app installs for the person on Windows
// (internal/windowsengine): Rancher Desktop with its dockerd engine,
// offered on the prerequisites card when no engine answers and none is
// installed, in two steps where Windows Subsystem for Linux is not yet on
// (WSL with administrator consent and a restart, then Rancher Desktop for
// this user). Unlike the macOS engine it stays the person's own
// application: the app starts it when it opens if the app installed it,
// and never stops or removes it.

// WindowsEngine is the Windows engine's part of the prerequisites story.
type WindowsEngine struct {
	// Supported says this build can install it here (Windows on x86-64).
	Supported bool `json:"supported"`
	// RancherInstalled says Rancher Desktop is installed, by the app or by
	// the person; InstalledByApp that the app installed it.
	RancherInstalled bool `json:"rancherInstalled"`
	InstalledByApp   bool `json:"installedByApp"`
	// DockerDesktopInstalled says Docker Desktop is installed, which the
	// card then asks the person to start rather than offering another.
	DockerDesktopInstalled bool `json:"dockerDesktopInstalled"`
	// WSL is windowsengine.WSL: ready, missing or restartPending.
	WSL string `json:"wsl"`
	// Starting says the app is starting Rancher Desktop now.
	Starting bool `json:"starting"`
	// Version is the Rancher Desktop the app installs, Licence its
	// licence, DownloadMB about what the installer weighs.
	Version    string `json:"version,omitempty"`
	Licence    string `json:"licence,omitempty"`
	DownloadMB int    `json:"downloadMb,omitempty"`
}

// The states the Windows engine adds to the card.
const (
	// StateWindowsOffer: no engine answers or is installed, and the app can
	// install Rancher Desktop (after WSL, where WSL is missing).
	StateWindowsOffer = "windowsOffer"
	// StateWindowsRestart: the app installed WSL and Windows has not
	// restarted since; Rancher Desktop installs after the restart.
	StateWindowsRestart = "windowsRestart"
	// StateWindowsStopped: Rancher Desktop is installed and its engine does
	// not answer; one click starts it.
	StateWindowsStopped = "windowsStopped"
)

// windowsState is the card's state on Windows.
func windowsState(dockerOK bool, w WindowsEngine) string {
	switch {
	case dockerOK:
		return StateReady
	case w.RancherInstalled:
		return StateWindowsStopped
	case w.WSL == string(windowsengine.WSLRestartPending):
		return StateWindowsRestart
	case w.Supported && !w.DockerDesktopInstalled:
		return StateWindowsOffer
	default:
		return StateManual
	}
}

// windowsInstaller is the Windows engine's installer, nil where this build
// installs none; a var so a test hands it one under a temporary directory.
var windowsInstaller = func() *windowsengine.Installer {
	if !windowsengine.Supported() {
		return nil
	}
	i, err := windowsengine.Default()
	if err != nil {
		return nil
	}
	return i
}

// dockerDesktopInstalled says whether Docker Desktop is installed on this
// computer, for this user or for the machine; a var for tests.
var dockerDesktopInstalled = func() bool {
	for _, p := range []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Docker", "Docker", "Docker Desktop.exe"),
		filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "DockerDesktop", "Docker Desktop.exe"),
	} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

// windowsStarting is set while the app starts Rancher Desktop.
var windowsStarting atomic.Bool

// windowsStory is the Windows engine's facts for the card.
func windowsStory(ctx context.Context) WindowsEngine {
	i := windowsInstaller()
	if i == nil {
		return WindowsEngine{}
	}
	w := WindowsEngine{
		Supported: true, Version: windowsengine.RancherVersion, Licence: windowsengine.Licence, DownloadMB: windowsengine.DownloadMB,
		RancherInstalled: i.RancherInstalled(), InstalledByApp: i.InstalledByApp(),
		DockerDesktopInstalled: dockerDesktopInstalled(), Starting: windowsStarting.Load(),
	}
	if !w.RancherInstalled {
		w.WSL = string(i.WSLState(ctx))
	}
	return w
}

// windowsReadyWait bounds the wait for Rancher Desktop's engine to answer
// after a start: the first imports its WSL distributions.
var windowsReadyWait = 10 * time.Minute

// windowsDetect is the detection the wait after a start runs; a var so a
// test can have the engine answer once Rancher Desktop was started.
var windowsDetect = func(ctx context.Context) Engine { return DetectEngine(ctx, true) }

// ErrWindowsRestartNeeded is the install's first step done: WSL is
// installed and Windows must restart before Rancher Desktop can.
var ErrWindowsRestartNeeded = errors.New("restart Windows to finish: Windows Subsystem for Linux is installed, and the engine installs after the restart")

// InstallWindowsEngine installs Rancher Desktop for this user and starts it
// with its dockerd engine, or, where WSL is missing, installs WSL first and
// returns ErrWindowsRestartNeeded. It applies only where no engine answers.
func InstallWindowsEngine(ctx context.Context, logw io.Writer) error {
	i := windowsInstaller()
	if i == nil {
		return errors.New("this build installs no container engine on this computer")
	}
	if p := DetectEngine(ctx, false); p.Found {
		return fmt.Errorf("a container engine already answers (%s); nothing to install", p.Summary())
	}
	i.Log = logw
	switch i.WSLState(ctx) {
	case windowsengine.WSLRestartPending:
		return ErrWindowsRestartNeeded
	case windowsengine.WSLMissing:
		if err := i.InstallWSL(ctx); err != nil {
			return err
		}
		return ErrWindowsRestartNeeded
	}
	msi, err := i.Download(ctx)
	if err != nil {
		return err
	}
	if err := i.InstallMSI(ctx, msi); err != nil {
		return err
	}
	return startWindows(ctx, i, true, logw)
}

// StartWindowsEngine starts an installed Rancher Desktop, for the card when
// it is stopped.
func StartWindowsEngine(ctx context.Context, logw io.Writer) error {
	i := windowsInstaller()
	if i == nil || !i.RancherInstalled() {
		return errors.New("no Rancher Desktop is installed on this computer")
	}
	i.Log = logw
	return startWindows(ctx, i, false, logw)
}

// startWindows starts Rancher Desktop and waits, bounded, until detection
// finds an engine; the docker command it ships is on the app's PATH by
// then (detection extends PATH with Rancher Desktop's directory).
func startWindows(ctx context.Context, i *windowsengine.Installer, first bool, logw io.Writer) error {
	windowsStarting.Store(true)
	defer windowsStarting.Store(false)
	if err := i.Start(ctx, first); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(logw, "Waiting for Rancher Desktop's engine to answer (its first start sets up its WSL distributions)…")
	deadline := time.Now().Add(windowsReadyWait)
	for {
		if e := windowsDetect(ctx); e.Found {
			_, _ = fmt.Fprintf(logw, "%s\n", e.Summary())
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("no engine answered within %s of Rancher Desktop's start; open Rancher Desktop to see what it says", windowsReadyWait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
}

// StartWindowsEngineAtLaunch starts Rancher Desktop when the app opens, in
// the background, where the app installed it, no engine answers, and the
// app was not started with an engine named in its environment. Errors go
// to logw; the card says what detection then finds.
func StartWindowsEngineAtLaunch(ctx context.Context, logw io.Writer) {
	i := windowsInstaller()
	if i == nil || !i.InstalledByApp() || launchNamed() {
		return
	}
	if found := DetectEngine(ctx, true); found.Found {
		return
	}
	i.Log = logw
	if err := startWindows(ctx, i, false, logw); err != nil {
		_, _ = fmt.Fprintf(logw, "start Rancher Desktop: %v\n", err)
	}
}
