//go:build !linux

package meeting

import "context"

// NewRecorder is this platform's recorder; Windows and macOS come later.
func NewRecorder() Recorder { return unsupported{} }

type unsupported struct{}

func (unsupported) Record(context.Context, Call, Tracks) (Capture, error) {
	return nil, ErrUnsupported
}
