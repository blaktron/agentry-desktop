// Your computer: the link service a run uses applications on this computer
// through, and what the page shows and does about it. The link service
// serves the link socket in the runner home while the app runs, for a run
// from the CLI on this computer, and binds the app's own runs in process.
// Plugins → Computer use lists the catalogue's applications with their
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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
	"github.com/blaktron/agentry-desktop/internal/linkservice"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/tray"
	"github.com/blaktron/agentry-desktop/internal/version"
)

// eventYourComputer tells the page a run bound, changed state or ended.
const eventYourComputer = "yourcomputer:changed"

// YourComputerAPI is Plugins → Computer use and the run setup's cards.
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
	// onComputer is told computer use started, changed or ended, for the
	// tray and the banner; set once the app has started.
	onComputer func()
	banner     bannerWindow
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
	if dir, err := configdir.Dir(); err == nil {
		linkservice.SetTokenPath(filepath.Join(dir, "computer-use-portal.json"))
	}
	y.link = linkservice.New(linkservice.Options{
		Ask:          y.askExec,
		ScreeningOff: screeningOff,
		Recent:       recent,
		Changed: func() {
			nativeEmit(eventYourComputer)
			if f := y.onComputer; f != nil {
				go f()
			}
		},
		Computer: linkservice.ComputerOptions{
			Enabled: func() bool { st, err := settings.Load(); return err == nil && st.ComputerUse.On },
			Keep:    func() bool { st, err := settings.Load(); return err == nil && st.ComputerUse.Keep },
			StopKey: StopKeyName,
			AskScreen: func(ctx context.Context, who string) error {
				return y.app.askConsent("Let it see your whole screen?", who+" asks to see and use your whole screen on this computer for this run: every window, not only the applications it names. Agentry Desktop shows that it is working and you can stop it with "+StopKeyName+".\n\nLet it see and use your whole screen?")
			},
		},
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

// YourComputerView is Plugins → Computer use.
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
	// Computer is computer use's switches and what this session offers.
	Computer ComputerUseView `json:"computer"`
}

// ComputerUseView is Plugins → Computer use's computer use.
type ComputerUseView struct {
	On      bool                `json:"on"`
	Keep    bool                `json:"keep"`
	StopKey string              `json:"stopKey"`
	Support linkservice.Support `json:"support"`
}

// State is Plugins → Computer use.
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
		Computer:     computerUseView(),
	}
}

func computerUseView() ComputerUseView {
	st, _ := settings.Load()
	return ComputerUseView{On: st.ComputerUse.On, Keep: st.ComputerUse.Keep, StopKey: StopKeyName, Support: linkservice.ComputerSupport()}
}

// SetComputerUse turns computer use on or off. Turning it on is asked
// again in a native dialog; turning it off asks nothing, and stops every
// run's computer use now.
func (y *YourComputerAPI) SetComputerUse(on bool) (*YourComputerView, error) {
	if on {
		msg := "A QuickAgent that says it does computer use will be able to see and use the windows of the applications it names on this computer, while it runs and while you watch: Agentry Desktop shows that it is working and lets you pause or stop it from the tray, the banner or " + StopKeyName + ". " +
			computerAskWords() + "\n\nTurn computer use on?"
		if err := y.app.askConsent("Turn on computer use?", msg); err != nil {
			return nil, err
		}
	}
	if err := settings.SetComputerUse(on); err != nil {
		return nil, err
	}
	if on {
		// Where the system asks for permissions ahead of a run (macOS:
		// Screen Recording and Accessibility), it is asked now, from this
		// switch, never from a run.
		_ = linkservice.RequestComputerPermissions()
	}
	if !on {
		y.stopComputerUse()
	}
	nativeEmit(eventYourComputer)
	return y.State(), nil
}

// computerAskWords say what the system itself asks when computer use is
// turned on or first used.
func computerAskWords() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS asks you to allow Agentry Desktop Screen & System Audio Recording and Accessibility in System Settings; after allowing the first, quit Agentry Desktop and open it again."
	case "windows":
		return "Windows asks nothing more; an application run as administrator cannot take Agentry Desktop's input, and a run that uses one is refused."
	}
	return "Your desktop may ask you, through its own dialog, which window to share and whether to allow control of the pointer and the keyboard."
}

// computerPrivacyPanes are the System Settings panes a computer-use
// permission links to: the only addresses OpenComputerPrivacy opens.
var computerPrivacyPanes = map[string]string{
	linkservice.PermissionScreen:        "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture",
	linkservice.PermissionAccessibility: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility",
}

// OpenComputerPrivacy opens the macOS privacy settings pane of a permission
// computer use needs: Screen & System Audio Recording, or Accessibility.
func (y *YourComputerAPI) OpenComputerPrivacy(which string) error {
	pane, ok := computerPrivacyPanes[which]
	if !ok || runtime.GOOS != "darwin" {
		return fmt.Errorf("no privacy settings named %q to open here", which)
	}
	cmd := exec.Command("/usr/bin/open", pane)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// SetKeepWhatAgentSaw turns keeping each screenshot in the run's folder on
// or off, from the next run.
func (y *YourComputerAPI) SetKeepWhatAgentSaw(on bool) (*YourComputerView, error) {
	if err := settings.SetKeepWhatAgentSaw(on); err != nil {
		return nil, err
	}
	nativeEmit(eventYourComputer)
	return y.State(), nil
}

// ForgetComputerChoices drops what the desktop remembered of the windows
// and the control you allowed, so the next run asks again.
func (y *YourComputerAPI) ForgetComputerChoices() error {
	dir, err := configdir.Dir()
	if err != nil {
		return err
	}
	linkservice.NewTokens(filepath.Join(dir, "computer-use-portal.json")).Forget()
	return nil
}

// usingComputer says a run's computer use is going on, paused included.
func (y *YourComputerAPI) usingComputer() bool {
	if y == nil || y.link == nil {
		return false
	}
	for _, b := range y.link.Runs() {
		if b.ComputerUse != "" && b.State != linkservice.StateStopped {
			return true
		}
	}
	return false
}

// computerUsers are the runs whose computer use is going on.
func (y *YourComputerAPI) computerUsers() []linkservice.Bound {
	var out []linkservice.Bound
	if y == nil || y.link == nil {
		return nil
	}
	for _, b := range y.link.Runs() {
		if b.ComputerUse != "" && b.State != linkservice.StateStopped {
			out = append(out, b)
		}
	}
	return out
}

// computerWhat is who uses what, as the tray and the banner say it.
func computerWhat(users []linkservice.Bound) (what string, paused bool) {
	parts := make([]string, 0, len(users))
	paused = len(users) > 0
	for _, b := range users {
		who := cmp.Or(b.Agent, agentName(b.Ref), b.Run)
		where := "your screen"
		if b.ComputerUse == computeruse.ScopeApp {
			where = cmp.Or(strings.Join(b.ComputerApps, ", "), "the window you picked")
		}
		parts = append(parts, who+" in "+where)
		if b.State != linkservice.StatePaused {
			paused = false
		}
	}
	return strings.Join(parts, "; "), paused
}

// computerTray is computer use as the tray shows it.
func (y *YourComputerAPI) computerTray() tray.Computer {
	users := y.computerUsers()
	if len(users) == 0 {
		return tray.Computer{}
	}
	what, paused := computerWhat(users)
	return tray.Computer{Active: true, Paused: paused, What: what}
}

// pauseComputerUse pauses every run using the computer.
func (y *YourComputerAPI) pauseComputerUse() {
	for _, b := range y.computerUsers() {
		_ = y.link.Pause(b.Run)
	}
}

// resumeComputerUse asks, in a native dialog, to resume every paused run.
func (y *YourComputerAPI) resumeComputerUse() {
	for _, b := range y.computerUsers() {
		if b.State == linkservice.StatePaused {
			_ = y.Resume(b.Run)
		}
	}
}

// stopComputerUse stops every run using the computer: the stop of the
// tray, the banner and the stop key.
func (y *YourComputerAPI) stopComputerUse() {
	if y == nil || y.link == nil {
		return
	}
	for _, b := range y.link.Runs() {
		if b.ComputerUse != "" {
			_ = y.link.Stop(b.Run)
		}
	}
}

// ComputerBanner is the banner's line: who uses what, paused or not, and
// nothing when no run uses the computer.
func (y *YourComputerAPI) ComputerBanner() *BannerView {
	users := y.computerUsers()
	if len(users) == 0 {
		return &BannerView{}
	}
	what, paused := computerWhat(users)
	return &BannerView{Active: true, Paused: paused, What: what, StopKey: StopKeyName}
}

// BannerView is the computer-use banner.
type BannerView struct {
	Active  bool   `json:"active"`
	Paused  bool   `json:"paused"`
	What    string `json:"what"`
	StopKey string `json:"stopKey"`
}

// PauseComputerUse is the banner's Pause.
func (y *YourComputerAPI) PauseComputerUse() { y.pauseComputerUse() }

// ResumeComputerUse is the banner's Resume, asked again natively.
func (y *YourComputerAPI) ResumeComputerUse() { y.resumeComputerUse() }

// StopComputerUse is the banner's Stop.
func (y *YourComputerAPI) StopComputerUse() { y.stopComputerUse() }

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
			uses := strings.Join(b.Applications, ", ")
			switch {
			case b.ComputerUse != "" && uses != "":
				uses += " and see and use windows"
			case b.ComputerUse != "":
				uses = "see and use windows"
			}
			if err := y.app.askConsent("Let it use your computer again?", who+" is paused and cannot use "+uses+" on your computer. Let it use them again?"); err != nil {
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
