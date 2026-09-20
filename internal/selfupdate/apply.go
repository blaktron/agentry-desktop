// Applying a newer Agentry Desktop (agentry-desktop#88, M7; plan
// desktop-installers.md §7, §8, D6, D11).
//
// What is the same on every platform lives here; what differs lives in
// apply_darwin.go and apply_windows.go, and apply_other.go refuses.
//
// Three rules hold on all of them, and each is a refusal rather than a
// best-effort:
//
//  1. Nothing is applied that was not downloaded and checked against the
//     digest in the release's SIGNED checksums (download.go). By the time
//     anything here runs, the bytes are proven.
//  2. The app never runs elevated and never asks for an administrator
//     password. A per-user install is updated per-user; anything else is
//     reported as something the person must do themselves (plan §3, D11).
//  3. Nothing under the person's home is touched. Configuration lives outside
//     the installed application by construction (plan §8), so applying an
//     update preserves it by not going near it — which is a property of what
//     this code does NOT do, and is therefore asserted by the acceptance
//     scripts rather than by a line of code here.
package selfupdate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotWritable means the installed application is somewhere this person
// cannot replace without becoming someone else — /Applications for a
// non-admin on macOS, Program Files for a per-machine install on Windows.
//
// It is a refusal with a way forward, never a prompt: asking for an
// administrator password to apply an update the person did not install as an
// administrator is exactly the escalation plan §3 forbids.
var ErrNotWritable = errors.New("this copy of Agentry Desktop is somewhere you cannot replace without administrator rights")

// Applier applies a verified artifact. One per platform; the rest of the
// program never names which.
type Applier interface {
	// Ready reports whether an update could be applied here, and why not when
	// it could not. Called before anything is downloaded, so that a person is
	// not made to wait for a download that was never going to be installable.
	Ready() error
	// Apply installs the verified artifact at path and relaunches. It does not
	// return on success.
	Apply(path string) error
}

// writableDir reports whether this process can create and remove a file in
// dir — the question "can I replace what is in here?", asked by trying rather
// than by reading permission bits.
//
// Reading the bits is what a reasonable person writes first and it is wrong
// on every interesting case: a directory can be mode 0755 and owned by
// someone else, or writable by a group this process is in but which macOS's
// ACLs override, or on a read-only mount. Attempting the operation asks the
// kernel the question the caller actually has.
func writableDir(dir string) error {
	f, err := os.CreateTemp(dir, ".agentry-selfupdate-*")
	if err != nil {
		return fmt.Errorf("%w: %s (%v)", ErrNotWritable, dir, err)
	}
	name := f.Name()
	_ = f.Close()
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("%w: %s (%v)", ErrNotWritable, dir, err)
	}
	return nil
}

// swapDir replaces the directory at dst with the one at src, keeping the old
// one until the new one is in place.
//
// The order matters and is the only interesting thing here. A rename of the
// new directory over the old one is not a single operation on any platform
// this runs on, so the old one is moved ASIDE first: if putting the new one
// in place then fails, the old one is moved back and the person still has a
// working application. The alternative — delete then copy — has a window in
// which a failure leaves no application at all.
//
// Both paths must be on the same filesystem, which they are: the caller
// stages the new copy beside the old one for exactly this reason.
func swapDir(src, dst string) (err error) {
	aside := dst + ".replaced-" + filepath.Base(src)
	_ = os.RemoveAll(aside)

	if err := os.Rename(dst, aside); err != nil {
		return fmt.Errorf("moving the installed copy aside: %w", err)
	}
	defer func() {
		if err != nil {
			// Put it back. If this fails too there is nothing further to try,
			// and the error says where the old copy is so it can be moved by
			// hand rather than hunted for.
			if back := os.Rename(aside, dst); back != nil {
				err = fmt.Errorf("%w — and the installed copy could not be put back; it is at %s", err, aside)
			}
			return
		}
		_ = os.RemoveAll(aside)
	}()

	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("putting the new copy in place: %w", err)
	}
	return nil
}

// ApplierFor returns this platform's applier.
func ApplierFor() Applier { return newApplier() }
