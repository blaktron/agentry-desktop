package install

// An install an organization's managed settings ask for. It goes through the
// same resolve, download, digest and attestation checks as an install the
// person asks for, and is held to the install sources and the risk rule of
// the settings held here, but nothing is asked and nothing is consented to:
// the record it leaves holds no capabilities and names the organization, so
// the person's first run shows the contract and asks.

import (
	"context"
	"errors"

	"github.com/blaktron/agentry/pkg/apiclient"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"
)

// ErrCurrent is InstallAssigned's answer when the runner home already holds
// the resolved release, intact, and its record names that version: nothing
// was installed again.
var ErrCurrent = errors.New("already installed at that version")

// Assignment is one QuickAgent an organization requires.
type Assignment struct {
	// Registry is the registry the managed settings came from, and the one
	// the release is resolved on.
	Registry string
	// Ref is the release's selector: ns/name, or ns/name@version.
	Ref string
	// Organization is the handle of the organization that requires it.
	Organization string
	// Held is the managed settings in force, which the install is held to.
	Held managed.Effective
}

// InstallAssigned installs or updates the release an organization requires,
// into the runner home, and records it as the organization's: no capability
// consent is written. ErrCurrent says nothing needed doing.
func (e *Engine) InstallAssigned(ctx context.Context, a Assignment) (*Result, error) {
	p, err := e.prepare(ctx, a.Registry, a.Ref, installdir.Target, "", &a)
	if err != nil {
		return nil, err
	}
	return e.write(p)
}

// installIsCurrent says the runner home holds the resolved release, intact,
// and its record names that version.
func installIsCurrent(res *apiclient.ResolveResult) (bool, error) {
	home, err := policy.Home()
	if err != nil {
		return false, err
	}
	state, err := installed.Load()
	if err != nil {
		return false, nil
	}
	rec := state.Find(installdir.ConsentKey(home, res.Namespace, res.Name))
	if rec == nil || rec.Version != res.Version {
		return false, nil
	}
	_, ierr := installdir.Inspect(installdir.ReleaseDir(home, res.Namespace, res.Name, res.Version, res.Digest), res.Digest)
	return ierr == nil, nil
}
