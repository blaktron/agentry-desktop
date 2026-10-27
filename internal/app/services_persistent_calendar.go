package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/blaktron/agentry-cli/pkg/runner/persistent"
	"github.com/blaktron/agentry-cli/pkg/runner/spec"
)

// The Assistant's calendar feeds in the app: a name the person chooses and
// a calendar's iCal (ICS) address, typed in a password field and handed to
// Go, which fetches and reads it once (persistent.CalendarFeedReadable)
// before anything is kept, as the CLI's --calendar-feed does. The address
// goes into the system keychain under the feed's name and is never sent
// back to the page, which sees the feed's name, its host and service, and
// when its address was last checked. Adding, renaming and removing a feed
// are refused while the agent runs, here or in another client, as the CLI
// refuses them.

// PersistentCalendarFeedView is one calendar feed as the app shows it:
// never its address. Set is when the address was last fetched and read
// before it was kept.
type PersistentCalendarFeedView struct {
	Name    string `json:"name"`
	Host    string `json:"host"`
	Service string `json:"service,omitempty"`
	On      bool   `json:"on"`
	Set     string `json:"set,omitempty"`
	// SignedIn says a browser sign-in is kept for the feed.
	SignedIn bool `json:"signedIn,omitempty"`
	// Entry is the feed's roster id.
	Entry string `json:"entry"`
}

// calendarFeedViews are the agent's calendar feeds, none for an agent that
// reads no calendar.
func calendarFeedViews(a persistent.Agent, cfg persistent.Config) []PersistentCalendarFeedView {
	out := []PersistentCalendarFeedView{}
	for _, f := range persistent.CalendarFeeds(a, cfg) {
		v := PersistentCalendarFeedView{Name: f.Name, Host: f.Host, Service: f.Service, On: f.On, SignedIn: f.SignedIn, Entry: f.Entry}
		if !f.Set.IsZero() {
			v.Set = f.Set.Format(time.RFC3339)
		}
		out = append(out, v)
	}
	return out
}

// calendarAgent is the agent named, if it reads calendars and is stopped:
// not running here, not starting, and not run by another client.
func (p *PersistentAPI) calendarAgent(name string) (persistent.Agent, string, persistent.Config, error) {
	a, err := p.agent(name)
	if err != nil {
		return persistent.Agent{}, "", persistent.Config{}, err
	}
	if !a.Builtin.ReadsCalendars() || !a.Builtin.KeepsSecrets() {
		return persistent.Agent{}, "", persistent.Config{}, fmt.Errorf("the %s has no calendar feeds: the Assistant alone reads them", a.Title)
	}
	home, err := p.home()
	if err != nil {
		return persistent.Agent{}, "", persistent.Config{}, err
	}
	p.mu.Lock()
	_, starting := p.starting[a.Name]
	p.mu.Unlock()
	_, running := p.running(a.Name)
	held, _ := p.heldElsewhere(home, a.Name)
	if running || starting || held {
		return persistent.Agent{}, "", persistent.Config{}, fmt.Errorf("the %s is running: stop it first, then change its calendar feeds", a.Title)
	}
	cfg, err := persistent.LoadConfig(home, a.Name)
	if err != nil {
		return persistent.Agent{}, "", persistent.Config{}, err
	}
	return a, home, cfg, nil
}

// findFeed is the agent's feed named name, case not mattering.
func findFeed(cfg persistent.Config, name string) (persistent.CalendarFeed, bool) {
	for _, f := range cfg.CalendarFeeds {
		if strings.EqualFold(f.Name, name) {
			return f, true
		}
	}
	return persistent.CalendarFeed{}, false
}

// calendarReadable fetches and reads a feed's address once: the CLI's own
// check, but in a test.
func (p *PersistentAPI) calendarReadable(ctx context.Context, address string) (string, int, error) {
	if p.calendarCheck != nil {
		return p.calendarCheck(ctx, address)
	}
	return persistent.CalendarFeedReadable(ctx, address)
}

// AddCalendarFeed keeps a calendar's address under the name the person
// chose, once it is fetched and read as a calendar: a webcal address is
// read as https, and anything else but https is refused in the CLI's
// words. A name the agent has already sets that feed's address again,
// after asking, and drops a sign-in kept for it. An address that asks for
// a sign-in says to use Sign in in a browser window. No error repeats the
// address.
func (p *PersistentAPI) AddCalendarFeed(name, feed, address string) (*PersistentAgentView, error) {
	a, _, cfg, err := p.calendarAgent(name)
	if err != nil {
		return nil, err
	}
	feed, addr, host, err := p.feedAddress(a, cfg, feed, address, false)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, _, err := p.calendarReadable(ctx, addr); err != nil {
		if errors.Is(err, persistent.ErrCalendarFeedSignIn) {
			return nil, fmt.Errorf("%w: use Sign in in a browser window, and sign in to the account that can see the calendar", err)
		}
		return nil, err
	}
	if err := p.keepFeed(a, feed, host, addr, ""); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// SignInCalendarFeed keeps a calendar's address with a browser sign-in: a
// browser window opens at the address, the person signs in to the account
// that can see the calendar, and once the address reads as a calendar the
// window closes and the address and the browser's cookies for its host are
// kept. An empty address signs a feed the agent has in again at its own
// address. CancelCalendarSignIn ends the wait and keeps nothing.
func (p *PersistentAPI) SignInCalendarFeed(name, feed, address string) (*PersistentAgentView, error) {
	a, _, cfg, err := p.calendarAgent(name)
	if err != nil {
		return nil, err
	}
	feed, addr, host, err := p.feedAddress(a, cfg, feed, address, true)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.mu.Lock()
	if p.calendarSignIns == nil {
		p.calendarSignIns = map[string]context.CancelFunc{}
	}
	if _, busy := p.calendarSignIns[a.Name]; busy {
		p.mu.Unlock()
		return nil, errors.New("a calendar sign-in is already waiting in a browser window: finish it or cancel it first")
	}
	p.calendarSignIns[a.Name] = cancel
	p.mu.Unlock()
	defer func() {
		p.mu.Lock()
		delete(p.calendarSignIns, a.Name)
		p.mu.Unlock()
	}()
	session, err := p.calendarBrowserSignIn(ctx, addr)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, errors.New("the sign-in was cancelled: nothing was kept")
		}
		return nil, fmt.Errorf("the browser sign-in for %s: %w", feed, err)
	}
	if err := p.keepFeed(a, feed, host, addr, session); err != nil {
		return nil, err
	}
	return p.Get(name)
}

// CancelCalendarSignIn ends a calendar sign-in waiting in a browser
// window: the window closes and nothing is kept.
func (p *PersistentAPI) CancelCalendarSignIn(name string) {
	p.mu.Lock()
	cancel := p.calendarSignIns[name]
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// calendarBrowserSignIn is the CLI's browser sign-in, but in a test.
func (p *PersistentAPI) calendarBrowserSignIn(ctx context.Context, address string) (string, error) {
	if p.calendarSignIn != nil {
		return p.calendarSignIn(ctx, address)
	}
	session, _, _, err := persistent.SignInCalendarFeed(ctx, address, nil)
	return session, err
}

// feedAddress checks the feed's name and the address given for it, and
// returns the name (a feed the agent has keeps the one it was first
// given), the address as fetched and its host. A feed the agent has is
// asked about before its address is replaced; with keep, an empty address
// is that feed's own, read from the keychain.
func (p *PersistentAPI) feedAddress(a persistent.Agent, cfg persistent.Config, feed, address string, keep bool) (string, string, string, error) {
	feed = strings.TrimSpace(feed)
	if err := spec.CheckCalendarFeedName(feed); err != nil {
		return "", "", "", err
	}
	f, has := findFeed(cfg, feed)
	if has {
		// Set again under the name it was first given.
		feed = f.Name
	}
	address = strings.TrimSpace(address)
	if address == "" && keep && has {
		kept, err := p.secretStore().Get(persistent.CalendarKeychainAccount(a.Name, feed))
		if err != nil {
			return "", "", "", fmt.Errorf("the address of %s could not be read from your keychain: give it again", feed)
		}
		address = kept
	} else if has {
		if err := p.app.askConsent("Replace the address of "+feed+"?", "This replaces the address of the calendar feed "+feed+" that "+called(a, cfg)+" reads, in your system keychain. The old address is not kept here; reset it in your calendar's settings if you no longer use it."); err != nil {
			return "", "", "", err
		}
	}
	addr, host, err := spec.CalendarFeedAddress(address)
	if err != nil {
		return "", "", "", err
	}
	if err := persistent.CheckSecretValue(addr); err != nil {
		return "", "", "", errors.New("the calendar feed's address cannot be kept in the keychain as it is")
	}
	return feed, addr, host, nil
}

// keepFeed keeps a feed's address, and its browser sign-in or none, in the
// keychain, then records the feed, which asks first. A change refused or
// failed puts the keychain back as it was.
func (p *PersistentAPI) keepFeed(a persistent.Agent, feed, host, addr, session string) error {
	keys := p.secretStore()
	account := persistent.CalendarKeychainAccount(a.Name, feed)
	previous, hadPrevious := "", false
	if v, err := keys.Get(account); err == nil {
		previous, hadPrevious = v, true
	}
	previousSession, _ := persistent.CalendarSession(keys, a.Name, feed)
	restore := func() {
		if hadPrevious {
			_ = keys.Set(account, previous)
		} else {
			_ = keys.Delete(account)
		}
		if previousSession != "" {
			_ = persistent.KeepCalendarSession(keys, a.Name, feed, previousSession)
		} else {
			_ = persistent.ForgetCalendarSession(keys, a.Name, feed)
		}
	}
	if err := keys.Set(account, addr); err != nil {
		return err
	}
	var err error
	if session != "" {
		err = persistent.KeepCalendarSession(keys, a.Name, feed, session)
	} else {
		err = persistent.ForgetCalendarSession(keys, a.Name, feed)
	}
	if err != nil {
		restore()
		return err
	}
	if err := p.configure(a, persistent.Change{SetCalendarFeed: &persistent.CalendarFeed{Name: feed, Host: host, Set: time.Now().UTC(), SignedIn: session != ""}}); err != nil {
		restore()
		return err
	}
	return nil
}

// RenameCalendarFeed gives a feed another name: its address moves to the
// new name's keychain entry, and the feed keeps its host, when it was
// checked and whether it is switched on. The new name's sentence is a new
// one to agree to, so a feed that is on asks first. A refused or failed
// rename leaves the feed and the keychain as they were.
func (p *PersistentAPI) RenameCalendarFeed(name, feed, to string) (*PersistentAgentView, error) {
	a, _, cfg, err := p.calendarAgent(name)
	if err != nil {
		return nil, err
	}
	f, ok := findFeed(cfg, feed)
	if !ok {
		return nil, fmt.Errorf("the %s has no calendar feed %q", a.Title, feed)
	}
	to = strings.TrimSpace(to)
	if err := spec.CheckCalendarFeedName(to); err != nil {
		return nil, err
	}
	if to == f.Name {
		return p.Get(name)
	}
	if strings.EqualFold(to, f.Name) {
		return nil, fmt.Errorf("%q differs from %q only in its capitals, which a feed's name does not tell apart: remove the feed and add it again to change them", to, f.Name)
	}
	if other, taken := findFeed(cfg, to); taken {
		return nil, fmt.Errorf("the %s has a calendar feed called %s already", a.Title, other.Name)
	}
	keys := p.secretStore()
	from, account := persistent.CalendarKeychainAccount(a.Name, f.Name), persistent.CalendarKeychainAccount(a.Name, to)
	addr, err := keys.Get(from)
	if err != nil {
		return nil, fmt.Errorf("the address of %s could not be read from your keychain, so the feed is left as it is: %w", f.Name, err)
	}
	if err := keys.Set(account, addr); err != nil {
		return nil, err
	}
	if f.SignedIn {
		session, err := persistent.CalendarSession(keys, a.Name, f.Name)
		if err == nil {
			err = persistent.KeepCalendarSession(keys, a.Name, to, session)
		}
		if err != nil {
			_ = keys.Delete(account)
			_ = persistent.ForgetCalendarSession(keys, a.Name, to)
			return nil, fmt.Errorf("the sign-in of %s could not be moved in your keychain, so the feed is left as it is: %w", f.Name, err)
		}
	}
	ch := persistent.Change{SetCalendarFeed: &persistent.CalendarFeed{Name: to, Host: f.Host, Set: f.Set, SignedIn: f.SignedIn}, RemoveCalendarFeed: f.Name}
	for _, s := range persistent.CalendarFeeds(a, cfg) {
		if s.Name == f.Name && !s.On {
			ch.Switch = &persistent.Switch{ID: persistent.CalendarEntryID(to), On: false}
		}
	}
	if err := p.configure(a, ch); err != nil {
		_ = persistent.ForgetCalendarFeed(keys, a.Name, to)
		return nil, err
	}
	if err := persistent.ForgetCalendarFeed(keys, a.Name, f.Name); err != nil {
		return nil, fmt.Errorf("the feed is renamed %s, but its old keychain entry %s could not be removed: %w", to, from, err)
	}
	return p.Get(name)
}

// RemoveCalendarFeed takes a feed away from the agent, then its address
// out of the keychain. The address stays valid at the calendar until the
// person resets it there.
func (p *PersistentAPI) RemoveCalendarFeed(name, feed string) (*PersistentAgentView, error) {
	a, _, cfg, err := p.calendarAgent(name)
	if err != nil {
		return nil, err
	}
	f, ok := findFeed(cfg, feed)
	if !ok {
		return nil, fmt.Errorf("the %s has no calendar feed %q", a.Title, feed)
	}
	if err := p.configure(a, persistent.Change{RemoveCalendarFeed: f.Name}); err != nil {
		return nil, err
	}
	if err := persistent.ForgetCalendarFeed(p.secretStore(), a.Name, f.Name); err != nil {
		return nil, err
	}
	return p.Get(name)
}
