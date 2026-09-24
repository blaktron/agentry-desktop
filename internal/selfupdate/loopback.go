package selfupdate

import "net"

// isLoopback reports whether host names this machine: localhost, or a
// loopback address (127.0.0.0/8, ::1).
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
