package app

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-cli/pkg/runner/installdir"
	"github.com/blaktron/agentry-cli/pkg/runner/localrun"
	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/policy"

	"github.com/blaktron/agentry-desktop/internal/install"
)

// Install installs the release ref (ns/name, or ns/name@version) from the
// registry reg as a persistent agent beside the built-ins. The release is
// resolved, downloaded and verified as any install is; then one native
// dialog states everything it is given: what it may do, its persona, the
// knowledge it ships and each subagent it declares, pinned to the newest
// version its constraint allows, with that version's own reach. A No
// installs nothing. A release that cannot be a persistent agent here is
// refused before the person is asked. The agent is enabled and left
// stopped; its conversation and files from an earlier install of the same
// reference are its again.
func (p *PersistentAPI) Install(reg, ref string) (*PersistentAgentView, error) {
	return p.installRelease(reg, ref)
}

// Update installs the newest version of a published agent from the
// registry it came from, asking again in the same dialog, which names what
// the new version adds. Its subagents are pinned afresh. A running agent is
// stopped first.
func (p *PersistentAPI) Update(name string) (*PersistentAgentView, error) {
	a, err := p.agent(name)
	if err != nil {
		return nil, err
	}
	if !a.Published() {
		return nil, fmt.Errorf("%s is built into Agentry and is updated with the app", a.Title)
	}
	if a.Source != persistent.SourceRegistry {
		return nil, fmt.Errorf("%s was installed from a folder on this computer: install the folder again from the CLI (agentry agent install --local)", a.Ref)
	}
	reg := a.Registry
	if reg == "" && p.ops != nil {
		reg = p.ops.registry()
	}
	return p.installRelease(reg, a.Ref)
}

// installRelease is Install and Update.
func (p *PersistentAPI) installRelease(reg, ref string) (*PersistentAgentView, error) {
	if p.installs == nil {
		return nil, errors.New("this app cannot install QuickAgents")
	}
	home, err := p.home()
	if err != nil {
		return nil, err
	}
	engine := p.installs.engine
	plan, err := engine.Prepare(p.app.ctx, reg, ref, installdir.Target, "")
	if err != nil {
		return nil, err
	}
	m, ok := engine.Manifest(plan.ID)
	if !ok {
		return nil, errors.New("the install was cancelled")
	}
	if err := p.persistentInstallable(home, m); err != nil {
		engine.Cancel(plan.ID)
		return nil, err
	}
	pins := install.ResolveSubagents(p.app.ctx, plan.Registry, m)
	exact := plan.Namespace + "/" + plan.Name + "@" + plan.Version
	if err := p.app.askConsent("Install "+exact+" as a persistent agent?", persistentInstallText(plan, m, pins)); err != nil {
		engine.Cancel(plan.ID)
		return nil, err
	}
	if _, err := engine.Confirm(plan.ID, plan.Unsigned, true, plan.Unscanned); err != nil {
		return nil, err
	}
	p.installs.refreshRow(installdir.ConsentKey(home, plan.Namespace, plan.Name))
	a, err := persistent.SaveRelease(home, persistent.Release{
		Ref: plan.Namespace + "/" + plan.Name, Version: plan.Version, Digest: plan.Digest,
		Source: persistent.SourceRegistry, Registry: plan.Registry, Subagents: pins, InstalledAt: time.Now().UTC(),
	})
	if err != nil {
		return nil, err
	}
	p.stateChanged(a.Name)
	return p.Get(a.Name)
}

// persistentInstallable refuses what cannot be a persistent agent here,
// before the person is asked: a release that does not declare the
// persistent mode, a built-in's reference, a runner policy that allows no
// persistent release, a release that declares files or a target host
// (which a persistent agent is never given) or names an image server or a
// mail provider this build does not carry, and one that runs, here or in
// another client: a new version is installed while it is stopped.
func (p *PersistentAPI) persistentInstallable(home string, m *manifest.Manifest) error {
	ref := m.Namespace + "/" + m.Name
	if !m.IsPersistent() {
		return fmt.Errorf("%s is not a persistent QuickAgent: install it and run it from its page", ref)
	}
	if persistent.IsBuiltinRef(ref) {
		return fmt.Errorf("%s is built into Agentry: turn it on in Settings → Persistent Agents", ref)
	}
	if !persistentReleasesAllowed() {
		return errors.New(policy.PersistentReleasesRefused)
	}
	if fs := m.Capabilities.Filesystem; declaresFiles(fs.Read) || declaresFiles(fs.Write) {
		return fmt.Errorf("%s declares filesystem access, and a persistent agent's files are its share folder and its data volume alone", ref)
	}
	if len(m.Capabilities.TargetHosts) > 0 {
		return fmt.Errorf("%s declares a target host, which a persistent agent is never given: a target host is named in a run's intake", ref)
	}
	if err := localrun.CheckPersistentNames(m.Capabilities.Persistent); err != nil {
		return fmt.Errorf("%s cannot run on this version of Agentry: %w", ref, err)
	}
	key := persistent.Key(m.Namespace, m.Name)
	p.mu.Lock()
	_, running := p.sups[key]
	starting := p.starting[key] != nil
	p.mu.Unlock()
	if running || starting {
		return fmt.Errorf("%s is running: stop it first, then install again", ref)
	}
	if held, h := p.heldElsewhere(home, key); held {
		return fmt.Errorf("%s is running in %s: stop it there first, then install again", ref, holderWords(h))
	}
	return nil
}

// persistentReleasesAllowed reads the runner policy the app starts its
// agents under: a policy that sets allowPersistentReleases: false allows
// none. A missing policy allows them, as a new one does.
func persistentReleasesAllowed() bool {
	polFile, err := policy.Resolve("")
	if err != nil {
		return true
	}
	pol, _, err := policy.Load(polFile.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	return err == nil && pol.PersistentReleasesAllowed()
}

// declaresFiles says a filesystem line grants access: declared, and not
// none.
func declaresFiles(a manifest.FSAccess) bool { return !a.IsZero() && a.Mode != manifest.FSModeNone }

// persistentInstallText is the one consent of a persistent install: where
// the release comes from when that needs accepting, what a persistent
// agent keeps, what this version may do (on an update, what it adds), its
// persona, the knowledge it ships and its subagents, each with the version
// it is pinned to and that version's own reach.
func persistentInstallText(plan *install.Plan, m *manifest.Manifest, pins []persistent.SubagentPin) string {
	var b strings.Builder
	exact := plan.Namespace + "/" + plan.Name + "@" + plan.Version
	if plan.Changed != nil {
		b.WriteString("The installed copy of " + exact + " changed after it was installed (" + plan.Changed.Text(install.MaxNamedPaths) + "); it is laid down again from the download just checked.\n\n")
	}
	if accepting := provenanceAccepted(plan.Unsigned, plan.Unscanned); accepting != "" {
		b.WriteString(strings.TrimSuffix(accepting, " Continue?") + "\n\n")
	}
	b.WriteString(exact + " runs as a persistent agent on this computer: it keeps its conversation, its files and a share folder with you between starts, until you reset it.\n")
	if plan.RequiresWidenedConfirm && plan.Diff != nil {
		b.WriteString("\nUpdating from " + plan.PrevVersion + " widens what it may do:\n")
		for _, w := range plan.Diff.Widened {
			b.WriteString("  • " + w + "\n")
		}
	}
	lines := persistentKeyLines(m.Capabilities.Persistent)
	lines = append(lines, declaredLines(plan)...)
	if len(lines) > 0 {
		b.WriteString("\nIt may:\n")
		for _, l := range lines {
			b.WriteString("  • " + l + "\n")
		}
	}
	if pr := m.Persona; pr != nil {
		lock := "you may rename it and replace its personality"
		if !pr.IsEditable() {
			lock = "you may rename it; its publisher locked its personality"
		}
		b.WriteString("\nIt takes on the persona " + pr.Name + " (" + lock + ").\n")
	}
	if len(pins) > 0 {
		b.WriteString("\nIts subagents, each run in a sandbox of its own under its own contract, and pinned until you update the agent:\n")
		for _, s := range pins {
			if s.Version != "" {
				b.WriteString("  • " + s.Ref + "@" + s.Version + " (" + s.Constraint + "), " + s.Why + ". It reaches " + s.Reach + ".\n")
			} else {
				b.WriteString("  • " + s.Ref + " (" + s.Constraint + ") resolves to no version it can run: " + s.Unresolved + ".\n")
			}
		}
	}
	b.WriteString("\nThe runner enforces this contract. Install it?")
	return b.String()
}

// persistentKeyLines are a release's capabilities.persistent keys as the
// consent states them, in the words of the registry's report.
func persistentKeyLines(p *manifest.Persistent) []string {
	var lines []string
	for _, pc := range manifest.PersistentCapabilities() {
		if !p.Declares(pc.Key) {
			continue
		}
		line := pc.Words
		switch pc.Key {
		case "secrets":
			line += " (" + strings.Join(p.Secrets, ", ") + ")"
		case "mail":
			line += " (" + strings.Join(p.Mail, ", ") + ")"
		case "imageServers":
			line += " (" + strings.Join(p.ImageServers, ", ") + ")"
		}
		lines = append(lines, line)
	}
	return lines
}

// Uninstall uninstalls a published agent: it is no longer listed or
// started, and its release, its MCP servers' connections and the agreement
// to it are removed. Its conversation, its files and everything chosen for
// it are kept, so installing it again takes them up, unless removeData:
// then its volumes and its folder under the runner home go too. The files
// in its share folder are the person's and always stay. A native dialog
// asks first, saying which of the two it is. A running agent is refused.
func (p *PersistentAPI) Uninstall(name string, removeData bool) error {
	a, err := p.agent(name)
	if err != nil {
		return err
	}
	if !a.Published() {
		return fmt.Errorf("%s is built into Agentry and is not uninstalled: turn it off instead", a.Title)
	}
	home, err := p.home()
	if err != nil {
		return err
	}
	p.mu.Lock()
	_, running := p.sups[a.Name]
	starting := p.starting[a.Name] != nil
	p.mu.Unlock()
	if running || starting {
		return fmt.Errorf("%s is running: stop it first", a.Ref)
	}
	if held, h := p.heldElsewhere(home, a.Name); held {
		return fmt.Errorf("%s is running in %s: stop it there first", a.Ref, holderWords(h))
	}
	title, body := "Uninstall "+a.Ref+"?", "Its conversation, its files and everything you chose for it are kept: installing it again takes them up. The files in its share folder are yours and stay."
	if removeData {
		title, body = "Uninstall "+a.Ref+" and remove its data?", "This removes its conversation, its files and everything you chose for it, and cannot be undone. The files in its share folder are yours and stay."
	}
	if err := p.app.askConsent(title, body); err != nil {
		return err
	}
	if err := persistent.Uninstall(p.app.ctx, home, a.Name, p.dockerClient(), removeData); err != nil {
		return err
	}
	ns, short, _ := strings.Cut(a.Ref, "/")
	if p.installs != nil {
		if _, err := p.installs.Uninstall(installdir.ConsentKey(home, ns, short)); err != nil && !strings.Contains(err.Error(), "no install record") {
			return fmt.Errorf("%s is uninstalled, but its release folder was not removed: %w", a.Ref, err)
		}
	}
	p.stateChanged(a.Name)
	return nil
}

// persistentAgentsRelease refuses a plain install of a release installed as
// a persistent agent: it would replace the version the agent is pinned to,
// which the agent's Update moves with its record.
func persistentAgentsRelease(ref string) error {
	r, err := manifest.ParseRef(ref)
	if err != nil {
		return nil
	}
	home, err := policy.Home()
	if err != nil {
		return nil
	}
	if _, err := persistent.LoadRelease(home, persistent.Key(r.Namespace, r.Name)); err == nil {
		return fmt.Errorf("%s/%s is installed as a persistent agent: update it in Settings → Persistent Agents", r.Namespace, r.Name)
	}
	return nil
}
