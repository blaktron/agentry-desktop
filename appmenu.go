package main

import "github.com/wailsapp/wails/v3/pkg/application"

// appMenu is the menu bar's content. On macOS a webview's text fields take
// their editing keys (Cmd+X, Cmd+C, Cmd+V, Cmd+A, Cmd+Z) from the Edit menu,
// so the app keeps one there; without a menu of its own the framework's
// default would also offer Reload, which drops what a page holds, and a Help
// item that opens the framework's website in the app's window. The View menu
// keeps zoom and full screen. Windows and Linux draw no menu bar for the
// window, and their webviews handle the same keys themselves.
func appMenu() *application.Menu {
	m := application.NewMenu()
	m.AddRole(application.AppMenu) // the application's own menu, on macOS only
	m.AddRole(application.EditMenu)
	view := m.AddSubmenu("View")
	view.AddRole(application.ResetZoom)
	view.AddRole(application.ZoomIn)
	view.AddRole(application.ZoomOut)
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)
	m.AddRole(application.WindowMenu)
	return m
}
