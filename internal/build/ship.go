package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/blaktron/agentry/pkg/manifest"
)

// Shipping a knowledge pack with a QuickAgent. The pack goes into the
// release's archive under knowledge/<name>/, unencrypted, so everyone who
// can download the release can read it. The form lists the packs the person
// may ship (ShipOption), keeps the ticked ones (Model.KnowledgeShips and
// Model.Ships), lays them into the workspace at every save through a
// Shipper, and sends the publisher's confirmation of each one with the
// upload (KnowledgeShipping).

// MaxShips is how many packs one release ships.
const MaxShips = manifest.MaxShippedPacks

// ShipSource is a stored pack the form ships: which one, and where the
// person keeps it. The pack is read again from its store at every save, so
// a version ships the pack as it is now.
type ShipSource struct {
	// Name is the pack's name, which is also its folder under knowledge/.
	Name string `json:"name"`
	// Kind is "own" for one of the person's packs and "org" for an
	// organization's shared pack, which Org names.
	Kind string `json:"kind"`
	Org  string `json:"org,omitempty"`
	// ID is the pack's id where it is kept on this computer, AgentryID its
	// id in Agentry; either may be empty.
	ID        string `json:"id,omitempty"`
	AgentryID string `json:"agentryId,omitempty"`
	// Bytes and Pages are what the list said of it when it was ticked.
	Bytes int64 `json:"bytes"`
	Pages int   `json:"pages"`
}

// The kinds of ShipSource.
const (
	ShipOwn = "own"
	ShipOrg = "org"
)

// ShipOption is a pack of the person's the form can offer: a pack the
// Knowledge page lists, with what decides whether it may ship.
type ShipOption struct {
	ShipSource
	// Ready says the pack has been read by AI Review, here or in Agentry.
	Ready bool `json:"ready"`
	// OrgAdmin says the person administers Org.
	OrgAdmin bool `json:"orgAdmin"`
}

// Reason is why the pack cannot ship with a QuickAgent in namespace, in a
// sentence for its row, or "" when it can. The rules are the registry's:
// only a ready pack, within the pack's caps, and an organization's pack
// only from an administrator of that organization into a QuickAgent in its
// own namespace.
func (o ShipOption) Reason(namespace string) string {
	switch {
	case o.Kind == ShipOrg && !o.OrgAdmin:
		return "only an administrator of " + o.Org + " can ship its pack"
	case o.Kind == ShipOrg && namespace != o.Org:
		return "ships only into a QuickAgent in the " + o.Org + " namespace, and this one is in " + namespaceWord(namespace)
	case !o.Ready:
		return "wait until AI Review has read it, then it can ship"
	case o.Bytes > manifest.MaxKnowledgePackBytes:
		return fmt.Sprintf("it holds %s, and a release ships at most %s of one pack", PackSize(o.Bytes), PackSize(manifest.MaxKnowledgePackBytes))
	}
	return ""
}

func namespaceWord(ns string) string {
	if strings.TrimSpace(ns) == "" {
		return "no namespace yet"
	}
	return ns
}

// shipSource is the source of a pack the form ships; a name with none is a
// pack whose files are the workspace's own.
func (m *Model) shipSource(name string) (ShipSource, bool) {
	for _, s := range m.Ships {
		if s.Name == name {
			return s, true
		}
	}
	return ShipSource{}, false
}

// TickShip adds a pack to what the form ships, or refreshes its source if it
// is there. It refuses a sixth pack.
func (m *Model) TickShip(src ShipSource) error {
	names := compact(m.KnowledgeShips)
	if !slices.Contains(names, src.Name) {
		if len(names) >= MaxShips {
			return fmt.Errorf("a release ships at most %d knowledge packs", MaxShips)
		}
		names = append(names, src.Name)
	}
	m.KnowledgeShips = names
	m.Ships = append(slices.DeleteFunc(slices.Clone(m.Ships), func(s ShipSource) bool { return s.Name == src.Name }), src)
	// An include list that came with the workspace has to pack the pack too;
	// the default list does already.
	if inc := compact(m.PublishInclude); len(inc) > 0 && !slices.Contains(inc, shipsGlob) {
		m.PublishInclude = append(inc, shipsGlob)
	}
	return nil
}

// UntickShip takes a pack out of what the form ships.
func (m *Model) UntickShip(name string) {
	m.KnowledgeShips = slices.DeleteFunc(compact(m.KnowledgeShips), func(n string) bool { return n == name })
	m.Ships = slices.DeleteFunc(slices.Clone(m.Ships), func(s ShipSource) bool { return s.Name == name })
	// With no pack left to ship, the glob that packed them goes too: files
	// left under knowledge/ belong to no shipped pack.
	if len(m.KnowledgeShips) == 0 {
		m.PublishInclude = slices.DeleteFunc(compact(m.PublishInclude), func(g string) bool { return g == shipsGlob })
	}
}

// ReconcileShips ties the packs a loaded release shipped to the packs the
// person can ship now, as Edit does: a name that matches an option the
// person may ship in this namespace stays ticked and is read again from its
// store; any other is unticked and named in the notes, never carried
// silently. The model's ships are replaced.
func (m *Model) ReconcileShips(options []ShipOption) (notes []string) {
	kept := []string{}
	m.Ships = nil
	for _, name := range compact(m.KnowledgeShips) {
		var found *ShipOption
		for i := range options {
			if options[i].Name == name {
				// Prefer a pack the person may ship in this namespace over
				// one of the same name they may not.
				if found == nil || (found.Reason(m.Namespace) != "" && options[i].Reason(m.Namespace) == "") {
					found = &options[i]
				}
			}
		}
		switch {
		case found == nil:
			notes = append(notes, fmt.Sprintf("The previous version shipped the knowledge pack %s. It is not among your packs now, so it is unticked and this version will not ship it.", name))
		case found.Reason(m.Namespace) != "":
			notes = append(notes, fmt.Sprintf("The previous version shipped the knowledge pack %s. It cannot ship now (%s), so it is unticked.", name, found.Reason(m.Namespace)))
		default:
			kept = append(kept, name)
			m.Ships = append(m.Ships, found.ShipSource)
		}
	}
	m.KnowledgeShips = kept
	return notes
}

// KeepChosenShips holds a draft's ships to the packs its person chose to
// ship: a ship the draft lists that is not among chosen is dropped and named,
// and a chosen pack the draft left out is added and named, so the draft ships
// exactly what the person chose. It returns the notes.
func (m *Model) KeepChosenShips(chosen []ShipSource) (notes []string) {
	chosenBy := map[string]ShipSource{}
	for _, c := range chosen {
		chosenBy[c.Name] = c
	}
	m.Ships = nil
	var kept []string
	for _, name := range compact(m.KnowledgeShips) {
		src, ok := chosenBy[name]
		if !ok {
			notes = append(notes, fmt.Sprintf("The draft listed the knowledge pack %s to ship, which you did not choose to ship, so it was left out.", name))
			continue
		}
		kept = append(kept, name)
		m.Ships = append(m.Ships, src)
	}
	for _, c := range chosen {
		if !slices.Contains(kept, c.Name) {
			kept = append(kept, c.Name)
			m.Ships = append(m.Ships, c)
			notes = append(notes, fmt.Sprintf("The draft did not list the knowledge pack %s, which you chose to ship, so it was added. Its instructions may not mention it: read them before you submit.", c.Name))
		}
	}
	m.KnowledgeShips = kept
	return notes
}

// Confirmation is the publisher's confirmation for one shipped pack, in two
// parts: what happens to it and what to check.
type Confirmation struct {
	// Lead names the pack and its size and says who can read it.
	Lead string `json:"lead"`
	// Rest says what that means and what to check.
	Rest string `json:"rest"`
}

// Text is the confirmation as one paragraph.
func (c Confirmation) Text() string { return c.Lead + " " + c.Rest }

// ShipConfirmation is the confirmation asked for one pack before it is sent
// with a release, and before a QuickAgent whose versions ship it becomes
// public. bytes is the pack's size, or negative where it is not known.
// private says the QuickAgent is and stays private, so the readers are those
// who can see it. versions names the published versions that already ship the
// pack (Make public); none means the release being submitted.
func ShipConfirmation(name string, bytes int64, private bool, versions []string) Confirmation {
	size := ""
	if bytes >= 0 {
		size = " (" + PackSize(bytes) + ")"
	}
	release, stays := "this release", "this version"
	switch len(versions) {
	case 0:
	case 1:
		release, stays = "version "+versions[0], "that version"
	default:
		release, stays = "versions "+strings.Join(versions, ", "), "those versions"
	}
	c := Confirmation{Rest: "Check that it holds no private data: names, emails, customer records, internal hostnames, keys."}
	recall := "retiring or deleting the QuickAgent later does not recall copies already downloaded. "
	if private {
		c.Lead = "Everyone who can see this QuickAgent can read " + name + size + "."
		c.Rest = "They can download it with " + release + ", and it stays in " + stays + " for good: " + recall + c.Rest
		return c
	}
	c.Lead = name + size + " becomes public."
	c.Rest = "Anyone can download it with " + release + ", and it stays in " + stays + " for good: " + recall + c.Rest
	return c
}

// PackSize is a pack's size as the Knowledge pages count it, in binary
// units: "512 B", "1.5 KB", "10 MB".
func PackSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB"}
	v, i := float64(n)/1024, 0
	for fmt.Sprintf("%.1f", v) == "1024.0" || v >= 1024 {
		if i == len(units)-1 {
			break
		}
		v /= 1024
		i++
	}
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0") + " " + units[i]
}

// PackOrigin is the stored pack a shipped pack was laid from. The registry
// holds the claim to the facts, so a pack laid from a copy the store no
// longer matches carries none.
type PackOrigin struct {
	Kind string `json:"kind"`
	Org  string `json:"org,omitempty"`
	// PackID is the registry's id for the stored pack.
	PackID string `json:"packId"`
}

// ShippedPack is one entry of the acknowledgement.
type ShippedPack struct {
	Name   string      `json:"name"`
	Origin *PackOrigin `json:"origin,omitempty"`
}

// KnowledgeShipping is the publisher's confirmation sent with a release
// that ships packs: one entry for each, no more and no fewer.
type KnowledgeShipping struct {
	Packs []ShippedPack `json:"packs"`
}

// Laid is what a Shipper put in the workspace for one pack.
type Laid struct {
	Bytes int64
	Files int
	// Origin is where the files came from, nil when the store's copy is not
	// one the registry holds.
	Origin *PackOrigin
}

// Shipper lays stored packs into a workspace and remembers where each one
// came from. The form reads a pack again at every save, so what a version
// ships is the pack as it stands when the person submits.
type Shipper interface {
	// Lay puts src's files under dir/knowledge/<name>/, replacing what was
	// there, with the manifest.json the registry's rules read, and records
	// where they came from. It refuses a pack the person may not ship.
	Lay(ctx context.Context, dir string, src ShipSource) (*Laid, error)
	// Prune removes the packs Lay put in dir that are not in keep, with
	// their records. A folder under knowledge/ that Lay did not put there is
	// left alone.
	Prune(dir string, keep []string) error
	// Origins is the record of where each pack Lay put in dir came from.
	Origins(dir string) (map[string]*PackOrigin, error)
}

// ErrNoShipper is the answer where no Shipper is set up.
var ErrNoShipper = errors.New("shipping a knowledge pack is not available in this build")

// PackTreeSize is the bytes and the number of files of the pack laid at
// dir/knowledge/<name>/: the pack's wiki and sources, the files the registry
// counts against the pack's caps and the consent names the size of. The
// pack's own manifest.json is not among them.
func PackTreeSize(dir, name string) (bytes int64, files int, err error) {
	root := filepath.Join(dir, manifest.KnowledgeDir, name)
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return rerr
		}
		if _, ok := manifest.ShippedPackFile([]string{name}, filepath.ToSlash(rel)); !ok {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return ierr
		}
		bytes += info.Size()
		files++
		return nil
	})
	return bytes, files, err
}

// ShippedNames are the packs the workspace's agent.yaml ships.
func ShippedNames(dir string) ([]string, error) {
	m, err := ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	return m.ShippedPacks(), nil
}

// AcknowledgementFor is the confirmation to send for the packs names, with
// the origins the Shipper recorded for them.
func AcknowledgementFor(names []string, origins map[string]*PackOrigin) *KnowledgeShipping {
	k := &KnowledgeShipping{Packs: make([]ShippedPack, 0, len(names))}
	for _, n := range names {
		k.Packs = append(k.Packs, ShippedPack{Name: n, Origin: origins[n]})
	}
	return k
}

// jsonBytes is what travels in the upload's knowledgeShipping part.
func (k *KnowledgeShipping) jsonBytes() ([]byte, error) { return json.Marshal(k) }
