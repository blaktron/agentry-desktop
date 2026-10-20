package app

// Make public for a QuickAgent whose versions ship knowledge packs: everyone
// can then download what they ship, so the publisher confirms each pack
// first, in a native dialog the page cannot answer, and the confirmation goes
// to the registry with the visibility change.

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"

	"github.com/blaktron/agentry/pkg/apiclient"
	"github.com/blaktron/agentry/pkg/manifest"

	"github.com/blaktron/agentry-desktop/internal/build"
)

// shippedKnowledgeSlug is the problem the registry answers Make public with
// while a version ships a pack the publisher has not confirmed.
const shippedKnowledgeSlug = "shipped-knowledge-unacknowledged"

// shippingVersion is a version that ships packs, as the registry names it.
type shippingVersion struct {
	Version string   `json:"version"`
	Packs   []string `json:"packs"`
}

// shippedConflict reads the versions and packs out of the registry's refusal.
func shippedConflict(body []byte) []shippingVersion {
	var p struct {
		Versions []shippingVersion `json:"versions"`
	}
	if json.Unmarshal(body, &p) != nil {
		return nil
	}
	return p.Versions
}

// shippedPack is a pack that some versions ship.
type shippedPack struct {
	name     string
	versions []string
}

// packsOf turns versions that ship packs into packs and the versions that
// ship each, in name order.
func packsOf(vs []shippingVersion) []shippedPack {
	by := map[string][]string{}
	for _, v := range vs {
		for _, n := range v.Packs {
			if !slices.Contains(by[n], v.Version) {
				by[n] = append(by[n], v.Version)
			}
		}
	}
	out := make([]shippedPack, 0, len(by))
	for n, versions := range by {
		out = append(out, shippedPack{name: n, versions: versions})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// publisherShipsUnnamed is what Make public says when the registry asks for
// the packs' confirmation and names none: a sentence for the person.
func publisherShipsUnnamed() string {
	return publisherNothingChanged + " the registry asks you to confirm the knowledge packs it ships, and did not say which. Try again in a moment."
}

// confirmPublicShips asks the confirmation of each pack the QuickAgent's
// versions ship, naming the versions that ship it and its size where the
// registry's file listing of the newest of them gives one. It returns false
// at the first No. A refusal that names no pack is not a confirmation to
// give: it is an error.
func (p *PublisherAPI) confirmPublicShips(ctx context.Context, client *apiclient.ClientWithResponses, ns, name string, vs []shippingVersion) (bool, error) {
	packs := packsOf(vs)
	if len(packs) == 0 {
		return false, errors.New(publisherShipsUnnamed())
	}
	for _, pk := range packs {
		// The size is read from the first version whose files the registry
		// lists; each ships the pack by name.
		size := int64(-1)
		for _, v := range pk.versions {
			if n, ok := shippedPackSize(ctx, client, ns, name, pk.name, v); ok {
				size = n
				break
			}
		}
		c := build.ShipConfirmation(pk.name, size, false, pk.versions)
		if err := p.app.askConsent("Make the knowledge pack "+pk.name+" public?", c.Text()+"\n\nMake "+ns+"/"+name+" public, with it?"); err != nil {
			if errors.Is(err, errNotConfirmed) {
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
}

// shippedPackSize is the size of the pack's wiki and sources in a published
// version, from the registry's listing of the files in its archive, as the
// install consent counts it.
func shippedPackSize(ctx context.Context, client *apiclient.ClientWithResponses, ns, name, pack, version string) (int64, bool) {
	res, err := client.ListVersionFilesWithResponse(ctx, ns, name, version)
	if err != nil || res.JSON200 == nil {
		return 0, false
	}
	var total int64
	for _, f := range res.JSON200.Files {
		if _, ok := manifest.ShippedPackFile([]string{pack}, f.Path); ok {
			total += f.Size
		}
	}
	return total, true
}
