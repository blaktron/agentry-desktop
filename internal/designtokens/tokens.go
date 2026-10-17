// Package designtokens reads the desktop's design token table out of
// frontend/src/style.css and holds it to the site's.
//
// The desktop's stylesheet copies the site's token table by design: one product, one palette. A copy drifts unless
// something compares it, so the tests in this package do two things `go test
// ./...` runs in CI:
//
//   - parity: every token name the desktop's table shares with a pasted copy
//     of the site's (testdata/site-globals.css) carries the site's value, in
//     the light palette and in the dark one;
//   - contrast: the site's contrast audit (which this file ports) passes over the
//     desktop's own table, so a value that is equal but used somewhere the
//     site does not use it is still measured.
//
// The parser and the WCAG maths are a port of the site's, so the desktop
// and the site read a stylesheet the same way.
package designtokens

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Tokens maps a custom property name ("--color-bg") to its declared value.
type Tokens map[string]string

// Palettes is a stylesheet's two schemes. Dark is the light table with the
// prefers-color-scheme: dark override applied, so a token the override does
// not mention is inherited rather than lost.
type Palettes struct {
	Light Tokens
	Dark  Tokens
}

var (
	commentRe   = regexp.MustCompile(`(?s)/\*.*?\*/`)
	darkMediaRe = regexp.MustCompile(`@media\s*\([^)]*prefers-color-scheme\s*:\s*dark[^)]*\)\s*`)
	themeRe     = regexp.MustCompile(`@theme(?:\s+static)?\s*`)
	rootRe      = regexp.MustCompile(`(?m)(?:^|[}\s]):root\s*`)
	nestedRe    = regexp.MustCompile(`\{[^{}]*\}`)
	declRe      = regexp.MustCompile(`(--[\w-]+)\s*:\s*([^;{}]+);`)
	hexRe       = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)
	percentRe   = regexp.MustCompile(`^([\d.]+)%$`)
)

// blockSpan finds the first brace-balanced block whose header matches re and
// returns its body and the span [start, end) the header and block occupy.
func blockSpan(css string, re *regexp.Regexp) (body string, start, end int, ok bool) {
	loc := re.FindStringIndex(css)
	if loc == nil {
		return "", 0, 0, false
	}
	open := strings.IndexByte(css[loc[0]:], '{')
	if open < 0 {
		return "", 0, 0, false
	}
	open += loc[0]
	depth := 0
	for i := open; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[open+1 : i], loc[0], i + 1, true
			}
		}
	}
	return "", 0, 0, false
}

// declarations reads `--name: value;` pairs, ignoring anything inside a
// nested rule.
func declarations(body string) Tokens {
	flat := nestedRe.ReplaceAllString(body, " ")
	out := Tokens{}
	for _, m := range declRe.FindAllStringSubmatch(flat, -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

// Parse reads both palettes out of a stylesheet. Light is the @theme table
// (the site's) plus the first :root block outside the dark media query (the
// desktop keeps its whole light table there); dark is light with the media
// query's :root applied.
func Parse(raw string) Palettes {
	css := commentRe.ReplaceAllString(raw, " ")
	outside := css
	darkBody, start, end, hasDark := blockSpan(css, darkMediaRe)
	if hasDark {
		// Cut the media query out first, so its :root is not taken for the
		// light one.
		outside = css[:start] + css[end:]
	}
	light := Tokens{}
	if body, _, _, ok := blockSpan(outside, themeRe); ok {
		for k, v := range declarations(body) {
			light[k] = v
		}
	}
	if body, _, _, ok := blockSpan(outside, rootRe); ok {
		for k, v := range declarations(body) {
			light[k] = v
		}
	}
	dark := Tokens{}
	for k, v := range light {
		dark[k] = v
	}
	if hasDark {
		if body, _, _, ok := blockSpan(darkBody, rootRe); ok {
			for k, v := range declarations(body) {
				dark[k] = v
			}
		}
	}
	return Palettes{Light: light, Dark: dark}
}

// RGB is a colour in gamma-encoded sRGB, 0-255 per channel.
type RGB [3]float64

// Hex parses a three- or six-digit hex colour.
func Hex(value string) (RGB, bool) {
	m := hexRe.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil {
		return RGB{}, false
	}
	h := m[1]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	var c RGB
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseUint(h[2*i:2*i+2], 16, 8)
		if err != nil {
			return RGB{}, false
		}
		c[i] = float64(v)
	}
	return c, true
}

func channel(v float64) float64 {
	s := v / 255
	if s <= 0.03928 {
		return s / 12.92
	}
	return math.Pow((s+0.055)/1.055, 2.4)
}

// Luminance is WCAG 2.x relative luminance.
func Luminance(c RGB) float64 {
	return 0.2126*channel(c[0]) + 0.7152*channel(c[1]) + 0.0722*channel(c[2])
}

// Ratio is the WCAG 2.x contrast ratio between two colours.
func Ratio(a, b RGB) float64 {
	la, lb := Luminance(a), Luminance(b)
	hi, lo := math.Max(la, lb), math.Min(la, lb)
	return (hi + 0.05) / (lo + 0.05)
}

// mixOver composites color-mix(in srgb, colour p, transparent) over ground,
// the way a browser paints it.
func mixOver(colour RGB, p float64, ground RGB) RGB {
	var out RGB
	for i := range out {
		out[i] = p*colour[i] + (1-p)*ground[i]
	}
	return out
}

// The grounds a badge, a link or a paragraph can sit on.
var grounds = []string{"bg", "surface", "raised", "accent-soft", "warm-soft"}

// Colours that tint themselves into the ground and set their own text on
// that tint.
var tintedBadges = []string{"sev-critical", "sev-high", "sev-medium", "sev-low", "sev-info", "ok", "warm", "unscanned"}

var highlights = []string{"keyword", "string", "comment", "number", "name", "type", "error"}

// AuditPalette measures every pair the site's contrast gate measures, in one
// palette, and returns one line per pair below its floor or per token the
// table never defines as a hex value. Empty means pass.
func AuditPalette(mode string, t Tokens) []string {
	var failures []string
	reported := map[string]bool{}
	missing := func(name string) {
		if !reported[name] {
			reported[name] = true
			failures = append(failures, fmt.Sprintf("%s: %s is not defined as a hex value", mode, name))
		}
	}
	get := func(name string) (RGB, bool) {
		c, ok := Hex(t[name])
		if !ok {
			missing(name)
		}
		return c, ok
	}
	colour := func(name string) (RGB, bool) { return get("--color-" + name) }
	check := func(label string, fg RGB, fgOK bool, bg RGB, bgOK bool, floor float64) {
		if !fgOK || !bgOK {
			return
		}
		if r := Ratio(fg, bg); r < floor {
			failures = append(failures, fmt.Sprintf("%s: %s is %.2f:1, floor %.1f", mode, label, r, floor))
		}
	}
	pair := func(label, fg, bg string, floor float64) {
		f, fok := colour(fg)
		b, bok := colour(bg)
		check(label, f, fok, b, bok, floor)
	}

	tint := -1.0
	if m := percentRe.FindStringSubmatch(strings.TrimSpace(t["--badge-tint"])); m != nil {
		v, _ := strconv.ParseFloat(m[1], 64)
		tint = v / 100
	} else {
		failures = append(failures, mode+": --badge-tint is not defined as a percentage")
	}

	for _, text := range []string{"fg", "lead", "muted"} {
		for _, g := range grounds {
			pair(text+" on "+g, text, g, 4.5)
		}
	}
	for _, fill := range []string{"accent", "accent-hover"} {
		pair("accent-fg on "+fill, "accent-fg", fill, 4.5)
	}
	for _, g := range []string{"bg", "surface", "raised"} {
		pair("accent on "+g, "accent", g, 4.5)
	}
	for _, g := range []string{"accent-soft", "warm-soft"} {
		pair("accent-hover on "+g, "accent-hover", g, 4.5)
	}
	for _, g := range []string{"bg", "surface", "warm-soft"} {
		pair("warm on "+g, "warm", g, 4.5)
	}
	pair("bg on sev-critical (solid critical badge)", "bg", "sev-critical", 4.5)
	for _, fill := range []string{"ok-strong", "danger-strong"} {
		pair("solid-fg on "+fill, "solid-fg", fill, 4.5)
	}
	pair("bg on sev-high (.agy-invisible)", "bg", "sev-high", 4.5)

	if tint >= 0 {
		for _, badge := range tintedBadges {
			for _, g := range grounds {
				fill, fok := colour(badge)
				under, uok := colour(g)
				if !fok || !uok {
					continue
				}
				check(badge+" on its own tint over "+g, fill, true, mixOver(fill, tint, under), true, 4.5)
			}
		}
		for _, g := range grounds {
			fill, fok := colour("accent")
			under, uok := colour(g)
			text, tok := colour("accent-hover")
			if !fok || !uok || !tok {
				continue
			}
			check("accent-hover on the accent tint over "+g, text, true, mixOver(fill, tint, under), true, 4.5)
		}
	}

	for _, name := range highlights {
		h, hok := get("--hl-" + name)
		s, sok := colour("surface")
		check("hl-"+name+" on surface", h, hok, s, sok, 4.5)
	}

	pair("bg (the mark's check) on accent", "bg", "accent", 3.0)

	if c, cok := colour("sev-critical"); cok {
		if h, hok := colour("sev-high"); hok {
			if r := Ratio(c, h); r < 1.4 {
				failures = append(failures, fmt.Sprintf("%s: sev-critical vs sev-high is %.2f:1, floor 1.4", mode, r))
			}
		}
	}
	return failures
}

// Audit runs AuditPalette over both palettes of a stylesheet.
func Audit(css string) []string {
	p := Parse(css)
	return append(AuditPalette("light", p.Light), AuditPalette("dark", p.Dark)...)
}

// Drift lists every token name both palettes define whose value differs
// between want (the site's) and got (the desktop's), in one scheme, except
// the names in skip. Values are compared with surrounding space trimmed and
// hex digits lower-cased.
func Drift(mode string, want, got Tokens, skip map[string]string) []string {
	var out []string
	for name, w := range want {
		g, shared := got[name]
		if !shared {
			continue
		}
		if _, skipped := skip[name]; skipped {
			continue
		}
		if normalise(w) != normalise(g) {
			out = append(out, fmt.Sprintf("%s: %s is %q here, %q on the site", mode, name, g, w))
		}
	}
	return out
}

func normalise(v string) string {
	return strings.ToLower(strings.Join(strings.Fields(v), " "))
}
