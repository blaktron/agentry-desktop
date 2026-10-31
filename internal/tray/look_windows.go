package tray

import (
	"bytes"
	_ "embed"
	"image/png"
)

//go:embed icons/tray.png
var iconPNG []byte

//go:embed icons/tray.ico
var iconICO []byte

var recordingICO = pngICO(withDot(iconPNG))

var (
	normalLook    = look{image: iconICO}
	recordingLook = look{image: recordingICO}
)

// Activate brings the app forward after its window is shown; showing it is
// enough on Windows.
func Activate() {}

// pngICO wraps one PNG image in an icon file, which Windows reads from
// Vista on: a six-byte header and one sixteen-byte entry before the image.
func pngICO(img []byte) []byte {
	w, h := 0, 0
	if cfg, err := png.DecodeConfig(bytes.NewReader(img)); err == nil {
		w, h = cfg.Width, cfg.Height
	}
	dim := func(n int) byte {
		if n <= 0 || n >= 256 {
			return 0 // 256 or more
		}
		return byte(n)
	}
	n := len(img)
	out := []byte{0, 0, 1, 0, 1, 0, dim(w), dim(h), 0, 0, 1, 0, 32, 0,
		byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24), 22, 0, 0, 0}
	return append(out, img...)
}
