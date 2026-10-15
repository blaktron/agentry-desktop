package mcpview

import (
	"strings"

	"github.com/blaktron/agentry-cli/pkg/runner/browser"
	"github.com/blaktron/agentry/pkg/manifest"
)

// The browser (capabilities.browser, V15): a headless Chromium a run drives,
// held to the sites the person names and the declared hosts, or any public
// site. The install review, the consent dialog, the run setup's capability
// lines and the agent page's tools all say it in these words, so they cannot
// differ.

// BrowserReach is where a declared browser may go, in the CLI's words
// (agentry-cli cmd/agentry/caps.go browserLine, replicated because
// cmd/agentry is package main): the site the person names for each
// question with its subdomains, the declared hosts, and any public
// site, joined with ", and ".
func BrowserReach(b *manifest.Browser) string {
	if b == nil {
		return ""
	}
	var where []string
	if len(b.Sites) > 0 {
		where = append(where, "the site you name for "+strings.Join(b.Sites, ", ")+" and its subdomains")
	}
	if len(b.Hosts) > 0 {
		where = append(where, strings.Join(b.Hosts, ", "))
	}
	if b.IsPublic() {
		where = append(where, "ANY public site (policy-gated)")
	}
	if len(where) == 0 {
		return "declared"
	}
	return strings.Join(where, ", and ")
}

// BrowserLine is the CLI's install consent line for the browser, word for
// word, which the site's consent row shares: where it may go, and the
// declared secrets it may type, by name.
func BrowserLine(b *manifest.Browser) string {
	if b == nil {
		return ""
	}
	line := "a web browser that can open " + BrowserReach(b)
	if len(b.Secrets) > 0 {
		line += "; it can type " + strings.Join(b.Secrets, ", ") + " into the site's sign-in forms (it never sees them)"
	}
	return line
}

// BrowserTools is the browser server's tools the runner's allowlist passes
// for b (agentry-cli pkg/runner/browser Tools): without
// browser_evaluate when the browser may type secrets. None for no
// browser.
func BrowserTools(b *manifest.Browser) []string {
	if b == nil {
		return nil
	}
	return browser.Tools(len(b.Secrets) > 0)
}
