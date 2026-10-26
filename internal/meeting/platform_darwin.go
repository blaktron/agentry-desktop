package meeting

// #include "coreaudio_darwin.h"
import "C"

import "runtime"

// PlatformSupport says why this computer cannot record meetings, "" when it
// can: process taps need macOS 14.4 or later.
func PlatformSupport() string {
	if C.ca_supported() != 1 {
		return "Meeting notes needs macOS 14.4 or later."
	}
	return ""
}

// HardwareTier is the models' hardware tier here: Apple silicon runs the
// transcriber on Metal, an Intel Mac on its CPU.
func HardwareTier() string {
	if runtime.GOARCH == "arm64" {
		return TierMetal
	}
	return TierCPU
}

// OffersGPU is false: Apple silicon runs on Metal from the start, and an
// Intel Mac stays on its CPU.
func OffersGPU() bool { return false }
