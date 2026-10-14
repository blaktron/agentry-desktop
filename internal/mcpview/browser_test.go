package mcpview

import (
	"slices"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

// The browser's consent line is the CLI's word for word (agentry-cli
// cmd/agentry/caps.go browserLine), which the site's consent row shares
// (agentry-web#1644).
func TestBrowserLineIsTheCLIs(t *testing.T) {
	cases := map[string]struct {
		b    *manifest.Browser
		want string
	}{
		"site, hosts and secrets": {
			&manifest.Browser{Sites: []string{"site"}, Hosts: []string{"fonts.googleapis.com", "fonts.gstatic.com"}, Secrets: []string{"SITE_USER", "SITE_PASSWORD"}},
			"a web browser that can open the site you name for site and its subdomains, and fonts.googleapis.com, fonts.gstatic.com; it can type SITE_USER, SITE_PASSWORD into the site's sign-in forms (it never sees them)",
		},
		"public": {
			&manifest.Browser{Sites: []string{"site"}, Scope: manifest.BrowserScopePublic},
			"a web browser that can open the site you name for site and its subdomains, and ANY public site (policy-gated)",
		},
	}
	for name, c := range cases {
		if got := BrowserLine(c.b); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", name, got, c.want)
		}
	}
	if BrowserLine(nil) != "" || BrowserTools(nil) != nil {
		t.Fatal("no browser says nothing")
	}
}

// The tools are the runner's allowlist: browser_evaluate only for a
// browser that types no secret, never a tool that runs code in the server.
func TestBrowserToolsAreTheRunnersAllowlist(t *testing.T) {
	plain := BrowserTools(&manifest.Browser{Sites: []string{"site"}})
	keyed := BrowserTools(&manifest.Browser{Sites: []string{"site"}, Secrets: []string{"K"}})
	if !slices.Contains(plain, "browser_evaluate") || slices.Contains(keyed, "browser_evaluate") {
		t.Fatalf("browser_evaluate: plain %v, keyed %v", plain, keyed)
	}
	for _, t2 := range []string{"browser_run_code_unsafe", "browser_file_upload"} {
		if slices.Contains(plain, t2) {
			t.Errorf("%s passed the allowlist", t2)
		}
	}
}
