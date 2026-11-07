//go:build !linux && !darwin && !windows

package linkservice

import (
	"context"
	"fmt"

	"github.com/blaktron/agentry-desktop/internal/computeruse"
)

// SetTokenPath has nothing to keep on this system.
func SetTokenPath(string) {}

// openComputer refuses: this version of Agentry Desktop does computer use
// on Linux alone.
func openComputer(context.Context, ComputerRequest) (computeruse.Backend, error) {
	return nil, fmt.Errorf("%w: this version of Agentry Desktop does computer use on Linux, macOS and Windows only", computeruse.ErrNotGranted)
}

// ComputerSupport says computer use is not in this version here.
func ComputerSupport() Support {
	return Support{Session: "this system", Problem: "This version of Agentry Desktop does computer use on Linux, macOS and Windows only."}
}

// RequestComputerPermissions has nothing to ask on this system.
func RequestComputerPermissions() Support { return ComputerSupport() }
