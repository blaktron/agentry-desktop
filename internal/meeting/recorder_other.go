//go:build !linux

package meeting

import "context"

// NewRecorder is this platform's recorder: none here.
func NewRecorder() Recorder { return unsupported{} }

type unsupported struct{}

func (unsupported) Record(context.Context, Call, Tracks) (Capture, error) {
	return nil, ErrUnsupported
}

// NewDetector is this platform's detector: none here.
func NewDetector() Detector { return nil }
