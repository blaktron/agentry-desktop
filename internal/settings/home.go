package settings

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// The Home page's layout: an ordered, versioned list of cards under the
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
	// HomeDeliverables lists the most recent deliverables across runs.
	HomeDeliverables = "deliverables"
	// HomePinned is a card of pinned QuickAgents, one of several.
	HomePinned = "pinned"
	// HomeScheduled is the Scheduled page's mini calendar: at most one,
	// Large, pinned from the Scheduled page and shown or hidden in Settings
	// → Home. It is a card, not one of the 16 QuickAgent pins, and it is no
	// default: Reset Home removes it.
	HomeScheduled = "scheduled"
	// HomeMain is Main's card: a chat with Agentry's own persistent agent,
	// first on Home. It starts nothing until the person sends a message.
	HomeMain = "main"
)

// homeDefaults are the default cards, in their order: Main's card first, the
// place to ask Agentry for something, then the Deliverables card, because
// the files a run was for are what a person comes back to.
// Each is one card of its kind, shown or hidden by kind. The page's list is
// HOME_DEFAULTS in frontend/src/homeCards.ts: the two change together.
var homeDefaults = []string{HomeMain, HomeDeliverables, HomeRecentRuns, HomeInstalled, HomeProfile, HomeRunDefaults}

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
// not fit.
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
// Voice and Knowledge cards, kept for an older app. Cols and Rows are the
// card's place on Home's grid, set by the person's resize: 0 is the card's
// default, else 1 to HomeMaxCols wide and 1 to HomeMaxRows tall. Rest keeps
// a card's keys this app does not know.
type HomeCard struct {
	Kind   string `yaml:"kind" json:"kind"`
	Hidden bool   `yaml:"hidden,omitempty" json:"hidden,omitempty"`
	Size   string `yaml:"size,omitempty" json:"size,omitempty"`
	Cols   int    `yaml:"cols,omitempty" json:"cols,omitempty"`
	Rows   int    `yaml:"rows,omitempty" json:"rows,omitempty"`
	// Pins are a Pinned card's QuickAgents, at most PinsPerCard.
	Pins []HomePin      `yaml:"pins,omitempty" json:"pins,omitempty"`
	Rest map[string]any `yaml:",inline" json:"-"`
}

// HomePin is one pinned QuickAgent: its ref (namespace/name, no version)
// and the registry it was installed from, empty for a QuickAgent
// installed from a folder. A pin is kept on this computer only and is
// not a star. Rest keeps a pin's keys this app does not know.
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
	l.Cards = withMain(withDeliverables(l.Cards))
	return l, homeOK
}

// withDeliverables is cards with the Deliverables card added at the end when
// a stored layout holds none, a Home arranged before the card existed. It is
// added after every other card, so the person's order never moves, and once
// the layout is saved with it the card is the person's to hide: hiding keeps
// its entry, so it is never added a second time. A Home with no stored
// layout has it first, in the defaults.
func withDeliverables(cards []HomeCard) []HomeCard {
	for _, c := range cards {
		if c.Kind == HomeDeliverables {
			return cards
		}
	}
	return append(slices.Clone(cards), HomeCard{Kind: HomeDeliverables})
}

// withMain is cards with Main's card put first when a stored layout holds
// none, a Home arranged before the card existed. As with the Deliverables
// card, once the layout is saved with it the card is the person's to hide,
// and its hidden entry keeps it from being put back.
func withMain(cards []HomeCard) []HomeCard {
	for _, c := range cards {
		if c.Kind == HomeMain {
			return cards
		}
	}
	return append([]HomeCard{{Kind: HomeMain}}, cards...)
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

// SetHomeCardShown shows or hides the default card of kind, or the
// Scheduled card, keeping its place in the order. One the stored layout
// lacks is added at the end when it is shown (the Scheduled card Large).
// Any other kind is refused, and so is a change to a newer layout.
func SetHomeCardShown(kind string, shown bool) error {
	if !slices.Contains(homeDefaults, kind) && kind != HomeScheduled {
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
		c := HomeCard{Kind: kind, Hidden: !shown}
		if kind == HomeScheduled {
			if !shown {
				return nil
			}
			c.Size = HomeLarge
		}
		l.Cards = append(l.Cards, c)
	}
	s.Home = &l
	return Save(s)
}

// PinScheduledToHome is the Scheduled page's Pin to Home: the Scheduled
// card, Large, after every other card, or a hidden one shown again in its
// place. One already shown is left as it is.
func PinScheduledToHome() error { return SetHomeCardShown(HomeScheduled, true) }

// UnpinScheduledFromHome removes the Scheduled card, hidden or not; a Home
// without one is left as it is.
func UnpinScheduledFromHome() error {
	s, l, err := usableHome()
	if err != nil {
		return err
	}
	cards := slices.DeleteFunc(slices.Clone(l.Cards), func(c HomeCard) bool { return c.Kind == HomeScheduled })
	if len(cards) == len(l.Cards) {
		return nil
	}
	l.Cards = cards
	s.Home = &l
	return Save(s)
}

// Home's grid: four columns, a card one to four of them wide and one to
// three rows tall.
const (
	HomeMaxCols = 4
	HomeMaxRows = 3
)

// cardAt is the index of the card at i in l when it is of kind, the check
// that the page moves or sizes the card it drew, not one a change since its
// read put in that place.
func cardAt(l HomeLayout, i int, kind string) error {
	if i < 0 || i >= len(l.Cards) {
		return fmt.Errorf("there is no card %d on Home; read Home again", i)
	}
	if l.Cards[i].Kind != kind {
		return fmt.Errorf("card %d on Home is %q, not %q; read Home again", i, l.Cards[i].Kind, kind)
	}
	return nil
}

// MoveHomeCard puts the card at from (of kind) at to in the order, the
// others keeping theirs; hidden cards keep their places among them. A move
// to its own place writes nothing.
func MoveHomeCard(from int, kind string, to int) error {
	s, l, err := usableHome()
	if err != nil {
		return err
	}
	if err := cardAt(l, from, kind); err != nil {
		return err
	}
	if to < 0 || to >= len(l.Cards) {
		return fmt.Errorf("there is no place %d on Home", to)
	}
	if from == to {
		return nil
	}
	c := l.Cards[from]
	cards := slices.Delete(slices.Clone(l.Cards), from, from+1)
	l.Cards = slices.Insert(cards, to, c)
	s.Home = &l
	return Save(s)
}

// SizeHomeCard sets the card at i (of kind) to cols wide and rows tall on
// Home's grid. Anything outside the grid is refused.
func SizeHomeCard(i int, kind string, cols, rows int) error {
	if cols < 1 || cols > HomeMaxCols || rows < 1 || rows > HomeMaxRows {
		return fmt.Errorf("a Home card is 1 to %d columns wide and 1 to %d rows tall, not %d by %d", HomeMaxCols, HomeMaxRows, cols, rows)
	}
	s, l, err := usableHome()
	if err != nil {
		return err
	}
	if err := cardAt(l, i, kind); err != nil {
		return err
	}
	if l.Cards[i].Cols == cols && l.Cards[i].Rows == rows {
		return nil
	}
	l.Cards = slices.Clone(l.Cards)
	l.Cards[i].Cols, l.Cards[i].Rows = cols, rows
	s.Home = &l
	return Save(s)
}

// The themes Settings → Home offers, in its order. System follows the
// operating system's light or dark setting with the base palette; each
// other is a block of frontend/src/themes.css (internal/designtokens holds
// the two lists together and audits every block's contrast).
const ThemeSystem = "system"

var Themes = []string{ThemeSystem, "light", "dark", "slate", "forest", "midnight", "contrast"}

// ThemeNow is the stored theme, System when none is stored or the stored
// one is not a theme this app has (a newer app's).
func ThemeNow() (string, error) {
	s, err := Load()
	if err != nil {
		return "", err
	}
	if s.Theme == "" || !slices.Contains(Themes, s.Theme) {
		return ThemeSystem, nil
	}
	return s.Theme, nil
}

// SetTheme stores one of Themes; System is stored as none.
func SetTheme(name string) error {
	if !slices.Contains(Themes, name) {
		return fmt.Errorf("%q is not a theme Agentry Desktop has", name)
	}
	s, err := Load()
	if err != nil {
		return err
	}
	if name == ThemeSystem {
		name = ""
	}
	if s.Theme == name {
		return nil
	}
	s.Theme = name
	return Save(s)
}

// HomeAgentNow is the persistent agent Home's agent card runs, by its name:
// "" for Agentry Helper. The name is kept as it was stored; the app checks it
// against the agents it has, and draws Agentry Helper for one it lacks.
func HomeAgentNow() (string, error) {
	s, err := Load()
	if err != nil {
		return "", err
	}
	return s.HomeAgent, nil
}

// SetHomeAgent stores the agent Home's agent card runs: "" for Agentry
// Helper. The caller has checked the name.
func SetHomeAgent(name string) error {
	s, err := Load()
	if err != nil {
		return err
	}
	if s.HomeAgent == name {
		return nil
	}
	s.HomeAgent = name
	return Save(s)
}

// ResetHome removes the stored layout and the agent card's agent, so Home
// is the defaults again. It is the one change a newer layout takes, because
// the person asked for it.
func ResetHome() error {
	s, err := Load()
	if err != nil {
		return err
	}
	if s.Home == nil && s.HomeAgent == "" {
		return nil
	}
	s.Home = nil
	s.HomeAgent = ""
	return Save(s)
}

// Pins: four to a Pinned card, at most four Pinned cards, so at most 16
// pins. Go places them: a new pin joins the first Pinned card with room, and
// a new card, Large, is added after every other card only when each one
// holds four. An unpin leaves its gap for the next pin and moves no other
// pin between cards; a card it empties is removed.
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
// this computer: the folder install's pin (no registry) when fromFolder, else
// its pins from any registry, since a runner record's origin follows the
// app's default registry. A layout this app keeps rather than uses is left
// alone, without an error: an uninstall never fails on Home.
func UnpinRef(ref string, fromFolder bool) error {
	err := unpin(func(p HomePin) bool { return p.Ref == ref && (p.Registry == "") == fromFolder })
	if errors.Is(err, errNewerHome) || errors.Is(err, errUnreadableHome) {
		return nil
	}
	return err
}
