package tray

import (
	_ "embed"
	"runtime"

	"fyne.io/systray"
)

//go:embed icons/tray.ico
var iconICO []byte

// start gives the tray an OS thread of its own: the library makes its hidden
// window on the calling thread and pumps that thread's messages, so both must
// be the same thread, and not the window's UI thread.
func start(onReady, onExit func()) {
	go func() {
		runtime.LockOSThread()
		systray.Run(onReady, onExit)
	}()
}

func stop() { systray.Quit() }

func setIcon() { systray.SetIcon(iconICO) }

func afterBuild() {}

// Activate brings the app forward after its window is shown; showing it is
// enough on Windows.
func Activate() {}
