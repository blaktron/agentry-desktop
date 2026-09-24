package designtokens

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The desktop's stylesheet, relative to this package.
var desktopCSS = filepath.Join("..", "..", "frontend", "src", "style.css")

// The pasted copy of the site's token table (see its header).
var siteCSS = filepath.Join("testdata", "site-globals.css")

// Token names the two tables both define but spell differently on purpose,
// with the reason. Anything else that differs is drift.
var deliberate = map[string]string{
	"--font-sans": "the site reaches IBM Plex through next/font's --font-plex-* variables; the desktop through fonts.css by family name",
	"--font-mono": "as --font-sans",
}

// The families of names the desktop must carry whenever the site does:
// dropping one is drift too, not only changing it.
var mustCarry = regexp.MustCompile(`^--(color|hl|radius|shadow)-|^--badge-tint$`)

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// follow lists every way desk fails to follow site: a shared name with
// another value in either scheme, or a name of the must-carry families the
// site defines and the desktop does not.
func follow(site, desk Palettes) []string {
	var problems []string
	problems = append(problems, Drift("light", site.Light, desk.Light, deliberate)...)
	problems = append(problems, Drift("dark", site.Dark, desk.Dark, deliberate)...)
	for name := range site.Light {
		if _, ok := desk.Light[name]; !ok && mustCarry.MatchString(name) {
			problems = append(problems, "the desktop does not define "+name)
		}
	}
	slices.Sort(problems)
	return problems
}

func TestDesktopTableFollowsTheSite(t *testing.T) {
	site := Parse(read(t, siteCSS))
	desk := Parse(read(t, desktopCSS))
	if len(site.Light) < 20 || len(desk.Light) < 20 {
		t.Fatalf("the parser found %d site tokens and %d desktop tokens; expected the full tables", len(site.Light), len(desk.Light))
	}
	problems := follow(site, desk)
	for _, p := range problems {
		t.Error(p)
	}
	if len(problems) > 0 {
		t.Log("frontend/src/style.css follows the site's table: take the site's value, or record why not (a D-nnn entry and the deliberate list here)")
	}
}

func TestDesktopTableClearsTheContrastFloors(t *testing.T) {
	for _, f := range Audit(read(t, desktopCSS)) {
		t.Error(f)
	}
}

func TestSiteTableClearsTheContrastFloors(t *testing.T) {
	// The pasted copy passes the ported audit, as the original passes the
	// site's: a port that disagreed with the site would show here first.
	for _, f := range Audit(read(t, siteCSS)) {
		t.Error(f)
	}
}

func TestParseReadsBothPalettes(t *testing.T) {
	for _, path := range []string{desktopCSS, siteCSS} {
		p := Parse(read(t, path))
		if _, ok := Hex(p.Light["--color-bg"]); !ok {
			t.Errorf("%s: light --color-bg = %q", path, p.Light["--color-bg"])
		}
		if p.Dark["--color-bg"] == p.Light["--color-bg"] {
			t.Errorf("%s: the dark override did not move --color-bg", path)
		}
		// Inherited, not lost: solid-fg is not overridden in dark.
		if p.Dark["--color-solid-fg"] != p.Light["--color-solid-fg"] || p.Dark["--color-solid-fg"] == "" {
			t.Errorf("%s: dark --color-solid-fg = %q, light %q", path, p.Dark["--color-solid-fg"], p.Light["--color-solid-fg"])
		}
		if p.Light["--badge-tint"] != "8%" || p.Dark["--badge-tint"] != "16%" {
			t.Errorf("%s: --badge-tint light %q dark %q", path, p.Light["--badge-tint"], p.Dark["--badge-tint"])
		}
	}
}

// The gate itself: each check fails when it should.

func TestAuditFailsWhenLightMediumGoesBack(t *testing.T) {
	css := strings.Replace(read(t, desktopCSS), "--color-sev-medium: #9f4707;", "--color-sev-medium: #b45309;", 1)
	failures := Audit(css)
	if len(failures) == 0 {
		t.Fatal("no failure for the old light medium")
	}
	for _, f := range failures {
		if !strings.HasPrefix(f, "light: sev-medium on its own tint") {
			t.Errorf("unexpected failure: %s", f)
		}
	}
	// The site's test asserts these two numbers; the port measures the same.
	for _, want := range []string{
		"light: sev-medium on its own tint over bg is 4.19:1, floor 4.5",
		"light: sev-medium on its own tint over warm-soft is 3.88:1, floor 4.5",
	} {
		if !slices.Contains(failures, want) {
			t.Errorf("missing %q in %q", want, failures)
		}
	}
}

func TestAuditFailsWhenATokenIsMissing(t *testing.T) {
	css := regexp.MustCompile(`\s*--color-lead: #3d3328;`).ReplaceAllString(read(t, desktopCSS), "")
	if !slices.Contains(Audit(css), "light: --color-lead is not defined as a hex value") {
		t.Error("a missing --color-lead was not reported")
	}
}

func TestAuditFailsWhenCriticalAndHighConverge(t *testing.T) {
	css := strings.Replace(read(t, desktopCSS), "--color-sev-critical: #7f1d1d;", "--color-sev-critical: #991b1b;", 1)
	found := false
	for _, f := range Audit(css) {
		found = found || strings.Contains(f, "sev-critical vs sev-high")
	}
	if !found {
		t.Error("critical converging on high was not reported")
	}
}

func TestFollowCatchesAChangedAndADroppedToken(t *testing.T) {
	site := Parse(read(t, siteCSS))
	changed := Parse(strings.Replace(read(t, desktopCSS), "--color-border: #33291f;", "--color-border: #292524;", 1))
	if got := follow(site, changed); len(got) != 1 || !strings.HasPrefix(got[0], "dark: --color-border ") {
		t.Errorf("follow = %q, want the dark --color-border alone", got)
	}
	dropped := Parse(regexp.MustCompile(`\s*--color-warm-soft: #f4ece2;[^\n]*`).ReplaceAllString(read(t, desktopCSS), ""))
	if got := follow(site, dropped); !slices.Contains(got, "the desktop does not define --color-warm-soft") {
		t.Errorf("follow = %q, want the dropped --color-warm-soft", got)
	}
	fonts := Parse(strings.Replace(read(t, desktopCSS), `--font-mono: "IBM Plex Mono"`, `--font-mono: "Other Mono"`, 1))
	if got := follow(site, fonts); len(got) != 0 {
		t.Errorf("a deliberate difference was reported: %q", got)
	}
}
