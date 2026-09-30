//go:build !linux && !darwin && !windows

package run

// machineName has no answer here: records carry no machine and are judged
// by the pid alone.
func machineName() string { return "" }
