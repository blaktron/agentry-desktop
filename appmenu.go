package main

import "github.com/wailsapp/wails/v3/pkg/application"

// appMenu is the menu bar's content. On macOS a webview's text fields take
// their editing keys (Cmd+X, Cmd+C, Cmd+V, Cmd+A, Cmd+Z) from the Edit menu,
// so the app keeps one there; without a menu of its own the framework's
// default would also offer Reload, which drops what a page holds, and a Help
// item that opens the framework's website in the app's window. Windows and
// Linux draw no menu bar for the window, and their webviews handle the same
// keys themselves. It needs the running application, which names the
// application's own menu. zoom steps the text size: 1 larger, -1 smaller,
// 0 back to the default.
func appMenu(zoom func(step int)) *application.Menu {
	m := application.NewMenu()
	m.AddRole(application.AppMenu) // the application's own menu, on macOS only
	addWindowMenus(m, zoom)
	return m
}

// addWindowMenus adds the menus that follow the application's own: Edit,
// View with the text size and full screen, and Window. The text size items
// step the size Settings → Home → Accessibility keeps, rather than the
// webview's own zoom, so the menu, the keys and Settings agree.
func addWindowMenus(m *application.Menu, zoom func(step int)) {
	m.AddRole(application.EditMenu)
	view := m.AddSubmenu("View")
	view.Add("Actual Size").SetAccelerator("CmdOrCtrl+0").OnClick(func(*application.Context) { zoom(0) })
	view.Add("Zoom In").SetAccelerator("CmdOrCtrl+plus").OnClick(func(*application.Context) { zoom(1) })
	view.Add("Zoom Out").SetAccelerator("CmdOrCtrl+-").OnClick(func(*application.Context) { zoom(-1) })
	view.AddSeparator()
	view.AddRole(application.ToggleFullscreen)
	m.AddRole(application.WindowMenu)
}
