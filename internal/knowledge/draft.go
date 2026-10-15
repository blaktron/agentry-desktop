package knowledge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blaktron/agentry-cli/pkg/drafter"
	kn "github.com/blaktron/agentry-cli/pkg/knowledge"
	"github.com/blaktron/agentry-cli/pkg/knowledge/store"
)

// DraftPacks are a draft's packs kept on this computer, opened for it: each
// pack as the Drafter's library takes it, with Agentry's scan record where
// the registry confirms one; the scratch directory inside the store that
// drafter.ReadyKnowledge copies the packs into before the Drafter reads
// them; and ScanFirst, the names of the packs AI Review scans with the
// person's key before the draft starts, because no scan in Agentry stands
// in for them. Done removes the opened copies and the scratch.
type DraftPacks struct {
	Packs     []drafter.KnowledgeIn
	Scratch   string
	ScanFirst []string
	Done      func()
}

// Names are the packs' names, as the Drafter's task lists them.
func (d *DraftPacks) Names() []string {
	if d == nil {
		return nil
	}
	out := make([]string, 0, len(d.Packs))
	for _, p := range d.Packs {
		out = append(out, p.Pack.Manifest.Name)
	}
	return out
}

// ForDraft opens each named pack kept on this computer for a draft, in the
// order of `agentry draft --knowledge`: every pack opened and the whole held
// to the Drafter's bound (drafter.MaxKnowledgePacks, MaxKnowledgeBytes)
// before any registry is asked about one; then each pack's scan in Agentry,
// confirmed with the registry that holds it (store.AgentryScan). A pack kept
// only here has none, and one whose record cannot be confirmed is treated
// as not scanned, saying why. A pack that is not kept or will not open is
// refused, naming it. Nothing is scanned here: Run scans what ScanFirst
// names, after the description and with the person's key.
func (e Env) ForDraft(ctx context.Context, current string, names []string) (_ *DraftPacks, err error) {
	d := &DraftPacks{}
	var cleanups []func()
	d.Done = func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
	defer func() {
		if err != nil {
			d.Done()
		}
	}()
	if len(names) == 0 {
		return d, nil
	}
	s, err := e.Store(current)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var packs []*kn.Pack
	for _, name := range names {
		if err := kn.ValidName(name); err != nil {
			return nil, fmt.Errorf("draft with knowledge: %w", err)
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		if len(seen) > drafter.MaxKnowledgePacks {
			return nil, fmt.Errorf("a draft reads at most %d knowledge packs", drafter.MaxKnowledgePacks)
		}
		p, cleanup, err := s.OpenScratch(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("draft with knowledge: %w", err)
		}
		cleanups = append(cleanups, cleanup)
		packs = append(packs, p)
	}
	if err := drafter.CheckKnowledgeSize(packs); err != nil {
		return nil, err
	}
	for _, p := range packs {
		in := drafter.KnowledgeIn{Pack: p}
		rec, err := s.AgentryScan(ctx, p.Manifest.Name, p.Manifest.SHA256)
		switch {
		case err == nil:
			in.Agentry = rec
		case errors.Is(err, store.ErrScanUnconfirmed):
			in.Unconfirmed = strings.TrimPrefix(err.Error(), "knowledge pack "+p.Manifest.Name+": ")
		default:
			return nil, err
		}
		if ok, _ := drafter.AgentryScanStandsIn(p, in.Agentry); !ok {
			d.ScanFirst = append(d.ScanFirst, p.Manifest.Name)
		}
		d.Packs = append(d.Packs, in)
	}
	scratch, done, err := s.Scratch()
	if err != nil {
		return nil, err
	}
	cleanups = append(cleanups, done)
	d.Scratch = scratch
	return d, nil
}
