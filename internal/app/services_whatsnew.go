// What's new: the highlights of the versions Agentry Desktop moved through
// since the person last looked, shown once on the first start at a newer
// version, and again from Settings on request.
//
// Everything here is read from the highlights compiled into this build
// (internal/highlights) and from desktop.yaml; nothing reaches the network.
// The one thing written is the last version seen, and only by this app: an
// installer of a newer version never touches configuration.
package app

import (
	"errors"
	"os"

	"github.com/blaktron/agentry-desktop/internal/highlights"
	"github.com/blaktron/agentry-desktop/internal/settings"
	"github.com/blaktron/agentry-desktop/internal/version"
)

const (
	// whatsNewCards is how many versions the screen shows at most; the
	// rest are counted, and the last card links to every release's notes.
	whatsNewCards = 5
	// desktopReleasesURL is where every version's notes are.
	desktopReleasesURL = "https://github.com/blaktron/agentry-desktop/releases"
)

// WhatsNewCard is one version's highlights.
type WhatsNewCard struct {
	Version    string   `json:"version"`
	Highlights []string `json:"highlights"`
}

// WhatsNewView is what the What's new screen shows: its cards newest first,
// none when there is nothing to show, and how many older versions there
// were beyond them.
type WhatsNewView struct {
	// Current is the version running now, X.Y.Z, or "" for a build that
	// names no release.
	Current     string         `json:"current"`
	Cards       []WhatsNewCard `json:"cards"`
	Earlier     int            `json:"earlier"`
	ReleasesURL string         `json:"releasesUrl"`
}

// WhatsNewAPI is the What's new screen's bindings.
type WhatsNewAPI struct {
	// settingsExisted says desktop.yaml was there when the app started:
	// without it, this is a fresh install, which has nothing new to show.
	settingsExisted bool
	version         string
	entries         func() ([]highlights.Entry, error)
}

// newWhatsNewAPI looks for desktop.yaml now, before anything this start
// writes it, so that a fresh install can be told from an installation whose
// settings record no last-seen version.
func newWhatsNewAPI() *WhatsNewAPI {
	existed := false
	if path, err := settings.Path(); err == nil {
		_, err := os.Stat(path)
		existed = err == nil || !errors.Is(err, os.ErrNotExist)
	}
	return &WhatsNewAPI{settingsExisted: existed, version: version.Version, entries: highlights.All}
}

func whatsNewView(current string, entries []highlights.Entry, earlier int) *WhatsNewView {
	v := &WhatsNewView{Current: current, Cards: []WhatsNewCard{}, Earlier: earlier, ReleasesURL: desktopReleasesURL}
	for _, e := range entries {
		v.Cards = append(v.Cards, WhatsNewCard{Version: e.Version, Highlights: e.Highlights})
	}
	return v
}

// Pending is what the screen shows on this start: a card for every version
// newer than the last one seen, up to the running one, at most
// whatsNewCards of them. A fresh install shows nothing, and an installation
// whose settings record no last-seen version shows the running version's
// card alone, since which version it came from is not known. When there is nothing to show,
// the running version is recorded as seen.
func (w *WhatsNewAPI) Pending() (*WhatsNewView, error) {
	current, ok := highlights.Version(w.version)
	if !ok {
		return whatsNewView("", nil, 0), nil
	}
	entries, err := w.entries()
	if err != nil {
		return nil, err
	}
	st, err := settings.Load()
	if err != nil {
		return nil, err
	}
	var shown []highlights.Entry
	earlier := 0
	switch seen := st.WhatsNewSeen; {
	case seen == "" && !w.settingsExisted:
	case seen == "":
		if e, ok := highlights.For(entries, current); ok {
			shown = []highlights.Entry{e}
		}
	case highlights.Compare(seen, current) < 0:
		shown, earlier = highlights.Between(entries, seen, current, whatsNewCards)
	}
	if len(shown) == 0 && st.WhatsNewSeen != current {
		if err := settings.SetWhatsNewSeen(current); err != nil {
			return nil, err
		}
	}
	return whatsNewView(current, shown, earlier), nil
}

// Recent is what Settings' What's new shows: the newest versions up to the
// running one. Nothing is recorded.
func (w *WhatsNewAPI) Recent() (*WhatsNewView, error) {
	current, ok := highlights.Version(w.version)
	if !ok {
		return whatsNewView("", nil, 0), nil
	}
	entries, err := w.entries()
	if err != nil {
		return nil, err
	}
	shown, earlier := highlights.Between(entries, "", current, whatsNewCards)
	return whatsNewView(current, shown, earlier), nil
}

// Dismiss records the running version as seen, so the screen does not show
// again until the app moves to a newer one. Done, Dismiss and Escape all end
// here.
func (w *WhatsNewAPI) Dismiss() error {
	current, ok := highlights.Version(w.version)
	if !ok {
		return nil
	}
	return settings.SetWhatsNewSeen(current)
}
