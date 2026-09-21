// Package webview holds what the app's window is allowed to do. The one
// HTML sink in the renderer is a README the registry served — sanitized
// there, taken as served here — and every Go binding is reachable from renderer
// JS, so a script that ran in this page would run with the local machine
// as its API. The Content-Security-Policy below is the second layer the
// review found missing (security review 2026-09-20 §8.3): no inline
// script, no script from anywhere but the app's own assets, no plugins,
// no framing.
package webview

import (
	"net/http"
	"strings"
)

// CSP is the policy every asset-server response carries.
//
//   - script-src 'self': the bundle only. The Wails IPC bridge is a user
//     script the webview injects, not page content, and is unaffected.
//   - style-src 'self' 'unsafe-inline': React sets style attributes; the
//     bundle's stylesheet is a file.
//   - img-src 'self' https: data:: README images are the registry's signed
//     image-proxy URLs on its https origin; icons are data: or bundled.
//   - connect-src 'self': the page fetches nothing itself — every request
//     goes through a Go binding.
//   - object-src 'none', frame-ancestors 'none', base-uri 'self': nothing
//     embeds, nothing frames this, nothing moves the document base.
var CSP = strings.Join([]string{
	"default-src 'self'",
	"script-src 'self'",
	"style-src 'self' 'unsafe-inline'",
	"img-src 'self' https: data:",
	"font-src 'self'",
	"connect-src 'self'",
	"object-src 'none'",
	"frame-ancestors 'none'",
	"base-uri 'self'",
	"form-action 'none'",
}, "; ")

// Middleware sets the policy on every response the asset server sends,
// including the index document and every bundled asset, before the next
// handler writes anything.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", CSP)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
