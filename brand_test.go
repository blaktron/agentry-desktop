package main

// The Agentry mark is drawn in three places this repository ships: the
// in-app mark (frontend/src/brand.tsx), the app icon's source
// (build/appicon.svg) and its small-size source (build/appicon-small.svg).
// They carry the same geometry as the site's lib/brand-mark.ts, the faceted
// shield with the delta A (agentry-desktop#367); this fails when one drifts
// or the shield-and-check it replaced comes back.

import (
	"os"
	"strings"
	"testing"
)

const (
	markShield       = "M24 3 41 10.5 V 23 C 41 34.5 34 42.5 24 46.5 C 14 42.5 7 34.5 7 23 V 10.5 Z"
	markFacet        = "M24 3 41 10.5 V 23 C 41 34.5 34 42.5 24 46.5 Z"
	markFacetOpacity = "0.22"
	markA            = "M24 9.5 L35 38 Q29.5 32 24 32 Q18.5 32 13 38 Z M24 19.2 L27.2 27.4 H20.8 Z"
	oldCheck         = "M15.5 24.5 L 21.5 30.5 L 32.5 18.5"
)

func TestTheMarkIsTheSameEverywhere(t *testing.T) {
	for _, file := range []string{"frontend/src/brand.tsx", "build/appicon.svg", "build/appicon-small.svg"} {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		src := string(b)
		for name, want := range map[string]string{
			"the shield":           markShield,
			"the facet":            markFacet,
			"the facet's opacity":  markFacetOpacity,
			"the A":                markA,
			"the A's evenodd rule": "evenodd",
		} {
			if !strings.Contains(src, want) {
				t.Errorf("%s does not carry %s (%q)", file, name, want)
			}
		}
		if strings.Contains(src, oldCheck) {
			t.Errorf("%s still draws the shield-and-check mark", file)
		}
	}
}
