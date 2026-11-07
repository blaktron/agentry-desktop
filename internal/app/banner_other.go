//go:build !linux

package app

// keepBannerAbove leaves the banner's own always-on-top to the system.
func keepBannerAbove(int) {}
