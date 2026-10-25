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
	m.live = map[string]Call{}
	had := m.offer != nil
	m.offer = nil
	m.mu.Unlock()
	if had {
		m.withdraw(tagOffer)
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

// hours is a length of time said in whole hours, or minutes below one.
func hours(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	if h := int(d.Hours()); h != 1 {
		return fmt.Sprintf("%d hours", h)
	}
	return "1 hour"
}

// callEvent acts on a call starting or ending. A call that starts is
// recorded at once when its app is on the automatic list and nothing else
// records; otherwise it is offered, unless an offer already stands. A call
// is offered once: Not now, or an offer that lapses, holds until the call
// has ended.
func (m *Manager) callEvent(ev CallEvent) {
	key := ev.Call.AppKey()
	if key == "" {
		return
	}
	s := m.Settings()
	m.mu.Lock()
	if !s.On || m.ctx == nil {
		m.mu.Unlock()
		return
	}
	if ev.Ended {
		delete(m.live, key)
		var gone bool
		if m.offer != nil && m.offer.call.AppKey() == key {
			m.offer, gone = nil, true
		}
		m.mu.Unlock()
		if gone {
			m.withdraw(tagOffer)
			m.changed()
		}
		return
	}
	if _, known := m.live[key]; known {
		m.live[key] = ev.Call
		m.mu.Unlock()
		return
	}
	m.live[key] = ev.Call
	ctx := m.ctx
	busy := m.rec != nil || m.offer != nil
	m.mu.Unlock()
	m.rememberApp(ev.Call)
	if busy {
		return
	}
	if slices.Contains(s.Automatic, key) {
		if _, err := m.startRecording(ctx, ev.Call, Automatically); err != nil {
			log.Printf("meeting notes: recording %s automatically: %v", ev.Call.App, err)
			m.notify(Notice{Title: "A meeting was not recorded", Body: "Agentry could not record " + ev.Call.App + ": " + err.Error()})
		}
		return
	}
	m.makeOffer(ev.Call)
}

// makeOffer offers a call for recording, and lets the offer lapse after
// OfferFor.
func (m *Manager) makeOffer(call Call) {
	o := &Offer{ID: offerID(), App: call.App, Expires: m.now().Add(m.offerFor()), call: call}
	m.mu.Lock()
	if m.offer != nil || m.rec != nil {
		m.mu.Unlock()
		return
	}
	m.offer = o
	m.mu.Unlock()
	name := call.App
	if name == "" {
		name = "a meeting app"
	}
	m.notify(Notice{
		Tag: tagOffer, Resident: true,
		Title: "Record this meeting? (" + name + ")",
		Body:  "Agentry can record your microphone and " + name + " and transcribe them on this computer. " + ConsentSentence,
		Actions: []NoticeAction{
			{ID: ActionRecord + ":" + o.ID, Label: "Record"},
			{ID: ActionNotNow + ":" + o.ID, Label: "Not now"},
		},
	})
	m.changed()
	time.AfterFunc(m.offerFor(), func() { _ = m.DeclineOffer(o.ID) })
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
	m.withdraw(tagOffer)
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
// offered again.
func (m *Manager) DeclineOffer(id string) error {
	_, err := m.takeOffer(id)
	return err
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
