package meeting

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// Detection: while the feature is on, the Detector's calls are offered for
// recording, or recorded at once for an app on the automatic list. Nothing
// else starts a recording but the person's own Record.

// DefaultOfferFor is how long an offer to record stands before it lapses
// and records nothing.
const DefaultOfferFor = 2 * time.Minute

// detectRetry is how long the watcher waits before it connects again after
// the audio system went away (a restart, a session change).
const detectRetry = 5 * time.Second

// Offer is a detected call offered for recording.
type Offer struct {
	ID      string    `json:"id"`
	App     string    `json:"app"`
	Expires time.Time `json:"expires"`
	call    Call
}

// KnownApp is a meeting app whose call has been detected here, for the
// automatic list.
type KnownApp struct {
	Key      string    `json:"key"`
	Name     string    `json:"name"`
	LastSeen time.Time `json:"lastSeen"`
}

// Refresh reads the switch again: the detector starts watching when the
// feature is on and stops when it is off.
func (m *Manager) Refresh() {
	m.mu.Lock()
	ch := m.refresh
	m.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

// watch runs the detector while the feature is on.
func (m *Manager) watch(ctx context.Context) {
	defer m.wg.Done()
	var stop context.CancelFunc
	var done chan struct{}
	halt := func() {
		if stop == nil {
			return
		}
		stop()
		<-done
		stop, done = nil, nil
		m.forgetCalls()
	}
	defer halt()
	for {
		on := m.Detector != nil && m.Settings().On
		switch {
		case on && stop == nil:
			dctx, cancel := context.WithCancel(ctx)
			stop, done = cancel, make(chan struct{})
			go func() {
				defer close(done)
				m.detect(dctx)
			}()
		case !on:
			halt()
		}
		select {
		case <-ctx.Done():
			return
		case <-m.refresh:
		}
	}
}

// detect follows the detector's calls until ctx ends, connecting again
// after a failure.
func (m *Manager) detect(ctx context.Context) {
	for {
		events := make(chan CallEvent)
		errc := make(chan error, 1)
		go func() { errc <- m.Detector.Watch(ctx, events) }()
	follow:
		for {
			select {
			case ev := <-events:
				m.callEvent(ev)
			case err := <-errc:
				if err != nil && ctx.Err() == nil {
					log.Printf("meeting notes: watching for calls: %v", err)
				}
				break follow
			}
		}
		// The calls are read afresh on the next connection.
		m.forgetCalls()
		select {
		case <-ctx.Done():
			return
		case <-time.After(detectRetry):
		}
	}
}

// forgetCalls drops what is known of the calls under way and any offer.
func (m *Manager) forgetCalls() {
	m.mu.Lock()
	m.live, m.handled, m.order = map[string]Call{}, map[string]bool{}, nil
	had := m.offer != nil
	m.offer = nil
	m.mu.Unlock()
	if had {
		m.withdrawOffer()
		m.changed()
	}
}

func (m *Manager) offerFor() time.Duration {
	if m.OfferFor > 0 {
		return m.OfferFor
	}
	return DefaultOfferFor
}

func (m *Manager) maxLength() time.Duration {
	if l := m.Settings().MaxLength; l > 0 {
		return l
	}
	return DefaultMaxLength
}

// lengthWords is a length of time said in whole hours, or minutes below one.
func lengthWords(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if h := int(d.Hours()); h != 1 {
		return fmt.Sprintf("%d hours", h)
	}
	return "1 hour"
}

// callEvent follows a call starting or ending, then considers the calls
// under way.
func (m *Manager) callEvent(ev CallEvent) {
	key := ev.Call.AppKey()
	if key == "" {
		return
	}
	m.mu.Lock()
	if m.ctx == nil {
		m.mu.Unlock()
		return
	}
	if ev.Ended {
		delete(m.live, key)
		delete(m.handled, key)
		m.order = slices.DeleteFunc(m.order, func(k string) bool { return k == key })
		gone := m.offer != nil && m.offer.call.AppKey() == key
		if gone {
			m.offer = nil
		}
		m.mu.Unlock()
		if gone {
			m.withdrawOffer()
			m.changed()
		}
		m.consider()
		return
	}
	_, known := m.live[key]
	m.live[key] = ev.Call
	if !known {
		m.order = append(m.order, key)
	}
	m.mu.Unlock()
	if !known {
		m.rememberApp(ev.Call)
	}
	m.consider()
}

// consider acts on the calls under way that have not been answered, in the
// order they started, while nothing records: a call of an app on the
// automatic list is recorded at once, taking the place of an offer that
// stands (that call waits its turn); otherwise the first is offered, one
// offer at a time. A call is answered once, by its offer, its automatic
// recording or the person's Stop, and is not taken up again until it has
// ended.
func (m *Manager) consider() {
	s := m.Settings()
	m.mu.Lock()
	if !s.On || m.ctx == nil || m.ctx.Err() != nil || m.closing || m.rec != nil {
		m.mu.Unlock()
		return
	}
	var auto, ask *Call
	for _, k := range m.order {
		if m.handled[k] {
			continue
		}
		c := m.live[k]
		if slices.Contains(s.Automatic, k) {
			if auto == nil {
				auto = &c
			}
		} else if ask == nil {
			ask = &c
		}
	}
	ctx := m.ctx
	switch {
	case auto != nil:
		m.handled[auto.AppKey()] = true
		if o := m.offer; o != nil {
			delete(m.handled, o.call.AppKey())
		}
		m.mu.Unlock()
		// startRecording takes away an offer that stands.
		if _, err := m.startRecording(ctx, *auto, Automatically); err != nil {
			log.Printf("meeting notes: recording %s automatically: %v", auto.App, err)
			_ = m.notify(Notice{Title: "A meeting was not recorded", Body: "Agentry could not record " + auto.App + ": " + err.Error()})
		}
	case ask != nil && m.offer == nil:
		m.handled[ask.AppKey()] = true
		o := &Offer{ID: offerID(), App: ask.App, Expires: m.now().Add(m.offerFor()), call: *ask}
		m.offer = o
		m.mu.Unlock()
		m.raiseOffer(o)
	default:
		m.mu.Unlock()
	}
}

// raiseOffer raises an offer's notification, and lets the offer lapse after
// OfferFor.
func (m *Manager) raiseOffer(o *Offer) {
	name := o.App
	if name == "" {
		name = "a meeting app"
	}
	// noticeMu orders this against a recording's start, which takes the
	// offer away: an offer answered already is not raised.
	m.noticeMu.Lock()
	m.mu.Lock()
	standing := m.offer == o
	m.mu.Unlock()
	if !standing {
		m.noticeMu.Unlock()
		return
	}
	_ = m.notify(Notice{
		Tag: tagOffer, Resident: true,
		Title: "Record this meeting? (" + name + ")",
		Body:  "Agentry can record your microphone and " + name + " and transcribe them on this computer. " + ConsentSentence,
		Actions: []NoticeAction{
			{ID: ActionRecord + ":" + o.ID, Label: "Record"},
			{ID: ActionNotNow + ":" + o.ID, Label: "Not now"},
		},
	})
	m.noticeMu.Unlock()
	m.changed()
	time.AfterFunc(m.offerFor(), func() { _ = m.DeclineOffer(o.ID) })
}

// withdrawOffer takes the offer's notification away, under noticeMu, so an
// offer being raised at the same moment is taken away after it.
func (m *Manager) withdrawOffer() {
	m.noticeMu.Lock()
	defer m.noticeMu.Unlock()
	m.withdraw(tagOffer)
}

// offerID names an offer: random, so an answer to one that lapsed never
// answers the next.
func offerID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ErrNoOffer is an answer to an offer that no longer stands: it lapsed, it
// was answered, or its call ended.
var ErrNoOffer = errors.New("that meeting is no longer offered for recording")

// takeOffer removes the offer with id, if it still stands.
func (m *Manager) takeOffer(id string) (*Offer, error) {
	m.mu.Lock()
	o := m.offer
	if o == nil || o.ID != id {
		m.mu.Unlock()
		return nil, ErrNoOffer
	}
	m.offer = nil
	m.mu.Unlock()
	m.withdrawOffer()
	m.changed()
	return o, nil
}

// AcceptOffer records the offered call: the person pressed Record on it.
func (m *Manager) AcceptOffer(ctx context.Context, id string) (*Meeting, error) {
	if !m.Settings().On {
		return nil, ErrOff
	}
	o, err := m.takeOffer(id)
	if err != nil {
		return nil, err
	}
	return m.startRecording(ctx, o.call, FromOffer)
}

// DeclineOffer lets the offer go: nothing is recorded, and the call is not
// offered again. Another call waiting its turn is considered.
func (m *Manager) DeclineOffer(id string) error {
	if _, err := m.takeOffer(id); err != nil {
		return err
	}
	m.consider()
	return nil
}

// HandleAction answers a press on one of the feature's notification
// buttons.
func (m *Manager) HandleAction(action string) error {
	verb, id, _ := strings.Cut(action, ":")
	switch verb {
	case ActionRecord:
		_, err := m.AcceptOffer(m.ctxOrBackground(), id)
		return err
	case ActionNotNow:
		return m.DeclineOffer(id)
	case ActionPause:
		return m.SetPaused(true)
	case ActionResume:
		return m.SetPaused(false)
	case ActionStop:
		return m.Stop()
	}
	return fmt.Errorf("no such action %q", action)
}

func (m *Manager) ctxOrBackground() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
}

func (m *Manager) appsPath() string { return filepath.Join(m.Root, "apps.json") }

// Apps are the meeting apps whose calls have been detected here, the most
// recently seen first.
func (m *Manager) Apps() []KnownApp {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.readAppsLocked()
}

func (m *Manager) readAppsLocked() []KnownApp {
	var apps []KnownApp
	if raw, err := os.ReadFile(m.appsPath()); err == nil {
		_ = json.Unmarshal(raw, &apps)
	}
	sort.SliceStable(apps, func(i, j int) bool { return apps[i].LastSeen.After(apps[j].LastSeen) })
	return apps
}

// maxKnownApps bounds the list of apps kept.
const maxKnownApps = 32

// rememberApp keeps a detected call's app in the list the automatic
// choices are made from.
func (m *Manager) rememberApp(c Call) {
	m.mu.Lock()
	defer m.mu.Unlock()
	apps := m.readAppsLocked()
	key, name := c.AppKey(), c.App
	if name == "" {
		name = key
	}
	apps = slices.DeleteFunc(apps, func(a KnownApp) bool { return a.Key == key })
	apps = append([]KnownApp{{Key: key, Name: name, LastSeen: m.now()}}, apps...)
	if len(apps) > maxKnownApps {
		apps = apps[:maxKnownApps]
	}
	raw, err := json.MarshalIndent(apps, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(m.Root, 0o700); err != nil {
		return
	}
	_ = os.WriteFile(m.appsPath(), raw, 0o600)
}
