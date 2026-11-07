// Package portal speaks to the desktop portal (xdg-desktop-portal) for
// computer use on Wayland: the ScreenCast portal, which shares a window or
// a display as a PipeWire stream once the person picks it, and the
// RemoteDesktop portal, which posts pointer and keyboard input once the
// person allows it. A restore token from each lets a later session start
// without asking again, where the portal keeps the person's choice.
package portal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
)

const (
	busName    = "org.freedesktop.portal.Desktop"
	objectPath = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	ifaceSC    = "org.freedesktop.portal.ScreenCast"
	ifaceRD    = "org.freedesktop.portal.RemoteDesktop"
	ifaceReq   = "org.freedesktop.portal.Request"
	ifaceSess  = "org.freedesktop.portal.Session"
)

// Source types of the ScreenCast portal.
const (
	SourceMonitor uint32 = 1
	SourceWindow  uint32 = 2
)

// Device types of the RemoteDesktop portal.
const (
	DeviceKeyboard uint32 = 1
	DevicePointer  uint32 = 2
)

// persistUntilRevoked keeps the person's choice until they revoke it.
const persistUntilRevoked uint32 = 2

// ErrCancelled says the person dismissed the portal's dialog.
var ErrCancelled = errors.New("the request was cancelled in the portal's dialog")

// ErrNoPortal says the portal or one of its interfaces is not there.
var ErrNoPortal = errors.New("the desktop portal does not offer it")

// Conn is a session bus connection to the portal.
type Conn struct {
	bus *dbus.Conn
	obj dbus.BusObject
	n   atomic.Uint64
}

// Connect opens a private connection to the session bus.
func Connect() (*Conn, error) {
	bus, err := dbus.ConnectSessionBus(dbus.WithSignalHandler(dbus.NewSequentialSignalHandler()))
	if err != nil {
		return nil, fmt.Errorf("%w: the session bus: %v", ErrNoPortal, err)
	}
	return &Conn{bus: bus, obj: bus.Object(busName, objectPath)}, nil
}

// Close closes the connection; every session on it ends with it.
func (c *Conn) Close() error { return c.bus.Close() }

// Bus is the underlying connection.
func (c *Conn) Bus() *dbus.Conn { return c.bus }

// Version is an interface's version, or 0 where the portal lacks it.
func (c *Conn) Version(iface string) uint32 {
	v, err := c.obj.GetProperty(iface + ".version")
	if err != nil {
		return 0
	}
	n, _ := v.Value().(uint32)
	return n
}

// SourceTypes are the ScreenCast portal's available source types.
func (c *Conn) SourceTypes() uint32 {
	v, err := c.obj.GetProperty(ifaceSC + ".AvailableSourceTypes")
	if err != nil {
		return 0
	}
	n, _ := v.Value().(uint32)
	return n
}

func (c *Conn) token() string {
	return fmt.Sprintf("agentry_%d_%d", os.Getpid(), c.n.Add(1))
}

// request calls a portal method that answers through a Request object and
// waits for its Response.
func (c *Conn) request(ctx context.Context, method string, opts map[string]dbus.Variant, args ...any) (map[string]dbus.Variant, error) {
	names := c.bus.Names()
	if len(names) == 0 {
		return nil, errors.New("the session bus gave no name")
	}
	sender := strings.ReplaceAll(strings.TrimPrefix(names[0], ":"), ".", "_")
	tok := c.token()
	opts["handle_token"] = dbus.MakeVariant(tok)
	want := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + sender + "/" + tok)
	ch := make(chan *dbus.Signal, 8)
	c.bus.Signal(ch)
	defer c.bus.RemoveSignal(ch)
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(want), dbus.WithMatchInterface(ifaceReq), dbus.WithMatchMember("Response")}
	if err := c.bus.AddMatchSignalContext(ctx, match...); err != nil {
		return nil, err
	}
	defer func() { _ = c.bus.RemoveMatchSignal(match...) }()
	var handle dbus.ObjectPath
	if err := c.obj.CallWithContext(ctx, method, 0, append(args, opts)...).Store(&handle); err != nil {
		return nil, fmt.Errorf("%s: %w", method, err)
	}
	for {
		select {
		case <-ctx.Done():
			_ = c.bus.Object(busName, handle).Call(ifaceReq+".Close", 0).Err
			return nil, ctx.Err()
		case s, ok := <-ch:
			if !ok {
				return nil, errors.New("the session bus closed")
			}
			if s.Path != handle || s.Name != ifaceReq+".Response" || len(s.Body) < 2 {
				continue
			}
			code, _ := s.Body[0].(uint32)
			res, _ := s.Body[1].(map[string]dbus.Variant)
			switch code {
			case 0:
				return res, nil
			case 1:
				return nil, ErrCancelled
			default:
				return nil, fmt.Errorf("%s: the portal ended the request", method)
			}
		}
	}
}

// Stream is one stream a session shares.
type Stream struct {
	Node uint32
	// Source is SourceMonitor or SourceWindow, where the portal says.
	Source uint32
	// Position and Size, where the portal says: a monitor's position in
	// the compositor's space, and the stream's size.
	X, Y, Width, Height int32
}

// Session is a portal session: a ScreenCast session, a RemoteDesktop
// session, or a RemoteDesktop session that also shares streams.
type Session struct {
	c       *Conn
	handle  dbus.ObjectPath
	remote  bool
	streams []Stream
	// Restore is the restore token the portal gave at Start, for the next
	// session to start without asking.
	Restore string
	once    sync.Once
}

// Streams are the session's streams after Start.
func (s *Session) Streams() []Stream { return s.streams }

// Close ends the session.
func (s *Session) Close() error {
	var err error
	s.once.Do(func() {
		err = s.c.bus.Object(busName, s.handle).Call(ifaceSess+".Close", 0).Err
	})
	return err
}

func (c *Conn) create(ctx context.Context, iface string) (*Session, error) {
	if c.Version(iface) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoPortal, iface)
	}
	res, err := c.request(ctx, iface+".CreateSession", map[string]dbus.Variant{"session_handle_token": dbus.MakeVariant(c.token())})
	if err != nil {
		return nil, err
	}
	h, _ := res["session_handle"].Value().(string)
	if h == "" {
		return nil, errors.New("the portal created no session")
	}
	return &Session{c: c, handle: dbus.ObjectPath(h), remote: iface == ifaceRD}, nil
}

// ShareOptions are what a session asks to share.
type ShareOptions struct {
	// Types are SourceMonitor, SourceWindow or both.
	Types uint32
	// Restore is a restore token from an earlier session.
	Restore string
	// CursorHidden leaves the pointer out of the stream.
	CursorHidden bool
}

func (o ShareOptions) variant() map[string]dbus.Variant {
	m := map[string]dbus.Variant{
		"types":        dbus.MakeVariant(o.Types),
		"multiple":     dbus.MakeVariant(false),
		"persist_mode": dbus.MakeVariant(persistUntilRevoked),
	}
	if o.CursorHidden {
		m["cursor_mode"] = dbus.MakeVariant(uint32(1))
	}
	if o.Restore != "" {
		m["restore_token"] = dbus.MakeVariant(o.Restore)
	}
	return m
}

// ScreenCast starts a ScreenCast session sharing what the person picks in
// the portal's dialog, or what a restore token names without asking.
func (c *Conn) ScreenCast(ctx context.Context, o ShareOptions) (*Session, error) {
	s, err := c.create(ctx, ifaceSC)
	if err != nil {
		return nil, err
	}
	if _, err := c.request(ctx, ifaceSC+".SelectSources", o.variant(), s.handle); err != nil {
		_ = s.Close()
		return nil, err
	}
	if err := s.start(ctx, ifaceSC); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

// RemoteOptions are what a RemoteDesktop session asks for.
type RemoteOptions struct {
	// Restore is a restore token from an earlier session.
	Restore string
	// Share, when Types is set, also shares a stream in the same session,
	// which absolute pointer motion is relative to.
	Share ShareOptions
}

// RemoteDesktop starts a RemoteDesktop session for the keyboard and the
// pointer, and, where asked, a stream shared with it.
func (c *Conn) RemoteDesktop(ctx context.Context, o RemoteOptions) (*Session, error) {
	s, err := c.create(ctx, ifaceRD)
	if err != nil {
		return nil, err
	}
	dev := map[string]dbus.Variant{
		"types":        dbus.MakeVariant(DeviceKeyboard | DevicePointer),
		"persist_mode": dbus.MakeVariant(persistUntilRevoked),
	}
	if o.Restore != "" {
		dev["restore_token"] = dbus.MakeVariant(o.Restore)
	}
	if _, err := c.request(ctx, ifaceRD+".SelectDevices", dev, s.handle); err != nil {
		_ = s.Close()
		return nil, err
	}
	if o.Share.Types != 0 {
		share := o.Share.variant()
		delete(share, "persist_mode")
		delete(share, "restore_token")
		if _, err := c.request(ctx, ifaceSC+".SelectSources", share, s.handle); err != nil {
			_ = s.Close()
			return nil, err
		}
	}
	if err := s.start(ctx, ifaceRD); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Session) start(ctx context.Context, iface string) error {
	res, err := s.c.request(ctx, iface+".Start", map[string]dbus.Variant{}, s.handle, "")
	if err != nil {
		return err
	}
	if v, ok := res["restore_token"]; ok {
		s.Restore, _ = v.Value().(string)
	}
	if iface == ifaceRD {
		if v, ok := res["devices"]; ok {
			if d, _ := v.Value().(uint32); d&DevicePointer == 0 || d&DeviceKeyboard == 0 {
				return fmt.Errorf("%w: the pointer and the keyboard were not both allowed", ErrCancelled)
			}
		}
	}
	if v, ok := res["streams"]; ok {
		s.streams = parseStreams(v)
	}
	return nil
}

func parseStreams(v dbus.Variant) []Stream {
	var out []Stream
	raw, ok := v.Value().([][]any)
	if !ok {
		return nil
	}
	for _, item := range raw {
		if len(item) != 2 {
			continue
		}
		node, _ := item[0].(uint32)
		props, _ := item[1].(map[string]dbus.Variant)
		st := Stream{Node: node}
		if p, ok := props["source_type"]; ok {
			st.Source, _ = p.Value().(uint32)
		}
		if p, ok := props["position"]; ok {
			if xy, ok := p.Value().([]any); ok && len(xy) == 2 {
				st.X, _ = xy[0].(int32)
				st.Y, _ = xy[1].(int32)
			}
		}
		if p, ok := props["size"]; ok {
			if wh, ok := p.Value().([]any); ok && len(wh) == 2 {
				st.Width, _ = wh[0].(int32)
				st.Height, _ = wh[1].(int32)
			}
		}
		out = append(out, st)
	}
	return out
}

// PipeWire opens the PipeWire remote the session's streams play on. The
// caller owns the file descriptor.
func (s *Session) PipeWire(ctx context.Context) (int, error) {
	var fd dbus.UnixFD
	if err := s.c.obj.CallWithContext(ctx, ifaceSC+".OpenPipeWireRemote", 0, s.handle, map[string]dbus.Variant{}).Store(&fd); err != nil {
		return -1, fmt.Errorf("open the stream: %w", err)
	}
	return int(fd), nil
}

func (s *Session) notify(ctx context.Context, method string, args ...any) error {
	if !s.remote {
		return errors.New("a ScreenCast session posts no input")
	}
	return s.c.obj.CallWithContext(ctx, ifaceRD+"."+method, 0, append([]any{s.handle, map[string]dbus.Variant{}}, args...)...).Err
}

// MotionAbsolute moves the pointer to a position in a stream of the
// session, in the stream's own coordinates.
func (s *Session) MotionAbsolute(ctx context.Context, node uint32, x, y float64) error {
	return s.notify(ctx, "NotifyPointerMotionAbsolute", node, x, y)
}

// Motion moves the pointer by a distance.
func (s *Session) Motion(ctx context.Context, dx, dy float64) error {
	return s.notify(ctx, "NotifyPointerMotion", dx, dy)
}

// Linux input event codes of the mouse buttons.
const (
	BtnLeft   int32 = 0x110
	BtnRight  int32 = 0x111
	BtnMiddle int32 = 0x112
)

// Button presses or releases a button by its Linux input event code.
func (s *Session) Button(ctx context.Context, code int32, down bool) error {
	state := uint32(0)
	if down {
		state = 1
	}
	return s.notify(ctx, "NotifyPointerButton", code, state)
}

// Scroll scrolls by steps of the wheel on an axis: 0 vertical, 1
// horizontal; a positive step is down or right.
func (s *Session) Scroll(ctx context.Context, axis uint32, steps int32) error {
	return s.notify(ctx, "NotifyPointerAxisDiscrete", axis, steps)
}

// Keysym presses or releases the key of an X keysym.
func (s *Session) Keysym(ctx context.Context, keysym uint32, down bool) error {
	state := uint32(0)
	if down {
		state = 1
	}
	return s.notify(ctx, "NotifyKeyboardKeysym", int32(keysym), state)
}
