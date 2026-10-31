package tray

import _ "embed"

//go:embed icons/tray.png
var iconPNG []byte

var recordingPNG = withDot(iconPNG)

var (
	normalLook    = look{image: iconPNG}
	recordingLook = look{image: recordingPNG}
)

// Activate brings the app forward after its window is shown; the window
// manager does it on Linux.
func Activate() {}
