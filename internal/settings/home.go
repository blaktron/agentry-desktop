package settings

import (
	"errors"
	"fmt"
	"slices"

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
	Kind   string         `yaml:"kind" json:"kind"`
	Hidden bool           `yaml:"hidden,omitempty" json:"hidden,omitempty"`
	Size   string         `yaml:"size,omitempty" json:"size,omitempty"`
	Rest   map[string]any `yaml:",inline" json:"-"`
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
	s, err := Load()
	if err != nil {
		return err
	}
	l, st := homeOf(s)
	switch st {
	case homeNewer:
		return errNewerHome
	case homeUnreadable:
		return errUnreadableHome
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
