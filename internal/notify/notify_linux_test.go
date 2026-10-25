package notify

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// fakeServer is a notification server on a private bus: it records each
// Notify and answers with ids from 1.
type fakeServer struct {
	mu     sync.Mutex
	caps   []string
	calls  []fakeCall
	closed []uint32
}

type fakeCall struct {
	app, icon, summary, body string
	actions                  []string
	hints                    map[string]dbus.Variant
	replaces                 uint32
	timeout                  int32
}

func (f *fakeServer) GetCapabilities() ([]string, *dbus.Error) { return f.caps, nil }

// Notify answers with a new id, or the replaced one's.
func (f *fakeServer) Notify(app string, replaces uint32, icon, summary, body string, actions []string, hints map[string]dbus.Variant, timeout int32) (uint32, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fakeCall{app, icon, summary, body, actions, hints, replaces, timeout})
	if replaces != 0 {
		return replaces, nil
	}
	return uint32(len(f.calls)), nil
}

func (f *fakeServer) CloseNotification(id uint32) *dbus.Error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, id)
	return nil
}

func (f *fakeServer) last(t *testing.T) fakeCall {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("no notification reached the server")
	}
	return f.calls[len(f.calls)-1]
}

// privateBus starts a dbus-daemon of the test's own, serves f on it as the
// notification server, and points this package at it. It answers the
// server's connection and the bus's address.
func privateBus(t *testing.T, f *fakeServer) (*dbus.Conn, string) {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("no dbus-daemon on this machine")
	}
	dir := t.TempDir()
	conf := filepath.Join(dir, "bus.conf")
	if err := os.WriteFile(conf, []byte(`<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig><type>session</type><listen>unix:dir=`+dir+`</listen>
<policy context="default"><allow send_destination="*" eavesdrop="true"/><allow eavesdrop="true"/><allow own="*"/></policy></busconfig>`), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(daemon, "--config-file="+conf, "--nofork", "--print-address=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	addr, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	addr = strings.TrimSpace(addr)

	server, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	if err := server.Export(f, busPath, busIface); err != nil {
		t.Fatal(err)
	}
	if reply, err := server.RequestName(busName, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("the fake server did not own the name: %v %v", reply, err)
	}

	was := connectBus
	connectBus = func(...dbus.ConnOption) (*dbus.Conn, error) { return dbus.Connect(addr) }
	t.Cleanup(func() {
		connectBus = was
		session.Lock()
		if session.conn != nil {
			_ = session.conn.Close()
		}
		session.conn, session.keys, session.order, session.tags = nil, nil, nil, nil
		session.Unlock()
	})
	return server, addr
}

func TestANotificationReachesTheServerAndItsClickComesBack(t *testing.T) {
	f := &fakeServer{caps: []string{"actions", "body"}}
	server, addr := privateBus(t, f)
	clicks := make(chan string, 1)
	SetOnClick(func(key string) { clicks <- key })
	t.Cleanup(func() { SetOnClick(nil) })

	if err := Show(Notice{Title: "Assistant: Morning summary", Body: "Three meetings & <b>one</b> deadline", Key: "assistant"}); err != nil {
		t.Fatal(err)
	}
	c := f.last(t)
	if c.app != appName || c.icon != appIcon || c.summary != "Assistant: Morning summary" {
		t.Errorf("notification %+v", c)
	}
	if c.body != "Three meetings & <b>one</b> deadline" {
		t.Errorf("a server that reads no markup got %q", c.body)
	}
	if len(c.actions) != 2 || c.actions[0] != actionKey {
		t.Errorf("actions %q", c.actions)
	}
	if e, _ := c.hints["desktop-entry"].Value().(string); e != appIcon {
		t.Errorf("desktop-entry %q", e)
	}

	// Another program's signal, another action, and another notification's
	// click open nothing.
	other, err := dbus.Connect(addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if err := other.Emit(busPath, busIface+".ActionInvoked", uint32(1), actionKey); err != nil {
		t.Fatal(err)
	}
	if err := server.Emit(busPath, busIface+".ActionInvoked", uint32(1), "Open"); err != nil {
		t.Fatal(err)
	}
	if err := server.Emit(busPath, busIface+".ActionInvoked", uint32(9), actionKey); err != nil {
		t.Fatal(err)
	}
	if err := server.Emit(busPath, busIface+".ActionInvoked", uint32(1), actionKey); err != nil {
		t.Fatal(err)
	}
	select {
	case k := <-clicks:
		if k != "assistant" {
			t.Errorf("click key %q", k)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the click did not come back")
	}
	// A closed notification is forgotten: its later click opens nothing.
	if err := Show(Notice{Title: "t", Body: "b", Key: "coder"}); err != nil {
		t.Fatal(err)
	}
	if err := server.Emit(busPath, busIface+".NotificationClosed", uint32(2), uint32(2)); err != nil {
		t.Fatal(err)
	}
	if err := server.Emit(busPath, busIface+".ActionInvoked", uint32(2), actionKey); err != nil {
		t.Fatal(err)
	}
	select {
	case k := <-clicks:
		t.Errorf("a closed notification's click opened %q", k)
	case <-time.After(500 * time.Millisecond):
	}
}

func TestABodyIsEscapedWhereTheServerReadsMarkup(t *testing.T) {
	f := &fakeServer{caps: []string{"actions", "body", "body-markup", "body-hyperlinks", "body-images"}}
	_, _ = privateBus(t, f)
	if err := Show(Notice{Title: "<b>t</b>", Body: `<a href="https://example.com">x</a> & <img src="https://example.com/i.png"/>`, Key: "assistant"}); err != nil {
		t.Fatal(err)
	}
	c := f.last(t)
	if c.body != `&lt;a href="https://example.com"&gt;x&lt;/a&gt; &amp; &lt;img src="https://example.com/i.png"/&gt;` {
		t.Errorf("body %q", c.body)
	}
	if c.summary != "<b>t</b>" {
		t.Errorf("the summary is plain text in the protocol and is sent as written: %q", c.summary)
	}
}

func TestNoServerIsAnError(t *testing.T) {
	was := connectBus
	connectBus = func(...dbus.ConnOption) (*dbus.Conn, error) { return nil, os.ErrNotExist }
	t.Cleanup(func() { connectBus = was })
	if err := Show(Notice{Title: "t", Key: "k"}); err == nil {
		t.Error("no bus, no error")
	}
}

func TestOnlyTheNewestKeysAreKept(t *testing.T) {
	session.Lock()
	session.keys, session.order = map[uint32]shown{}, nil
	session.Unlock()
	t.Cleanup(func() {
		session.Lock()
		session.keys, session.order = nil, nil
		session.Unlock()
	})
	for i := uint32(1); i <= maxKeys+5; i++ {
		remember(i, shown{key: "k"})
	}
	if forget(1) != "" || forget(maxKeys+5) != "k" {
		t.Error("the oldest keys are dropped and the newest kept")
	}
}

func TestButtonsReplacingAndWithdrawing(t *testing.T) {
	f := &fakeServer{caps: []string{"actions", "body"}}
	server, _ := privateBus(t, f)
	presses := make(chan string, 4)
	SetOnAction(func(key, action string) { presses <- key + " " + action })
	t.Cleanup(func() { SetOnAction(nil) })

	n := Notice{
		Title: "Record this meeting? (Zoom)", Body: "b", Key: "meeting-notes", Tag: "offer", Resident: true,
		Actions: []Action{{ID: "record:1", Label: "Record"}, {ID: "not-now:1", Label: "Not now"}, {ID: actionKey, Label: "a button that is the click"}},
	}
	if err := Show(n); err != nil {
		t.Fatal(err)
	}
	c := f.last(t)
	if strings.Join(c.actions, "|") != actionKey+"|Open|record:1|Record|not-now:1|Not now" {
		t.Errorf("actions %q", c.actions)
	}
	if r, _ := c.hints["resident"].Value().(bool); !r || c.timeout != 0 || c.replaces != 0 {
		t.Errorf("a resident notification: resident %v, timeout %d, replaces %d", r, c.timeout, c.replaces)
	}
	n.Title = "Record this meeting? (Zoom) again"
	if err := Show(n); err != nil {
		t.Fatal(err)
	}
	if c := f.last(t); c.replaces != 1 {
		t.Errorf("the same tag replaced id %d, want 1", c.replaces)
	}

	// A button it did not offer does nothing; one it offered comes back with
	// the key, and a resident notification's buttons keep working.
	for _, a := range []string{"stop", "record:1", "not-now:1"} {
		if err := server.Emit(busPath, busIface+".ActionInvoked", uint32(1), a); err != nil {
			t.Fatal(err)
		}
	}
	// Each press runs on a goroutine of its own, so they may arrive in
	// either order.
	got := map[string]bool{}
	for range 2 {
		select {
		case p := <-presses:
			got[p] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("the presses came back as %v", got)
		}
	}
	if !got["meeting-notes record:1"] || !got["meeting-notes not-now:1"] {
		t.Errorf("presses %v, want record:1 and not-now:1", got)
	}
	select {
	case p := <-presses:
		t.Errorf("a button it did not offer came back: %q", p)
	case <-time.After(200 * time.Millisecond):
	}

	if err := Withdraw("offer"); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	closed := append([]uint32(nil), f.closed...)
	f.mu.Unlock()
	if len(closed) != 1 || closed[0] != 1 {
		t.Errorf("closed %v, want [1]", closed)
	}
	if err := server.Emit(busPath, busIface+".ActionInvoked", uint32(1), "record:1"); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-presses:
		t.Errorf("a withdrawn notification's button came back: %q", got)
	case <-time.After(500 * time.Millisecond):
	}
	if err := Withdraw("nothing shown with this tag"); err != nil {
		t.Errorf("withdrawing what is not shown: %v", err)
	}
	if err := Show(Notice{Title: "t", Tag: "offer"}); err != nil {
		t.Fatal(err)
	}
	if c := f.last(t); c.replaces != 0 || c.timeout != -1 {
		t.Errorf("after the withdrawal: replaces %d, timeout %d, want a new notification with the server's timeout", c.replaces, c.timeout)
	}
}
