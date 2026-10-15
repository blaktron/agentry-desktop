// Connections: the run setup's Connect tools signs in to each declared MCP
// server that signs in by OAuth, in the person's browser, the first time a
// run of that QuickAgent version needs it; Settings → Connections lists what
// is kept, with Disconnect. internal/run holds the store; this is the app's
// half: the browser, the address pasted back, the events the page follows.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"

	"github.com/blaktron/agentry-cli/pkg/installed"
	"github.com/blaktron/agentry-cli/pkg/runner/installdir"

	"github.com/blaktron/agentry-desktop/internal/install"
	"github.com/blaktron/agentry-desktop/internal/run"
)

// Events a consent emits while it waits on the person: the vendor's
// consent address once known (connectURLEvent), and what the consent says
// along the way (connectSayEvent). Neither ever carries a token.
const (
	connectURLEvent = "connect:url"
	connectSayEvent = "connect:say"
)

// ConnectURLView is the consent address the browser was sent to, for the
// page to show beside "paste the address it ended on".
type ConnectURLView struct {
	Server string `json:"server"`
	URL    string `json:"url"`
}

// ConnectSayView is one line the consent says.
type ConnectSayView struct {
	Server string `json:"server"`
	Line   string `json:"line"`
}

// ConnectView is how one Connect ended: the connection made, the steps
// for the person's own OAuth client where the vendor needs one (Google),
// or a cancel.
type ConnectView struct {
	Connection     *run.ServerConnection `json:"connection,omitempty"`
	NeedsOwnClient *run.OwnClientGuide   `json:"needsOwnClient,omitempty"`
	Cancelled      bool                  `json:"cancelled,omitempty"`
}

// KeptConnectionsView is Settings → Connections: never a token or a
// client secret.
type KeptConnectionsView struct {
	Connections []run.KeptConnection `json:"connections"`
	Clients     []run.KeptClient     `json:"clients"`
}

// consent is the one sign-in in progress in this app: two at once would
// leave the person unsure which browser tab answers which.
type consent struct {
	// mu is held for the whole consent; cmu guards the two fields below,
	// which PasteRedirect and CancelConnect read while it runs.
	mu     sync.Mutex
	cmu    sync.Mutex
	cancel context.CancelFunc
	pasted chan string
}

var consentNow consent

// runConnect is the consent itself; a test substitutes its own.
var runConnect = run.Connect

// ReleaseConnections is the connection state of each server of ref's
// release that signs in by OAuth: what Connect tools draws.
func (r *RunAPI) ReleaseConnections(reg, ref string) (*run.ReleaseConnections, error) {
	return run.ConnectionsFor(r.ctx(), reg, ref)
}

// Connect signs in to server for ref's release: the vendor's consent
// screen in the person's browser, its address also sent to the page
// (connectURLEvent) so it can be opened on another computer, whose
// redirect is then pasted back with PasteRedirect. It blocks until the
// vendor's answer, a cancel or the consent's timeout. A vendor that needs
// the person's own OAuth client, where none is kept, answers with
// Agentry's steps for making one; KeepOwnClient keeps it, and Connect is
// called again.
func (r *RunAPI) Connect(reg, ref, server string) (*ConnectView, error) {
	if !consentNow.mu.TryLock() {
		return nil, errors.New("a sign-in is already under way: finish it in the browser, or cancel it")
	}
	defer consentNow.mu.Unlock()
	ctx, cancel := context.WithCancel(r.ctx())
	defer cancel()
	pasted := make(chan string, 1)
	setConsent(cancel, pasted)
	defer setConsent(nil, nil)

	ui := run.ConsentUI{
		Pasted: pasted,
		Say:    func(line string) { emitEvent(r.app, connectSayEvent, ConnectSayView{Server: server, Line: line}) },
		Present: func(authURL string) {
			// pkg/mcpauth holds the vendor's authorization endpoint to https;
			// this app holds the address it hands the OS to it again.
			if u, err := url.Parse(authURL); err != nil || u.Scheme != "https" || u.Host == "" {
				emitEvent(r.app, connectSayEvent, ConnectSayView{Server: server,
					Line: "The vendor's consent address is not an https address, so it was not opened."})
				return
			}
			emitEvent(r.app, connectURLEvent, ConnectURLView{Server: server, URL: authURL})
			if err := r.openURL(authURL); err != nil {
				emitEvent(r.app, connectSayEvent, ConnectSayView{Server: server,
					Line: "The browser could not be opened (" + err.Error() + "): open the address below yourself."})
			}
		},
	}
	c, err := runConnect(ctx, reg, ref, server, ui)
	var own *run.NeedsOwnClient
	switch {
	case errors.As(err, &own):
		g := own.Guide
		return &ConnectView{NeedsOwnClient: &g}, nil
	case err != nil && errors.Is(ctx.Err(), context.Canceled) && r.ctx().Err() == nil:
		return &ConnectView{Cancelled: true}, nil
	case err != nil:
		return nil, err
	}
	return &ConnectView{Connection: c}, nil
}

// PasteRedirect hands the consent in progress the address the browser ended
// on, where it was on another computer and that page did not load. The
// consent checks its state before it takes the code in it; an address that
// is not this sign-in's is said and waited past.
func (r *RunAPI) PasteRedirect(address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return errors.New("paste the whole address from the browser's address bar")
	}
	consentNow.cmu.Lock()
	ch := consentNow.pasted
	consentNow.cmu.Unlock()
	if ch == nil {
		return errors.New("no sign-in is waiting for an address")
	}
	select {
	case ch <- address:
		return nil
	default:
		return errors.New("the sign-in is still reading the last address you pasted")
	}
}

// CancelConnect ends the consent in progress, if there is one, and says
// whether there was. Its loopback listener closes with it, so a late
// answer in the browser keeps nothing.
func (r *RunAPI) CancelConnect() bool {
	consentNow.cmu.Lock()
	defer consentNow.cmu.Unlock()
	if consentNow.cancel == nil {
		return false
	}
	consentNow.cancel()
	return true
}

// KeepOwnClient keeps the person's own OAuth client for issuer: one per
// vendor on this computer, used by every later consent there. The secret
// goes to the OS keychain (a 0600 file where none answers) and is never sent
// back to the page.
func (r *RunAPI) KeepOwnClient(issuer, clientID, secret string) error {
	if strings.TrimSpace(clientID) == "" {
		return errors.New("enter the client ID")
	}
	return run.SetOwnClient(issuer, clientID, secret)
}

// Connections is every connection and own OAuth client kept on this
// computer, the CLI's included (they share the runner home).
func (r *RunAPI) Connections() (*KeptConnectionsView, error) {
	conns, clients, err := run.KeptConnections()
	if err != nil {
		return nil, err
	}
	return &KeptConnectionsView{Connections: conns, Clients: clients}, nil
}

// Disconnect removes one kept connection and revokes it at the vendor where
// it offers that; it returns the revocation's outcome.
func (r *RunAPI) Disconnect(ref, digest, server string) (string, error) {
	return run.Disconnect(r.ctx(), ref, digest, server)
}

// ForgetOwnClient forgets the person's OAuth client for issuer. The
// connections made with it stop refreshing and ask again.
func (r *RunAPI) ForgetOwnClient(issuer string) (bool, error) {
	return run.ForgetOwnClient(issuer)
}

func setConsent(cancel context.CancelFunc, pasted chan string) {
	consentNow.cmu.Lock()
	consentNow.cancel, consentNow.pasted = cancel, pasted
	consentNow.cmu.Unlock()
}

func (r *RunAPI) openURL(u string) error {
	if r.openBrowser != nil {
		return r.openBrowser(u)
	}
	return r.app.OpenExternal(u)
}

// removeConnectionsIfGone removes the connections of ns/name once no install
// of it is left in the runner home: after the app uninstalls its last runner
// install. It returns what it removed, for the uninstall's message, and
// never fails the uninstall.
func removeConnectionsIfGone(k installed.Key) string {
	if k.Target != installdir.Target {
		return ""
	}
	items, err := install.List()
	if err != nil {
		slog.Warn("connections: not checked after an uninstall", "ref", k.Namespace+"/"+k.Name, "err", err)
		return ""
	}
	for _, it := range items {
		if it.Present && it.Target == installdir.Target && it.Namespace == k.Namespace && it.Name == k.Name {
			return ""
		}
	}
	removed, err := run.RemoveConnectionsOf(context.Background(), k.Namespace, k.Name)
	if err != nil {
		slog.Warn("connections: not all removed after an uninstall", "ref", k.Namespace+"/"+k.Name, "err", err)
	}
	if len(removed) == 0 {
		return ""
	}
	return fmt.Sprintf("; %s", strings.Join(removed, "; "))
}
