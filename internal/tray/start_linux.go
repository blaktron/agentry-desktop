package tray

import (
	_ "embed"

	"fyne.io/systray"
)

//go:embed icons/tray.png
var iconPNG []byte

var end func()

// start runs the tray beside GTK's loop: the item lives on the session bus
// (a StatusNotifierItem and its dbusmenu), so it needs no thread of its own.
func start(onReady, onExit func()) {
	s, e := systray.RunWithExternalLoop(onReady, onExit)
	end = e
	s()
}

func stop() {
	if end != nil {
		end()
	}
}

func setIcon() { systray.SetIcon(iconPNG) }

func afterBuild() {}

// Activate brings the app forward after its window is shown; the window
// manager does it on Linux.
func Activate() {}
