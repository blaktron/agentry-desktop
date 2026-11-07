//go:build linux

package stopkey

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func bind(ctx context.Context, stop func()) (Binding, error) {
	if os.Getenv("WAYLAND_DISPLAY") != "" && !strings.EqualFold(os.Getenv("AGENTRY_DESKTOP_COMPUTER_USE_DISPLAY"), "x11") {
		return bindPortal(ctx, stop)
	}
	if os.Getenv("DISPLAY") != "" {
		return bindX11(stop)
	}
	return nil, errors.New("no graphical session to bind the stop key in")
}

// portalBinding is the stop key through the GlobalShortcuts portal.
type portalBinding struct {
	bus     *dbus.Conn
	session dbus.ObjectPath
	once    sync.Once
}

func (p *portalBinding) Close() error {
	p.once.Do(func() {
		_ = p.bus.Object("org.freedesktop.portal.Desktop", p.session).Call("org.freedesktop.portal.Session.Close", 0).Err
		_ = p.bus.Close()
	})
	return nil
}

func bindPortal(ctx context.Context, stop func()) (Binding, error) {
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	obj := bus.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	// The portal binds a shortcut only for a known application: a process
	// outside a sandbox says which one it is by its desktop entry's name.
	_ = obj.CallWithContext(ctx, "org.freedesktop.host.portal.Registry.Register", 0, DesktopEntry, map[string]dbus.Variant{}).Err
	const iface = "org.freedesktop.portal.GlobalShortcuts"
	if v, err := obj.GetProperty(iface + ".version"); err != nil || v.Value() == nil {
		_ = bus.Close()
		return nil, errors.New("the desktop offers no global shortcuts portal")
	}
	signals := make(chan *dbus.Signal, 16)
	bus.Signal(signals)
	if err := bus.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")); err != nil {
		_ = bus.Close()
		return nil, err
	}
	if err := bus.AddMatchSignal(dbus.WithMatchInterface(iface), dbus.WithMatchMember("Activated")); err != nil {
		_ = bus.Close()
		return nil, err
	}
	wait := func(method string, args ...any) (map[string]dbus.Variant, error) {
		var handle dbus.ObjectPath
		if err := obj.CallWithContext(ctx, iface+"."+method, 0, args...).Store(&handle); err != nil {
			return nil, err
		}
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case s := <-signals:
				if s.Path != handle || len(s.Body) < 2 {
					continue
				}
				if code, _ := s.Body[0].(uint32); code != 0 {
					return nil, fmt.Errorf("%s was not allowed", method)
				}
				res, _ := s.Body[1].(map[string]dbus.Variant)
				return res, nil
			}
		}
	}
	res, err := wait("CreateSession", map[string]dbus.Variant{
		"handle_token":         dbus.MakeVariant("agentry_stopkey_c"),
		"session_handle_token": dbus.MakeVariant("agentry_stopkey"),
	})
	if err != nil {
		_ = bus.Close()
		return nil, err
	}
	session := dbus.ObjectPath(fmt.Sprint(res["session_handle"].Value()))
	shortcuts := []struct {
		ID   string
		Opts map[string]dbus.Variant
	}{{ID: "stop-computer-use", Opts: map[string]dbus.Variant{
		"description":       dbus.MakeVariant("Stop every agent's use of your computer"),
		"preferred_trigger": dbus.MakeVariant(Trigger),
	}}}
	if _, err := wait("BindShortcuts", session, shortcuts, "", map[string]dbus.Variant{"handle_token": dbus.MakeVariant("agentry_stopkey_b")}); err != nil {
		_ = bus.Close()
		return nil, err
	}
	p := &portalBinding{bus: bus, session: session}
	go func() {
		for s := range signals {
			if s.Name == iface+".Activated" && len(s.Body) >= 2 {
				if path, _ := s.Body[0].(dbus.ObjectPath); path == session {
					if id, _ := s.Body[1].(string); id == "stop-computer-use" {
						stop()
					}
				}
			}
		}
	}()
	return p, nil
}

// x11Binding is the stop key grabbed on the X root window.
type x11Binding struct {
	conn *xgb.Conn
	once sync.Once
}

func (x *x11Binding) Close() error {
	x.once.Do(func() { x.conn.Close() })
	return nil
}

func bindX11(stop func()) (Binding, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	root := xproto.Setup(conn).DefaultScreen(conn).Root
	setup := xproto.Setup(conn)
	km, err := xproto.GetKeyboardMapping(conn, setup.MinKeycode, byte(setup.MaxKeycode-setup.MinKeycode+1)).Reply()
	if err != nil {
		conn.Close()
		return nil, err
	}
	const escape = 0xff1b
	var code xproto.Keycode
	per := int(km.KeysymsPerKeycode)
	for i := 0; i*per < len(km.Keysyms); i++ {
		if km.Keysyms[i*per] == escape {
			code = xproto.Keycode(int(setup.MinKeycode) + i)
			break
		}
	}
	if code == 0 {
		conn.Close()
		return nil, errors.New("the keyboard has no Escape key")
	}
	mods := uint16(xproto.ModMaskControl | xproto.ModMaskShift)
	// Caps Lock and Num Lock change the modifier state; the key is grabbed
	// under each of their combinations.
	for _, extra := range []uint16{0, xproto.ModMaskLock, xproto.ModMask2, xproto.ModMaskLock | xproto.ModMask2} {
		if err := xproto.GrabKeyChecked(conn, true, root, mods|extra, code, xproto.GrabModeAsync, xproto.GrabModeAsync).Check(); err != nil {
			conn.Close()
			return nil, fmt.Errorf("the stop key is taken by another program: %v", err)
		}
	}
	x := &x11Binding{conn: conn}
	go func() {
		for {
			ev, err := conn.WaitForEvent()
			if ev == nil && err == nil {
				return
			}
			if kp, ok := ev.(xproto.KeyPressEvent); ok && kp.Detail == code {
				stop()
			}
		}
	}()
	return x, nil
}
