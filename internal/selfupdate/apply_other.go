//go:build !darwin && !windows

package selfupdate

import "fmt"

// On Linux a newer Agentry Desktop is the package manager's to install (D7,
// D-027). The app says so and does nothing, which is the whole of the feature
// there — and the reason is not squeamishness: overwriting a file dpkg or rpm
// owns leaves the package database describing something that is no longer
// there, and the next upgrade either reverts it or refuses.
//
// An apt/dnf repository is the right answer and is a later issue.
type notApplicable struct{}

func newApplier() Applier { return notApplicable{} }

func (notApplicable) Ready() error { return ErrNotApplicable }

func (notApplicable) Apply(string) error {
	return fmt.Errorf("%w", ErrNotApplicable)
}
