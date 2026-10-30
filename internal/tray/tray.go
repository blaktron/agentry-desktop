// Package tray is the app's status icon while it runs in the background:
// a menu of Open Agentry, a meeting being recorded (what it records, Pause
// or Resume, and Stop) or, while none is, Record a meeting in the room, an
// item for each persistent agent that is running
// (it opens the agent's page) and Quit, and a left click that shows the
// window. While a meeting is recorded the icon carries a red dot. It is the
// window library's system tray, which each platform draws its own way.
//
// The icon's click calls back on the platform's UI thread, and a call into
// the window from there re-enters that loop (on macOS it crashed the app), so
// that callback runs its work on a goroutine of its own; a menu item's
// callback already does.
package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
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
	// OnMeetingRecordRoom starts recording a meeting in the room; its item
	// shows while Meeting.CanRecordRoom.
	OnMeetingRecordRoom func()
}

// Meeting is the meeting recording the tray shows; the zero value is none.
type Meeting struct {
	Recording bool
	Paused    bool
	// What is what is recorded, as the menu says it: "Zoom", or "everything
	// this computer plays".
	What string
	// CanRecordRoom: nothing is recorded, and a meeting in the room can be.
	CanRecordRoom bool
}

// meetingItems are the menu's items for a meeting being recorded, and the
// one that records a meeting in the room.
type meetingItems struct {
	status, pause, stop, room *application.MenuItem
}

// look is how the icon is drawn: its image, and whether the menu bar tints it
// (a template image, which macOS draws in the colour of the bar).
type look struct {
	image    []byte
	template bool
}

var (
	mu      sync.Mutex
	started bool
	menu    Menu
	// icon is the status item, and drawn the look it shows.
	icon  *application.SystemTray
	drawn look
	slots = map[string]*application.MenuItem{}
	// running is what SetRunning was last told; it is drawn when the menu is
	// built, and again when the icon is made anew.
	running map[string]string
	// meeting is what SetMeeting was last told, and meetingMenu its items
	// once the menu is built.
	meeting     Meeting
	meetingMenu *meetingItems
)

// Start shows the icon. A second call does nothing.
func Start(m Menu) {
	mu.Lock()
	defer mu.Unlock()
	if started {
		return
	}
	started, menu = true, m
	if !build() {
		started = false
	}
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
	defer mu.Unlock()
	if icon != nil {
		icon.Destroy()
		icon = nil
	}
}

// SetRunning shows an item for each agent running, labelled as given (name to
// label), and hides the others.
func SetRunning(r map[string]string) {
	mu.Lock()
	defer mu.Unlock()
	running = r
	if len(slots) > 0 {
		applyRunning()
	}
}

// applyRunning shows and labels the slots; mu is held.
func applyRunning() {
	for name, item := range slots {
		if label, ok := running[name]; ok {
			item.SetLabel(label)
			item.SetHidden(false)
		} else {
			item.SetHidden(true)
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
	it.room.SetHidden(!meeting.CanRecordRoom || meeting.Recording)
	if !meeting.Recording {
		it.status.SetHidden(true)
		it.pause.SetHidden(true)
		it.stop.SetHidden(true)
		setLook(normalLook)
		icon.SetTooltip(menu.Tooltip)
		return
	}
	status, pause := MeetingLabels(meeting)
	it.status.SetLabel(status)
	it.pause.SetLabel(pause)
	it.status.SetHidden(false)
	it.pause.SetHidden(false)
	it.stop.SetHidden(false)
	setLook(recordingLook)
	icon.SetTooltip(menu.Tooltip + ": " + status)
}

// setLook puts the icon's image on the status item. A template image the
// status item has once shown cannot be turned back into a coloured one, so
// going from one to the other makes the status item anew.
func setLook(l look) {
	if l.template == drawn.template && bytes.Equal(l.image, drawn.image) {
		return
	}
	if drawn.template && !l.template {
		icon.Destroy()
		icon = nil
		build()
		return
	}
	paint(l)
}

// paint sets l on the status item; mu is held.
func paint(l look) {
	if l.template {
		icon.SetTemplateIcon(l.image)
	} else {
		icon.SetIcon(l.image)
	}
	drawn = l
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

// build makes the status item with its menu and the state the package holds;
// mu is held. It reports whether the application was there to hold it.
func build() bool {
	wails := application.Get()
	if wails == nil {
		return false
	}
	m := menu
	icon = wails.SystemTray.New()
	mn := wails.Menu.New()
	// The agents' items follow Open Agentry directly, and one separator
	// stands before Quit, so a menu with no agent running has no empty
	// section.
	mn.Add("Open Agentry").SetTooltip("Show the Agentry window").OnClick(func(*application.Context) {
		if m.OnOpen != nil {
			m.OnOpen()
		}
	})
	mi := &meetingItems{
		status: mn.Add("Recording").SetTooltip("A meeting is being recorded"),
		pause:  mn.Add("Pause recording").SetTooltip("Pause or resume the meeting's recording"),
		stop:   mn.Add("Stop and transcribe").SetTooltip("Stop recording the meeting and transcribe it"),
		room:   mn.Add("Record a meeting in the room").SetTooltip("Record a meeting in this room through the microphone"),
	}
	mi.status.SetEnabled(false)
	for _, it := range []*application.MenuItem{mi.status, mi.pause, mi.stop, mi.room} {
		it.SetHidden(true)
	}
	mi.room.OnClick(func(*application.Context) {
		if m.OnMeetingRecordRoom != nil {
			m.OnMeetingRecordRoom()
		}
	})
	mi.pause.OnClick(func(*application.Context) {
		mu.Lock()
		paused := meeting.Paused
		mu.Unlock()
		if f := m.OnMeetingPause; !paused && f != nil {
			f()
		} else if f := m.OnMeetingResume; paused && f != nil {
			f()
		}
	})
	mi.stop.OnClick(func(*application.Context) {
		if m.OnMeetingStop != nil {
			m.OnMeetingStop()
		}
	})
	items := map[string]*application.MenuItem{}
	for _, name := range m.Agents {
		name := name
		it := mn.Add(name).SetTooltip("Open this agent's page")
		it.SetHidden(true)
		it.OnClick(func(*application.Context) {
			if m.OnAgent != nil {
				m.OnAgent(name)
			}
		})
		items[name] = it
	}
	mn.AddSeparator()
	mn.Add("Quit").SetTooltip("Stop every agent and quit Agentry").OnClick(func(*application.Context) {
		if m.OnQuit != nil {
			m.OnQuit()
		}
	})
	slots, meetingMenu = items, mi
	drawn = look{}
	icon.SetMenu(mn)
	icon.OnClick(async(m.OnOpen))
	// A status item made while a meeting is recorded starts with the
	// recording icon, so applyMeeting below has nothing to swap.
	if meeting.Recording {
		paint(recordingLook)
	} else {
		paint(normalLook)
	}
	icon.SetTooltip(m.Tooltip)
	applyRunning()
	applyMeeting()
	return true
}
