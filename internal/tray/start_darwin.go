package tray

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
void agentryTrayStartOnMain(void);
void agentryTrayActivate(void);
void agentryTrayFixTarget(void);
*/
import "C"

import (
	_ "embed"

	"fyne.io/systray"
)

//go:embed icons/tray.png
var iconPNG []byte

//go:embed icons/tray-template.png
var iconTemplate []byte

var (
	begin func()
	end   func()
)

// start makes the status item on the main thread, which AppKit requires;
// the window's OnStartup runs on another goroutine. With an external loop
// the library leaves the window's application delegate alone, so the app's
// own quit and reopen handling stay the window's.
func start(onReady, onExit func()) {
	begin, end = systray.RunWithExternalLoop(onReady, onExit)
	C.agentryTrayStartOnMain()
}

//export goAgentryTrayStart
func goAgentryTrayStart() { begin() }

func stop() {
	if end != nil {
		end()
	}
}

// setIcon is a template image, which the menu bar tints for light and dark.
func setIcon() { systray.SetTemplateIcon(iconTemplate, iconPNG) }

// afterBuild points the status button's action at the library's delegate:
// under an external loop it has no target, so a keyboard or VoiceOver press
// would do nothing.
func afterBuild() { C.agentryTrayFixTarget() }

// Activate brings the app forward: a window shown from the status item of an
// app that is not active stays behind the active app's windows.
func Activate() { C.agentryTrayActivate() }
