//go:build windows

package stopkey

// Name is the stop key as the person is told it. Windows keeps
// Ctrl+Shift+Escape for Task Manager, so the stop key is another.
const Name = "Ctrl+Shift+F12"
