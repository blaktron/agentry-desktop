package main

import (
	"strings"
	"testing"

	"github.com/blaktron/agentry-cli/pkg/configdir"
)

// Home's layout through its binding (plan desktop-home-and-library.md §4.4,
// D-067; agentry-desktop#658): the defaults until something is changed,
// one change per call, and a view that names every card in order.
func TestHomeBindingShowsHidesAndResets(t *testing.T) {
	t.Setenv(configdir.EnvVar, t.TempDir())
	h := &HomeAPI{}
	order := func(v *HomeLayoutView) string {
		var b []string
		for _, c := range v.Cards {
			k := c.Kind
			if c.Hidden {
				k += "(hidden)"
			}
			b = append(b, k)
		}
		return strings.Join(b, ",")
	}
	v, err := h.Layout()
	if err != nil {
		t.Fatal(err)
	}
	if got := order(v); got != "recent-runs,installed,profile,run-defaults" || v.Newer {
		t.Fatalf("Layout() = %s newer=%v", got, v.Newer)
	}
	if v, err = h.SetShown("profile", false); err != nil {
		t.Fatal(err)
	}
	if got := order(v); got != "recent-runs,installed,profile(hidden),run-defaults" {
		t.Fatalf("SetShown answered %s", got)
	}
	if _, err := h.SetShown("pinned", false); err == nil {
		t.Fatal("a pinned card was shown or hidden by kind")
	}
	if v, err = h.Reset(); err != nil {
		t.Fatal(err)
	}
	if got := order(v); got != "recent-runs,installed,profile,run-defaults" {
		t.Fatalf("Reset answered %s", got)
	}
}
