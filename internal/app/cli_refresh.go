package app

import (
	"log/slog"

	"github.com/blaktron/agentry-desktop/internal/clipath"
)

// refreshScriptCLI brings an older agentry CLI from the site's install
// script up to the one this app carries, once per start, and logs what it
// did. It never stops the app.
func refreshScriptCLI() {
	r, err := clipath.RefreshInstalled()
	switch {
	case err != nil:
		slog.Warn("cli refresh: the agentry CLI from the install script was not refreshed", "err", err)
	case r.Outcome == clipath.Replaced || r.Outcome == clipath.Kept:
		slog.Info("cli refresh: " + r.String())
	}
}
