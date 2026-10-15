// Browser sign-in: the desktop half. The app opens the registry's authorize
// page in the system browser — sign-in there goes through whatever the
// registry mandates (the directory service, MFA included) — and receives a
// freshly minted PAT on a one-shot loopback listener. The token then takes
// exactly the pasted-PAT path: verified against the registry, stored
// keychain-first, signing keys pinned on first use.
package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/blaktron/agentry-cli/pkg/authflow"

	"github.com/blaktron/agentry-desktop/internal/hostedrun"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// browserSignInMu serializes browser sign-ins: two concurrent flows would
// race the keychain write and confuse whichever browser tab finishes last.
var browserSignInMu sync.Mutex

// browserSignInCancel ends the in-flight flow's wait (nil when none is in
// flight). It exists for the Settings page's "Cancel sign-in": a closed
// browser window, or an authorize page that never completes, must not hold
// the app for the whole authflow timeout.
var (
	browserSignInCancelMu sync.Mutex
	browserSignInCancel   context.CancelFunc
)

// errSignInCancelled is what SignInViaBrowser returns after
// CancelBrowserSignIn; the frontend shows it as a notice, not a failure.
var errSignInCancelled = errors.New("browser sign-in cancelled")

// SignInViaBrowser runs the whole flow and blocks until the browser hands
// the token back, the user cancels, or the authflow timeout.
func (s *SettingsAPI) SignInViaBrowser(url string) (*SignInResult, error) {
	if !browserSignInMu.TryLock() {
		return nil, fmt.Errorf("a browser sign-in is already in progress — cancel it, or finish it in the browser first")
	}
	defer browserSignInMu.Unlock()

	url = settings.Normalize(url)
	if err := settings.ValidateURL(url); err != nil {
		return nil, err
	}
	// A registry the app keeps no sign-in for is refused before the browser
	// opens, so no token is minted that would then be thrown away.
	cfg, err := settings.Load()
	if err != nil {
		return nil, err
	}
	if err := cfg.Admit(url); err != nil {
		return nil, err
	}
	base := s.app.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	defer cancel()
	setBrowserSignInCancel(cancel)
	defer setBrowserSignInCancel(nil)

	flow, err := authflow.Start()
	if err != nil {
		return nil, err
	}
	defer flow.Close()
	if err := s.openExternal(authorizeURL(flow, url)); err != nil {
		return nil, fmt.Errorf("open the system browser: %w", err)
	}
	token, err := flow.Wait(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) && base.Err() == nil {
			return nil, errSignInCancelled
		}
		return nil, err
	}
	// From here it is an ordinary sign-in: whoami-verify, store, TOFU-pin.
	return s.SignIn(url, token)
}

// requestedScopes are the scopes this app's surfaces need, named on the
// authorize page's URL. Browse and install, Build → publish and identity are
// what the registry has always minted for a desktop sign-in; a run In
// Agentry adds run:read and run:write, Knowledge knowledge:read and
// knowledge:write, the Writing Voice voice:read and voice:write, and the
// Publisher section agent:manage. The registry decides what it grants — the
// scope set of a desktop grant is fixed server-side so no caller can widen
// it — so this names what the app asks for, and HostedAvailability reads
// back what the token actually carries. A registry that does not know the
// parameter ignores it, and every other surface keeps working with the token
// it mints.
var requestedScopes = []string{
	"agent:read", "agent:publish", "repo:read", "repo:write", "user:profile",
	hostedrun.ScopeRunRead, hostedrun.ScopeRunWrite,
	// The Knowledge panel's packs in Agentry, and the wrap and unwrap of
	// their keys; the core's desktop grant carries both.
	"knowledge:read", "knowledge:write",
	// The Writing Voice in the account, its Submit and Use, and the Voice
	// Writer In Agentry; the core's sign-in grant carries both.
	hostedrun.ScopeVoiceRead, hostedrun.ScopeVoiceWrite,
	// The Publisher section's owner actions: make a QuickAgent public or
	// private, retire, restore and delete it; the core's sign-in grant
	// carries it.
	"agent:manage",
}

// authorizeURL is the flow's authorize page plus the scopes the app asks
// for. AuthorizeURL also records the registry on the flow, so it is always
// called, never bypassed.
func authorizeURL(flow *authflow.Flow, registryURL string) string {
	u := flow.AuthorizeURL(registryURL)
	sep := "&"
	if !strings.Contains(u, "?") {
		sep = "?"
	}
	return u + sep + "scopes=" + url.QueryEscape(strings.Join(requestedScopes, " "))
}

// CancelBrowserSignIn abandons the in-flight browser sign-in, if there is
// one, and reports whether there was. The loopback listener closes with the
// flow, so a late click on the authorize page fails in the browser and
// nothing is stored; a new sign-in can start immediately.
func (s *SettingsAPI) CancelBrowserSignIn() bool {
	browserSignInCancelMu.Lock()
	defer browserSignInCancelMu.Unlock()
	if browserSignInCancel == nil {
		return false
	}
	browserSignInCancel()
	return true
}

func setBrowserSignInCancel(fn context.CancelFunc) {
	browserSignInCancelMu.Lock()
	browserSignInCancel = fn
	browserSignInCancelMu.Unlock()
}

func (s *SettingsAPI) openExternal(url string) error {
	if s.open != nil {
		return s.open(url)
	}
	return s.app.OpenExternal(url)
}
