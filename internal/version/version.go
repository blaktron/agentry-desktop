// Package version carries the build-stamped application version.
package version

// Version is stamped at build time via
// -ldflags "-X github.com/blaktron/agentry-desktop/internal/version.Version=…".
var Version = "0.0.0-dev"
