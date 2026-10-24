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

// session is the connection this package keeps to the session bus, its own
// so that the signals it subscribes to are not another library's.
var session struct {
	sync.Mutex
	conn *dbus.Conn
	// markup is the server's "body-markup" capability: a body it reads as
	// markup is escaped, and one it shows as written is not.
	markup bool
	// keys are the clicks' keys by notification id, oldest first.
	keys  map[uint32]string
	order []uint32
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
	var id uint32
	err = c.Object(busName, busPath).Call(busIface+".Notify", 0,
		appName, uint32(0), appIcon, n.Title, body, []string{actionKey, "Open"}, hints, int32(-1)).Store(&id)
	if err != nil {
		// The next notification connects again: the server or the bus may
		// have restarted under this connection.
		drop(c)
		return fmt.Errorf("the desktop's notification service refused the notification: %w", err)
	}
	remember(id, n.Key)
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
	session.conn, session.markup, session.keys = c, slices.Contains(caps, "body-markup"), map[uint32]string{}
	return c, session.markup, nil
}

// follow turns the server's signals into clicks and forgets a closed
// notification's key.
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
			if action, _ := s.Body[1].(string); action == actionKey {
				clicked(forget(id))
			}
		case busIface + ".NotificationClosed":
			forget(id)
		}
	}
}

func remember(id uint32, key string) {
	session.Lock()
	defer session.Unlock()
	if _, ok := session.keys[id]; !ok {
		session.order = append(session.order, id)
	}
	session.keys[id] = key
	for len(session.order) > maxKeys {
		delete(session.keys, session.order[0])
		session.order = session.order[1:]
	}
}

func forget(id uint32) string {
	session.Lock()
	defer session.Unlock()
	key := session.keys[id]
	delete(session.keys, id)
	session.order = slices.DeleteFunc(session.order, func(x uint32) bool { return x == id })
	return key
}

// escapeMarkup is text the server shows as written though it reads the body
// as markup: the three characters markup is made of, as entities.
func escapeMarkup(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
