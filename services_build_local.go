package main

// The Build tab's Install (agentry-desktop#425, D-049): a QuickAgent that
// was never published, laid down in the runner home from the folder the
// person saved it to, through the CLI's local install (DR-52). Private and
// local: no registry, no pipeline, no credit. What is installed is what
// `agentry publish` would submit for the folder, held to its integrity
// record like any release, and unsigned on every run.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/localinstall"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/registry"
)

// LocalInstallView is what the Build tab says after an Install.
type LocalInstallView struct {
	// Ref is namespace/name@version, the name a run finds it by.
	Ref string `json:"ref"`
	// Digest is the archive digest `agentry publish` would claim.
	Digest string `json:"digest"`
	// Repaired says a changed installed copy was laid down again.
	Repaired bool `json:"repaired"`
	// Warnings are the manifest's own warnings and a policy the export
	// style could not be read from; none of them stopped the install.
	Warnings []string `json:"warnings"`
}

// Install lays the QuickAgent in dir down on this computer. dir is the
// folder Save wrote, which the native picker gave (D-030). The consent is
// a native dialog the page cannot answer: the contract, and that the
// install is local and unsigned. A copy of the same release whose files
// changed since it was installed is laid down again only on a second Yes.
func (b *BuildAPI) Install(dir string) (*LocalInstallView, error) {
	if !b.mu.TryLock() {
		return nil, fmt.Errorf("a submit or an install is already in progress")
	}
	defer b.mu.Unlock()
	if err := b.app.requirePicked("folder", dir); err != nil {
		return nil, err
	}
	home, err := policy.Home()
	if err != nil {
		return nil, err
	}
	s, err := localinstall.Stage(dir, registry.ExtractLimits())
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()

	m := s.Manifest
	var prev *installed.Record
	if st, lerr := installed.Load(); lerr == nil {
		prev = st.Find(installdir.ConsentKey(home, m.Namespace, m.Name))
	}
	if err := b.app.askConsent("Install "+m.Namespace+"/"+m.Name+" on this computer?", localInstallConsentText(s, prev, dir)); err != nil {
		return nil, err
	}

	view := &LocalInstallView{Ref: s.Ref(), Digest: s.Digest, Warnings: []string{}}
	for _, w := range s.Warnings {
		view.Warnings = append(view.Warnings, w.Error())
	}
	style, warning := installdir.StyleForHome(home)
	if warning != "" {
		view.Warnings = append(view.Warnings, warning)
	}
	_, err = s.Install(home, style)
	var changed *installdir.ChangedError
	if errors.As(err, &changed) {
		if cerr := b.app.askConsent("Replace the changed copy?", "The installed copy of "+s.Ref()+" changed after it was installed ("+changedText(changed)+").\n\nLay it down again from "+dir+"?"); cerr != nil {
			return nil, cerr
		}
		_, err = s.Repair(home, style)
		view.Repaired = true
	}
	if err != nil {
		return nil, err
	}
	return view, nil
}

// localInstallConsentText is the Install dialog: what the release may do
// (on a reinstall, what widened), and that nothing vouches for it.
func localInstallConsentText(s *localinstall.Staged, prev *installed.Record, dir string) string {
	m := s.Manifest
	plan := &install.Plan{
		Namespace: m.Namespace, Name: m.Name, Version: m.Version,
		Capabilities:           installed.Snapshot(m.Contract()),
		NeedsCapabilityConfirm: true,
	}
	if prev != nil {
		d := installed.DiffCapabilities(prev.Capabilities, plan.Capabilities)
		plan.PrevVersion, plan.Diff, plan.RequiresWidenedConfirm = prev.Version, &d, d.HasWidened()
	}
	lines := []string{
		s.Ref() + " is installed from " + dir + ", for this computer only: no registry holds it and nothing is submitted.",
		"It is local and unsigned: no pipeline scanned it and no one signed it, and every run asks you to accept that again.",
	}
	if prev != nil {
		from := "a registry"
		if prev.Local() {
			from = "a folder"
		}
		lines = append(lines, "It replaces the installed "+prev.Version+", from "+from+".")
	}
	lines = append(lines, "")
	return strings.Join(lines, "\n") + contractConsentText(plan, nil)
}

// changedText names what changed in an installed copy, briefly.
func changedText(c *installdir.ChangedError) string {
	var parts []string
	for _, k := range []struct {
		label string
		paths []string
	}{{"changed", c.Changed}, {"added", c.Added}, {"removed", c.Removed}} {
		if len(k.paths) > 0 {
			parts = append(parts, k.label+" "+strings.Join(k.paths, ", "))
		}
	}
	if len(parts) == 0 {
		return "its record no longer matches"
	}
	return strings.Join(parts, "; ")
}
