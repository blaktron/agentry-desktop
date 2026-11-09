// Your computer: the person's approved applications, the link service a run
// uses them through, and what the page shows and does about them. The link
// service serves the link socket in the runner home while the app runs, for
// a run from the CLI on this computer, and binds the app's own runs in
// process. Settings → Your computer lists the person's applications with
// their state here, adds one from the library or by hand, approves each for
// its MCP server and its windows, classes its tools, and shows the runs
// using the computer now with Pause and Stop, and the runs that used it.
// The run setup resolves each application a release names the same way the
// link service does, and lets the person choose one where the release asks.
//
// What the page asks for is a claim: approving an application for its MCP
// server or its windows, and loosening a tool's class, are asked again in a
// native dialog, and so is a paused run's Resume; an exec call is put to the
// person in a native dialog the page cannot answer. Pause, Stop, turning an
// approval off, withholding a tool and Remove only take away.
package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/blaktron/agentry-cli/pkg/configdir"
	"github.com/blaktron/agentry-cli/pkg/runner/desktop"
	"github.com/blaktron/agentry-cli/pkg/runner/engine"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
	"github.com/blaktron/agentry-desktop/internal/library"
	"github.com/blaktron/agentry-desktop/internal/linkservice"
	"github.com/blaktron/agentry-desktop/internal/run"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/tray"
	"github.com/blaktron/agentry-desktop/internal/version"
	"github.com/blaktron/agentry-desktop/internal/yourcomputer"
)

// eventYourComputer tells the page a run bound, changed state or ended.
const eventYourComputer = "yourcomputer:changed"

// YourComputerAPI is Plugins → Your computer and the run setup's cards.
type YourComputerAPI struct {
	app  *App
	run  *RunAPI
	link *linkservice.Service
	// list is the person's applications.
	list *yourcomputer.Store
	// lib is the library of presets, kept current from the repository.
	lib *library.Live
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
	libCache := ""
	if dir, err := configdir.Dir(); err == nil {
		linkservice.SetTokenPath(filepath.Join(dir, "computer-use-portal.json"))
		y.list = yourcomputer.InDir(dir)
		libCache = filepath.Join(dir, "library.yaml")
	} else {
		y.list = yourcomputer.Open("")
	}
	y.lib = library.NewLive(library.LiveOptions{URL: library.URLFor(version.Version), Cache: libCache, OnChange: library.SetCurrent})
	y.link = linkservice.New(linkservice.Options{
		Applications: y.list,
		Choose:       y.askChoose,
		Ask:          y.askExec,
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

// screeningOff says the person had turned a preset's script screening off
// with the switch earlier versions had, which the discovery carries over.
func screeningOff(id string) bool {
	st, err := settings.Load()
	return err == nil && st.IsScreeningOff(id)
}

// discover adds, once, the library's applications found ready on this
// computer, nothing approved.
func (y *YourComputerAPI) discover(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	probe := yourcomputer.Probe{
		LookPath: exec.LookPath,
		Exists:   func(p string) bool { _, err := os.Stat(p); return err == nil },
		Flatpak: func(id string) bool {
			if _, err := exec.LookPath("flatpak"); err != nil {
				return false
			}
			return exec.CommandContext(ctx, "flatpak", "info", id).Run() == nil //nolint:gosec // a library preset's Flatpak id
		},
		Listening: func(ctx context.Context, port int) bool { return linkservice.DialLoopback(ctx, port) == nil },
	}
	var added []string
	_, err := y.list.Update(func(d *yourcomputer.Doc) error {
		added = yourcomputer.Discover(ctx, d, library.Current(), runtime.GOOS, probe, screeningOff)
		return nil
	})
	if err == nil && len(added) > 0 {
		nativeEmit(eventYourComputer)
	}
}

// libraryEvery is how often a running app reads the library again.
const libraryEvery = 6 * time.Hour

// keepLibrary reads the library from the repository now and every
// libraryEvery until ctx ends, telling the page when it moved. The first
// read comes before the discovery, so a new list is checked against the
// newest library.
func (y *YourComputerAPI) keepLibrary(ctx context.Context) {
	y.refreshLibrary(ctx)
	y.discover(ctx)
	t := time.NewTicker(libraryEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			y.refreshLibrary(ctx)
		}
	}
}

// refreshLibrary reads the library once, telling the page when it moved.
func (y *YourComputerAPI) refreshLibrary(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if changed, _ := y.lib.Refresh(ctx); changed {
		nativeEmit(eventYourComputer)
	}
}

// CheckLibrary reads the library from the repository now: Check again.
// A library that could not be read leaves the one in use, and the view
// says why.
func (y *YourComputerAPI) CheckLibrary() *YourComputerView {
	y.refreshLibrary(y.app.ctxOrBackground())
	return y.State()
}

// start serves the link socket in the runner home until the app quits.
func (y *YourComputerAPI) start(ctx context.Context) {
	go y.keepLibrary(ctx)
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
	// Applications are the person's, with their state on this computer.
	Applications []linkservice.AppState `json:"applications"`
	// ListError says why the list could not be read.
	ListError string `json:"listError,omitempty"`
	// Library are the presets offered on this system, in the library's
	// order, each saying whether its id is on the list: Add from the
	// library.
	Library []LibraryPreset `json:"library"`
	// LibraryStatus says which copy of the library is in use and how its
	// last read from the repository went.
	LibraryStatus library.Status `json:"libraryStatus"`
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

// ComputerUseView is Plugins → Your computer's computer use.
type ComputerUseView struct {
	On      bool                `json:"on"`
	Keep    bool                `json:"keep"`
	StopKey string              `json:"stopKey"`
	Support linkservice.Support `json:"support"`
}

// State is Plugins → Your computer.
func (y *YourComputerAPI) State() *YourComputerView {
	ctx, cancel := context.WithTimeout(y.app.ctxOrBackground(), 10*time.Second)
	defer cancel()
	y.mu.Lock()
	socket, socketErr := y.socket, y.socketErr
	y.mu.Unlock()
	apps, err := y.link.States(ctx)
	listErr := ""
	if err != nil {
		listErr = err.Error()
	}
	if apps == nil {
		apps = []linkservice.AppState{}
	}
	return &YourComputerView{
		Applications:  apps,
		ListError:     listErr,
		Library:       y.libraryOffer(apps),
		LibraryStatus: y.lib.Status(),
		Using:         y.link.Runs(),
		Recent:        y.recent(),
		Socket:        socket,
		SocketError:   socketErr,
		Computer:      computerUseView(),
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

// LibraryPreset is a preset as Add from the library offers it: what it is,
// the server it sets up, the steps, the tools that run code, and whether
// an application of its id is on the list already.
type LibraryPreset struct {
	ID          string   `json:"id"`
	Application string   `json:"application"`
	Category    string   `json:"category"`
	Summary     string   `json:"summary"`
	Package     string   `json:"package"`
	Steps       []string `json:"steps"`
	// Exec are the preset's tools that run code inside the application.
	Exec []string `json:"exec"`
	// Asks are the variables the person fills in once it is added.
	Asks []string `json:"asks"`
	// OnList says an application of this id is on the list.
	OnList bool `json:"onList"`
}

// libraryOffer are the presets offered on this system, each saying
// whether its id is on the list.
func (y *YourComputerAPI) libraryOffer(apps []linkservice.AppState) []LibraryPreset {
	out := []LibraryPreset{}
	for _, p := range library.Current().All() {
		if !p.OfferedOn(runtime.GOOS) {
			continue
		}
		lp := LibraryPreset{
			ID: p.ID, Application: p.Application, Category: p.Category, Summary: p.Summary, Package: p.Package.Ref(),
			Steps: append([]string{}, p.Steps...), Exec: []string{}, Asks: []string{},
			OnList: slices.ContainsFunc(apps, func(a linkservice.AppState) bool { return a.ID == p.ID }),
		}
		for _, t := range p.Tools {
			if t.Class == library.ClassExec {
				lp.Exec = append(lp.Exec, t.Name)
			}
		}
		if p.Launch != nil {
			for _, k := range slices.Sorted(maps.Keys(p.Launch.Env)) {
				if p.Launch.Env[k] == "" {
					lp.Asks = append(lp.Asks, k)
				}
			}
		}
		out = append(out, lp)
	}
	return out
}

// changed tells the page and the link service's readers the list moved.
func (y *YourComputerAPI) changed() *YourComputerView {
	nativeEmit(eventYourComputer)
	return y.State()
}

// AddFromLibrary puts the checked presets on the list as applications of
// the person's, nothing approved, in one change: an id that is on the list
// already is left as it is, and an id the library does not offer on this
// computer adds nothing.
func (y *YourComputerAPI) AddFromLibrary(ids []string) (*YourComputerView, error) {
	if len(ids) == 0 {
		return nil, errors.New("check the applications to add")
	}
	lib := library.Current()
	presets := make([]library.Preset, 0, len(ids))
	for _, id := range ids {
		p, ok := lib.Lookup(id)
		if !ok || !p.OfferedOn(runtime.GOOS) {
			return nil, fmt.Errorf("the library has no %s for this computer", id)
		}
		presets = append(presets, p)
	}
	if _, err := y.list.Update(func(d *yourcomputer.Doc) error {
		for _, p := range presets {
			if _, have := d.Lookup(p.ID); have {
				continue
			}
			if err := d.Add(yourcomputer.FromPreset(p, lib.Version(), runtime.GOOS)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return y.changed(), nil
}

// NewApplication is what Add another application gives: a name, an id
// (made from the name when empty), the identifiers of its windows, and its
// MCP server, if any.
type NewApplication struct {
	Name    string   `json:"name"`
	ID      string   `json:"id"`
	Windows []string `json:"windows"`
	// Server is "" for none, "stdio" for a command Agentry Desktop starts,
	// or "loopback-http" for an address on this computer the application
	// serves.
	Server   string            `json:"server"`
	Command  []string          `json:"command"`
	Env      map[string]string `json:"env"`
	Endpoint string            `json:"endpoint"`
	Listener int               `json:"listener"`
}

// application is the person's entry the form describes, its tools as the
// last Test listed them, nothing classed.
func (n NewApplication) application(tools []string) (yourcomputer.Application, error) {
	a := yourcomputer.Application{Name: strings.TrimSpace(n.Name), ID: strings.TrimSpace(n.ID)}
	if a.ID == "" {
		a.ID = yourcomputer.IDFor(a.Name)
	}
	for _, w := range n.Windows {
		if w = strings.TrimSpace(w); w != "" && !slices.Contains(a.Windows, w) {
			a.Windows = append(a.Windows, w)
		}
	}
	srv, err := n.server()
	if err != nil {
		return a, err
	}
	a.Server = srv
	for _, t := range tools {
		a.Tools = append(a.Tools, yourcomputer.Tool{Name: t})
	}
	return a, a.Validate()
}

func (n NewApplication) server() (*yourcomputer.Server, error) {
	switch n.Server {
	case "":
		return nil, nil
	case string(library.AttachmentStdio):
		var cmd []string
		for _, c := range n.Command {
			if c = strings.TrimSpace(c); c != "" {
				cmd = append(cmd, c)
			}
		}
		return &yourcomputer.Server{Attachment: library.AttachmentStdio, Command: cmd, Env: n.Env, Listener: n.Listener}, nil
	case string(library.AttachmentLoopbackHTTP):
		return &yourcomputer.Server{Attachment: library.AttachmentLoopbackHTTP, Endpoint: strings.TrimSpace(n.Endpoint), Listener: n.Listener}, nil
	}
	return nil, fmt.Errorf("a server is started by Agentry Desktop or served by the application, not %q", n.Server)
}

// TestedServer is what Test found: the tools the server listed, or why it
// did not answer.
type TestedServer struct {
	Tools []string `json:"tools"`
	Error string   `json:"error,omitempty"`
}

// TestServer starts the server the form describes, as the person, lists
// its tools and ends it. Starting a command the person typed is asked
// again in a native dialog.
func (y *YourComputerAPI) TestServer(n NewApplication) (*TestedServer, error) {
	srv, err := n.server()
	if err != nil {
		return nil, err
	}
	if srv == nil {
		return nil, fmt.Errorf("the form gives no MCP server to test")
	}
	if err := srv.Validate(); err != nil {
		return nil, err
	}
	if srv.Attachment == library.AttachmentStdio {
		if err := y.app.askConsent("Start this command to test it?", "Agentry Desktop will start this command as you, on this computer, to list the tools its MCP server offers, and then end it:\n\n"+srv.Launch()+"\n\nStart it?"); err != nil {
			return nil, err
		}
	}
	ctx, cancel := context.WithTimeout(y.app.ctxOrBackground(), 60*time.Second)
	defer cancel()
	tools, err := y.link.ListServerTools(ctx, *srv)
	if err != nil {
		return &TestedServer{Tools: []string{}, Error: err.Error()}, nil
	}
	return &TestedServer{Tools: tools}, nil
}

// AddApplication puts an application the person describes on the list,
// nothing approved, its tools those the last Test listed, none classed.
func (y *YourComputerAPI) AddApplication(n NewApplication, tools []string) (*YourComputerView, error) {
	a, err := n.application(tools)
	if err != nil {
		return nil, err
	}
	if _, err := y.list.Update(func(d *yourcomputer.Doc) error { return d.Add(a) }); err != nil {
		return nil, err
	}
	return y.changed(), nil
}

// RunningWindows are the windows open on this display, each with the
// identifiers Add another application can take, where the window system
// lets Agentry Desktop read them.
func (y *YourComputerAPI) RunningWindows() ([]linkservice.RunningWindow, error) {
	ctx, cancel := context.WithTimeout(y.app.ctxOrBackground(), 10*time.Second)
	defer cancel()
	return linkservice.RunningWindows(ctx)
}

// SetApproval turns one of an application's approvals on or off: "mcp"
// lets agents use its MCP server, "computer" see and use its windows.
// Turning one on is asked again in a native dialog; turning one off asks
// nothing.
func (y *YourComputerAPI) SetApproval(id, which string, on bool) (*YourComputerView, error) {
	d, err := y.list.Load()
	if err != nil {
		return nil, err
	}
	a, ok := d.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("no application %s is on your list", id)
	}
	if on {
		var title, msg string
		switch which {
		case "mcp":
			if a.Server == nil {
				return nil, fmt.Errorf("%s has no MCP server set up", a.Name)
			}
			title = "Let agents use " + a.Name + "'s MCP server?"
			msg = fmt.Sprintf("A QuickAgent or agent that names %s (as %s) will be able to call the tools of its MCP server through Agentry Desktop, which starts the server as you, with your files and your network. A tool nobody has classed, and every tool that runs code, asks you before each call. %s.\n\nLet agents use %s's MCP server?", a.Name, a.ID, capitalize(a.From.Words()), a.Name)
		case "computer":
			if len(a.Windows) == 0 {
				return nil, fmt.Errorf("the identifiers of %s's windows are not known yet: give them first", a.Name)
			}
			title = "Let agents see and use " + a.Name + "'s windows?"
			msg = fmt.Sprintf("While computer use is on, a QuickAgent or agent that names %s (as %s) will be able to take screenshots of its windows and click and type in them. Agentry Desktop shows when it does, with Pause and Stop.\n\nLet agents see and use %s's windows?", a.Name, a.ID, a.Name)
		default:
			return nil, fmt.Errorf("an approval is mcp or computer, not %q", which)
		}
		if err := y.app.askConsent(title, msg); err != nil {
			return nil, err
		}
	}
	if _, err := y.list.Update(func(d *yourcomputer.Doc) error {
		return d.Change(id, func(a *yourcomputer.Application) error {
			switch which {
			case "mcp":
				a.Approve.MCP = on
			case "computer":
				a.Approve.Computer = on
			default:
				return fmt.Errorf("an approval is mcp or computer, not %q", which)
			}
			return nil
		})
	}); err != nil {
		return nil, err
	}
	return y.changed(), nil
}

// SetToolClass classes one of an application's tools read, act or exec.
// Loosening a class (fewer questions before its calls) is asked again in a
// native dialog; tightening asks nothing.
func (y *YourComputerAPI) SetToolClass(id, tool, class string) (*YourComputerView, error) {
	c := library.Class(class)
	if !slices.Contains(library.Classes(), c) {
		return nil, fmt.Errorf("a class is read, act or exec, not %q", class)
	}
	d, err := y.list.Load()
	if err != nil {
		return nil, err
	}
	a, ok := d.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("no application %s is on your list", id)
	}
	from := a.ClassOf(tool)
	if yourcomputer.Loosens(from, c) {
		words := map[library.Class]string{library.ClassRead: "reads the application's state", library.ClassAct: "changes the application or works inside it"}[c]
		msg := fmt.Sprintf("%s's tool %s is %s now, so Agentry Desktop asks you before each call. Classed %s (it %s), its calls run without asking.\n\nClass %s as %s?", a.Name, tool, from, c, words, tool, c)
		if from == library.ClassAct {
			msg = fmt.Sprintf("%s's tool %s is classed act. Classed read, it is shown as only reading %s's state.\n\nClass %s as read?", a.Name, tool, a.Name, tool)
		}
		if err := y.app.askConsent("Ask less before "+tool+"?", msg); err != nil {
			return nil, err
		}
	}
	if _, err := y.list.Update(func(d *yourcomputer.Doc) error {
		return d.Change(id, func(a *yourcomputer.Application) error {
			for i := range a.Tools {
				if a.Tools[i].Name == tool {
					a.Tools[i].Class = c
					return nil
				}
			}
			a.Tools = append(a.Tools, yourcomputer.Tool{Name: tool, Class: c})
			return nil
		})
	}); err != nil {
		return nil, err
	}
	return y.changed(), nil
}

// SetToolWithheld withholds a tool from every run, or gives it back.
// Giving it back asks again in a native dialog.
func (y *YourComputerAPI) SetToolWithheld(id, tool string, withheld bool) (*YourComputerView, error) {
	if !withheld {
		if err := y.app.askConsent("Give "+tool+" back to agents?", "Agents that use this application will be able to call its tool "+tool+" again, as its class says.\n\nGive it back?"); err != nil {
			return nil, err
		}
	}
	if _, err := y.list.Update(func(d *yourcomputer.Doc) error {
		return d.Change(id, func(a *yourcomputer.Application) error {
			for i := range a.Tools {
				if a.Tools[i].Name == tool {
					a.Tools[i].Withheld = withheld
					return nil
				}
			}
			a.Tools = append(a.Tools, yourcomputer.Tool{Name: tool, Withheld: withheld})
			return nil
		})
	}); err != nil {
		return nil, err
	}
	return y.changed(), nil
}

// EditApplication changes an application's name, windows and server. A
// preset's application whose server changes is no longer the library's as
// it was, and the card says so. A changed server is asked again in a
// native dialog while it is approved, since agents use it as it is now.
func (y *YourComputerAPI) EditApplication(id string, n NewApplication) (*YourComputerView, error) {
	d, err := y.list.Load()
	if err != nil {
		return nil, err
	}
	cur, ok := d.Lookup(id)
	if !ok {
		return nil, fmt.Errorf("no application %s is on your list", id)
	}
	srv, err := n.server()
	if err != nil {
		return nil, err
	}
	if srv != nil && cur.Server != nil {
		srv.Package = cur.Server.Package
	}
	if cur.Approve.MCP && srv != nil && (cur.Server == nil || srv.Launch() != cur.Server.Launch() || srv.Endpoint != cur.Server.Endpoint) {
		what := srv.Launch()
		if srv.Attachment == library.AttachmentLoopbackHTTP {
			what = srv.Endpoint
		}
		if err := y.app.askConsent("Change "+cur.Name+"'s MCP server?", "Agents may use "+cur.Name+"'s MCP server, and will use it as it is after this change:\n\n"+what+"\n\nChange it?"); err != nil {
			return nil, err
		}
	}
	if _, err := y.list.Update(func(d *yourcomputer.Doc) error {
		return d.Change(id, func(a *yourcomputer.Application) error {
			if name := strings.TrimSpace(n.Name); name != "" {
				a.Name = name
			}
			var windows []string
			for _, w := range n.Windows {
				if w = strings.TrimSpace(w); w != "" && !slices.Contains(windows, w) {
					windows = append(windows, w)
				}
			}
			a.Windows = windows
			if len(windows) == 0 {
				a.Approve.Computer = false
			}
			a.SetServer(srv)
			if srv == nil {
				a.Approve.MCP = false
			}
			return nil
		})
	}); err != nil {
		return nil, err
	}
	return y.changed(), nil
}

// RemoveApplication takes an application off the list, and the choices
// that named it. Nothing brings a removed library application back but
// adding it again.
func (y *YourComputerAPI) RemoveApplication(id string) (*YourComputerView, error) {
	if _, err := y.list.Update(func(d *yourcomputer.Doc) error { return d.Remove(id) }); err != nil {
		return nil, err
	}
	return y.changed(), nil
}

// RunApplication is one application a release names, as the run setup
// shows it: resolved to one of the person's applications, or not yet, with
// the step to take or the applications to choose from.
type RunApplication struct {
	// Entry is the id the release names, "" for an application the person
	// chooses.
	Entry string `json:"entry"`
	// Use is what the run uses of it: mcp, computer or both.
	Use string `json:"use"`
	// Resolved is the person's application it is, with its state.
	Resolved *linkservice.AppState `json:"resolved,omitempty"`
	// Chosen says the person chose it for this QuickAgent.
	Chosen bool `json:"chosen"`
	// Preset names the library's application when the id is a preset not
	// set up.
	Preset string `json:"preset,omitempty"`
	// Candidates are the person's applications approved for the use, to
	// choose from.
	Candidates []linkservice.Candidate `json:"candidates"`
	// Ready says a run can use it now; Missing is the step to take when
	// it cannot.
	Ready   bool   `json:"ready"`
	Missing string `json:"missing,omitempty"`
}

// RunApplications resolves each application a release names as the link
// service will at the run's start: by its id on the list, else by the
// person's choice for this QuickAgent.
func (y *YourComputerAPI) RunApplications(ref string, entries []run.DesktopEntry) []RunApplication {
	ctx, cancel := context.WithTimeout(y.app.ctxOrBackground(), 10*time.Second)
	defer cancel()
	out := make([]RunApplication, 0, len(entries))
	d, err := y.list.Load()
	if err != nil {
		for _, e := range entries {
			out = append(out, RunApplication{Entry: e.ID, Use: e.Use, Candidates: []linkservice.Candidate{}, Missing: err.Error()})
		}
		return out
	}
	computerOn := computerUseView().On
	for _, e := range entries {
		ra := RunApplication{Entry: e.ID, Use: e.Use, Candidates: []linkservice.Candidate{}}
		mcp, windows := e.Use != run.UseComputer, e.Use != run.UseMCP
		approved := func(a yourcomputer.Application) bool {
			return (!mcp || a.UsesMCP()) && (!windows || a.UsesWindows())
		}
		var a yourcomputer.Application
		found := false
		if e.ID != "" {
			a, found = d.Lookup(e.ID)
		}
		key := e.ID
		if e.ID == "" {
			key = "chosen"
		}
		if !found {
			if c, ok := d.Choice(ref, key); ok && approved(c) {
				a, found, ra.Chosen = c, true, true
			}
		}
		for _, c := range d.Applications {
			if approved(c) {
				ra.Candidates = append(ra.Candidates, linkservice.Candidate{ID: c.ID, Name: c.Name})
			}
		}
		if !found {
			p, isPreset := library.Current().Lookup(e.ID)
			switch {
			case e.ID != "" && isPreset && p.OfferedOn(runtime.GOOS):
				ra.Preset = p.Application
				ra.Missing = p.Application + " is not set up in Agentry Desktop: add it from the library in Settings → Your computer and approve it, or choose which of your applications it is."
			case e.ID != "":
				ra.Missing = "This QuickAgent uses an application called " + e.ID + ": choose which of yours it is, or add it in Settings → Your computer."
			default:
				ra.Missing = "This QuickAgent uses an application you choose: choose one of yours."
			}
			if len(ra.Candidates) == 0 {
				ra.Missing += " None of your applications is approved for what it uses yet."
			}
			out = append(out, ra)
			continue
		}
		st := y.link.Check(ctx, a)
		ra.Resolved = &st
		switch {
		case mcp && a.Server == nil:
			ra.Missing = a.Name + " has no MCP server set up in Agentry Desktop: add its server in Settings → Your computer."
		case mcp && !a.Approve.MCP:
			ra.Missing = "Agents may not use " + a.Name + "'s MCP server: turn on \"Agents may use its MCP server\" for it in Settings → Your computer."
		case windows && len(a.Windows) == 0:
			ra.Missing = "Agentry Desktop does not know " + a.Name + "'s windows: give their identifiers in Settings → Your computer."
		case windows && !a.Approve.Computer:
			ra.Missing = "Agents may not see and use " + a.Name + "'s windows: turn on \"Agents may see and use its windows\" for it in Settings → Your computer."
		case windows && !computerOn:
			ra.Missing = "Computer use is off on this computer: turn it on in Settings → Your computer."
		case mcp && !st.Ready:
			ra.Missing = st.Sentence()
		default:
			ra.Ready = true
		}
		out = append(out, ra)
	}
	return out
}

// ChooseApplication remembers which of the person's applications a
// QuickAgent's entry is: an id the list does not have, or "" for an
// application the person chooses.
func (y *YourComputerAPI) ChooseApplication(ref, entry, id string) error {
	if ref == "" {
		return fmt.Errorf("a choice is for a QuickAgent")
	}
	key := entry
	if key == "" {
		key = "chosen"
	}
	_, err := y.list.Update(func(d *yourcomputer.Doc) error {
		if _, ok := d.Lookup(id); !ok {
			return fmt.Errorf("no application %s is on your list", id)
		}
		if entry != "" {
			if _, onList := d.Lookup(entry); onList {
				return fmt.Errorf("%s is on your list already: it is used by its id", entry)
			}
		}
		d.Choose(ref, key, id)
		return nil
	})
	if err == nil {
		nativeEmit(eventYourComputer)
	}
	return err
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

// maxChoiceButtons bounds the native chooser's buttons.
const maxChoiceButtons = 4

// chooseNone is the chooser's refusal.
const chooseNone = "None of these"

// askChoose asks which of the person's applications a release's entry is.
func (y *YourComputerAPI) askChoose(ctx context.Context, q linkservice.ChooseQuestion) (string, error) {
	if y.app.chooseApp != nil {
		return y.app.chooseApp(ctx, q)
	}
	return nativeChoose(ctx, q)
}

// chooseDialogText is the chooser's title and message.
func chooseDialogText(q linkservice.ChooseQuestion) (title, message string) {
	who := cmp.Or(q.Who, "A QuickAgent")
	if q.Entry == "" {
		title = "Which application should it use?"
		message = fmt.Sprintf("%s uses an application you choose, through %s. Which of yours should it use? Agentry Desktop remembers your choice for this QuickAgent.", who, q.Use)
	} else {
		title = "Which application is " + q.Entry + "?"
		message = fmt.Sprintf("%s uses an application called %s, through %s, and none of yours has that id. Which of yours is it? Agentry Desktop remembers your choice for this QuickAgent.", who, q.Entry, q.Use)
	}
	return title, message
}
