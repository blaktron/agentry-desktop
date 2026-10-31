package app

// Whether the registry still accepts the sign-in this app holds. A stored
// token stands for a sign-in until the registry says otherwise: its 401 to
// that token (registry.Unauthorized, from the one transport under every
// token-bearing client) marks the registry rejected, the settings view then
// shows it signed out with Sign in, and the page is told so it reads the
// settings again. Signing in again, or out, clears the mark.

import (
	"sync"

	"github.com/blaktron/agentry-desktop/internal/registry"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// eventAuthChanged tells the page a registry's sign-in state changed under
// it, with the registry's origin.
const eventAuthChanged = "auth:changed"

var (
	rejectedMu     sync.Mutex
	rejectedTokens = map[string]bool{}
)

// tokenRejected says the registry answered 401 to the token stored for it,
// and nothing has cleared that since.
func tokenRejected(regURL string) bool {
	rejectedMu.Lock()
	defer rejectedMu.Unlock()
	return rejectedTokens[settings.Normalize(regURL)]
}

// markTokenRejected records the registry's 401 and reports whether it is
// news.
func markTokenRejected(regURL string) bool {
	reg := settings.Normalize(regURL)
	rejectedMu.Lock()
	defer rejectedMu.Unlock()
	if rejectedTokens[reg] {
		return false
	}
	rejectedTokens[reg] = true
	return true
}

// tokenAccepted clears the mark: a sign-in stored a token the registry
// accepted, or signed out.
func tokenAccepted(regURL string) {
	rejectedMu.Lock()
	delete(rejectedTokens, settings.Normalize(regURL))
	rejectedMu.Unlock()
}

// watchAuth hands the transport's rejections to the mark and the page.
func watchAuth() {
	registry.Unauthorized = func(origin string) {
		if markTokenRejected(origin) {
			nativeEmit(eventAuthChanged, settings.Normalize(origin))
		}
	}
}
