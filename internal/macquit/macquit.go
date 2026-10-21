// Package macquit tells the app, on macOS, that the person asked it to quit.
// There the window's close button, Cmd-Q, the Dock's Quit and the session
// ending at logout all reach the window's OnBeforeClose the same way, and
// only the first should hide the window while agents run in the background.
// Install points the application menu's Quit item and the system's quit
// Apple event (the Dock's Quit, logout, restart and shutdown, osascript's
// quit) at a handler that calls quit first and then quits as before.
// Elsewhere Install does nothing: the window's close is the only close there,
// and a quit comes from the app's own tray or a signal.
package macquit

// Install calls quit whenever the person asks the app to quit, before the
// quit goes ahead. It must be called once the window's menu exists.
func Install(quit func()) { install(quit) }
