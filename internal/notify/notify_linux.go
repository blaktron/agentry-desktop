package notify

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

// The freedesktop notification service: the desktop's own notification
// server answers on the session bus (Plasma's, GNOME Shell's, dunst, mako).
const (
	busName   = "org.freedesktop.Notifications"
	busPath   = dbus.ObjectPath("/org/freedesktop/Notifications")
	busIface  = "org.freedesktop.Notifications"
	appName   = "Agentry"
	appIcon   = "agentry-desktop"
	actionKey = "default"
)

// maxKeys bounds the notifications whose clicks are remembered: the server
// hands back an id, not the key itself.
const maxKeys = 64

// shown is what is remembered of a notification by its id: the key its
// click hands back, the buttons it offered, whether it stays on screen
// after a press, and its tag.
type shown struct {
	key      string
	actions  []string
	resident bool
	tag      string
}

// session is the connection this package keeps to the session bus, its own
// so that the signals it subscribes to are not another library's.
var session struct {
	sync.Mutex
	conn *dbus.Conn
	// markup is the server's "body-markup" capability: a body it reads as
	// markup is escaped, and one it shows as written is not.
	markup bool
	// keys are the notifications by id, oldest first in order; tags are
	// the ids of the notifications raised with a tag.
	keys  map[uint32]shown
	order []uint32
	tags  map[string]uint32
}

// connectBus is the bus connection, made once; a test points it elsewhere.
var connectBus = dbus.ConnectSessionBus

// Init readies notifications. There is nothing to ready on Linux: the bus is
// reached at the first notification.
func Init() error { return nil }

func show(n Notice) error {
	c, markup, err := connection()
	if err != nil {
		return err
	}
	body := n.Body
	if markup {
		body = escapeMarkup(body)
	}
	hints := map[string]dbus.Variant{
		"desktop-entry": dbus.MakeVariant(appIcon),
		"urgency":       dbus.MakeVariant(byte(1)),
	}
	// The default action is the click; each button follows as its id and
	// its label. An id that is the default action's is not a button.
	actions := []string{actionKey, "Open"}
	var ids []string
	for _, a := range n.Actions {
		if a.ID == "" || a.ID == actionKey {
			continue
		}
		actions = append(actions, a.ID, a.Label)
		ids = append(ids, a.ID)
	}
	timeout := int32(-1)
	if n.Resident {
		hints["resident"] = dbus.MakeVariant(true)
		timeout = 0
	}
	var id uint32
	err = c.Object(busName, busPath).Call(busIface+".Notify", 0,
		appName, tagged(n.Tag), appIcon, n.Title, body, actions, hints, timeout).Store(&id)
	if err != nil {
		// The next notification connects again: the server or the bus may
		// have restarted under this connection.
		drop(c)
		return fmt.Errorf("the desktop's notification service refused the notification: %w", err)
	}
	remember(id, shown{key: n.Key, actions: ids, resident: n.Resident, tag: n.Tag})
	return nil
}

// tagged is the id of the notification shown with tag, which a new one
// replaces; 0 for none.
func tagged(tag string) uint32 {
	if tag == "" {
		return 0
	}
	session.Lock()
	defer session.Unlock()
	return session.tags[tag]
}

// withdraw closes the notification shown with tag.
func withdraw(tag string) error {
	session.Lock()
	id, ok := session.tags[tag]
	c := session.conn
	session.Unlock()
	if !ok || c == nil {
		return nil
	}
	forget(id)
	if err := c.Object(busName, busPath).Call(busIface+".CloseNotification", 0, id).Err; err != nil {
		return fmt.Errorf("the desktop's notification service would not close the notification: %w", err)
	}
	return nil
}

// drop closes c and forgets it, if it is still the one kept.
func drop(c *dbus.Conn) {
	session.Lock()
	defer session.Unlock()
	if session.conn == c {
		session.conn = nil
		_ = c.Close()
	}
}

// connection is the session bus, connected at the first call with the
// server's capabilities read and its click and close signals followed.
func connection() (*dbus.Conn, bool, error) {
	session.Lock()
	defer session.Unlock()
	if session.conn != nil {
		return session.conn, session.markup, nil
	}
	c, err := connectBus()
	if err != nil {
		return nil, false, fmt.Errorf("no session bus to raise a notification on: %w", err)
	}
	var caps []string
	if err := c.Object(busName, busPath).Call(busIface+".GetCapabilities", 0).Store(&caps); err != nil {
		_ = c.Close()
		return nil, false, fmt.Errorf("no desktop notification service answers on the session bus: %w", err)
	}
	for _, member := range []string{"ActionInvoked", "NotificationClosed"} {
		// From the notification server alone: another program on the bus
		// cannot click a notification for the person.
		if err := c.AddMatchSignal(dbus.WithMatchSender(busName), dbus.WithMatchObjectPath(busPath), dbus.WithMatchInterface(busIface), dbus.WithMatchMember(member)); err != nil {
			_ = c.Close()
			return nil, false, err
		}
	}
	signals := make(chan *dbus.Signal, 16)
	c.Signal(signals)
	go follow(signals)
	session.conn, session.markup, session.keys, session.tags = c, slices.Contains(caps, "body-markup"), map[uint32]shown{}, map[string]uint32{}
	return c, session.markup, nil
}

// follow turns the server's signals into clicks and button presses, and
// forgets a closed notification. A press counts only for a button the
// notification offered.
func follow(signals <-chan *dbus.Signal) {
	for s := range signals {
		if len(s.Body) < 2 {
			continue
		}
		id, ok := s.Body[0].(uint32)
		if !ok {
			continue
		}
		switch s.Name {
		case busIface + ".ActionInvoked":
			action, _ := s.Body[1].(string)
			n, ok := lookup(id)
			if !ok {
				continue
			}
			if action != actionKey && !slices.Contains(n.actions, action) {
				continue
			}
			if !n.resident {
				forget(id)
			}
			if action == actionKey {
				clicked(n.key)
			} else {
				acted(n.key, action)
			}
		case busIface + ".NotificationClosed":
			forget(id)
		}
	}
}

func remember(id uint32, n shown) {
	session.Lock()
	defer session.Unlock()
	if _, ok := session.keys[id]; !ok {
		session.order = append(session.order, id)
	}
	session.keys[id] = n
	if n.tag != "" {
		if session.tags == nil {
			session.tags = map[string]uint32{}
		}
		session.tags[n.tag] = id
	}
	for len(session.order) > maxKeys {
		forgetLocked(session.order[0])
	}
}

func lookup(id uint32) (shown, bool) {
	session.Lock()
	defer session.Unlock()
	n, ok := session.keys[id]
	return n, ok
}

func forget(id uint32) string {
	session.Lock()
	defer session.Unlock()
	return forgetLocked(id)
}

func forgetLocked(id uint32) string {
	n := session.keys[id]
	delete(session.keys, id)
	session.order = slices.DeleteFunc(session.order, func(x uint32) bool { return x == id })
	if n.tag != "" && session.tags[n.tag] == id {
		delete(session.tags, n.tag)
	}
	return n.key
}

// escapeMarkup is text the server shows as written though it reads the body
// as markup: the three characters markup is made of, as entities.
func escapeMarkup(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
