// Package baked answers what a build was made to talk to, so a release can
// read the values its linker flags set back from the binary itself: a -X on
// a variable that was renamed or moved is ignored without an error, and a
// search of the binary for the registry's address also finds it inside the
// update channel's.
package baked

import (
	"fmt"
	"io"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// Flag makes the binary print the values and exit, before any window opens.
// It is a variable, not a constant, so its bytes are in the binary as they
// read: a release runs a binary only once it finds them, since one built
// before the flag would open its window instead.
var Flag = "--print-baked"

// Main prints the registry the app starts pointed at and the update
// channel it checks, one per line, and answers the exit code.
func Main(w io.Writer) int {
	if _, err := fmt.Fprintf(w, "registry %s\nchannel %s\n", settings.DefaultRegistryURL, selfupdate.ManifestURL); err != nil {
		return 1
	}
	return 0
}
