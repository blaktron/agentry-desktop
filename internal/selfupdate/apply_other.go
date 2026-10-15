//go:build !darwin && !windows

package selfupdate

import "fmt"

// On Linux a newer Agentry Desktop is the package manager's to install. The
// app says so and does nothing, which is the whole of the feature there — and
// the reason is not squeamishness: overwriting a file dpkg or rpm owns leaves
// the package database describing something that is no longer there, and the
// next upgrade either reverts it or refuses.
//
// A package repository (apt, dnf) is the proper update channel there.
type notApplicable struct{}

func newApplier() Applier { return notApplicable{} }

func (notApplicable) Ready() error { return ErrNotApplicable }

func (notApplicable) Apply(string, bool) error {
	return fmt.Errorf("%w", ErrNotApplicable)
}
