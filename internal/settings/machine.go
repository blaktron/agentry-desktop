package settings

// The machine settings file: settings the administrator of this computer
// deploys (/etc/agentry/managed.json, a macOS configuration profile, or
// Windows Group Policy), read through the agentry command's package. When
// it pins a registry, that registry is the only one this app uses: the
// default, the only one it adds, and the only one it sends a token to.
// Machine settings that exist and cannot be used leave the registry
// unknown, so the app picks none.

import (
	"fmt"

	"github.com/blaktron/agentry-cli/pkg/managed"
)

// machineRegistry is the registry the machine settings pin; tests replace
// it.
var machineRegistry = managed.MachineRegistry

// PinnedRegistry is the registry this computer's machine settings pin,
// normalised, or "" when they pin none. An error is machine settings that
// cannot be used: no registry may be chosen while it stands.
func PinnedRegistry() (string, error) {
	reg, err := machineRegistry()
	if err != nil {
		return "", err
	}
	return Normalize(reg), nil
}

// PinRefusal is why regURL cannot be used under the machine settings, or
// nil when it can: another registry than the pinned one, or any while the
// machine settings cannot be used.
func PinRefusal(regURL string) error { return pinnedOut(regURL) }

func pinnedOut(regURL string) error {
	pin, err := PinnedRegistry()
	switch {
	case err != nil:
		return fmt.Errorf("this computer's machine settings cannot be used, so no registry can be chosen until its administrator corrects them: %w", err)
	case pin != "" && !SameOrigin(regURL, pin):
		return fmt.Errorf("this computer's administrator pins Agentry to the registry %s, so %s cannot be used", DisplayOrigin(pin), DisplayOrigin(regURL))
	}
	return nil
}

// applyPin holds s to the machine settings' registry: the pinned one is the
// default and is listed; with machine settings that cannot be used, no
// registry is the default. The other registries stay in the file, for the
// day the pin is lifted, and get no token meanwhile (SignInFor).
func (s Settings) applyPin() Settings {
	pin, err := PinnedRegistry()
	switch {
	case err != nil:
		s.DefaultRegistry = ""
	case pin != "":
		s.DefaultRegistry = pin
		if s.Find(pin) == nil {
			s.Registries = append(s.Registries, Registry{URL: pin})
		}
	}
	return s
}
