package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// HomeAPI is the Home page's layout: which cards Home draws, in what order,
// in desktop.yaml. Go owns the file: the page reads the layout and asks for
// one change at a time, and never writes the layout whole. Pins are Go's
// too, with their placement rules.
type HomeAPI struct{}

// HomeLayoutView is the layout as the page draws it. Newer says the stored
// layout was arranged by a newer Agentry Desktop, Unreadable that it is a
// value this app cannot decode: either way the defaults are drawn, and the
// stored one is kept as it is.
type HomeLayoutView struct {
	Cards      []settings.HomeCard `json:"cards"`
	Newer      bool                `json:"newer"`
	Unreadable bool                `json:"unreadable"`
	// MaxPins and PinsPerCard are the limits Go enforces, so the page says
	// them rather than its own copy.
	MaxPins     int `json:"maxPins"`
	PinsPerCard int `json:"pinsPerCard"`
	// MaxCols and MaxRows are the grid's bounds a card is sized within.
	MaxCols int `json:"maxCols"`
	MaxRows int `json:"maxRows"`
}

func homeView() (*HomeLayoutView, error) {
	l, newer, unreadable, err := settings.HomeLayoutNow()
	if err != nil {
		return nil, err
	}
	return &HomeLayoutView{Cards: l.Cards, Newer: newer, Unreadable: unreadable, MaxPins: settings.MaxPins, PinsPerCard: settings.PinsPerCard, MaxCols: settings.HomeMaxCols, MaxRows: settings.HomeMaxRows}, nil
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

// Move puts the card at from, of kind, at to in Home's order. The index is
// the layout's, hidden cards included; kind is the card the page drew there.
func (h *HomeAPI) Move(from int, kind string, to int) (*HomeLayoutView, error) {
	if err := settings.MoveHomeCard(from, kind, to); err != nil {
		return nil, err
	}
	return homeView()
}

// Resize sets the card at index, of kind, to cols wide and rows tall on
// Home's grid.
func (h *HomeAPI) Resize(index int, kind string, cols, rows int) (*HomeLayoutView, error) {
	if err := settings.SizeHomeCard(index, kind, cols, rows); err != nil {
		return nil, err
	}
	return homeView()
}

// Theme is the theme the app is drawn in: one of settings.Themes.
func (h *HomeAPI) Theme() (string, error) { return settings.ThemeNow() }

// SetTheme keeps the person's theme in desktop.yaml; the page applies it.
func (h *HomeAPI) SetTheme(name string) error { return settings.SetTheme(name) }

// ThemeScript serves /theme.js, which index.html loads before the first
// paint: one statement that puts the stored theme on the root element, or
// nothing for System. The window's policy runs no inline script, so the
// theme comes from the app's own origin like the bundle. Only a name in
// settings.Themes is ever written into the script; anything else, and any
// other path, is not served.
func ThemeScript() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/theme.js" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		theme, err := settings.ThemeNow()
		if err != nil || theme == settings.ThemeSystem || !slices.Contains(settings.Themes, theme) {
			return
		}
		_, _ = fmt.Fprintf(w, "document.documentElement.dataset.theme = %q;\n", theme)
	})
}

// Reset puts Home back to the default cards.
func (h *HomeAPI) Reset() (*HomeLayoutView, error) {
	if err := settings.ResetHome(); err != nil {
		return nil, err
	}
	return homeView()
}

// Pin pins an installed QuickAgent to Home: ref is namespace/name, registry
// the record's origin, "" for one installed from a folder. Only a QuickAgent
// on this computer can be pinned. It writes only desktop.yaml and asks no
// registry: a pin is not a star.
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
		if !it.Present || refOf(it) != ref {
			continue
		}
		if it.Local {
			if registry == "" {
				return true, nil
			}
		} else if registry != "" && settings.Normalize(it.Origin) == settings.Normalize(registry) {
			return true, nil
		}
	}
	return false, nil
}

// refOf is a record's QuickAgent as a pin names it, namespace/name.
func refOf(it install.Item) string { return it.Namespace + "/" + it.Name }

// unpinIfGone removes ref's pins once nothing of that kind of it is left on
// this computer, after an uninstall or a Forget in this app: the folder pin
// when no folder install of it is left, the registry pins when no install
// from a registry is. An uninstall by the CLI leaves the pin, which Home
// then shows as no longer installed, with Unpin. It never fails the removal
// that called it.
func unpinIfGone(ref string) {
	items, err := install.List()
	if err != nil {
		slog.Warn("home: pins not checked after a removal", "ref", ref, "err", err)
		return
	}
	fromFolder, fromRegistry := false, false
	for _, it := range items {
		if it.Present && refOf(it) == ref {
			fromFolder = fromFolder || it.Local
			fromRegistry = fromRegistry || !it.Local
		}
	}
	for _, folder := range []bool{true, false} {
		if (folder && fromFolder) || (!folder && fromRegistry) {
			continue
		}
		if err := settings.UnpinRef(ref, folder); err != nil {
			slog.Warn("home: the pin of a removed QuickAgent was not removed", "ref", ref, "err", err)
		}
	}
}

// PinScheduled is the Scheduled page's Pin to Home: the Scheduled card,
// Large, at the end of Home, or a hidden one shown again. It is a card, not
// one of the QuickAgent pins.
func (h *HomeAPI) PinScheduled() (*HomeLayoutView, error) {
	if err := settings.PinScheduledToHome(); err != nil {
		return nil, err
	}
	return homeView()
}

// UnpinScheduled removes the Scheduled card from Home.
func (h *HomeAPI) UnpinScheduled() (*HomeLayoutView, error) {
	if err := settings.UnpinScheduledFromHome(); err != nil {
		return nil, err
	}
	return homeView()
}
