package tray

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
void agentryTrayActivate(void);
*/
import "C"

import _ "embed"

//go:embed icons/tray.png
var iconPNG []byte

//go:embed icons/tray-template.png
var iconTemplate []byte

var recordingPNG = withDot(iconPNG)

var (
	// The icon is a template image, which the menu bar tints for light and
	// dark; the recording icon is the colour one with its red dot, since a
	// template image would lose the red.
	normalLook    = look{image: iconTemplate, template: true}
	recordingLook = look{image: recordingPNG}
)

// Activate brings the app forward after its window is shown.
func Activate() { C.agentryTrayActivate() }
