// Package tray is the app's status icon while it runs in the background:
// a menu of Open Agentry, an item for each persistent agent that is running
// (it opens the agent's page) and Quit, and a left click that shows the
// window. It is fyne.io/systray started beside the window's own loop as each
// platform needs (start_*.go).
//
// The library calls back on the platform's UI thread, and a call into the
// window's runtime from there re-enters that loop (on macOS it crashed the
// app), so every callback here runs on a goroutine of its own.
package tray

import (
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
}

var (
	mu      sync.Mutex
	started bool
	menu    Menu
	slots   = map[string]*systray.MenuItem{}
	// pending is what SetRunning was told before the menu was built.
	pending map[string]string
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
	mu.Unlock()
	afterBuild()
}
