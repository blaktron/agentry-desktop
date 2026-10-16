//go:build !agentry_dev_loopback

package selfupdate

// devLoopbackChannel is false in every build without the
// agentry_dev_loopback tag, which is every build the Makefile, CI and a
// release make: the update channel must be https, with no
// exception for this machine.
const devLoopbackChannel = false
