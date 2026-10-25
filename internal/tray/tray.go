// Package tray is the app's status icon while it runs in the background:
// a menu of Open Agentry, a meeting being recorded (what it records, Pause
// or Resume, and Stop), an item for each persistent agent that is running
// (it opens the agent's page) and Quit, and a left click that shows the
// window. While a meeting is recorded the icon carries a red dot. It is fyne.io/systray started beside the window's own loop as each
// platform needs (start_*.go).
//
// The library calls back on the platform's UI thread, and a call into the
// window's runtime from there re-enters that loop (on macOS it crashed the
// app), so every callback here runs on a goroutine of its own.
package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"

	"fyne.io/systray"
)

// Menu is what the tray offers.
type Menu struct {
	Tooltip string
	// OnOpen shows the window: the left click and Open Agentry.
	OnOpen func()
	// OnAgent opens the page of the agent named.
	OnAgent func(name string)
	// OnQuit quits the app.
	OnQuit func()
	// Agents are the agents an item may stand for, by name, in the menu's
	// order; an item shows only while its agent runs (SetRunning).
	Agents []string
	// OnMeetingPause, OnMeetingResume and OnMeetingStop act on the meeting
	// being recorded (SetMeeting).
	OnMeetingPause  func()
	OnMeetingResume func()
	OnMeetingStop   func()
}

// Meeting is the meeting recording the tray shows; the zero value is none.
type Meeting struct {
	Recording bool
	Paused    bool
	// What is what is recorded, as the menu says it: "Zoom", or "everything
	// this computer plays".
	What string
}

// meetingItems are the menu's items for a meeting being recorded.
type meetingItems struct {
	status, pause, stop *systray.MenuItem
}

var (
	mu      sync.Mutex
	started bool
	menu    Menu
	slots   = map[string]*systray.MenuItem{}
	// pending is what SetRunning was told before the menu was built.
	pending map[string]string
	// meeting is what SetMeeting was last told, and meetingMenu its items
	// once the menu is built.
	meeting     Meeting
	meetingMenu *meetingItems
)

// Start shows the icon. A second call does nothing.
func Start(m Menu) {
	mu.Lock()
	if started {
		mu.Unlock()
		return
	}
	started, menu = true, m
	mu.Unlock()
	start(build, func() {})
}

// Started reports whether the icon is up.
func Started() bool {
	mu.Lock()
	defer mu.Unlock()
	return started
}

// Stop takes the icon away, as the app quits.
func Stop() {
	mu.Lock()
	was := started
	mu.Unlock()
	if was {
		stop()
	}
}

// SetRunning shows an item for each agent running, labelled as given (name to
// label), and hides the others.
func SetRunning(running map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	if len(slots) == 0 {
		pending = running
		return
	}
	apply(running)
}

// apply shows and labels the slots; mu is held.
func apply(running map[string]string) {
	for name, item := range slots {
		if label, ok := running[name]; ok {
			item.SetTitle(label)
			item.Show()
		} else {
			item.Hide()
		}
	}
}

// SetMeeting shows the meeting being recorded, or none, in the menu and on
// the icon.
func SetMeeting(m Meeting) {
	mu.Lock()
	defer mu.Unlock()
	meeting = m
	if meetingMenu != nil {
		applyMeeting()
	}
}

// MeetingLabels are the menu's words for m: its status line and its pause
// item's title.
func MeetingLabels(m Meeting) (status, pause string) {
	what := m.What
	if what == "" {
		what = "a meeting"
	}
	if m.Paused {
		return "Recording paused: " + what, "Resume recording"
	}
	return "Recording " + what, "Pause recording"
}

// applyMeeting draws meeting; mu is held.
func applyMeeting() {
	it := meetingMenu
	if !meeting.Recording {
		it.status.Hide()
		it.pause.Hide()
		it.stop.Hide()
		setIcon()
		systray.SetTooltip(menu.Tooltip)
		return
	}
	status, pause := MeetingLabels(meeting)
	it.status.SetTitle(status)
	it.pause.SetTitle(pause)
	it.status.Show()
	it.pause.Show()
	it.stop.Show()
	setRecordingIcon()
	systray.SetTooltip(menu.Tooltip + ": " + status)
}

// withDot is a PNG icon with a red dot in its lower right corner, the
// recording mark, or the icon as it was when it cannot be read.
func withDot(icon []byte) []byte {
	src, err := png.Decode(bytes.NewReader(icon))
	if err != nil {
		return icon
	}
	b := src.Bounds()
	out := image.NewNRGBA(b)
	draw.Draw(out, b, src, b.Min, draw.Src)
	r := b.Dx() * 3 / 16
	cx, cy := b.Max.X-r-1, b.Max.Y-r-1
	red := color.NRGBA{R: 0xe0, G: 0x24, B: 0x24, A: 0xff}
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r {
				out.SetNRGBA(x, y, red)
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return icon
	}
	return buf.Bytes()
}

// async runs f on a goroutine of its own, never on the UI thread the
// library calls back on.
func async(f func()) func() {
	return func() {
		if f != nil {
			go f()
		}
	}
}

// build is the library's onReady: the icon, the left click and the menu.
func build() {
	setIcon()
	mu.Lock()
	m := menu
	mu.Unlock()
	systray.SetTooltip(m.Tooltip)
	systray.SetOnTapped(async(m.OnOpen))
	// The agents' items follow Open Agentry directly, and one separator
	// stands before Quit, so a menu with no agent running has no empty
	// section.
	open := systray.AddMenuItem("Open Agentry", "Show the Agentry window")
	mi := &meetingItems{
		status: systray.AddMenuItem("Recording", "A meeting is being recorded"),
		pause:  systray.AddMenuItem("Pause recording", "Pause or resume the meeting's recording"),
		stop:   systray.AddMenuItem("Stop and transcribe", "Stop recording the meeting and transcribe it"),
	}
	mi.status.Disable()
	mi.status.Hide()
	mi.pause.Hide()
	mi.stop.Hide()
	go func() {
		for range mi.pause.ClickedCh {
			mu.Lock()
			paused := meeting.Paused
			mu.Unlock()
			if f := m.OnMeetingPause; !paused && f != nil {
				f()
			} else if f := m.OnMeetingResume; paused && f != nil {
				f()
			}
		}
	}()
	go func() {
		for range mi.stop.ClickedCh {
			if m.OnMeetingStop != nil {
				m.OnMeetingStop()
			}
		}
	}()
	items := map[string]*systray.MenuItem{}
	for _, name := range m.Agents {
		it := systray.AddMenuItem(name, "Open this agent's page")
		it.Hide()
		items[name] = it
		name := name
		go func() {
			for range it.ClickedCh {
				if m.OnAgent != nil {
					m.OnAgent(name)
				}
			}
		}()
	}
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "Stop every agent and quit Agentry")
	go func() {
		for range open.ClickedCh {
			if m.OnOpen != nil {
				m.OnOpen()
			}
		}
	}()
	go func() {
		for range quit.ClickedCh {
			if m.OnQuit != nil {
				m.OnQuit()
			}
		}
	}()
	mu.Lock()
	slots = items
	if pending != nil {
		apply(pending)
		pending = nil
	}
	meetingMenu = mi
	applyMeeting()
	mu.Unlock()
	afterBuild()
}
