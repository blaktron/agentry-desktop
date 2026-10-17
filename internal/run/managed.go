package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/dockerx"
	"github.com/blaktron/agentry-desktop/internal/managedengine"
)

// The engine the app manages itself on macOS (internal/managedengine):
// offered on the prerequisites card when the computer offers no engine,
// started when the app opens, stopped when it quits, and the first place
// detection asks, since the app put it there.

// Managed is the managed engine's part of the prerequisites story.
type Managed struct {
	// Supported says this build can manage an engine here (macOS, an
	// architecture the pinned files cover).
	Supported bool `json:"supported"`
	// Installed says its binaries are in place; Current that they are the
	// versions this app pins. Running says its VM answers; Starting that
	// the app is starting it now (at launch, or from the card).
	Installed bool `json:"installed"`
	Current   bool `json:"current"`
	Running   bool `json:"running"`
	Starting  bool `json:"starting"`
	// SocketPathError, when set, is why this computer cannot have the
	// engine: its socket path would be too long for macOS to bind. The
	// card says it instead of offering the click.
	SocketPathError string `json:"socketPathError,omitempty"`
	// Dir is where it lives, Socket where it serves the Docker API, Versions
	// the three it is made of, Licences their licences, DownloadMB about
	// what the install fetches.
	Dir        string `json:"dir,omitempty"`
	Socket     string `json:"socket,omitempty"`
	Versions   string `json:"versions,omitempty"`
	Licences   string `json:"licences,omitempty"`
	DownloadMB int    `json:"downloadMb,omitempty"`
}

// The states the managed engine adds to the card.
const (
	// StateManagedOffer: no engine answers, and the app can install one it
	// runs itself in one click.
	StateManagedOffer = "managedOffer"
	// StateManagedStopped: the app's engine is installed and does not
	// answer; one click starts it, another removes it.
	StateManagedStopped = "managedStopped"
)

// managedState is the card's state off Linux: ready when an engine answers;
// the app's own engine stopped when it is installed and none does; the
// offer where this build can install one; else the hand-off to the choices.
func managedState(dockerOK bool, m Managed) string {
	switch {
	case dockerOK:
		return StateReady
	case m.Installed:
		return StateManagedStopped
	case m.Supported:
		return StateManagedOffer
	default:
		return StateManual
	}
}

// managedHow is how detection names the engine the app manages.
const managedHow = "Agentry's engine (Colima)"

// managedEngine is the engine the app manages, nil where it manages none;
// a var so a test hands it one under a temporary directory.
var managedEngine = func() *managedengine.Engine {
	if !managedengine.Supported() {
		return nil
	}
	e, err := managedengine.Default()
	if err != nil {
		return nil
	}
	e.Ping = dockerx.PingSocket
	return e
}

// ManagedVersions is the three versions in one phrase, ManagedLicences
// their licences, for the consent the app asks before the install.
func ManagedVersions() string {
	return fmt.Sprintf("Colima %s, Lima %s and the docker command %s", managedengine.ColimaVersion, managedengine.LimaVersion, managedengine.DockerVersion)
}

func ManagedLicences() string { return managedengine.Licences }

// ManagedDownloadMB is about what the install fetches.
func ManagedDownloadMB() int { return managedengine.DownloadMB }

// managedStory is the managed engine's facts for the card.
func managedStory(ctx context.Context) Managed {
	e := managedEngine()
	if e == nil {
		return Managed{}
	}
	m := Managed{Supported: true, Dir: e.Dir, Socket: e.Socket(), Versions: ManagedVersions(), Licences: managedengine.Licences, DownloadMB: managedengine.DownloadMB, Starting: managedStarting.Load()}
	if err := e.CheckSocketPath(); err != nil {
		m.SocketPathError = err.Error()
	}
	if e.Installed() {
		m.Installed = true
		m.Current = e.Current()
		m.Running = pingSocket(ctx, e.Socket()) == nil
	}
	return m
}

// managedStarting is set while the app starts its engine, so a page's read
// in that window says starting rather than stopped.
var managedStarting atomic.Bool

// IsManaged says whether the engine found is the app's own.
func (e Engine) IsManaged() bool { return e.Found && e.How == managedHow }

// adoptManaged makes the managed engine the one every client drives: its
// socket as the endpoint, its own docker command as the CLI, its own
// docker configuration in place of the person's (whose credential helper
// may be another engine's and fail every pull).
func adoptManaged(e *managedengine.Engine) {
	adoptVar("DOCKER_HOST", e.Endpoint())
	adoptVar(dockerx.EnvDocker, e.Docker())
	adoptVar("DOCKER_CONFIG", e.DockerConfigDir())
}

// managedStartWait bounds a start: the first creates the VM and fetches
// its image, later ones boot it. managedUpdateWait bounds the re-download
// the app makes at launch when its pins moved.
const (
	managedStartWait  = 5 * time.Minute
	managedUpdateWait = 15 * time.Minute
)

// InstallManagedEngine installs the app's own engine and starts it, then
// adopts it. Output streams to logw. It applies only where no engine
// answers and the app can manage one.
func InstallManagedEngine(ctx context.Context, logw io.Writer) error {
	e := managedEngine()
	if e == nil {
		return errors.New("this build manages no container engine on this computer")
	}
	if p := DetectEngine(ctx, false); p.Found {
		return fmt.Errorf("a container engine already answers (%s); nothing to install", p.Summary())
	}
	e.Log = logw
	if err := e.Install(ctx); err != nil {
		return err
	}
	return startManaged(ctx, e, logw)
}

// StartManagedEngine starts the app's installed engine and adopts it; for
// the card's click when it is stopped.
func StartManagedEngine(ctx context.Context, logw io.Writer) error {
	e := managedEngine()
	if e == nil || !e.Installed() {
		return errors.New("the app's engine is not installed on this computer")
	}
	e.Log = logw
	return startManaged(ctx, e, logw)
}

// startManaged starts the engine, bounded, and adopts it under the
// detection lock, so a page's read that follows finds it.
func startManaged(ctx context.Context, e *managedengine.Engine, logw io.Writer) error {
	managedStarting.Store(true)
	defer managedStarting.Store(false)
	ctx, cancel := context.WithTimeout(ctx, managedStartWait)
	defer cancel()
	if err := e.Start(ctx); err != nil {
		return err
	}
	adoptMu.Lock()
	defer adoptMu.Unlock()
	clearAdoption()
	adoptManaged(e)
	d, err := describeEngine(ctx, newDockerClient())
	if err != nil {
		clearAdoption()
		return fmt.Errorf("the engine started but the docker command cannot drive it: %w", err)
	}
	d.How = managedHow
	adopted = &d
	_, _ = fmt.Fprintf(logw, "%s\n", d.Summary())
	return nil
}

// StartManagedEngineAtLaunch is the app's own start when it opens: nothing
// where the engine is not installed, where the app was started with an
// engine named in its environment or with another engine chosen in
// Settings, or where another engine already answers (a Docker Desktop installed since; the person removes the app's
// engine from the card when it is no longer wanted); a re-install first
// where the app's pins moved, bounded, and a failed one leaves the
// installed engine to start; errors go to logw and nowhere else, since the
// card says what detection then finds.
func StartManagedEngineAtLaunch(ctx context.Context, logw io.Writer) {
	e := managedEngine()
	if e == nil || !e.Installed() {
		return
	}
	if launchNamed() {
		_, _ = fmt.Fprintln(logw, "Agentry's engine is installed and left stopped: the app was started with an engine named in its environment")
		return
	}
	if c := loadEngineChoice(); c != "" && c != ChoiceManaged {
		_, _ = fmt.Fprintf(logw, "Agentry's engine is installed and left stopped: the engine chosen in Settings is %s\n", ChoiceLabel(c))
		return
	}
	if found := DetectEngine(ctx, true); found.Found && found.How != managedHow {
		_, _ = fmt.Fprintf(logw, "Agentry's engine is installed and left stopped: %s answers\n", found.Summary())
		return
	}
	e.Log = logw
	if !e.Current() {
		// A failed update (offline, a publisher down) leaves the installed
		// engine, which still runs; the next launch tries again. An update
		// interrupted partway leaves nothing installed, and the card offers
		// the install again.
		_, _ = fmt.Fprintf(logw, "Agentry's engine is behind this app's versions (%s); updating it.\n", ManagedVersions())
		uctx, cancel := context.WithTimeout(ctx, managedUpdateWait)
		err := e.Install(uctx)
		cancel()
		if err != nil {
			_, _ = fmt.Fprintf(logw, "update Agentry's engine: %v\n", err)
			if !e.Installed() {
				return
			}
			_, _ = fmt.Fprintln(logw, "starting the installed one")
		}
	}
	if err := startManaged(ctx, e, logw); err != nil {
		_, _ = fmt.Fprintf(logw, "start Agentry's engine: %v\n", err)
	}
}

// StopManagedEngine stops the app's engine when the app quits; bounded,
// errors to logw.
func StopManagedEngine(ctx context.Context, logw io.Writer) {
	e := managedEngine()
	if e == nil || !e.Installed() {
		return
	}
	e.Log = logw
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if err := e.Stop(ctx); err != nil {
		_, _ = fmt.Fprintf(logw, "at quit: %v\n", err)
	}
}

// RemoveManagedEngine stops and deletes the app's engine with its data and
// forgets the adoption, so detection looks for another.
func RemoveManagedEngine(ctx context.Context, logw io.Writer) error {
	e := managedEngine()
	if e == nil {
		return errors.New("this build manages no container engine on this computer")
	}
	e.Log = logw
	if err := e.Remove(ctx); err != nil {
		return err
	}
	adoptMu.Lock()
	defer adoptMu.Unlock()
	if adopted != nil && adopted.How == managedHow {
		clearAdoption()
	}
	return nil
}
