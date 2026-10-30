// Package highlights is what each release of Agentry Desktop brings, in a
// few plain lines for the person using it: the What's new screen shows them
// after the app moves to a newer version, and the release's notes and the
// download page carry them too.
//
// The lines are compiled into the app (highlights.json), so a newer build
// already holds every older version's lines. The screen needs no network,
// works wherever the app was installed from, and shows only text the app's
// own signature covers.
package highlights

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// MaxLines is how many lines one version may carry, and MaxLineLength how
// long each may be, in characters.
const (
	MaxLines      = 4
	MaxLineLength = 120
)

// Entry is one version's highlights. Version is X.Y.Z: every dev
// prerelease of a version shares its entry.
type Entry struct {
	Version    string   `json:"version"`
	Highlights []string `json:"highlights"`
}

type file struct {
	Versions []Entry `json:"versions"`
}

//go:embed highlights.json
var embedded []byte

var load = sync.OnceValues(func() ([]Entry, error) { return Parse(embedded) })

// All is every version's entry, newest first. An error means the compiled
// file breaks its own rules, which its test refuses before a build.
func All() ([]Entry, error) { return load() }

// Parse reads a highlights file and checks every rule: versions are X.Y.Z,
// newest first, each once, with one to MaxLines lines that CheckLine accepts.
func Parse(data []byte) ([]Entry, error) {
	var f file
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("highlights: %w", err)
	}
	if dec.More() {
		return nil, errors.New("highlights: more than one JSON value")
	}
	for i, e := range f.Versions {
		if _, ok := parseTriple(e.Version); !ok || e.Version != strings.TrimSpace(e.Version) || strings.Count(e.Version, ".") != 2 || strings.ContainsAny(e.Version, "-+v") {
			return nil, fmt.Errorf("highlights: %q is not a version of the form X.Y.Z", e.Version)
		}
		if i > 0 && Compare(f.Versions[i-1].Version, e.Version) <= 0 {
			return nil, fmt.Errorf("highlights: %s follows %s: versions go newest first, each once", e.Version, f.Versions[i-1].Version)
		}
		if n := len(e.Highlights); n == 0 || n > MaxLines {
			return nil, fmt.Errorf("highlights: %s has %d lines, and a version has one to %d", e.Version, n, MaxLines)
		}
		for _, line := range e.Highlights {
			if err := CheckLine(line); err != nil {
				return nil, fmt.Errorf("highlights: %s: %w", e.Version, err)
			}
		}
	}
	return f.Versions, nil
}

var (
	// Markdown or HTML: the lines are shown as text, never rendered.
	markupRE = regexp.MustCompile("[*_`#<>\\[\\]|~]|^\\s*[-+] |^\\s*\\d+[.)] ")
	// A commit subject's prefix, "feat:" or "fix(run card):".
	prefixRE = regexp.MustCompile(`(?i)^(feat|fix|chore|docs|refactor|perf|test|tests|build|ci|style|revert|deps)(\([^)]*\))?!?:`)
	// A register id, written for the people who build the app.
	registerRE = regexp.MustCompile(`\b(DEV|DR|D)-\d+\b`)
	// An address or a host name. The product names its places in words.
	hostRE = regexp.MustCompile(`(?i)\bhttps?://|\b[a-z0-9-]+(\.[a-z0-9-]+)*\.(net|com|org|io|dev|ai|app|internal|local|lan)\b|\bagentry-[a-z0-9-]+`)
)

// CheckLine says why a line cannot be a highlight, or nil. A line is one
// plain sentence for someone using the app, ending in a full stop, a
// question mark or an exclamation mark.
func CheckLine(line string) error {
	switch {
	case strings.TrimSpace(line) != line || line == "":
		return fmt.Errorf("%q: a line has no leading or trailing space and is not empty", line)
	case utf8.RuneCountInString(line) > MaxLineLength:
		return fmt.Errorf("%q: %d characters, and a line has at most %d", line, utf8.RuneCountInString(line), MaxLineLength)
	case strings.ContainsAny(line, "\n\r\t"):
		return fmt.Errorf("%q: a line is one line", line)
	case prefixRE.MatchString(line):
		return fmt.Errorf("%q: a commit prefix; say what the person can now do", line)
	case markupRE.MatchString(line):
		return fmt.Errorf("%q: Markdown, HTML or a reference; a line is plain text", line)
	case registerRE.MatchString(line):
		return fmt.Errorf("%q: a register id; say what the person can now do", line)
	case hostRE.MatchString(line):
		return fmt.Errorf("%q: an address or a host name; name the place in words", line)
	case !strings.ContainsAny(line[len(line)-1:], ".?!"):
		return fmt.Errorf("%q: a line is a sentence, ending in a full stop", line)
	}
	return nil
}

type triple [3]int

// parseTriple reads the X.Y.Z a version starts with: "v0.6.12-dev.3",
// "0.6.12" and "0.6.12-4-gabc1234" are all 0.6.12.
func parseTriple(v string) (triple, bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return triple{}, false
	}
	var t triple
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p != strconv.Itoa(n) {
			return triple{}, false
		}
		t[i] = n
	}
	return t, true
}

// Version is the X.Y.Z a build's version belongs to, and false for a
// version that names no release (the 0.0.0-dev of a build from source).
func Version(v string) (string, bool) {
	t, ok := parseTriple(v)
	if !ok || t == (triple{}) {
		return "", false
	}
	return fmt.Sprintf("%d.%d.%d", t[0], t[1], t[2]), true
}

// Compare orders two versions by their X.Y.Z: negative, zero or positive as
// a is older than, the same as or newer than b. A version that cannot be read
// is older than every other.
func Compare(a, b string) int {
	ta, oka := parseTriple(a)
	tb, okb := parseTriple(b)
	switch {
	case !oka && !okb:
		return 0
	case !oka:
		return -1
	case !okb:
		return 1
	}
	for i := range ta {
		if ta[i] != tb[i] {
			return ta[i] - tb[i]
		}
	}
	return 0
}

// Between is the entries newer than after and no newer than upTo, newest
// first: at most limit of them, and how many more there were. An empty
// after means every entry up to upTo.
func Between(entries []Entry, after, upTo string, limit int) (shown []Entry, more int) {
	for _, e := range entries {
		if Compare(e.Version, upTo) > 0 || (after != "" && Compare(e.Version, after) <= 0) {
			continue
		}
		if len(shown) < limit {
			shown = append(shown, e)
		} else {
			more++
		}
	}
	return shown, more
}

// For is the entry of version's X.Y.Z, if there is one.
func For(entries []Entry, version string) (Entry, bool) {
	for _, e := range entries {
		if Compare(e.Version, version) == 0 {
			return e, true
		}
	}
	return Entry{}, false
}
