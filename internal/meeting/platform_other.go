//go:build !darwin

package meeting

import "runtime"

// PlatformSupport says why this computer cannot record meetings, "" when it
// can.
func PlatformSupport() string {
	switch runtime.GOOS {
	case "linux", "windows":
		return ""
	}
	return "Meeting notes records on Linux, Windows and macOS only."
}

// HardwareTier is the models' hardware tier here: the CPU, until a usable
// graphics card is offered.
func HardwareTier() string { return TierCPU }
