package hostedrun

import (
	"net/http"
	"strings"
	"testing"
)

// OAuth connections M9 (agentry-desktop#667): run create's 409
// connection-missing names the servers still to connect, and the site's
// Connect tools for the exact version this computer runs is the page that
// connects them. Nothing here carries a token: the vault returns none.

func TestConnectionMissingNamesTheServers(t *testing.T) {
	const linear = "https://mcp.linear.app/mcp"
	f := &fakeRuns{enabled: true, createRefusal: func(w http.ResponseWriter) {
		problemJSON(w, http.StatusConflict, SlugConnectionMissing, "A tool is not connected",
			"The release's tools sign in by OAuth, and you have not connected "+linear+" for this version. Connect them, then start the run.",
			map[string]any{"missing": []string{linear}, "connections": []string{linear, "npm:@acme/mcp"}})
	}}
	c := New(f.start(t), testToken)
	_, err := c.Create(t.Context(), CreateRequest{Ref: "acme/linear@0.1.0"})
	ref := RefusalOf(err)
	if ref == nil || ref.Slug != SlugConnectionMissing || ref.Status != http.StatusConflict {
		t.Fatalf("refusal = %+v (%v)", ref, err)
	}
	if strings.Join(ref.Missing, ",") != linear {
		t.Fatalf("missing %v", ref.Missing)
	}
}

func TestConnectPageIsTheSitesSetupPinnedToTheVersion(t *testing.T) {
	for _, tc := range []struct{ reg, ref, want string }{
		{"https://dev.secureagentry.net", "acme/linear@0.1.0", "https://dev.secureagentry.net/acme/linear/run/setup?version=0.1.0&step=tools"},
		// The registry's origin alone: a path or a query on it goes.
		{"https://registry.example:8443/api/v1?x=1", "acme/linear@1.2.0-rc.1", "https://registry.example:8443/acme/linear/run/setup?version=1.2.0-rc.1&step=tools"},
		// Build metadata is encoded: a query reads a bare "+" as a space.
		{"https://registry.example", "acme/linear@1.2.0+b1", "https://registry.example/acme/linear/run/setup?version=1.2.0%2Bb1&step=tools"},
	} {
		got, err := ConnectPage(tc.reg, tc.ref)
		if err != nil || got != tc.want {
			t.Errorf("ConnectPage(%q, %q) = %q, %v; want %q", tc.reg, tc.ref, got, err, tc.want)
		}
	}
}

func TestConnectPageNeedsAnExactVersionAndARegistry(t *testing.T) {
	for _, tc := range []struct{ reg, ref string }{
		{"https://registry.example", "acme/linear"},
		{"https://registry.example", "acme/linear@^1.0"},
		{"https://registry.example", "acme/linear@beta"},
		{"https://registry.example", "acme/linear@1"},
		{"https://registry.example", "acme/linear@1.x"},
		{"https://registry.example", "not a ref"},
		{"file:///etc", "acme/linear@1.0.0"},
		{"javascript:alert(1)", "acme/linear@1.0.0"},
		{"", "acme/linear@1.0.0"},
	} {
		if got, err := ConnectPage(tc.reg, tc.ref); err == nil {
			t.Errorf("ConnectPage(%q, %q) = %q, want a refusal", tc.reg, tc.ref, got)
		}
	}
}
