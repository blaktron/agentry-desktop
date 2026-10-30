package main

import "github.com/godbus/dbus/v5"

// sessionBusUsable says whether this session has a message bus to hold the
// single-instance name on. A desktop session always has one; a bare X server
// or a minimal window manager may not, and the app then runs without the
// one-instance lock rather than not at all.
func sessionBusUsable() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
