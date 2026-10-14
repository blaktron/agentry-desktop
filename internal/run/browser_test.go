package run

import (
	"strings"
	"testing"

	"github.com/blaktron/agentry/pkg/manifest"
)

// No run In Agentry carries a browser yet (agentry-desktop#878): a release
// that declares one is refused there, in the site's words, and one that
// does not is left alone.
func TestARunInAgentryOfABrowserReleaseIsRefused(t *testing.T) {
	m := &manifest.Manifest{Capabilities: manifest.Capabilities{Browser: &manifest.Browser{Sites: []string{"site"}}}}
	err := RefuseHostedBrowser(m)
	if err == nil || !strings.Contains(err.Error(), "a run In Agentry has no browser yet") {
		t.Fatalf("got %v", err)
	}
	if err := RefuseHostedBrowser(&manifest.Manifest{}); err != nil {
		t.Fatalf("a release without a browser: %v", err)
	}
}
