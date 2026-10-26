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

// HardwareTier is the models' hardware tier here: the CPU, until the person
// takes the offer of a graphics card (Manager.ProbeGPU).
func HardwareTier() string { return TierCPU }

// OffersGPU says whether the transcriber here may carry a graphics backend
// worth measuring: Vulkan on Linux and Windows.
func OffersGPU() bool { return runtime.GOOS == "linux" || runtime.GOOS == "windows" }
