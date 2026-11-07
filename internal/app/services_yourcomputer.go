// Your computer: the link service a run uses applications on this computer
// through, and what the page shows and does about it. The link service
// serves the link socket in the runner home while the app runs, for a run
// from the CLI on this computer, and binds the app's own runs in process.
// Settings → Your computer lists the catalogue's applications with their
// state here and the step to take, the per-application script-screening switch,
// the runs using the computer now with Pause and Stop, and the runs that
// used it. The run setup reads the same state for each application a
// release declares.
//
// What the page asks for is a claim: turning an application's script
// screening off is asked again in a native dialog, and so is a paused run's
// Resume; an exec call is put to the person in a native dialog the page
// cannot answer.
// Pause and Stop only take away.
package app

import (
	"cmp"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"

	"github.com/blaktron/agentry-desktop/internal/linkservice"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// eventYourComputer tells the page a run bound, changed state or ended.
const eventYourComputer = "yourcomputer:changed"

// YourComputerAPI is Settings → Your computer and the run setup's cards.
type YourComputerAPI struct {
	app  *App
	run  *RunAPI
	link *linkservice.Service
	// socket is the link socket's path while it is served, and socketErr
	// why it is not; mu holds them and stop.
	mu        sync.Mutex
	socket    string
	socketErr string
	stop      context.CancelFunc
}

// newYourComputerAPI builds the link service the app's runs and the CLI's
// bind to.
func newYourComputerAPI(app *App, run *RunAPI) *YourComputerAPI {
	y := &YourComputerAPI{app: app, run: run}
	linkservice.SetClientVersion(version.Version)
	var recent *linkservice.Recent
	if dir, err := configdir.Dir(); err == nil {
		recent = linkservice.OpenRecent(filepath.Join(dir, "desktop-link-runs.json"))
	}
	y.link = linkservice.New(linkservice.Options{
		Ask:          y.askExec,
		ScreeningOff: screeningOff,
		Recent:       recent,
		Changed:      func() { nativeEmit(eventYourComputer) },
	})
	run.link = y.link
	return y
}

func screeningOff(id string) bool {
	st, err := settings.Load()
	return err == nil && st.IsScreeningOff(id)
}

// start serves the link socket in the runner home until the app quits.
func (y *YourComputerAPI) start(ctx context.Context) {
	y.mu.Lock()
	defer y.mu.Unlock()
	home, err := y.run.runnerHome()
	if err != nil {
		y.socketErr = err.Error()
		return
	}
	ln, path, err := linkservice.Listen(home)
	if err != nil {
		y.socketErr = err.Error()
		return
	}
	sctx, cancel := context.WithCancel(ctx)
	y.socket, y.stop = path, cancel
	go func() { _ = y.link.Serve(sctx, ln, path) }()
}

// shutdown closes the socket, then stops every run's use of the computer.
func (y *YourComputerAPI) shutdown() {
	y.mu.Lock()
	stop := y.stop
	y.mu.Unlock()
	if stop != nil {
		stop()
	}
	y.link.StopAll()
}

// bindDesktop binds one of the app's own runs in process.
func (r *RunAPI) bindDesktop(ctx context.Context, req desktop.BindRequest) (*engine.DesktopLink, error) {
	if r.link == nil {
		return nil, desktop.ErrNoService
	}
	return r.link.BindInProcess(ctx, req)
}

// askExec is an exec call's native dialog: what would run, quoted, and
// three answers, of which declining is the default and the cancel.
func (y *YourComputerAPI) askExec(ctx context.Context, q linkservice.ExecQuestion) (linkservice.ExecAnswer, error) {
	if y.app.chooseExec != nil {
		return y.app.chooseExec(ctx, q)
	}
	return nativeAskExec(ctx, q)
}

// execDialogText is an exec call's dialog: its title and message.
func execDialogText(q linkservice.ExecQuestion) (title, message string) {
	who := q.Who
	if who == "" {
		who = "A QuickAgent"
	}
	title = "Run code inside " + q.Application + "?"
	message = fmt.Sprintf("%s asks to run this inside %s on your computer, through its tool %s:\n\n%s\n\nIt runs as you, with your files and your network, outside the run's sandbox.",
		who, q.Application, q.Tool, q.What)
	return title, message
}

// The exec dialog's buttons.
const (
	execRunIt    = "Run it"
	execAllowRun = "Allow every exec call for this run"
	execDecline  = "Don't run it"
)

// YourComputerView is Settings → Your computer.
type YourComputerView struct {
	// Applications are the catalogue's, with their state on this computer.
	Applications []linkservice.AppState `json:"applications"`
	// Using are the runs bound now.
	Using []linkservice.Bound `json:"using"`
	// Recent are the runs that used the computer, the newest first.
	Recent []linkservice.RecentRun `json:"recent"`
	// Socket is the link socket a run from the CLI binds over, and
	// SocketError why it is not served.
	Socket      string `json:"socket,omitempty"`
	SocketError string `json:"socketError,omitempty"`
}

// State is Settings → Your computer.
func (y *YourComputerAPI) State() *YourComputerView {
	ctx, cancel := context.WithTimeout(y.app.ctxOrBackground(), 10*time.Second)
	defer cancel()
	y.mu.Lock()
	socket, socketErr := y.socket, y.socketErr
	y.mu.Unlock()
	return &YourComputerView{
		Applications: y.link.States(ctx),
		Using:        y.link.Runs(),
		Recent:       y.recent(),
		Socket:       socket,
		SocketError:  socketErr,
	}
}

func (y *YourComputerAPI) recent() []linkservice.RecentRun {
	if l := y.link.Recent(); l != nil {
		return l
	}
	return []linkservice.RecentRun{}
}

// Applications is the state of each application a release declares, by
// catalogue id, for the run setup's cards: an id this app's catalogue does
// not know is an application that is not ready, with the step to take.
func (y *YourComputerAPI) Applications(ids []string) []linkservice.AppState {
	ctx, cancel := context.WithTimeout(y.app.ctxOrBackground(), 10*time.Second)
	defer cancel()
	out := make([]linkservice.AppState, 0, len(ids))
	for _, id := range ids {
		st, ok := y.link.State(ctx, id)
		if !ok {
			st = linkservice.AppState{ID: id, Application: id, Missing: "This Agentry Desktop does not know the application " + id + ": update Agentry Desktop."}
		}
		out = append(out, st)
	}
	return out
}

// SetScreening turns an application server's own script screening on or off.
// Lifting it is asked again in a native dialog; turning it on asks
// nothing. It applies from the next run that binds the application.
func (y *YourComputerAPI) SetScreening(id string, on bool) (*YourComputerView, error) {
	st, ok := y.link.State(y.app.ctxOrBackground(), id)
	if !ok || st.ScreeningVariable == "" {
		return nil, fmt.Errorf("%s has no script screening to switch", id)
	}
	if !on {
		msg := fmt.Sprintf("%s's installed server screens each script for file, process, network and persistence calls before it runs (%s=1). With screening off, a script an agent sends runs inside %s as it is, once you allow it.\n\nTurn script screening off for %s?", st.Application, st.ScreeningVariable, st.Application, st.Application)
		if err := y.app.askConsent("Turn off "+st.Application+"'s script screening?", msg); err != nil {
			return nil, err
		}
	}
	if err := settings.SetScreeningOff(id, !on); err != nil {
		return nil, err
	}
	nativeEmit(eventYourComputer)
	return y.State(), nil
}

// Pause refuses a run's later calls to applications on this computer
// until Resume.
func (y *YourComputerAPI) Pause(run string) error { return y.link.Pause(run) }

// Resume lets a paused run's calls through again, once the person says so
// in a native dialog: the page may take a run's use of the computer away,
// never give it back.
func (y *YourComputerAPI) Resume(run string) error {
	who := run
	for _, b := range y.link.Runs() {
		if b.Run == run {
			who = cmp.Or(b.Agent, b.Ref, run)
			if err := y.app.askConsent("Let it use your computer again?", who+" is paused and cannot use "+strings.Join(b.Applications, ", ")+" on your computer. Let it use them again?"); err != nil {
				return err
			}
			return y.link.Resume(run)
		}
	}
	return fmt.Errorf("no run %s is using your computer", who)
}

// Stop ends a run's calls in flight and its installed servers, and refuses
// every later call of the run.
func (y *YourComputerAPI) Stop(run string) error { return y.link.Stop(run) }
