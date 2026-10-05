package settings

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// The Home page's layout (plan desktop-home-and-library.md §4.4, D-067;
// agentry-desktop#658): an ordered, versioned list of cards under the
// settings file's home key. It is basic on purpose, show or hide a default
// card, and shaped for more: order is the list's, a card carries a size,
// and whatever a card or the layout holds that this app does not know is
// kept through every save, so an older app never destroys what a newer one
// arranged.

// HomeVersion is the layout version this app reads and writes. A layout of
// a higher version is read as the defaults and never written over.
const HomeVersion = 1

// The card kinds this app knows. The page's registry (frontend
// homeCards.ts) draws them; a kind it has no renderer for is skipped.
const (
	HomeRecentRuns  = "recent-runs"
	HomeInstalled   = "installed"
	HomeProfile     = "profile"
	HomeRunDefaults = "run-defaults"
	// HomePinned is a card of pinned QuickAgents, one of several (M4).
	HomePinned = "pinned"
)

// homeDefaults are the default cards, in their order. Each is one card of
// its kind, shown or hidden by kind.
var homeDefaults = []string{HomeRecentRuns, HomeInstalled, HomeProfile, HomeRunDefaults}

// HomeLayout is the stored layout. Rest keeps the keys this app does not
// know.
type HomeLayout struct {
	Version int            `yaml:"version" json:"version"`
	Cards   []HomeCard     `yaml:"cards" json:"cards"`
	Rest    map[string]any `yaml:",inline" json:"-"`

	// raw is a home value this app could not decode (a field of another
	// type, a later shape): kept as it was read and written back unchanged,
	// so it can never make the settings file unreadable, here or for the
	// newer app that wrote it.
	raw *yaml.Node
}

// homeFields is HomeLayout without its methods, for decoding and encoding.
type homeFields HomeLayout

// UnmarshalYAML decodes the layout, or keeps the node whole when it does
// not fit (the review of agentry-desktop#658).
func (l *HomeLayout) UnmarshalYAML(n *yaml.Node) error {
	var f homeFields
	if err := n.Decode(&f); err != nil {
		*l = HomeLayout{raw: n}
		return nil
	}
	*l = HomeLayout(f)
	return nil
}

// MarshalYAML writes back a kept node as it was read.
func (l HomeLayout) MarshalYAML() (any, error) {
	if l.raw != nil {
		return l.raw, nil
	}
	return homeFields(l), nil
}

// HomeCard is one card. Size is "" or "large", the size of the Build,
// Voice and Knowledge cards (D12); smaller sizes come with later
// customising. Rest keeps a card's keys this app does not know.
type HomeCard struct {
	Kind   string `yaml:"kind" json:"kind"`
	Hidden bool   `yaml:"hidden,omitempty" json:"hidden,omitempty"`
	Size   string `yaml:"size,omitempty" json:"size,omitempty"`
	// Pins are a Pinned card's QuickAgents, at most PinsPerCard (D5).
	Pins []HomePin      `yaml:"pins,omitempty" json:"pins,omitempty"`
	Rest map[string]any `yaml:",inline" json:"-"`
}

// HomePin is one pinned QuickAgent (agentry-desktop#660): its ref
// (namespace/name, no version) and the registry it was installed from,
// empty for a QuickAgent installed from a folder. A pin is kept on this
// computer only and is not a star (D5). Rest keeps a pin's keys this app
// does not know.
type HomePin struct {
	Ref      string         `yaml:"ref" json:"ref"`
	Registry string         `yaml:"registry,omitempty" json:"registry,omitempty"`
	Rest     map[string]any `yaml:",inline" json:"-"`
}

// What the stored layout is to this app: usable, of a newer version, or a
// value it cannot decode. The last two are drawn as the defaults and kept
// as they are.
type homeState string

const (
	homeOK         homeState = ""
	homeNewer      homeState = "newer"
	homeUnreadable homeState = "unreadable"
)

// The refusals of a change to a layout this app keeps rather than uses.
var (
	errNewerHome      = errors.New("this Home was arranged by a newer Agentry Desktop, so this one leaves it as it is; Reset Home starts again from the defaults")
	errUnreadableHome = errors.New("the settings file's Home arrangement cannot be read by this Agentry Desktop, so it is left as it is; Reset Home starts again from the defaults")
)

func defaultHome() HomeLayout {
	cards := make([]HomeCard, 0, len(homeDefaults))
	for _, k := range homeDefaults {
		cards = append(cards, HomeCard{Kind: k})
	}
	return HomeLayout{Version: HomeVersion, Cards: cards}
}

// homeOf is the layout s holds, or the defaults when it holds none or one
// this app keeps rather than uses (the state says which).
func homeOf(s Settings) (HomeLayout, homeState) {
	switch {
	case s.Home == nil:
		return defaultHome(), homeOK
	case s.Home.raw != nil:
		return defaultHome(), homeUnreadable
	case s.Home.Version > HomeVersion:
		return defaultHome(), homeNewer
	}
	l := *s.Home
	l.Version = HomeVersion
	return l, homeOK
}

func homeLayoutState() (HomeLayout, homeState, error) {
	s, err := Load()
	if err != nil {
		return HomeLayout{}, homeOK, err
	}
	l, st := homeOf(s)
	return l, st, nil
}

// HomeLayoutNow reads the layout: the stored one, or the defaults when
// there is none or the stored one is kept rather than used, which newer
// (of a newer version) or unreadable (a value this app cannot decode) then
// says. Reading never writes.
func HomeLayoutNow() (layout HomeLayout, newer, unreadable bool, err error) {
	l, st, err := homeLayoutState()
	return l, st == homeNewer, st == homeUnreadable, err
}

// SetHomeCardShown shows or hides the default card of kind, keeping its
// place in the order. A default card the stored layout lacks is added at
// the end. Any other kind is refused, and so is a change to a newer layout.
func SetHomeCardShown(kind string, shown bool) error {
	if !slices.Contains(homeDefaults, kind) {
		return fmt.Errorf("%q is not a Home card that can be shown or hidden", kind)
	}
	s, l, err := usableHome()
	if err != nil {
		return err
	}
	found := false
	for i := range l.Cards {
		if l.Cards[i].Kind == kind {
			l.Cards[i].Hidden = !shown
			found = true
		}
	}
	if !found {
		l.Cards = append(l.Cards, HomeCard{Kind: kind, Hidden: !shown})
	}
	s.Home = &l
	return Save(s)
}

// ResetHome removes the stored layout, so Home is the defaults again. It
// is the one change a newer layout takes, because the person asked for it.
func ResetHome() error {
	s, err := Load()
	if err != nil {
		return err
	}
	if s.Home == nil {
		return nil
	}
	s.Home = nil
	return Save(s)
}

// Pins (plan desktop-home-and-library.md D5, D11, D12; agentry-desktop#660):
// four to a Pinned card, at most four Pinned cards, so at most 16 pins. Go
// places them: a new pin joins the first Pinned card with room, and a new
// card, Large, is added after every other card only when each one holds
// four. An unpin leaves its gap for the next pin and moves no other pin
// between cards; a card it empties is removed.
const (
	PinsPerCard    = 4
	MaxPinnedCards = 4
	MaxPins        = PinsPerCard * MaxPinnedCards
	// HomeLarge is the one card size, the Build, Voice and Knowledge cards'.
	HomeLarge = "large"
)

// ErrHomeFull refuses a pin past the 16th.
var ErrHomeFull = fmt.Errorf("this Home holds %d pinned QuickAgents, %d to a Pinned card, and every one is full; unpin one to pin another", MaxPins, PinsPerCard)

// validPinRef reports whether ref is namespace/name: two non-empty parts,
// no version, no space.
func validPinRef(ref string) bool {
	ns, name, ok := strings.Cut(ref, "/")
	return ok && ns != "" && name != "" && !strings.ContainsAny(ref, "@ \t\n") && !strings.Contains(name, "/")
}

func (p HomePin) is(ref, registry string) bool {
	return p.Ref == ref && Normalize(p.Registry) == Normalize(registry)
}

// usableHome loads the settings and the layout a change may be made to.
func usableHome() (Settings, HomeLayout, error) {
	s, err := Load()
	if err != nil {
		return Settings{}, HomeLayout{}, err
	}
	l, st := homeOf(s)
	switch st {
	case homeNewer:
		return s, l, errNewerHome
	case homeUnreadable:
		return s, l, errUnreadableHome
	}
	return s, l, nil
}

func pinnedIn(l HomeLayout, ref, registry string) bool {
	for _, c := range l.Cards {
		if c.Kind != HomePinned {
			continue
		}
		for _, p := range c.Pins {
			if p.is(ref, registry) {
				return true
			}
		}
	}
	return false
}

// IsPinned reports whether ref from registry is pinned to Home. A layout
// this app keeps rather than uses pins nothing.
func IsPinned(ref, registry string) bool {
	l, st, err := homeLayoutState()
	return err == nil && st == homeOK && pinnedIn(l, ref, registry)
}

// PinToHome pins ref (namespace/name) from registry ("" for a folder
// install). A pin already there is left as it is; the 17th is ErrHomeFull.
// It writes only the settings file.
func PinToHome(ref, registry string) error {
	if !validPinRef(ref) {
		return fmt.Errorf("%q is not a QuickAgent to pin: a pin names namespace/name", ref)
	}
	s, l, err := usableHome()
	if err != nil {
		return err
	}
	if pinnedIn(l, ref, registry) {
		return nil
	}
	pin := HomePin{Ref: ref, Registry: Normalize(registry)}
	cards := 0
	for i := range l.Cards {
		if l.Cards[i].Kind != HomePinned {
			continue
		}
		cards++
		if len(l.Cards[i].Pins) < PinsPerCard {
			l.Cards[i].Pins = append(l.Cards[i].Pins, pin)
			s.Home = &l
			return Save(s)
		}
	}
	if cards >= MaxPinnedCards {
		return ErrHomeFull
	}
	l.Cards = append(l.Cards, HomeCard{Kind: HomePinned, Size: HomeLarge, Pins: []HomePin{pin}})
	s.Home = &l
	return Save(s)
}

// unpin removes the pins match picks and every Pinned card left empty,
// and saves when anything went.
func unpin(match func(HomePin) bool) error {
	s, l, err := usableHome()
	if err != nil {
		return err
	}
	changed := false
	cards := l.Cards[:0:0]
	for _, c := range l.Cards {
		if c.Kind == HomePinned {
			kept := c.Pins[:0:0]
			for _, p := range c.Pins {
				if match(p) {
					changed = true
					continue
				}
				kept = append(kept, p)
			}
			if len(kept) == 0 && len(c.Pins) > 0 {
				continue
			}
			c.Pins = kept
		}
		cards = append(cards, c)
	}
	if !changed {
		return nil
	}
	l.Cards = cards
	s.Home = &l
	return Save(s)
}

// UnpinFromHome removes the pin of ref from registry; one not pinned is
// left as it is.
func UnpinFromHome(ref, registry string) error {
	return unpin(func(p HomePin) bool { return p.is(ref, registry) })
}

// UnpinRef removes ref's pins when its last install of one kind goes from
// this computer (D5): the folder install's pin (no registry) when fromFolder,
// else its pins from any registry, since a runner record's origin follows
// the app's default registry. A layout this app keeps rather than uses is
// left alone, without an error: an uninstall never fails on Home.
func UnpinRef(ref string, fromFolder bool) error {
	err := unpin(func(p HomePin) bool { return p.Ref == ref && (p.Registry == "") == fromFolder })
	if errors.Is(err, errNewerHome) || errors.Is(err, errUnreadableHome) {
		return nil
	}
	return err
}
