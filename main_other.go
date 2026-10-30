//go:build !linux

package main

// sessionBusUsable: the single-instance lock needs no message bus outside
// Linux.
func sessionBusUsable() bool { return true }
