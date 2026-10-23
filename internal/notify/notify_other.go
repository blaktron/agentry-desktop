//go:build !linux && !darwin && !windows

package notify

// Init readies nothing: this platform has no facility here.
func Init() error { return nil }

func show(Notice) error { return ErrUnsupported }
