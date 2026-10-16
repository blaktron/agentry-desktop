//go:build windows

package run

// terminateProcess: Windows has no gentle signal and no podman API service
// of a run's own; nothing is sent, and endProcess returns.
func terminateProcess(int) bool { return false }
