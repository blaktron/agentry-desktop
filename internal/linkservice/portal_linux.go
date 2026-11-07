//go:build linux

package linkservice

import "github.com/blaktron/agentry-desktop/internal/computeruse/portal"

// portalVersions are the RemoteDesktop and ScreenCast portals' versions, 0
// where the portal lacks one.
func portalVersions() (remote, screen uint32) {
	c, err := portal.Connect()
	if err != nil {
		return 0, 0
	}
	defer func() { _ = c.Close() }()
	return c.Version("org.freedesktop.portal.RemoteDesktop"), c.Version("org.freedesktop.portal.ScreenCast")
}
