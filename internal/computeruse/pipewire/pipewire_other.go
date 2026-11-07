//go:build !(linux && cgo)

// Package pipewire reads the video of one PipeWire node, such as a window
// stream the ScreenCast portal shares, and returns its newest frame.
package pipewire

import (
	"context"
	"errors"
	"image"
)

var errUnavailable = errors.New("this build cannot capture a window stream")

// Stream is a connection to one PipeWire video node.
type Stream struct{}

// Open always fails in a build without Linux and cgo.
func Open(fd int, node uint32) (*Stream, error) { return nil, errUnavailable }

// Frame always fails in a build without Linux and cgo.
func (s *Stream) Frame(ctx context.Context, fresh bool) (*image.RGBA, error) {
	return nil, errUnavailable
}

// Size is always zero in a build without Linux and cgo.
func (s *Stream) Size() image.Point { return image.Point{} }

// Close does nothing in a build without Linux and cgo.
func (s *Stream) Close() error { return nil }

// Available reports whether this build can capture a window stream.
func Available() bool { return false }
