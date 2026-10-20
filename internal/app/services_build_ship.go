package app

// Shipping a knowledge pack with a QuickAgent from the Build tab. The form
// lists the packs the person may ship (ShipOptions) and keeps the ticked
// ones; every save lays them into the workspace from their stores, and a
// submission asks the publisher's confirmation of each one in a native
// dialog the page cannot answer, then sends it with the upload.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/knowledge"
)

// ShipOptions are the packs the Knowledge section can offer: the person's
// own packs kept on this computer or in Agentry, and the shared packs of
// the organizations they belong to, with what decides whether each may ship.
// The page tells a pack that may not ship from one that may, for the
// QuickAgent's namespace, with build.ShipOption.Reason's rules.
func (b *BuildAPI) ShipOptions(reg string) ([]build.ShipOption, error) {
	l, err := b.packList(b.app.ctx, reg)
	if err != nil {
		return nil, err
	}
	return shipOptionsOf(l), nil
}

// packList is the Knowledge page's list, or the test's.
func (b *BuildAPI) packList(ctx context.Context, reg string) (*knowledge.List, error) {
	if b.lister != nil {
		return b.lister(ctx, reg)
	}
	env := knowledge.Default()
	if b.run != nil {
		env = b.run.knowledge()
	}
	return env.List(ctx, reg)
}

// shipOptionsOf reads the Knowledge page's list as the packs a QuickAgent
// can ship. A pack is ready when AI Review has read a copy the registry or
// this computer holds; an organization's pack ships from the registry's
// copy, which the registry checks against the organization.
func shipOptionsOf(l *knowledge.List) []build.ShipOption {
	out := []build.ShipOption{}
	for _, p := range l.Packs {
		if !p.Local && !p.InAgentry {
			continue
		}
		o := build.ShipOption{ShipSource: build.ShipSource{Name: p.Name, Kind: build.ShipOwn, ID: p.ID, AgentryID: p.AgentryID, Bytes: p.Bytes, Pages: p.Pages}}
		if p.Shared {
			o.Kind, o.Org, o.OrgAdmin = build.ShipOrg, p.Org, p.OrgAdmin
			o.Ready = p.InAgentry && p.AgentryReady
		} else {
			o.Ready = (p.Local && p.ScanState == knowledge.ScanScanned) || p.AgentryReady
		}
		out = append(out, o)
	}
	return out
}

// saveWorkspace writes the form's files (build.Save) and then lays each pack
// it ships into the folder, reading it again from its store, so a version
// ships the pack as it stands now. A pack the form has no source for is the
// folder's own files and is left as it is; a pack laid earlier that the form
// no longer ships is taken out.
func (b *BuildAPI) saveWorkspace(ctx context.Context, dir string, m *build.Model) error {
	if err := build.Save(dir, m); err != nil {
		return err
	}
	return b.layShips(ctx, dir, m)
}

func (b *BuildAPI) layShips(ctx context.Context, dir string, m *build.Model) error {
	sources := map[string]build.ShipSource{}
	for _, s := range m.Ships {
		sources[s.Name] = s
	}
	var names []string
	for _, n := range m.KnowledgeShips {
		if n != "" {
			names = append(names, n)
		}
	}
	if b.shipper == nil {
		if len(sources) > 0 {
			return build.ErrNoShipper
		}
		return nil
	}
	for _, n := range names {
		src, ok := sources[n]
		if !ok {
			continue
		}
		if _, err := b.shipper.Lay(ctx, dir, src); err != nil {
			return fmt.Errorf("the knowledge pack %s could not be put in the folder: %w", n, err)
		}
	}
	return b.shipper.Prune(dir, names)
}

// reconcileShips ties the packs a release shipped, which the form read as
// names, to the person's packs now (build.Model.ReconcileShips): a pack they
// can still ship stays ticked and is read again at the next save, and any
// other is unticked and named in the notes, never carried silently. A list
// of their packs that cannot be read unticks them all, and says so.
func (b *BuildAPI) reconcileShips(ctx context.Context, reg string, m *build.Model) []string {
	if len(m.KnowledgeShips) == 0 {
		return nil
	}
	l, err := b.packList(ctx, reg)
	if err != nil {
		names := joinNames(m.KnowledgeShips)
		for _, n := range slices.Clone(m.KnowledgeShips) {
			m.UntickShip(n)
		}
		return []string{fmt.Sprintf("Your knowledge packs could not be read (%v), so the packs the previous version shipped (%s) are unticked: tick them again in the Knowledge section.", err, names)}
	}
	return m.ReconcileShips(shipOptionsOf(l))
}

// confirmShips asks the publisher's confirmation of each pack the folder
// ships, one native dialog each, in the words every client uses, and returns
// the acknowledgement to send with the upload. A No on any pack returns nil
// and sends nothing. A folder that ships nothing is not asked.
func (b *BuildAPI) confirmShips(dir string, names []string, private bool) (*build.KnowledgeShipping, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var origins map[string]*build.PackOrigin
	if b.shipper != nil {
		var err error
		if origins, err = b.shipper.Origins(dir); err != nil {
			return nil, fmt.Errorf("read where the shipped packs came from: %w", err)
		}
	}
	for _, n := range names {
		size, _, err := build.PackTreeSize(dir, n)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("the QuickAgent ships the knowledge pack %s, and the folder %s holds no knowledge/%s: save it again", n, dir, n)
			}
			return nil, err
		}
		c := build.ShipConfirmation(n, size, private, nil)
		if err := b.app.askConsent("Ship the knowledge pack "+n+"?", c.Text()+"\n\nShip it?"); err != nil {
			if errors.Is(err, errNotConfirmed) {
				return nil, nil
			}
			return nil, err
		}
	}
	return build.AcknowledgementFor(names, origins), nil
}

// shipsNote is what Submit's own dialog says of the packs it ships, before
// each one is asked.
func shipsNote(names []string) string {
	if len(names) == 0 {
		return ""
	}
	word := "pack"
	if len(names) > 1 {
		word = "packs"
	}
	return fmt.Sprintf("It ships the knowledge %s %s, readable by everyone who can download the release; you confirm each pack next.", word, joinNames(names))
}

func joinNames(names []string) string {
	s := slices.Clone(names)
	out := ""
	for i, n := range s {
		switch {
		case i == 0:
			out = n
		case i == len(s)-1:
			out += " and " + n
		default:
			out += ", " + n
		}
	}
	return out
}

// shipChoices resolves the names of the packs a draft ships to the packs
// they are: each must be among the packs the draft reads, and a pack the
// person may ship in a QuickAgent in namespace ns. A person's own pack wins
// over an organization's of the same name. It returns them in the order
// given.
func (b *BuildAPI) shipChoices(ctx context.Context, reg, ns string, packs, ships []string) ([]build.ShipSource, error) {
	if len(ships) == 0 {
		return nil, nil
	}
	if len(ships) > build.MaxShips {
		return nil, fmt.Errorf("a release ships at most %d knowledge packs", build.MaxShips)
	}
	l, err := b.packList(ctx, reg)
	if err != nil {
		return nil, fmt.Errorf("your knowledge packs could not be read, so none can be shipped: %w", err)
	}
	options := shipOptionsOf(l)
	var out []build.ShipSource
	seen := map[string]bool{}
	for _, n := range ships {
		if seen[n] {
			continue
		}
		seen[n] = true
		if !slices.Contains(packs, n) {
			return nil, fmt.Errorf("the draft ships %s, which is not among the packs it reads", n)
		}
		var pick *build.ShipOption
		for i := range options {
			if options[i].Name != n {
				continue
			}
			if pick == nil || (pick.Reason(ns) != "" && options[i].Reason(ns) == "") {
				pick = &options[i]
			}
		}
		switch {
		case pick == nil:
			return nil, fmt.Errorf("%s is not among your knowledge packs", n)
		case pick.Reason(ns) != "":
			return nil, fmt.Errorf("%s cannot ship with this QuickAgent: %s", n, pick.Reason(ns))
		}
		out = append(out, pick.ShipSource)
	}
	return out, nil
}

// shipConsent is what the draft's dialog says of the packs it ships, or "".
func shipConsent(ships []build.ShipSource) string {
	if len(ships) == 0 {
		return ""
	}
	names := make([]string, 0, len(ships))
	for _, s := range ships {
		names = append(names, s.Name)
	}
	return "You chose to ship " + joinNames(names) + " with the QuickAgent as well: it goes into the release, readable by everyone who can download it, when you submit the QuickAgent, and Submit asks you to confirm each pack then. "
}
