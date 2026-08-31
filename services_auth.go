// Browser sign-in (core DEV-058): the desktop half. The app opens the
// registry's authorize page in the system browser — sign-in there goes
// through whatever the registry mandates (the directory service under
// DEV-057, MFA included) — and receives a freshly minted PAT on a one-shot
// loopback listener. The token then takes exactly the pasted-PAT path:
// verified against the registry, stored keychain-first, signing keys
// pinned on first use.
package main

import (
	"fmt"
	"sync"

	"github.com/blaktron/agentry-desktop/internal/authflow"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// browserSignInMu serializes browser sign-ins: two concurrent flows would
// race the keychain write and confuse whichever browser tab finishes last.
var browserSignInMu sync.Mutex

// SignInViaBrowser runs the whole flow and blocks until the browser hands
// the token back (or the authflow timeout).
func (s *SettingsAPI) SignInViaBrowser(url string) (*SignInResult, error) {
	if !browserSignInMu.TryLock() {
		return nil, fmt.Errorf("a browser sign-in is already in progress — finish or close that tab first")
	}
	defer browserSignInMu.Unlock()

	url = settings.Normalize(url)
	if err := settings.ValidateURL(url); err != nil {
		return nil, err
	}
	flow, err := authflow.Start()
	if err != nil {
		return nil, err
	}
	defer flow.Close()
	if err := s.app.OpenExternal(flow.AuthorizeURL(url)); err != nil {
		return nil, fmt.Errorf("open the system browser: %w", err)
	}
	token, err := flow.Wait(s.app.ctx)
	if err != nil {
		return nil, err
	}
	// From here it is an ordinary sign-in: whoami-verify, store, TOFU-pin.
	return s.SignIn(url, token)
}
