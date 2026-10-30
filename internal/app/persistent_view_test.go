package app

import (
	"path/filepath"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
)

// The page's view is kept for every agent and read back; a view this app
// lacks is refused and leaves the kept one as it was.
func TestThePageViewIsKeptAndARefusalKeepsTheLastOne(t *testing.T) {
	t.Setenv(configdir.EnvVar, filepath.Join(t.TempDir(), "config"))
	p := &PersistentAPI{}
	if v, err := p.PageView(); err != nil || v != "terminal" {
		t.Fatalf("default: %q %v", v, err)
	}
	if v, err := p.SetPageView("focus"); err != nil || v != "focus" {
		t.Fatalf("focus: %q %v", v, err)
	}
	if v, err := p.SetPageView("cinema"); err == nil || v != "focus" {
		t.Fatalf("a view this app lacks: %q %v", v, err)
	}
	if v, _ := p.PageView(); v != "focus" {
		t.Fatalf("read back %q", v)
	}
}
