//go:build agentry_dev_loopback

package selfupdate

// devLoopbackChannel lets a developer's build (`go build -tags
// agentry_dev_loopback`) point its update channel at a plain-http server on
// this machine, to try the self-update against a local channel. Nothing the
// release ships is built with this tag.
const devLoopbackChannel = true
