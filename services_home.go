package main

import "github.com/blaktron/agentry-desktop/internal/settings"

// HomeAPI is the Home page's layout (plan desktop-home-and-library.md §4.4,
// D-067; agentry-desktop#658): which cards Home draws, in what order, in
// desktop.yaml. Go owns the file: the page reads the layout and asks for
// one change at a time, and never writes the layout whole. Pinning (M4)
// joins it with its own placement rules.
type HomeAPI struct{}

// HomeLayoutView is the layout as the page draws it. Newer says the stored
// layout was arranged by a newer Agentry Desktop, Unreadable that it is a
// value this app cannot decode: either way the defaults are drawn, and the
// stored one is kept as it is.
type HomeLayoutView struct {
	Cards      []settings.HomeCard `json:"cards"`
	Newer      bool                `json:"newer"`
	Unreadable bool                `json:"unreadable"`
}

func homeView() (*HomeLayoutView, error) {
	l, newer, unreadable, err := settings.HomeLayoutNow()
	if err != nil {
		return nil, err
	}
	return &HomeLayoutView{Cards: l.Cards, Newer: newer, Unreadable: unreadable}, nil
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
