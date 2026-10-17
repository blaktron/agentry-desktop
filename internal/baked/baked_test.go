package baked

import (
	"strings"
	"testing"

	"github.com/blaktron/agentry-desktop/internal/selfupdate"
	"github.com/blaktron/agentry-desktop/internal/settings"
)

// The read-back names exactly the two variables a release sets, each on its
// own line, so a script can compare them whole.
func TestMainPrintsTheBakedValues(t *testing.T) {
	var b strings.Builder
	if code := Main(&b); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := "registry " + settings.DefaultRegistryURL + "\nchannel " + selfupdate.ManifestURL + "\n"
	if b.String() != want {
		t.Fatalf("printed %q, want %q", b.String(), want)
	}
}
