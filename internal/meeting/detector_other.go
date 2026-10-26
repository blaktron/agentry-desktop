//go:build !linux && !windows && !darwin

package meeting

// NewDetector is this platform's detector: none here, so calls are not
// noticed and a recording starts only at the person's own Record.
func NewDetector() Detector { return nil }
