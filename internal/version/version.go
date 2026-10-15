// Package version carries the build-stamped application version.
package version

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"sync"
)

// devVersion is what an unstamped `go build` reports.
const devVersion = "0.0.0-dev"

// Version is stamped at build time via
// -ldflags "-X github.com/blaktron/agentry-desktop/internal/version.Version=…".
var Version = devVersion

var (
	buildOnce sync.Once
	build     string
)

// Build names this binary for the plan cache: a compiled run plan records
// it (spec.Plan.CompiledBy), and a plan recorded by another build is
// recompiled, because the compiler's output can change while the plan
// schema does not.
//
// A stamped release is named by its version, which is unique to the commit
// it was built from. An unstamped build (0.0.0-dev) or a dirty one
// (describe's -dirty suffix, which the Makefile's VERSION carries) keeps
// the same version across code changes, so it would reuse the plans an
// older build compiled. Its name adds a digest of the running executable
// instead: another binary is another build, and the same binary is the same
// build. Computed once per process.
//
// A copy of the CLI's internal/version.Build: the CLI's internal package
// is not importable, and the two must spell the identity the same way —
// <version>+exe.<16 hex> — so a plan's CompiledBy reads alike whichever
// client wrote it.
func Build() string {
	buildOnce.Do(func() { build = buildID(Version, os.Executable) })
	return build
}

// buildID is Build with its inputs injected. When the executable cannot be
// read it returns the version alone: the plan check is then what it was
// before, never stricter than it can prove.
func buildID(v string, executable func() (string, error)) string {
	if v != devVersion && !strings.HasSuffix(v, "-dirty") {
		return v
	}
	path, err := executable()
	if err != nil {
		return v
	}
	f, err := os.Open(path) //nolint:gosec // the running executable
	if err != nil {
		return v
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return v
	}
	return v + "+exe." + hex.EncodeToString(h.Sum(nil))[:16]
}
