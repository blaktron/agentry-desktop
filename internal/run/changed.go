package run

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/engine"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
)

// ChangedInstallError is a registry release installed on this computer whose
// files, or whose install.json, changed after it was laid down. It wraps the
// runner's own refusal (errors.As finds the *installdir.ChangedError with
// every path) and names the desktop's way back, Repair in the run setup,
// rather than the CLI's command.
type ChangedInstallError struct {
	*installdir.ChangedError
}

func (e *ChangedInstallError) Unwrap() error { return e.ChangedError }

func (e *ChangedInstallError) Error() string {
	what := e.Dir
	if e.Ref != "" {
		what = fmt.Sprintf("%s (%s)", e.Ref, e.Dir)
	}
	return fmt.Sprintf("installed release %s changed after it was installed: %s — refused; Repair, in the run setup, lays it down again from a fresh download, checked as any install is",
		what, changedParts(e.ChangedError))
}

// maxNamedPaths bounds how many paths of each kind a message spells out; the
// error's fields carry them all.
const maxNamedPaths = 10

// changedParts names what failed: the record itself, then the changed, added
// and removed files.
func changedParts(c *installdir.ChangedError) string {
	var parts []string
	switch c.Record {
	case installdir.InstallJSONChanged:
		parts = append(parts, "install.json changed")
	case installdir.RecordInvalid:
		parts = append(parts, "its integrity record ("+installdir.RecordFile+") does not verify under this runner home's key")
	}
	for _, k := range []struct {
		label string
		paths []string
	}{{"changed", c.Changed}, {"added", c.Added}, {"removed", c.Removed}} {
		if len(k.paths) == 0 {
			continue
		}
		named, more := k.paths, ""
		if len(named) > maxNamedPaths {
			named, more = named[:maxNamedPaths], fmt.Sprintf(" and %d more", len(named)-maxNamedPaths)
		}
		parts = append(parts, k.label+" "+strings.Join(named, ", ")+more)
	}
	if len(parts) == 0 {
		return "its record no longer matches"
	}
	return strings.Join(parts, "; ")
}

// asChanged names a changed install for the release the run resolved (not
// for the install.json that failed its record) with the way back that fits
// it: a local install's folder, or Repair for a registry release.
func asChanged(sig *Signature, err error) error {
	var changed *installdir.ChangedError
	if !errors.As(err, &changed) {
		return err
	}
	changed.Ref = sig.Ref
	if sig.Local() {
		changed.Repair = localRepair
		return changed
	}
	return &ChangedInstallError{changed}
}

// recheckInstall holds the install to its record again just before anything
// is compiled from it, so files edited while the setup or a dialog was open
// are refused rather than compiled into a plan that later runs reuse.
func recheckInstall(sig *Signature, installDir string) error {
	_, err := installdir.Inspect(installDir, sig.Digest)
	if err == nil {
		return nil
	}
	var changed *installdir.ChangedError
	if errors.As(err, &changed) {
		return engine.Classify(engine.ExitVerify, asChanged(sig, err))
	}
	return engine.Classify(engine.ExitVerify, fmt.Errorf("installed %s: %w", sig.Ref, err))
}

// copyCheck is the engine's check of the run's own copy of the release's
// files against the install's record: an edit made between the last check
// and the copy does not run either.
func copyCheck(sig *Signature, installDir string) func(string) error {
	return func(copyDir string) error {
		return asChanged(sig, installdir.CheckCopy(installDir, copyDir))
	}
}

// InstalledChanges says whether the release ref resolves to on this computer
// is installed and changed after it was laid down: the refusal a run would
// meet, with every path, or nil when the install is whole, has no record
// yet, or is not installed at all. The run setup shows it with Repair before
// Run is pressed.
func InstalledChanges(ctx context.Context, regURL, ref string) (*installdir.ChangedError, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	sig, err := resolveForRun(ctx, regURL, ref, false)
	if err != nil {
		var changed *installdir.ChangedError
		if errors.As(err, &changed) {
			return changed, nil
		}
		return nil, err
	}
	if sig.Local() {
		return nil, nil
	}
	_, err = installdir.Inspect(installdir.ReleaseDir(home, sig.Namespace, sig.Name, sig.Version, sig.Digest), sig.Digest)
	var changed *installdir.ChangedError
	if errors.As(err, &changed) {
		changed.Ref = sig.Ref
		return changed, nil
	}
	return nil, nil
}
