package app

// The QuickAgents an organization's managed settings give its members (the
// quickagents row). A required one is installed after each real read of the
// settings, through the install engine's assigned path: every check of an
// ordinary install, and no consent written, so the member's first run shows
// the contract under "Required by <organization>" and asks. It cannot be
// uninstalled while a held document requires it. An offered one is listed
// for the member to install on demand. A preset fills the run setup, or
// fixes it when locked.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/blaktron/agentry/pkg/managedsettings"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/managed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/run"
)

// eventLibraryChanged tells the page what is installed on this computer
// changed without the person asking: an organization's required QuickAgent
// was installed or updated.
const eventLibraryChanged = "library:changed"

// PresetView is the run settings an organization gave a QuickAgent: each a
// default for the run setup, or fixed when Locked.
type PresetView struct {
	Organization   string            `json:"organization"`
	Locked         bool              `json:"locked"`
	TimeoutSeconds int               `json:"timeoutSeconds,omitempty"`
	MaxTurns       int               `json:"maxTurns,omitempty"`
	Where          string            `json:"where,omitempty"`
	Inputs         map[string]string `json:"inputs,omitempty"`
}

// AssignmentView is one QuickAgent an organization's held document lists,
// with what this computer holds of it.
type AssignmentView struct {
	// Ref is ns/name; Version the pinned version or "latest"; Selector the
	// reference an install resolves.
	Ref      string `json:"ref"`
	Version  string `json:"version"`
	Selector string `json:"selector"`
	// Mode is "required" or "offered".
	Mode         string `json:"mode"`
	Required     bool   `json:"required"`
	Organization string `json:"organization"`
	Registry     string `json:"registry"`
	// Installed says the runner home holds it, at InstalledVersion.
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installedVersion,omitempty"`
	// Problem is why the last install of a required one did not happen.
	Problem string `json:"problem,omitempty"`
	// Preset is the run settings the organization gave it, nil when none.
	Preset *PresetView `json:"preset,omitempty"`
}

func splitRef(ref string) (ns, name string, ok bool) {
	r, err := manifest.ParseRef(ref)
	if err != nil {
		return "", "", false
	}
	return r.Namespace, r.Name, true
}

func presetView(a managed.Assigned) *PresetView {
	if a.Preset == nil {
		return nil
	}
	p := a.Preset
	return &PresetView{Organization: a.Organization, Locked: a.PresetLocked, TimeoutSeconds: p.TimeoutSeconds, MaxTurns: p.MaxTurns, Where: p.Where, Inputs: p.Inputs}
}

// presetOf is the preset the held documents give the release ref names.
func presetOf(ref string) *PresetView {
	ns, name, ok := splitRef(ref)
	if !ok {
		return nil
	}
	a, ok := managedCalls.current().AssignmentOf(ns + "/" + name)
	if !ok {
		return nil
	}
	return presetView(a)
}

// ManagedAssignments are the QuickAgents the held documents offer or
// require, for the Library's "From <organization>" section. The page reads
// them again on managed:changed and library:changed.
func (s *SettingsAPI) ManagedAssignments() []AssignmentView {
	held := managedCalls.current()
	out := []AssignmentView{}
	items, _ := install.List()
	s.assignMu.Lock()
	defer s.assignMu.Unlock()
	for _, a := range held.Offered() {
		v := AssignmentView{
			Ref: a.Ref, Version: a.Version, Selector: a.Selector(), Mode: a.Mode, Required: a.Required(),
			Organization: a.Organization, Registry: a.Registry, Problem: s.assignProblems[a.Ref], Preset: presetView(a),
		}
		for _, it := range items {
			if it.Target == installdir.Target && it.Present && it.Namespace+"/"+it.Name == a.Ref {
				v.Installed, v.InstalledVersion = true, it.Version
				break
			}
		}
		out = append(out, v)
	}
	return out
}

// installRequired installs or updates each QuickAgent the freshly read
// document requires. A failure installs nothing for that QuickAgent, is
// logged and kept for the Library, and never stops the others or the read.
func (s *SettingsAPI) installRequired(ctx context.Context, st *managed.State) {
	if st == nil || s.installs == nil {
		return
	}
	required := (managed.Effective{Documents: []managed.State{*st}}).Required()
	if len(required) == 0 {
		return
	}
	held := managedCalls.current()
	for _, a := range required {
		if err := s.installAssigned(ctx, a, held); err != nil {
			slog.Warn("managed settings: a required QuickAgent was not installed", "ref", a.Ref, "organization", a.Organization, "err", oneLine(err))
		}
	}
	emitEvent(s.app, eventLibraryChanged)
}

// installAssigned is one required install and the bookkeeping around it.
func (s *SettingsAPI) installAssigned(ctx context.Context, a managed.Assigned, held managed.Effective) error {
	asg := install.Assignment{Registry: a.Registry, Ref: a.Selector(), Organization: a.Organization, Held: held}
	do := s.installs.engine.InstallAssigned
	if s.installOne != nil {
		do = s.installOne
	}
	_, err := do(ctx, asg)
	if errors.Is(err, install.ErrCurrent) {
		err = nil
	}
	s.assignMu.Lock()
	if s.assignProblems == nil {
		s.assignProblems = map[string]string{}
	}
	if err != nil {
		s.assignProblems[a.Ref] = fmt.Sprintf("%s, required by %s, was not installed: %s", a.Ref, a.Organization, oneLine(err))
	} else {
		delete(s.assignProblems, a.Ref)
	}
	s.assignMu.Unlock()
	if err == nil {
		if ns, name, ok := splitRef(a.Ref); ok {
			if home, herr := policy.Home(); herr == nil {
				s.installs.refreshRow(installdir.ConsentKey(home, ns, name))
			}
		}
	}
	return err
}

// InstallRequired tries again the install of a QuickAgent a held document
// requires and that is not installed. It records no consent, as the
// install after a read does.
func (s *SettingsAPI) InstallRequired(ref string) error {
	if s.installs == nil {
		return errors.New("installing is not available here")
	}
	held := managedCalls.current()
	for _, a := range held.Required() {
		if a.Ref == ref {
			err := s.installAssigned(s.app.ctxOrBackground(), a, held)
			emitEvent(s.app, eventLibraryChanged)
			return err
		}
	}
	return fmt.Errorf("%s is not required by the managed settings held on this computer", ref)
}

// oneLine is err's text on one line.
func oneLine(err error) string { return strings.Join(strings.Fields(err.Error()), " ") }

// presetRefusal is the refusal of a value that conflicts with a locked
// preset.
func presetRefusal(org, ref, what, fix string) error {
	return fmt.Errorf("%s conflicts with %s's managed settings, which fix %s for %s (%s)", what, org, fix, ref, managedsettings.QuickAgents)
}

var presetPlaces = map[string]string{
	managedsettings.WhereLocal: "this computer", managedsettings.WhereHosted: "In Agentry",
	managedsettings.WhereRunhost: "a runhost", managedsettings.WhereTargetHost: "a target host",
}

// applyPreset fills a run's settings from the preset an organization gave
// the QuickAgent it is of, and refuses what a locked preset forbids. Each
// preset field is a default: what the person set stands, unless the preset
// is locked, when a different value is refused. A timeout of 0, turns below
// 0 and an unanswered question are unset. where is the place the run goes
// ("local", "hosted" or "runhost"); intake is returned as a copy when the
// preset adds answers. Every other managed setting still holds the run
// afterwards.
func applyPreset(held managed.Effective, ref, where string, timeout, maxTurns int, intake map[string]string) (int, int, map[string]string, error) {
	ns, name, ok := splitRef(ref)
	if !ok {
		return timeout, maxTurns, intake, nil
	}
	a, ok := held.AssignmentOf(ns + "/" + name)
	if !ok || a.Preset == nil {
		return timeout, maxTurns, intake, nil
	}
	p, org, id := a.Preset, a.Organization, a.Ref
	if p.TimeoutSeconds > 0 {
		switch {
		case timeout <= 0:
			timeout = p.TimeoutSeconds
		case a.PresetLocked && timeout != p.TimeoutSeconds:
			return 0, 0, nil, presetRefusal(org, id, fmt.Sprintf("a timeout of %d seconds", timeout), fmt.Sprintf("the timeout at %d seconds", p.TimeoutSeconds))
		}
	}
	if p.MaxTurns > 0 {
		switch {
		case maxTurns < 0:
			maxTurns = p.MaxTurns
		case a.PresetLocked && maxTurns != p.MaxTurns:
			return 0, 0, nil, presetRefusal(org, id, fmt.Sprintf("%d turns", maxTurns), fmt.Sprintf("the tool-call budget at %d", p.MaxTurns))
		}
	}
	if a.PresetLocked && p.Where != "" && p.Where != where {
		return 0, 0, nil, presetRefusal(org, id, "a run on "+presetPlaces[where], "its runs to "+presetPlaces[p.Where])
	}
	var answered map[string]string
	for qid, want := range p.Inputs {
		got := strings.TrimSpace(intake[qid])
		switch {
		case got == "":
			if answered == nil {
				answered = make(map[string]string, len(intake)+len(p.Inputs))
				for k, v := range intake {
					answered[k] = v
				}
			}
			answered[qid] = want
		case a.PresetLocked && intake[qid] != want:
			return 0, 0, nil, presetRefusal(org, id, "the answer to "+qid, "the answer to "+qid)
		}
	}
	if answered != nil {
		intake = answered
	}
	return timeout, maxTurns, intake, nil
}

// askRequiredConsent asks the person's consent to the contract of a
// QuickAgent an organization installed for them, before a run of it starts:
// in a native dialog the page cannot answer, headed "Required by
// <organization>", whatever the contract declares. What the person accepts
// is recorded by the run, so the next run asks nothing until a version
// widens it. A QuickAgent the person installed, or one they already
// accepted, raises nothing.
func (r *RunAPI) askRequiredConsent(reg, ref string) error {
	ns, name, ok := splitRef(ref)
	if !ok {
		return nil
	}
	// What cannot be read refuses the run: a record that cannot be checked
	// might be one an organization left with no consent in it.
	home, err := policy.Home()
	if err != nil {
		return fmt.Errorf("the runner home could not be found to check your consent to %s: %w", ref, err)
	}
	state, err := installed.Load()
	if err != nil {
		return fmt.Errorf("the record of what you accepted could not be read to check your consent to %s: %w", ref, err)
	}
	if rec := state.Find(installdir.ConsentKey(home, ns, name)); rec == nil || rec.AssignedBy == "" {
		return nil
	}
	info, err := run.ProfileStatus(r.app.ctxOrBackground(), reg, ref)
	if err != nil {
		return fmt.Errorf("%s was installed by your organization's managed settings, and its contract could not be read to ask your consent: %w", ref, err)
	}
	if info.RequiredBy == "" {
		return nil
	}
	return r.app.askConsent("Accept what "+ref+" may do?", requiredConsentText(ref, info))
}

// requiredConsentText is the contract of a release an organization requires,
// as the consent dialog states it.
func requiredConsentText(ref string, info *run.ProfileInfo) string {
	lines := []string{"Required by " + info.RequiredBy, "", ref + " declares:"}
	for _, l := range info.CapabilityLines {
		lines = append(lines, "  • "+l)
	}
	lines = append(lines, "", "Running it accepts this contract. Continue?")
	return strings.Join(lines, "\n")
}
