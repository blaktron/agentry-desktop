package knowledge

// Shipping a stored pack with a QuickAgent: the pack is read from where the
// person keeps it, held to what a release ships, and laid into the
// QuickAgent's folder with the CLI's own package (ship), which writes the
// files the registry's rules read and the record of where the pack came
// from. The record is the same file `agentry knowledge ship` writes, so a
// folder made here publishes the same from the command line.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/ship"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"

	"github.com/blaktron/agentry-desktop/internal/build"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// shipPack is a pack opened to be shipped, and where it came from.
type shipPack struct {
	pack   *kn.Pack
	origin *ship.Origin
	// note says why a pack has no origin.
	note string
	done func()
}

// Shipper lays the packs a QuickAgent ships into its folder, from the
// stores this Env reaches, for the QuickAgent being published to a
// registry.
type Shipper struct{ Env Env }

var _ build.Shipper = Shipper{}

// Lay puts src's files under dir/knowledge/<name>/ and records its origin.
func (s Shipper) Lay(ctx context.Context, reg, dir string, src build.ShipSource) (*build.Laid, error) {
	m, err := build.ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	p, err := s.Env.openForShip(ctx, reg, m.Namespace, src)
	if err != nil {
		return nil, err
	}
	defer p.done()
	if err := ship.Check(p.pack); err != nil {
		return nil, err
	}
	rec, err := ship.ReadRecord(dir)
	if err != nil {
		return nil, err
	}
	laid, err := ship.Lay(dir, p.pack)
	if err != nil {
		return nil, err
	}
	e := ship.Entry{Name: p.pack.Manifest.Name, SHA256: laid.SHA256}
	if p.origin != nil {
		e.Origin, e.Registry = p.origin, settings.Normalize(reg)
	}
	rec.Set(e)
	if err := ship.WriteRecord(dir, rec); err != nil {
		return nil, err
	}
	return &build.Laid{Bytes: laid.Bytes, Files: laid.Files, Origin: originOf(p.origin)}, nil
}

// Prune takes out the packs laid in dir that are not in keep, with their
// records. A folder under knowledge/ that has no record was not laid here and
// stays.
func (s Shipper) Prune(dir string, keep []string) error {
	rec, err := ship.ReadRecord(dir)
	if err != nil {
		return err
	}
	for _, e := range slices.Clone(rec.Packs) {
		if slices.Contains(keep, e.Name) {
			continue
		}
		if err := ship.Remove(dir, e.Name); err != nil {
			return err
		}
		rec.Remove(e.Name)
	}
	return ship.WriteRecord(dir, rec)
}

// Origins are the origins to claim for the packs laid in dir, when it is
// published to reg: a pack whose files are no longer the ones laid claims
// none.
func (s Shipper) Origins(dir, reg string) (map[string]*build.PackOrigin, error) {
	rec, err := ship.ReadRecord(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]*build.PackOrigin{}
	for _, e := range rec.Packs {
		if o, _ := ship.OriginOf(dir, rec, e.Name, settings.Normalize(reg)); o != nil {
			out[e.Name] = originOf(o)
		}
	}
	return out, nil
}

func originOf(o *ship.Origin) *build.PackOrigin {
	if o == nil {
		return nil
	}
	return &build.PackOrigin{Kind: o.Kind, Org: o.Org, PackID: o.PackID}
}

// ownOrigin is the origin to record for one of the person's own kept packs:
// its copy in Agentry, when it has one at this registry that is the pack as
// it stands. Without one it says why none is recorded, and the release ships
// the pack as the person's own files.
func ownOrigin(reg string, e store.Entry) (*ship.Origin, string) {
	switch a := e.Key.Agentry; {
	case a == nil:
		return nil, fmt.Sprintf("%s is kept only on this computer, so the release ships it as your own files with no origin recorded. Copy it to Agentry first and ship it again to record the pack in Agentry as its origin.", e.Name())
	case settings.Normalize(a.Registry) != settings.Normalize(reg):
		return nil, fmt.Sprintf("%s's copy in Agentry is at %s, not %s, so no origin is recorded.", e.Name(), a.Registry, settings.Normalize(reg))
	case a.Digest != e.Manifest.SHA256:
		return nil, fmt.Sprintf("%s changed on this computer since its copy to Agentry, so no origin is recorded: the release ships it as your own files. Copy it to Agentry again and ship it again to record the origin.", e.Name())
	default:
		return &ship.Origin{Kind: ship.KindOwn, PackID: a.ID}, ""
	}
}

// openForShip opens the pack src names, in a scratch folder of the store, with the
// origin to record for it. The person's own pack is the one kept here (or
// their copy in Agentry, downloaded first); an organization's is read through
// the registry's export, which answers an administrator, after the two rules
// of shipping it are checked: the person administers the organization and the
// QuickAgent is in its namespace.
func (e Env) openForShip(ctx context.Context, reg, namespace string, src build.ShipSource) (*shipPack, error) {
	if err := kn.ValidName(src.Name); err != nil {
		return nil, err
	}
	s, err := e.Store(reg)
	if err != nil {
		return nil, err
	}
	switch src.Kind {
	case build.ShipOwn:
		en, err := s.Find(src.Name)
		if errors.Is(err, store.ErrNotFound) {
			if !e.SignedIn(reg) {
				return nil, fmt.Errorf("the knowledge pack %s is not kept on this computer, and you are not signed in to %s to download your copy: %w", src.Name, hostOf(settings.Normalize(reg)), store.ErrSignedOut)
			}
			if en, err = s.Download(ctx, src.Name); err != nil {
				return nil, fmt.Errorf("download your copy of %s from Agentry: %w", src.Name, err)
			}
		} else if err != nil {
			return nil, err
		}
		if en.Key.Org != "" {
			return nil, fmt.Errorf("%s is %s's shared pack, not one of yours", src.Name, en.Key.Org)
		}
		p, done, err := s.OpenScratch(ctx, src.Name)
		if err != nil {
			return nil, err
		}
		sp := &shipPack{pack: p, done: done}
		sp.origin, sp.note = ownOrigin(reg, en)
		return sp, nil
	case build.ShipOrg:
		return e.openOrgForShip(ctx, s, reg, namespace, src)
	}
	return nil, fmt.Errorf("%q is not a kind of pack to ship", src.Kind)
}

func (e Env) openOrgForShip(ctx context.Context, s *store.Store, reg, namespace string, src build.ShipSource) (*shipPack, error) {
	org := src.Org
	if namespace != org {
		return nil, fmt.Errorf("%s's pack ships only with a QuickAgent in %s's own namespace, and this one is in %s", org, org, namespace)
	}
	reg = settings.Normalize(reg)
	orgs, err := e.memberships(ctx, reg)
	if err != nil {
		return nil, fmt.Errorf("read your organizations: %w", err)
	}
	if !slices.ContainsFunc(orgs, func(m Membership) bool { return m.Org == org && m.Admin }) {
		return nil, fmt.Errorf("only an administrator of %s can ship its knowledge pack", org)
	}
	sh, err := e.shared(reg, org)
	if err != nil {
		return nil, err
	}
	packs, err := sh.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list %s's knowledge packs: %w", org, err)
	}
	i := slices.IndexFunc(packs, func(r store.Remote) bool { return r.Name == src.Name })
	if i < 0 {
		return nil, fmt.Errorf("%s shares no knowledge pack named %s in Agentry", org, src.Name)
	}
	r := packs[i]
	if r.State != store.StateReady {
		return nil, fmt.Errorf("%s's pack %s is %s in Agentry: it ships once its scan has finished", org, src.Name, r.State)
	}
	tree, digest, err := sh.Export(ctx, r.ID)
	if err != nil {
		return nil, fmt.Errorf("export %s's pack %s: %w", org, src.Name, err)
	}
	if digest == "" || digest != r.Digest {
		return nil, fmt.Errorf("%s's pack %s: the tree Agentry sent is not the one it describes", org, src.Name)
	}
	scratch, done, err := s.Scratch()
	if err != nil {
		return nil, err
	}
	p, err := kn.Unpack(bytes.NewReader(tree), digest, filepath.Join(scratch, src.Name),
		kn.Manifest{Name: r.Name, Created: r.Created.UTC().Truncate(time.Second), Updated: r.Updated.UTC().Truncate(time.Second)})
	if err != nil {
		done()
		return nil, fmt.Errorf("%s's pack %s: %w", org, src.Name, err)
	}
	return &shipPack{pack: p, done: done, origin: &ship.Origin{Kind: ship.KindOrg, Org: org, PackID: r.ID}}, nil
}

// ShipIns are the packs a draft ships, each among the packs the draft reads
// and kept on this computer: the whole pack as the person chose it, held to
// what a release ships, with the origin it claims. The draft copies them
// into its package from here, not from the copy the Drafter read, which has
// what AI Review withheld cut out of it.
func (d *DraftPacks) ShipIns(e Env, reg string, names []string) ([]drafter.ShipIn, error) {
	if len(names) == 0 {
		return nil, nil
	}
	if len(names) > ship.MaxPacks {
		return nil, fmt.Errorf("a release ships at most %d knowledge packs", ship.MaxPacks)
	}
	s, err := e.Store(reg)
	if err != nil {
		return nil, err
	}
	var out []drafter.ShipIn
	for _, n := range names {
		i := slices.IndexFunc(d.Packs, func(k drafter.KnowledgeIn) bool { return k.Pack.Manifest.Name == n })
		if i < 0 {
			return nil, fmt.Errorf("the draft ships %s, which is not among the packs it reads", n)
		}
		p := d.Packs[i].Pack
		if err := ship.Check(p); err != nil {
			return nil, err
		}
		en, err := s.Find(n)
		if err != nil {
			return nil, err
		}
		if en.Key.Org != "" {
			return nil, fmt.Errorf("%s is %s's shared pack, which ships from the Knowledge section once the draft is made", n, en.Key.Org)
		}
		o, _ := ownOrigin(reg, en)
		out = append(out, drafter.ShipIn{Pack: p, Origin: o})
	}
	return out, nil
}
