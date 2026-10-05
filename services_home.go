package main

import (
	"fmt"
	"log/slog"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// HomeAPI is the Home page's layout (plan desktop-home-and-library.md §4.4,
// D-067; agentry-desktop#658): which cards Home draws, in what order, in
// desktop.yaml. Go owns the file: the page reads the layout and asks for
// one change at a time, and never writes the layout whole. Pins
// (agentry-desktop#660) are Go's too, with their placement rules (D5, D11).
type HomeAPI struct{}

// HomeLayoutView is the layout as the page draws it. Newer says the stored
// layout was arranged by a newer Agentry Desktop, Unreadable that it is a
// value this app cannot decode: either way the defaults are drawn, and the
// stored one is kept as it is.
type HomeLayoutView struct {
	Cards      []settings.HomeCard `json:"cards"`
	Newer      bool                `json:"newer"`
	Unreadable bool                `json:"unreadable"`
	// MaxPins is how many pins Home holds (D5), so the Pin button says the
	// limit Go enforces.
	MaxPins int `json:"maxPins"`
}

func homeView() (*HomeLayoutView, error) {
	l, newer, unreadable, err := settings.HomeLayoutNow()
	if err != nil {
		return nil, err
	}
	return &HomeLayoutView{Cards: l.Cards, Newer: newer, Unreadable: unreadable, MaxPins: settings.MaxPins}, nil
}

// Layout reads the layout; it never writes.
func (h *HomeAPI) Layout() (*HomeLayoutView, error) { return homeView() }

// SetShown shows or hides one of the default cards, keeping its place.
func (h *HomeAPI) SetShown(kind string, shown bool) (*HomeLayoutView, error) {
	if err := settings.SetHomeCardShown(kind, shown); err != nil {
		return nil, err
	}
	return homeView()
}

// Reset puts Home back to the default cards.
func (h *HomeAPI) Reset() (*HomeLayoutView, error) {
	if err := settings.ResetHome(); err != nil {
		return nil, err
	}
	return homeView()
}

// Pin pins an installed QuickAgent to Home (plan §4.4, D5, D11;
// agentry-desktop#660): ref is namespace/name, registry the record's
// origin, "" for one installed from a folder. Only a QuickAgent on this
// computer can be pinned. It writes only desktop.yaml and asks no registry:
// a pin is not a star.
func (h *HomeAPI) Pin(ref, registry string) (*HomeLayoutView, error) {
	here, err := installedHere(ref, registry)
	if err != nil {
		return nil, err
	}
	if !here {
		return nil, fmt.Errorf("%s is not installed on this computer, so it cannot be pinned to Home", ref)
	}
	if err := settings.PinToHome(ref, registry); err != nil {
		return nil, err
	}
	return homeView()
}

// Unpin removes a pin; one that is not pinned is left as it is.
func (h *HomeAPI) Unpin(ref, registry string) (*HomeLayoutView, error) {
	if err := settings.UnpinFromHome(ref, registry); err != nil {
		return nil, err
	}
	return homeView()
}

// installedHere reports whether ref from registry is installed on this
// computer: a record whose directory is there, from that registry, or from
// a folder when registry is "".
func installedHere(ref, registry string) (bool, error) {
	items, err := install.List()
	if err != nil {
		return false, err
	}
	for _, it := range items {
		if !it.Present || it.Namespace+"/"+it.Name != ref {
			continue
		}
		if it.Local && registry == "" || !it.Local && registry != "" && settings.Normalize(it.Origin) == settings.Normalize(registry) {
			return true, nil
		}
	}
	return false, nil
}

// unpinIfGone removes ref's pins once nothing of it is left on this
// computer, after an uninstall or a Forget (D5). Another install of the
// same QuickAgent keeps them. It never fails the removal that called it.
func unpinIfGone(ref string) {
	items, err := install.List()
	if err != nil {
		slog.Warn("home: pins not checked after a removal", "ref", ref, "err", err)
		return
	}
	for _, it := range items {
		if it.Present && it.Namespace+"/"+it.Name == ref {
			return
		}
	}
	if err := settings.UnpinRef(ref); err != nil {
		slog.Warn("home: the pin of a removed QuickAgent was not removed", "ref", ref, "err", err)
	}
}
